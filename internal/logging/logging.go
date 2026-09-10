// Package logging 提供可选的日志文件输出。
package logging

import (
	"io"
	"log/slog"
	"os"
)

// Setup 初始化全局日志。
//   - path 为空时，将日志重定向到 io.Discard，避免污染 TUI 屏幕。
//   - path 不为空时，日志追加到该文件。
//
// 返回的函数用于程序退出前关闭日志文件。
func Setup(path string) (func(), error) {
	noop := func() {}
	if path == "" {
		slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelInfo})))
		return noop, nil
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return noop, err
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(f, &slog.HandlerOptions{Level: slog.LevelDebug})))
	return func() { _ = f.Close() }, nil
}
