package msgio

import (
	"bytes"
	"io"
	"sync"
)

// LimitedReader 使用 msgio 帧读取器包装一个 io.Reader。
// 当消息长度读取完成时，LimitedReader 将返回一个带有 io.EOF 的读取器。
// 参数:
//   - r: 输入的 io.Reader
//
// 返回值:
//   - io.Reader: 限制长度的读取器
//   - error: 错误信息
func LimitedReader(r io.Reader) (io.Reader, error) {
	// 读取消息长度
	l, err := ReadLen(r, nil)
	// 返回限制长度的读取器
	return io.LimitReader(r, int64(l)), err
}

// NewLimitedWriter 使用 msgio 帧写入器包装一个 io.Writer。
// 它是 LimitedReader 的反向操作:它会缓冲所有写入直到调用"Flush"。
// 当调用 Flush 时，它会先写入缓冲区大小，然后刷新缓冲区，重置缓冲区，并开始接受更多的传入写入。
// 参数:
//   - w: 输出的 io.Writer
//
// 返回值:
//   - *LimitedWriter: 限制长度的写入器
func NewLimitedWriter(w io.Writer) *LimitedWriter {
	return &LimitedWriter{W: w}
}

// LimitedWriter 实现了一个带缓冲的限制长度写入器
type LimitedWriter struct {
	W io.Writer    // 底层写入器
	B bytes.Buffer // 缓冲区
	M sync.Mutex   // 互斥锁
}

// Write 实现了 io.Writer 接口
// 参数:
//   - buf: 要写入的字节切片
//
// 返回值:
//   - n: 写入的字节数
//   - err: 错误信息
func (w *LimitedWriter) Write(buf []byte) (n int, err error) {
	// 加锁保护并发访问
	w.M.Lock()
	// 写入缓冲区
	n, err = w.B.Write(buf)
	// 解锁
	w.M.Unlock()
	return n, err
}

// Flush 将缓冲区的数据写入底层写入器
// 返回值:
//   - error: 错误信息
func (w *LimitedWriter) Flush() error {
	// 加锁保护并发访问
	w.M.Lock()
	defer w.M.Unlock()
	// 写入消息长度
	if err := WriteLen(w.W, w.B.Len()); err != nil {
		return err
	}
	// 写入消息内容
	_, err := w.B.WriteTo(w.W)
	return err
}
