// Package endpoint 提供 Endpoint 聚合模块的实现
package endpoint

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	coreif "github.com/dep2p/go-dep2p/pkg/interfaces/endpoint"
	muxerif "github.com/dep2p/go-dep2p/pkg/interfaces/muxer"
	securityif "github.com/dep2p/go-dep2p/pkg/interfaces/security"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// Connection 实现了 coreif.Connection 接口
// 它包装了安全连接和多路复用器，提供统一的连接抽象
type Connection struct {
	// 底层连接
	secureConn securityif.SecureConn
	muxer      muxerif.Muxer

	// 身份信息
	localID    types.NodeID
	remoteID   types.NodeID
	remotePub  coreif.PublicKey
	localAddrs []coreif.Address
	remoteAddr coreif.Address

	// 方向
	direction types.Direction

	// 流处理器（连接级别）
	handlers   map[coreif.ProtocolID]coreif.ProtocolHandler
	handlersMu sync.RWMutex

	// 活跃流
	streams   map[coreif.StreamID]*Stream
	streamsMu sync.RWMutex

	// 状态
	closed    int32 // atomic
	closeCh   chan struct{}
	closeOnce sync.Once
	ctx       context.Context
	cancel    context.CancelFunc

	// 统计
	stats     types.ConnectionStats
	statsMu   sync.RWMutex
	streamSeq uint64 // 流ID序列

	// 引用
	endpoint *Endpoint

	// Realm 上下文 (v1.1 新增)
	// 存储通过 RealmAuth 协议验证后的 Realm 信息
	realmContext *coreif.RealmContext
	realmCtxMu   sync.RWMutex
}

// 确保实现接口
var _ coreif.Connection = (*Connection)(nil)

// NewConnection 创建新的连接包装
func NewConnection(
	secureConn securityif.SecureConn,
	muxer muxerif.Muxer,
	direction types.Direction,
	endpoint *Endpoint,
) *Connection {
	ctx, cancel := context.WithCancel(context.Background())

	conn := &Connection{
		secureConn: secureConn,
		muxer:      muxer,
		direction:  direction,
		handlers:   make(map[coreif.ProtocolID]coreif.ProtocolHandler),
		streams:    make(map[coreif.StreamID]*Stream),
		closeCh:    make(chan struct{}),
		ctx:        ctx,
		cancel:     cancel,
		endpoint:   endpoint,
	}

	// 安全地从 secureConn 提取身份信息
	if secureConn != nil {
		conn.localID = secureConn.LocalIdentity()
		conn.remoteID = secureConn.RemoteIdentity()
		conn.remotePub = secureConn.RemotePublicKey()
		conn.remoteAddr = secureConn.RemoteAddr()
	}

	log.Debug("创建新连接",
		"remoteID", conn.remoteID.ShortString(),
		"direction", direction.String())

	return conn
}

// ==================== 对端信息 ====================

// RemoteID 返回远程节点 ID
func (c *Connection) RemoteID() coreif.NodeID {
	return c.remoteID
}

// RemotePublicKey 返回远程节点公钥
func (c *Connection) RemotePublicKey() coreif.PublicKey {
	return c.remotePub
}

// RemoteAddrs 返回远程节点地址
func (c *Connection) RemoteAddrs() []coreif.Address {
	if c.remoteAddr != nil {
		return []coreif.Address{c.remoteAddr}
	}
	return nil
}

// LocalID 返回本地节点 ID
func (c *Connection) LocalID() coreif.NodeID {
	return c.localID
}

// LocalAddrs 返回本地地址
func (c *Connection) LocalAddrs() []coreif.Address {
	if c.secureConn != nil {
		localAddr := c.secureConn.LocalAddr()
		if localAddr != nil {
			return []coreif.Address{localAddr}
		}
	}
	return c.localAddrs
}

// ==================== 流管理 ====================

// OpenStream 打开一个新流
func (c *Connection) OpenStream(ctx context.Context, protocolID coreif.ProtocolID) (coreif.Stream, error) {
	return c.OpenStreamWithPriority(ctx, protocolID, types.PriorityNormal)
}

