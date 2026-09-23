#!/usr/bin/env python3
"""Validate Forecast Ledger v2 JSON or YAML documents.

JSON Schema validates local shape. This tool enforces cross-record and exact
arithmetic rules: revision binding, domain compatibility, probability sums,
bin continuity, monotonic distributions, references, acyclicity, chronology,
artifact digests, lifecycle transitions, and revealed commitments.
"""

from __future__ import annotations

import argparse
import hashlib
import json
import re
import sys
from datetime import date, datetime, timedelta
from decimal import Decimal, InvalidOperation
from pathlib import Path
from typing import Any
from zoneinfo import ZoneInfo, ZoneInfoNotFoundError

from forecast_crypto import canonicalize, forecast_lifecycle_target, reveal_forecast
from jsonschema import Draft202012Validator, FormatChecker
from ruamel.yaml import YAML
from ruamel.yaml.error import YAMLError

ONE = Decimal(1)
CANONICAL_DECIMAL = re.compile(
    r"^(?:0|-?[1-9][0-9]*|-?(?:0|[1-9][0-9]*)\.[0-9]*[1-9])$"
)

INTEGRITY_TRANSITIONS = {
    "unanchored": {"retained"},
    "retained": {"pending", "verified"},
    "pending": {"pending", "verified", "failed"},
    "failed": {"pending", "verified"},
    "verified": {"verified"},
}


def pointer_join(path: str, token: str | int) -> str:
    escaped = str(token).replace("~", "~0").replace("/", "~1")
    return f"{path}/{escaped}" if path else f"/{escaped}"


def normalize_pointer(path: str) -> str:
    """Convert legacy $.items[0].field paths to RFC 6901 JSON Pointers."""

    if not path.startswith("$"):
        return path
    pointer = ""
    for name, bracket in re.findall(r"\.([A-Za-z0-9_*]+)|\[([^]]+)\]", path[1:]):
        token = name or bracket
        pointer = pointer_join(pointer, token)
    return pointer


class Problems:
    def __init__(self, locations: dict[str, tuple[int, int]] | None = None) -> None:
        self.items: list[str] = []
        self.locations = locations or {}

    def add(self, path: str, message: str) -> None:
        pointer = normalize_pointer(path)
        location = self.locations.get(pointer)
        suffix = f" [line {location[0]}, column {location[1]}]" if location else ""
        self.items.append(f"{pointer or '/'}{suffix}: {message}")

    def unique(self, values: list[Any], path: str) -> None:
        seen: set[Any] = set()
        for index, value in enumerate(values):
            if value in seen:
                self.add(f"{path}[{index}]", f"duplicate value {value!r}")
            seen.add(value)


def load_document(path: Path) -> Any:
    if path.suffix.lower() == ".json":
        return json.loads(path.read_text(encoding="utf-8"))
    yaml = YAML(typ="safe")
    return yaml.load(path.read_text(encoding="utf-8"))


class DocumentSyntaxError(ValueError):
    def __init__(self, line: int, column: int) -> None:
        super().__init__("document syntax is invalid")
        self.line = line
        self.column = column


def _plain_and_locations(value: Any) -> tuple[Any, dict[str, tuple[int, int]]]:
    locations: dict[str, tuple[int, int]] = {}

    def convert(node: Any, pointer: str) -> Any:
        if isinstance(node, dict):
            result: dict[str, Any] = {}
            for key, item in node.items():
                child = pointer_join(pointer, key)
                try:
                    line, column = node.lc.value(key)
                    locations[child] = (line + 1, column + 1)
                except (AttributeError, KeyError, TypeError, ValueError):
                    pass
                result[str(key)] = convert(item, child)
            return result
        if isinstance(node, list):
            result = []
            for index, item in enumerate(node):
                child = pointer_join(pointer, index)
                try:
                    line, column = node.lc.item(index)
                    locations[child] = (line + 1, column + 1)
                except (AttributeError, IndexError, TypeError, ValueError):
                    pass
                result.append(convert(item, child))
            return result
        return node

    return convert(value, ""), locations


def load_document_with_locations(path: Path) -> tuple[Any, dict[str, tuple[int, int]]]:
    text = path.read_text(encoding="utf-8")
    yaml = YAML(typ="rt")
    try:
        value = yaml.load(text)
    except YAMLError as error:
        mark = getattr(error, "problem_mark", None)
        line = mark.line + 1 if mark else 1
        column = mark.column + 1 if mark else 1
        raise DocumentSyntaxError(line, column) from None
    return _plain_and_locations(value)


def parse_time(value: str) -> datetime:
    return datetime.fromisoformat(value.replace("Z", "+00:00"))


