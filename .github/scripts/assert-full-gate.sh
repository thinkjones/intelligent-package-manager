#!/usr/bin/env bash
# usage: assert-full-gate.sh <sha>
# Succeeds when "CI Gate (full suite)" from GitHub Actions concluded success
# for <sha>, or for its newest ancestor that has the check when every file
# changed since matches docs-only.txt. The release workflow runs this first.
# Needs gh, a full-history checkout and GITHUB_REPOSITORY. Tested by plan_test.sh.
set -euo pipefail
# shellcheck source=.github/scripts/plan.sh
source "$(dirname "${BASH_SOURCE[0]}")/plan.sh"

GATE="CI Gate (full suite)"
ACTIONS_APP=15368 # GitHub Actions: a check of the same name from another app doesn't count
LOOKBACK=50

# gate_conclusion <sha> prints the gate's latest conclusion for the commit, or nothing.
gate_conclusion() {
  gh api "repos/$GITHUB_REPOSITORY/commits/$1/check-runs?per_page=100" --paginate \
    --jq ".check_runs | map(select(.name == \"$GATE\" and .app.id == $ACTIONS_APP)) | sort_by(.completed_at) | last | .conclusion // empty"
}

# ancestors <sha> prints the commits before it, newest first.
ancestors() { git rev-list --first-parent --skip=1 --max-count="$LOOKBACK" "$1"; }

# changed_files <from> <to> prints the files that differ.
changed_files() { git diff --name-only "$1" "$2"; }

assert_full_gate() {
  local sha=$1 c a files=()
  c=$(gate_conclusion "$sha")
  if [[ $c == success ]]; then
    echo "$GATE is green for $sha"
    return 0
  fi
  if [[ -z $c ]]; then
    for a in $(ancestors "$sha"); do
      c=$(gate_conclusion "$a")
      [[ -z $c ]] && continue
      while IFS= read -r f; do
        [[ -n $f ]] && files+=("$f")
      done < <(changed_files "$a" "$sha")
      if [[ $c == success ]] && docs_only "${files[@]}"; then
        echo "$GATE is green for $a, and only docs changed up to $sha"
        return 0
      fi
      break
    done
  fi
  echo "::error::no green \"$GATE\" for $sha — run: gh workflow run ci.yml --ref main, wait for green, then release again" >&2
  return 1
}

if [[ ${BASH_SOURCE[0]} == "$0" ]]; then
  assert_full_gate "${1:?usage: assert-full-gate.sh <sha>}"
fi
