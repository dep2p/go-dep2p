package mocks

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// MockPeerstore 模拟 Peerstore 接口实现
//
// 用于测试需要 Peerstore 依赖的组件。
type MockPeerstore struct {
	mu sync.RWMutex

	// 存储
	addrs     map[types.PeerID][]types.Multiaddr
	addrTTLs  map[types.PeerID]time.Time
	pubKeys   map[types.PeerID]interfaces.PublicKey
	privKeys  map[types.PeerID]interfaces.PrivateKey
	protocols map[types.PeerID][]types.ProtocolID
	metadata  map[types.PeerID]map[string]interface{}
	nodes     map[types.PeerID]*interfaces.NodeRecord

	// 可覆盖的方法（仅列出常用的）
	PeersFunc          func() []types.PeerID
	PeerInfoFunc       func(peerID types.PeerID) types.PeerInfo
	AddrsFunc          func(peerID types.PeerID) []types.Multiaddr
	PubKeyFunc         func(peerID types.PeerID) (interfaces.PublicKey, error)
	GetProtocolsFunc   func(peerID types.PeerID) ([]types.ProtocolID, error)
	SupportsProtocolsFunc func(peerID types.PeerID, protocols ...types.ProtocolID) ([]types.ProtocolID, error)
}

// NewMockPeerstore 创建带有默认值的 MockPeerstore
func NewMockPeerstore() *MockPeerstore {
	return &MockPeerstore{
		addrs:     make(map[types.PeerID][]types.Multiaddr),
		addrTTLs:  make(map[types.PeerID]time.Time),
		pubKeys:   make(map[types.PeerID]interfaces.PublicKey),
		privKeys:  make(map[types.PeerID]interfaces.PrivateKey),
		protocols: make(map[types.PeerID][]types.ProtocolID),
		metadata:  make(map[types.PeerID]map[string]interface{}),
		nodes:     make(map[types.PeerID]*interfaces.NodeRecord),
	}
}

// ============================================================================
// Peerstore 核心方法
// ============================================================================

// Peers 返回所有已知节点 ID
func (m *MockPeerstore) Peers() []types.PeerID {
	if m.PeersFunc != nil {
		return m.PeersFunc()
	}
	m.mu.RLock()
	defer m.mu.RUnlock()

	peerSet := make(map[types.PeerID]bool)
	for peerID := range m.addrs {
		peerSet[peerID] = true
	}
	for peerID := range m.pubKeys {
		peerSet[peerID] = true
	}
	for peerID := range m.protocols {
		peerSet[peerID] = true
	}

	peers := make([]types.PeerID, 0, len(peerSet))
	for peerID := range peerSet {
		peers = append(peers, peerID)
	}
	return peers
}

// PeerInfo 返回指定节点的完整信息
func (m *MockPeerstore) PeerInfo(peerID types.PeerID) types.PeerInfo {
	if m.PeerInfoFunc != nil {
		return m.PeerInfoFunc(peerID)
	}
	m.mu.RLock()
	defer m.mu.RUnlock()

	return types.PeerInfo{
		ID:    peerID,
		Addrs: m.addrs[peerID],
	}
}

// RemovePeer 移除节点信息（除地址外）
func (m *MockPeerstore) RemovePeer(peerID types.PeerID) {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.pubKeys, peerID)
	delete(m.privKeys, peerID)
	delete(m.protocols, peerID)
	delete(m.metadata, peerID)
}

// QuerySeeds 查询种子节点
func (m *MockPeerstore) QuerySeeds(count int, maxAge time.Duration) []*interfaces.NodeRecord {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var seeds []*interfaces.NodeRecord
	cutoff := time.Now().Add(-maxAge)

	for _, record := range m.nodes {
		if record.LastSeen.After(cutoff) {
			seeds = append(seeds, record)
			if len(seeds) >= count {
				break
			}
		}
	}
	return seeds
}