def check_schema(instance: Any, schema: dict[str, Any], problems: Problems) -> None:
    validator = Draft202012Validator(schema, format_checker=FormatChecker())
    for error in sorted(validator.iter_errors(instance), key=lambda item: list(item.path)):
        path = ""
        for part in error.path:
            path = pointer_join(path, part)
        if error.validator == "required" and isinstance(error.instance, dict):
            for missing in error.validator_value:
                if missing not in error.instance:
                    problems.add(pointer_join(path, missing), "required property is missing")
            continue
        if error.validator == "additionalProperties" and isinstance(error.instance, dict):
            allowed = set(error.schema.get("properties", {}))
            patterns = [re.compile(item) for item in error.schema.get("patternProperties", {})]
            for key in error.instance:
                if key not in allowed and not any(pattern.search(key) for pattern in patterns):
                    problems.add(pointer_join(path, key), "unknown property is not allowed")
            continue
        messages = {
            "type": "must have the required type",
            "const": "must equal the required constant",
            "enum": "must be one of the allowed values",
            "pattern": "does not match the required pattern",
            "format": "does not match the required format",
            "minItems": f"must contain at least {error.validator_value} item(s)",
            "oneOf": "must match exactly one allowed shape",
            "not": "contains a property forbidden for this state",
        }
        problems.add(path, messages.get(error.validator, "does not satisfy the schema constraint"))


def parse_scalar(value: Any, kind: str) -> Any:
    if kind == "binary":
        if not isinstance(value, bool):
            raise ValueError("must be a boolean")
        return value
    if not isinstance(value, str):
        raise ValueError("must be a string")
    if kind == "numeric":
        if not CANONICAL_DECIMAL.fullmatch(value):
            raise ValueError("must be a canonical decimal string")
        try:
            return Decimal(value)
        except InvalidOperation as error:
            raise ValueError("must be a decimal") from error
    if kind == "date":
        return date.fromisoformat(value)
    if kind == "datetime":
        return parse_time(value)
    return value


def check_digest(
    artifact_path: str,
    expected: str,
    repository_root: Path,
    path: str,
    problems: Problems,
) -> None:
    artifact = repository_root / artifact_path
    if not artifact.is_file():
        problems.add(path, "referenced artifact does not exist")
        return
    actual = hashlib.sha256(artifact.read_bytes()).hexdigest()
    if actual != expected:
        problems.add(path, f"artifact SHA-256 is {actual}, expected {expected}")


def check_provenance(
    provenance: dict[str, Any],
    platform_ids: set[str],
    repository_root: Path,
    path: str,
    problems: Problems,
) -> None:
    if provenance["platform"] not in platform_ids:
        problems.add(f"{path}.platform", f"unknown platform {provenance['platform']!r}")
    snapshot = provenance.get("snapshot")
    if snapshot:
        check_digest(
            snapshot["artifact_path"],
            snapshot["digest"]["value"],
            repository_root,
            f"{path}.snapshot.digest.value",
            problems,
        )


def check_integrity(
    integrity: dict[str, Any], repository_root: Path, path: str, problems: Problems
) -> None:
    if integrity["status"] == "unanchored":
        return
    target = integrity["target"]
    check_digest(
        target["artifact_path"],
        target["digest"]["value"],
        repository_root,
        f"{path}.target.digest.value",
        problems,
    )


def check_integrity_transition(before: dict[str, Any], after: dict[str, Any]) -> list[str]:
    """Check one normative integrity state transition without performing effects."""

    errors: list[str] = []
    before_status = before.get("status")
    after_status = after.get("status")
    if after_status not in INTEGRITY_TRANSITIONS.get(before_status, set()):
        errors.append(f"integrity transition {before_status!r} -> {after_status!r} is forbidden")
    if "target" in before and after.get("target") != before["target"]:
        errors.append("target identity must not change after retention")
    before_timestamps = before.get("timestamps", [])
    after_timestamps = after.get("timestamps", [])
    if len(after_timestamps) < len(before_timestamps):
        errors.append("timestamp evidence must be append-only")
    else:
        for old, new in zip(before_timestamps, after_timestamps, strict=False):
            retained = {key: value for key, value in old.items() if key != "state"}
            if any(new.get(key) != value for key, value in retained.items()):
                errors.append("timestamp evidence must be append-only")
                break
            if old.get("state") == "verified" and new.get("state") != "verified":
                errors.append("verified timestamp evidence must not regress")
                break
            if old.get("state") == "pending" and new.get("state") not in {
                "pending",
                "verified",
            }:
                errors.append("pending timestamp evidence has an invalid transition")
                break
    return errors


