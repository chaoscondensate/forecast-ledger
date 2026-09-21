#!/usr/bin/env bash

set -euo pipefail

binary=${1:-}
if [[ -z "$binary" || ! -x "$binary" ]]; then
  echo "usage: scripts/dogfood.sh /path/to/forecast-ledger" >&2
  exit 2
fi

work=$(mktemp -d)
cleanup() {
	if [[ "${DOGFOOD_KEEP:-}" == "1" ]]; then
		echo "dogfood files retained at $work" >&2
		return
	fi
  if [[ -n "${work:-}" && "$work" != "/" && -d "$work" ]]; then
    rm -rf -- "$work"
  fi
}
trap cleanup EXIT

ledger="$work/ledger.json"
key="$work/f-two.key"
package="$work/package"
backdated_ledger="$work/backdated-ledger.json"
yaml_ledger="$work/native-replacements.yaml"
yaml_key="$work/native-replacements.key"

# A historical ledger creation time must not become the default initial
# forecasted_at or recorded_at. Both omitted values come from this operation.
"$binary" --json init --file "$backdated_ledger" --ledger-id backdated --timezone UTC \
  --forecaster-id dogfood --forecaster-name Dogfood --created-at 2020-01-01T00:00:00Z \
  --question q-backdated --question-revision-id qr-backdated --question-title "Will the event happen?" \
  --question-resolution-criteria "Resolve from the named source." \
  --question-created-at 2020-01-01T00:00:00Z --question-expected-resolution-at 2099-01-02T00:00:00Z \
  --question-outcome-kind binary --initial-forecast f-backdated-001 \
  --initial-probability 0.5 --initial-probability-outcome >/dev/null
backdated_show=$("$binary" --json forecast show --file "$backdated_ledger" --question q-backdated --forecast f-backdated-001)
if grep -F '"forecasted_at":"2020-01-01T00:00:00Z"' <<<"$backdated_show" >/dev/null ||
   grep -F '"recorded_at":"2020-01-01T00:00:00Z"' <<<"$backdated_show" >/dev/null; then
  echo "init copied historical created_at into forecast times" >&2
  exit 1
fi

"$binary" --json init \
  --file "$ledger" \
  --ledger-id dogfood-ledger \
  --timezone UTC \
  --forecaster-id dogfood-forecaster \
  --forecaster-name "Dogfood Forecaster" \
  --created-at 2026-01-01T00:00:00Z \
  --question q-one --question-revision-id qr-one --question-title "Will it happen?" \
  --question-resolution-criteria "Resolve from the named source." \
  --question-created-at 2026-01-01T00:00:00Z --question-effective-at 2026-01-01T00:00:00Z \
  --question-revision-recorded-at 2026-01-01T00:00:00Z --question-expected-resolution-at 2027-01-01T00:00:00Z \
  --question-outcome-kind binary \
  --initial-forecast f-one --initial-forecasted-at 2026-01-01T00:00:00Z \
  --initial-recorded-at 2026-01-01T00:00:00Z --initial-probability 0.5 --initial-probability-outcome >/dev/null

"$binary" --json ledger update --file "$ledger" --title "Dogfood ledger" --description "Cross-platform command lifecycle" >/dev/null

"$binary" --json platform add --file "$ledger" --platform dogfood --name "Dogfood platform" --kind internal >/dev/null
"$binary" --json platform update --file "$ledger" --platform dogfood --name "Updated dogfood platform" >/dev/null
"$binary" --plain platform list --file "$ledger" | grep -F 'dogfood' >/dev/null
"$binary" --json platform show --file "$ledger" --platform dogfood | grep -F 'Updated dogfood platform' >/dev/null
"$binary" --json platform remove --file "$ledger" --platform dogfood --yes >/dev/null

set +e
"$binary" --json platform add --file "$ledger" --platform invalid-diagnostic --name "Invalid diagnostic" --kind informal --url not-a-url >"$work/diagnostic.out" 2>"$work/diagnostic.err"
diagnostic_exit=$?
set -e
if [[ $diagnostic_exit -ne 3 ]] || grep -E '"(line|column)":0' "$work/diagnostic.err" >/dev/null; then
  echo "semantic diagnostic returned a fabricated zero position" >&2
  exit 1
fi

"$binary" --json question add --file "$ledger" --question q-second --revision-id qr-second \
  --title "Will the second event happen?" --resolution-criteria "Resolve from the named source." \
  --created-at 2026-02-01T00:00:00Z --effective-at 2026-02-01T00:00:00Z \
  --revision-recorded-at 2026-02-01T00:00:00Z --expected-resolution-at 2027-01-02T00:00:00Z \
  --outcome-kind binary \
  --initial-forecast f-second-001 --initial-forecasted-at 2026-02-01T00:00:00Z \
  --initial-recorded-at 2026-02-01T00:01:00Z --initial-probability 0.4 --initial-probability-outcome >/dev/null

