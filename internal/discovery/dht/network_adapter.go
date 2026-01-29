package dht

import (
	"context"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"time"

	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// NetworkAdapter DHT 网络适配器
//
// ## 防递归设计
//
// NetworkAdapter 的核心目标是避免 "DHT→Connect→Discovery→DHT" 的递归依赖。
//
// 策略：
//  1. Connect 时，优先从 DHT 路由表获取节点地址（不触发 discovery）
//  2. 其次从 Peerstore 获取地址
//  3. 使用 Host.Connect 直接拨号（不触发高层 discovery）
//  4. 通过 Host.NewStream 创建 DHT 协议流
//
// 这样确保了 DHT 的网络操作不会触发回调到自身的发现流程。
type NetworkAdapter struct {
	// host 网络主机
	host pkgif.Host

	// routingTable DHT 路由表（用于获取已知地址）
	routingTable *RoutingTable

	// peerstore 节点信息存储（可选）
	peerstore pkgif.Peerstore

	// 发送超时
	sendTimeout time.Duration

	// 连接超时
	connectTimeout time.Duration

	// 状态
	closed atomic.Bool

	mu sync.RWMutex
}

// NewNetworkAdapter 创建网络适配器
func NewNetworkAdapter(
	host pkgif.Host,
	routingTable *RoutingTable,
	peerstore pkgif.Peerstore,
) *NetworkAdapter {
	return &NetworkAdapter{
		host:           host,
		routingTable:   routingTable,
		peerstore:      peerstore,
		sendTimeout:    15 * time.Second, // v2.0.1: 从 30s 减少到 15s，快速失败以便重试
		connectTimeout: 5 * time.Second,  // v2.0.1: 从 10s 减少到 5s，快速失败以便重试
	}
}

// Connect 连接到节点
//
// 防递归关键：不触发高层 Discovery，直接使用已知地址
func (na *NetworkAdapter) Connect(ctx context.Context, peerID types.PeerID) error {
	if na.closed.Load() {
		return ErrNetworkClosed
	}

	// 1. 从路由表获取地址（优先级最高，避免递归）
	addrs := na.getAddrsFromRoutingTable(peerID)

	// 2. 从 Peerstore 获取地址（备选）
	if len(addrs) == 0 && na.peerstore != nil {
		multiaddrs := na.peerstore.Addrs(peerID)
		// 转换 Multiaddr 为 string
		for _, ma := range multiaddrs {
			addrs = append(addrs, ma.String())
		}
	}

	// 3. 如果没有地址，返回错误（不触发 discovery）
	if len(addrs) == 0 {
		return fmt.Errorf("no addresses for peer %s", peerID)
	}

	// 4. 直接连接（使用 Host.Connect，不触发高层 Discovery）
	connectCtx, cancel := context.WithTimeout(ctx, na.connectTimeout)
	defer cancel()

	return na.host.Connect(connectCtx, string(peerID), addrs)
}

// getAddrsFromRoutingTable 从路由表获取节点地址
func (na *NetworkAdapter) getAddrsFromRoutingTable(peerID types.PeerID) []string {
	node := na.routingTable.Get(types.NodeID(peerID))
	if node == nil {
		return nil
	}
	return node.Addrs
}

// SendMessage 发送消息到节点
func (na *NetworkAdapter) SendMessage(ctx context.Context, peerID types.PeerID, msg *Message) (*Message, error) {
	if na.closed.Load() {
		return nil, ErrNetworkClosed
	}

	// 确保已连接
	if err := na.Connect(ctx, peerID); err != nil {
		return nil, fmt.Errorf("connect failed: %w", err)
	}

	// 创建流
	stream, err := na.host.NewStream(ctx, string(peerID), ProtocolID)
	if err != nil {
		return nil, fmt.Errorf("create stream failed: %w", err)
	}
	defer stream.Close()

	// 发送请求
	if err := na.writeMessage(stream, msg); err != nil {
		return nil, fmt.Errorf("write message failed: %w", err)
	}

	// 读取响应
	sendCtx, cancel := context.WithTimeout(ctx, na.sendTimeout)
	defer cancel()

	respCh := make(chan *Message, 1)
	errCh := make(chan error, 1)

	go func() {
		resp, err := na.readMessage(stream)
		if err != nil {
			errCh <- err
			return
		}
		respCh <- resp
	}()

	select {
	case resp := <-respCh:
		return resp, nil
	case err := <-errCh:
		return nil, err
	case <-sendCtx.Done():
		return nil, ErrTimeout
	}
}

// writeMessage 写入消息
func (na *NetworkAdapter) writeMessage(w io.Writer, msg *Message) error {
	data, err := msg.Encode()
	if err != nil {
		return fmt.Errorf("encode message failed: %w", err)
	}

	// 写入消息长度（4字节）
	lenBuf := make([]byte, 4)
	lenBuf[0] = byte(len(data) >> 24)
	lenBuf[1] = byte(len(data) >> 16)
	lenBuf[2] = byte(len(data) >> 8)
	lenBuf[3] = byte(len(data))

	if _, err := w.Write(lenBuf); err != nil {
		return fmt.Errorf("write length failed: %w", err)
	}

	// 写入消息体
	if _, err := w.Write(data); err != nil {
		return fmt.Errorf("write data failed: %w", err)
	}

	return nil
}

// readMessage 读取消息
func (na *NetworkAdapter) readMessage(r io.Reader) (*Message, error) {
	// 读取消息长度（4字节）
	lenBuf := make([]byte, 4)
	if _, err := io.ReadFull(r, lenBuf); err != nil {
		return nil, fmt.Errorf("read length failed: %w", err)
	}

	msgLen := int(lenBuf[0])<<24 | int(lenBuf[1])<<16 | int(lenBuf[2])<<8 | int(lenBuf[3])

	// 限制消息大小（最大 1MB）
	if msgLen <= 0 || msgLen > 1024*1024 {
		return nil, fmt.Errorf("invalid message length: %d", msgLen)
	}

	// 读取消息体
	dataBuf := make([]byte, msgLen)
	if _, err := io.ReadFull(r, dataBuf); err != nil {
		return nil, fmt.Errorf("read data failed: %w", err)
	}

	// 解码消息
	msg, err := DecodeMessage(dataBuf)
	if err != nil {
		return nil, fmt.Errorf("decode message failed: %w", err)
	}

	return msg, nil
}

// Close 关闭适配器
func (na *NetworkAdapter) Close() error {
	na.closed.Store(true)
	return nil
}

// Ping 发送 Ping 消息
func (na *NetworkAdapter) Ping(ctx context.Context, peerID types.PeerID) (time.Duration, error) {
	if na.closed.Load() {
		return 0, ErrNetworkClosed
	}

	localID := types.NodeID(na.host.ID())
	//使用 AdvertisedAddrs 确保包含 Relay 地址
	localAddrs := na.host.AdvertisedAddrs()

	msg := NewPingRequest(0, localID, localAddrs)

	start := time.Now()
	resp, err := na.SendMessage(ctx, peerID, msg)
	rtt := time.Since(start)

	if err != nil {
		return 0, err
	}

	if !resp.Success {
		return 0, fmt.Errorf("ping failed: %s", resp.Error)
	}

	return rtt, nil
}

// FindNode 发送 FindNode 请求
func (na *NetworkAdapter) FindNode(ctx context.Context, peerID types.PeerID, target types.NodeID) ([]PeerRecord, error) {
	if na.closed.Load() {
		return nil, ErrNetworkClosed
	}

	localID := types.NodeID(na.host.ID())
	//使用 AdvertisedAddrs 确保包含 Relay 地址
	localAddrs := na.host.AdvertisedAddrs()

	msg := NewFindNodeRequest(0, localID, localAddrs, target)

	resp, err := na.SendMessage(ctx, peerID, msg)
	if err != nil {
		return nil, err
	}

	if !resp.Success {
		return nil, fmt.Errorf("find node failed: %s", resp.Error)
	}

	return resp.CloserPeers, nil
}

// FindValue 发送 FindValue 请求
func (na *NetworkAdapter) FindValue(ctx context.Context, peerID types.PeerID, key string) ([]byte, []PeerRecord, error) {
	if na.closed.Load() {
		return nil, nil, ErrNetworkClosed
	}

	localID := types.NodeID(na.host.ID())
	//使用 AdvertisedAddrs 确保包含 Relay 地址
	localAddrs := na.host.AdvertisedAddrs()

	msg := NewFindValueRequest(0, localID, localAddrs, key)

	resp, err := na.SendMessage(ctx, peerID, msg)
	if err != nil {
		return nil, nil, err
	}

	if !resp.Success {
		return nil, nil, fmt.Errorf("find value failed: %s", resp.Error)
	}

	// 如果找到值，返回值
	if len(resp.Value) > 0 {
		return resp.Value, nil, nil
	}

	// 否则返回更近的节点
	return nil, resp.CloserPeers, nil
}

// Store 发送 Store 请求
func (na *NetworkAdapter) Store(ctx context.Context, peerID types.PeerID, key string, value []byte, ttl uint32) error {
	if na.closed.Load() {
		return ErrNetworkClosed
	}

	localID := types.NodeID(na.host.ID())
	//使用 AdvertisedAddrs 确保包含 Relay 地址
	localAddrs := na.host.AdvertisedAddrs()

	msg := NewStoreRequest(0, localID, localAddrs, key, value, ttl)

	resp, err := na.SendMessage(ctx, peerID, msg)
	if err != nil {
		return err
	}

	if !resp.Success {
		return fmt.Errorf("store failed: %s", resp.Error)
	}

	return nil
}

// AddProvider 发送 AddProvider 请求
func (na *NetworkAdapter) AddProvider(ctx context.Context, peerID types.PeerID, key string, ttl uint32) error {
	if na.closed.Load() {
		return ErrNetworkClosed
	}

	localID := types.NodeID(na.host.ID())
	//使用 AdvertisedAddrs 确保包含 Relay 地址
	localAddrs := na.host.AdvertisedAddrs()

	msg := NewAddProviderRequest(0, localID, localAddrs, key, ttl)

	resp, err := na.SendMessage(ctx, peerID, msg)
	if err != nil {
		return err
	}

	if !resp.Success {
		return fmt.Errorf("add provider failed: %s", resp.Error)
	}

	return nil
}

// GetProviders 发送 GetProviders 请求
func (na *NetworkAdapter) GetProviders(ctx context.Context, peerID types.PeerID, key string) ([]PeerRecord, []PeerRecord, error) {
	if na.closed.Load() {
		return nil, nil, ErrNetworkClosed
	}

	localID := types.NodeID(na.host.ID())
	//使用 AdvertisedAddrs 确保包含 Relay 地址
	localAddrs := na.host.AdvertisedAddrs()

	msg := NewGetProvidersRequest(0, localID, localAddrs, key)

	resp, err := na.SendMessage(ctx, peerID, msg)
	if err != nil {
		return nil, nil, err
	}

	if !resp.Success {
		return nil, nil, fmt.Errorf("get providers failed: %s", resp.Error)
	}

	return resp.Providers, resp.CloserPeers, nil
}