def parse_duration(value: str, kind: str) -> timedelta:
    pattern = r"P([1-9][0-9]*)D" if kind == "date" else r"PT([1-9][0-9]*)S"
    match = re.fullmatch(pattern, value)
    if not match:
        expected = "P<n>D" if kind == "date" else "PT<n>S"
        raise ValueError(f"step must use {expected} with a positive integer")
    amount = int(match.group(1))
    return timedelta(days=amount) if kind == "date" else timedelta(seconds=amount)


def scalar_in_domain(value: Any, domain: dict[str, Any]) -> tuple[bool, str]:
    kind = domain["kind"]
    try:
        parsed = parse_scalar(value, kind)
    except (TypeError, ValueError) as error:
        return False, str(error)
    if kind in {"categorical", "ordinal"}:
        ids = {option["id"] for option in domain["option_set"]["options"]}
        return (parsed in ids, "must reference an option in the bound option set")
    if kind == "binary":
        return True, ""

    bounds = domain.get("bounds", {})
    for side in ("lower", "upper"):
        if side not in bounds:
            continue
        try:
            boundary = parse_scalar(bounds[side]["value"], kind)
        except (TypeError, ValueError) as error:
            return False, f"invalid {side} bound: {error}"
        inclusive = bounds[side]["inclusive"]
        if side == "lower" and (parsed < boundary or (parsed == boundary and not inclusive)):
            return False, "is below the domain lower bound"
        if side == "upper" and (parsed > boundary or (parsed == boundary and not inclusive)):
            return False, "is above the domain upper bound"

    values = domain["values"]
    if values["kind"] == "allowed_values" and value not in values["values"]:
        return False, "is not in domain.values.allowed_values"
    if values["kind"] == "step":
        origin_raw = values.get("origin")
        if kind == "numeric":
            step = parse_scalar(values["step"], kind)
            if step <= 0:
                return False, "domain step must be positive"
            origin = parse_scalar(origin_raw or "0", kind)
            if (parsed - origin) % step != 0:
                return False, "does not align with the domain step"
        else:
            try:
                step = parse_duration(values["step"], kind)
                origin = (
                    parse_scalar(origin_raw, kind)
                    if origin_raw
                    else (
                        date(1970, 1, 1)
                        if kind == "date"
                        else datetime.fromisoformat("1970-01-01T00:00:00+00:00")
                    )
                )
                elapsed = parsed - origin
                if elapsed.total_seconds() % step.total_seconds() != 0:
                    return False, "does not align with the domain step"
            except (TypeError, ValueError) as error:
                return False, str(error)
    return True, ""


def check_bins(domain: dict[str, Any], path: str, problems: Problems) -> None:
    kind = domain["kind"]
    if kind not in {"numeric", "date", "datetime"}:
        return
    bin_sets = domain.get("bin_sets", [])
    problems.unique([(item["id"], item["version"]) for item in bin_sets], f"{path}.bin_sets")
    for set_index, bin_set in enumerate(bin_sets):
        spath = f"{path}.bin_sets[{set_index}]"
        bins = bin_set["bins"]
        problems.unique([item["id"] for item in bins], f"{spath}.bins")
        previous = None
        for bin_index, item in enumerate(bins):
            bpath = f"{spath}.bins[{bin_index}]"
            try:
                lower = parse_scalar(item["lower"], kind)
                upper = parse_scalar(item["upper"], kind)
            except (TypeError, ValueError) as error:
                problems.add(bpath, str(error))
                continue
            if lower >= upper:
                problems.add(bpath, "lower must be strictly less than upper")
            if previous:
                if lower != previous["upper"]:
                    problems.add(bpath, "bins must have neither gaps nor overlaps")
                if item["lower_inclusive"] == previous["inclusive"]:
                    problems.add(
                        bpath,
                        "exactly one adjacent bin must include their shared boundary",
                    )
            previous = {"upper": upper, "inclusive": item["upper_inclusive"]}


