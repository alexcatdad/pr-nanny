#!/bin/sh
# Integration check: builds the dev bundle, validates it and runs the headless
# smoke test (bundled helper starts, answers health/version over its socket, stops).
set -eu
ROOT=$(CDPATH='' cd -- "$(dirname -- "$0")/../.." && pwd)
WORK=$(mktemp -d)
trap 'rm -rf "$WORK"' EXIT HUP INT TERM
APP="$WORK/PR Nanny Dev.app"
DEV_BUILD=1 sh "$ROOT/macos/Scripts/package-app.sh" "$APP"
codesign --verify --deep --strict "$APP"
"$APP/Contents/MacOS/PRNanny" --smoke-test
echo "test-package-app: ok"
