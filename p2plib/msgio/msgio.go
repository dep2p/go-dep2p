package msgio

import (
	"errors"
	"io"
	"sync"

	pool "github.com/dep2p/go-dep2p/p2plib/buffer/pool"
)

// ErrMsgTooLarge 当消息长度过大时返回此错误
var ErrMsgTooLarge = errors.New("message too large")

const (
	lengthSize     = 4               // 消息长度字段的字节数
	defaultMaxSize = 8 * 1024 * 1024 // 默认最大消息大小为8MB
)

// Writer 是 msgio Writer 接口,用于写入带长度前缀的消息
type Writer interface {
	// Write 将传入的缓冲区作为单个消息写入
	// 参数:
	//   - []byte: 要写入的字节切片
	//
	// 返回值:
	//   - int: 写入的字节数
	//   - error: 错误信息
	Write([]byte) (int, error)

	// WriteMsg 将传入缓冲区中的消息写入
	// 参数:
	//   - []byte: 要写入的消息字节切片
	//
	// 返回值:
	//   - error: 错误信息
	WriteMsg([]byte) error
}

// WriteCloser 是 Writer + Closer 接口,类似于 golang/pkg/io
type WriteCloser interface {
	Writer
	io.Closer
}

// Reader 是 msgio Reader 接口,用于读取带长度前缀的消息
type Reader interface {
	// Read 从 Reader 读取下一条消息
	// 客户端必须传入足够大的缓冲区,否则将返回 io.ErrShortBuffer
	// 参数:
	//   - []byte: 用于存储消息的缓冲区
	//
	// 返回值:
	//   - int: 读取的字节数
	//   - error: 错误信息
	Read([]byte) (int, error)

	// ReadMsg 从 Reader 读取下一条消息
	// 内部使用 pool.BufferPool 重用缓冲区
	// 用户可以调用 ReleaseMsg(msg) 来表示缓冲区可以被重用
	// 返回值:
	//   - []byte: 读取的消息内容
	//   - error: 错误信息
	ReadMsg() ([]byte, error)

	// ReleaseMsg 表示缓冲区可以被重用
	// 参数:
	//   - []byte: 要释放的消息缓冲区
	ReleaseMsg([]byte)

	// NextMsgLen 返回下一条(预览的)消息的长度
	// 不会破坏消息或产生其他副作用
	// 返回值:
	//   - int: 下一条消息的长度
	//   - error: 错误信息
	NextMsgLen() (int, error)
}

// ReadCloser 组合了 Reader 和 Closer 接口
type ReadCloser interface {
	Reader
	io.Closer
}

// ReadWriter 组合了 Reader 和 Writer 接口
type ReadWriter interface {
	Reader
	Writer
}

// ReadWriteCloser 组合了 Reader、Writer 和 Closer 接口
type ReadWriteCloser interface {
	Reader
	Writer
	io.Closer
}

// writer 是实现 Writer 接口的底层类型
type writer struct {
	W    io.Writer        // 底层写入器
	pool *pool.BufferPool // 缓冲池
	lock sync.Mutex       // 互斥锁
}

// NewWriter 使用 msgio 帧写入器包装一个 io.Writer
// msgio.Writer 将为每个写入的消息写入长度前缀
// 参数:
//   - w: 底层的 io.Writer
//
// 返回值:
//   - WriteCloser: 包装后的写入器
func NewWriter(w io.Writer) WriteCloser {
	return NewWriterWithPool(w, pool.GlobalPool)
}

// NewWriterWithPool 与 NewWriter 相同,但允许用户传入自定义缓冲池
// 参数:
//   - w: 底层的 io.Writer
//   - p: 自定义的缓冲池
//
// 返回值:
//   - WriteCloser: 包装后的写入器
func NewWriterWithPool(w io.Writer, p *pool.BufferPool) WriteCloser {
	return &writer{W: w, pool: p}
}

