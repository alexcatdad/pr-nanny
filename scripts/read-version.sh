#!/bin/sh
set -eu

ROOT=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
VERSION_FILE=${1:-"$ROOT/VERSION"}
[ -f "$VERSION_FILE" ] || { echo 'VERSION file does not exist' >&2; exit 2; }
[ "$(awk 'END { print NR }' "$VERSION_FILE")" -eq 1 ] || {
  echo 'VERSION must contain exactly one line' >&2
  exit 2
}
VERSION=$(cat "$VERSION_FILE")
numeric='(0|[1-9][0-9]*)'
prerelease_id="(0|[1-9][0-9]*|[0-9A-Za-z-]*[A-Za-z-][0-9A-Za-z-]*)"
build_id='[0-9A-Za-z-]+'
pattern="^$numeric\\.$numeric\\.$numeric(-$prerelease_id(\\.$prerelease_id)*)?(\\+$build_id(\\.$build_id)*)?$"
printf '%s\n' "$VERSION" | grep -Eq "$pattern" || {
  echo 'VERSION must contain one valid SemVer value' >&2
  exit 2
}
printf '%s\n' "$VERSION"
