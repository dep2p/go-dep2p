// Package identity 实现身份管理
//
// DeviceIdentity 提供设备身份管理功能：
//   - 设备证书的创建和验证
//   - 设备与节点身份的绑定
//   - 设备证书的持久化存储
package identity

import (
	"bytes"
	"crypto/ed25519"
	"encoding/binary"
	"errors"
	"fmt"
	"sync"
	"time"

	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/lib/log"
	"github.com/dep2p/go-dep2p/pkg/types"
)

var deviceLogger = log.Logger("identity/device")

// ============================================================================
//                              错误定义
// ============================================================================

var (
	// ErrInvalidDeviceCert 无效的设备证书
	ErrInvalidDeviceCert = errors.New("invalid device certificate")

	// ErrDeviceCertExpired 设备证书已过期
	ErrDeviceCertExpired = errors.New("device certificate expired")

	// ErrDeviceNotBound 设备未绑定
	ErrDeviceNotBound = errors.New("device not bound to peer")

	// ErrDeviceAlreadyBound 设备已绑定
	ErrDeviceAlreadyBound = errors.New("device already bound")

	// ErrInvalidDeviceSignature 无效的设备签名
	ErrInvalidDeviceSignature = errors.New("invalid device signature")

	// ErrDeviceStoreClosed 设备存储已关闭
	ErrDeviceStoreClosed = errors.New("device store closed")
)

// ============================================================================
//                              常量定义
// ============================================================================

const (
	// DeviceCertVersion 设备证书版本
	DeviceCertVersion = 1

	// DefaultDeviceCertValidity 默认设备证书有效期（1年）
	DefaultDeviceCertValidity = 365 * 24 * time.Hour

	// DeviceSignaturePrefix 签名前缀
	DeviceSignaturePrefix = "dep2p-device-cert-v1:"

	// MinDeviceCertSize 最小证书大小
	MinDeviceCertSize = 1 + 8 + 8 + 32 + 64
)

// ============================================================================
//                              DeviceCertificate 结构
// ============================================================================

// DeviceCertificate 设备证书
//
// 设备证书用于标识和验证一个物理设备的身份。
// 每个设备可以绑定到一个节点身份。
type DeviceCertificate struct {
	// Version 证书版本
	Version uint8

	// DeviceID 设备唯一标识
	DeviceID string

	// PeerID 绑定的节点 ID
	PeerID types.PeerID

	// PublicKey 设备公钥
	PublicKey []byte

	// IssuedAt 签发时间
	IssuedAt int64

	// ExpiresAt 过期时间
	ExpiresAt int64

	// Signature 签名（由设备私钥签名）
	Signature []byte

	// Metadata 元数据
	Metadata map[string]string
}

// IsExpired 检查证书是否过期
func (c *DeviceCertificate) IsExpired() bool {
	return time.Now().Unix() > c.ExpiresAt
}

// IsValid 检查证书是否有效
func (c *DeviceCertificate) IsValid() bool {
	if c.Version != DeviceCertVersion {
		return false
	}
	if c.IsExpired() {
		return false
	}
	if len(c.PublicKey) != ed25519.PublicKeySize {
		return false
	}
	if len(c.Signature) != ed25519.SignatureSize {
		return false
	}
	return true
}

// ============================================================================
//                              DeviceIdentity 结构
// ============================================================================

// DeviceIdentity 设备身份管理器
//
// 管理设备证书的创建、验证和存储。
type DeviceIdentity struct {
	// 设备密钥对
	privateKey pkgif.PrivateKey
	publicKey  pkgif.PublicKey

	// 设备 ID
	deviceID string

	// 当前证书
	certificate *DeviceCertificate
	certMu      sync.RWMutex

	// 绑定的节点身份
	peerIdentity pkgif.Identity
	peerMu       sync.RWMutex

	// 证书有效期
	certValidity time.Duration
}

// DeviceIdentityConfig 设备身份配置
type DeviceIdentityConfig struct {
	// DeviceID 设备 ID（如果为空则自动生成）
	DeviceID string

	// CertValidity 证书有效期
	CertValidity time.Duration
}

