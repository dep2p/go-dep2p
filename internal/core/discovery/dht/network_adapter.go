// Package dht 提供分布式哈希表实现
package dht

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dep2p/go-dep2p/pkg/interfaces/endpoint"
	discoveryif "github.com/dep2p/go-dep2p/pkg/interfaces/discovery"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              地址簿接口
// ============================================================================

// AddressBookGetter 地址簿查询接口
//
// 用于从外部地址簿获取节点地址，避免 DHT RPC 拨号时触发 discovery 递归。
type AddressBookGetter interface {
	// Get 获取节点的已知地址
	Get(nodeID types.NodeID) []endpoint.Address
}

// ============================================================================
//                              错误定义
// ============================================================================

var (
	// ErrNetworkClosed 网络已关闭
	ErrNetworkClosed = errors.New("dht network adapter is closed")

	// ErrSendFailed 发送失败
	ErrSendFailed = errors.New("failed to send DHT message")

	// ErrTimeout 超时
	ErrTimeout = errors.New("DHT request timeout")

	// ErrInvalidResponse 无效响应
	ErrInvalidResponse = errors.New("invalid DHT response")

	// ErrPeerNotFound 节点未找到
	ErrPeerNotFound = errors.New("peer not found")
)

// ============================================================================
//                              配置
// ============================================================================

const (
	// defaultRequestTimeout 默认请求超时
	defaultRequestTimeout = 10 * time.Second

	// maxMessageSize 最大消息大小（1MB）
	maxMessageSize = 1 << 20

	// frameLengthSize 帧长度字段大小（4字节）
	frameLengthSize = 4
)

// ============================================================================
//                              NetworkAdapter 实现
// ============================================================================

// NetworkAdapter DHT 网络适配器
//
// 实现 dht.Network 接口，将 DHT 请求转换为 dep2p 协议消息
//
// 设计说明：
//
//	为避免 "DHT→Connect→DHT FindPeer" 递归依赖，NetworkAdapter 在发送 DHT RPC 时：
//	1. 优先从 DHT 路由表获取节点地址
//	2. 其次从外部 AddressBook 获取地址
//	3. 使用 ConnectWithAddrs 直接拨号，不触发 discovery 回调
//
// 参考：docs/01-design/protocols/network/01-discovery.md#312-冷启动与拨号闭环
type NetworkAdapter struct {
	// endpoint 节点端点
	endpoint endpoint.Endpoint

	// addressBook 外部地址簿（可选），用于获取节点地址，避免触发 discovery 递归
	addressBook   AddressBookGetter
	addressBookMu sync.RWMutex

	// routingTable DHT 路由表引用（可选），用于获取节点地址
	routingTable   *RoutingTable
	routingTableMu sync.RWMutex

	// localID 本地节点 ID
	localID types.NodeID

	// localAddrs 本地地址
	localAddrs   []string
	localAddrsMu sync.RWMutex

	// requestID 请求 ID 计数器
	requestID uint64

	// requestTimeout 请求超时
	requestTimeout time.Duration

	// closed 是否已关闭
	closed int32
}

// NewNetworkAdapter 创建网络适配器
func NewNetworkAdapter(ep endpoint.Endpoint) *NetworkAdapter {
	var localID types.NodeID
	var localAddrs []string

	if ep != nil {
		localID = types.NodeID(ep.ID())
		// 获取初始地址
		for _, addr := range ep.AdvertisedAddrs() {
			localAddrs = append(localAddrs, addr.String())
		}
	}

	return &NetworkAdapter{
		endpoint:       ep,
		localID:        localID,
		localAddrs:     localAddrs,
		requestTimeout: defaultRequestTimeout,
	}
}

// ============================================================================
//                              Network 接口实现
// ============================================================================

