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

# pick_conclusion reads GitHub's check runs for one commit on stdin and prints
# the newest verdict, or nothing. Skipped (a run that scheduled no Tier 2),
# cancelled and unfinished runs are not verdicts.
pick_conclusion() {
  jq -r '[.check_runs[] | select(.conclusion != null and .conclusion != "skipped" and .conclusion != "cancelled")]
    | sort_by(.completed_at) | last | .conclusion // empty'
}

# gate_conclusion <sha> prints the gate's verdict for the commit, or nothing.
gate_conclusion() {
  gh api -X GET "repos/$GITHUB_REPOSITORY/commits/$1/check-runs" \
    -f check_name="$GATE" -F app_id="$ACTIONS_APP" -F per_page=100 | pick_conclusion
}

# ancestors <sha> prints the commits before it, newest first.
ancestors() { git rev-list --first-parent --skip=1 --max-count="$LOOKBACK" "$1"; }

# changed_files <from> <to> prints the files that differ.
# Renames count as a deletion and an addition, so code moved into docs/ shows.
changed_files() { git diff --no-renames --name-only "$1" "$2"; }

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
      # No files at all is the same tree, for example a commit and its revert.
      if [[ $c == success ]] && { [[ ${#files[@]} -eq 0 ]] || docs_only "${files[@]}"; }; then
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
