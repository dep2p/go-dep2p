// Package dht 提供分布式哈希表实现
//
// 本文件实现本地 PeerRecord 管理器，负责：
// - 管理本地 PeerRecord 的 seq 递增
// - 检测地址变化并触发重新发布
// - 提供签名 PeerRecord 的创建
package dht

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/dep2p/go-dep2p/pkg/lib/crypto"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              LocalPeerRecordManager
// ============================================================================

// LocalPeerRecordManager 本地 PeerRecord 管理器
//
// 职责：
//   - 管理本地 PeerRecord 的序列号（seq）递增
//   - 检测地址变化并触发重新发布
//   - 创建和签名本地 PeerRecord
//   - 跟踪最后发布时间
type LocalPeerRecordManager struct {
	mu sync.RWMutex

	// seq 当前序列号（原子操作）
	seq uint64

	// lastRecord 最后发布的记录
	lastRecord *SignedRealmPeerRecord

	// lastPublishTime 最后发布时间
	lastPublishTime time.Time

	// lastAddrs 最后发布的地址（用于变化检测）
	lastDirectAddrs []string
	lastRelayAddrs  []string

	// privKey 私钥（用于签名）
	privKey crypto.PrivateKey

	// localNodeID 本地节点 ID
	localNodeID types.NodeID

	// realmID 当前 Realm ID
	realmID types.RealmID
}

// NewLocalPeerRecordManager 创建本地 PeerRecord 管理器
func NewLocalPeerRecordManager() *LocalPeerRecordManager {
	return &LocalPeerRecordManager{
		seq: 0,
	}
}

// Initialize 初始化管理器
//
// 必须在使用前调用，设置私钥和节点信息
func (m *LocalPeerRecordManager) Initialize(privKey crypto.PrivateKey, nodeID types.NodeID, realmID types.RealmID) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.privKey = privKey
	m.localNodeID = nodeID
	m.realmID = realmID
}

// IsInitialized 检查是否已初始化
func (m *LocalPeerRecordManager) IsInitialized() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.privKey != nil && m.localNodeID != ""
}

// RealmID 返回当前 RealmID
func (m *LocalPeerRecordManager) RealmID() (types.RealmID, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.realmID == "" {
		return "", false
	}
	return m.realmID, true
}

// SetRealmID 设置/更新 RealmID
//
// Step B3 对齐：用于 Realm Join 后切换发布目标
// 调用后，CreateSignedRecord 创建的记录将包含新的 RealmID，
// 发布时会自动使用 RealmPeerKey 而非 GlobalPeerKey。
func (m *LocalPeerRecordManager) SetRealmID(realmID types.RealmID) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.realmID = realmID
}

// NextSeq 获取下一个序列号（原子递增）
func (m *LocalPeerRecordManager) NextSeq() uint64 {
	return atomic.AddUint64(&m.seq, 1)
}

// CurrentSeq 获取当前序列号
func (m *LocalPeerRecordManager) CurrentSeq() uint64 {
	return atomic.LoadUint64(&m.seq)
}

// SetSeq 设置序列号（用于从持久化恢复）
func (m *LocalPeerRecordManager) SetSeq(seq uint64) {
	atomic.StoreUint64(&m.seq, seq)
}

// CreateSignedRecord 创建并签名新的 PeerRecord
//
// 参数:
//   - directAddrs: 直连地址列表
//   - relayAddrs: 中继地址列表
//   - natType: NAT 类型
//   - reachability: 可达性状态
//   - capabilities: 支持的能力
//   - ttl: 记录 TTL
//
// 返回:
//   - 签名的 PeerRecord
//   - 错误信息
func (m *LocalPeerRecordManager) CreateSignedRecord(
	directAddrs []string,
	relayAddrs []string,
	natType types.NATType,
	reachability types.Reachability,
	capabilities []string,
	ttl time.Duration,
) (*SignedRealmPeerRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.privKey == nil {
		return nil, ErrNilPeerRecord
	}

	// 递增序列号
	seq := atomic.AddUint64(&m.seq, 1)

	// 创建记录
	record := &RealmPeerRecord{
		NodeID:       m.localNodeID,
		RealmID:      m.realmID,
		DirectAddrs:  directAddrs,
		RelayAddrs:   relayAddrs,
		NATType:      natType,
		Reachability: reachability,
		Capabilities: capabilities,
		Seq:          seq,
		Timestamp:    time.Now().UnixNano(),
		TTL:          int64(ttl / time.Millisecond),
	}

	// 签名
	signed, err := SignRealmPeerRecord(m.privKey, record)
	if err != nil {
		return nil, err
	}

	// 保存最后发布的记录
	m.lastRecord = signed
	m.lastPublishTime = time.Now()
	m.lastDirectAddrs = directAddrs
	m.lastRelayAddrs = relayAddrs

	return signed, nil
}