// Write 实现 Writer 接口的写入方法
// 参数:
//   - msg: 要写入的消息
//
// 返回值:
//   - int: 写入的字节数
//   - error: 错误信息
func (s *writer) Write(msg []byte) (int, error) {
	err := s.WriteMsg(msg)
	if err != nil {
		return 0, err
	}
	return len(msg), nil
}

// WriteMsg 实现消息的写入
// 参数:
//   - msg: 要写入的消息
//
// 返回值:
//   - error: 错误信息
func (s *writer) WriteMsg(msg []byte) (err error) {
	s.lock.Lock()
	defer s.lock.Unlock()

	// 从缓冲池获取缓冲区
	buf := s.pool.Get(len(msg) + lengthSize)
	// 写入消息长度
	NBO.PutUint32(buf, uint32(len(msg)))
	// 复制消息内容
	copy(buf[lengthSize:], msg)
	// 写入底层写入器
	_, err = s.W.Write(buf)
	// 归还缓冲区到缓冲池
	s.pool.Put(buf)

	return err
}

// Close 实现 Closer 接口
// 返回值:
//   - error: 错误信息
func (s *writer) Close() error {
	if c, ok := s.W.(io.Closer); ok {
		return c.Close()
	}
	return nil
}

// reader 是实现 Reader 接口的底层类型
type reader struct {
	R    io.Reader        // 底层读取器
	lbuf [lengthSize]byte // 长度缓冲区
	next int              // 下一条消息的长度
	pool *pool.BufferPool // 缓冲池
	lock sync.Mutex       // 互斥锁
	max  int              // 此读取器处理的最大消息大小(字节)
}

// NewReader 使用 msgio 帧读取器包装一个 io.Reader
// msgio.Reader 将一次读取完整的消息(使用长度)
// 假设另一端有等效的写入器
// 参数:
//   - r: 底层的 io.Reader
//
// 返回值:
//   - ReadCloser: 包装后的读取器
func NewReader(r io.Reader) ReadCloser {
	return NewReaderWithPool(r, pool.GlobalPool)
}

// NewReaderSize 与 NewReader 等效,但允许指定最大消息大小
// 参数:
//   - r: 底层的 io.Reader
//   - maxMessageSize: 最大消息大小
//
// 返回值:
//   - ReadCloser: 包装后的读取器
func NewReaderSize(r io.Reader, maxMessageSize int) ReadCloser {
	return NewReaderSizeWithPool(r, maxMessageSize, pool.GlobalPool)
}

// NewReaderWithPool 与 NewReader 相同,但允许指定缓冲池
// 参数:
//   - r: 底层的 io.Reader
//   - p: 自定义的缓冲池
//
// 返回值:
//   - ReadCloser: 包装后的读取器
func NewReaderWithPool(r io.Reader, p *pool.BufferPool) ReadCloser {
	return NewReaderSizeWithPool(r, defaultMaxSize, p)
}

// NewReaderSizeWithPool 与 NewReader 相同,但允许指定缓冲池和最大消息大小
// 参数:
//   - r: 底层的 io.Reader
//   - maxMessageSize: 最大消息大小
//   - p: 自定义的缓冲池
//
// 返回值:
//   - ReadCloser: 包装后的读取器
func NewReaderSizeWithPool(r io.Reader, maxMessageSize int, p *pool.BufferPool) ReadCloser {
	if p == nil {
		panic("nil pool")
	}
	return &reader{
		R:    r,
		next: -1,
		pool: p,
		max:  maxMessageSize,
	}
}

// NextMsgLen 将下一条消息的长度读入 s.lbuf 并返回
// 警告:与 Read 一样,NextMsgLen 是破坏性的,它从内部读取器读取
// 返回值:
//   - int: 下一条消息的长度
//   - error: 错误信息
func (s *reader) NextMsgLen() (int, error) {
	s.lock.Lock()
	defer s.lock.Unlock()
	return s.nextMsgLen()
}

