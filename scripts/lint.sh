#!/bin/sh
# Check only; formatting fixes are always an explicit developer action.
set -eu
root=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
cd "$root"
case "${1:-}" in ''|--backend) ;; *) echo 'usage: sh scripts/lint.sh [--backend]' >&2; exit 2;; esac
unformatted=$(git ls-files '*.go' | xargs gofmt -l)
[ -z "$unformatted" ] || { printf 'Run gofmt on:\n%s\n' "$unformatted" >&2; exit 1; }
go vet ./internal/...
mkdir -p .tools
go -C tools/lint build -o ../../.tools/golangci-lint github.com/golangci/golangci-lint/v2/cmd/golangci-lint
.tools/golangci-lint run --config "$root/.golangci.yml" ./internal/...
shellcheck scripts/*.sh
go run github.com/rhysd/actionlint/cmd/actionlint@v1.7.12 -pyflakes=
zizmor --offline --no-progress --min-severity low .github/workflows
if [ "${1:-}" != --backend ]; then
  (cd frontend && pnpm lint && pnpm check)
fi
