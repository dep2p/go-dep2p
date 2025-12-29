// Package protocol 提供协议协商器实现
//
// 基于 multistream-select 协议实现协议协商：
// https://github.com/multiformats/multistream-select
package protocol

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/dep2p/go-dep2p/pkg/interfaces/endpoint"
	protocolif "github.com/dep2p/go-dep2p/pkg/interfaces/protocol"
	"github.com/dep2p/go-dep2p/pkg/interfaces/transport"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              常量
// ============================================================================

const (
	// MultistreamID multistream-select 协议标识
	MultistreamID = "/multistream/1.0.0"

	// NA 协议不支持响应
	NA = "na"

	// LS 列出协议命令
	LS = "ls"

	// MaxMsgLen 最大消息长度
	MaxMsgLen = 1024 * 64 // 64KB

	// DefaultNegotiationTimeout 默认协商超时
	DefaultNegotiationTimeout = 10 * time.Second

	// MaxCacheSize 最大缓存条目数
	MaxCacheSize = 10000
)

// ============================================================================
//                              错误定义
// ============================================================================

// 协议协商相关错误
var (
	// ErrNegotiationTimeout 协商超时
	ErrNegotiationTimeout = errors.New("negotiation timeout")
	ErrNoCommonProtocol    = errors.New("no common protocol")
	ErrInvalidMessage      = errors.New("invalid negotiation message")
	ErrMessageTooLong      = errors.New("message too long")
	ErrUnexpectedResponse  = errors.New("unexpected response")
	ErrConnectionClosed    = errors.New("connection closed during negotiation")
)

// ============================================================================
//                              Negotiator 实现
// ============================================================================

// Negotiator 协议协商器
type Negotiator struct {
	registry *Registry
	endpoint endpoint.Endpoint
	timeout  time.Duration

	// 缓存已协商的结果
	cache   map[string]types.ProtocolID
	cacheMu sync.RWMutex
}

// NewNegotiator 创建协商器
func NewNegotiator(registry *Registry, endpoint endpoint.Endpoint, timeout time.Duration) *Negotiator {
	if timeout == 0 {
		timeout = DefaultNegotiationTimeout
	}

	return &Negotiator{
		registry: registry,
		endpoint: endpoint,
		timeout:  timeout,
		cache:    make(map[string]types.ProtocolID),
	}
}

// 确保实现接口
var _ protocolif.Negotiator = (*Negotiator)(nil)

// ============================================================================
//                              协商接口
// ============================================================================

// Negotiate 在连接上协商协议
func (n *Negotiator) Negotiate(conn transport.Conn, protocols []types.ProtocolID) (types.ProtocolID, error) {
	if len(protocols) == 0 {
		return "", ErrNoCommonProtocol
	}

	// 设置超时
	deadline := time.Now().Add(n.timeout)
	conn.SetDeadline(deadline)
	defer conn.SetDeadline(time.Time{})

	// 创建读写器
	reader := bufio.NewReader(conn)
	writer := bufio.NewWriter(conn)

	// 发送 multistream 协议头
	if err := writeMessage(writer, MultistreamID); err != nil {
		return "", fmt.Errorf("failed to send multistream header: %w", err)
	}
	if err := writer.Flush(); err != nil {
		return "", err
	}

	// 读取响应
	resp, err := readMessage(reader)
	if err != nil {
		return "", fmt.Errorf("failed to read multistream response: %w", err)
	}

	if resp != MultistreamID {
		return "", fmt.Errorf("%w: expected %s, got %s", ErrUnexpectedResponse, MultistreamID, resp)
	}

	// 尝试每个协议
	for _, proto := range protocols {
		// 发送协议请求
		if err := writeMessage(writer, string(proto)); err != nil {
			return "", err
		}
		if err := writer.Flush(); err != nil {
			return "", err
		}

		// 读取响应
		resp, err := readMessage(reader)
		if err != nil {
			return "", err
		}

		if resp == string(proto) {
			// 协商成功
			log.Debug("协议协商成功",
				"protocol", string(proto))
			return proto, nil
		}

		if resp != NA {
			return "", fmt.Errorf("%w: got %s", ErrUnexpectedResponse, resp)
		}

		// 继续尝试下一个协议
	}

	return "", ErrNoCommonProtocol
}

