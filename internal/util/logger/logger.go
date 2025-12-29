// Package logger 提供 dep2p 的统一日志系统
//
// 基于标准库 log/slog，支持：
//   - 按子系统配置日志级别
//   - 环境变量配置（DEP2P_LOG_LEVEL, DEP2P_LOG_FORMAT）
//   - 结构化日志
//
// 使用示例:
//
//	package discovery
//
//	import "github.com/dep2p/go-dep2p/internal/util/logger"
//
//	var log = logger.Logger("discovery")
//
//	func foo() {
//	    log.Info("peer discovered", "peer", peerID, "count", len(peers))
//	    log.Debug("connection details", "addr", addr, "latency", latency)
//	    log.Error("connection failed", "err", err, "peer", peerID)
//	}
//
// 环境变量配置:
//
//	# 设置所有模块为 info，discovery 模块为 debug
//	DEP2P_LOG_LEVEL=discovery=debug,info
//
//	# 使用 JSON 格式输出
//	DEP2P_LOG_FORMAT=json
package logger

import (
	"io"
	"log/slog"
	"sync"
)

var (
	// loggers 缓存各子系统的 Logger
	loggers sync.Map // map[string]*slog.Logger

	// handlers 缓存各子系统的 Handler（用于动态调整级别）
	handlers sync.Map // map[string]*subsystemHandler

	// globalLogger 全局默认 Logger
	globalLogger     *slog.Logger
	globalLoggerOnce sync.Once
)

// Logger 获取指定子系统的 Logger
//
// Logger 会根据 DEP2P_LOG_LEVEL 环境变量配置日志级别。
// 同一子系统多次调用会返回相同的 Logger 实例。
//
// 示例:
//
//	var log = logger.Logger("discovery")
//	log.Info("peer found", "peer", peerID)
func Logger(subsystem string) *slog.Logger {
	// 尝试从缓存获取
	if l, ok := loggers.Load(subsystem); ok {
		return l.(*slog.Logger)
	}

	// 创建新 Logger
	cfg := ConfigFromEnv()
	level := cfg.LevelForSubsystem(subsystem)

	handler := newHandler(subsystem, level, cfg.Format)
	logger := slog.New(handler)

	// 缓存
	actual, _ := loggers.LoadOrStore(subsystem, logger)
	if h, ok := handler.(*subsystemHandler); ok {
		handlers.Store(subsystem, h)
	}

	return actual.(*slog.Logger)
}

// GlobalLogger 返回全局 Logger
//
// 用于不属于特定子系统的日志，或作为 fx 注入的默认 Logger。
func GlobalLogger() *slog.Logger {
	globalLoggerOnce.Do(func() {
		globalLogger = Logger("dep2p")
	})
	return globalLogger
}

// SetLevel 动态设置子系统的日志级别
//
// 这允许在运行时调整日志级别，无需重启。
//
// 示例:
//
//	logger.SetLevel("discovery", slog.LevelDebug)
func SetLevel(subsystem string, level slog.Level) {
	if h, ok := handlers.Load(subsystem); ok {
		h.(*subsystemHandler).SetLevel(level)
	}
}

// SetGlobalLevel 设置所有子系统的默认日志级别
func SetGlobalLevel(level slog.Level) {
	handlers.Range(func(_, value any) bool {
		value.(*subsystemHandler).SetLevel(level)
		return true
	})
}

// Discard 返回一个丢弃所有日志的 Logger
//
// 主要用于测试，避免日志输出干扰测试结果。
//
// 示例:
//
//	func TestFoo(t *testing.T) {
//	    log := logger.Discard()
//	    // ...
//	}
func Discard() *slog.Logger {
	return slog.New(DiscardHandler())
}

// With 创建带有预设属性的 Logger
//
// 示例:
//
//	log := logger.Logger("discovery").With("peer", peerID)
//	log.Info("connected")  // 自动包含 peer 属性
func With(subsystem string, args ...any) *slog.Logger {
	return Logger(subsystem).With(args...)
}

// Debug 记录 Debug 级别日志
func Debug(subsystem, msg string, args ...any) {
	Logger(subsystem).Debug(msg, args...)
}

// Info 记录 Info 级别日志
func Info(subsystem, msg string, args ...any) {
	Logger(subsystem).Info(msg, args...)
}

// Warn 记录 Warn 级别日志
func Warn(subsystem, msg string, args ...any) {
	Logger(subsystem).Warn(msg, args...)
}

// Error 记录 Error 级别日志
func Error(subsystem, msg string, args ...any) {
	Logger(subsystem).Error(msg, args...)
}

// SetOutput 设置全局日志输出目标
//
// 必须在创建任何 Logger 之前调用，否则已创建的 Logger 不受影响。
// 建议在程序启动早期调用。
//
// 示例:
//
//	file, _ := os.OpenFile("app.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
//	logger.SetOutput(file)
func SetOutput(w io.Writer) {
	globalOutputMu.Lock()
	globalOutput = w
	globalOutputMu.Unlock()
	
	// 注意：由于使用了 dynamicWriter，无需清空已缓存的 loggers
	// 所有 logger 的输出会自动重定向到新的 writer
}

