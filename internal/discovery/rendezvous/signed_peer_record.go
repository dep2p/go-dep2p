package rendezvous

import (
	"encoding/binary"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/dep2p/go-dep2p/pkg/lib/crypto"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              SignedPeerRecord 定义
// ============================================================================

// PeerRecordEnvelopePayloadType 信封载荷类型标识
var PeerRecordEnvelopePayloadType = []byte("/dep2p/peer-record")

// PeerRecord 节点记录
type PeerRecord struct {
	// PeerID 节点 ID
	PeerID types.PeerID

	// Addrs 节点地址列表
	Addrs []types.Multiaddr

	// Seq 序列号（单调递增，用于版本控制）
	Seq uint64

	// Timestamp 记录创建时间
	Timestamp time.Time
}

// SignedPeerRecord 签名的节点记录
type SignedPeerRecord struct {
	// PeerRecord 节点记录
	PeerRecord *PeerRecord

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

// Marshal 序列化 PeerRecord
func (r *PeerRecord) Marshal() ([]byte, error) {
	if r == nil {
		return nil, errors.New("nil peer record")
	}

	// 计算总大小
	peerIDBytes := []byte(r.PeerID)

	// 序列化地址（预分配）
	addrsBytes := make([][]byte, 0, len(r.Addrs))
	for _, addr := range r.Addrs {
		addrsBytes = append(addrsBytes, []byte(addr.String()))
	}

	// 格式: [peerID_len(2) | peerID | seq(8) | timestamp(8) | addrs_count(2) | addrs...]
	size := 2 + len(peerIDBytes) + 8 + 8 + 2
	for _, ab := range addrsBytes {
		size += 2 + len(ab)
	}

	buf := make([]byte, size)
	offset := 0

	// PeerID
	binary.BigEndian.PutUint16(buf[offset:], uint16(len(peerIDBytes)))
	offset += 2
	copy(buf[offset:], peerIDBytes)
	offset += len(peerIDBytes)

	// Seq
	binary.BigEndian.PutUint64(buf[offset:], r.Seq)
	offset += 8

	// Timestamp
	binary.BigEndian.PutUint64(buf[offset:], uint64(r.Timestamp.UnixNano()))
	offset += 8

	// Addrs count
	binary.BigEndian.PutUint16(buf[offset:], uint16(len(addrsBytes)))
	offset += 2

	// Addrs
	for _, ab := range addrsBytes {
		binary.BigEndian.PutUint16(buf[offset:], uint16(len(ab)))
		offset += 2
		copy(buf[offset:], ab)
		offset += len(ab)
	}

	return buf, nil
}

// UnmarshalPeerRecord 反序列化 PeerRecord
func UnmarshalPeerRecord(data []byte) (*PeerRecord, error) {
	if len(data) < 20 { // 最小: 2 + 0 + 8 + 8 + 2
		return nil, errors.New("data too short")
	}

	offset := 0

	// PeerID
	peerIDLen := int(binary.BigEndian.Uint16(data[offset:]))
	offset += 2
	if offset+peerIDLen > len(data) {
		return nil, errors.New("invalid peerID length")
	}
	peerID := types.PeerID(data[offset : offset+peerIDLen])
	offset += peerIDLen

	// Seq
	if offset+8 > len(data) {
		return nil, errors.New("data too short for seq")
	}
	seq := binary.BigEndian.Uint64(data[offset:])
	offset += 8

	// Timestamp
	if offset+8 > len(data) {
		return nil, errors.New("data too short for timestamp")
	}
	timestampNano := int64(binary.BigEndian.Uint64(data[offset:]))
	timestamp := time.Unix(0, timestampNano)
	offset += 8

	// Addrs count
	if offset+2 > len(data) {
		return nil, errors.New("data too short for addrs count")
	}
	addrsCount := int(binary.BigEndian.Uint16(data[offset:]))
	offset += 2

	// Addrs
	addrs := make([]types.Multiaddr, 0, addrsCount)
	for i := 0; i < addrsCount; i++ {
		if offset+2 > len(data) {
			return nil, errors.New("data too short for addr length")
		}
		addrLen := int(binary.BigEndian.Uint16(data[offset:]))
		offset += 2
		if offset+addrLen > len(data) {
			return nil, errors.New("data too short for addr")
		}
		addrStr := string(data[offset : offset+addrLen])
		offset += addrLen

		ma, err := types.NewMultiaddr(addrStr)
		if err == nil {
			addrs = append(addrs, ma)
		}
	}

	return &PeerRecord{
		PeerID:    peerID,
		Addrs:     addrs,
		Seq:       seq,
		Timestamp: timestamp,
	}, nil
}

// ============================================================================
//                              签名/验证
// ============================================================================

// SignPeerRecord 签名节点记录
func SignPeerRecord(privKey crypto.PrivateKey, record *PeerRecord) (*SignedPeerRecord, error) {
	if privKey == nil {
		return nil, errors.New("nil private key")
	}
	if record == nil {
		return nil, errors.New("nil peer record")
	}

	// 序列化记录
	rawRecord, err := record.Marshal()
	if err != nil {
		return nil, fmt.Errorf("failed to marshal record: %w", err)
	}

	// 创建签名数据（类型 + 内容）
	toSign := append(PeerRecordEnvelopePayloadType, rawRecord...)

	// 签名
	sig, err := privKey.Sign(toSign)
	if err != nil {
		return nil, fmt.Errorf("failed to sign: %w", err)
	}

	return &SignedPeerRecord{
		PeerRecord: record,
		RawRecord:  rawRecord,
		PublicKey:  privKey.GetPublic(),
		Signature:  sig,
	}, nil
}

// VerifySignedPeerRecord 验证签名的节点记录
func VerifySignedPeerRecord(signed *SignedPeerRecord) (bool, error) {
	if signed == nil {
		return false, errors.New("nil signed record")
	}
	if signed.PublicKey == nil {
		return false, errors.New("nil public key")
	}
	if len(signed.RawRecord) == 0 {
		return false, errors.New("empty raw record")
	}
	if len(signed.Signature) == 0 {
		return false, errors.New("empty signature")
	}

	// 重建签名数据
	toVerify := append(PeerRecordEnvelopePayloadType, signed.RawRecord...)

	// 验证签名
	return signed.PublicKey.Verify(toVerify, signed.Signature)
}

// ============================================================================
//                              序列化 SignedPeerRecord
// ============================================================================

// Marshal 序列化 SignedPeerRecord
func (s *SignedPeerRecord) Marshal() ([]byte, error) {
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

// UnmarshalSignedPeerRecord 反序列化 SignedPeerRecord
func UnmarshalSignedPeerRecord(data []byte) (*SignedPeerRecord, error) {
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

	// 反序列化 PeerRecord
	peerRecord, err := UnmarshalPeerRecord(rawRecord)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal peer record: %w", err)
	}

	return &SignedPeerRecord{
		PeerRecord: peerRecord,
		RawRecord:  rawRecord,
		PublicKey:  pubKey,
		Signature:  sig,
	}, nil
}

// ============================================================================
//                              PeerRecordManager 管理器
// ============================================================================

// PeerRecordManager 管理本地 PeerRecord 的序列号
type PeerRecordManager struct {
	privKey crypto.PrivateKey
	peerID  types.PeerID
	seq     uint64
}

// NewPeerRecordManager 创建 PeerRecord 管理器
func NewPeerRecordManager(privKey crypto.PrivateKey, peerID types.PeerID) *PeerRecordManager {
	return &PeerRecordManager{
		privKey: privKey,
		peerID:  peerID,
		seq:     0,
	}
}

// CreateSignedRecord 创建并签名新的 PeerRecord
func (m *PeerRecordManager) CreateSignedRecord(addrs []types.Multiaddr) (*SignedPeerRecord, error) {
	// 递增序列号
	seq := atomic.AddUint64(&m.seq, 1)

	record := &PeerRecord{
		PeerID:    m.peerID,
		Addrs:     addrs,
		Seq:       seq,
		Timestamp: time.Now(),
	}

	return SignPeerRecord(m.privKey, record)
}

// ============================================================================
//                              辅助函数
// ============================================================================

// ExtractPeerIDFromSignedRecord 从 SignedPeerRecord 提取 PeerID
func ExtractPeerIDFromSignedRecord(data []byte) (types.PeerID, error) {
	signed, err := UnmarshalSignedPeerRecord(data)
	if err != nil {
		return "", err
	}

	// 验证签名
	valid, err := VerifySignedPeerRecord(signed)
	if err != nil {
		return "", fmt.Errorf("verification failed: %w", err)
	}
	if !valid {
		return "", errors.New("invalid signature")
	}

	return signed.PeerRecord.PeerID, nil
}

// ExtractPeerInfoFromSignedRecord 从 SignedPeerRecord 提取 PeerInfo
func ExtractPeerInfoFromSignedRecord(data []byte) (types.PeerInfo, error) {
	signed, err := UnmarshalSignedPeerRecord(data)
	if err != nil {
		return types.PeerInfo{}, err
	}

	// 验证签名
	valid, err := VerifySignedPeerRecord(signed)
	if err != nil {
		return types.PeerInfo{}, fmt.Errorf("verification failed: %w", err)
	}
	if !valid {
		return types.PeerInfo{}, errors.New("invalid signature")
	}

	return types.PeerInfo{
		ID:    signed.PeerRecord.PeerID,
		Addrs: signed.PeerRecord.Addrs,
	}, nil
}
