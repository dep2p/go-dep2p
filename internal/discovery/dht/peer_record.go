// Package dht 提供分布式哈希表实现
//
// 本文件定义 DHT 专用的 PeerRecord 模型，用于权威地址目录。
package dht

import (
	"encoding/binary"
	"errors"
	"fmt"
	"time"

	"github.com/dep2p/go-dep2p/pkg/lib/crypto"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              常量定义
// ============================================================================

// PeerRecordPayloadType 信封载荷类型标识
var PeerRecordPayloadType = []byte("/dep2p/dht/peer-record/v1")

const (
	// DefaultPeerRecordTTL 默认 PeerRecord TTL（1 小时）
	DefaultPeerRecordTTL = 1 * time.Hour

	// MinPeerRecordTTL 最小 TTL（15 分钟，用于 NAT 频繁变化场景）
	MinPeerRecordTTL = 15 * time.Minute

	// MaxPeerRecordTTL 最大 TTL（24 小时）
	MaxPeerRecordTTL = 24 * time.Hour
)

// 注意：错误定义在 errors.go 中统一管理

// ============================================================================
//                              RealmPeerRecord 定义
// ============================================================================

// RealmPeerRecord DHT 专用的 Realm 节点记录
//
// 包含完整的地址信息、NAT 状态、能力和签名，用于 DHT 权威目录。
// 相比 protocol.go 中的 PeerRecord（用于消息传输），这个结构更完整。
type RealmPeerRecord struct {
	// NodeID 节点 ID
	NodeID types.NodeID

	// RealmID Realm 隔离边界
	RealmID types.RealmID

	// DirectAddrs 直连地址列表（经过 AutoNAT/dialback 验证）
	DirectAddrs []string

	// RelayAddrs 中继地址列表（稳定，主路径）
	RelayAddrs []string

	// NATType NAT 类型
	NATType types.NATType

	// Reachability 可达性状态
	Reachability types.Reachability

	// Capabilities 支持的能力（如 "relay", "dht-server"）
	Capabilities []string

	// Seq 递增序号（防重放）
	Seq uint64

	// Timestamp 创建时间戳（Unix 纳秒）
	Timestamp int64

	// TTL 存活时间（毫秒）
	TTL int64
}

// SignedRealmPeerRecord 签名的节点记录
type SignedRealmPeerRecord struct {
	// Record 节点记录
	Record *RealmPeerRecord

	// RawRecord 序列化的原始记录
	RawRecord []byte

	// PublicKey 签名者公钥
	PublicKey crypto.PublicKey

	// Signature 签名
	Signature []byte
}

// ============================================================================
//                              序列化
// ============================================================================

// Marshal 序列化 RealmPeerRecord
//
// 格式:
//
//	[nodeID_len(2) | nodeID |
//	 realmID_len(2) | realmID |
//	 direct_count(2) | direct_addrs... |
//	 relay_count(2) | relay_addrs... |
//	 nat_type(1) | reachability(1) |
//	 cap_count(2) | capabilities... |
//	 seq(8) | timestamp(8) | ttl(8)]
func (r *RealmPeerRecord) Marshal() ([]byte, error) {
	if r == nil {
		return nil, ErrNilPeerRecord
	}

	// 计算总大小
	nodeIDBytes := []byte(r.NodeID)
	realmIDBytes := []byte(r.RealmID)

	size := 2 + len(nodeIDBytes) // nodeID
	size += 2 + len(realmIDBytes) // realmID
	size += 2                     // direct_count
	for _, addr := range r.DirectAddrs {
		size += 2 + len(addr)
	}
	size += 2 // relay_count
	for _, addr := range r.RelayAddrs {
		size += 2 + len(addr)
	}
	size += 1 // nat_type
	size += 1 // reachability
	size += 2 // cap_count
	for _, cap := range r.Capabilities {
		size += 2 + len(cap)
	}
	size += 8 + 8 + 8 // seq + timestamp + ttl

	buf := make([]byte, size)
	offset := 0

	// NodeID
	binary.BigEndian.PutUint16(buf[offset:], uint16(len(nodeIDBytes)))
	offset += 2
	copy(buf[offset:], nodeIDBytes)
	offset += len(nodeIDBytes)

	// RealmID
	binary.BigEndian.PutUint16(buf[offset:], uint16(len(realmIDBytes)))
	offset += 2
	copy(buf[offset:], realmIDBytes)
	offset += len(realmIDBytes)

	// DirectAddrs
	binary.BigEndian.PutUint16(buf[offset:], uint16(len(r.DirectAddrs)))
	offset += 2
	for _, addr := range r.DirectAddrs {
		binary.BigEndian.PutUint16(buf[offset:], uint16(len(addr)))
		offset += 2
		copy(buf[offset:], addr)
		offset += len(addr)
	}

	// RelayAddrs
	binary.BigEndian.PutUint16(buf[offset:], uint16(len(r.RelayAddrs)))
	offset += 2
	for _, addr := range r.RelayAddrs {
		binary.BigEndian.PutUint16(buf[offset:], uint16(len(addr)))
		offset += 2
		copy(buf[offset:], addr)
		offset += len(addr)
	}

	// NATType
	buf[offset] = byte(r.NATType)
	offset++

	// Reachability
	buf[offset] = byte(r.Reachability)
	offset++

	// Capabilities
	binary.BigEndian.PutUint16(buf[offset:], uint16(len(r.Capabilities)))
	offset += 2
	for _, cap := range r.Capabilities {
		binary.BigEndian.PutUint16(buf[offset:], uint16(len(cap)))
		offset += 2
		copy(buf[offset:], cap)
		offset += len(cap)
	}

	// Seq
	binary.BigEndian.PutUint64(buf[offset:], r.Seq)
	offset += 8

	// Timestamp
	binary.BigEndian.PutUint64(buf[offset:], uint64(r.Timestamp))
	offset += 8

	// TTL
	binary.BigEndian.PutUint64(buf[offset:], uint64(r.TTL))

	return buf, nil
}

// UnmarshalRealmPeerRecord 反序列化 RealmPeerRecord
func UnmarshalRealmPeerRecord(data []byte) (*RealmPeerRecord, error) {
	if len(data) < 32 { // 最小长度
		return nil, errors.New("data too short")
	}

	offset := 0
	r := &RealmPeerRecord{}

	// NodeID
	if offset+2 > len(data) {
		return nil, errors.New("data too short for nodeID length")
	}
	nodeIDLen := int(binary.BigEndian.Uint16(data[offset:]))
	offset += 2
	if offset+nodeIDLen > len(data) {
		return nil, errors.New("invalid nodeID length")
	}
	r.NodeID = types.NodeID(data[offset : offset+nodeIDLen])
	offset += nodeIDLen

	// RealmID
	if offset+2 > len(data) {
		return nil, errors.New("data too short for realmID length")
	}
	realmIDLen := int(binary.BigEndian.Uint16(data[offset:]))
	offset += 2
	if offset+realmIDLen > len(data) {
		return nil, errors.New("invalid realmID length")
	}
	r.RealmID = types.RealmID(data[offset : offset+realmIDLen])
	offset += realmIDLen

	// DirectAddrs
	if offset+2 > len(data) {
		return nil, errors.New("data too short for direct_count")
	}
	directCount := int(binary.BigEndian.Uint16(data[offset:]))
	offset += 2
	r.DirectAddrs = make([]string, 0, directCount)
	for i := 0; i < directCount; i++ {
		if offset+2 > len(data) {
			return nil, errors.New("data too short for direct addr length")
		}
		addrLen := int(binary.BigEndian.Uint16(data[offset:]))
		offset += 2
		if offset+addrLen > len(data) {
			return nil, errors.New("invalid direct addr length")
		}
		r.DirectAddrs = append(r.DirectAddrs, string(data[offset:offset+addrLen]))
		offset += addrLen
	}

	// RelayAddrs
	if offset+2 > len(data) {
		return nil, errors.New("data too short for relay_count")
	}
	relayCount := int(binary.BigEndian.Uint16(data[offset:]))
	offset += 2
	r.RelayAddrs = make([]string, 0, relayCount)
	for i := 0; i < relayCount; i++ {
		if offset+2 > len(data) {
			return nil, errors.New("data too short for relay addr length")
		}
		addrLen := int(binary.BigEndian.Uint16(data[offset:]))
		offset += 2
		if offset+addrLen > len(data) {
			return nil, errors.New("invalid relay addr length")
		}
		r.RelayAddrs = append(r.RelayAddrs, string(data[offset:offset+addrLen]))
		offset += addrLen
	}

	// NATType
	if offset+1 > len(data) {
		return nil, errors.New("data too short for nat_type")
	}
	r.NATType = types.NATType(data[offset])
	offset++

	// Reachability
	if offset+1 > len(data) {
		return nil, errors.New("data too short for reachability")
	}
	r.Reachability = types.Reachability(data[offset])
	offset++

	// Capabilities
	if offset+2 > len(data) {
		return nil, errors.New("data too short for cap_count")
	}
	capCount := int(binary.BigEndian.Uint16(data[offset:]))
	offset += 2
	r.Capabilities = make([]string, 0, capCount)
	for i := 0; i < capCount; i++ {
		if offset+2 > len(data) {
			return nil, errors.New("data too short for capability length")
		}
		capLen := int(binary.BigEndian.Uint16(data[offset:]))
		offset += 2
		if offset+capLen > len(data) {
			return nil, errors.New("invalid capability length")
		}
		r.Capabilities = append(r.Capabilities, string(data[offset:offset+capLen]))
		offset += capLen
	}

	// Seq
	if offset+8 > len(data) {
		return nil, errors.New("data too short for seq")
	}
	r.Seq = binary.BigEndian.Uint64(data[offset:])
	offset += 8

	// Timestamp
	if offset+8 > len(data) {
		return nil, errors.New("data too short for timestamp")
	}
	r.Timestamp = int64(binary.BigEndian.Uint64(data[offset:]))
	offset += 8

	// TTL
	if offset+8 > len(data) {
		return nil, errors.New("data too short for ttl")
	}
	r.TTL = int64(binary.BigEndian.Uint64(data[offset:]))

	return r, nil
}

// ============================================================================
//                              签名/验证
// ============================================================================

// SignRealmPeerRecord 签名节点记录
func SignRealmPeerRecord(privKey crypto.PrivateKey, record *RealmPeerRecord) (*SignedRealmPeerRecord, error) {
	if privKey == nil {
		return nil, errors.New("nil private key")
	}
	if record == nil {
		return nil, ErrNilPeerRecord
	}
	if record.Seq == 0 {
		return nil, ErrInvalidSeq
	}

	// 序列化记录
	rawRecord, err := record.Marshal()
	if err != nil {
		return nil, fmt.Errorf("failed to marshal record: %w", err)
	}

	// 创建签名数据（类型 + 内容）
	toSign := append(PeerRecordPayloadType, rawRecord...)

	// 签名
	sig, err := privKey.Sign(toSign)
	if err != nil {
		return nil, fmt.Errorf("failed to sign: %w", err)
	}

	return &SignedRealmPeerRecord{
		Record:    record,
		RawRecord: rawRecord,
		PublicKey: privKey.GetPublic(),
		Signature: sig,
	}, nil
}

// VerifySignedRealmPeerRecord 验证签名的节点记录
func VerifySignedRealmPeerRecord(signed *SignedRealmPeerRecord) error {
	if signed == nil {
		return errors.New("nil signed record")
	}
	if signed.PublicKey == nil {
		return errors.New("nil public key")
	}
	if len(signed.RawRecord) == 0 {
		return errors.New("empty raw record")
	}
	if len(signed.Signature) == 0 {
		return errors.New("empty signature")
	}

	// 重建签名数据
	toVerify := append(PeerRecordPayloadType, signed.RawRecord...)

	// 验证签名
	valid, err := signed.PublicKey.Verify(toVerify, signed.Signature)
	if err != nil {
		return fmt.Errorf("verification error: %w", err)
	}
	if !valid {
		return ErrInvalidSignature
	}

	return nil
}

// ============================================================================
//                              序列化 SignedRealmPeerRecord
// ============================================================================

// Marshal 序列化 SignedRealmPeerRecord
func (s *SignedRealmPeerRecord) Marshal() ([]byte, error) {
	if s == nil {
		return nil, errors.New("nil signed peer record")
	}

	// 序列化公钥
	pubKeyBytes, err := s.PublicKey.Raw()
	if err != nil {
		return nil, fmt.Errorf("failed to marshal public key: %w", err)
	}
	keyType := byte(s.PublicKey.Type())

	// 格式: [keyType(1) | pubKey_len(2) | pubKey | record_len(2) | record | sig_len(2) | sig]
	size := 1 + 2 + len(pubKeyBytes) + 2 + len(s.RawRecord) + 2 + len(s.Signature)
	buf := make([]byte, size)
	offset := 0

	// Key type
	buf[offset] = keyType
	offset++

	// Public key
	binary.BigEndian.PutUint16(buf[offset:], uint16(len(pubKeyBytes)))
	offset += 2
	copy(buf[offset:], pubKeyBytes)
	offset += len(pubKeyBytes)

	// Raw record
	binary.BigEndian.PutUint16(buf[offset:], uint16(len(s.RawRecord)))
	offset += 2
	copy(buf[offset:], s.RawRecord)
	offset += len(s.RawRecord)

	// Signature
	binary.BigEndian.PutUint16(buf[offset:], uint16(len(s.Signature)))
	offset += 2
	copy(buf[offset:], s.Signature)

	return buf, nil
}

// UnmarshalSignedRealmPeerRecord 反序列化 SignedRealmPeerRecord
func UnmarshalSignedRealmPeerRecord(data []byte) (*SignedRealmPeerRecord, error) {
	if len(data) < 7 { // 最小: 1 + 2 + 0 + 2 + 0 + 2 + 0
		return nil, errors.New("data too short")
	}

	offset := 0

	// Key type
	keyType := crypto.KeyType(data[offset])
	offset++

	// Public key
	pubKeyLen := int(binary.BigEndian.Uint16(data[offset:]))
	offset += 2
	if offset+pubKeyLen > len(data) {
		return nil, errors.New("invalid pubKey length")
	}
	pubKeyBytes := data[offset : offset+pubKeyLen]
	offset += pubKeyLen

	pubKey, err := crypto.UnmarshalPublicKey(keyType, pubKeyBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal public key: %w", err)
	}

	// Raw record
	if offset+2 > len(data) {
		return nil, errors.New("data too short for record length")
	}
	recordLen := int(binary.BigEndian.Uint16(data[offset:]))
	offset += 2
	if offset+recordLen > len(data) {
		return nil, errors.New("invalid record length")
	}
	rawRecord := data[offset : offset+recordLen]
	offset += recordLen

	// Signature
	if offset+2 > len(data) {
		return nil, errors.New("data too short for sig length")
	}
	sigLen := int(binary.BigEndian.Uint16(data[offset:]))
	offset += 2
	if offset+sigLen > len(data) {
		return nil, errors.New("invalid sig length")
	}
	sig := data[offset : offset+sigLen]

	// 反序列化 RealmPeerRecord
	record, err := UnmarshalRealmPeerRecord(rawRecord)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal peer record: %w", err)
	}

	return &SignedRealmPeerRecord{
		Record:    record,
		RawRecord: rawRecord,
		PublicKey: pubKey,
		Signature: sig,
	}, nil
}

