#!/bin/sh
# Builds 'PR Nanny Dev.app' (bundle ID com.alexcatdad.pr-nanny.dev, ad-hoc
# signed, data in ~/Library/Application Support/pr-nanny-dev) for iterating on
# the menu-bar UI. It never touches the installed app.
#   sh macos/Scripts/dev-app.sh [--run]
set -eu
ROOT=$(CDPATH='' cd -- "$(dirname -- "$0")/../.." && pwd)
DEST="$ROOT/macos/.build/dev/PR Nanny Dev.app"
DEV_BUILD=1 sh "$ROOT/macos/Scripts/package-app.sh" "$DEST"
if [ "${1:-}" = --run ]; then
  EXE="$DEST/Contents/MacOS/PRNanny"
  # Stop only this exact dev executable, never the installed app.
  for pid in $(pgrep -x PRNanny || true); do
    if [ "$(ps -o comm= -p "$pid")" = "$EXE" ]; then kill -TERM "$pid"; fi
  done
  sleep 1
  open "$DEST"
fi
