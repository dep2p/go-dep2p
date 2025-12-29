// Package address 提供地址管理模块的实现
package address

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"time"

	"github.com/dep2p/go-dep2p/pkg/interfaces/endpoint"
	"github.com/dep2p/go-dep2p/pkg/interfaces/identity"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              错误定义
// ============================================================================

var (
	// ErrInvalidSignature 签名无效
	ErrInvalidSignature = errors.New("invalid address record signature")

	// ErrExpiredRecord 记录已过期
	ErrExpiredRecord = errors.New("address record has expired")

	// ErrStaleSequence 序列号过旧
	ErrStaleSequence = errors.New("address record sequence is stale")

	// ErrEmptyAddresses 地址列表为空
	ErrEmptyAddresses = errors.New("address record has no addresses")
)

// ============================================================================
//                              AddressRecord - 签名地址记录
// ============================================================================

// AddressRecord 签名地址记录
//
// 用于在 P2P 网络中安全地传播节点地址信息：
// - 签名防止伪造（只有私钥持有者才能创建有效记录）
// - 序列号防止重放攻击（只接受更新的记录）
// - TTL 控制有效期
type AddressRecord struct {
	// NodeID 节点 ID
	NodeID types.NodeID

	// RealmID 领域 ID（可选，用于 Realm 隔离）
	RealmID types.RealmID

	// Sequence 序列号（单调递增，用于防重放）
	Sequence uint64

	// Addresses 地址列表
	Addresses []endpoint.Address

	// Timestamp 记录创建时间
	Timestamp time.Time

	// TTL 有效期
	TTL time.Duration

	// Signature 签名
	Signature []byte
}

// NewAddressRecord 创建新的地址记录
func NewAddressRecord(nodeID types.NodeID, addresses []endpoint.Address, ttl time.Duration) *AddressRecord {
	return &AddressRecord{
		NodeID:    nodeID,
		Sequence:  uint64(time.Now().UnixNano()), // 使用纳秒时间戳作为初始序列号
		Addresses: addresses,
		Timestamp: time.Now(),
		TTL:       ttl,
	}
}

// NewAddressRecordWithRealm 创建带 Realm 的地址记录
func NewAddressRecordWithRealm(nodeID types.NodeID, realmID types.RealmID, addresses []endpoint.Address, ttl time.Duration) *AddressRecord {
	record := NewAddressRecord(nodeID, addresses, ttl)
	record.RealmID = realmID
	return record
}

// ============================================================================
//                              签名与验证
// ============================================================================

// signedData 返回用于签名的数据
//
// 签名数据格式: NodeID || RealmID || Sequence || AddressCount || Addresses... || Timestamp || TTL
func (r *AddressRecord) signedData() []byte {
	// 计算所需缓冲区大小
	size := 32 + // NodeID
		len(r.RealmID) + // RealmID (变长)
		8 + // Sequence
		4 // AddressCount

	// 计算地址数据大小
	for _, addr := range r.Addresses {
		size += 4 + len(addr.String()) // 长度前缀 + 地址字符串
	}

	size += 8 + 8 // Timestamp + TTL

	// 分配缓冲区
	data := make([]byte, 0, size)

	// NodeID
	data = append(data, r.NodeID[:]...)

	// RealmID
	data = append(data, []byte(r.RealmID)...)

	// Sequence
	seqBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(seqBytes, r.Sequence)
	data = append(data, seqBytes...)

	// AddressCount
	countBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(countBytes, uint32(len(r.Addresses)))
	data = append(data, countBytes...)

	// Addresses
	for _, addr := range r.Addresses {
		addrStr := addr.String()
		lenBytes := make([]byte, 4)
		binary.BigEndian.PutUint32(lenBytes, uint32(len(addrStr)))
		data = append(data, lenBytes...)
		data = append(data, []byte(addrStr)...)
	}

	// Timestamp (Unix 纳秒)
	tsBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(tsBytes, uint64(r.Timestamp.UnixNano()))
	data = append(data, tsBytes...)

	// TTL (纳秒)
	ttlBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(ttlBytes, uint64(r.TTL.Nanoseconds()))
	data = append(data, ttlBytes...)

	return data
}