// ============================================================================
//                              辅助方法
// ============================================================================

// IsExpired 检查记录是否已过期
func (r *RealmPeerRecord) IsExpired() bool {
	if r == nil {
		return true
	}
	expiryTime := time.Unix(0, r.Timestamp).Add(time.Duration(r.TTL) * time.Millisecond)
	return time.Now().After(expiryTime)
}

// ExpiryTime 返回过期时间
func (r *RealmPeerRecord) ExpiryTime() time.Time {
	if r == nil {
		return time.Time{}
	}
	return time.Unix(0, r.Timestamp).Add(time.Duration(r.TTL) * time.Millisecond)
}

// AllAddrs 返回所有地址（直连 + 中继）
func (r *RealmPeerRecord) AllAddrs() []string {
	if r == nil {
		return nil
	}
	addrs := make([]string, 0, len(r.DirectAddrs)+len(r.RelayAddrs))
	addrs = append(addrs, r.DirectAddrs...)
	addrs = append(addrs, r.RelayAddrs...)
	return addrs
}

// HasCapability 检查是否支持某能力
func (r *RealmPeerRecord) HasCapability(cap string) bool {
	if r == nil {
		return false
	}
	for _, c := range r.Capabilities {
		if c == cap {
			return true
		}
	}
	return false
}