def check_domain(domain: dict[str, Any], path: str, problems: Problems) -> None:
    kind = domain["kind"]
    if kind in {"categorical", "ordinal"}:
        problems.unique(
            [item["id"] for item in domain["option_set"]["options"]],
            f"{path}.option_set.options",
        )
        return
    if kind == "binary":
        return
    bounds = domain.get("bounds", {})
    try:
        if "lower" in bounds and "upper" in bounds:
            lower = parse_scalar(bounds["lower"]["value"], kind)
            upper = parse_scalar(bounds["upper"]["value"], kind)
            if lower > upper or (
                lower == upper
                and not (bounds["lower"]["inclusive"] and bounds["upper"]["inclusive"])
            ):
                problems.add(f"{path}.bounds", "bounds describe an empty domain")
        values = domain["values"]
        if values["kind"] == "allowed_values":
            parsed = [parse_scalar(item, kind) for item in values["values"]]
            problems.unique(parsed, f"{path}.values.values")
            if parsed != sorted(parsed):
                problems.add(f"{path}.values.values", "allowed values must be sorted")
            for index, item in enumerate(values["values"]):
                ok, reason = scalar_in_domain(item, {**domain, "values": {"kind": "continuous"}})
                if not ok:
                    problems.add(f"{path}.values.values[{index}]", reason)
        elif values["kind"] == "step":
            if kind == "numeric":
                if parse_scalar(values["step"], kind) <= 0:
                    problems.add(f"{path}.values.step", "must be positive")
                if "origin" in values:
                    parse_scalar(values["origin"], kind)
            else:
                parse_duration(values["step"], kind)
                if "origin" in values:
                    parse_scalar(values["origin"], kind)
    except (InvalidOperation, TypeError, ValueError) as error:
        problems.add(path, str(error))
    check_bins(domain, path, problems)


def option_set_ref_matches(rep: dict[str, Any], domain: dict[str, Any]) -> bool:
    target = domain["option_set"]
    return rep["option_set_ref"] == {"id": target["id"], "version": target["version"]}


def find_bin_set(rep: dict[str, Any], domain: dict[str, Any]) -> dict[str, Any] | None:
    ref = rep["bin_set_ref"]
    return next(
        (
            item
            for item in domain.get("bin_sets", [])
            if item["id"] == ref["id"] and item["version"] == ref["version"]
        ),
        None,
    )


def check_representation(
    rep: dict[str, Any], domain: dict[str, Any], path: str, problems: Problems
) -> None:
    rep_kind = rep["kind"]
    kind = domain["kind"]
    allowed = {
        "binary": {"probability", "point"},
        "categorical": {"pmf", "point"},
        "ordinal": {"pmf", "point"},
        "numeric": {"binned_pmf", "quantiles", "cdf", "point", "credible_intervals"},
        "date": {"binned_pmf", "quantiles", "cdf", "point", "credible_intervals"},
        "datetime": {"binned_pmf", "quantiles", "cdf", "point", "credible_intervals"},
    }
    if rep_kind not in allowed[kind]:
        problems.add(path, f"{rep_kind!r} is incompatible with {kind!r} outcome space")
        return
    if rep_kind == "pmf":
        if not option_set_ref_matches(rep, domain):
            problems.add(
                f"{path}.option_set_ref", "must bind the exact question option-set version"
            )
        expected = [item["id"] for item in domain["option_set"]["options"]]
        actual = [item["option_id"] for item in rep["entries"]]
        problems.unique(actual, f"{path}.entries")
        if set(actual) != set(expected):
            problems.add(f"{path}.entries", "must cover every option exactly once")
        total = sum(Decimal(item["probability"]) for item in rep["entries"])
        if total != ONE:
            problems.add(f"{path}.entries", f"probabilities sum to {total}, not 1")
    elif rep_kind == "binned_pmf":
        bin_set = find_bin_set(rep, domain)
        if bin_set is None:
            problems.add(f"{path}.bin_set_ref", "must bind an existing exact bin-set version")
            return
        expected = [item["id"] for item in bin_set["bins"]]
        actual = [item["bin_id"] for item in rep["entries"]]
        problems.unique(actual, f"{path}.entries")
        if set(actual) != set(expected):
            problems.add(f"{path}.entries", "must cover every bin exactly once")
        total = (
            sum(Decimal(item["probability"]) for item in rep["entries"])
            + Decimal(rep["left_tail_probability"])
            + Decimal(rep["right_tail_probability"])
        )
        if total != ONE:
            problems.add(path, f"bin and tail probabilities sum to {total}, not 1")
        bounds = domain.get("bounds", {})
        first, last = bin_set["bins"][0], bin_set["bins"][-1]
        if (
            "lower" in bounds
            and first["lower"] == bounds["lower"]["value"]
            and Decimal(rep["left_tail_probability"]) != 0
        ):
            problems.add(
                f"{path}.left_tail_probability", "must be 0 when bins start at the domain bound"
            )
        if (
            "upper" in bounds
            and last["upper"] == bounds["upper"]["value"]
            and Decimal(rep["right_tail_probability"]) != 0
        ):
            problems.add(
                f"{path}.right_tail_probability", "must be 0 when bins end at the domain bound"
            )
    elif rep_kind == "quantiles":
        levels = [Decimal(item["level"]) for item in rep["points"]]
        if levels != sorted(levels) or len(levels) != len(set(levels)):
            problems.add(f"{path}.points", "quantile levels must be strictly increasing")
        parsed = []
        for index, item in enumerate(rep["points"]):
            ok, reason = scalar_in_domain(item["value"], domain)
            if not ok:
                problems.add(f"{path}.points[{index}].value", reason)
            else:
                parsed.append(parse_scalar(item["value"], kind))
        if len(parsed) == len(rep["points"]) and parsed != sorted(parsed):
            problems.add(f"{path}.points", "quantile values must be non-decreasing")
    elif rep_kind == "cdf":
        parsed = []
        probabilities = []
        for index, item in enumerate(rep["points"]):
            ok, reason = scalar_in_domain(item["value"], domain)
            if not ok:
                problems.add(f"{path}.points[{index}].value", reason)
            else:
                parsed.append(parse_scalar(item["value"], kind))
            probabilities.append(Decimal(item["probability"]))
        if len(parsed) == len(rep["points"]) and (
            parsed != sorted(parsed) or len(parsed) != len(set(parsed))
        ):
            problems.add(f"{path}.points", "CDF values must be strictly increasing")
        if probabilities != sorted(probabilities):
            problems.add(f"{path}.points", "CDF probabilities must be non-decreasing")
        left = Decimal(rep["left_tail_probability"])
        right = Decimal(rep["right_tail_probability"])
        if left > probabilities[0]:
            problems.add(
                f"{path}.left_tail_probability", "must not exceed the first CDF probability"
            )
        if probabilities[-1] + right != ONE:
            problems.add(path, "last CDF probability plus right tail probability must equal 1")
    elif rep_kind == "point":
        statistic = rep["statistic"]
        if statistic == "mean" and kind not in {"numeric", "date", "datetime"}:
            problems.add(
                f"{path}.statistic", "mean is only defined for numeric, date, or datetime outcomes"
            )
        if statistic == "mode" and kind == "binary" and not isinstance(rep["value"], bool):
            problems.add(f"{path}.value", "binary mode must be boolean")
        ok, reason = scalar_in_domain(rep["value"], domain)
        if not ok:
            problems.add(f"{path}.value", reason)
    elif rep_kind == "credible_intervals":
        coverages = [item["coverage"] for item in rep["intervals"]]
        problems.unique(coverages, f"{path}.intervals")
        for index, interval in enumerate(rep["intervals"]):
            ipath = f"{path}.intervals[{index}]"
            lower_ok, lower_reason = scalar_in_domain(interval["lower"], domain)
            upper_ok, upper_reason = scalar_in_domain(interval["upper"], domain)
            if not lower_ok:
                problems.add(f"{ipath}.lower", lower_reason)
            if not upper_ok:
                problems.add(f"{ipath}.upper", upper_reason)
            if (
                lower_ok
                and upper_ok
                and parse_scalar(interval["lower"], kind) > parse_scalar(interval["upper"], kind)
            ):
                problems.add(ipath, "lower must not exceed upper")


