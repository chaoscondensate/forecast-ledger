#!/usr/bin/env python3
"""Reference encoding and semantic checks for Forecast Ledger v2.2 sidecars."""

from __future__ import annotations

import argparse
import hashlib
import json
from pathlib import Path
from typing import Any

from forecast_crypto import canonicalize
from jsonschema import Draft202012Validator, FormatChecker

ROOT = Path(__file__).resolve().parents[1]
INDEX_SCHEMA = ROOT / "schema/forecast-evidence-index.schema.json"
PUBLICATION_SCHEMA = ROOT / "schema/forecast-ledger-publication.schema.json"
INDEX_PATH = "proofs/evidence-index.json"


def _schema_errors(value: Any, schema_path: Path) -> list[str]:
    schema = json.loads(schema_path.read_text(encoding="utf-8"))
    validator = Draft202012Validator(schema, format_checker=FormatChecker())
    return [error.message for error in validator.iter_errors(value)]


def _strict_paths(entries: list[dict[str, Any]]) -> None:
    paths = [entry["path"] for entry in entries]
    if paths != sorted(paths) or len(paths) != len(set(paths)):
        raise ValueError("entries must have unique paths in strict ascending order")


def validate_evidence_index(value: dict[str, Any], *, allow_empty: bool) -> None:
    errors = _schema_errors(value, INDEX_SCHEMA)
    if errors:
        raise ValueError("evidence index schema validation failed")
    entries = value["entries"]
    if not allow_empty and not entries:
        raise ValueError("a local evidence index must not be empty")
    _strict_paths(entries)
    by_path = {entry["path"]: entry for entry in entries}
    if INDEX_PATH in by_path:
        raise ValueError("the evidence index must not index itself")
    for entry in entries:
        references = entry.get("references", {})
        target_path = references.get("target_path")
        if target_path:
            target = by_path.get(target_path)
            if not target or target["role"] not in {"forecast_target", "lifecycle_target"}:
                raise ValueError("an RFC 3161 entry references a missing target")
        request_path = references.get("request_path")
        if request_path:
            request = by_path.get(request_path)
            if not request or request["role"] != "rfc3161_request":
                raise ValueError("an RFC 3161 response references a missing request")
        trust_path = references.get("trust_path")
        if trust_path:
            trust = by_path.get(trust_path)
            if not trust or trust["role"] != "x509_ca_bundle":
                raise ValueError("an RFC 3161 response references a missing trust bundle")


def encode_evidence_index(value: dict[str, Any], *, allow_empty: bool = False) -> bytes:
    validate_evidence_index(value, allow_empty=allow_empty)
    return canonicalize(value)


def decode_evidence_index(data: bytes, *, allow_empty: bool = False) -> dict[str, Any]:
    if not data or len(data) > 8 << 20:
        raise ValueError("evidence index is empty or too large")
    try:
        value = json.loads(data)
    except (UnicodeDecodeError, json.JSONDecodeError):
        raise ValueError("evidence index is not valid JSON") from None
    encoded = encode_evidence_index(value, allow_empty=allow_empty)
    if encoded != data:
        raise ValueError("evidence index is not exact RFC 8785 JSON")
    return value


def validate_publication(value: dict[str, Any], evidence_index: dict[str, Any]) -> None:
    errors = _schema_errors(value, PUBLICATION_SCHEMA)
    if errors:
        raise ValueError("publication manifest schema validation failed")
    validate_evidence_index(evidence_index, allow_empty=True)
    _strict_paths(value["entries"])
    entries = {entry["path"]: entry for entry in value["entries"]}
    ledger = entries.get(value["ledger_path"])
    if not ledger or ledger["role"] != "ledger":
        raise ValueError("ledger_path does not identify the ledger entry")
    index_entry = entries.get(value["evidence_index_path"])
    if not index_entry or index_entry["role"] != "evidence_index":
        raise ValueError("evidence_index_path does not identify the index entry")
    manifest_evidence = {
        path: entry["role"]
        for path, entry in entries.items()
        if entry["role"] not in {"ledger", "evidence_index"}
    }
    indexed_evidence = {entry["path"]: entry["role"] for entry in evidence_index["entries"]}
    if manifest_evidence != indexed_evidence:
        raise ValueError("publication evidence does not exactly match the evidence index")
    if value["contract"] != evidence_index["contract"]:
        raise ValueError("publication and evidence index contract identities differ")


def encode_publication(value: dict[str, Any], evidence_index: dict[str, Any]) -> bytes:
    validate_publication(value, evidence_index)
    return canonicalize(value)


def decode_publication(data: bytes, evidence_index: dict[str, Any]) -> dict[str, Any]:
    if not data or len(data) > 8 << 20:
        raise ValueError("publication manifest is empty or too large")
    try:
        value = json.loads(data)
    except (UnicodeDecodeError, json.JSONDecodeError):
        raise ValueError("publication manifest is not valid JSON") from None
    encoded = encode_publication(value, evidence_index)
    if encoded != data:
        raise ValueError("publication manifest is not exact RFC 8785 JSON")
    return value


def verify_vector(vector: dict[str, Any]) -> None:
    profile = vector.get("schema")
    if profile == "forecast-evidence-index-test-vector/v1":
        data = encode_evidence_index(vector["index"], allow_empty=vector.get("allow_empty", False))
    elif profile == "forecast-ledger-publication-test-vector/v3":
        data = encode_publication(vector["manifest"], vector["evidence_index"])
    else:
        raise AssertionError("unsupported sidecar vector profile")
    expected = vector["expected"]
    if data.decode("utf-8") != expected["canonical_json"]:
        raise AssertionError("canonical sidecar bytes differ from the vector")
    if hashlib.sha256(data).hexdigest() != expected["sha256"]:
        raise AssertionError("sidecar SHA-256 differs from the vector")


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("vectors", nargs="+", type=Path)
    args = parser.parse_args()
    for path in args.vectors:
        verify_vector(json.loads(path.read_text(encoding="utf-8")))
        print(f"ok: {path}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
