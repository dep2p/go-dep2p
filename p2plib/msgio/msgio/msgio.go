package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	msgio "github.com/dep2p/go-dep2p/p2plib/msgio"
)

// Args 全局参数对象
var Args ArgType

// ArgType 命令行参数类型
type ArgType struct {
	Command string   // 命令名称
	Args    []string // 命令参数列表
}

// Arg 获取指定索引的参数
// 参数:
//   - i: 参数索引
//
// 返回值:
//   - string: 参数值
func (a *ArgType) Arg(i int) string {
	n := i + 1
	if len(a.Args) < n {
		die(fmt.Sprintf("expected %d argument(s)", n))
	}
	return a.Args[i]
}

// usageStr 帮助信息字符串
var usageStr = `
msgio - tool to wrap messages with msgio header

Usage
    msgio header 1020 >header
    cat file | msgio wrap >wrapped

Commands
    header <size>   output a msgio header of given size
    wrap            wrap incoming stream with msgio
`

// usage 打印帮助信息并退出程序
func usage() {
	fmt.Println(strings.TrimSpace(usageStr))
	os.Exit(0)
}

// die 打印错误信息并以错误状态退出程序
// 参数:
//   - err: 错误信息
func die(err string) {
	fmt.Fprintf(os.Stderr, "error: %s\n", err)
	os.Exit(-1)
}

// main 程序入口函数
func main() {
	if err := run(); err != nil {
		die(err.Error())
	}
}

// argParse 解析命令行参数
func argParse() {
	flag.Usage = usage
	flag.Parse()

	args := flag.Args()
	if l := len(args); l < 1 || l > 2 {
		usage()
	}

	Args.Command = flag.Args()[0]
	Args.Args = flag.Args()[1:]
}

// run 执行主程序逻辑
// 返回值:
//   - error: 错误信息
func run() error {
	argParse()

	w := os.Stdout
	r := os.Stdin

	switch Args.Command {
	case "header":
		size, err := strconv.Atoi(Args.Arg(0))
		if err != nil {
			return err
		}
		return header(w, size)
	case "wrap":
		return wrap(w, r)
	default:
		usage()
		return nil
	}
}

// header 写入指定大小的消息头
// 参数:
//   - w: 写入器
//   - size: 消息大小
//
// 返回值:
//   - error: 错误信息
func header(w io.Writer, size int) error {
	return msgio.WriteLen(w, size)
}

// wrap 将输入流包装成带有消息头的格式
// 参数:
//   - w: 写入器
//   - r: 读取器
//
// 返回值:
//   - error: 错误信息
func wrap(w io.Writer, r io.Reader) error {
	buf, err := io.ReadAll(r)
	if err != nil {
		return err
	}

	if err := msgio.WriteLen(w, len(buf)); err != nil {
		return err
	}

	_, err = w.Write(buf)
	return err
}
