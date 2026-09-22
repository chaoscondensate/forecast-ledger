#!/usr/bin/env python3
"""Reference cryptographic operations for Forecast Ledger seal profiles v1 and v2.

The implementation supports the RFC 8785 subset used by this project: strings,
booleans, null, arrays, objects, and I-JSON safe integers. Floating-point JSON
numbers are rejected; exact values and probabilities are decimal strings in v2.
"""

from __future__ import annotations

import argparse
import base64
import copy
import hashlib
import json
from pathlib import Path
from typing import Any

from cryptography.hazmat.primitives.ciphers.aead import ChaCha20Poly1305

MAX_SAFE_INTEGER = 9_007_199_254_740_991
SEAL_SCHEME_V1 = "forecast-seal/v1"
SEAL_SCHEME_V2 = "forecast-seal/v2"
ENVELOPE_SCHEMA_V1 = "forecast-envelope/v1"
ENVELOPE_SCHEMA_V2 = "forecast-envelope/v2"
LIFECYCLE_SCHEMA_V1 = "forecast-lifecycle/v1"

PRIVATE_BUNDLE_V2_FIELDS = {
    "representations",
    "rationale",
    "key_factors",
    "comment",
}

# Backward-compatible aliases for applications importing the original names.
SEAL_SCHEME = SEAL_SCHEME_V1
ENVELOPE_SCHEMA = ENVELOPE_SCHEMA_V1


class CanonicalizationError(ValueError):
    """Raised when a value is outside the project's RFC 8785 subset."""


def _reject_surrogates(value: str) -> None:
    if any(0xD800 <= ord(character) <= 0xDFFF for character in value):
        raise CanonicalizationError("lone Unicode surrogates are not valid I-JSON")


def _string(value: str) -> str:
    _reject_surrogates(value)
    return json.dumps(value, ensure_ascii=False, separators=(",", ":"))


def _utf16_sort_key(value: str) -> bytes:
    _reject_surrogates(value)
    return value.encode("utf-16-be")


def _serialize(value: Any) -> str:
    if value is None:
        return "null"
    if value is True:
        return "true"
    if value is False:
        return "false"
    if isinstance(value, int):
        if abs(value) > MAX_SAFE_INTEGER:
            raise CanonicalizationError("integer is outside the I-JSON safe range")
        return str(value)
    if isinstance(value, float):
        raise CanonicalizationError(
            "floating-point values are forbidden; use canonical decimal strings"
        )
    if isinstance(value, str):
        return _string(value)
    if isinstance(value, list):
        return "[" + ",".join(_serialize(item) for item in value) + "]"
    if isinstance(value, dict):
        if not all(isinstance(key, str) for key in value):
            raise CanonicalizationError("JSON object keys must be strings")
        keys = sorted(value, key=_utf16_sort_key)
        return "{" + ",".join(f"{_string(key)}:{_serialize(value[key])}" for key in keys) + "}"
    raise CanonicalizationError(f"unsupported JSON value: {type(value).__name__}")


def canonicalize(value: Any) -> bytes:
    """Return canonical UTF-8 bytes for the project's RFC 8785 subset."""

    return _serialize(value).encode("utf-8")


def sha256_ref(data: bytes) -> dict[str, str]:
    return {"algorithm": "sha-256", "value": hashlib.sha256(data).hexdigest()}


def _b64(data: bytes) -> str:
    return base64.b64encode(data).decode("ascii")


def _unb64(value: str) -> bytes:
    return base64.b64decode(value, validate=True)


def _aad_v1(question_id: str, forecast_id: str, commitment_hex: str) -> bytes:
    return canonicalize(
        {
            "scheme": SEAL_SCHEME_V1,
            "question_id": question_id,
            "forecast_id": forecast_id,
            "commitment_sha256": commitment_hex,
        }
    )


def _aad_v2(
    question_id: str,
    question_revision_id: str,
    forecast_id: str,
    commitment_hex: str,
) -> bytes:
    return canonicalize(
        {
            "scheme": SEAL_SCHEME_V2,
            "question_id": question_id,
            "question_revision_id": question_revision_id,
            "forecast_id": forecast_id,
            "commitment_sha256": commitment_hex,
        }
    )


