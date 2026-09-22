#!/usr/bin/env python3
"""Build immutable RFC 8785 forecast-envelope targets for timestamping."""

from __future__ import annotations

import argparse
import json
from pathlib import Path

from forecast_crypto import (
    canonicalize,
    forecast_envelope,
    forecast_lifecycle_target,
    sha256_ref,
)
from validate import load_document


def build_targets(ledger: dict, output: Path) -> list[dict]:
    """Write one canonical v2 envelope per forecast and return the build report."""

    output.mkdir(parents=True, exist_ok=True)
    report = []
    for question in ledger["questions"]:
        revisions = {revision["id"]: revision for revision in question["revisions"]}
        for forecast in question["forecasts"]:
            revision = revisions[forecast["question_revision_id"]]
            envelope = forecast_envelope(question["id"], forecast, revision)
            data = canonicalize(envelope)
            path = output / f"{forecast['id']}.json"
            path.write_bytes(data)
            report.append(
                {
                    "scope": "forecast-envelope/v2",
                    "question_id": question["id"],
                    "forecast_id": forecast["id"],
                    "artifact_path": path.as_posix(),
                    "digest": sha256_ref(data),
                }
            )
            for checkpoint in forecast.get("activity_checkpoints", []):
                lifecycle = forecast_lifecycle_target(
                    question["id"],
                    forecast,
                    revision,
                    checkpoint["head_event_id"],
                )
                lifecycle_data = canonicalize(lifecycle)
                lifecycle_path = output / (
                    f"{forecast['id']}.lifecycle.{checkpoint['head_event_id']}.json"
                )
                lifecycle_path.write_bytes(lifecycle_data)
                report.append(
                    {
                        "scope": "forecast-lifecycle/v1",
                        "question_id": question["id"],
                        "forecast_id": forecast["id"],
                        "head_event_id": checkpoint["head_event_id"],
                        "artifact_path": lifecycle_path.as_posix(),
                        "digest": sha256_ref(lifecycle_data),
                    }
                )
    return report


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("ledger", type=Path)
    parser.add_argument("--output", type=Path, default=Path("proofs/targets"))
    args = parser.parse_args()

    ledger = load_document(args.ledger)
    report = build_targets(ledger, args.output)
    print(json.dumps(report, indent=2))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