// Sign 对记录进行签名
//
// 使用节点的私钥对记录进行签名，确保记录的真实性。
func (r *AddressRecord) Sign(privateKey identity.PrivateKey) error {
	if privateKey == nil {
		return errors.New("private key is nil")
	}

	// 获取待签名数据
	data := r.signedData()

	// 计算哈希
	hash := sha256.Sum256(data)

	// 签名
	sig, err := privateKey.Sign(hash[:])
	if err != nil {
		return err
	}

	r.Signature = sig
	return nil
}

// Verify 验证记录签名
//
// 使用节点的公钥验证记录签名，确保记录未被篡改。
func (r *AddressRecord) Verify(publicKey identity.PublicKey) bool {
	if publicKey == nil || len(r.Signature) == 0 {
		return false
	}

	// 获取待签名数据
	data := r.signedData()

	// 计算哈希
	hash := sha256.Sum256(data)

	// 验证签名
	valid, err := publicKey.Verify(hash[:], r.Signature)
	if err != nil {
		return false
	}
	return valid
}

// ============================================================================
//                              有效性检查
// ============================================================================

// IsExpired 检查记录是否已过期
func (r *AddressRecord) IsExpired() bool {
	if r.TTL == 0 {
		return false // TTL 为 0 表示永不过期
	}
	return time.Since(r.Timestamp) > r.TTL
}

// IsNewerThan 检查记录是否比另一个记录更新
//
// 基于序列号比较，序列号大的更新。
func (r *AddressRecord) IsNewerThan(other *AddressRecord) bool {
	if other == nil {
		return true
	}
	return r.Sequence > other.Sequence
}

// Validate 验证记录的基本有效性
//
// 不包括签名验证，仅检查结构完整性。
func (r *AddressRecord) Validate() error {
	if r.NodeID.IsEmpty() {
		return errors.New("node ID is empty")
	}
	if len(r.Addresses) == 0 {
		return ErrEmptyAddresses
	}
	if r.IsExpired() {
		return ErrExpiredRecord
	}
	return nil
}

// ============================================================================
//                              辅助方法
// ============================================================================

// ExpiresAt 返回记录过期时间
func (r *AddressRecord) ExpiresAt() time.Time {
	if r.TTL == 0 {
		return time.Time{} // 永不过期
	}
	return r.Timestamp.Add(r.TTL)
}

// RemainingTTL 返回剩余有效期
func (r *AddressRecord) RemainingTTL() time.Duration {
	if r.TTL == 0 {
		return time.Duration(1<<63 - 1) // 最大 Duration
	}
	remaining := r.TTL - time.Since(r.Timestamp)
	if remaining < 0 {
		return 0
	}
	return remaining
}

// Clone 克隆记录
func (r *AddressRecord) Clone() *AddressRecord {
	clone := &AddressRecord{
		NodeID:    r.NodeID,
		RealmID:   r.RealmID,
		Sequence:  r.Sequence,
		Addresses: make([]endpoint.Address, len(r.Addresses)),
		Timestamp: r.Timestamp,
		TTL:       r.TTL,
		Signature: make([]byte, len(r.Signature)),
	}
	copy(clone.Addresses, r.Addresses)
	copy(clone.Signature, r.Signature)
	return clone
}

// IncrementSequence 递增序列号
//
// 在更新地址时调用，确保新记录的序列号大于旧记录。
func (r *AddressRecord) IncrementSequence() {
	r.Sequence++
	r.Timestamp = time.Now()
	r.Signature = nil // 清除旧签名
}

// UpdateAddresses 更新地址列表
//
// 自动递增序列号并清除签名（需要重新签名）。
func (r *AddressRecord) UpdateAddresses(addresses []endpoint.Address) {
	r.Addresses = addresses
	r.IncrementSequence()
}