def seal_forecast(
    *,
    question_id: str,
    forecast_id: str,
    bundle: dict[str, Any],
    salt: bytes,
    key: bytes,
    nonce: bytes,
    key_hint: str,
    scheme: str = SEAL_SCHEME_V1,
    question_revision_id: str | None = None,
) -> tuple[dict[str, Any], bytes]:
    """Seal a private forecast and return its public commitment plus plaintext."""

    if len(salt) != 32:
        raise ValueError("salt must be exactly 32 bytes")
    if len(key) != 32:
        raise ValueError("key must be exactly 32 bytes")
    if len(nonce) != 12:
        raise ValueError("nonce must be exactly 12 bytes")
    if scheme not in {SEAL_SCHEME_V1, SEAL_SCHEME_V2}:
        raise ValueError(f"unsupported seal scheme: {scheme}")

    if scheme == SEAL_SCHEME_V2:
        if question_revision_id is None:
            raise ValueError("question_revision_id is required for forecast-seal/v2")
        _validate_private_bundle_v2(bundle)
    else:
        required_bundle_fields = {
            "forecasted_at",
            "recorded_at",
            "value",
            "rationale",
            "key_factors",
            "comment",
        }
        missing = sorted(required_bundle_fields - bundle.keys())
        if missing:
            raise ValueError(f"sealed bundle is missing: {', '.join(missing)}")

    payload = {
        "schema": scheme,
        "question_id": question_id,
        "forecast_id": forecast_id,
        "bundle": bundle,
        "salt": salt.hex(),
    }
    if scheme == SEAL_SCHEME_V2:
        payload["question_revision_id"] = question_revision_id
    plaintext = canonicalize(payload)
    commitment = sha256_ref(plaintext)
    if scheme == SEAL_SCHEME_V1:
        aad = _aad_v1(question_id, forecast_id, commitment["value"])
    else:
        aad = _aad_v2(
            question_id,
            str(question_revision_id),
            forecast_id,
            commitment["value"],
        )
    ciphertext = ChaCha20Poly1305(key).encrypt(nonce, plaintext, aad)

    public_commitment = {
        "scheme": scheme,
        "commitment_hash": commitment,
        "encryption": {
            "algorithm": "chacha20-poly1305",
            "nonce": _b64(nonce),
            "ciphertext": _b64(ciphertext),
        },
        "key_hint": key_hint,
    }
    return public_commitment, plaintext


def reveal_forecast(
    *,
    question_id: str,
    forecast_id: str,
    commitment: dict[str, Any],
    key: bytes,
    question_revision_id: str | None = None,
) -> dict[str, Any]:
    """Decrypt, authenticate, and verify a v1 or v2 sealed forecast payload."""

    if len(key) != 32:
        raise ValueError("key must be exactly 32 bytes")
    scheme = commitment["scheme"]
    commitment_hex = commitment["commitment_hash"]["value"]
    encryption = commitment["encryption"]
    nonce = _unb64(encryption["nonce"])
    ciphertext = _unb64(encryption["ciphertext"])
    if scheme == SEAL_SCHEME_V1:
        aad = _aad_v1(question_id, forecast_id, commitment_hex)
    elif scheme == SEAL_SCHEME_V2:
        if question_revision_id is None:
            raise ValueError("question_revision_id is required for forecast-seal/v2")
        aad = _aad_v2(question_id, question_revision_id, forecast_id, commitment_hex)
    else:
        raise ValueError(f"unsupported seal scheme: {scheme}")
    plaintext = ChaCha20Poly1305(key).decrypt(nonce, ciphertext, aad)

    actual = hashlib.sha256(plaintext).hexdigest()
    if actual != commitment_hex:
        raise ValueError("decrypted payload does not match the commitment hash")
    payload = json.loads(plaintext)
    expected_payload_fields = {"schema", "question_id", "forecast_id", "bundle", "salt"}
    if scheme == SEAL_SCHEME_V2:
        expected_payload_fields.add("question_revision_id")
    if set(payload) != expected_payload_fields:
        raise ValueError("sealed payload contains unknown or missing properties")
    if payload.get("schema") != scheme:
        raise ValueError("unexpected sealed payload scheme")
    if payload.get("question_id") != question_id:
        raise ValueError("sealed payload belongs to another question")
    if payload.get("forecast_id") != forecast_id:
        raise ValueError("sealed payload belongs to another forecast")
    if scheme == SEAL_SCHEME_V2:
        if payload.get("question_revision_id") != question_revision_id:
            raise ValueError("sealed payload belongs to another question revision")
        _validate_private_bundle_v2(payload.get("bundle"))
    if canonicalize(payload) != plaintext:
        raise ValueError("decrypted payload is not canonical RFC 8785 data")
    return payload


