//go:build !mcp

package websecurity

func contentPolicy() string { return CSP }
