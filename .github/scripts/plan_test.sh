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

# A pipe after plan.sh would hide its failure from the workflow step.
if grep -nE 'plan\.sh[^#]*\|' ../workflows/ci.yml; then
  echo "FAIL: ci.yml pipes plan.sh; its exit status must reach the step"
  failed=1
fi

# assert-full-gate.sh, with the GitHub and git lookups replaced.
# gate <head conclusion> <parent conclusion> <files changed since the parent> <pass|fail>
gate() {
  local got=fail
  # The stand-ins are called by assert_full_gate; FILES is split on purpose.
  # shellcheck disable=SC1091,SC2329,SC2086
  if (
    source ./assert-full-gate.sh
    gate_conclusion() { if [[ $1 == head ]]; then echo "$HEAD_C"; elif [[ $1 == parent ]]; then echo "$PARENT_C"; fi; }
    ancestors() { printf 'older\nparent\noldest\n'; }
    changed_files() { printf '%s\n' $FILES; }
    HEAD_C=$1 PARENT_C=$2 FILES=$3 assert_full_gate head
  ) >/dev/null 2>plan_test.err; then got=pass; fi
  if [[ $got != "$4" ]]; then
    echo "FAIL: gate head='$1' parent='$2' files='$3': got $got, want $4"
    failed=1
  elif [[ $4 == fail ]] && ! grep -q "gh workflow run ci.yml --ref main" plan_test.err; then
    echo "FAIL: gate head='$1': the refusal should say how to run the full suite: $(cat plan_test.err)"
    failed=1
  fi
}

gate success "" "" pass
gate "" "" "" fail
gate failure success "docs/a.md" fail
gate "" success "docs/a.md README.md" pass
gate "" success "docs/a.md cli/x.go" fail
gate "" success "" fail
gate "" failure "docs/a.md" fail

rm -f plan_test.err
[[ $failed -eq 0 ]] && echo "plan_test: ok"
exit $failed