// UpdateDialAttempt 更新拨号尝试结果
func (m *MockPeerstore) UpdateDialAttempt(peerID types.PeerID, success bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	record, ok := m.nodes[peerID]
	if !ok {
		record = &interfaces.NodeRecord{ID: string(peerID)}
		m.nodes[peerID] = record
	}

	if success {
		record.FailedDials = 0
		record.LastSeen = time.Now()
	} else {
		record.FailedDials++
	}
	return nil
}

// NodeDBSize 返回节点缓存大小
func (m *MockPeerstore) NodeDBSize() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.nodes)
}

// Close 关闭存储
func (m *MockPeerstore) Close() error {
	return nil
}

// ============================================================================
// AddrBook 方法
// ============================================================================

// AddAddr 添加单个地址
func (m *MockPeerstore) AddAddr(peerID types.PeerID, addr types.Multiaddr, ttl time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.addrs[peerID] = append(m.addrs[peerID], addr)
	m.addrTTLs[peerID] = time.Now().Add(ttl)
}

// AddAddrs 添加节点地址
func (m *MockPeerstore) AddAddrs(peerID types.PeerID, addrs []types.Multiaddr, ttl time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.addrs[peerID] = append(m.addrs[peerID], addrs...)
	m.addrTTLs[peerID] = time.Now().Add(ttl)
}

// SetAddr 设置单个地址（覆盖现有）
func (m *MockPeerstore) SetAddr(peerID types.PeerID, addr types.Multiaddr, ttl time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.addrs[peerID] = []types.Multiaddr{addr}
	m.addrTTLs[peerID] = time.Now().Add(ttl)
}

// SetAddrs 设置节点地址（覆盖现有）
func (m *MockPeerstore) SetAddrs(peerID types.PeerID, addrs []types.Multiaddr, ttl time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.addrs[peerID] = addrs
	m.addrTTLs[peerID] = time.Now().Add(ttl)
}

// UpdateAddrs 更新地址 TTL
func (m *MockPeerstore) UpdateAddrs(peerID types.PeerID, oldTTL time.Duration, newTTL time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.addrs[peerID]; ok {
		m.addrTTLs[peerID] = time.Now().Add(newTTL)
	}
}

// Addrs 获取节点地址
func (m *MockPeerstore) Addrs(peerID types.PeerID) []types.Multiaddr {
	if m.AddrsFunc != nil {
		return m.AddrsFunc(peerID)
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.addrs[peerID]
}

// AddrStream 返回节点地址更新的通道
func (m *MockPeerstore) AddrStream(ctx context.Context, peerID types.PeerID) <-chan types.Multiaddr {
	ch := make(chan types.Multiaddr)
	go func() {
		defer close(ch)
		m.mu.RLock()
		addrs := m.addrs[peerID]
		m.mu.RUnlock()
		for _, addr := range addrs {
			select {
			case <-ctx.Done():
				return
			case ch <- addr:
			}
		}
	}()
	return ch
}

// ClearAddrs 清除节点地址
func (m *MockPeerstore) ClearAddrs(peerID types.PeerID) {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.addrs, peerID)
	delete(m.addrTTLs, peerID)
}

// PeersWithAddrs 返回拥有地址的节点列表
func (m *MockPeerstore) PeersWithAddrs() []types.PeerID {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var peers []types.PeerID
	for peerID, addrs := range m.addrs {
		if len(addrs) > 0 {
			peers = append(peers, peerID)
		}
	}
	return peers
}

// ============================================================================
// KeyBook 方法
// ============================================================================

// PubKey 获取节点公钥
func (m *MockPeerstore) PubKey(peerID types.PeerID) (interfaces.PublicKey, error) {
	if m.PubKeyFunc != nil {
		return m.PubKeyFunc(peerID)
	}
	m.mu.RLock()
	defer m.mu.RUnlock()

	if key, ok := m.pubKeys[peerID]; ok {
		return key, nil
	}
	return nil, errors.New("public key not found")
}

// AddPubKey 添加节点公钥
func (m *MockPeerstore) AddPubKey(peerID types.PeerID, pubKey interfaces.PublicKey) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.pubKeys[peerID] = pubKey
	return nil
}

