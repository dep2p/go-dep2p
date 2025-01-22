package mocknet

import (
	"bytes"
	"errors"
	"io"
	"net"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/dep2p/go-dep2p/core/network"
	"github.com/dep2p/go-dep2p/core/protocol"
)

// 流ID计数器
var streamCounter atomic.Int64

// stream 实现了 network.Stream 接口
type stream struct {
	rstream *stream // 远程流
	conn    *conn   // 连接对象
	id      int64   // 流ID

	write     *io.PipeWriter        // 写管道
	read      *io.PipeReader        // 读管道
	toDeliver chan *transportObject // 待传输对象通道

	reset  chan struct{} // 重置信号通道
	close  chan struct{} // 关闭信号通道
	closed chan struct{} // 已关闭信号通道

	writeErr error // 写入错误

	protocol atomic.Pointer[protocol.ID] // 协议ID
	stat     network.Stats               // 流统计信息
}

// 流关闭错误
var ErrClosed = errors.New("流已关闭")

// transportObject 表示待传输的对象
type transportObject struct {
	msg         []byte    // 消息内容
	arrivalTime time.Time // 到达时间
}

// newStreamPair 创建一对相互连接的流
// 返回值:
//   - *stream: 第一个流
//   - *stream: 第二个流
func newStreamPair() (*stream, *stream) {
	// 创建两对管道
	ra, wb := io.Pipe()
	rb, wa := io.Pipe()

	// 创建两个流对象
	sa := newStream(wa, ra, network.DirOutbound)
	sb := newStream(wb, rb, network.DirInbound)
	// 设置互相引用
	sa.rstream = sb
	sb.rstream = sa
	return sa, sb
}

// newStream 创建新的流对象
// 参数:
//   - w: *io.PipeWriter 写管道
//   - r: *io.PipeReader 读管道
//   - dir: network.Direction 流方向
//
// 返回值:
//   - *stream: 新创建的流对象
func newStream(w *io.PipeWriter, r *io.PipeReader, dir network.Direction) *stream {
	s := &stream{
		read:      r,
		write:     w,
		id:        streamCounter.Add(1),
		reset:     make(chan struct{}, 1),
		close:     make(chan struct{}, 1),
		closed:    make(chan struct{}),
		toDeliver: make(chan *transportObject),
		stat:      network.Stats{Direction: dir},
	}

	// 启动传输协程
	go s.transport()
	return s
}

// Write 写入数据到流
// 参数:
//   - p: []byte 要写入的数据
//
// 返回值:
//   - n: int 写入的字节数
//   - err: error 错误信息
func (s *stream) Write(p []byte) (n int, err error) {
	l := s.conn.link
	// 计算延迟时间
	delay := l.GetLatency() + l.RateLimit(len(p))
	t := time.Now().Add(delay)

	// 复制数据
	cpy := make([]byte, len(p))
	copy(cpy, p)

	select {
	case <-s.closed: // 如果流已关闭则退出
		log.Debugf("流已关闭")
		return 0, s.writeErr
	case s.toDeliver <- &transportObject{msg: cpy, arrivalTime: t}:
	}
	return len(p), nil
}

// ID 获取流ID
// 返回值:
//   - string: 流ID字符串
func (s *stream) ID() string {
	return strconv.FormatInt(s.id, 10)
}

// Protocol 获取流协议
// 返回值:
//   - protocol.ID: 协议ID
func (s *stream) Protocol() protocol.ID {
	p := s.protocol.Load()
	if p == nil {
		return ""
	}
	return *p
}

// Stat 获取流统计信息
// 返回值:
//   - network.Stats: 流统计信息
func (s *stream) Stat() network.Stats {
	return s.stat
}

// SetProtocol 设置流协议
// 参数:
//   - proto: protocol.ID 协议ID
//
// 返回值:
//   - error: 错误信息
func (s *stream) SetProtocol(proto protocol.ID) error {
	s.protocol.Store(&proto)
	return nil
}

// CloseWrite 关闭写入端
// 返回值:
//   - error: 错误信息
func (s *stream) CloseWrite() error {
	select {
	case s.close <- struct{}{}:
	default:
	}
	<-s.closed
	if s.writeErr != ErrClosed {
		log.Debugf("写入错误: %v", s.writeErr)
		return s.writeErr
	}
	return nil
}

// CloseRead 关闭读取端
// 返回值:
//   - error: 错误信息
func (s *stream) CloseRead() error {
	return s.read.CloseWithError(ErrClosed)
}

// Close 关闭流
// 返回值:
//   - error: 错误信息
func (s *stream) Close() error {
	_ = s.CloseRead()
	return s.CloseWrite()
}

