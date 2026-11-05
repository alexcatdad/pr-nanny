#!/bin/sh
set -eu
# The shipped helper must be a production build: production tag, no
# development tags, no Wails, and no test fixtures compiled in.
binary=${1:?usage: check-production.sh pr-nanny-helper}
metadata=$(go version -m "$binary")
printf '%s\n' "$metadata" | grep -Eq 'build[[:space:]]+-tags=production$' || { echo 'Production build tag missing' >&2; exit 1; }
if printf '%s\n' "$metadata" | grep -Eq -- '-tags=.*(mcp|devtools|server)'; then
  echo 'Development tags in production binary' >&2; exit 1
fi
if printf '%s\n' "$metadata" | grep -q 'wailsapp/wails'; then
  echo 'Wails linked into the helper' >&2; exit 1
fi
if strings "$binary" | grep -Eq 'fixture-account|synthetic-pat'; then
  echo 'Test fixture data present in production binary' >&2; exit 1
fi
echo 'Production helper checks passed.'
