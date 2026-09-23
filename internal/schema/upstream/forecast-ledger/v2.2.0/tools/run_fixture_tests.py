#!/usr/bin/env python3
"""Run the valid examples and mutation-based invalid fixture suite."""

from __future__ import annotations

import copy
import json
import sys
from pathlib import Path
from typing import Any

from validate import Problems, check_schema, check_semantics, load_document

ROOT = Path(__file__).resolve().parents[1]
SCHEMA_PATH = ROOT / "schema/forecast-ledger.schema.json"


def pointer_parent(document: Any, pointer: str) -> tuple[Any, str]:
    tokens = [token.replace("~1", "/").replace("~0", "~") for token in pointer.split("/")[1:]]
    current = document
    for token in tokens[:-1]:
        current = current[int(token)] if isinstance(current, list) else current[token]
    return current, tokens[-1]


def apply_operations(document: Any, operations: list[dict[str, Any]]) -> Any:
    result = copy.deepcopy(document)
    for operation in operations:
        parent, token = pointer_parent(result, operation["path"])
        key: Any = int(token) if isinstance(parent, list) else token
        if operation["op"] == "replace":
            parent[key] = operation["value"]
        elif operation["op"] == "add":
            if isinstance(parent, list):
                parent.insert(key, operation["value"])
            else:
                parent[key] = operation["value"]
        elif operation["op"] == "remove":
            del parent[key]
        else:
            raise ValueError(f"unsupported operation: {operation['op']}")
    return result


def problems_for(document: dict[str, Any], schema: dict[str, Any]) -> list[str]:
    problems = Problems()
    check_schema(document, schema, problems)
    if not problems.items:
        check_semantics(document, ROOT, problems)
    return problems.items


def main() -> int:
    schema = json.loads(SCHEMA_PATH.read_text(encoding="utf-8"))
    failed = False

    valid_paths = sorted((ROOT / "examples/valid").glob("*.*"))
    valid_paths += sorted((ROOT / "tests/conformance/valid").glob("*.*"))
    for path in valid_paths:
        errors = problems_for(load_document(path), schema)
        if errors:
            failed = True
            print(f"FAIL valid fixture {path.relative_to(ROOT)}")
            for error in errors:
                print(f"  - {error}")
        else:
            print(f"OK   valid fixture {path.relative_to(ROOT)}")

    cases = json.loads((ROOT / "tests/invalid-cases.json").read_text(encoding="utf-8"))
    for case in cases:
        base = load_document(ROOT / case["base"])
        mutated = apply_operations(base, case["operations"])
        errors = problems_for(mutated, schema)
        combined = "\n".join(errors)
        if not errors:
            failed = True
            print(f"FAIL invalid fixture {case['name']}: unexpectedly accepted")
        elif "expect_errors" in case:
            expected_errors = case["expect_errors"]
            if errors != expected_errors:
                failed = True
                print(f"FAIL invalid fixture {case['name']}: wrong rejection")
                for expected in expected_errors:
                    print(f"  - expected: {expected}")
                for error in errors:
                    print(f"  - actual: {error}")
            else:
                print(f"OK   invalid fixture {case['name']}")
        elif case["expect_contains"] not in combined:
            failed = True
            print(f"FAIL invalid fixture {case['name']}: wrong rejection")
            for error in errors:
                print(f"  - {error}")
        else:
            print(f"OK   invalid fixture {case['name']}")

    return 1 if failed else 0


if __name__ == "__main__":
    sys.exit(main())