def reveal_into_forecast(
    forecast: dict[str, Any],
    payload: dict[str, Any],
    revealed_commitment: dict[str, Any],
) -> dict[str, Any]:
    """Return a revealed forecast containing exactly the authenticated private fields."""

    bundle = payload["bundle"]
    _validate_private_bundle_v2(bundle)
    if payload.get("schema") != SEAL_SCHEME_V2:
        raise ValueError("unexpected sealed payload scheme")
    if payload.get("forecast_id") != forecast.get("id"):
        raise ValueError("sealed payload belongs to another forecast")
    if payload.get("question_revision_id") != forecast.get("question_revision_id"):
        raise ValueError("sealed payload belongs to another question revision")
    original_commitment = forecast.get("commitment", {})
    retained_fields = {"scheme", "commitment_hash", "encryption", "key_hint"}
    if any(
        revealed_commitment.get(field) != original_commitment.get(field)
        for field in retained_fields
    ):
        raise ValueError("revealed commitment does not retain the original seal")
    result = copy.deepcopy(forecast)
    for field in PRIVATE_BUNDLE_V2_FIELDS:
        result.pop(field, None)
    for field in PRIVATE_BUNDLE_V2_FIELDS:
        if field in bundle:
            result[field] = copy.deepcopy(bundle[field])
    result["visibility"] = "revealed"
    result["commitment"] = copy.deepcopy(revealed_commitment)
    return result


def _validate_private_bundle_v2(bundle: Any) -> None:
    """Validate the closed forecast-seal/v2 private bundle without exposing values."""

    if not isinstance(bundle, dict):
        raise ValueError("sealed bundle must be an object")
    unknown = sorted(set(bundle) - PRIVATE_BUNDLE_V2_FIELDS)
    if unknown:
        raise ValueError(f"sealed bundle contains unknown properties: {', '.join(unknown)}")
    if "representations" not in bundle:
        raise ValueError("/representations: required property is missing")
    if not isinstance(bundle["representations"], list) or not bundle["representations"]:
        raise ValueError("/representations: must be a non-empty array")
    for field in ("rationale", "comment"):
        if field in bundle and not isinstance(bundle[field], str):
            raise ValueError(f"/{field}: must be a string")
    if "key_factors" in bundle:
        factors = bundle["key_factors"]
        if not isinstance(factors, list) or not all(
            isinstance(item, str) and item for item in factors
        ):
            raise ValueError("/key_factors: must be an array of non-empty strings")


def _included_forecast(forecast: dict[str, Any], *, sealed: bool) -> dict[str, Any]:
    if sealed:
        commitment = forecast["commitment"]
        included = {
            "id": forecast["id"],
            "question_revision_id": forecast.get("question_revision_id"),
            "forecasted_at": forecast["forecasted_at"],
            "recorded_at": forecast["recorded_at"],
            "visibility": "sealed",
            "commitment": {
                "scheme": commitment["scheme"],
                "commitment_hash": commitment["commitment_hash"],
                "encryption": commitment["encryption"],
            },
        }
        if included["question_revision_id"] is None:
            del included["question_revision_id"]
        optional = (
            "public_note",
            "supersedes_forecast_id",
            "provenance",
        )
    else:
        included = {
            key: forecast[key]
            for key in (
                "id",
                "question_revision_id",
                "forecasted_at",
                "recorded_at",
                "visibility",
                "representations",
                "value",
                "rationale",
                "key_factors",
                "comment",
                "public_note",
                "supersedes_forecast_id",
                "provenance",
            )
            if key in forecast
        }
        optional = ()
    for key in optional:
        if key in forecast:
            included[key] = forecast[key]
    return included


