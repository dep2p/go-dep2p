package dep2pwebrtc

import (
	logging "github.com/dep2p/log"
	pionLogging "github.com/pion/logging"
)

// WebRTC传输层的日志记录器
var log = logging.Logger("p2p-transport-webrtc")

// pionLog 是提供给pion用于内部日志记录的记录器
var pionLog = logging.Logger("p2p-transport-webrtc-pion")

// pionLogger 包装了StandardLogger接口以提供pion所需的LeveledLogger接口
// Pion的日志过于冗长且日志级别不合理,pionLogger将所有日志降级为debug级别
type pionLogger struct {
	logging.StandardLogger
}

// pLog 是pionLogger的实例
var pLog = pionLogger{pionLog}

// 确保pionLogger实现了pionLogging.LeveledLogger接口
var _ pionLogging.LeveledLogger = pLog

// Debug 记录debug级别的日志
// 参数:
//   - s: string 日志内容
func (l pionLogger) Debug(s string) {
	l.StandardLogger.Debug(s)
}

// Error 记录error级别的日志(降级为debug)
// 参数:
//   - s: string 日志内容
func (l pionLogger) Error(s string) {
	l.StandardLogger.Debug(s)
}

// Errorf 记录格式化的error级别日志(降级为debug)
// 参数:
//   - s: string 日志格式
//   - args: ...interface{} 格式化参数
func (l pionLogger) Errorf(s string, args ...interface{}) {
	l.StandardLogger.Debugf(s, args...)
}

// Info 记录info级别的日志(降级为debug)
// 参数:
//   - s: string 日志内容
func (l pionLogger) Info(s string) {
	l.StandardLogger.Debug(s)
}

// Infof 记录格式化的info级别日志(降级为debug)
// 参数:
//   - s: string 日志格式
//   - args: ...interface{} 格式化参数
func (l pionLogger) Infof(s string, args ...interface{}) {
	l.StandardLogger.Debugf(s, args...)
}

// Warn 记录warn级别的日志(降级为debug)
// 参数:
//   - s: string 日志内容
func (l pionLogger) Warn(s string) {
	l.StandardLogger.Debug(s)
}

// Warnf 记录格式化的warn级别日志(降级为debug)
// 参数:
//   - s: string 日志格式
//   - args: ...interface{} 格式化参数
func (l pionLogger) Warnf(s string, args ...interface{}) {
	l.StandardLogger.Debugf(s, args...)
}

// Trace 记录trace级别的日志(降级为debug)
// 参数:
//   - s: string 日志内容
func (l pionLogger) Trace(s string) {
	l.StandardLogger.Debug(s)
}

// Tracef 记录格式化的trace级别日志(降级为debug)
// 参数:
//   - s: string 日志格式
//   - args: ...interface{} 格式化参数
func (l pionLogger) Tracef(s string, args ...interface{}) {
	l.StandardLogger.Debugf(s, args...)
}

// loggerFactory 为所有新的日志实例返回pLog
type loggerFactory struct{}

// NewLogger 为所有新的日志实例返回pLog
// Pion内部会不必要地创建大量独立的日志对象
// 为避免内存分配,我们对所有pion日志使用单个日志对象
// 参数:
//   - scope: string 日志作用域
//
// 返回值:
//   - pionLogging.LeveledLogger pion日志接口实现
func (loggerFactory) NewLogger(scope string) pionLogging.LeveledLogger {
	return pLog
}

// 确保loggerFactory实现了pionLogging.LoggerFactory接口
var _ pionLogging.LoggerFactory = loggerFactory{}

// pionLoggerFactory 是loggerFactory的实例
var pionLoggerFactory = loggerFactory{}
