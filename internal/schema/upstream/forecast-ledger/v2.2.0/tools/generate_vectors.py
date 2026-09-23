#!/usr/bin/env python3
"""Regenerate the deterministic Forecast Ledger v2.2.0 vector corpus."""

from __future__ import annotations

import hashlib
import json
from pathlib import Path
from typing import Any

from forecast_crypto import (
    _demo_vector,
    _key_demo_vector,
    _lifecycle_demo_vector,
    _presence_demo_vector,
    canonicalize,
    envelope_from_target_vector,
    forecast_envelope,
    forecast_lifecycle_target,
)
from ruamel.yaml import YAML
from sidecar_contracts import encode_evidence_index, encode_publication

ROOT = Path(__file__).resolve().parents[1]
VECTORS = ROOT / "tests/vectors"


def write_json(path: Path, value: Any) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(value, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")


def digest(value: str) -> dict[str, str]:
    return {"algorithm": "sha-256", "value": value * 64}


def contract_identity() -> dict[str, str]:
    schema_path = ROOT / "schema/forecast-ledger.schema.json"
    return {
        "schema_id": (
            "https://raw.githubusercontent.com/chaoscondensate/schema/"
            "v2.2.0/schema/forecast-ledger.schema.json"
        ),
        "schema_version": "2.2.0",
        "schema_sha256": hashlib.sha256(schema_path.read_bytes()).hexdigest(),
    }


def update_target_vector(path: Path) -> None:
    vector = json.loads(path.read_text(encoding="utf-8"))
    vector["schema"] = "forecast-envelope-test-vector/v3"
    data = canonicalize(envelope_from_target_vector(vector))
    vector["expected"] = {
        "canonical_envelope": data.decode("utf-8"),
        "sha256": hashlib.sha256(data).hexdigest(),
    }
    write_json(path, vector)


def evidence_index() -> dict[str, Any]:
    return {
        "schema": "forecast-evidence-index/v1",
        "ledger_id": "vector-ledger",
        "contract": contract_identity(),
        "entries": [
            {
                "role": "forecast_target",
                "path": "proofs/targets/f-vector.json",
                "size": 512,
                "digest": digest("1"),
                "binding": {
                    "question_id": "q-vector",
                    "forecast_id": "f-vector",
                    "scope": "forecast-envelope/v3",
                },
            },
            {
                "role": "lifecycle_target",
                "path": "proofs/targets/f-vector.lifecycle.event-withdrawn.json",
                "size": 768,
                "digest": digest("2"),
                "binding": {
                    "question_id": "q-vector",
                    "forecast_id": "f-vector",
                    "checkpoint_id": "checkpoint-withdrawn",
                    "head_event_id": "event-withdrawn",
                    "scope": "forecast-lifecycle/v2",
                },
            },
            {
                "role": "rfc3161_request",
                "path": "proofs/timestamps/f-vector.tsq",
                "size": 64,
                "digest": digest("3"),
                "references": {"target_path": "proofs/targets/f-vector.json"},
                "binding": {
                    "hash_algorithm": "sha256",
                    "message_imprint_sha256": "1" * 64,
                },
            },
            {
                "role": "rfc3161_response",
                "path": "proofs/timestamps/f-vector.tsr",
                "size": 1024,
                "digest": digest("4"),
                "references": {
                    "target_path": "proofs/targets/f-vector.json",
                    "request_path": "proofs/timestamps/f-vector.tsq",
                    "trust_path": "trust/tsa-ca.pem",
                },
                "binding": {"tsa_url": "https://tsa.example.test/"},
            },
            {
                "role": "x509_ca_bundle",
                "path": "trust/tsa-ca.pem",
                "size": 2048,
                "digest": digest("5"),
                "format": "pem-certificate-bundle",
            },
        ],
    }


def vectorize_index(value: dict[str, Any], *, allow_empty: bool) -> dict[str, Any]:
    data = encode_evidence_index(value, allow_empty=allow_empty)
    return {
        "schema": "forecast-evidence-index-test-vector/v1",
        "allow_empty": allow_empty,
        "index": value,
        "expected": {
            "canonical_json": data.decode("utf-8"),
            "sha256": hashlib.sha256(data).hexdigest(),
        },
    }


def publication_vector(index: dict[str, Any], *, name: str) -> dict[str, Any]:
    index_bytes = encode_evidence_index(index, allow_empty=True)
    entries = [
        {
            "role": "ledger",
            "path": "ledger.json",
            "size": 1024,
            "digest": digest("a"),
        },
        {
            "role": "evidence_index",
            "path": "proofs/evidence-index.json",
            "size": len(index_bytes),
            "digest": {
                "algorithm": "sha-256",
                "value": hashlib.sha256(index_bytes).hexdigest(),
            },
        },
    ]
    entries.extend(
        {
            "role": entry["role"],
            "path": entry["path"],
            "size": entry["size"],
            "digest": entry["digest"],
        }
        for entry in index["entries"]
    )
    entries.sort(key=lambda entry: entry["path"])
    manifest = {
        "profile": "forecast-ledger-publication/v3",
        "contract": index["contract"],
        "ledger_path": "ledger.json",
        "evidence_index_path": "proofs/evidence-index.json",
        "entries": entries,
    }
    data = encode_publication(manifest, index)
    return {
        "schema": "forecast-ledger-publication-test-vector/v3",
        "name": name,
        "evidence_index": index,
        "manifest": manifest,
        "expected": {
            "canonical_json": data.decode("utf-8"),
            "sha256": hashlib.sha256(data).hexdigest(),
        },
    }


def update_lifecycle_fixture(vector: dict[str, Any]) -> None:
    fixture_path = ROOT / "tests/conformance/valid/lifecycle-checkpoints.json"
    fixture = json.loads(fixture_path.read_text(encoding="utf-8"))
    forecast = fixture["questions"][0]["forecasts"][0]
    expected = {item["head_event_id"]: item["expected"] for item in vector["checkpoints"]}
    target_dir = VECTORS / "targets"
    target_dir.mkdir(parents=True, exist_ok=True)
    revision = fixture["questions"][0]["revisions"][0]
    for checkpoint in forecast["activity_checkpoints"]:
        head = checkpoint["head_event_id"]
        target = forecast_lifecycle_target(fixture["questions"][0]["id"], forecast, revision, head)
        data = canonicalize(target)
        filename = f"forecast-lifecycle-v2-{head.removeprefix('event-')}.json"
        (target_dir / filename).write_bytes(data)
        checkpoint["integrity"] = {
            "status": "retained",
            "target": {
                "scope": "forecast-lifecycle/v2",
                "canonicalization": "RFC8785",
                "artifact_path": f"tests/vectors/targets/{filename}",
                "digest": {
                    "algorithm": "sha-256",
                    "value": expected[head]["sha256"],
                },
            },
        }
    write_json(fixture_path, fixture)


def update_revealed_fixtures(
    seal_vector: dict[str, Any], presence_vector: dict[str, Any]
) -> None:
    team_path = ROOT / "examples/valid/team-ledger.yaml"
    yaml = YAML(typ="rt")
    yaml.preserve_quotes = True
    yaml.width = 4096
    team = yaml.load(team_path.read_text(encoding="utf-8"))
    forecast = team["questions"][0]["forecasts"][0]
    forecast["commitment"] = {
        **seal_vector["expected"]["commitment"],
        "revealed_at": "2026-10-01T09:00:00+01:00",
        "revealed_key": seal_vector["material"]["key_hex"],
    }
    with team_path.open("w", encoding="utf-8") as stream:
        yaml.dump(team, stream)

    fixture_path = ROOT / "tests/conformance/valid/revealed-representation-only.json"
    fixture = json.loads(fixture_path.read_text(encoding="utf-8"))
    forecast = fixture["questions"][0]["forecasts"][0]
    presence = presence_vector["cases"][0]["expected"]
    forecast["commitment"] = {
        "scheme": "forecast-seal/v3",
        "commitment_hash": {
            "algorithm": "sha-256",
            "value": presence["commitment_sha256"],
        },
        "encryption": {
            "algorithm": "chacha20-poly1305",
            "nonce": "MzMzMzMzMzMzMzMz",
            "ciphertext": presence["ciphertext_base64"],
        },
        "key_hint": presence_vector["key_hint"],
        "revealed_at": "2026-09-03T10:00:00Z",
        "revealed_key": presence_vector["material"]["key_hex"],
    }
    write_json(fixture_path, fixture)


def create_retained_forecast_fixture() -> None:
    source_path = ROOT / "examples/valid/individual-ledger.json"
    source = json.loads(source_path.read_text(encoding="utf-8"))
    source["ledger_id"] = "retained-forecast-fixture"
    question = source["questions"][0]
    forecast = question["forecasts"][0]
    revisions = {revision["id"]: revision for revision in question["revisions"]}
    data = canonicalize(
        forecast_envelope(question["id"], forecast, revisions[forecast["question_revision_id"]])
    )
    filename = f"{forecast['id']}.forecast-envelope-v3.json"
    target_path = VECTORS / "targets" / filename
    target_path.write_bytes(data)
    forecast["integrity"] = {
        "status": "retained",
        "target": {
            "scope": "forecast-envelope/v3",
            "canonicalization": "RFC8785",
            "artifact_path": f"tests/vectors/targets/{filename}",
            "digest": {
                "algorithm": "sha-256",
                "value": hashlib.sha256(data).hexdigest(),
            },
        },
    }
    write_json(ROOT / "tests/conformance/valid/retained-forecast.json", source)


def main() -> int:
    seal = _demo_vector()
    presence = _presence_demo_vector()
    write_json(VECTORS / "forecast-seal-v3.json", seal)
    write_json(VECTORS / "forecast-key-v3.json", _key_demo_vector())
    write_json(VECTORS / "forecast-seal-v3-presence.json", presence)
    lifecycle = _lifecycle_demo_vector()
    write_json(VECTORS / "forecast-lifecycle-v2.json", lifecycle)
    update_target_vector(VECTORS / "forecast-envelope-v3-public-lifecycle.json")
    update_target_vector(VECTORS / "forecast-envelope-v3-sealed-lifecycle.json")
    update_lifecycle_fixture(lifecycle)
    update_revealed_fixtures(seal, presence)
    create_retained_forecast_fixture()

    index = evidence_index()
    empty_index = {
        "schema": "forecast-evidence-index/v1",
        "ledger_id": "empty-example-ledger",
        "contract": contract_identity(),
        "entries": [],
    }
    write_json(
        VECTORS / "forecast-evidence-index-v1.json",
        vectorize_index(index, allow_empty=False),
    )
    write_json(
        VECTORS / "forecast-evidence-index-v1-empty.json",
        vectorize_index(empty_index, allow_empty=True),
    )
    write_json(
        VECTORS / "forecast-ledger-publication-v3.json",
        publication_vector(index, name="complete-evidence"),
    )
    write_json(
        VECTORS / "forecast-ledger-publication-v3-empty.json",
        publication_vector(empty_index, name="empty-evidence"),
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
