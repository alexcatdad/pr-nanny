//go:build production && (mcp || devtools || server)

package main

// Production must never be combined with capabilities that expose development interfaces.
var _ [-(1)]byte
