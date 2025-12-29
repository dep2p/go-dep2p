// Package types 定义 DeP2P 的基础类型
package types

import (
	"encoding/binary"
	"errors"
	"time"
)

// ============================================================================
//                              PSK 成员证明（IMPL-1227）
// ============================================================================

// MembershipProof PSK 成员证明
//
// 用于证明节点是 Realm 的合法成员（持有 realmKey）。
//
// 证明公式:
//
//	MAC = HMAC-SHA256(
//	    key  = HKDF(realmKey, "dep2p-realm-membership-v1"),
//	    data = nodeID || realmID || peerID || nonce || timestamp
//	)
//
// 字段说明:
//   - NodeID:    证明发起者（自己的 NodeID）
//   - RealmID:   所属 Realm 的 ID
//   - PeerID:    目标节点（通信对方的 NodeID）—— 绑定证明到特定目标
//   - Nonce:     随机数（防重放）
//   - Timestamp: 时间戳（限制有效期，通常 5 分钟窗口）
//   - MAC:       HMAC-SHA256 签名
//
// 设计决策（见 DISC-1227-api-layer-design.md）:
//   - PeerID 是目标节点，不是验证者
//   - 理由1：绑定目标 —— 证明含义为"我要与 PeerID 通信"
//   - 理由2：防中间人 —— R 无法将 A→B 的证明用于 A→C
//   - 理由3：双重验证 —— B 收到时验证 PeerID == 自己
type MembershipProof struct {
	// NodeID 证明发起者（自己的 NodeID）
	NodeID NodeID

	// RealmID 所属 Realm 的 ID
	RealmID RealmID

	// PeerID 目标节点（通信对方的 NodeID）
	// 绑定证明到特定目标，防止证明被重用于其他节点
	PeerID NodeID

	// Nonce 随机数（16字节）
	// 每次生成证明时随机产生，防止重放攻击
	Nonce [16]byte

	// Timestamp 时间戳（Unix 秒）
	// 用于限制证明有效期，通常 5 分钟窗口
	Timestamp int64

	// MAC HMAC-SHA256 签名（32字节）
	// 使用 HKDF(realmKey, "dep2p-realm-membership-v1") 派生的密钥计算
	MAC [32]byte
}

// MembershipProofSize 成员证明序列化后的固定大小
// = NodeID(32) + RealmID长度(2) + RealmID(变长) + PeerID(32) + Nonce(16) + Timestamp(8) + MAC(32)
// 最小: 32 + 2 + 0 + 32 + 16 + 8 + 32 = 122 字节
// 典型: 32 + 2 + 64 + 32 + 16 + 8 + 32 = 186 字节（RealmID 64字符）
const MembershipProofMinSize = 122

// 成员证明相关错误
var (
	// ErrProofExpired 证明已过期
	ErrProofExpired = errors.New("membership proof expired")

	// ErrInvalidProof 无效的证明（MAC 验证失败）
	ErrInvalidProof = errors.New("invalid membership proof")

	// ErrPeerIDMismatch 目标节点不匹配
	ErrPeerIDMismatch = errors.New("peer ID mismatch in proof")

	// ErrProofTooShort 证明数据太短
	ErrProofTooShort = errors.New("membership proof data too short")
)

// ProofValidityWindow 证明有效期窗口（秒）
// 默认 5 分钟，允许一定的时钟偏差
const ProofValidityWindow int64 = 300

// IsExpired 检查证明是否已过期
//
// 使用 5 分钟窗口，允许时钟前后偏差。
func (p *MembershipProof) IsExpired() bool {
	now := time.Now().Unix()
	diff := now - p.Timestamp
	if diff < 0 {
		diff = -diff
	}
	return diff > ProofValidityWindow
}

// Serialize 序列化证明为字节切片
//
// 格式:
//
//	NodeID(32) + RealmIDLen(2) + RealmID(变长) + PeerID(32) + Nonce(16) + Timestamp(8) + MAC(32)
func (p *MembershipProof) Serialize() []byte {
	realmIDBytes := []byte(p.RealmID)
	realmIDLen := len(realmIDBytes)

	// 计算总长度
	totalLen := 32 + 2 + realmIDLen + 32 + 16 + 8 + 32
	buf := make([]byte, totalLen)

	offset := 0

	// NodeID (32 bytes)
	copy(buf[offset:offset+32], p.NodeID[:])
	offset += 32

	// RealmID length (2 bytes, big endian)
	binary.BigEndian.PutUint16(buf[offset:offset+2], uint16(realmIDLen))
	offset += 2

	// RealmID (variable)
	copy(buf[offset:offset+realmIDLen], realmIDBytes)
	offset += realmIDLen

	// PeerID (32 bytes)
	copy(buf[offset:offset+32], p.PeerID[:])
	offset += 32

	// Nonce (16 bytes)
	copy(buf[offset:offset+16], p.Nonce[:])
	offset += 16

	// Timestamp (8 bytes, big endian)
	binary.BigEndian.PutUint64(buf[offset:offset+8], uint64(p.Timestamp))
	offset += 8

	// MAC (32 bytes)
	copy(buf[offset:offset+32], p.MAC[:])

	return buf
}

// DeserializeMembershipProof 从字节切片反序列化证明
func DeserializeMembershipProof(data []byte) (*MembershipProof, error) {
	if len(data) < MembershipProofMinSize {
		return nil, ErrProofTooShort
	}

	proof := &MembershipProof{}
	offset := 0

	// NodeID (32 bytes)
	copy(proof.NodeID[:], data[offset:offset+32])
	offset += 32

	// RealmID length (2 bytes)
	realmIDLen := int(binary.BigEndian.Uint16(data[offset : offset+2]))
	offset += 2

	// 检查剩余长度是否足够
	expectedRemaining := realmIDLen + 32 + 16 + 8 + 32
	if len(data)-offset < expectedRemaining {
		return nil, ErrProofTooShort
	}

	// RealmID (variable)
	proof.RealmID = RealmID(data[offset : offset+realmIDLen])
	offset += realmIDLen

	// PeerID (32 bytes)
	copy(proof.PeerID[:], data[offset:offset+32])
	offset += 32

	// Nonce (16 bytes)
	copy(proof.Nonce[:], data[offset:offset+16])
	offset += 16

	// Timestamp (8 bytes)
	proof.Timestamp = int64(binary.BigEndian.Uint64(data[offset : offset+8]))
	offset += 8

	// MAC (32 bytes)
	copy(proof.MAC[:], data[offset:offset+32])

	return proof, nil
}

// DataForMAC 返回用于 MAC 计算的数据
//
// 格式: nodeID || realmID || peerID || nonce || timestamp
//
// 注意：这不包括 MAC 字段本身。
func (p *MembershipProof) DataForMAC() []byte {
	realmIDBytes := []byte(p.RealmID)

	// 计算总长度
	totalLen := 32 + len(realmIDBytes) + 32 + 16 + 8
	buf := make([]byte, totalLen)

	offset := 0

	// NodeID (32 bytes)
	copy(buf[offset:offset+32], p.NodeID[:])
	offset += 32

	// RealmID (variable, no length prefix for MAC calculation)
	copy(buf[offset:offset+len(realmIDBytes)], realmIDBytes)
	offset += len(realmIDBytes)

	// PeerID (32 bytes)
	copy(buf[offset:offset+32], p.PeerID[:])
	offset += 32

	// Nonce (16 bytes)
	copy(buf[offset:offset+16], p.Nonce[:])
	offset += 16

	// Timestamp (8 bytes, big endian)
	binary.BigEndian.PutUint64(buf[offset:offset+8], uint64(p.Timestamp))

	return buf
}