// NeedsRepublish 检查是否需要重新发布
//
// 条件：
//  1. 超过 republishInterval 未发布
//  2. 地址发生变化
func (m *LocalPeerRecordManager) NeedsRepublish(republishInterval time.Duration, currentDirectAddrs, currentRelayAddrs []string) (needsRepublish bool, reason string) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// 从未发布过
	if m.lastRecord == nil {
		return true, "never published"
	}

	// 检查时间间隔
	elapsed := time.Since(m.lastPublishTime)
	if elapsed >= republishInterval {
		return true, "interval expired"
	}

	// 检查地址变化
	if !stringSliceEqual(m.lastDirectAddrs, currentDirectAddrs) {
		return true, "direct addrs changed"
	}
	if !stringSliceEqual(m.lastRelayAddrs, currentRelayAddrs) {
		return true, "relay addrs changed"
	}

	return false, ""
}

// LastPublishTime 返回最后发布时间
func (m *LocalPeerRecordManager) LastPublishTime() time.Time {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.lastPublishTime
}

// LastRecord 返回最后发布的记录
func (m *LocalPeerRecordManager) LastRecord() *SignedRealmPeerRecord {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.lastRecord
}

// NodeID 返回本地节点 ID
func (m *LocalPeerRecordManager) NodeID() types.NodeID {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.localNodeID
}

// Clear 清理管理器状态
//
// Phase D 对齐：优雅关闭时调用，阻止后续的 republish 操作。
// 清理后 IsInitialized() 将返回 false。
func (m *LocalPeerRecordManager) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 清空私钥（阻止后续签名）
	m.privKey = nil

	// 清空记录状态
	m.lastRecord = nil
	m.lastPublishTime = time.Time{}
	m.lastDirectAddrs = nil
	m.lastRelayAddrs = nil

	// 注意：保留 localNodeID 和 realmID 用于 UnpublishPeerRecord 中获取 Key
	// 但 IsInitialized() 会返回 false（因为 privKey 为 nil）
}

// ============================================================================
//                              辅助函数
// ============================================================================

// stringSliceEqual 比较两个字符串切片是否相等
func stringSliceEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}

	// 创建 map 统计
	aMap := make(map[string]int)
	for _, s := range a {
		aMap[s]++
	}

	for _, s := range b {
		if aMap[s] == 0 {
			return false
		}
		aMap[s]--
	}

	return true
}

// ============================================================================
//                              AddressChangeDetector
// ============================================================================

// AddressChangeDetector 地址变化检测器
//
// 用于检测本地地址变化，触发 PeerRecord 重新发布
type AddressChangeDetector struct {
	mu sync.RWMutex

	// lastAddrs 上次检测到的地址
	lastAddrs []string

	// onChange 变化回调
	onChange func(oldAddrs, newAddrs []string)
}

// NewAddressChangeDetector 创建地址变化检测器
func NewAddressChangeDetector(onChange func(oldAddrs, newAddrs []string)) *AddressChangeDetector {
	return &AddressChangeDetector{
		onChange: onChange,
	}
}

// Check 检查地址是否变化
//
// 如果变化，调用 onChange 回调并返回 true
func (d *AddressChangeDetector) Check(currentAddrs []string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()

	if stringSliceEqual(d.lastAddrs, currentAddrs) {
		return false
	}

	oldAddrs := d.lastAddrs
	d.lastAddrs = make([]string, len(currentAddrs))
	copy(d.lastAddrs, currentAddrs)

	if d.onChange != nil {
		d.onChange(oldAddrs, currentAddrs)
	}

	return true
}

// LastAddrs 返回上次检测到的地址
func (d *AddressChangeDetector) LastAddrs() []string {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.lastAddrs
}