def check_revealed(
    question: dict[str, Any], forecast: dict[str, Any], path: str, problems: Problems
) -> None:
    if forecast["visibility"] != "revealed":
        return
    try:
        payload = reveal_forecast(
            question_id=question["id"],
            question_revision_id=forecast["question_revision_id"],
            forecast_id=forecast["id"],
            commitment=forecast["commitment"],
            key=bytes.fromhex(forecast["commitment"]["revealed_key"]),
        )
    except Exception:  # cryptographic errors are intentionally redacted from diagnostics
        problems.add(f"{path}.commitment", "reveal verification failed")
        return
    bundle = payload["bundle"]
    if forecast.get("representations") != bundle["representations"]:
        problems.add(f"{path}.representations", "does not match the decrypted sealed bundle")
    for field in ("rationale", "key_factors", "comment"):
        if (field in forecast) != (field in bundle):
            problems.add(
                f"{path}.{field}",
                "presence does not match the decrypted sealed bundle",
            )
        elif field in forecast and forecast[field] != bundle[field]:
            problems.add(f"{path}.{field}", "does not match the decrypted sealed bundle")


def check_lifecycle(
    events: list[dict[str, Any]],
    forecasted_at: datetime,
    forecast_recorded_at: datetime,
    path: str,
    problems: Problems,
) -> None:
    problems.unique([event["id"] for event in events], path)
    active = True
    previous_effective: datetime | None = None
    previous_recorded: datetime | None = None
    for index, event in enumerate(events):
        epath = f"{path}[{index}]"
        effective = parse_time(event["effective_at"])
        recorded = parse_time(event["recorded_at"])
        if effective < forecasted_at:
            problems.add(
                f"{epath}.effective_at",
                "must not precede forecast.forecasted_at",
            )
        if recorded < effective:
            problems.add(f"{epath}.recorded_at", "must not precede effective_at")
        if recorded < forecast_recorded_at:
            problems.add(
                f"{epath}.recorded_at",
                "must not precede forecast.recorded_at",
            )
        if previous_effective and effective < previous_effective:
            problems.add(f"{epath}.effective_at", "events must be ordered by effective_at")
        if previous_recorded and recorded < previous_recorded:
            problems.add(
                f"{epath}.recorded_at",
                "events must be append-only ordered by recorded_at",
            )
        if event["type"] in {"withdrawn", "expired"}:
            if not active:
                problems.add(f"{epath}.type", "cannot deactivate an inactive forecast")
            active = False
        else:
            if active:
                problems.add(f"{epath}.type", "cannot reaffirm an active forecast")
            active = True
        previous_effective, previous_recorded = effective, recorded


