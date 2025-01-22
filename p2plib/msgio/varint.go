package msgio

import (
	"encoding/binary"
	"io"
	"sync"

	pool "github.com/dep2p/go-dep2p/p2plib/buffer/pool"
	"github.com/multiformats/go-varint"
)

// varintWriter 是实现 Writer 接口的底层类型
type varintWriter struct {
	W    io.Writer        // 底层写入器
	pool *pool.BufferPool // 缓冲池
	lock sync.Mutex       // 用于线程安全写入的互斥锁
}

// NewVarintWriter 使用 varint msgio 帧写入器包装一个 io.Writer
// 写入器将使用 https://golang.org/pkg/encoding/binary/#PutUvarint 将每个消息的长度前缀写为 varint
// 参数:
//   - w: 底层的 io.Writer
//
// 返回值:
//   - WriteCloser: 包装后的写入器
func NewVarintWriter(w io.Writer) WriteCloser {
	return NewVarintWriterWithPool(w, pool.GlobalPool)
}

// NewVarintWriterWithPool 与 NewVarintWriter 相同，但允许指定缓冲池
// 参数:
//   - w: 底层的 io.Writer
//   - p: 缓冲池
//
// 返回值:
//   - WriteCloser: 包装后的写入器
func NewVarintWriterWithPool(w io.Writer, p *pool.BufferPool) WriteCloser {
	return &varintWriter{
		pool: p,
		W:    w,
	}
}

// Write 实现 io.Writer 接口
// 参数:
//   - msg: 要写入的消息
//
// 返回值:
//   - int: 写入的字节数
//   - error: 错误信息
func (s *varintWriter) Write(msg []byte) (int, error) {
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
func (s *varintWriter) WriteMsg(msg []byte) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	// 从缓冲池获取缓冲区
	buf := s.pool.Get(len(msg) + binary.MaxVarintLen64)
	// 写入长度前缀
	n := binary.PutUvarint(buf, uint64(len(msg)))
	// 写入消息内容
	n += copy(buf[n:], msg)
	// 写入底层写入器
	_, err := s.W.Write(buf[:n])
	// 归还缓冲区
	s.pool.Put(buf)

	return err
}

// Close 实现 Closer 接口
// 返回值:
//   - error: 错误信息
func (s *varintWriter) Close() error {
	if c, ok := s.W.(io.Closer); ok {
		return c.Close()
	}
	return nil
}

// varintReader 是实现 Reader 接口的底层类型
type varintReader struct {
	R    io.Reader        // 底层读取器
	br   io.ByteReader    // 用于读取 varint
	next int              // 下一条消息的长度
	pool *pool.BufferPool // 缓冲池
	lock sync.Mutex       // 互斥锁
	max  int              // 此读取器处理的最大消息大小(字节)
}

// NewVarintReader 使用 varint msgio 帧读取器包装一个 io.Reader
// 读取器将一次读取完整的消息(使用长度)
// Varint 根据 https://golang.org/pkg/encoding/binary/#ReadUvarint 读取
// 假设另一端有等效的写入器
// 参数:
//   - r: 底层的 io.Reader
//
// 返回值:
//   - ReadCloser: 包装后的读取器
func NewVarintReader(r io.Reader) ReadCloser {
	return NewVarintReaderSize(r, defaultMaxSize)
}

// NewVarintReaderSize 等同于 NewVarintReader，但允许指定最大消息大小
// 参数:
//   - r: 底层的 io.Reader
//   - maxMessageSize: 最大消息大小
//
// 返回值:
//   - ReadCloser: 包装后的读取器
func NewVarintReaderSize(r io.Reader, maxMessageSize int) ReadCloser {
	return NewVarintReaderSizeWithPool(r, maxMessageSize, pool.GlobalPool)
}

// NewVarintReaderWithPool 与 NewVarintReader 相同，但允许指定缓冲池
// 参数:
//   - r: 底层的 io.Reader
//   - p: 缓冲池
//
// 返回值:
//   - ReadCloser: 包装后的读取器
func NewVarintReaderWithPool(r io.Reader, p *pool.BufferPool) ReadCloser {
	return NewVarintReaderSizeWithPool(r, defaultMaxSize, p)
}

// NewVarintReaderSizeWithPool 与 NewVarintReader 相同，但允许指定缓冲池和最大消息大小
// 参数:
//   - r: 底层的 io.Reader
//   - maxMessageSize: 最大消息大小
//   - p: 缓冲池
//
// 返回值:
//   - ReadCloser: 包装后的读取器
func NewVarintReaderSizeWithPool(r io.Reader, maxMessageSize int, p *pool.BufferPool) ReadCloser {
	if p == nil {
		panic("nil pool")
	}
	return &varintReader{
		R:    r,
		br:   &simpleByteReader{R: r},
		next: -1,
		pool: p,
		max:  maxMessageSize,
	}
}

// NextMsgLen 读取下一条消息的长度
// 警告: 与 Read 一样，NextMsgLen 是破坏性的。它从内部读取器读取
// 返回值:
//   - int: 下一条消息的长度
//   - error: 错误信息
func (s *varintReader) NextMsgLen() (int, error) {
	s.lock.Lock()
	defer s.lock.Unlock()
	return s.nextMsgLen()
}

// nextMsgLen 是 NextMsgLen 的内部实现
// 返回值:
//   - int: 下一条消息的长度
//   - error: 错误信息
func (s *varintReader) nextMsgLen() (int, error) {
	if s.next == -1 {
		length, err := varint.ReadUvarint(s.br)
		if err != nil {
			return 0, err
		}
		s.next = int(length)
	}
	return s.next, nil
}

// Read 实现 io.Reader 接口
// 参数:
//   - msg: 用于存储消息的缓冲区
//
// 返回值:
//   - int: 读取的字节数
//   - error: 错误信息
func (s *varintReader) Read(msg []byte) (int, error) {
	s.lock.Lock()
	defer s.lock.Unlock()

	length, err := s.nextMsgLen()
	if err != nil {
		return 0, err
	}

	if length > len(msg) {
		return 0, io.ErrShortBuffer
	}
	_, err = io.ReadFull(s.R, msg[:length])
	s.next = -1 // 表示我们已消费此消息
	return length, err
}

// ReadMsg 实现消息的读取
// 返回值:
//   - []byte: 读取的消息内容
//   - error: 错误信息
func (s *varintReader) ReadMsg() ([]byte, error) {
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

	if length > s.max {
		return nil, ErrMsgTooLarge
	}

	msg := s.pool.Get(length)
	_, err = io.ReadFull(s.R, msg)
	s.next = -1 // 表示我们已消费此消息
	return msg, err
}

// ReleaseMsg 将消息缓冲区归还到缓冲池
// 参数:
//   - msg: 要释放的消息缓冲区
func (s *varintReader) ReleaseMsg(msg []byte) {
	s.pool.Put(msg)
}

// Close 实现 Closer 接口
// 返回值:
//   - error: 错误信息
func (s *varintReader) Close() error {
	if c, ok := s.R.(io.Closer); ok {
		return c.Close()
	}
	return nil
}

// simpleByteReader 实现了一个简单的字节读取器
type simpleByteReader struct {
	R   io.Reader // 底层读取器
	buf [1]byte   // 单字节缓冲区
}

// ReadByte 实现 io.ByteReader 接口
// 返回值:
//   - byte: 读取的字节
//   - error: 错误信息
func (r *simpleByteReader) ReadByte() (c byte, err error) {
	if _, err := io.ReadFull(r.R, r.buf[:]); err != nil {
		return 0, err
	}
	return r.buf[0], nil
}
