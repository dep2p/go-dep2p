package swarm

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	ic "github.com/dep2p/go-dep2p/core/crypto"
	"github.com/dep2p/go-dep2p/core/network"
	"github.com/dep2p/go-dep2p/core/peer"
	"github.com/dep2p/go-dep2p/core/transport"

	ma "github.com/dep2p/go-dep2p/multiformats/multiaddr"
)

// TODO: 将此内容移至其他位置

// ErrConnClosed 在操作已关闭的连接时返回
var ErrConnClosed = errors.New("连接已关闭")

// Conn 是 swarm 使用的连接类型。通常情况下,你不会直接使用此类型
type Conn struct {
	// id 是连接的唯一标识符
	id uint64
	// conn 是底层的传输层连接
	conn transport.CapableConn
	// swarm 是此连接所属的 Swarm 实例
	swarm *Swarm

	// closeOnce 确保连接只被关闭一次
	closeOnce sync.Once
	// err 存储关闭时的错误信息
	err error

	// notifyLk 用于通知操作的互斥锁
	notifyLk sync.Mutex

	// streams 存储此连接上的所有流
	streams struct {
		sync.Mutex
		m map[*Stream]struct{}
	}

	// stat 存储连接的统计信息
	stat network.ConnStats
}

// 确保 Conn 实现了 network.Conn 接口
var _ network.Conn = &Conn{}

// IsClosed 检查连接是否已关闭
//
// 返回值:
//   - bool 如果连接已关闭返回 true,否则返回 false
func (c *Conn) IsClosed() bool {
	return c.conn.IsClosed()
}

// ID 返回连接的唯一标识符
//
// 返回值:
//   - string 格式为 "<peer id 的前10个字符>-<全局连接序号>"
func (c *Conn) ID() string {
	return fmt.Sprintf("%s-%d", c.RemotePeer().String()[:10], c.id)
}

// Close 关闭此连接
//
// 返回值:
//   - error 关闭过程中的错误,如果有的话
//
// 注意:
//   - 此方法不会等待关闭通知完成,因为这可能在打开通知时造成死锁
//   - 所有打开通知必须在触发关闭通知之前完成
func (c *Conn) Close() error {
	c.closeOnce.Do(c.doClose)
	return c.err
}

// doClose 执行实际的关闭操作
func (c *Conn) doClose() {
	// 从 swarm 中移除此连接
	c.swarm.removeConn(c)

	// 防止新的流被打开
	c.streams.Lock()
	streams := c.streams.m
	c.streams.m = nil
	c.streams.Unlock()

	// 关闭底层连接
	c.err = c.conn.Close()

	// 在关闭连接后发送连接状态事件
	// 这确保了远程和本地的连接关闭事件都在底层传输连接关闭后发送
	c.swarm.connectednessEventEmitter.RemoveConn(c.RemotePeer())

	// 清理状态,连接已经关闭
	// 我们可以优化这部分,但实际上并不值得
	for s := range streams {
		s.Reset()
	}

	// 在 goroutine 中执行以避免在打开通知中调用 close 时死锁
	go func() {
		// 防止在完成打开通知之前发出关闭通知
		c.notifyLk.Lock()
		defer c.notifyLk.Unlock()

		// 只有在之前发送了连接通知的情况下才发送断开连接通知
		c.swarm.notifyAll(func(f network.Notifiee) {
			f.Disconnected(c.swarm, c)
		})
		c.swarm.refs.Done()
	}()
}

// removeStream 从连接中移除一个流
//
// 参数:
//   - s: *Stream 要移除的流
func (c *Conn) removeStream(s *Stream) {
	c.streams.Lock()
	c.stat.NumStreams--
	delete(c.streams.m, s)
	c.streams.Unlock()
	s.scope.Done()
}

// start 监听新的流
//
// 注意:
//   - 调用者必须在调用前获取 swarm 引用
//   - 此函数会减少 swarm 引用计数
func (c *Conn) start() {
	go func() {
		defer c.swarm.refs.Done()
		defer c.Close()
		for {
			ts, err := c.conn.AcceptStream()
			if err != nil {
				return
			}
			scope, err := c.swarm.ResourceManager().OpenStream(c.RemotePeer(), network.DirInbound)
			if err != nil {
				ts.Reset()
				continue
			}
			c.swarm.refs.Add(1)
			go func() {
				s, err := c.addStream(ts, network.DirInbound, scope)

				// 不要使用 defer,我们不想在连接处理器上阻塞 swarm 关闭
				c.swarm.refs.Done()

				// 只有在 swarm 关闭或正在关闭时才会出现错误
				if err != nil {
					scope.Done()
					return
				}

				if h := c.swarm.StreamHandler(); h != nil {
					h(s)
				}
				s.completeAcceptStreamGoroutine()
			}()
		}
	}()
}

// String 返回连接的字符串表示
//
// 返回值:
//   - string 连接的描述性字符串
func (c *Conn) String() string {
	return fmt.Sprintf(
		"<swarm.Conn[%T] %s (%s) <-> %s (%s)>",
		c.conn.Transport(),
		c.conn.LocalMultiaddr(),
		c.conn.LocalPeer(),
		c.conn.RemoteMultiaddr(),
		c.conn.RemotePeer(),
	)
}

