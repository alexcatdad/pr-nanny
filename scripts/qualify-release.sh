#!/bin/sh
set -eu
run_id=${1:?usage: qualify-release.sh successful-main-validation-run-id}
case "$run_id" in ''|*[!0-9]*) echo 'Invalid validation run ID' >&2; exit 2;; esac
repo=${GITHUB_REPOSITORY:-alexcatdad/pr-nanny}
[ "$repo" = alexcatdad/pr-nanny ] || { echo 'Unexpected release repository' >&2; exit 1; }
run=$(gh api "repos/$repo/actions/runs/$run_id")
printf '%s' "$run" | jq -e --arg repo "$repo" '
 .name == "Validate" and .path == ".github/workflows/ci.yml" and
 .status == "completed" and .conclusion == "success" and
 .event == "push" and .head_branch == "main" and .head_repository.full_name == $repo
' >/dev/null || { echo 'Release requires a successful Validate push run on main' >&2; exit 1; }
sha=$(printf '%s' "$run" | jq -r '.head_sha')
case "$sha" in *[!0-9a-f]*|'') echo 'Invalid qualification commit' >&2; exit 1;; esac
[ "${#sha}" -eq 40 ] || { echo 'Invalid qualification commit' >&2; exit 1; }
jobs=$(gh api --paginate --slurp "repos/$repo/actions/runs/$run_id/jobs?per_page=100")
printf '%s' "$jobs" | jq -e '
 [.[].jobs[]] as $jobs |
 ["Production frontend","Frontend and dependency audit","Backend coverage","Lint and formatting","Security gates","CodeQL (go)","CodeQL (javascript-typescript)","Native (macos-15)","Native (windows-2025)","Native (ubuntu-24.04)","Qualification"] |
 all(.[]; . as $name | any($jobs[]; .name == $name and .status == "completed" and .conclusion == "success"))
' >/dev/null || { echo 'Required platform and security jobs did not all pass' >&2; exit 1; }
printf '%s\n' "$sha"