// OpenStreamWithPriority 打开指定优先级的流
func (c *Connection) OpenStreamWithPriority(ctx context.Context, protocolID coreif.ProtocolID, priority coreif.Priority) (coreif.Stream, error) {
	if c.IsClosed() {
		return nil, coreif.ErrConnectionClosed
	}

	// 检查 muxer 是否可用
	if c.muxer == nil {
		return nil, fmt.Errorf("连接未启用多路复用: %w", coreif.ErrConnectionClosed)
	}

	// 通过 muxer 创建底层流
	muxerStream, err := c.muxer.NewStream(ctx)
	if err != nil {
		// 检测是否为致命错误（muxer 已关闭）
		// 如果是，主动关闭连接对象，避免被复用
		if isFatalAcceptError(err) {
			_ = c.Close()
		}
		return nil, fmt.Errorf("创建多路复用流失败: %w", err)
	}

	// 发送协议协商
	if err := c.negotiateProtocol(muxerStream, protocolID); err != nil {
		_ = muxerStream.Close()
		return nil, fmt.Errorf("协议协商失败: %w", err)
	}

	// 创建流包装
	streamID := c.nextStreamID()
	stream := NewStream(muxerStream, streamID, protocolID, c, priority)

	// 注册流
	c.streamsMu.Lock()
	c.streams[streamID] = stream
	c.streamsMu.Unlock()

	log.Debug("打开新流",
		"streamID", streamID.String(),
		"protocolID", string(protocolID))

	return stream, nil
}

// AcceptStream 接受一个新流
func (c *Connection) AcceptStream(_ context.Context) (coreif.Stream, error) {
	if c.IsClosed() {
		return nil, coreif.ErrConnectionClosed
	}

	// 检查 muxer 是否可用
	if c.muxer == nil {
		return nil, fmt.Errorf("连接未启用多路复用: %w", coreif.ErrConnectionClosed)
	}

	// 从 muxer 接受流
	muxerStream, err := c.muxer.AcceptStream()
	if err != nil {
		return nil, fmt.Errorf("接受多路复用流失败: %w", err)
	}

	// 接收协议协商
	protocolID, err := c.receiveProtocolNegotiation(muxerStream)
	if err != nil {
		_ = muxerStream.Close()
		return nil, fmt.Errorf("协议协商失败: %w", err)
	}

	// 创建流包装
	streamID := c.nextStreamID()
	stream := NewStream(muxerStream, streamID, protocolID, c, types.PriorityNormal)

	// 注册流
	c.streamsMu.Lock()
	c.streams[streamID] = stream
	c.streamsMu.Unlock()

	log.Debug("接受新流",
		"streamID", streamID.String(),
		"protocolID", string(protocolID))

	return stream, nil
}

// Streams 返回所有活跃流
func (c *Connection) Streams() []coreif.Stream {
	c.streamsMu.RLock()
	defer c.streamsMu.RUnlock()

	streams := make([]coreif.Stream, 0, len(c.streams))
	for _, s := range c.streams {
		if !s.IsClosed() {
			streams = append(streams, s)
		}
	}
	return streams
}

// StreamCount 返回当前流数量
func (c *Connection) StreamCount() int {
	c.streamsMu.RLock()
	defer c.streamsMu.RUnlock()
	return len(c.streams)
}

// ==================== 连接信息 ====================

// Stats 返回连接统计
func (c *Connection) Stats() coreif.ConnectionStats {
	c.statsMu.RLock()
	defer c.statsMu.RUnlock()
	return c.stats
}

// Direction 返回连接方向
func (c *Connection) Direction() coreif.Direction {
	return c.direction
}

// Transport 返回底层传输协议名称
func (c *Connection) Transport() string {
	if c.secureConn != nil {
		return c.secureConn.Transport()
	}
	return "unknown"
}

// ==================== 中继信息 ====================

// IsRelayed 返回连接是否通过中继建立
//
// 检查远程地址是否包含 /p2p-circuit/ 标记来判断是否为中继连接。
func (c *Connection) IsRelayed() bool {
	if c.remoteAddr == nil {
		return false
	}
	return types.IsRelayAddr(c.remoteAddr.String())
}

// RelayID 返回中继节点 ID
//
// 如果连接是通过中继建立的，解析中继地址获取中继节点 ID。
// 如果是直连或解析失败，返回空 NodeID。
func (c *Connection) RelayID() coreif.NodeID {
	if c.remoteAddr == nil {
		return types.EmptyNodeID
	}

	addrStr := c.remoteAddr.String()
	if !types.IsRelayAddr(addrStr) {
		return types.EmptyNodeID
	}

	info, err := types.ParseRelayAddr(addrStr)
	if err != nil {
		return types.EmptyNodeID
	}

	return info.RelayID
}

// ==================== 生命周期 ====================

// Close 关闭连接
func (c *Connection) Close() error {
	return c.CloseWithError(0, "正常关闭")
}

