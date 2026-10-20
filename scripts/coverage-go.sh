#!/bin/sh
set -eu

cd "$(dirname "$0")/.."
mkdir -p coverage

go test -count=1 -race -covermode=atomic -coverpkg=./internal/... \
	-coverprofile=coverage/go.out ./internal/...
go tool cover -func=coverage/go.out > coverage/go-summary.txt
go tool cover -html=coverage/go.out -o coverage/go.html

total=$(awk '/^total:/ { gsub(/%/, "", $3); print $3 }' coverage/go-summary.txt)
if [ -z "$total" ]; then
	printf '%s\n' 'Could not read total Go coverage.' >&2
	exit 1
fi
if ! awk -v total="$total" 'BEGIN { exit !(total >= 80) }'; then
	printf 'Go internal coverage %s%% is below the 80%% floor.\n' "$total" >&2
	exit 1
fi
printf 'Go internal coverage: %s%% (80%% floor)\n' "$total"