// nextMsgLen 是 NextMsgLen 的内部实现
// 返回值:
//   - int: 下一条消息的长度
//   - error: 错误信息
func (s *reader) nextMsgLen() (int, error) {
	if s.next == -1 {
		n, err := ReadLen(s.R, s.lbuf[:])
		if err != nil {
			return 0, err
		}

		s.next = n
	}
	return s.next, nil
}

// Read 实现 Reader 接口的读取方法
// 参数:
//   - msg: 用于存储消息的缓冲区
//
// 返回值:
//   - int: 读取的字节数
//   - error: 错误信息
func (s *reader) Read(msg []byte) (int, error) {
	s.lock.Lock()
	defer s.lock.Unlock()

	length, err := s.nextMsgLen()
	if err != nil {
		return 0, err
	}

	if length > len(msg) {
		return 0, io.ErrShortBuffer
	}

	read, err := io.ReadFull(s.R, msg[:length])
	if read < length {
		s.next = length - read // 我们只部分消费了消息
	} else {
		s.next = -1 // 表示我们已消费此消息
	}
	return read, err
}

// ReadMsg 实现消息的读取
// 返回值:
//   - []byte: 读取的消息内容
//   - error: 错误信息
func (s *reader) ReadMsg() ([]byte, error) {
	s.lock.Lock()
	defer s.lock.Unlock()

	length, err := s.nextMsgLen()
	if err != nil {
		return nil, err
	}

	if length == 0 {
		s.next = -1
		return nil, nil
	}

	if length > s.max || length < 0 {
		return nil, ErrMsgTooLarge
	}

	msg := s.pool.Get(length)
	read, err := io.ReadFull(s.R, msg)
	if read < length {
		s.next = length - read // 我们只部分消费了消息
	} else {
		s.next = -1 // 表示我们已消费此消息
	}
	return msg[:read], err
}

// ReleaseMsg 将消息缓冲区归还到缓冲池
// 参数:
//   - msg: 要释放的消息缓冲区
func (s *reader) ReleaseMsg(msg []byte) {
	s.pool.Put(msg)
}

// Close 实现 Closer 接口
// 返回值:
//   - error: 错误信息
func (s *reader) Close() error {
	if c, ok := s.R.(io.Closer); ok {
		return c.Close()
	}
	return nil
}

// readWriter 是实现 ReadWriter 的底层类型
type readWriter struct {
	Reader
	Writer
}

// NewReadWriter 使用 msgio.ReadWriter 包装一个 io.ReadWriter
// 写入和读取将被适当地加上帧
// 参数:
//   - rw: 底层的 io.ReadWriter
//
// 返回值:
//   - ReadWriteCloser: 包装后的读写器
func NewReadWriter(rw io.ReadWriter) ReadWriteCloser {
	return &readWriter{
		Reader: NewReader(rw),
		Writer: NewWriter(rw),
	}
}

// Combine 将一对 msgio.Writer 和 msgio.Reader 包装成 msgio.ReadWriter
// 参数:
//   - w: Writer 接口实现
//   - r: Reader 接口实现
//
// 返回值:
//   - ReadWriteCloser: 组合后的读写器
func Combine(w Writer, r Reader) ReadWriteCloser {
	return &readWriter{Reader: r, Writer: w}
}

// Close 实现 Closer 接口
// 返回值:
//   - error: 错误信息
func (rw *readWriter) Close() error {
	var errs []error

	if w, ok := rw.Writer.(WriteCloser); ok {
		if err := w.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if r, ok := rw.Reader.(ReadCloser); ok {
		if err := r.Close(); err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return multiErr(errs)
	}
	return nil
}

// multiErr 是一个返回多个错误的工具类型
type multiErr []error

// Error 实现 error 接口
// 返回值:
//   - string: 错误信息字符串
func (m multiErr) Error() string {
	if len(m) == 0 {
		return "no errors"
	}

	s := "Multiple errors: "
	for i, e := range m {
		if i != 0 {
			s += ", "
		}
		s += e.Error()
	}
	return s
}