def public_forecast_envelope(
    question_id: str,
    forecast: dict[str, Any],
    question_revision: dict[str, Any] | None = None,
) -> dict[str, Any]:
    """Build the immutable timestamp target for a public forecast."""

    if question_revision is None:
        return {
            "schema": ENVELOPE_SCHEMA_V1,
            "question_id": question_id,
            "forecast": _included_forecast(forecast, sealed=False),
        }
    if forecast.get("question_revision_id") != question_revision.get("id"):
        raise ValueError("forecast does not reference the supplied question revision")
    return {
        "schema": ENVELOPE_SCHEMA_V2,
        "question": {"id": question_id, "revision": question_revision},
        "forecast": _included_forecast(forecast, sealed=False),
    }


def sealed_forecast_envelope(
    question_id: str,
    forecast: dict[str, Any],
    question_revision: dict[str, Any] | None = None,
) -> dict[str, Any]:
    """Build the immutable target that binds every security-relevant seal input."""

    if question_revision is None:
        return {
            "schema": ENVELOPE_SCHEMA_V1,
            "question_id": question_id,
            "forecast": _included_forecast(forecast, sealed=True),
        }
    if forecast.get("question_revision_id") != question_revision.get("id"):
        raise ValueError("forecast does not reference the supplied question revision")
    return {
        "schema": ENVELOPE_SCHEMA_V2,
        "question": {"id": question_id, "revision": question_revision},
        "forecast": _included_forecast(forecast, sealed=True),
    }


def forecast_envelope(
    question_id: str,
    forecast: dict[str, Any],
    question_revision: dict[str, Any],
) -> dict[str, Any]:
    """Build the envelope used by the ledger for the forecast's current visibility."""

    if forecast["visibility"] == "public":
        return public_forecast_envelope(question_id, forecast, question_revision)
    return sealed_forecast_envelope(question_id, forecast, question_revision)


def forecast_lifecycle_target(
    question_id: str,
    forecast: dict[str, Any],
    question_revision: dict[str, Any],
    head_event_id: str,
) -> dict[str, Any]:
    """Build a closed target binding one complete ordered lifecycle prefix."""

    events = forecast.get("lifecycle_events", [])
    head_index = next(
        (index for index, event in enumerate(events) if event["id"] == head_event_id),
        None,
    )
    if head_index is None:
        raise ValueError("covered lifecycle head does not exist")
    envelope_bytes = canonicalize(forecast_envelope(question_id, forecast, question_revision))
    return {
        "schema": LIFECYCLE_SCHEMA_V1,
        "question_id": question_id,
        "forecast_id": forecast["id"],
        "forecast_envelope_sha256": sha256_ref(envelope_bytes),
        "lifecycle_events": events[: head_index + 1],
        "head_event_id": head_event_id,
    }


def verify_vector(vector: dict[str, Any]) -> None:
    material = vector["material"]
    vector_schema = vector.get("schema", "forecast-seal-test-vector/v1")
    scheme = SEAL_SCHEME_V2 if vector_schema.endswith("/v2") else SEAL_SCHEME_V1
    revision_id = vector.get("question_revision_id")
    commitment, plaintext = seal_forecast(
        question_id=vector["question_id"],
        question_revision_id=revision_id,
        forecast_id=vector["forecast_id"],
        bundle=vector["bundle"],
        salt=bytes.fromhex(material["salt_hex"]),
        key=bytes.fromhex(material["key_hex"]),
        nonce=bytes.fromhex(material["nonce_hex"]),
        key_hint=vector["expected"]["commitment"]["key_hint"],
        scheme=scheme,
    )
    if plaintext.decode("utf-8") != vector["expected"]["canonical_plaintext"]:
        raise AssertionError("canonical plaintext differs from the test vector")
    if commitment != vector["expected"]["commitment"]:
        raise AssertionError("commitment differs from the test vector")
    payload = reveal_forecast(
        question_id=vector["question_id"],
        question_revision_id=revision_id,
        forecast_id=vector["forecast_id"],
        commitment=commitment,
        key=bytes.fromhex(material["key_hex"]),
    )
    if payload["bundle"] != vector["bundle"]:
        raise AssertionError("revealed bundle differs from the input")


def envelope_from_target_vector(vector: dict[str, Any]) -> dict[str, Any]:
    """Build the public or sealed envelope described by a target vector."""

    projection = vector["projection"]
    if projection == "public":
        return public_forecast_envelope(
            vector["question_id"], vector["forecast"], vector["question_revision"]
        )
    if projection == "sealed":
        return sealed_forecast_envelope(
            vector["question_id"], vector["forecast"], vector["question_revision"]
        )
    raise ValueError(f"unsupported target projection: {projection}")