// SendFindNode 发送 FIND_NODE 请求
func (n *NetworkAdapter) SendFindNode(ctx context.Context, to types.NodeID, target types.NodeID) ([]discoveryif.PeerInfo, error) {
	if atomic.LoadInt32(&n.closed) == 1 {
		return nil, ErrNetworkClosed
	}

	reqID := atomic.AddUint64(&n.requestID, 1)

	n.localAddrsMu.RLock()
	addrs := make([]string, len(n.localAddrs))
	copy(addrs, n.localAddrs)
	n.localAddrsMu.RUnlock()

	req := NewFindNodeRequest(reqID, n.localID, addrs, target)

	resp, err := n.sendRequest(ctx, to, req)
	if err != nil {
		return nil, err
	}

	if !resp.Success {
		if resp.Error != "" {
			return nil, errors.New(resp.Error)
		}
		return nil, ErrSendFailed
	}

	// 转换 CloserPeers 为 PeerInfo
	result := make([]discoveryif.PeerInfo, len(resp.CloserPeers))
	for i, peer := range resp.CloserPeers {
		result[i] = discoveryif.PeerInfo{
			ID:    peer.ID,
			Addrs: types.StringsToMultiaddrs(peer.Addrs),
		}
	}

	return result, nil
}

// SendFindValue 发送 FIND_VALUE 请求
func (n *NetworkAdapter) SendFindValue(ctx context.Context, to types.NodeID, key string) ([]byte, []discoveryif.PeerInfo, error) {
	if atomic.LoadInt32(&n.closed) == 1 {
		return nil, nil, ErrNetworkClosed
	}

	reqID := atomic.AddUint64(&n.requestID, 1)

	n.localAddrsMu.RLock()
	addrs := make([]string, len(n.localAddrs))
	copy(addrs, n.localAddrs)
	n.localAddrsMu.RUnlock()

	req := NewFindValueRequest(reqID, n.localID, addrs, key)

	resp, err := n.sendRequest(ctx, to, req)
	if err != nil {
		return nil, nil, err
	}

	if !resp.Success {
		if resp.Error != "" {
			return nil, nil, errors.New(resp.Error)
		}
		return nil, nil, ErrSendFailed
	}

	// 如果找到值，返回值
	if len(resp.Value) > 0 {
		return resp.Value, nil, nil
	}

	// 否则返回更近的节点
	peers := make([]discoveryif.PeerInfo, len(resp.CloserPeers))
	for i, peer := range resp.CloserPeers {
		peers[i] = discoveryif.PeerInfo{
			ID:    peer.ID,
			Addrs: types.StringsToMultiaddrs(peer.Addrs),
		}
	}

	return nil, peers, nil
}

// SendStore 发送 STORE 请求
func (n *NetworkAdapter) SendStore(ctx context.Context, to types.NodeID, key string, value []byte, ttl time.Duration) error {
	if atomic.LoadInt32(&n.closed) == 1 {
		return ErrNetworkClosed
	}

	reqID := atomic.AddUint64(&n.requestID, 1)

	n.localAddrsMu.RLock()
	addrs := make([]string, len(n.localAddrs))
	copy(addrs, n.localAddrs)
	n.localAddrsMu.RUnlock()

	ttlSeconds := uint32(ttl.Seconds())
	if ttlSeconds == 0 {
		ttlSeconds = 86400 // 默认 24 小时
	}

	req := NewStoreRequest(reqID, n.localID, addrs, key, value, ttlSeconds)

	resp, err := n.sendRequest(ctx, to, req)
	if err != nil {
		return err
	}

	if !resp.Success {
		if resp.Error != "" {
			return errors.New(resp.Error)
		}
		return ErrSendFailed
	}

	return nil
}

// SendPing 发送 PING 请求
func (n *NetworkAdapter) SendPing(ctx context.Context, to types.NodeID) (time.Duration, error) {
	if atomic.LoadInt32(&n.closed) == 1 {
		return 0, ErrNetworkClosed
	}

	start := time.Now()
	reqID := atomic.AddUint64(&n.requestID, 1)

	n.localAddrsMu.RLock()
	addrs := make([]string, len(n.localAddrs))
	copy(addrs, n.localAddrs)
	n.localAddrsMu.RUnlock()

	req := NewPingRequest(reqID, n.localID, addrs)

	resp, err := n.sendRequest(ctx, to, req)
	if err != nil {
		return 0, err
	}

	if !resp.Success {
		if resp.Error != "" {
			return 0, errors.New(resp.Error)
		}
		return 0, ErrSendFailed
	}

	return time.Since(start), nil
}

// LocalID 返回本地节点 ID
func (n *NetworkAdapter) LocalID() types.NodeID {
	return n.localID
}

