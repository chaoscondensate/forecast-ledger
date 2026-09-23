#!/usr/bin/env python3
"""Verify evidence-index and publication-v3 closure and canonical bytes."""

from __future__ import annotations

import copy
import json
from pathlib import Path

from sidecar_contracts import (
    decode_evidence_index,
    decode_publication,
    encode_evidence_index,
    encode_publication,
    validate_evidence_index,
    validate_publication,
)

ROOT = Path(__file__).resolve().parents[1]


def rejected(operation) -> None:
    try:
        operation()
    except ValueError:
        return
    raise AssertionError("invalid sidecar was accepted")


def main() -> int:
    index_vector = json.loads(
        (ROOT / "tests/vectors/forecast-evidence-index-v1.json").read_text(encoding="utf-8")
    )
    publication_vector = json.loads(
        (ROOT / "tests/vectors/forecast-ledger-publication-v3.json").read_text(
            encoding="utf-8"
        )
    )
    empty_vector = json.loads(
        (ROOT / "tests/vectors/forecast-evidence-index-v1-empty.json").read_text(
            encoding="utf-8"
        )
    )
    index = index_vector["index"]
    manifest = publication_vector["manifest"]
    encoded_index = encode_evidence_index(index)
    encoded_manifest = encode_publication(manifest, index)
    if decode_evidence_index(encoded_index) != index:
        raise AssertionError("evidence index round trip changed the value")
    if decode_publication(encoded_manifest, index) != manifest:
        raise AssertionError("publication manifest round trip changed the value")

    rejected(lambda: validate_evidence_index(empty_vector["index"], allow_empty=False))
    validate_evidence_index(empty_vector["index"], allow_empty=True)
    rejected(lambda: decode_evidence_index(encoded_index + b"\n"))

    old_index = copy.deepcopy(index)
    old_index["schema"] = "forecast-evidence-index/v0"
    rejected(lambda: validate_evidence_index(old_index, allow_empty=False))
    self_indexed = copy.deepcopy(index)
    self_indexed["entries"][0]["path"] = "proofs/evidence-index.json"
    rejected(lambda: validate_evidence_index(self_indexed, allow_empty=False))
    unsorted = copy.deepcopy(index)
    unsorted["entries"].reverse()
    rejected(lambda: validate_evidence_index(unsorted, allow_empty=False))
    missing_reference = copy.deepcopy(index)
    response = next(
        entry
        for entry in missing_reference["entries"]
        if entry["role"] == "rfc3161_response"
    )
    response["references"]["trust_path"] = "trust/missing.pem"
    rejected(lambda: validate_evidence_index(missing_reference, allow_empty=False))

    old_manifest = copy.deepcopy(manifest)
    old_manifest["profile"] = "forecast-ledger-publication/v2"
    rejected(lambda: validate_publication(old_manifest, index))
    detached = copy.deepcopy(manifest)
    detached["entries"] = [
        entry for entry in detached["entries"] if entry["role"] != "lifecycle_target"
    ]
    rejected(lambda: validate_publication(detached, index))
    unindexed = copy.deepcopy(manifest)
    unindexed["entries"].append(
        {
            "role": "forecast_target",
            "path": "proofs/targets/unindexed.json",
            "size": 1,
            "digest": {"algorithm": "sha-256", "value": "f" * 64},
        }
    )
    unindexed["entries"].sort(key=lambda entry: entry["path"])
    rejected(lambda: validate_publication(unindexed, index))
    rejected(lambda: decode_publication(encoded_manifest + b"\n", index))

    print("OK   exact RFC 8785 sidecar bytes")
    print("OK   local empty-index prohibition and package empty-index allowance")
    print("OK   evidence references, ordering, self-exclusion, and shared trust")
    print("OK   publication v3 closure and v2 rejection")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