def verify_target_vector(vector: dict[str, Any]) -> bytes:
    """Verify exact canonical bytes and SHA-256 for an envelope target vector."""

    data = canonicalize(envelope_from_target_vector(vector))
    if data.decode("utf-8") != vector["expected"]["canonical_envelope"]:
        raise AssertionError("canonical envelope differs from the target vector")
    actual = hashlib.sha256(data).hexdigest()
    if actual != vector["expected"]["sha256"]:
        raise AssertionError(
            f"target SHA-256 is {actual}, expected {vector['expected']['sha256']}"
        )
    return data


def verify_lifecycle_vector(vector: dict[str, Any]) -> None:
    """Verify exact lifecycle targets and deterministic tamper candidates."""

    expected_by_name: dict[str, bytes] = {}
    for checkpoint in vector["checkpoints"]:
        target = forecast_lifecycle_target(
            vector["question_id"],
            vector["forecast"],
            vector["question_revision"],
            checkpoint["head_event_id"],
        )
        if set(target) != {
            "schema",
            "question_id",
            "forecast_id",
            "forecast_envelope_sha256",
            "lifecycle_events",
            "head_event_id",
        }:
            raise AssertionError(f"{checkpoint['name']}: lifecycle target is not closed")
        data = canonicalize(target)
        if data.decode("utf-8") != checkpoint["expected"]["canonical_target"]:
            raise AssertionError(f"{checkpoint['name']}: canonical lifecycle target differs")
        if hashlib.sha256(data).hexdigest() != checkpoint["expected"]["sha256"]:
            raise AssertionError(f"{checkpoint['name']}: lifecycle target SHA-256 differs")
        head_index = next(
            index
            for index, event in enumerate(vector["forecast"]["lifecycle_events"])
            if event["id"] == checkpoint["head_event_id"]
        )
        prefix_forecast = copy.deepcopy(vector["forecast"])
        prefix_forecast["lifecycle_events"] = prefix_forecast["lifecycle_events"][
            : head_index + 1
        ]
        prefix_forecast["activity_checkpoints"] = [
            {
                "id": "excluded-metadata",
                "head_event_id": checkpoint["head_event_id"],
                "recorded_at": "2026-09-30T00:00:00Z",
                "integrity": {"status": "failed"},
            }
        ]
        prefix_data = canonicalize(
            forecast_lifecycle_target(
                vector["question_id"],
                prefix_forecast,
                vector["question_revision"],
                checkpoint["head_event_id"],
            )
        )
        if prefix_data != data:
            raise AssertionError(
                f"{checkpoint['name']}: later events or checkpoint metadata changed the prefix"
            )
        expected_by_name[checkpoint["name"]] = data

    for case in vector["tamper_cases"]:
        target = forecast_lifecycle_target(
            case.get("question_id", vector["question_id"]),
            case["forecast"],
            vector["question_revision"],
            case["head_event_id"],
        )
        data = canonicalize(target)
        if data.decode("utf-8") != case["expected_candidate"]["canonical_target"]:
            raise AssertionError(f"{case['name']}: canonical tamper candidate differs")
        if hashlib.sha256(data).hexdigest() != case["expected_candidate"]["sha256"]:
            raise AssertionError(f"{case['name']}: tamper candidate SHA-256 differs")
        if data == expected_by_name[case["covered_checkpoint"]]:
            raise AssertionError(f"{case['name']}: tampering was not detected")


