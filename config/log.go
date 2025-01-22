package config

import (
	"strings"
	"sync"

	logging "github.com/dep2p/log"
	"go.uber.org/fx/fxevent"
)

// log 是用于 p2p 配置的日志记录器
var log = logging.Logger("config")

var (
	// fxLogger 是 fx 框架的日志记录器
	fxLogger fxevent.Logger
	// logInitOnce 确保日志记录器只初始化一次
	logInitOnce sync.Once
)

// fxLogWriter 实现了 io.Writer 接口,用于将 fx 日志写入到 p2p 日志记录器
type fxLogWriter struct{}

// Write 将日志内容写入到 p2p 日志记录器
// 参数:
//   - b: 要写入的字节切片
//
// 返回:
//   - int: 写入的字节数
//   - error: 写入过程中的错误,如果成功则返回 nil
func (l *fxLogWriter) Write(b []byte) (int, error) {
	// 移除末尾的换行符并以 Debug 级别记录日志
	log.Debug(strings.TrimSuffix(string(b), "\n"))
	return len(b), nil
}

// getFXLogger 返回 fx 框架使用的日志记录器
// 返回:
//   - fxevent.Logger: fx 日志记录器实例
func getFXLogger() fxevent.Logger {
	// 确保日志记录器只初始化一次
	logInitOnce.Do(func() { fxLogger = &fxevent.ConsoleLogger{W: &fxLogWriter{}} })
	return fxLogger
}
