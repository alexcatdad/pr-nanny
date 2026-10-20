#!/bin/sh
set -eu
# The fake gh returns deterministic metadata, never a real account token.
root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
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
       for name in 'Frontend and dependency audit' 'Native (macos-15)' 'Native (windows-2025)' 'Native (ubuntu-24.04)' 'Security gates'; do
        printf '%s{"name":"%s","status":"completed","conclusion":"success"}' "$separator" "$name"
        separator=,
       done
       printf ']}]';;
   esac;;
 *)
   case "${QUALIFY_CASE:-pass}" in
    pending) conclusion=null; status=in_progress; event=push; branch=main;;
    failed) conclusion='"failure"'; status=completed; event=push; branch=main;;
    pr) conclusion='"success"'; status=completed; event=pull_request; branch=feature;;
    *) conclusion='"success"'; status=completed; event=push; branch=main;;
   esac
   printf '{"name":"Validate","path":".github/workflows/ci.yml","status":"%s","conclusion":%s,"event":"%s","head_branch":"%s","head_repository":{"full_name":"alexcatdad/pr-nanny"},"head_sha":"0123456789abcdef0123456789abcdef01234567"}' "$status" "$conclusion" "$event" "$branch";;
esac
EOF_GH
chmod +x "$work/gh"
PATH="$work:$PATH" QUALIFY_CASE=pass sh "$root/scripts/qualify-release.sh" 123 >/dev/null
for scenario in pending failed pr missing; do
 if PATH="$work:$PATH" QUALIFY_CASE="$scenario" sh "$root/scripts/qualify-release.sh" 123 >/dev/null 2>&1; then
  echo "Qualification incorrectly accepted $scenario" >&2; exit 1
 fi
done
if PATH="$work:$PATH" sh "$root/scripts/qualify-release.sh" '123; echo unsafe' >/dev/null 2>&1; then
 echo 'Qualification accepted unsafe run ID' >&2; exit 1
fi
echo 'Release qualification fixture tests passed.'