// DefaultDeviceIdentityConfig 返回默认配置
func DefaultDeviceIdentityConfig() DeviceIdentityConfig {
	return DeviceIdentityConfig{
		CertValidity: DefaultDeviceCertValidity,
	}
}

// ============================================================================
//                              构造函数
// ============================================================================

// NewDeviceIdentity 创建设备身份
func NewDeviceIdentity(config DeviceIdentityConfig) (*DeviceIdentity, error) {
	// 生成设备密钥对
	privKey, pubKey, err := GenerateEd25519Key()
	if err != nil {
		return nil, fmt.Errorf("generate device key: %w", err)
	}

	// 设备 ID
	deviceID := config.DeviceID
	if deviceID == "" {
		// 从公钥派生设备 ID
		pubKeyBytes, _ := pubKey.Raw()
		deviceID = fmt.Sprintf("device-%x", pubKeyBytes[:8])
	}

	// 证书有效期
	validity := config.CertValidity
	if validity <= 0 {
		validity = DefaultDeviceCertValidity
	}

	di := &DeviceIdentity{
		privateKey:   privKey,
		publicKey:    pubKey,
		deviceID:     deviceID,
		certValidity: validity,
	}

	deviceLogger.Info("设备身份已创建",
		"deviceID", deviceID,
		"certValidity", validity)

	return di, nil
}

// NewDeviceIdentityFromKey 从现有密钥创建设备身份
func NewDeviceIdentityFromKey(privKey pkgif.PrivateKey, config DeviceIdentityConfig) (*DeviceIdentity, error) {
	if privKey == nil {
		return nil, ErrNilPrivateKey
	}

	pubKey := privKey.PublicKey()

	deviceID := config.DeviceID
	if deviceID == "" {
		pubKeyBytes, _ := pubKey.Raw()
		deviceID = fmt.Sprintf("device-%x", pubKeyBytes[:8])
	}

	validity := config.CertValidity
	if validity <= 0 {
		validity = DefaultDeviceCertValidity
	}

	return &DeviceIdentity{
		privateKey:   privKey,
		publicKey:    pubKey,
		deviceID:     deviceID,
		certValidity: validity,
	}, nil
}

// ============================================================================
//                              绑定管理
// ============================================================================

// BindToPeer 绑定到节点身份
func (di *DeviceIdentity) BindToPeer(peerIdentity pkgif.Identity) error {
	di.peerMu.Lock()
	defer di.peerMu.Unlock()

	if di.peerIdentity != nil {
		return ErrDeviceAlreadyBound
	}

	di.peerIdentity = peerIdentity

	// 创建新证书
	cert, err := di.createCertificate(peerIdentity.PeerID())
	if err != nil {
		di.peerIdentity = nil
		return err
	}

	di.certMu.Lock()
	di.certificate = cert
	di.certMu.Unlock()

	deviceLogger.Info("设备已绑定到节点",
		"deviceID", di.deviceID,
		"peerID", peerIdentity.PeerID())

	return nil
}

// Unbind 解除绑定
func (di *DeviceIdentity) Unbind() {
	di.peerMu.Lock()
	di.peerIdentity = nil
	di.peerMu.Unlock()

	di.certMu.Lock()
	di.certificate = nil
	di.certMu.Unlock()

	deviceLogger.Info("设备已解除绑定", "deviceID", di.deviceID)
}

// IsBound 检查是否已绑定
func (di *DeviceIdentity) IsBound() bool {
	di.peerMu.RLock()
	defer di.peerMu.RUnlock()
	return di.peerIdentity != nil
}

// BoundPeerID 返回绑定的节点 ID
func (di *DeviceIdentity) BoundPeerID() (types.PeerID, bool) {
	di.peerMu.RLock()
	defer di.peerMu.RUnlock()

	if di.peerIdentity == nil {
		return "", false
	}
	return types.PeerID(di.peerIdentity.PeerID()), true
}

// ============================================================================
//                              证书管理
// ============================================================================