"$binary" --json forecast add --file "$ledger" --question q-one --forecast f-public-002 \
  --question-revision qr-one \
  --forecasted-at 2026-03-01T00:00:00Z --recorded-at 2026-03-01T00:01:00Z \
  --probability 0.6 --probability-outcome --rationale "Public dogfood revision." --supersedes-forecast f-one >/dev/null

private_canary='PRIVATE-DOGFOOD-CANARY'
printf '%s\n' "{\"representations\":[{\"kind\":\"probability\",\"outcome\":true,\"probability\":\"0.65\"}],\"rationale\":\"$private_canary\",\"key_factors\":[\"private factor\"],\"comment\":\"private comment\"}" |
  "$binary" --json forecast seal --file "$ledger" --question q-one --forecast f-sealed-003 \
  --question-revision qr-one \
  --forecasted-at 2026-04-01T00:00:00Z --recorded-at 2026-04-01T00:01:00Z \
  --supersedes-forecast f-public-002 --secret-input - --key-file "$key" >/dev/null

sealed_output=$("$binary" --plain forecast show --file "$ledger" --question q-one --forecast f-sealed-003)
if grep -F "$private_canary" <<<"$sealed_output" >/dev/null; then
  echo "sealed forecast output exposed private input" >&2
  exit 1
fi

"$binary" --json forecast reveal --file "$ledger" --question q-one --forecast f-sealed-003 --key-file "$key" --revealed-at 2026-04-02T00:00:00Z --yes >/dev/null
"$binary" --plain forecast list --file "$ledger" --question q-one | grep -F 'f-sealed-003' >/dev/null
"$binary" --plain question show --file "$ledger" --question q-one | grep -F 'Will it happen?' >/dev/null

unbuilt_check=$("$binary" --json target check --file "$ledger" --question q-one --forecast f-one)
if ! grep -F '"code":"target.checked"' <<<"$unbuilt_check" >/dev/null ||
   ! grep -F '"state":"not_applicable"' <<<"$unbuilt_check" >/dev/null ||
   ! grep -F 'content.no_retained_target' <<<"$unbuilt_check" >/dev/null; then
  echo "never-built target was not reported as not_applicable" >&2
  exit 1
fi
all_unbuilt=$("$binary" --json target check --file "$ledger" --all)
if ! grep -F 'f-sealed-003' <<<"$all_unbuilt" >/dev/null || ! grep -F 'content.no_retained_target' <<<"$all_unbuilt" >/dev/null; then
  echo "target check --all did not aggregate never-built forecasts" >&2
  exit 1
fi

"$binary" --json target build --file "$ledger" --question q-one --forecast f-one >/dev/null
"$binary" --json target check --file "$ledger" --question q-one --forecast f-one >/dev/null
"$binary" --json timestamp status --file "$ledger" --question q-one --forecast f-one | grep -F 'unanchored' >/dev/null

set +e
"$binary" --json timestamp stamp --file "$ledger" --question q-one --forecast f-one \
  --tsa-url https://tsa.invalid/ --ca-bundle trust/tsa.pem --offline \
  >"$work/offline.out" 2>"$work/offline.err"
offline_exit=$?
set -e
if [[ $offline_exit -ne 8 ]]; then
  echo "offline timestamp stamp returned $offline_exit, want 8" >&2
  exit 1
fi

"$binary" --json question update --file "$ledger" --question q-one --status closed --notes "Closed by dogfood lifecycle." >/dev/null
"$binary" --json question resolve --file "$ledger" --question q-one --outcome-boolean=true \
  --question-revision qr-one \
  --outcome-known-at 2027-01-01T00:00:00Z --recorded-at 2027-01-01T00:01:00Z \
  --source "Official result,https://example.org/result,2027-01-01T00:00:30Z" --yes >/dev/null

"$binary" --json question update --file "$ledger" --question q-second --status closed >/dev/null
"$binary" --json question void --file "$ledger" --question q-second \
  --reason "The second event was cancelled." --recorded-at 2027-01-02T00:01:00Z --yes >/dev/null
"$binary" --json question dispute --file "$ledger" --question q-second \
  --reason "The cancellation is under review." --recorded-at 2027-01-02T00:02:00Z --yes >/dev/null

# Keep the documented YAML path in the native Linux, macOS, and Windows
# lifecycle. These operations cover normalized scalar, mapping, and sequence
# replacements through the same binary used for filesystem smoke tests.
"$binary" --json init --file "$yaml_ledger" --ledger-id native-yaml --timezone UTC \
  --forecaster-id dogfood --forecaster-name Dogfood --created-at 2026-01-01T00:00:00Z >/dev/null
