#!/usr/bin/env bash
# Decides what a CI run schedules. Reads MODE (the QUALITY_MODE repo variable),
# EVENT, SUITE (manual runs: full or fast) and CHANGED (a pull request's files,
# one per line) and prints gate=, tier1= and full= for $GITHUB_OUTPUT.
# Tested by plan_test.sh. release's assert-full-gate.sh sources docs_only.
set -euo pipefail

# docs_only succeeds when every file given matches a pattern in docs-only.txt.
docs_only() {
  local list f pattern ok
  list="$(dirname "${BASH_SOURCE[0]}")/../docs-only.txt"
  [[ $# -gt 0 ]] || return 1
  for f in "$@"; do
    ok=false
    while IFS= read -r pattern; do
      [[ -z $pattern || $pattern == \#* ]] && continue
      # In a case pattern * already crosses folders.
      pattern=${pattern//\*\*/*}
      # shellcheck disable=SC2254
      case $f in $pattern) ok=true ;; esac
    done <"$list"
    $ok || return 1
  done
}

plan() {
  local mode=${MODE:-} event=${EVENT:-} suite=${SUITE:-} gate=true tier1=true full=false files=()
  case $mode in
    fast | prod) ;;
    *)
      echo "::error::Repo variable QUALITY_MODE must be 'fast' or 'prod' (got '$mode') — gh variable set QUALITY_MODE --body fast" >&2
      return 1
      ;;
  esac
  case $event in
    workflow_dispatch) [[ $suite == full ]] && full=true ;;
    pull_request)
      [[ $mode == prod ]] && full=true
      while IFS= read -r f; do
        [[ -n $f ]] && files+=("$f")
      done <<<"${CHANGED:-}"
      # A docs-only pull request schedules nothing; the merge gate still reports.
      if [[ ${#files[@]} -gt 0 ]] && docs_only "${files[@]}"; then
        tier1=false full=false
      fi
      ;;
    push)
      # fast: a merge runs nothing, and reports no gate.
      if [[ $mode == prod ]]; then full=true; else gate=false tier1=false; fi
      ;;
    *)
      echo "::error::plan.sh: unknown event '$event'" >&2
      return 1
      ;;
  esac
  printf 'gate=%s\ntier1=%s\nfull=%s\n' "$gate" "$tier1" "$full"
}

if [[ ${BASH_SOURCE[0]} == "$0" ]]; then
  plan
fi
