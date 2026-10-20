#!/bin/sh
set -eu
# Inspect the selected Go files as well as embedded build metadata. MCP can be
# compiled into Wails independently of production, so checking the tag alone is insufficient.
production_binary=${1:?usage: check-production.sh native-binary}
production_files=$(go list -tags production -f '{{join .GoFiles " "}}' github.com/wailsapp/wails/v3/pkg/application)
case "$production_files" in *application_production.go*mcp_disabled.go*) :;; *) echo 'Production Wails policy files missing' >&2; exit 1;; esac
case "$production_files" in *mcp_enabled.go*|*mcp_tools_enabled.go*|*application_dev.go*|*webview_window_darwin_dev.go*|*webview_window_windows_dev.go*) echo 'Development Wails files selected' >&2; exit 1;; esac
production_metadata=$(go version -m "$production_binary")
printf '%s\n' "$production_metadata" | grep -Eq 'build[[:space:]]+-tags=production$'
if printf '%s\n' "$production_metadata" | grep -Eq -- '-tags=.*(mcp|devtools|server)'; then
 echo 'Development tags in production binary' >&2; exit 1
fi
if strings "$production_binary" | grep -Eq 'WAILS_MCP_HOST|WAILS_MCP_TOKEN|window\.__wailsMCP'; then
 echo 'MCP interface present in production binary' >&2; exit 1
fi
if [ -d frontend/dist ] && grep -R -E -q '__PR_NANNY_TEST_BRIDGE__|acme/design-system|fixture-account' frontend/dist; then
 echo 'Test bridge or fixture data present in production frontend' >&2; exit 1
fi
echo 'Production debug/MCP/fixture checks passed.'
