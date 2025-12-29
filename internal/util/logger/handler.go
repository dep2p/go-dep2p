package logger

import (
	"context"
	"io"
	"log/slog"
	"os"
	"sync"
)

var (
	// globalOutput 全局日志输出目标，默认为 stderr
	globalOutput   io.Writer = os.Stderr
	globalOutputMu sync.RWMutex
)

// dynamicWriter 是一个动态查找 globalOutput 的 io.Writer
// 这样即使在 logger 创建后修改 globalOutput，也能生效
type dynamicWriter struct{}

func (w *dynamicWriter) Write(p []byte) (n int, err error) {
	globalOutputMu.RLock()
	output := globalOutput
	globalOutputMu.RUnlock()
	return output.Write(p)
}

// subsystemHandler 是一个支持子系统级别控制的 slog.Handler
type subsystemHandler struct {
	subsystem string
	level     slog.Level
	inner     slog.Handler
	mu        sync.RWMutex
}

// newHandler 创建新的子系统 Handler
func newHandler(subsystem string, level slog.Level, format LogFormat) slog.Handler {
	opts := &slog.HandlerOptions{
		Level:     level,
		AddSource: ConfigFromEnv().AddSource,
		ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
			// 简化时间格式
			if a.Key == slog.TimeKey {
				a.Key = "ts"
			}
			// 简化级别名称
			if a.Key == slog.LevelKey {
				if lvl, ok := a.Value.Any().(slog.Level); ok {
					a.Value = slog.StringValue(levelToString(lvl))
				}
			}
			return a
		},
	}

	// 使用 dynamicWriter，这样即使 logger 创建后修改 globalOutput 也能生效
	output := &dynamicWriter{}

	var inner slog.Handler
	if format == FormatJSON {
		inner = slog.NewJSONHandler(output, opts)
	} else {
		inner = slog.NewTextHandler(output, opts)
	}

	// 添加 subsystem 属性
	inner = inner.WithAttrs([]slog.Attr{
		slog.String("subsystem", subsystem),
	})

	return &subsystemHandler{
		subsystem: subsystem,
		level:     level,
		inner:     inner,
	}
}

// Enabled 检查是否启用指定级别
func (h *subsystemHandler) Enabled(_ context.Context, level slog.Level) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return level >= h.level
}

// Handle 处理日志记录
func (h *subsystemHandler) Handle(ctx context.Context, r slog.Record) error {
	return h.inner.Handle(ctx, r)
}

// WithAttrs 添加属性
func (h *subsystemHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &subsystemHandler{
		subsystem: h.subsystem,
		level:     h.level,
		inner:     h.inner.WithAttrs(attrs),
	}
}

// WithGroup 添加组
func (h *subsystemHandler) WithGroup(name string) slog.Handler {
	return &subsystemHandler{
		subsystem: h.subsystem,
		level:     h.level,
		inner:     h.inner.WithGroup(name),
	}
}

// SetLevel 动态设置日志级别
func (h *subsystemHandler) SetLevel(level slog.Level) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.level = level
}

// levelToString 将日志级别转换为小写字符串
func levelToString(level slog.Level) string {
	switch level {
	case slog.LevelDebug:
		return "debug"
	case slog.LevelInfo:
		return "info"
	case slog.LevelWarn:
		return "warn"
	case slog.LevelError:
		return "error"
	default:
		return "info"
	}
}

// discardHandler 丢弃所有日志的 Handler（用于测试）
type discardHandler struct{}

func (discardHandler) Enabled(context.Context, slog.Level) bool  { return false }
func (discardHandler) Handle(context.Context, slog.Record) error { return nil }
func (d discardHandler) WithAttrs([]slog.Attr) slog.Handler      { return d }
func (d discardHandler) WithGroup(string) slog.Handler           { return d }

// DiscardHandler 返回一个丢弃所有日志的 Handler
func DiscardHandler() slog.Handler {
	return discardHandler{}
}