// LocalMultiaddr 返回本地端的多地址
//
// 返回值:
//   - ma.Multiaddr 本地端的多地址
func (c *Conn) LocalMultiaddr() ma.Multiaddr {
	return c.conn.LocalMultiaddr()
}

// LocalPeer 返回连接本地端的对等点标识
//
// 返回值:
//   - peer.ID 本地端的对等点标识
func (c *Conn) LocalPeer() peer.ID {
	return c.conn.LocalPeer()
}

// RemoteMultiaddr 返回远程端的多地址
//
// 返回值:
//   - ma.Multiaddr 远程端的多地址
func (c *Conn) RemoteMultiaddr() ma.Multiaddr {
	return c.conn.RemoteMultiaddr()
}

// RemotePeer 返回连接远程端的对等点标识
//
// 返回值:
//   - peer.ID 远程端的对等点标识
func (c *Conn) RemotePeer() peer.ID {
	return c.conn.RemotePeer()
}

// RemotePublicKey 返回远程端对等点的公钥
//
// 返回值:
//   - ic.PubKey 远程端的公钥
func (c *Conn) RemotePublicKey() ic.PubKey {
	return c.conn.RemotePublicKey()
}

// ConnState 返回安全连接状态,包括早期数据结果
// 如果不支持则返回空
//
// 返回值:
//   - network.ConnectionState 连接状态
func (c *Conn) ConnState() network.ConnectionState {
	return c.conn.ConnState()
}

// Stat 返回此连接的元数据
//
// 返回值:
//   - network.ConnStats 连接统计信息
func (c *Conn) Stat() network.ConnStats {
	c.streams.Lock()
	defer c.streams.Unlock()
	return c.stat
}

// NewStream 从此连接创建新的流
//
// 参数:
//   - ctx: context.Context 上下文对象
//
// 返回值:
//   - network.Stream 新创建的流
//   - error 创建过程中的错误,如果有的话
func (c *Conn) NewStream(ctx context.Context) (network.Stream, error) {
	if c.Stat().Limited {
		if useLimited, _ := network.GetAllowLimitedConn(ctx); !useLimited {
			log.Debugf("连接限制: %v", network.ErrLimitedConn)
			return nil, network.ErrLimitedConn
		}
	}

	scope, err := c.swarm.ResourceManager().OpenStream(c.RemotePeer(), network.DirOutbound)
	if err != nil {
		log.Debugf("打开流失败: %v", err)
		return nil, err
	}

	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, defaultNewStreamTimeout)
		defer cancel()
	}

	s, err := c.openAndAddStream(ctx, scope)
	if err != nil {
		scope.Done()
		if errors.Is(err, context.DeadlineExceeded) {
			err = fmt.Errorf("超时: %w", err)
			log.Debugf("超时: %v", err)
		}
		return nil, err
	}
	return s, nil
}

// openAndAddStream 打开并添加一个新的流
//
// 参数:
//   - ctx: context.Context 上下文对象
//   - scope: network.StreamManagementScope 流管理作用域
//
// 返回值:
//   - network.Stream 新创建的流
//   - error 创建过程中的错误,如果有的话
func (c *Conn) openAndAddStream(ctx context.Context, scope network.StreamManagementScope) (network.Stream, error) {
	ts, err := c.conn.OpenStream(ctx)
	if err != nil {
		log.Debugf("打开流失败: %v", err)
		return nil, err
	}
	return c.addStream(ts, network.DirOutbound, scope)
}

// addStream 添加一个新的流到连接
//
// 参数:
//   - ts: network.MuxedStream 多路复用的流
//   - dir: network.Direction 流的方向
//   - scope: network.StreamManagementScope 流管理作用域
//
// 返回值:
//   - *Stream 新添加的流
//   - error 添加过程中的错误,如果有的话
func (c *Conn) addStream(ts network.MuxedStream, dir network.Direction, scope network.StreamManagementScope) (*Stream, error) {
	c.streams.Lock()
	// 检查是否仍在线
	if c.streams.m == nil {
		c.streams.Unlock()
		ts.Reset()
		log.Debugf("连接已关闭")
		return nil, ErrConnClosed
	}

	// 包装并注册流
	s := &Stream{
		stream: ts,
		conn:   c,
		scope:  scope,
		stat: network.Stats{
			Direction: dir,
			Opened:    time.Now(),
		},
		id:                             c.swarm.nextStreamID.Add(1),
		acceptStreamGoroutineCompleted: dir != network.DirInbound,
	}
	c.stat.NumStreams++
	c.streams.m[s] = struct{}{}

	// 在流断开连接通知完成后释放(在 Swarm.remove 中)
	c.swarm.refs.Add(1)

	c.streams.Unlock()
	return s, nil
}

// GetStreams 返回与此连接关联的所有流
//
// 返回值:
//   - []network.Stream 流的切片
func (c *Conn) GetStreams() []network.Stream {
	c.streams.Lock()
	defer c.streams.Unlock()
	streams := make([]network.Stream, 0, len(c.streams.m))
	for s := range c.streams.m {
		streams = append(streams, s)
	}
	return streams
}

// Scope 返回连接的作用域
//
// 返回值:
//   - network.ConnScope 连接作用域
func (c *Conn) Scope() network.ConnScope {
	return c.conn.Scope()
}
