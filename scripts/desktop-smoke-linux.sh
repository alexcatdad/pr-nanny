#!/bin/sh
set -eu
[ "${GITHUB_ACTIONS:-}" = true ] || { echo 'Desktop smoke requires a disposable CI runner' >&2; exit 2; }
binary=${1:?usage: desktop-smoke-linux.sh native-binary}
dunst > "${RUNNER_TEMP:?}/pr-nanny-dunst.log" 2>&1 &
daemon_pid=$!
trap 'kill "$daemon_pid" 2>/dev/null || true; wait "$daemon_pid" 2>/dev/null || true' EXIT HUP INT TERM
timeout 15 gdbus wait --session org.freedesktop.Notifications
"$binary" --desktop-smoke-test
displayed=$(dunstctl count displayed)
case "$displayed" in ''|*[!0-9]*) echo 'Invalid notification daemon count' >&2; exit 1;; esac
[ "$displayed" -ge 1 ] || { echo 'Native notification did not reach the daemon' >&2; exit 1; }
echo 'Native tray activation, autostart roundtrip, frontend IPC and notification daemon delivery passed.'
