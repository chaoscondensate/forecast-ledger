#!/usr/bin/env python3
"""Verify normative retained-integrity transition semantics."""

from __future__ import annotations

import copy

from validate import check_integrity_transition


def main() -> int:
    target = {
        "scope": "forecast-envelope/v3",
        "canonicalization": "RFC8785",
        "artifact_path": "proofs/targets/f-example.json",
        "digest": {"algorithm": "sha-256", "value": "a" * 64},
    }
    timestamp = {
        "type": "rfc3161",
        "request_path": "proofs/timestamps/f-example.tsq",
        "response_path": "proofs/timestamps/f-example.tsr",
        "tsa_url": "https://tsa.example.test/",
        "hash_algorithm": "sha256",
        "state": "pending",
    }
    unanchored = {"status": "unanchored"}
    retained = {"status": "retained", "target": target}
    pending = {"status": "pending", "target": target, "timestamps": [timestamp]}
    verified = {
        "status": "verified",
        "target": target,
        "timestamps": [{**timestamp, "state": "verified"}],
        "verified_at": "2026-09-22T12:00:00Z",
    }
    failed = {
        "status": "failed",
        "failure_reason": "Retained timestamp evidence did not verify.",
        "target": target,
        "timestamps": [timestamp],
    }
    allowed = [
        (unanchored, retained),
        (retained, pending),
        (retained, verified),
        (pending, failed),
        (failed, pending),
        (failed, verified),
        (verified, {**verified, "timestamps": [*verified["timestamps"], timestamp]}),
    ]
    for before, after in allowed:
        errors = check_integrity_transition(before, after)
        if errors:
            raise AssertionError(errors)

    changed_target = copy.deepcopy(pending)
    changed_target["target"]["digest"]["value"] = "b" * 64
    invalid = [
        (unanchored, pending),
        (retained, unanchored),
        (pending, retained),
        (pending, changed_target),
        (verified, retained),
    ]
    for before, after in invalid:
        if not check_integrity_transition(before, after):
            raise AssertionError("invalid integrity transition was accepted")

    print("OK   retained integrity transition graph")
    print("OK   target identity and timestamp append-only invariants")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