def check_activity_checkpoints(
    question_id: str,
    forecast: dict[str, Any],
    revision: dict[str, Any],
    repository_root: Path,
    path: str,
    problems: Problems,
) -> None:
    checkpoints = forecast.get("activity_checkpoints", [])
    problems.unique([checkpoint["id"] for checkpoint in checkpoints], path)
    events = forecast.get("lifecycle_events", [])
    event_indexes = {event["id"]: index for index, event in enumerate(events)}
    problems.unique([checkpoint["head_event_id"] for checkpoint in checkpoints], path)
    previous_head_index: int | None = None
    previous_recorded: datetime | None = None
    for index, checkpoint in enumerate(checkpoints):
        cpath = f"{path}[{index}]"
        head_id = checkpoint["head_event_id"]
        head_index = event_indexes.get(head_id)
        if head_index is None:
            problems.add(f"{cpath}.head_event_id", "must reference a lifecycle event")
            continue
        if previous_head_index is not None and head_index <= previous_head_index:
            problems.add(
                f"{cpath}.head_event_id",
                "checkpoints must cover strictly increasing lifecycle prefixes",
            )
        checkpoint_recorded = parse_time(checkpoint["recorded_at"])
        head_recorded = parse_time(events[head_index]["recorded_at"])
        if checkpoint_recorded < head_recorded:
            problems.add(
                f"{cpath}.recorded_at",
                "must not precede the covered lifecycle head recorded_at",
            )
        if previous_recorded and checkpoint_recorded < previous_recorded:
            problems.add(
                f"{cpath}.recorded_at",
                "checkpoints must be append-only ordered by recorded_at",
            )
        integrity = checkpoint["integrity"]
        check_integrity(integrity, repository_root, f"{cpath}.integrity", problems)
        if "target" in integrity:
            expected = canonicalize(
                forecast_lifecycle_target(question_id, forecast, revision, head_id)
            )
            expected_digest = hashlib.sha256(expected).hexdigest()
            if integrity["target"]["digest"]["value"] != expected_digest:
                problems.add(
                    f"{cpath}.integrity.target.digest.value",
                    "does not match the canonical forecast-lifecycle/v2 target",
                )
        previous_head_index = head_index
        previous_recorded = checkpoint_recorded


def check_relationships(
    ledger: dict[str, Any], questions: dict[str, dict[str, Any]], problems: Problems
) -> None:
    groups = {group["id"]: group for group in ledger.get("groups", [])}
    problems.unique([group["id"] for group in ledger.get("groups", [])], "$.groups")
    relationships = ledger.get("relationships", [])
    problems.unique([rel["id"] for rel in relationships], "$.relationships")
    memberships: list[tuple[str, str]] = []
    graph: dict[str, set[str]] = {question_id: set() for question_id in questions}
    for index, rel in enumerate(relationships):
        path = f"$.relationships[{index}]"
        if rel["kind"] == "group_membership":
            if rel["group_id"] not in groups:
                problems.add(f"{path}.group_id", "unknown group")
            if rel["question_id"] not in questions:
                problems.add(f"{path}.question_id", "unknown question")
            memberships.append((rel["group_id"], rel["question_id"]))
            continue
        parent = questions.get(rel["parent_question_id"])
        child = questions.get(rel["child_question_id"])
        if parent is None:
            problems.add(f"{path}.parent_question_id", "unknown question")
        if child is None:
            problems.add(f"{path}.child_question_id", "unknown question")
        if parent and child:
            revisions = {rev["id"]: rev for rev in parent["revisions"]}
            revision = revisions.get(rel["parent_question_revision_id"])
            if revision is None:
                problems.add(f"{path}.parent_question_revision_id", "unknown parent revision")
            else:
                ok, reason = scalar_in_domain(rel["parent_outcome"], revision["domain"])
                if not ok:
                    problems.add(f"{path}.parent_outcome", reason)
            graph[parent["id"]].add(child["id"])
    problems.unique(memberships, "$.relationships[group_membership]")

    visiting: set[str] = set()
    visited: set[str] = set()

    def visit(node: str) -> bool:
        if node in visiting:
            return True
        if node in visited:
            return False
        visiting.add(node)
        if any(visit(child) for child in graph[node]):
            return True
        visiting.remove(node)
        visited.add(node)
        return False

    if any(visit(node) for node in graph if node not in visited):
        problems.add("$.relationships", "conditional relationships must be acyclic")


