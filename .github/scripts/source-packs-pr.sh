#!/usr/bin/env bash
# Commits a change to the source packs files to the branch source-packs/update
# and opens a draft pull request for it, or updates the one already open.
# Does nothing when the files are unchanged. Needs GH_TOKEN, and REPORT: the
# Markdown report of the run, used as the pull request's body.
#
# The pull request is a draft because one opened with the workflow's token
# starts no CI run. Marking it ready for review, which a person does, starts
# CI, and it can then merge.
set -euo pipefail

files=(source-packs.toml source-packs.skipped.txt)
branch=source-packs/update
# GitHub refuses a body over 65536 characters.
max_body=60000

if [[ -z $(git status --porcelain -- "${files[@]}") ]]; then
  echo "source packs unchanged: no pull request"
  exit 0
fi

title=$(sed -n '1s/^## //p' "$REPORT")
if [[ -z $title ]]; then title="Update the source packs"; fi
title="Source packs: $title"

body=$(mktemp)
head -c "$max_body" "$REPORT" >"$body"
if [[ $(wc -c <"$REPORT") -gt $max_body ]]; then
  printf '\n\n…cut short: the whole report is in the run summary.\n' >>"$body"
fi
{
  printf '\n---\n\n'
  printf 'Opened by the Source packs workflow'
  if [[ -n ${GITHUB_RUN_ID:-} ]]; then
    printf ' ([run](%s/%s/actions/runs/%s))' "${GITHUB_SERVER_URL:-https://github.com}" "${GITHUB_REPOSITORY:-}" "$GITHUB_RUN_ID"
  fi
  printf '. It is a draft because a pull request opened by a workflow starts no CI: read the list, then mark it **Ready for review** to run the checks and merge.\n'
} >>"$body"

git config user.name "github-actions[bot]"
git config user.email "41898282+github-actions[bot]@users.noreply.github.com"
git switch --quiet --force-create "$branch"
for f in "${files[@]}"; do
  if [[ -e $f ]]; then git add -- "$f"; fi
done
# The hooks run the gate, which CI runs on the pull request anyway.
git commit --quiet --no-verify -m "$title"
git push --quiet --force origin "$branch"

if [[ -n $(gh pr list --head "$branch" --state open --json number --jq '.[].number') ]]; then
  gh pr edit "$branch" --title "$title" --body-file "$body"
  echo "updated the open pull request"
else
  gh pr create --draft --base main --head "$branch" --title "$title" --body-file "$body"
fi
