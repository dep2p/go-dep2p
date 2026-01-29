// Package log 提供 DeP2P 统一日志接口
//
// 基于 Go 标准库 log/slog 封装，提供简洁的日志 API。
// 直接使用，无需抽象接口。
package log

import (
	"context"
	"io"
	"log/slog"
	"os"
)

// 默认 logger
var defaultLogger = slog.Default()

// 日志级别常量（从 slog 导出，方便使用）
const (
	LevelDebug = slog.LevelDebug
	LevelInfo  = slog.LevelInfo
	LevelWarn  = slog.LevelWarn
	LevelError = slog.LevelError
)

// SetDefault 设置默认 logger
func SetDefault(l *slog.Logger) {
	defaultLogger = l
	slog.SetDefault(l)
}

// Default 返回默认 logger
func Default() *slog.Logger {
	return slog.Default()
}

// New 创建新的 logger
func New(w io.Writer, opts *slog.HandlerOptions) *slog.Logger {
	if opts == nil {
		opts = &slog.HandlerOptions{}
	}
	return slog.New(slog.NewTextHandler(w, opts))
}

// NewJSON 创建 JSON 格式的 logger
func NewJSON(w io.Writer, opts *slog.HandlerOptions) *slog.Logger {
	if opts == nil {
		opts = &slog.HandlerOptions{}
	}
	return slog.New(slog.NewJSONHandler(w, opts))
}

// SetOutput 设置日志输出目标
//
// 重新创建默认 logger，将输出重定向到指定的 Writer。
// 常用于将日志输出到文件。
//
// 示例：
//
//	file, _ := os.OpenFile("app.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
//	log.SetOutput(file)
func SetOutput(w io.Writer) {
	opts := &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}
	defaultLogger = slog.New(slog.NewTextHandler(w, opts))
	slog.SetDefault(defaultLogger)
}

// SetOutputWithLevel 同时设置日志输出目标和级别
//
// 重新创建默认 logger，将输出重定向到指定的 Writer，并设置日志级别。
// 用于需要同时配置输出和级别的场景（如 Demo 中启用 DEBUG 日志）。
//
// 示例：
//
//	file, _ := os.OpenFile("app.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
//	log.SetOutputWithLevel(file, slog.LevelDebug)
func SetOutputWithLevel(w io.Writer, level slog.Level) {
	opts := &slog.HandlerOptions{
		Level: level,
	}
	defaultLogger = slog.New(slog.NewTextHandler(w, opts))
	slog.SetDefault(defaultLogger)
}

// SetLevel 设置日志级别
//
// 重新创建默认 logger，使用指定的日志级别。
func SetLevel(level slog.Level) {
	opts := &slog.HandlerOptions{
		Level: level,
	}
	defaultLogger = slog.New(slog.NewTextHandler(os.Stderr, opts))
	slog.SetDefault(defaultLogger)
}

// ============================================================================
//                              LazyLogger
// ============================================================================

// LazyLogger 懒加载 logger
//
// 每次日志调用时都从 slog.Default() 获取最新的 handler，
// 支持在运行时动态切换日志输出目标。
//
// 使用方式：
//
//	var myLog = log.Logger("mycomponent")  // 返回 *LazyLogger
//	myLog.Info("hello")                     // 动态使用当前的 default logger
type LazyLogger struct {
	component string
}

// Debug 输出 Debug 级别日志
func (l *LazyLogger) Debug(msg string, args ...any) {
	slog.Default().With("component", l.component).Debug(msg, args...)
}

// Info 输出 Info 级别日志
func (l *LazyLogger) Info(msg string, args ...any) {
	slog.Default().With("component", l.component).Info(msg, args...)
}

// Warn 输出 Warn 级别日志
func (l *LazyLogger) Warn(msg string, args ...any) {
	slog.Default().With("component", l.component).Warn(msg, args...)
}

// Error 输出 Error 级别日志
func (l *LazyLogger) Error(msg string, args ...any) {
	slog.Default().With("component", l.component).Error(msg, args...)
}

// DebugContext 带 context 的 Debug 日志
func (l *LazyLogger) DebugContext(ctx context.Context, msg string, args ...any) {
	slog.Default().With("component", l.component).DebugContext(ctx, msg, args...)
}

// InfoContext 带 context 的 Info 日志
func (l *LazyLogger) InfoContext(ctx context.Context, msg string, args ...any) {
	slog.Default().With("component", l.component).InfoContext(ctx, msg, args...)
}

// WarnContext 带 context 的 Warn 日志
func (l *LazyLogger) WarnContext(ctx context.Context, msg string, args ...any) {
	slog.Default().With("component", l.component).WarnContext(ctx, msg, args...)
}

// ErrorContext 带 context 的 Error 日志
func (l *LazyLogger) ErrorContext(ctx context.Context, msg string, args ...any) {
	slog.Default().With("component", l.component).ErrorContext(ctx, msg, args...)
}

// With 添加额外的属性
func (l *LazyLogger) With(args ...any) *slog.Logger {
	return slog.Default().With("component", l.component).With(args...)
}

// WithComponent 返回带组件名的 LazyLogger
func WithComponent(component string) *LazyLogger {
	return &LazyLogger{component: component}
}

// Logger 返回带组件名的 LazyLogger
//
// 返回的 LazyLogger 会在每次日志调用时使用当前的 slog.Default()，
// 支持在运行时动态切换日志输出目标。
func Logger(component string) *LazyLogger {
	return &LazyLogger{component: component}
}

// ============================================================================
//                              快捷方法
// ============================================================================

// Debug 输出 Debug 级别日志
func Debug(msg string, args ...any) {
	slog.Default().Debug(msg, args...)
}

// Info 输出 Info 级别日志
func Info(msg string, args ...any) {
	slog.Default().Info(msg, args...)
}

// Warn 输出 Warn 级别日志
func Warn(msg string, args ...any) {
	slog.Default().Warn(msg, args...)
}

// Error 输出 Error 级别日志
func Error(msg string, args ...any) {
	slog.Default().Error(msg, args...)
}

// DebugContext 带 context 的 Debug 日志
func DebugContext(ctx context.Context, msg string, args ...any) {
	slog.Default().DebugContext(ctx, msg, args...)
}

// InfoContext 带 context 的 Info 日志
func InfoContext(ctx context.Context, msg string, args ...any) {
	slog.Default().InfoContext(ctx, msg, args...)
}

// WarnContext 带 context 的 Warn 日志
func WarnContext(ctx context.Context, msg string, args ...any) {
	slog.Default().WarnContext(ctx, msg, args...)
}

// ErrorContext 带 context 的 Error 日志
func ErrorContext(ctx context.Context, msg string, args ...any) {
	slog.Default().ErrorContext(ctx, msg, args...)
}

// ============================================================================
//                              工具函数
// ============================================================================

// TruncateID 安全截取 ID 用于日志显示
//
// 如果 ID 长度小于等于 maxLen，返回原 ID；
// 否则返回前 maxLen 个字符。
//
// 用于避免在日志中直接使用 id[:8] 导致 slice bounds out of range。
func TruncateID(id string, maxLen int) string {
	if len(id) <= maxLen {
		return id
	}
	return id[:maxLen]
}

// ============================================================================
//                              初始化
// ============================================================================

func init() {
	// 设置默认 logger 为带时间戳的文本格式
	opts := &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}
	defaultLogger = slog.New(slog.NewTextHandler(os.Stderr, opts))
}
