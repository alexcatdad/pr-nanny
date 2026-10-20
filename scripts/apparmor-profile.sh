#!/bin/sh
set -eu
# Generate Ubuntu's application-scoped user-namespace allowance. Loading the
# profile is a separate administrator action. WebKit's own sandbox stays enabled.
binary=${1:?usage: apparmor-profile.sh /absolute/path/to/pr-nanny}
case "$binary" in /*) :;; *) echo 'Use an absolute executable path' >&2; exit 2;; esac
case "$binary" in *[!A-Za-z0-9_./\ -]*) echo 'Unsupported characters in executable path' >&2; exit 2;; esac
[ -x "$binary" ] || { echo 'Executable does not exist or is not executable' >&2; exit 2; }
if command -v realpath >/dev/null 2>&1; then binary=$(realpath "$binary"); fi
case "$binary" in *[!A-Za-z0-9_./\ -]*) echo 'Unsupported resolved executable path' >&2; exit 2;; esac
cat <<EOF_PROFILE
abi <abi/4.0>,
include <tunables/global>
profile pr-nanny "$binary" flags=(unconfined) {
  userns,
}
EOF_PROFILE