// NegotiateWithPeer 与指定节点协商
func (n *Negotiator) NegotiateWithPeer(ctx context.Context, peer types.NodeID, protocols []types.ProtocolID) (types.ProtocolID, error) {
	if n.endpoint == nil {
		return "", errors.New("no endpoint available")
	}

	// 检查缓存
	cacheKey := fmt.Sprintf("%s:%v", peer.ShortString(), protocols)
	n.cacheMu.RLock()
	cached, ok := n.cache[cacheKey]
	n.cacheMu.RUnlock()
	if ok {
		return cached, nil
	}

	// 连接到节点
	conn, err := n.endpoint.Connect(ctx, peer)
	if err != nil {
		return "", fmt.Errorf("failed to connect to peer: %w", err)
	}

	// 打开协商流
	stream, err := conn.OpenStream(ctx, types.ProtocolID(MultistreamID))
	if err != nil {
		return "", fmt.Errorf("failed to open negotiation stream: %w", err)
	}
	defer func() { _ = stream.Close() }()

	// 使用流进行协商
	result, err := n.negotiateOnStream(stream, protocols)
	if err != nil {
		return "", err
	}

	// 缓存结果（带大小限制）
	n.cacheResult(cacheKey, result)

	return result, nil
}

// cacheResult 缓存协商结果，带大小限制
func (n *Negotiator) cacheResult(key string, result types.ProtocolID) {
	n.cacheMu.Lock()
	defer n.cacheMu.Unlock()

	// 缓存大小限制：超过限制时清理一半
	if len(n.cache) >= MaxCacheSize {
		count := 0
		for k := range n.cache {
			delete(n.cache, k)
			count++
			if count >= MaxCacheSize/2 {
				break
			}
		}
		log.Debug("清理协商缓存",
			"cleared", count,
			"remaining", len(n.cache))
	}

	n.cache[key] = result
}

// negotiateOnStream 在流上协商
func (n *Negotiator) negotiateOnStream(stream endpoint.Stream, protocols []types.ProtocolID) (types.ProtocolID, error) {
	reader := bufio.NewReader(stream)
	writer := bufio.NewWriter(stream)

	// 发送 multistream 协议头
	if err := writeMessage(writer, MultistreamID); err != nil {
		return "", err
	}
	if err := writer.Flush(); err != nil {
		return "", err
	}

	// 读取响应
	resp, err := readMessage(reader)
	if err != nil {
		return "", err
	}

	if resp != MultistreamID {
		return "", ErrUnexpectedResponse
	}

	// 尝试每个协议
	for _, proto := range protocols {
		if err := writeMessage(writer, string(proto)); err != nil {
			return "", err
		}
		if err := writer.Flush(); err != nil {
			return "", err
		}

		resp, err := readMessage(reader)
		if err != nil {
			return "", err
		}

		if resp == string(proto) {
			return proto, nil
		}
	}

	return "", ErrNoCommonProtocol
}

// SelectProtocol 从本地和远程支持的协议中选择最优协议
func (n *Negotiator) SelectProtocol(local, remote []types.ProtocolID) (types.ProtocolID, error) {
	if len(local) == 0 || len(remote) == 0 {
		return "", ErrNoCommonProtocol
	}

	// 按本地优先级顺序查找共同支持的协议
	for _, l := range local {
		for _, r := range remote {
			// 精确匹配
			if l == r {
				return l, nil
			}

			// 语义版本匹配
			lName, lVersion := ParseProtocolID(l)
			rName, rVersion := ParseProtocolID(r)

			if lName == rName && IsCompatibleVersion(lVersion, rVersion) {
				// 返回本地版本
				return l, nil
			}
		}
	}

	return "", ErrNoCommonProtocol
}

// ============================================================================
//                              Listener 功能
// ============================================================================

// HandleIncoming 处理入站连接的协商
func (n *Negotiator) HandleIncoming(stream endpoint.Stream) (types.ProtocolID, error) {
	return n.HandleIncomingWithTimeout(stream, DefaultNegotiationTimeout)
}

