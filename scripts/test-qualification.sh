#!/bin/sh
set -eu
# The fake gh returns deterministic metadata, never a real account token.
root=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT HUP INT TERM
cat > "$work/gh" <<'EOF_GH'
#!/bin/sh
case "$*" in
 *'/jobs?'*)
   case "${QUALIFY_CASE:-pass}" in
    missing) printf '[{"jobs":[]}]';;
    *) printf '[{"jobs":['
       separator=''
       for name in 'Production frontend' 'Frontend and dependency audit' 'Backend coverage' 'Lint and formatting' 'Security gates' 'CodeQL (go)' 'CodeQL (javascript-typescript)' 'Native (macos-15)' 'Native (windows-2025)' 'Native (ubuntu-24.04)' 'Qualification'; do
        conclusion=success
        [ "${QUALIFY_CASE:-pass}" != one_failed ] || [ "$name" != 'CodeQL (go)' ] || conclusion=failure
        printf '%s{"name":"%s","status":"completed","conclusion":"%s"}' "$separator" "$name" "$conclusion"
        separator=,
       done
       printf ']}]';;
   esac;;
 *)
   case "${QUALIFY_CASE:-pass}" in
    pending) conclusion=null; status=in_progress; event=push; branch=main;;
    failed) conclusion='"failure"'; status=completed; event=push; branch=main;;
    pr) conclusion='"success"'; status=completed; event=pull_request; branch=feature;;
    wrongrepo) conclusion='"success"'; status=completed; event=push; branch=main; repo=alexcatdad/other;;
    *) conclusion='"success"'; status=completed; event=push; branch=main; repo=alexcatdad/pr-nanny;;
   esac
   repo=${repo:-alexcatdad/pr-nanny}
   printf '{"name":"Validate","path":".github/workflows/ci.yml","status":"%s","conclusion":%s,"event":"%s","head_branch":"%s","head_repository":{"full_name":"%s"},"head_sha":"0123456789abcdef0123456789abcdef01234567"}' "$status" "$conclusion" "$event" "$branch" "$repo";;
esac
EOF_GH
chmod +x "$work/gh"
version=$(sh "$root/scripts/read-version.sh")
[ "$version" = "$(cat "$root/VERSION")" ] || { echo 'Version reader did not preserve VERSION' >&2; exit 1; }
for invalid_version in '01.2.3' '0.1.0-01' '1.2' '1.2.3+'; do
 printf '%s\n' "$invalid_version" > "$work/VERSION.invalid"
 if sh "$root/scripts/read-version.sh" "$work/VERSION.invalid" >/dev/null 2>&1; then
  echo "VERSION reader accepted invalid SemVer: $invalid_version" >&2; exit 1
 fi
done
printf '%s\n%s\n' "$version" "$version" > "$work/VERSION.multiline"
if sh "$root/scripts/read-version.sh" "$work/VERSION.multiline" >/dev/null 2>&1; then
 echo 'VERSION reader accepted multiple lines' >&2; exit 1
fi
PATH="$work:$PATH" QUALIFY_CASE=pass sh "$root/scripts/qualify-release.sh" 123 >/dev/null
for scenario in pending failed pr missing one_failed wrongrepo; do
 if PATH="$work:$PATH" QUALIFY_CASE="$scenario" sh "$root/scripts/qualify-release.sh" 123 >/dev/null 2>&1; then
  echo "Qualification incorrectly accepted $scenario" >&2; exit 1
 fi
done
if sh "$root/scripts/package.sh" "$version-mismatch" darwin >/dev/null 2>&1; then
 echo 'Packaging accepted a version different from VERSION' >&2; exit 1
fi
if sh "$root/scripts/homebrew.sh" cask "$version-mismatch" https://example.invalid/archive.zip "$(printf '%064d' 0)" >/dev/null 2>&1; then
 echo 'Homebrew manifest generation accepted a version different from VERSION' >&2; exit 1
fi
manifest=$(sh "$root/scripts/homebrew.sh" cask "$version" https://example.invalid/archive.zip "$(printf '%064d' 0)")
printf '%s\n' "$manifest" | grep -q "version \"$version\"" || { echo 'Homebrew manifest ignored VERSION' >&2; exit 1; }
if PATH="$work:$PATH" sh "$root/scripts/qualify-release.sh" '123; echo unsafe' >/dev/null 2>&1; then
 echo 'Qualification accepted unsafe run ID' >&2; exit 1
fi
echo 'Release qualification fixture tests passed.'