// LocalAddrs 返回本地地址
func (n *NetworkAdapter) LocalAddrs() []string {
	n.localAddrsMu.RLock()
	defer n.localAddrsMu.RUnlock()

	addrs := make([]string, len(n.localAddrs))
	copy(addrs, n.localAddrs)
	return addrs
}

// ============================================================================
//                              Provider 操作
// ============================================================================

// SendAddProvider 发送 ADD_PROVIDER 请求（携带 TTL）
func (n *NetworkAdapter) SendAddProvider(ctx context.Context, to types.NodeID, key string, ttl time.Duration) error {
	if atomic.LoadInt32(&n.closed) == 1 {
		return ErrNetworkClosed
	}

	reqID := atomic.AddUint64(&n.requestID, 1)

	n.localAddrsMu.RLock()
	addrs := make([]string, len(n.localAddrs))
	copy(addrs, n.localAddrs)
	n.localAddrsMu.RUnlock()

	ttlSeconds := uint32(ttl.Seconds())
	if ttlSeconds == 0 {
		ttlSeconds = uint32(DefaultProviderTTL.Seconds())
	}

	req := NewAddProviderRequest(reqID, n.localID, addrs, key, ttlSeconds)

	resp, err := n.sendRequest(ctx, to, req)
	if err != nil {
		return err
	}

	if !resp.Success {
		if resp.Error != "" {
			return errors.New(resp.Error)
		}
		return ErrSendFailed
	}

	return nil
}

// SendRemoveProvider 发送 REMOVE_PROVIDER 请求
func (n *NetworkAdapter) SendRemoveProvider(ctx context.Context, to types.NodeID, key string) error {
	if atomic.LoadInt32(&n.closed) == 1 {
		return ErrNetworkClosed
	}

	reqID := atomic.AddUint64(&n.requestID, 1)

	n.localAddrsMu.RLock()
	addrs := make([]string, len(n.localAddrs))
	copy(addrs, n.localAddrs)
	n.localAddrsMu.RUnlock()

	req := NewRemoveProviderRequest(reqID, n.localID, addrs, key)

	resp, err := n.sendRequest(ctx, to, req)
	if err != nil {
		return err
	}

	if !resp.Success {
		if resp.Error != "" {
			return errors.New(resp.Error)
		}
		return ErrSendFailed
	}

	return nil
}

// SendGetProviders 发送 GET_PROVIDERS 请求
func (n *NetworkAdapter) SendGetProviders(ctx context.Context, to types.NodeID, key string) ([]ProviderInfo, []types.NodeID, error) {
	if atomic.LoadInt32(&n.closed) == 1 {
		return nil, nil, ErrNetworkClosed
	}

	reqID := atomic.AddUint64(&n.requestID, 1)

	n.localAddrsMu.RLock()
	addrs := make([]string, len(n.localAddrs))
	copy(addrs, n.localAddrs)
	n.localAddrsMu.RUnlock()

	req := NewGetProvidersRequest(reqID, n.localID, addrs, key)

	resp, err := n.sendRequest(ctx, to, req)
	if err != nil {
		return nil, nil, err
	}

	if !resp.Success {
		if resp.Error != "" {
			return nil, nil, errors.New(resp.Error)
		}
		return nil, nil, ErrSendFailed
	}

	// 转换 Providers（含 TTL/时间戳）
	providers := make([]ProviderInfo, len(resp.Providers))
	for i, p := range resp.Providers {
		ttl := time.Duration(p.TTL) * time.Second
		if ttl <= 0 {
			ttl = DefaultProviderTTL
		}
		ts := time.Time{}
		if p.Timestamp != 0 {
			ts = time.Unix(0, p.Timestamp)
		} else {
			ts = time.Now()
		}
		providers[i] = ProviderInfo{
			ID:        p.ID,
			Addrs:     p.Addrs,
			Timestamp: ts,
			TTL:       ttl,
		}
	}

	// 转换 CloserPeers（仅 ID 列表）
	closerPeers := make([]types.NodeID, len(resp.CloserPeers))
	for i, p := range resp.CloserPeers {
		closerPeers[i] = p.ID
	}

	return providers, closerPeers, nil
}