// createCertificate 创建设备证书
func (di *DeviceIdentity) createCertificate(peerID string) (*DeviceCertificate, error) {
	pubKeyBytes, err := di.publicKey.Raw()
	if err != nil {
		return nil, err
	}

	now := time.Now()
	cert := &DeviceCertificate{
		Version:   DeviceCertVersion,
		DeviceID:  di.deviceID,
		PeerID:    types.PeerID(peerID),
		PublicKey: pubKeyBytes,
		IssuedAt:  now.Unix(),
		ExpiresAt: now.Add(di.certValidity).Unix(),
		Metadata:  make(map[string]string),
	}

	// 签名证书
	sigData := cert.signatureData()
	privKeyBytes, err := di.privateKey.Raw()
	if err != nil {
		return nil, err
	}

	cert.Signature = ed25519.Sign(privKeyBytes, sigData)

	return cert, nil
}

// Certificate 返回当前证书
func (di *DeviceIdentity) Certificate() *DeviceCertificate {
	di.certMu.RLock()
	defer di.certMu.RUnlock()
	return di.certificate
}

// RenewCertificate 续期证书
func (di *DeviceIdentity) RenewCertificate() error {
	di.peerMu.RLock()
	peerIdentity := di.peerIdentity
	di.peerMu.RUnlock()

	if peerIdentity == nil {
		return ErrDeviceNotBound
	}

	cert, err := di.createCertificate(peerIdentity.PeerID())
	if err != nil {
		return err
	}

	di.certMu.Lock()
	di.certificate = cert
	di.certMu.Unlock()

	deviceLogger.Info("证书已续期",
		"deviceID", di.deviceID,
		"expiresAt", time.Unix(cert.ExpiresAt, 0))

	return nil
}

// ============================================================================
//                              证书验证
// ============================================================================

// VerifyCertificate 验证设备证书
func VerifyCertificate(cert *DeviceCertificate) error {
	if cert == nil {
		return ErrInvalidDeviceCert
	}

	// 检查版本
	if cert.Version != DeviceCertVersion {
		return fmt.Errorf("%w: unsupported version %d", ErrInvalidDeviceCert, cert.Version)
	}

	// 检查过期
	if cert.IsExpired() {
		return ErrDeviceCertExpired
	}

	// 检查公钥
	if len(cert.PublicKey) != ed25519.PublicKeySize {
		return fmt.Errorf("%w: invalid public key length", ErrInvalidDeviceCert)
	}

	// 检查签名
	if len(cert.Signature) != ed25519.SignatureSize {
		return fmt.Errorf("%w: invalid signature length", ErrInvalidDeviceCert)
	}

	// 验证签名
	sigData := cert.signatureData()
	if !ed25519.Verify(cert.PublicKey, sigData, cert.Signature) {
		return ErrInvalidDeviceSignature
	}

	return nil
}

// signatureData 生成签名数据
func (c *DeviceCertificate) signatureData() []byte {
	var buf bytes.Buffer

	// 前缀
	buf.WriteString(DeviceSignaturePrefix)

	// 版本
	buf.WriteByte(c.Version)

	// 设备 ID
	buf.WriteString(c.DeviceID)

	// 节点 ID
	buf.WriteString(string(c.PeerID))

	// 公钥
	buf.Write(c.PublicKey)

	// 签发时间
	ts := make([]byte, 8)
	binary.BigEndian.PutUint64(ts, uint64(c.IssuedAt))
	buf.Write(ts)

	// 过期时间
	binary.BigEndian.PutUint64(ts, uint64(c.ExpiresAt))
	buf.Write(ts)

	return buf.Bytes()
}

// ============================================================================
//                              序列化
// ============================================================================