// CloseWithError 带错误码关闭连接
func (c *Connection) CloseWithError(code uint32, reason string) error {
	if !atomic.CompareAndSwapInt32(&c.closed, 0, 1) {
		return nil // 已经关闭
	}

	log.Debug("关闭连接",
		"remoteID", c.remoteID.ShortString(),
		"code", code,
		"reason", reason)

	c.closeOnce.Do(func() {
		// 取消上下文
		c.cancel()

		// 关闭通道
		close(c.closeCh)

		// 先收集所有流引用，然后释放锁再关闭，避免死锁
		c.streamsMu.Lock()
		streams := make([]*Stream, 0, len(c.streams))
		for _, stream := range c.streams {
			streams = append(streams, stream)
		}
		c.streams = make(map[coreif.StreamID]*Stream)
		c.streamsMu.Unlock()

		// 关闭所有流（不持有锁）
		for _, stream := range streams {
			_ = stream.closeInternal()
		}

		// 关闭 muxer
		if c.muxer != nil {
			_ = c.muxer.Close()
		}

		// 关闭安全连接
		if c.secureConn != nil {
			_ = c.secureConn.Close()
		}

		// 从 Endpoint 移除
		if c.endpoint != nil {
			c.endpoint.removeConnection(c.remoteID)
		}
	})

	return nil
}

// IsClosed 检查连接是否已关闭
func (c *Connection) IsClosed() bool {
	return atomic.LoadInt32(&c.closed) == 1
}

// Done 返回连接关闭的通道
func (c *Connection) Done() <-chan struct{} {
	return c.closeCh
}

// Context 返回连接的上下文
func (c *Connection) Context() context.Context {
	return c.ctx
}

// ==================== 扩展功能 ====================

// SetStreamHandler 设置连接级别的流处理器
func (c *Connection) SetStreamHandler(protocolID coreif.ProtocolID, handler coreif.ProtocolHandler) {
	c.handlersMu.Lock()
	defer c.handlersMu.Unlock()
	c.handlers[protocolID] = handler
}

// RemoveStreamHandler 移除连接级别的流处理器
func (c *Connection) RemoveStreamHandler(protocolID coreif.ProtocolID) {
	c.handlersMu.Lock()
	defer c.handlersMu.Unlock()
	delete(c.handlers, protocolID)
}

// ==================== Realm 上下文 (v1.1 新增) ====================

// RealmContext 返回连接级 Realm 上下文
//
// v1.1 新增: 用于 Protocol Router 判断是否允许非系统协议流
// 返回 nil 表示连接尚未进行 Realm 验证
func (c *Connection) RealmContext() *coreif.RealmContext {
	c.realmCtxMu.RLock()
	defer c.realmCtxMu.RUnlock()
	return c.realmContext
}

// SetRealmContext 设置连接级 Realm 上下文
//
// v1.1 新增: 由 RealmAuth 协议处理器在验证成功后调用
// 设置后，该连接上的非系统协议流才能被 Router 接受
func (c *Connection) SetRealmContext(ctx *coreif.RealmContext) {
	c.realmCtxMu.Lock()
	defer c.realmCtxMu.Unlock()
	c.realmContext = ctx

	if ctx != nil {
		log.Debug("设置连接 RealmContext",
			"remoteID", c.remoteID.ShortString(),
			"realmID", ctx.RealmID,
			"verified", ctx.Verified)
	}
}

// ==================== 内部方法 ====================

// nextStreamID 生成下一个流 ID
func (c *Connection) nextStreamID() coreif.StreamID {
	seq := atomic.AddUint64(&c.streamSeq, 1)
	return types.StreamID(seq)
}

// negotiateProtocol 发送协议协商
func (c *Connection) negotiateProtocol(stream muxerif.Stream, protocolID coreif.ProtocolID) error {
	// 写入协议 ID 长度和内容
	protocolBytes := []byte(protocolID)
	if len(protocolBytes) > 1024 {
		return fmt.Errorf("协议 ID 过长: %d", len(protocolBytes))
	}

	lenBuf := make([]byte, 2)
	lenBuf[0] = byte(len(protocolBytes) >> 8)
	lenBuf[1] = byte(len(protocolBytes))

	// 确保完整写入长度字段
	n, err := stream.Write(lenBuf)
	if err != nil {
		return fmt.Errorf("写入协议长度失败: %w", err)
	}
	if n != 2 {
		return fmt.Errorf("协议长度写入不完整: 期望 2 字节, 实际 %d 字节", n)
	}

	// 确保完整写入协议 ID
	n, err = stream.Write(protocolBytes)
	if err != nil {
		return fmt.Errorf("写入协议 ID 失败: %w", err)
	}
	if n != len(protocolBytes) {
		return fmt.Errorf("协议 ID 写入不完整: 期望 %d 字节, 实际 %d 字节", len(protocolBytes), n)
	}

	return nil
}