// ============================================================================
//                              辅助方法
// ============================================================================

// UpdateLocalAddrs 更新本地地址
func (n *NetworkAdapter) UpdateLocalAddrs(addrs []string) {
	n.localAddrsMu.Lock()
	n.localAddrs = addrs
	n.localAddrsMu.Unlock()
}

// SetAddressBook 设置外部地址簿
//
// 用于从 AddressBook 获取节点地址，避免 DHT RPC 拨号时触发 discovery 递归。
func (n *NetworkAdapter) SetAddressBook(ab AddressBookGetter) {
	n.addressBookMu.Lock()
	n.addressBook = ab
	n.addressBookMu.Unlock()
}

// SetRoutingTable 设置 DHT 路由表引用
//
// 用于从路由表获取节点地址，优先级高于 AddressBook。
func (n *NetworkAdapter) SetRoutingTable(rt *RoutingTable) {
	n.routingTableMu.Lock()
	n.routingTable = rt
	n.routingTableMu.Unlock()
}

// getKnownAddrs 获取节点的已知地址
//
// 地址来源优先级：
// 1. DHT 路由表（已知活跃节点）
// 2. 外部 AddressBook（发现服务缓存）
//
// 这确保 DHT RPC 拨号不会触发 discovery.FindPeer 递归。
func (n *NetworkAdapter) getKnownAddrs(nodeID types.NodeID) []endpoint.Address {
	// 1. 优先从路由表获取
	n.routingTableMu.RLock()
	rt := n.routingTable
	n.routingTableMu.RUnlock()

	if rt != nil {
		if node := rt.Find(nodeID); node != nil && len(node.Addrs) > 0 {
			addrs := make([]endpoint.Address, 0, len(node.Addrs))
			for _, addrStr := range node.Addrs {
				if addr, err := parseAddressString(addrStr); err == nil {
					addrs = append(addrs, addr)
				}
			}
			if len(addrs) > 0 {
				return addrs
			}
		}
	}

	// 2. 从 AddressBook 获取
	n.addressBookMu.RLock()
	ab := n.addressBook
	n.addressBookMu.RUnlock()

	if ab != nil {
		return ab.Get(nodeID)
	}

	return nil
}

// parseAddressString 解析地址字符串
func parseAddressString(s string) (endpoint.Address, error) {
	return &dhtSimpleAddress{addr: s}, nil
}

// dhtSimpleAddress 简单地址实现（仅用于 DHT 内部传递）
type dhtSimpleAddress struct {
	addr string
}

func (a *dhtSimpleAddress) String() string {
	return a.addr
}

func (a *dhtSimpleAddress) Network() string {
	// 基于地址字符串推断网络类型
	if strings.Contains(a.addr, "/ip4/") {
		return "ip4"
	}
	if strings.Contains(a.addr, "/ip6/") {
		return "ip6"
	}
	return "ip4" // 默认
}

func (a *dhtSimpleAddress) Bytes() []byte {
	return []byte(a.addr)
}

func (a *dhtSimpleAddress) Equal(other endpoint.Address) bool {
	return a.addr == other.String()
}

func (a *dhtSimpleAddress) IsPublic() bool {
	return types.Multiaddr(a.addr).IsPublic()
}

func (a *dhtSimpleAddress) IsPrivate() bool {
	return types.Multiaddr(a.addr).IsPrivate()
}

func (a *dhtSimpleAddress) IsLoopback() bool {
	return types.Multiaddr(a.addr).IsLoopback()
}

func (a *dhtSimpleAddress) Multiaddr() string {
	// DHT 地址通常已经是 multiaddr 格式
	return a.addr
}

// Close 关闭适配器
func (n *NetworkAdapter) Close() error {
	atomic.StoreInt32(&n.closed, 1)
	return nil
}