// HandleIncomingWithTimeout 处理入站连接的协商（带超时）
func (n *Negotiator) HandleIncomingWithTimeout(stream endpoint.Stream, timeout time.Duration) (types.ProtocolID, error) {
	// 设置读取超时防止无限阻塞
	if timeout > 0 {
		if err := stream.SetReadDeadline(time.Now().Add(timeout)); err != nil {
			log.Warn("设置读取超时失败", "err", err)
		}
		defer func() {
			// 清除超时
			_ = stream.SetReadDeadline(time.Time{})
		}()
	}

	reader := bufio.NewReader(stream)
	writer := bufio.NewWriter(stream)

	// 读取 multistream 协议头
	msg, err := readMessage(reader)
	if err != nil {
		return "", err
	}

	if msg != MultistreamID {
		return "", fmt.Errorf("%w: expected multistream header", ErrUnexpectedResponse)
	}

	// 响应 multistream 头
	if err := writeMessage(writer, MultistreamID); err != nil {
		return "", err
	}
	if err := writer.Flush(); err != nil {
		return "", err
	}

	// 协商循环（带最大尝试次数防止无限循环）
	const maxAttempts = 100
	for attempt := 0; attempt < maxAttempts; attempt++ {
		msg, err := readMessage(reader)
		if err != nil {
			if err == io.EOF {
				return "", ErrConnectionClosed
			}
			return "", err
		}

		// 处理特殊命令
		if msg == LS {
			// 列出支持的协议
			if err := n.sendProtocolList(writer); err != nil {
				return "", err
			}
			continue
		}

		proto := types.ProtocolID(msg)

		// 检查是否支持该协议
		if n.registry != nil {
			if _, ok := n.registry.MatchHandler(proto); ok {
				// 支持该协议
				if err := writeMessage(writer, msg); err != nil {
					return "", err
				}
				if err := writer.Flush(); err != nil {
					return "", err
				}

				log.Debug("接受入站协议",
					"protocol", msg)

				return proto, nil
			}
		}

		// 不支持，发送 NA
		if err := writeMessage(writer, NA); err != nil {
			return "", err
		}
		if err := writer.Flush(); err != nil {
			return "", err
		}

		log.Debug("拒绝入站协议",
			"protocol", msg)
	}

	return "", fmt.Errorf("超过最大协商尝试次数 (%d)", maxAttempts)
}

// sendProtocolList 发送协议列表
func (n *Negotiator) sendProtocolList(writer *bufio.Writer) error {
	var protocols []types.ProtocolID

	if n.registry != nil {
		protocols = n.registry.IDs()
	}

	// 编码协议列表
	var buf bytes.Buffer
	for _, proto := range protocols {
		buf.WriteString(string(proto))
		buf.WriteByte('\n')
	}

	if err := writeMessage(writer, buf.String()); err != nil {
		return err
	}
	return writer.Flush()
}

// ============================================================================
//                              缓存管理
// ============================================================================

// ClearCache 清空缓存
func (n *Negotiator) ClearCache() {
	n.cacheMu.Lock()
	n.cache = make(map[string]types.ProtocolID)
	n.cacheMu.Unlock()
}

// ClearCacheForPeer 清空指定节点的缓存
func (n *Negotiator) ClearCacheForPeer(peer types.NodeID) {
	n.cacheMu.Lock()
	defer n.cacheMu.Unlock()

	prefix := peer.ShortString() + ":"
	for key := range n.cache {
		if len(key) > len(prefix) && key[:len(prefix)] == prefix {
			delete(n.cache, key)
		}
	}
}

// ============================================================================
//                              消息编解码
// ============================================================================

// writeMessage 写入消息
//
// 消息格式: [varint length][data]\n
func writeMessage(w *bufio.Writer, msg string) error {
	if len(msg) > MaxMsgLen {
		return ErrMessageTooLong
	}

	// 写入长度（包含换行符）
	length := len(msg) + 1
	if err := writeVarint(w, length); err != nil {
		return err
	}

	// 写入消息
	if _, err := w.WriteString(msg); err != nil {
		return err
	}

	// 写入换行符
	return w.WriteByte('\n')
}

// readMessage 读取消息
func readMessage(r *bufio.Reader) (string, error) {
	// 读取长度
	length, err := readVarint(r)
	if err != nil {
		return "", err
	}

	if length > MaxMsgLen {
		return "", ErrMessageTooLong
	}

	if length == 0 {
		return "", nil
	}

	// 读取消息
	buf := make([]byte, length)
	if _, err := io.ReadFull(r, buf); err != nil {
		return "", err
	}

	// 去除换行符
	msg := string(buf)
	if len(msg) > 0 && msg[len(msg)-1] == '\n' {
		msg = msg[:len(msg)-1]
	}

	return msg, nil
}

// writeVarint 写入变长整数
func writeVarint(w *bufio.Writer, n int) error {
	var buf [binary.MaxVarintLen64]byte
	length := binary.PutUvarint(buf[:], uint64(n))
	_, err := w.Write(buf[:length])
	return err
}