"$binary" --json platform add --file "$yaml_ledger" --platform native --name "Native platform" --kind self_hosted >/dev/null
"$binary" --json platform update --file "$yaml_ledger" --platform native --name "Updated native platform" --kind internal --account-username native >/dev/null
"$binary" --json question add --file "$yaml_ledger" --question q-yaml --revision-id qr-yaml \
  --title "Will YAML replacements work?" --resolution-criteria "Use the named result." \
  --created-at 2026-01-01T00:00:00Z --effective-at 2026-01-01T00:00:00Z \
  --revision-recorded-at 2026-01-01T00:00:00Z --expected-resolution-at 2027-01-01T00:00:00Z \
  --outcome-kind binary --tag initial >/dev/null
"$binary" --json forecast add --file "$yaml_ledger" --question q-yaml --forecast f-yaml \
  --question-revision qr-yaml \
  --forecasted-at 2026-02-01T00:00:00Z --recorded-at 2026-02-01T00:01:00Z \
  --probability 0.55 --probability-outcome >/dev/null
printf '%s\n' '{"representations":[{"kind":"probability","outcome":true,"probability":"0.65"}],"rationale":"PRIVATE-NATIVE-YAML","key_factors":["private"],"comment":"private"}' |
  "$binary" --json forecast seal --file "$yaml_ledger" --question q-yaml --forecast f-yaml-sealed \
  --question-revision qr-yaml \
  --forecasted-at 2026-03-01T00:00:00Z --recorded-at 2026-03-01T00:01:00Z \
  --secret-input - --key-file "$yaml_key" >/dev/null
"$binary" --json forecast reveal --file "$yaml_ledger" --question q-yaml --forecast f-yaml-sealed \
  --key-file "$yaml_key" --revealed-at 2026-03-02T00:00:00Z --yes >/dev/null
"$binary" --json question revise --file "$yaml_ledger" --question q-yaml \
  --revision-id qr-yaml-2 --effective-at 2026-04-01T00:00:00Z --revision-recorded-at 2026-04-01T00:01:00Z \
  --title "Updated YAML replacement question" --resolution-criteria "Use the named result." \
  --expected-resolution-at 2027-01-01T00:00:00Z --outcome-kind binary >/dev/null
"$binary" --json question update --file "$yaml_ledger" --question q-yaml --status closed --tag updated --tag native >/dev/null
"$binary" --json question void --file "$yaml_ledger" --question q-yaml \
  --reason "Native YAML lifecycle complete." --recorded-at 2027-01-01T00:01:00Z --yes >/dev/null
"$binary" --json validate --file "$yaml_ledger" >/dev/null
if grep -E '^[[:space:]]+[^#[:space:]][^:]*: \{.+\}$|^[[:space:]]+[^#[:space:]][^:]*: \[.+\]$' "$yaml_ledger" >/dev/null; then
  echo "native YAML lifecycle emitted a populated flow collection" >&2
  exit 1
fi

set +e
verify_output=$("$binary" --plain verify --file "$ledger" --offline)
verify_exit=$?
set -e
if [[ $verify_exit -ne 9 ]] || ! grep -F $'overall\tincomplete' <<<"$verify_output" >/dev/null; then
  echo "unexpected layered verification result (exit $verify_exit):" >&2
  echo "$verify_output" >&2
  exit 1
fi
"$binary" --json publish build --file "$ledger" --output "$package" >/dev/null
set +e
package_output=$("$binary" --plain publish verify --file "$package/ledger/ledger.json" --manifest "$package/manifest.json")
package_exit=$?
set -e
if [[ $package_exit -ne 9 ]] || ! grep -F $'overall\tincomplete' <<<"$package_output" >/dev/null; then
  echo "unexpected package verification result (exit $package_exit):" >&2
  echo "$package_output" >&2
  exit 1
fi

if find "$package" -type f -name '*.key' -print -quit | grep -q .; then
  echo "publication package contains a key file" >&2
  exit 1
fi
if find "$package" -type f -name 'f-one.json' -path '*/proofs/targets/*' -print -quit | grep -q .; then
  echo "publication package discovered an adjacent unreferenced target" >&2
  exit 1
fi
if awk 'length($0) > 300 && $0 !~ /"ciphertext"[[:space:]]*:/ { exit 1 }' "$ledger"; then
  :
else
  echo "ledger contains an unreadably long line" >&2
  exit 1
fi

"$binary" mcp serve --ledger-root "main=$work" --read-only </dev/null
"$binary" mcp serve --ledger-root "main=$work" --offline </dev/null

mkdir -p "$work/overlap"
set +e
"$binary" --json mcp serve --ledger-root "main=$work" --output-root "packages=$work/overlap" </dev/null >"$work/mcp-overlap.out" 2>"$work/mcp-overlap.err"
overlap_exit=$?
set -e
if [[ $overlap_exit -ne 5 ]] || ! grep -F '"first_route":"ledger:main"' "$work/mcp-overlap.err" >/dev/null || ! grep -F '"second_route":"output:packages"' "$work/mcp-overlap.err" >/dev/null || grep -F "$work" "$work/mcp-overlap.err" >/dev/null; then
  echo "MCP overlap diagnostic is incomplete or exposes an absolute path" >&2
  exit 1
fi

echo "dogfood lifecycle passed"
