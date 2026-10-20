//go:build !production

package main

import (
	"context"
	"log/slog"
	"os"
)

type messageOnly struct{ base slog.Handler }

func (h messageOnly) Enabled(c context.Context, l slog.Level) bool { return h.base.Enabled(c, l) }
func (h messageOnly) Handle(c context.Context, r slog.Record) error {
	return h.base.Handle(c, slog.NewRecord(r.Time, r.Level, r.Message, r.PC))
}
func (h messageOnly) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h messageOnly) WithGroup(string) slog.Handler      { return h }
func previewLogger() *slog.Logger {
	return slog.New(messageOnly{slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug})})
}