def check_semantics(ledger: dict[str, Any], repository_root: Path, problems: Problems) -> None:
    try:
        ZoneInfo(ledger["default_timezone"])
    except ZoneInfoNotFoundError:
        problems.add("$.default_timezone", "is not a known IANA time zone")

    if ledger["forecaster"]["kind"] == "team":
        problems.unique(
            [member["id"] for member in ledger["forecaster"]["members"]],
            "$.forecaster.members",
        )
    platform_ids = set(ledger["platforms"])
    questions = {question["id"]: question for question in ledger["questions"]}
    problems.unique([question["id"] for question in ledger["questions"]], "$.questions")
    global_forecast_ids: list[str] = []

    for question_index, question in enumerate(ledger["questions"]):
        qpath = f"$.questions[{question_index}]"
        revisions = question["revisions"]
        problems.unique([revision["id"] for revision in revisions], f"{qpath}.revisions")
        revision_map = {revision["id"]: revision for revision in revisions}
        if question["current_revision_id"] not in revision_map:
            problems.add(f"{qpath}.current_revision_id", "must reference a question revision")
        elif question["current_revision_id"] != revisions[-1]["id"]:
            problems.add(f"{qpath}.current_revision_id", "must reference the last revision")
        previous_effective: datetime | None = None
        previous_recorded: datetime | None = None
        for revision_index, revision in enumerate(revisions):
            rpath = f"{qpath}.revisions[{revision_index}]"
            effective = parse_time(revision["effective_at"])
            recorded = parse_time(revision["recorded_at"])
            if recorded < effective:
                problems.add(f"{rpath}.recorded_at", "must not precede effective_at")
            if previous_effective and effective <= previous_effective:
                problems.add(rpath, "revisions must be strictly ordered by effective_at")
            if previous_recorded and recorded < previous_recorded:
                problems.add(rpath, "revisions must be append-only ordered by recorded_at")
            if revision["outcome_space"]["kind"] != revision["domain"]["kind"]:
                problems.add(f"{rpath}.domain.kind", "must match outcome_space.kind")
            check_domain(revision["domain"], f"{rpath}.domain", problems)
            if "provenance" in revision:
                check_provenance(
                    revision["provenance"],
                    platform_ids,
                    repository_root,
                    f"{rpath}.provenance",
                    problems,
                )
            previous_effective, previous_recorded = effective, recorded

        local_forecast_ids: list[str] = []
        previous_forecast_recorded: datetime | None = None
        for forecast_index, forecast in enumerate(question["forecasts"]):
            fpath = f"{qpath}.forecasts[{forecast_index}]"
            global_forecast_ids.append(forecast["id"])
            revision = revision_map.get(forecast["question_revision_id"])
            if revision is None:
                problems.add(
                    f"{fpath}.question_revision_id", "must reference a revision of this question"
                )
                continue
            forecasted = parse_time(forecast["forecasted_at"])
            recorded = parse_time(forecast["recorded_at"])
            if recorded < forecasted:
                problems.add(f"{fpath}.recorded_at", "must not precede forecasted_at")
            if forecasted < parse_time(revision["effective_at"]):
                problems.add(
                    f"{fpath}.forecasted_at", "must not precede the bound revision effective_at"
                )
            if "forecasting_opens_at" in revision and forecasted < parse_time(
                revision["forecasting_opens_at"]
            ):
                problems.add(f"{fpath}.forecasted_at", "must not precede forecasting_opens_at")
            if previous_forecast_recorded and recorded < previous_forecast_recorded:
                problems.add(fpath, "forecasts must be ordered by recorded_at")
            previous_forecast_recorded = recorded
            supersedes = forecast.get("supersedes_forecast_id")
            if supersedes and supersedes not in local_forecast_ids:
                problems.add(
                    f"{fpath}.supersedes_forecast_id",
                    "must reference an earlier forecast for this question",
                )
            local_forecast_ids.append(forecast["id"])

            representations = forecast.get("representations", [])
            problems.unique([rep["kind"] for rep in representations], f"{fpath}.representations")
            for rep_index, rep in enumerate(representations):
                check_representation(
                    rep, revision["domain"], f"{fpath}.representations[{rep_index}]", problems
                )
            if "provenance" in forecast:
                check_provenance(
                    forecast["provenance"],
                    platform_ids,
                    repository_root,
                    f"{fpath}.provenance",
                    problems,
                )
            events = forecast.get("lifecycle_events", [])
            check_lifecycle(
                events,
                forecasted,
                recorded,
                f"{fpath}.lifecycle_events",
                problems,
            )
            for event_index, event in enumerate(events):
                if "provenance" in event:
                    check_provenance(
                        event["provenance"],
                        platform_ids,
                        repository_root,
                        f"{fpath}.lifecycle_events[{event_index}].provenance",
                        problems,
                    )
            check_integrity(forecast["integrity"], repository_root, f"{fpath}.integrity", problems)
            check_activity_checkpoints(
                question["id"],
                forecast,
                revision,
                repository_root,
                f"{fpath}.activity_checkpoints",
                problems,
            )
            check_revealed(question, forecast, fpath, problems)

        resolution = question.get("resolution")
        if resolution:
            if resolution["status"] != question["status"]:
                problems.add(f"{qpath}.resolution.status", "must match question.status")
            if resolution["status"] == "resolved":
                revision = revision_map.get(resolution["question_revision_id"])
                if revision is None:
                    problems.add(
                        f"{qpath}.resolution.question_revision_id", "unknown question revision"
                    )
                else:
                    ok, reason = scalar_in_domain(resolution["outcome"], revision["domain"])
                    if not ok:
                        problems.add(f"{qpath}.resolution.outcome", reason)
                known = parse_time(resolution["outcome_known_at"])
                if parse_time(resolution["recorded_at"]) < known:
                    problems.add(
                        f"{qpath}.resolution.recorded_at", "must not precede outcome_known_at"
                    )
                for forecast_index, forecast in enumerate(question["forecasts"]):
                    integrity = forecast["integrity"]
                    if integrity["status"] != "verified":
                        continue
                    verified_times = [
                        parse_time(proof["gen_time"])
                        for proof in integrity["timestamps"]
                        if proof["state"] == "verified"
                    ]
                    if not any(gen_time < known for gen_time in verified_times):
                        problems.add(
                            f"{qpath}.forecasts[{forecast_index}].integrity.timestamps",
                            "must contain a verified RFC 3161 timestamp "
                            "predating the known outcome",
                        )

    problems.unique(global_forecast_ids, "$.questions[*].forecasts")
    check_relationships(ledger, questions, problems)
    relationship_map = {rel["id"]: rel for rel in ledger.get("relationships", [])}
    for question_index, question in enumerate(ledger["questions"]):
        resolution = question.get("resolution")
        if not resolution or resolution["status"] != "not_applicable":
            continue
        path = f"$.questions[{question_index}].resolution.relationship_id"
        relationship = relationship_map.get(resolution["relationship_id"])
        if not relationship or relationship.get("kind") != "conditional":
            problems.add(path, "must reference a conditional relationship")
        elif relationship["child_question_id"] != question["id"]:
            problems.add(path, "must reference a conditional relationship for this child question")
        else:
            parent = questions[relationship["parent_question_id"]]
            parent_resolution = parent.get("resolution")
            if (
                parent_resolution
                and parent_resolution["status"] == "resolved"
                and parent_resolution["outcome"] == relationship["parent_outcome"]
            ):
                problems.add(
                    path,
                    "cannot be not_applicable when the parent outcome satisfies the condition",
                )