def verify_presence_vector(vector: dict[str, Any]) -> None:
    """Verify exact seal vectors for every optional-field presence combination."""

    material = vector["material"]
    for case in vector["cases"]:
        commitment, plaintext = seal_forecast(
            question_id=vector["question_id"],
            question_revision_id=vector["question_revision_id"],
            forecast_id=vector["forecast_id"],
            bundle=case["bundle"],
            salt=bytes.fromhex(material["salt_hex"]),
            key=bytes.fromhex(material["key_hex"]),
            nonce=bytes.fromhex(material["nonce_hex"]),
            key_hint=vector["key_hint"],
            scheme=SEAL_SCHEME_V2,
        )
        if plaintext.decode("utf-8") != case["expected"]["canonical_plaintext"]:
            raise AssertionError(f"{case['name']}: canonical plaintext differs")
        if commitment["commitment_hash"]["value"] != case["expected"]["commitment_sha256"]:
            raise AssertionError(f"{case['name']}: commitment hash differs")
        if commitment["encryption"]["ciphertext"] != case["expected"]["ciphertext_base64"]:
            raise AssertionError(f"{case['name']}: ciphertext differs")
        payload = reveal_forecast(
            question_id=vector["question_id"],
            question_revision_id=vector["question_revision_id"],
            forecast_id=vector["forecast_id"],
            commitment=commitment,
            key=bytes.fromhex(material["key_hex"]),
        )
        if payload["bundle"] != case["bundle"]:
            raise AssertionError(f"{case['name']}: revealed bundle differs")

    for case in vector["invalid_cases"]:
        try:
            seal_forecast(
                question_id=vector["question_id"],
                question_revision_id=vector["question_revision_id"],
                forecast_id=vector["forecast_id"],
                bundle=case["bundle"],
                salt=bytes.fromhex(material["salt_hex"]),
                key=bytes.fromhex(material["key_hex"]),
                nonce=bytes.fromhex(material["nonce_hex"]),
                key_hint=vector["key_hint"],
                scheme=SEAL_SCHEME_V2,
            )
        except ValueError as error:
            if str(error) != case["expected_error"]:
                raise AssertionError(f"{case['name']}: unexpected error") from None
        else:
            raise AssertionError(f"{case['name']}: invalid bundle was accepted")


def _demo_vector(version: int) -> dict[str, Any]:
    if version == 1:
        bundle = {
            "forecasted_at": "2026-08-25T10:00:00+01:00",
            "recorded_at": "2026-08-25T10:01:00+01:00",
            "value": {"kind": "binary", "probability_bp": 6500},
            "rationale": (
                "The base rate and two independent indicators point in the same direction."
            ),
            "key_factors": ["base rate", "leading indicator"],
            "comment": "Reveal after the outcome is public.",
        }
        revision_id = None
        scheme = SEAL_SCHEME_V1
    else:
        bundle = {
            "representations": [{"kind": "probability", "outcome": True, "probability": "0.65"}],
            "rationale": (
                "The base rate and two independent indicators point in the same direction."
            ),
            "key_factors": ["base rate", "leading indicator"],
            "comment": "Reveal after the outcome is public.",
        }
        revision_id = "qr-example-binary-1"
        scheme = SEAL_SCHEME_V2
    salt = bytes.fromhex("aa" * 10 + "1f" + "aa" * 21)
    key = bytes.fromhex("bb" * 32)
    nonce = bytes.fromhex("cc" * 12)
    commitment, plaintext = seal_forecast(
        question_id="q-example-binary",
        question_revision_id=revision_id,
        forecast_id="f-example-binary-001",
        bundle=bundle,
        salt=salt,
        key=key,
        nonce=nonce,
        key_hint="secret-manager://forecast-ledger/f-example-binary-001",
        scheme=scheme,
    )
    vector = {
        "schema": f"forecast-seal-test-vector/v{version}",
        "question_id": "q-example-binary",
        "forecast_id": "f-example-binary-001",
        "bundle": bundle,
        "material": {
            "salt_hex": salt.hex(),
            "key_hex": key.hex(),
            "nonce_hex": nonce.hex(),
        },
        "expected": {
            "canonical_plaintext": plaintext.decode("utf-8"),
            "commitment": commitment,
        },
    }
    if revision_id:
        vector["question_revision_id"] = revision_id
    return vector