// receiveProtocolNegotiation 接收协议协商
func (c *Connection) receiveProtocolNegotiation(stream muxerif.Stream) (coreif.ProtocolID, error) {
	// 使用 io.ReadFull 确保读取完整的长度字段
	lenBuf := make([]byte, 2)
	if _, err := io.ReadFull(stream, lenBuf); err != nil {
		return "", fmt.Errorf("读取协议长度失败: %w", err)
	}

	length := int(lenBuf[0])<<8 | int(lenBuf[1])
	if length <= 0 {
		return "", fmt.Errorf("协议 ID 长度无效: %d", length)
	}
	if length > 1024 { // 协议 ID 最大 1KB
		return "", fmt.Errorf("协议 ID 过长: %d", length)
	}

	// 使用 io.ReadFull 确保读取完整的协议 ID
	protocolBuf := make([]byte, length)
	if _, err := io.ReadFull(stream, protocolBuf); err != nil {
		return "", fmt.Errorf("读取协议 ID 失败: %w", err)
	}

	return coreif.ProtocolID(protocolBuf), nil
}

// removeStream 从连接中移除流
func (c *Connection) removeStream(streamID coreif.StreamID) {
	c.streamsMu.Lock()
	defer c.streamsMu.Unlock()
	delete(c.streams, streamID)
}

// getHandler 获取协议处理器（优先连接级别）
func (c *Connection) getHandler(protocolID coreif.ProtocolID) (coreif.ProtocolHandler, bool) {
	// 先检查连接级别
	c.handlersMu.RLock()
	handler, ok := c.handlers[protocolID]
	c.handlersMu.RUnlock()
	if ok {
		return handler, true
	}

	// 再检查 Endpoint 级别
	if c.endpoint != nil {
		return c.endpoint.getHandler(protocolID)
	}

	return nil, false
}

// isFatalAcceptError 判断是否为致命的接受流错误（应退出循环）
func isFatalAcceptError(err error) bool {
	if err == nil {
		return false
	}
	// EOF 通常表示底层连接已断开。若不将其视为致命错误，
	// 连接对象可能继续存活并被复用，最终在 OpenStream 时表现为 “muxer 已关闭”。
	if err == io.EOF {
		return true
	}
	errStr := err.Error()
	return strings.Contains(errStr, "EOF") ||
		strings.Contains(errStr, "muxer closed") ||
		strings.Contains(errStr, "connection closed") ||
		strings.Contains(errStr, "use of closed") ||
		strings.Contains(errStr, "Application error 0x0")
}

// StartStreamAcceptLoop 启动流接受循环
func (c *Connection) StartStreamAcceptLoop(ctx context.Context) {
	go func() {
		const (
			initialBackoff = 10 * time.Millisecond
			maxBackoff     = 5 * time.Second
		)
		var (
			consecutiveErrors int
			backoff           = initialBackoff
		)

		for {
			select {
			case <-ctx.Done():
				return
			case <-c.closeCh:
				return
			default:
			}

			stream, err := c.AcceptStream(ctx)
			if err != nil {
				if c.IsClosed() {
					return
				}
				// 致命错误：muxer 已关闭，需要关闭连接对象
				// 这确保 Endpoint.Connect() 不会复用这个"僵尸"连接
				if isFatalAcceptError(err) {
					log.Debug("接受流循环退出（连接已关闭）", "err", err)
					// 关闭连接对象，确保 IsClosed() 返回 true
					_ = c.Close()
					return
				}
				// 仅首次失败时打印日志，避免日志爆炸
				consecutiveErrors++
				if consecutiveErrors == 1 {
					log.Debug("接受流失败", "err", err)
				}
				// 指数退避，避免紧循环
				select {
				case <-ctx.Done():
					return
				case <-c.closeCh:
					return
				case <-time.After(backoff):
				}
				if backoff < maxBackoff {
					backoff *= 2
					if backoff > maxBackoff {
						backoff = maxBackoff
					}
				}
				continue
			}

			// 成功接受流，重置退避
			consecutiveErrors = 0
			backoff = initialBackoff

			// 查找处理器
			handler, ok := c.getHandler(stream.ProtocolID())
			if !ok {
				log.Warn("未找到协议处理器",
					"protocolID", string(stream.ProtocolID()))
				_ = stream.Close()
				continue
			}

			// 在独立协程中处理流
			go handler(stream)
		}
	}()
}