def validate(path: Path, schema_path: Path, repository_root: Path) -> list[str]:
    ledger, locations = load_document_with_locations(path)
    schema = json.loads(schema_path.read_text(encoding="utf-8"))
    problems = Problems(locations)
    check_schema(ledger, schema, problems)
    if not problems.items:
        check_semantics(ledger, repository_root, problems)
    return problems.items


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("documents", nargs="+", type=Path)
    parser.add_argument("--schema", type=Path, default=Path("schema/forecast-ledger.schema.json"))
    parser.add_argument(
        "--repository-root",
        type=Path,
        default=Path.cwd(),
        help="Base directory for proof and provenance artifact paths",
    )
    args = parser.parse_args()
    failed = False
    for document in args.documents:
        try:
            display_document = document.resolve().relative_to(Path.cwd().resolve()).as_posix()
        except ValueError:
            display_document = document.name
        try:
            errors = validate(document, args.schema, args.repository_root)
        except DocumentSyntaxError as error:
            errors = [
                f"/ [line {error.line}, column {error.column}]: document syntax is invalid"
            ]
        if errors:
            failed = True
            print(f"FAIL {display_document}")
            for error in errors:
                print(f"  - {error}")
        else:
            print(f"OK   {display_document}")
    return 1 if failed else 0


if __name__ == "__main__":
    sys.exit(main())