// Reset 重置流
// 返回值:
//   - error: 错误信息
func (s *stream) Reset() error {
	// 取消所有待处理的读写操作
	s.write.CloseWithError(network.ErrReset)
	s.read.CloseWithError(network.ErrReset)

	select {
	case s.reset <- struct{}{}:
	default:
	}
	<-s.closed

	return nil
}

// teardown 清理流资源
func (s *stream) teardown() {
	// 从连接中移除流
	s.conn.removeStream(s)

	// 标记为已关闭
	close(s.closed)
}

// Conn 获取关联的连接
// 返回值:
//   - network.Conn: 连接对象
func (s *stream) Conn() network.Conn {
	return s.conn
}

// SetDeadline 设置读写超时时间(不支持)
// 参数:
//   - t: time.Time 超时时间
//
// 返回值:
//   - error: 错误信息
func (s *stream) SetDeadline(t time.Time) error {
	return &net.OpError{Op: "set", Net: "pipe", Source: nil, Addr: nil, Err: errors.New("不支持设置超时")}
}

// SetReadDeadline 设置读取超时时间(不支持)
// 参数:
//   - t: time.Time 超时时间
//
// 返回值:
//   - error: 错误信息
func (s *stream) SetReadDeadline(t time.Time) error {
	return &net.OpError{Op: "set", Net: "pipe", Source: nil, Addr: nil, Err: errors.New("不支持设置超时")}
}

// SetWriteDeadline 设置写入超时时间(不支持)
// 参数:
//   - t: time.Time 超时时间
//
// 返回值:
//   - error: 错误信息
func (s *stream) SetWriteDeadline(t time.Time) error {
	return &net.OpError{Op: "set", Net: "pipe", Source: nil, Addr: nil, Err: errors.New("不支持设置超时")}
}

// Read 从流中读取数据
// 参数:
//   - b: []byte 读取缓冲区
//
// 返回值:
//   - int: 读取的字节数
//   - error: 错误信息
func (s *stream) Read(b []byte) (int, error) {
	return s.read.Read(b)
}

// transport 处理消息传输
// 根据消息到达时间调度写入操作
func (s *stream) transport() {
	defer s.teardown()

	bufsize := 256
	buf := new(bytes.Buffer)
	timer := time.NewTimer(0)
	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
		}
	}

	// 清理定时器
	defer timer.Stop()

	// drainBuf 将缓冲区内容写入流
	drainBuf := func() error {
		if buf.Len() > 0 {
			_, err := s.write.Write(buf.Bytes())
			if err != nil {
				log.Debugf("写入错误: %v", err)
				return err
			}
			buf.Reset()
		}
		return nil
	}

	// deliverOrWait 处理传入的数据包
	// 等待到达时间后写入数据
	deliverOrWait := func(o *transportObject) error {
		buffered := len(o.msg) + buf.Len()

		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		delay := time.Until(o.arrivalTime)
		if delay >= 0 {
			timer.Reset(delay)
		} else {
			timer.Reset(0)
		}

		if buffered >= bufsize {
			select {
			case <-timer.C:
			case <-s.reset:
				select {
				case s.reset <- struct{}{}:
				default:
				}
				log.Debugf("重置流")
				return network.ErrReset
			}
			if err := drainBuf(); err != nil {
				log.Debugf("清空缓冲区失败: %v", err)
				return err
			}
			// 写入消息
			_, err := s.write.Write(o.msg)
			if err != nil {
				log.Debugf("写入消息失败: %v", err)
				return err
			}
		} else {
			buf.Write(o.msg)
		}
		return nil
	}

	for {
		// 优先处理重置信号
		select {
		case <-s.reset:
			s.writeErr = network.ErrReset
			return
		default:
		}

		select {
		case <-s.reset:
			s.writeErr = network.ErrReset
			return
		case <-s.close:
			if err := drainBuf(); err != nil {
				log.Debugf("清空缓冲区失败: %v", err)
				s.cancelWrite(err)
				return
			}
			s.writeErr = s.write.Close()
			if s.writeErr == nil {
				s.writeErr = ErrClosed
			}
			return
		case o := <-s.toDeliver:
			if err := deliverOrWait(o); err != nil {
				log.Debugf("处理消息失败: %v", err)
				s.cancelWrite(err)
				return
			}
		case <-timer.C: // 定时器到期,写出数据
			if err := drainBuf(); err != nil {
				log.Debugf("清空缓冲区失败: %v", err)
				s.cancelWrite(err)
				return
			}
		}
	}
}

// Scope 获取流作用域
// 返回值:
//   - network.StreamScope: 流作用域
func (s *stream) Scope() network.StreamScope {
	return &network.NullScope{}
}

// cancelWrite 取消写入操作
// 参数:
//   - err: error 错误信息
func (s *stream) cancelWrite(err error) {
	s.write.CloseWithError(err)
	s.writeErr = err
}
