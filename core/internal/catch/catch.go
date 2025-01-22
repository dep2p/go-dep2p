package catch

import (
	"fmt"
	"io"
	"os"
	"runtime/debug"
)

// panicWriter 定义了输出panic信息的目标writer,默认为标准错误输出
var panicWriter io.Writer = os.Stderr

// HandlePanic 处理并记录panic信息
// 参数:
//   - rerr: panic时的错误信息
//   - err: 用于存储格式化后的错误信息的指针
//   - where: 发生panic的位置描述
func HandlePanic(rerr interface{}, err *error, where string) {
	// 如果存在panic错误信息
	if rerr != nil {
		// 将panic的错误信息和调用栈打印到panicWriter
		fmt.Fprintf(panicWriter, "caught panic: %s\n%s\n", rerr, debug.Stack())
		// 将panic信息格式化后存储到err指针
		*err = fmt.Errorf("panic in %s: %s", where, rerr)
	}
}
