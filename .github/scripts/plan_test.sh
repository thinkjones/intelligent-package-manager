#!/usr/bin/env bash
# Tests for plan.sh: mise run ci:plan-test.
set -uo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")" || exit 1
failed=0

# check <mode> <event> <suite> <changed, space separated> <expected output on one line, or FAIL>
check() {
  local mode=$1 event=$2 suite=$3 changed=${4// /$'\n'} want=$5 got rc
  got=$(MODE=$mode EVENT=$event SUITE=$suite CHANGED=$changed ./plan.sh 2>plan_test.err | tr '\n' ' ')
  rc=${PIPESTATUS[0]}
  if [[ $want == FAIL ]]; then
    if [[ $rc -eq 0 ]] || ! grep -q QUALITY_MODE plan_test.err; then
      echo "FAIL: MODE='$mode' $event should fail naming QUALITY_MODE (rc $rc: $(cat plan_test.err))"
      failed=1
    fi
  elif [[ $rc -ne 0 || ${got% } != "$want" ]]; then
    echo "FAIL: MODE=$mode $event suite=$suite changed='$4': got '${got% }' (rc $rc), want '$want'"
    failed=1
  fi
}

check fast pull_request "" "cli/x.go" "gate=true tier1=true full=false"
check prod pull_request "" "cli/x.go" "gate=true tier1=true full=true"
check fast push "" "cli/x.go" "gate=false tier1=false full=false"
check prod push "" "cli/x.go" "gate=true tier1=true full=true"
check fast workflow_dispatch full "" "gate=true tier1=true full=true"
check fast workflow_dispatch fast "" "gate=true tier1=true full=false"
check prod pull_request "" "docs/a.md README.md" "gate=true tier1=false full=false"
check prod pull_request "" "docs/guides/deep/a.png LICENSE" "gate=true tier1=false full=false"
check prod pull_request "" "docs/a.md mise.toml" "gate=true tier1=true full=true"
check prod pull_request "" ".github/workflows/ci.yml" "gate=true tier1=true full=true"
check prod pull_request "" ".github/docs-only.txt" "gate=true tier1=true full=true"
check prod pull_request "" "" "gate=true tier1=true full=true"
check prod push "" "docs/a.md" "gate=true tier1=true full=true"
check "" pull_request "" "cli/x.go" FAIL
check Fast pull_request "" "cli/x.go" FAIL

rm -f plan_test.err
[[ $failed -eq 0 ]] && echo "plan_test: ok"
exit $failed