def _presence_demo_vector() -> dict[str, Any]:
    representation = [{"kind": "probability", "outcome": True, "probability": "0.65"}]
    optional_values = {
        "rationale": "Evidence supports the event.",
        "key_factors": ["base rate"],
        "comment": "Private note.",
    }
    presence_sets = (
        ("representation-only", ()),
        ("rationale-only", ("rationale",)),
        ("key-factors-only", ("key_factors",)),
        ("comment-only", ("comment",)),
        ("rationale-and-key-factors", ("rationale", "key_factors")),
        ("rationale-and-comment", ("rationale", "comment")),
        ("key-factors-and-comment", ("key_factors", "comment")),
        ("all-optional-fields", ("rationale", "key_factors", "comment")),
    )
    salt = bytes.fromhex("11" * 32)
    key = bytes.fromhex("22" * 32)
    nonce = bytes.fromhex("33" * 12)
    context = {
        "question_id": "q-seal-presence",
        "question_revision_id": "qr-seal-presence-1",
        "forecast_id": "f-seal-presence-1",
        "key_hint": "secret-manager://forecast-ledger/f-seal-presence-1",
    }
    cases = []
    for name, fields in presence_sets:
        bundle = {"representations": representation}
        bundle.update({field: optional_values[field] for field in fields})
        commitment, plaintext = seal_forecast(
            **context,
            bundle=bundle,
            salt=salt,
            key=key,
            nonce=nonce,
            scheme=SEAL_SCHEME_V2,
        )
        cases.append(
            {
                "name": name,
                "bundle": bundle,
                "expected": {
                    "canonical_plaintext": plaintext.decode("utf-8"),
                    "commitment_sha256": commitment["commitment_hash"]["value"],
                    "ciphertext_base64": commitment["encryption"]["ciphertext"],
                },
            }
        )
    empty_bundle = {
        "representations": representation,
        "rationale": "",
        "key_factors": [],
        "comment": "",
    }
    commitment, plaintext = seal_forecast(
        **context,
        bundle=empty_bundle,
        salt=salt,
        key=key,
        nonce=nonce,
        scheme=SEAL_SCHEME_V2,
    )
    cases.append(
        {
            "name": "explicit-empty-optionals",
            "bundle": empty_bundle,
            "expected": {
                "canonical_plaintext": plaintext.decode("utf-8"),
                "commitment_sha256": commitment["commitment_hash"]["value"],
                "ciphertext_base64": commitment["encryption"]["ciphertext"],
            },
        }
    )
    return {
        "schema": "forecast-seal-presence-test-vector/v1",
        **context,
        "material": {
            "salt_hex": salt.hex(),
            "key_hex": key.hex(),
            "nonce_hex": nonce.hex(),
        },
        "cases": cases,
        "invalid_cases": [
            {
                "name": "missing-representations",
                "bundle": {"rationale": "Present but insufficient."},
                "expected_error": "/representations: required property is missing",
            },
            {
                "name": "unknown-property",
                "bundle": {"representations": representation, "private_note": "forbidden"},
                "expected_error": "sealed bundle contains unknown properties: private_note",
            },
        ],
        "reveal_cases": [
            {
                "name": "invented-empty-rationale",
                "sealed_case": "representation-only",
                "revealed_private_fields": {
                    "representations": representation,
                    "rationale": "",
                },
                "expected_error": (
                    "/forecast/rationale: presence does not match the decrypted sealed bundle"
                ),
            },
            {
                "name": "omitted-authenticated-comment",
                "sealed_case": "comment-only",
                "revealed_private_fields": {"representations": representation},
                "expected_error": (
                    "/forecast/comment: presence does not match the decrypted sealed bundle"
                ),
            },
        ],
    }


