#!/usr/bin/env python3
"""Verify precise, source-aware, and secret-safe validator diagnostics."""

from __future__ import annotations

import copy
import json
import sys
from pathlib import Path
from tempfile import TemporaryDirectory

from validate import DocumentSyntaxError, load_document, validate

ROOT = Path(__file__).resolve().parents[1]
SCHEMA = ROOT / "schema/forecast-ledger.schema.json"


def require(condition: bool, message: str, errors: list[str]) -> None:
    if not condition:
        joined = "\n".join(errors)
        raise AssertionError(f"{message}\n{joined}")


def main() -> int:
    with TemporaryDirectory() as directory:
        temp = Path(directory)

        missing = load_document(ROOT / "examples/valid/team-ledger.yaml")
        del missing["questions"][0]["forecasts"][0]["representations"]
        missing_path = temp / "missing.json"
        missing_path.write_text(json.dumps(missing, indent=2), encoding="utf-8")
        errors = validate(missing_path, SCHEMA, ROOT)
        required = "/questions/0/forecasts/0/representations: required property is missing"
        require(required in errors, "missing property must use its exact JSON Pointer", errors)
        require(
            "line 1" not in "\n".join(errors),
            "missing property must not invent line 1",
            errors,
        )

        located = load_document(ROOT / "examples/valid/empty-ledger.json")
        located["schema_version"] = "0.0.0"
        located_text = json.dumps(located, indent=2) + "\n"
        located_path = temp / "located.json"
        located_path.write_text(located_text, encoding="utf-8")
        line = next(
            index
            for index, source_line in enumerate(located_text.splitlines(), start=1)
            if '"schema_version"' in source_line
        )
        errors = validate(located_path, SCHEMA, ROOT)
        require(
            any(error.startswith(f"/schema_version [line {line}, column ") for error in errors),
            "an invalid existing node must retain its source line and column",
            errors,
        )

        syntax_path = temp / "syntax.json"
        syntax_path.write_text('{\n  "schema_version": ]\n}\n', encoding="utf-8")
        try:
            validate(syntax_path, SCHEMA, ROOT)
        except DocumentSyntaxError as error:
            require(error.line == 2, "syntax error must retain its actual source line", [])
            require(error.column > 0, "syntax error must retain a bounded source column", [])
        else:
            raise AssertionError("invalid syntax was accepted")

        protected = copy.deepcopy(load_document(ROOT / "examples/valid/team-ledger.yaml"))
        secret = "c" * 63
        protected["questions"][0]["forecasts"][0]["commitment"]["revealed_key"] = secret
        protected_path = temp / "protected.json"
        protected_path.write_text(json.dumps(protected, indent=2), encoding="utf-8")
        errors = validate(protected_path, SCHEMA, ROOT)
        rendered = "\n".join(errors)
        require(secret not in rendered, "diagnostics must not expose secret values", errors)
        require(str(ROOT) not in rendered, "diagnostics must not expose unrestricted paths", errors)

    print("OK   precise missing-property pointer")
    print("OK   existing-node source location")
    print("OK   syntax-error source location")
    print("OK   secret-safe diagnostic rendering")
    return 0


if __name__ == "__main__":
    sys.exit(main())
