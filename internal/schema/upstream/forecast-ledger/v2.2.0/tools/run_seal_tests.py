#!/usr/bin/env python3
"""Verify forecast-seal/v3 optional-field presence and reveal semantics."""

from __future__ import annotations

import base64
import copy
import hashlib
import json
import sys
from pathlib import Path

from cryptography.hazmat.primitives.ciphers.aead import ChaCha20Poly1305
from forecast_crypto import (
    PRIVATE_BUNDLE_V3_FIELDS,
    SEAL_SCHEME_V3,
    RevealError,
    canonicalize,
    decode_key_file,
    encode_key_file,
    reveal_forecast,
    reveal_into_forecast,
    seal_forecast,
    verify_presence_vector,
)
from validate import Problems, check_revealed

ROOT = Path(__file__).resolve().parents[1]
VECTOR = ROOT / "tests/vectors/forecast-seal-v3-presence.json"


def expect_code(code: str, operation: object) -> None:
    try:
        operation()  # type: ignore[operator]
    except RevealError as error:
        if error.code != code:
            raise AssertionError(f"expected {code}, received {error.code}") from None
    else:
        raise AssertionError(f"expected {code}")


def main() -> int:
    vector = json.loads(VECTOR.read_text(encoding="utf-8"))
    verify_presence_vector(vector)
    material = vector["material"]
    key = bytes.fromhex(material["key_hex"])
    cases = {case["name"]: case for case in vector["cases"]}

    for name, case in cases.items():
        commitment, _ = seal_forecast(
            question_id=vector["question_id"],
            question_revision_id=vector["question_revision_id"],
            forecast_id=vector["forecast_id"],
            bundle=case["bundle"],
            salt=bytes.fromhex(material["salt_hex"]),
            key=key,
            nonce=bytes.fromhex(material["nonce_hex"]),
            key_hint=vector["key_hint"],
            scheme=SEAL_SCHEME_V3,
        )
        payload = reveal_forecast(
            question_id=vector["question_id"],
            question_revision_id=vector["question_revision_id"],
            forecast_id=vector["forecast_id"],
            commitment=commitment,
            key=key,
        )
        revealed_commitment = {
            **commitment,
            "revealed_at": "2026-09-03T10:00:00Z",
            "revealed_key": material["key_hex"],
        }
        sealed = {
            "id": vector["forecast_id"],
            "question_revision_id": vector["question_revision_id"],
            "forecasted_at": "2026-09-02T10:00:00Z",
            "recorded_at": "2026-09-02T10:00:05Z",
            "visibility": "sealed",
            "commitment": commitment,
            "integrity": {"status": "unanchored"},
        }
        revealed = reveal_into_forecast(sealed, payload, revealed_commitment)
        actual_private = set(revealed) & PRIVATE_BUNDLE_V3_FIELDS
        if actual_private != set(case["bundle"]):
            raise AssertionError(f"{name}: reveal invented or omitted a private field")

    absent_hash = cases["representation-only"]["expected"]["commitment_sha256"]
    empty_hash = cases["explicit-empty-optionals"]["expected"]["commitment_sha256"]
    if absent_hash == empty_hash:
        raise AssertionError("absent and explicitly empty private fields have the same commitment")

    for reveal_case in vector["reveal_cases"]:
        source = cases[reveal_case["sealed_case"]]
        commitment, _ = seal_forecast(
            question_id=vector["question_id"],
            question_revision_id=vector["question_revision_id"],
            forecast_id=vector["forecast_id"],
            bundle=source["bundle"],
            salt=bytes.fromhex(material["salt_hex"]),
            key=key,
            nonce=bytes.fromhex(material["nonce_hex"]),
            key_hint=vector["key_hint"],
            scheme=SEAL_SCHEME_V3,
        )
        altered = {
            "id": vector["forecast_id"],
            "question_revision_id": vector["question_revision_id"],
            "forecasted_at": "2026-09-02T10:00:00Z",
            "recorded_at": "2026-09-02T10:00:05Z",
            "visibility": "revealed",
            **copy.deepcopy(reveal_case["revealed_private_fields"]),
            "commitment": {
                **commitment,
                "revealed_at": "2026-09-03T10:00:00Z",
                "revealed_key": material["key_hex"],
            },
            "integrity": {"status": "unanchored"},
        }
        problems = Problems()
        check_revealed({"id": vector["question_id"]}, altered, "$.forecast", problems)
        if reveal_case["expected_error"] not in problems.items:
            raise AssertionError(f"{reveal_case['name']}: altered presence was not rejected")

    source = cases["representation-only"]
    commitment, _ = seal_forecast(
        question_id=vector["question_id"],
        question_revision_id=vector["question_revision_id"],
        forecast_id=vector["forecast_id"],
        bundle=source["bundle"],
        salt=bytes.fromhex(material["salt_hex"]),
        key=key,
        nonce=bytes.fromhex(material["nonce_hex"]),
        key_hint=vector["key_hint"],
    )

    def open_candidate(
        candidate: dict, *, candidate_key: bytes = key, question_id: str | None = None
    ):
        return reveal_forecast(
            question_id=question_id or vector["question_id"],
            question_revision_id=vector["question_revision_id"],
            forecast_id=vector["forecast_id"],
            commitment=candidate,
            key=candidate_key,
        )

    expect_code("reveal.commitment_malformed", lambda: open_candidate({"scheme": SEAL_SCHEME_V3}))
    unsupported = copy.deepcopy(commitment)
    unsupported["encryption"]["algorithm"] = "aes-256-gcm"
    expect_code("reveal.algorithm_unsupported", lambda: open_candidate(unsupported))
    invalid_nonce = copy.deepcopy(commitment)
    invalid_nonce["encryption"]["nonce"] = "***"
    expect_code("reveal.nonce_encoding_invalid", lambda: open_candidate(invalid_nonce))
    invalid_ciphertext = copy.deepcopy(commitment)
    invalid_ciphertext["encryption"]["ciphertext"] = "***"
    expect_code(
        "reveal.ciphertext_encoding_invalid", lambda: open_candidate(invalid_ciphertext)
    )
    expect_code(
        "reveal.authentication_failed", lambda: open_candidate(commitment, candidate_key=b"x" * 32)
    )
    expect_code(
        "reveal.authentication_failed",
        lambda: open_candidate(commitment, question_id="q-wrong-binding"),
    )

    nonce = bytes.fromhex(material["nonce_hex"])
    published_digest = commitment["commitment_hash"]["value"]
    aad = canonicalize(
        {
            "scheme": SEAL_SCHEME_V3,
            "question_id": vector["question_id"],
            "question_revision_id": vector["question_revision_id"],
            "forecast_id": vector["forecast_id"],
            "commitment_sha256": published_digest,
        }
    )
    digest_mismatch = copy.deepcopy(commitment)
    digest_mismatch["encryption"]["ciphertext"] = base64.b64encode(
        ChaCha20Poly1305(key).encrypt(nonce, b"{}", aad)
    ).decode("ascii")
    expect_code("reveal.commitment_digest_mismatch", lambda: open_candidate(digest_mismatch))

    invalid_plaintext = canonicalize(
        {
            "schema": SEAL_SCHEME_V3,
            "question_id": vector["question_id"],
            "question_revision_id": vector["question_revision_id"],
            "forecast_id": vector["forecast_id"],
            "bundle": {**source["bundle"], "unknown": "forbidden"},
            "salt": material["salt_hex"],
        }
    )
    profile_mismatch = copy.deepcopy(commitment)
    profile_digest = hashlib.sha256(invalid_plaintext).hexdigest()
    profile_mismatch["commitment_hash"]["value"] = profile_digest
    profile_aad = canonicalize(
        {
            "scheme": SEAL_SCHEME_V3,
            "question_id": vector["question_id"],
            "question_revision_id": vector["question_revision_id"],
            "forecast_id": vector["forecast_id"],
            "commitment_sha256": profile_digest,
        }
    )
    profile_mismatch["encryption"]["ciphertext"] = base64.b64encode(
        ChaCha20Poly1305(key).encrypt(nonce, invalid_plaintext, profile_aad)
    ).decode("ascii")
    expect_code("reveal.bundle_profile_mismatch", lambda: open_candidate(profile_mismatch))

    key_file = encode_key_file(
        question_id=vector["question_id"],
        question_revision_id=vector["question_revision_id"],
        forecast_id=vector["forecast_id"],
        commitment_sha256=published_digest,
        key=key,
    )
    decoded_key = decode_key_file(
        key_file,
        question_id=vector["question_id"],
        question_revision_id=vector["question_revision_id"],
        forecast_id=vector["forecast_id"],
        commitment_sha256=published_digest,
    )
    if decoded_key != key:
        raise AssertionError("forecast-key/v3 round trip changed the key")
    old_key_value = json.loads(key_file)
    old_key_value["schema"] = "forecast-key/v2"
    old_key_file = canonicalize(old_key_value) + b"\n"
    expect_code(
        "reveal.key_file_binding_failed",
        lambda: decode_key_file(
            old_key_file,
            question_id=vector["question_id"],
            question_revision_id=vector["question_revision_id"],
            forecast_id=vector["forecast_id"],
            commitment_sha256=published_digest,
        ),
    )
    expect_code(
        "reveal.key_file_binding_failed",
        lambda: decode_key_file(
            key_file,
            question_id=vector["question_id"],
            question_revision_id=vector["question_revision_id"],
            forecast_id=vector["forecast_id"],
            commitment_sha256="0" * 64,
        ),
    )

    print("OK   all forecast-seal/v3 optional-field presence combinations")
    print("OK   absence differs from explicitly empty values")
    print("OK   reveal adds only authenticated private fields")
    print("OK   altered optional-field presence is rejected")
    print("OK   reveal stage diagnostics are stable and secret-safe")
    print("OK   forecast-key/v3 is closed and rejects forecast-key/v2")
    return 0


if __name__ == "__main__":
    sys.exit(main())
