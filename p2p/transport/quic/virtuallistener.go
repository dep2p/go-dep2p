package dep2pquic

import (
	"sync"

	tpt "github.com/dep2p/go-dep2p/core/transport"
	"github.com/dep2p/go-dep2p/p2p/transport/quicreuse"

	ma "github.com/dep2p/go-dep2p/multiformats/multiaddr"
	"github.com/quic-go/quic-go"
)

// 每个版本的接受缓冲区大小
const acceptBufferPerVersion = 4

// virtualListener 是一个监听器,它暴露单个多地址但在底层使用另一个监听器
// 参数:
//   - listener: *listener 底层监听器
//   - udpAddr: string UDP 地址
//   - version: quic.Version QUIC 版本
//   - t: *transport 传输层实例
//   - acceptRunnner: *acceptLoopRunner 接受循环运行器
//   - acceptChan: chan acceptVal 接受通道
type virtualListener struct {
	*listener
	udpAddr       string
	version       quic.Version
	t             *transport
	acceptRunnner *acceptLoopRunner
	acceptChan    chan acceptVal
}

// 确保 virtualListener 实现了 tpt.Listener 接口
var _ tpt.Listener = &virtualListener{}

// Multiaddr 返回监听器的多地址
// 返回值:
//   - ma.Multiaddr 监听器的多地址
func (l *virtualListener) Multiaddr() ma.Multiaddr {
	return l.listener.localMultiaddrs[l.version]
}

// Close 关闭监听器
// 返回值:
//   - error 错误信息
func (l *virtualListener) Close() error {
	l.acceptRunnner.RmAcceptForVersion(l.version, tpt.ErrListenerClosed)
	return l.t.CloseVirtualListener(l)
}

// Accept 接受新的连接
// 返回值:
//   - tpt.CapableConn 新建立的连接
//   - error 错误信息
func (l *virtualListener) Accept() (tpt.CapableConn, error) {
	return l.acceptRunnner.Accept(l.listener, l.version, l.acceptChan)
}

// acceptVal 表示接受的连接或错误
// 参数:
//   - conn: tpt.CapableConn 连接对象
//   - err: error 错误信息
type acceptVal struct {
	conn tpt.CapableConn
	err  error
}

// acceptLoopRunner 管理接受循环的运行
// 参数:
//   - acceptSem: chan struct{} 接受信号量
//   - muxerMu: sync.Mutex 多路复用器互斥锁
//   - muxer: map[quic.Version]chan acceptVal 按版本索引的接受通道映射
//   - muxerClosed: bool 多路复用器是否已关闭
type acceptLoopRunner struct {
	acceptSem chan struct{}

	muxerMu     sync.Mutex
	muxer       map[quic.Version]chan acceptVal
	muxerClosed bool
}

// AcceptForVersion 为指定版本创建接受通道
// 参数:
//   - v: quic.Version QUIC 版本
//
// 返回值:
//   - chan acceptVal 接受通道
func (r *acceptLoopRunner) AcceptForVersion(v quic.Version) chan acceptVal {
	r.muxerMu.Lock()
	defer r.muxerMu.Unlock()

	ch := make(chan acceptVal, acceptBufferPerVersion)

	if _, ok := r.muxer[v]; ok {
		panic("接受多路复用器中意外发现已存在的通道")
	}

	r.muxer[v] = ch
	return ch
}

// RmAcceptForVersion 移除指定版本的接受通道
// 参数:
//   - v: quic.Version QUIC 版本
//   - err: error 错误信息
func (r *acceptLoopRunner) RmAcceptForVersion(v quic.Version, err error) {
	r.muxerMu.Lock()
	defer r.muxerMu.Unlock()

	if r.muxerClosed {
		// 已关闭,所有版本都已移除
		return
	}

	ch, ok := r.muxer[v]
	if !ok {
		panic("接受多路复用器中未找到预期的通道")
	}
	ch <- acceptVal{err: err}
	delete(r.muxer, v)
}

// sendErrAndClose 发送错误并关闭所有通道
// 参数:
//   - err: error 要发送的错误
func (r *acceptLoopRunner) sendErrAndClose(err error) {
	r.muxerMu.Lock()
	defer r.muxerMu.Unlock()
	r.muxerClosed = true
	for k, ch := range r.muxer {
		select {
		case ch <- acceptVal{err: err}:
		default:
		}
		delete(r.muxer, k)
		close(ch)
	}
}

// innerAccept 实现接受循环的内部逻辑
// 参数:
//   - l: *listener 监听器
//   - expectedVersion: quic.Version 期望的 QUIC 版本
//   - bufferedConnChan: chan acceptVal 缓冲连接通道
//
// 返回值:
//   - tpt.CapableConn 新建立的连接
//   - error 错误信息
//
// 注意:
//   - 调用者必须持有 acceptSemaphore
//   - 如果未找到期望版本的连接,可能同时返回 nil 连接和 nil 错误
func (r *acceptLoopRunner) innerAccept(l *listener, expectedVersion quic.Version, bufferedConnChan chan acceptVal) (tpt.CapableConn, error) {
	select {
	// 首先检查是否有来自早期 Accept 调用的缓冲连接
	case v, ok := <-bufferedConnChan:
		if !ok {
			return nil, tpt.ErrListenerClosed
		}
		return v.conn, v.err
	default:
	}

	conn, err := l.Accept()

	if err != nil {
		r.sendErrAndClose(err)
		log.Debugf("接受新连接时出错: %s", err)
		return nil, err
	}

	_, version, err := quicreuse.FromQuicMultiaddr(conn.RemoteMultiaddr())
	if err != nil {
		r.sendErrAndClose(err)
		log.Debugf("从QUIC多地址转换为版本时出错: %s", err)
		return nil, err
	}

	if version == expectedVersion {
		return conn, nil
	}

	// 这不是我们期望的版本,将其排队等待不同版本的 Accept 调用
	r.muxerMu.Lock()
	ch, ok := r.muxer[version]
	r.muxerMu.Unlock()

	if !ok {
		// 没有处理此连接版本的通道,关闭连接
		conn.Close()
		return nil, nil
	}

	// 非阻塞发送
	select {
	case ch <- acceptVal{conn: conn}:
	default:
		// 接受队列已满,丢弃连接
		conn.Close()
		log.Warn("接受队列已满,丢弃连接")
	}

	return nil, nil
}

// Accept 接受新的连接
// 参数:
//   - l: *listener 监听器
//   - expectedVersion: quic.Version 期望的 QUIC 版本
//   - bufferedConnChan: chan acceptVal 缓冲连接通道
//
// 返回值:
//   - tpt.CapableConn 新建立的连接
//   - error 错误信息
func (r *acceptLoopRunner) Accept(l *listener, expectedVersion quic.Version, bufferedConnChan chan acceptVal) (tpt.CapableConn, error) {
	for {
		var conn tpt.CapableConn
		var err error
		select {
		case r.acceptSem <- struct{}{}:
			conn, err = r.innerAccept(l, expectedVersion, bufferedConnChan)
			<-r.acceptSem

			if conn == nil && err == nil {
				// 未找到期望版本的连接且没有错误,继续尝试
				continue
			}
		case v, ok := <-bufferedConnChan:
			if !ok {
				log.Debugf("接受通道已关闭")
				return nil, tpt.ErrListenerClosed
			}
			conn = v.conn
			err = v.err
		}
		return conn, err
	}
}