def _lifecycle_demo_vector() -> dict[str, Any]:
    question_id = "q-lifecycle-vector"
    revision = {
        "id": "qr-lifecycle-vector-1",
        "effective_at": "2026-09-01T09:00:00Z",
        "recorded_at": "2026-09-01T09:00:05Z",
        "title": "Will the example event occur?",
        "resolution_criteria": "Resolve YES if the example event occurs.",
        "expected_resolution_at": "2026-12-31T23:59:59Z",
        "outcome_space": {"kind": "binary"},
        "domain": {"kind": "binary"},
    }
    forecast = {
        "id": "f-lifecycle-vector-1",
        "question_revision_id": revision["id"],
        "forecasted_at": "2026-09-02T10:00:00Z",
        "recorded_at": "2026-09-02T10:00:05Z",
        "visibility": "public",
        "representations": [
            {"kind": "probability", "outcome": True, "probability": "0.65"}
        ],
        "lifecycle_events": [
            {
                "id": "event-withdrawn",
                "type": "withdrawn",
                "effective_at": "2026-09-03T10:00:00Z",
                "recorded_at": "2026-09-03T10:00:05Z",
                "reason": "Evidence review started.",
            },
            {
                "id": "event-reaffirmed",
                "type": "reaffirmed",
                "effective_at": "2026-09-04T10:00:00Z",
                "recorded_at": "2026-09-04T10:00:05Z",
                "reason": "The review confirmed the forecast.",
            },
        ],
        "integrity": {"status": "unanchored", "note": "Vector fixture."},
    }

    def expected(candidate: dict[str, Any], head_event_id: str) -> dict[str, str]:
        data = canonicalize(
            forecast_lifecycle_target(question_id, candidate, revision, head_event_id)
        )
        return {
            "canonical_target": data.decode("utf-8"),
            "sha256": hashlib.sha256(data).hexdigest(),
        }

    checkpoints = [
        {
            "name": "withdrawal",
            "head_event_id": "event-withdrawn",
            "expected": expected(forecast, "event-withdrawn"),
        },
        {
            "name": "reaffirmation",
            "head_event_id": "event-reaffirmed",
            "expected": expected(forecast, "event-reaffirmed"),
        },
    ]
    tamper_specs = []
    altered = copy.deepcopy(forecast)
    altered["lifecycle_events"][0]["reason"] = "Altered reason."
    tamper_specs.append(("prefix-alteration", altered, "event-withdrawn", "withdrawal"))
    deleted = copy.deepcopy(forecast)
    del deleted["lifecycle_events"][0]
    tamper_specs.append(("event-deletion", deleted, "event-reaffirmed", "reaffirmation"))
    reordered = copy.deepcopy(forecast)
    reordered["lifecycle_events"].reverse()
    tamper_specs.append(("reordered-events", reordered, "event-reaffirmed", "reaffirmation"))
    rebound = copy.deepcopy(forecast)
    rebound["id"] = "f-wrong-binding"
    tamper_specs.append(("wrong-forecast-binding", rebound, "event-withdrawn", "withdrawal"))
    tamper_cases = [
        {
            "name": name,
            "forecast": candidate,
            "head_event_id": head,
            "covered_checkpoint": checkpoint,
            "expected_candidate": expected(candidate, head),
        }
        for name, candidate, head, checkpoint in tamper_specs
    ]
    return {
        "schema": "forecast-lifecycle-test-vector/v1",
        "question_id": question_id,
        "question_revision": revision,
        "forecast": forecast,
        "checkpoints": checkpoints,
        "tamper_cases": tamper_cases,
    }


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    subparsers = parser.add_subparsers(dest="command", required=True)
    show_parser = subparsers.add_parser("show-vector", help="Print a deterministic vector")
    show_parser.add_argument("--version", type=int, choices=(1, 2), default=2)
    subparsers.add_parser(
        "show-presence-vector", help="Print deterministic forecast-seal/v2 presence vectors"
    )
    subparsers.add_parser(
        "show-lifecycle-vector", help="Print deterministic forecast-lifecycle/v1 vectors"
    )
    verify_parser = subparsers.add_parser("verify-vector", help="Verify a vector file")
    verify_parser.add_argument("path", type=Path)
    target_parser = subparsers.add_parser(
        "verify-target-vector", help="Verify an envelope target vector file"
    )
    target_parser.add_argument("path", type=Path)
    lifecycle_parser = subparsers.add_parser(
        "verify-lifecycle-vector", help="Verify a lifecycle target vector corpus"
    )
    lifecycle_parser.add_argument("path", type=Path)
    presence_parser = subparsers.add_parser(
        "verify-presence-vector", help="Verify forecast-seal/v2 optional-field vectors"
    )
    presence_parser.add_argument("path", type=Path)
    args = parser.parse_args()

    if args.command == "show-vector":
        print(json.dumps(_demo_vector(args.version), ensure_ascii=False, indent=2))
        return 0
    if args.command == "show-presence-vector":
        print(json.dumps(_presence_demo_vector(), ensure_ascii=False, indent=2))
        return 0
    if args.command == "show-lifecycle-vector":
        print(json.dumps(_lifecycle_demo_vector(), ensure_ascii=False, indent=2))
        return 0
    vector = json.loads(args.path.read_text(encoding="utf-8"))
    if args.command == "verify-target-vector":
        verify_target_vector(vector)
    elif args.command == "verify-lifecycle-vector":
        verify_lifecycle_vector(vector)
    elif args.command == "verify-presence-vector":
        verify_presence_vector(vector)
    else:
        verify_vector(vector)
    print(f"ok: {args.path}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