// sendRequest 发送请求并等待响应
//
// 设计说明：
//
//	为避免 "DHT→Connect→DHT FindPeer" 递归依赖，此方法：
//	1. 先从路由表或 AddressBook 获取节点地址
//	2. 使用 ConnectWithAddrs 直接拨号（不触发 discovery 回调）
//	3. 如果没有已知地址，返回 ErrPeerNotFound（而非尝试 discovery）
//
// 参考：docs/01-design/protocols/network/01-discovery.md#312-冷启动与拨号闭环
func (n *NetworkAdapter) sendRequest(ctx context.Context, to types.NodeID, req *Message) (*Message, error) {
	if n.endpoint == nil {
		return nil, ErrNetworkClosed
	}

	// 设置超时
	timeout := n.requestTimeout
	if deadline, ok := ctx.Deadline(); ok {
		if remaining := time.Until(deadline); remaining < timeout {
			timeout = remaining
		}
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// 获取已知地址（避免触发 discovery 递归）
	knownAddrs := n.getKnownAddrs(to)

	var conn endpoint.Connection
	var err error

	if len(knownAddrs) > 0 {
		// 使用 ConnectWithAddrs 直接拨号（不触发 discovery）
		conn, err = n.endpoint.ConnectWithAddrs(ctx, endpoint.NodeID(to), knownAddrs)
		if err != nil {
			log.Debug("使用已知地址连接节点失败",
				"peer", to.ShortString(),
				"addrCount", len(knownAddrs),
				"err", err)
			return nil, err
		}
	} else {
		// 回退：尝试使用 Connect（可能触发 discovery，但仅在完全没有地址时）
		// 这是为了兼容启动阶段路由表和 AddressBook 都为空的情况
		// 此时依赖 Bootstrap/mDNS 等其他发现机制提供初始地址
		conn, err = n.endpoint.Connect(ctx, endpoint.NodeID(to))
		if err != nil {
			log.Debug("连接到节点失败（无已知地址）",
				"peer", to.ShortString(),
				"err", err)
			return nil, err
		}
	}

	// 打开流
	stream, err := conn.OpenStream(ctx, endpoint.ProtocolID(ProtocolID))
	if err != nil {
		log.Debug("打开 DHT 流失败",
			"peer", to.ShortString(),
			"err", err)
		return nil, err
	}
	defer func() { _ = stream.Close() }()

	// 编码请求
	reqData, err := req.Encode()
	if err != nil {
		return nil, err
	}

	// 发送请求（length-prefixed）
	if err := writeFrame(stream, reqData); err != nil {
		log.Debug("发送 DHT 请求失败",
			"peer", to.ShortString(),
			"type", req.Type.String(),
			"err", err)
		return nil, err
	}

	log.Debug("发送 DHT 请求",
		"peer", to.ShortString(),
		"type", req.Type.String(),
		"requestID", req.RequestID)

	// 读取响应
	respData, err := readFrame(stream)
	if err != nil {
		log.Debug("读取 DHT 响应失败",
			"peer", to.ShortString(),
			"err", err)
		return nil, err
	}

	// 解码响应
	resp, err := DecodeMessage(respData)
	if err != nil {
		return nil, err
	}

	// 验证响应
	if resp.RequestID != req.RequestID {
		return nil, ErrInvalidResponse
	}

	log.Debug("收到 DHT 响应",
		"peer", to.ShortString(),
		"type", resp.Type.String(),
		"success", resp.Success)

	return resp, nil
}

// writeFrame 写入带长度前缀的帧
func writeFrame(w io.Writer, data []byte) error {
	if len(data) > maxMessageSize {
		return errors.New("message too large")
	}

	// 写入长度
	lenBuf := make([]byte, frameLengthSize)
	binary.BigEndian.PutUint32(lenBuf, uint32(len(data)))
	if _, err := w.Write(lenBuf); err != nil {
		return err
	}

	// 写入数据
	_, err := w.Write(data)
	return err
}

// readFrame 读取带长度前缀的帧
func readFrame(r io.Reader) ([]byte, error) {
	// 读取长度
	lenBuf := make([]byte, frameLengthSize)
	if _, err := io.ReadFull(r, lenBuf); err != nil {
		return nil, err
	}

	length := binary.BigEndian.Uint32(lenBuf)
	if length > maxMessageSize {
		return nil, errors.New("message too large")
	}

	// 读取数据
	data := make([]byte, length)
	if _, err := io.ReadFull(r, data); err != nil {
		return nil, err
	}

	return data, nil
}

// ============================================================================
//                              确保实现接口
// ============================================================================

var _ Network = (*NetworkAdapter)(nil)