// Marshal 序列化证书
func (c *DeviceCertificate) Marshal() ([]byte, error) {
	var buf bytes.Buffer

	// 版本 (1 字节)
	buf.WriteByte(c.Version)

	// 设备 ID
	deviceIDBytes := []byte(c.DeviceID)
	deviceIDLen := make([]byte, 2)
	binary.BigEndian.PutUint16(deviceIDLen, uint16(len(deviceIDBytes)))
	buf.Write(deviceIDLen)
	buf.Write(deviceIDBytes)

	// 节点 ID
	peerIDBytes := []byte(c.PeerID)
	peerIDLen := make([]byte, 2)
	binary.BigEndian.PutUint16(peerIDLen, uint16(len(peerIDBytes)))
	buf.Write(peerIDLen)
	buf.Write(peerIDBytes)

	// 公钥
	pubKeyLen := make([]byte, 2)
	binary.BigEndian.PutUint16(pubKeyLen, uint16(len(c.PublicKey)))
	buf.Write(pubKeyLen)
	buf.Write(c.PublicKey)

	// 时间戳 (8 字节)
	ts := make([]byte, 8)
	binary.BigEndian.PutUint64(ts, uint64(c.IssuedAt))
	buf.Write(ts)
	binary.BigEndian.PutUint64(ts, uint64(c.ExpiresAt))
	buf.Write(ts)

	// 签名
	sigLen := make([]byte, 2)
	binary.BigEndian.PutUint16(sigLen, uint16(len(c.Signature)))
	buf.Write(sigLen)
	buf.Write(c.Signature)

	return buf.Bytes(), nil
}

// UnmarshalDeviceCertificate 反序列化证书
func UnmarshalDeviceCertificate(data []byte) (*DeviceCertificate, error) {
	if len(data) < MinDeviceCertSize {
		return nil, fmt.Errorf("%w: data too short", ErrInvalidDeviceCert)
	}

	c := &DeviceCertificate{
		Metadata: make(map[string]string),
	}
	offset := 0

	// 版本
	c.Version = data[offset]
	offset++

	// 设备 ID
	if offset+2 > len(data) {
		return nil, ErrInvalidDeviceCert
	}
	deviceIDLen := int(binary.BigEndian.Uint16(data[offset : offset+2]))
	offset += 2
	if offset+deviceIDLen > len(data) {
		return nil, ErrInvalidDeviceCert
	}
	c.DeviceID = string(data[offset : offset+deviceIDLen])
	offset += deviceIDLen

	// 节点 ID
	if offset+2 > len(data) {
		return nil, ErrInvalidDeviceCert
	}
	peerIDLen := int(binary.BigEndian.Uint16(data[offset : offset+2]))
	offset += 2
	if offset+peerIDLen > len(data) {
		return nil, ErrInvalidDeviceCert
	}
	c.PeerID = types.PeerID(data[offset : offset+peerIDLen])
	offset += peerIDLen

	// 公钥
	if offset+2 > len(data) {
		return nil, ErrInvalidDeviceCert
	}
	pubKeyLen := int(binary.BigEndian.Uint16(data[offset : offset+2]))
	offset += 2
	if offset+pubKeyLen > len(data) {
		return nil, ErrInvalidDeviceCert
	}
	c.PublicKey = make([]byte, pubKeyLen)
	copy(c.PublicKey, data[offset:offset+pubKeyLen])
	offset += pubKeyLen

	// 时间戳
	if offset+16 > len(data) {
		return nil, ErrInvalidDeviceCert
	}
	c.IssuedAt = int64(binary.BigEndian.Uint64(data[offset : offset+8]))
	offset += 8
	c.ExpiresAt = int64(binary.BigEndian.Uint64(data[offset : offset+8]))
	offset += 8

	// 签名
	if offset+2 > len(data) {
		return nil, ErrInvalidDeviceCert
	}
	sigLen := int(binary.BigEndian.Uint16(data[offset : offset+2]))
	offset += 2
	if offset+sigLen > len(data) {
		return nil, ErrInvalidDeviceCert
	}
	c.Signature = make([]byte, sigLen)
	copy(c.Signature, data[offset:offset+sigLen])

	return c, nil
}

// ============================================================================
//                              辅助方法
// ============================================================================

// DeviceID 返回设备 ID
func (di *DeviceIdentity) DeviceID() string {
	return di.deviceID
}

// PublicKey 返回设备公钥
func (di *DeviceIdentity) PublicKey() pkgif.PublicKey {
	return di.publicKey
}

// CertificateBytes 返回证书字节
func (di *DeviceIdentity) CertificateBytes() ([]byte, error) {
	di.certMu.RLock()
	cert := di.certificate
	di.certMu.RUnlock()

	if cert == nil {
		return nil, ErrDeviceNotBound
	}

	return cert.Marshal()
}
