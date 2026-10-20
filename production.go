//go:build production

package main

import (
	"io"
	"log/slog"
)

func developmentPreview() bool { return false }

func previewLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }
