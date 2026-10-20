//go:build mcp && !production

package websecurity

import (
	"os"
	"strconv"
	"strings"
)

// Explicit development builds allow only the configured MCP loopback callback.
// Production cannot compile with mcp (see production_guard.go).
func contentPolicy() string {
	port, err := strconv.Atoi(os.Getenv("WAILS_MCP_PORT"))
	if err != nil || port < 1 || port > 65535 {
		return CSP
	}
	return strings.Replace(CSP, "connect-src 'self'", "connect-src 'self' http://127.0.0.1:"+strconv.Itoa(port), 1)
}