// PrivKey 获取本地节点私钥
func (m *MockPeerstore) PrivKey(peerID types.PeerID) (interfaces.PrivateKey, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if key, ok := m.privKeys[peerID]; ok {
		return key, nil
	}
	return nil, errors.New("private key not found")
}

// AddPrivKey 添加本地节点私钥
func (m *MockPeerstore) AddPrivKey(peerID types.PeerID, privKey interfaces.PrivateKey) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.privKeys[peerID] = privKey
	return nil
}

// PeersWithKeys 返回拥有密钥的节点列表
func (m *MockPeerstore) PeersWithKeys() []types.PeerID {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var peers []types.PeerID
	for peerID := range m.pubKeys {
		peers = append(peers, peerID)
	}
	return peers
}

// ============================================================================
// ProtoBook 方法
// ============================================================================

// GetProtocols 获取节点支持的协议
func (m *MockPeerstore) GetProtocols(peerID types.PeerID) ([]types.ProtocolID, error) {
	if m.GetProtocolsFunc != nil {
		return m.GetProtocolsFunc(peerID)
	}
	m.mu.RLock()
	defer m.mu.RUnlock()

	if protos, ok := m.protocols[peerID]; ok {
		return protos, nil
	}
	return nil, nil
}

// AddProtocols 添加节点支持的协议
func (m *MockPeerstore) AddProtocols(peerID types.PeerID, protocols ...types.ProtocolID) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.protocols[peerID] = append(m.protocols[peerID], protocols...)
	return nil
}

// SetProtocols 设置节点支持的协议（覆盖）
func (m *MockPeerstore) SetProtocols(peerID types.PeerID, protocols ...types.ProtocolID) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.protocols[peerID] = protocols
	return nil
}

// RemoveProtocols 移除节点支持的协议
func (m *MockPeerstore) RemoveProtocols(peerID types.PeerID, protocols ...types.ProtocolID) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	existing := m.protocols[peerID]
	toRemove := make(map[types.ProtocolID]bool)
	for _, p := range protocols {
		toRemove[p] = true
	}

	var filtered []types.ProtocolID
	for _, p := range existing {
		if !toRemove[p] {
			filtered = append(filtered, p)
		}
	}
	m.protocols[peerID] = filtered
	return nil
}

// SupportsProtocols 检查节点是否支持指定协议
func (m *MockPeerstore) SupportsProtocols(peerID types.PeerID, protocols ...types.ProtocolID) ([]types.ProtocolID, error) {
	if m.SupportsProtocolsFunc != nil {
		return m.SupportsProtocolsFunc(peerID, protocols...)
	}
	m.mu.RLock()
	defer m.mu.RUnlock()

	supported := make(map[types.ProtocolID]bool)
	for _, p := range m.protocols[peerID] {
		supported[p] = true
	}

	var result []types.ProtocolID
	for _, p := range protocols {
		if supported[p] {
			result = append(result, p)
		}
	}
	return result, nil
}

// FirstSupportedProtocol 返回首个支持的协议
func (m *MockPeerstore) FirstSupportedProtocol(peerID types.PeerID, protocols ...types.ProtocolID) (types.ProtocolID, error) {
	supported, err := m.SupportsProtocols(peerID, protocols...)
	if err != nil {
		return "", err
	}
	if len(supported) == 0 {
		return "", errors.New("no supported protocol")
	}
	return supported[0], nil
}

// ============================================================================
// PeerMetadata 方法
// ============================================================================

// Get 获取元数据
func (m *MockPeerstore) Get(peerID types.PeerID, key string) (interface{}, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if meta, ok := m.metadata[peerID]; ok {
		if val, ok := meta[key]; ok {
			return val, nil
		}
	}
	return nil, errors.New("metadata not found")
}

// Put 存储元数据
func (m *MockPeerstore) Put(peerID types.PeerID, key string, val interface{}) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.metadata[peerID] == nil {
		m.metadata[peerID] = make(map[string]interface{})
	}
	m.metadata[peerID][key] = val
	return nil
}

// 确保实现接口
var _ interfaces.Peerstore = (*MockPeerstore)(nil)
