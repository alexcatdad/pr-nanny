//go:build !production

package main

import "os"

// Preview is an isolated unauthenticated graphical host for explicit development/MCP testing.
func developmentPreview() bool { return len(os.Args) == 2 && os.Args[1] == "--dev-preview" }
