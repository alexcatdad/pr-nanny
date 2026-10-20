//go:build !mcp || production

package websecurity

func contentPolicy() string { return CSP }