// readVarint 读取变长整数
func readVarint(r *bufio.Reader) (int, error) {
	var result uint64
	var shift uint

	for {
		b, err := r.ReadByte()
		if err != nil {
			return 0, err
		}

		result |= uint64(b&0x7F) << shift
		if b&0x80 == 0 {
			break
		}

		shift += 7
		if shift >= 64 {
			return 0, ErrInvalidMessage
		}
	}

	return int(result), nil
}

// ============================================================================
//                              Lazy Negotiator
// ============================================================================

// LazyNegotiator 延迟协商器
//
// 延迟协商直到第一次发送数据时才执行
type LazyNegotiator struct {
	*Negotiator
	stream     endpoint.Stream
	protocol   types.ProtocolID
	negotiated bool
	mu         sync.Mutex
}

// NewLazyNegotiator 创建延迟协商器
func NewLazyNegotiator(negotiator *Negotiator, stream endpoint.Stream) *LazyNegotiator {
	return &LazyNegotiator{
		Negotiator: negotiator,
		stream:     stream,
	}
}

// Write 写入数据（延迟协商）
func (ln *LazyNegotiator) Write(protocols []types.ProtocolID, data []byte) (int, error) {
	ln.mu.Lock()
	defer ln.mu.Unlock()

	if !ln.negotiated {
		proto, err := ln.negotiateOnStream(ln.stream, protocols)
		if err != nil {
			return 0, err
		}
		ln.protocol = proto
		ln.negotiated = true
	}

	return ln.stream.Write(data)
}

// Protocol 返回协商的协议
func (ln *LazyNegotiator) Protocol() types.ProtocolID {
	return ln.protocol
}

// IsNegotiated 是否已协商
func (ln *LazyNegotiator) IsNegotiated() bool {
	return ln.negotiated
}

// ============================================================================
//                              Simultaneous Negotiation
// ============================================================================

// SimultaneousNegotiator 同时协商器
//
// 支持双方同时发起协商（避免死锁）
type SimultaneousNegotiator struct {
	*Negotiator
}

// NewSimultaneousNegotiator 创建同时协商器
func NewSimultaneousNegotiator(negotiator *Negotiator) *SimultaneousNegotiator {
	return &SimultaneousNegotiator{Negotiator: negotiator}
}

// NegotiateSimultaneous 同时协商
func (sn *SimultaneousNegotiator) NegotiateSimultaneous(stream endpoint.Stream, isInitiator bool, protocols []types.ProtocolID) (types.ProtocolID, error) {
	reader := bufio.NewReader(stream)
	writer := bufio.NewWriter(stream)

	// 根据角色决定谁先发送
	if isInitiator {
		// 发起方先发送
		if err := writeMessage(writer, MultistreamID); err != nil {
			return "", err
		}
		if err := writer.Flush(); err != nil {
			return "", err
		}

		// 读取响应
		resp, err := readMessage(reader)
		if err != nil {
			return "", err
		}

		// 对方可能也发送了 multistream 头
		if resp == MultistreamID {
			// 继续常规协商
			return sn.continueNegotiation(reader, writer, protocols)
		}

		// 对方可能直接发送了协议
		if sn.isProtocolSupported(types.ProtocolID(resp), protocols) {
			return types.ProtocolID(resp), nil
		}
	} else {
		// 响应方等待接收
		resp, err := readMessage(reader)
		if err != nil {
			return "", err
		}

		if resp == MultistreamID {
			// 发送确认
			if err := writeMessage(writer, MultistreamID); err != nil {
				return "", err
			}
			if err := writer.Flush(); err != nil {
				return "", err
			}

			// 继续作为响应方
			return sn.HandleIncoming(stream)
		}
	}

	return "", ErrNoCommonProtocol
}

// continueNegotiation 继续协商
func (sn *SimultaneousNegotiator) continueNegotiation(reader *bufio.Reader, writer *bufio.Writer, protocols []types.ProtocolID) (types.ProtocolID, error) {
	for _, proto := range protocols {
		if err := writeMessage(writer, string(proto)); err != nil {
			return "", err
		}
		if err := writer.Flush(); err != nil {
			return "", err
		}

		resp, err := readMessage(reader)
		if err != nil {
			return "", err
		}

		if resp == string(proto) {
			return proto, nil
		}
	}

	return "", ErrNoCommonProtocol
}

// isProtocolSupported 检查协议是否支持
func (sn *SimultaneousNegotiator) isProtocolSupported(proto types.ProtocolID, supported []types.ProtocolID) bool {
	for _, p := range supported {
		if p == proto {
			return true
		}
	}
	return false
}

