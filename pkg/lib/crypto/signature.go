// Package crypto 提供 DeP2P 密码学工具
package crypto

import (
	"crypto/sha256"
	"errors"
)

// Signature 签名结构
type Signature struct {
	// Type 签名使用的密钥类型
	Type KeyType

	// Data 签名数据
	Data []byte
}

// Sign 使用私钥签名数据
func Sign(key PrivateKey, data []byte) (*Signature, error) {
	if key == nil {
		return nil, errors.New("nil private key")
	}

	sig, err := key.Sign(data)
	if err != nil {
		return nil, err
	}

	return &Signature{
		Type: key.Type(),
		Data: sig,
	}, nil
}

// Verify 使用公钥验证签名
func Verify(key PublicKey, data []byte, sig *Signature) (bool, error) {
	if key == nil {
		return false, errors.New("nil public key")
	}
	if sig == nil {
		return false, errors.New("nil signature")
	}
	if key.Type() != sig.Type {
		return false, errors.New("key type mismatch")
	}

	return key.Verify(data, sig.Data)
}

// SignRecord 签名记录（带哈希）
type SignRecord struct {
	// PeerID 签名者 ID
	PeerID string

	// Seq 序列号
	Seq uint64

	// Data 原始数据
	Data []byte

	// Signature 签名
	Signature *Signature
}

// CreateSignedRecord 创建签名记录
func CreateSignedRecord(key PrivateKey, peerID string, seq uint64, data []byte) (*SignRecord, error) {
	// 创建要签名的数据：peerID + seq + data
	toSign := hashForSigning(peerID, seq, data)

	sig, err := Sign(key, toSign)
	if err != nil {
		return nil, err
	}

	return &SignRecord{
		PeerID:    peerID,
		Seq:       seq,
		Data:      data,
		Signature: sig,
	}, nil
}

// VerifySignedRecord 验证签名记录
func VerifySignedRecord(key PublicKey, record *SignRecord) (bool, error) {
	if record == nil {
		return false, errors.New("nil record")
	}

	toVerify := hashForSigning(record.PeerID, record.Seq, record.Data)
	return Verify(key, toVerify, record.Signature)
}

// hashForSigning 计算签名用的哈希
func hashForSigning(peerID string, seq uint64, data []byte) []byte {
	h := sha256.New()
	h.Write([]byte(peerID))
	h.Write(uint64ToBytes(seq))
	h.Write(data)
	return h.Sum(nil)
}

// uint64ToBytes 将 uint64 转换为字节
func uint64ToBytes(n uint64) []byte {
	b := make([]byte, 8)
	for i := 7; i >= 0; i-- {
		b[i] = byte(n & 0xff)
		n >>= 8
	}
	return b
}

// SignedEnvelope 签名信封
type SignedEnvelope struct {
	// PublicKey 签名者公钥
	PublicKey PublicKey

	// TypeHint 内容类型提示
	TypeHint []byte

	// Contents 信封内容
	Contents []byte

	// Signature 签名
	Signature *Signature
}

// Seal 创建签名信封
func Seal(key PrivateKey, typeHint, contents []byte) (*SignedEnvelope, error) {
	toSign := append(typeHint, contents...)
	sig, err := Sign(key, toSign)
	if err != nil {
		return nil, err
	}

	return &SignedEnvelope{
		PublicKey: key.GetPublic(),
		TypeHint:  typeHint,
		Contents:  contents,
		Signature: sig,
	}, nil
}

// Open 打开并验证签名信封
func (e *SignedEnvelope) Open() ([]byte, error) {
	toVerify := append(e.TypeHint, e.Contents...)
	valid, err := Verify(e.PublicKey, toVerify, e.Signature)
	if err != nil {
		return nil, err
	}
	if !valid {
		return nil, errors.New("invalid signature")
	}
	return e.Contents, nil
}
