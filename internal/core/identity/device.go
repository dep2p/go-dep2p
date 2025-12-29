// Package identity 提供身份管理模块的实现
//
// 设备身份支持：
// - 主从身份模型
// - 设备证书签发
// - 设备撤销机制
package identity

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"sync"
	"time"

	"github.com/dep2p/go-dep2p/internal/util/logger"
	identityif "github.com/dep2p/go-dep2p/pkg/interfaces/identity"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// 包级别日志实例
var log = logger.Logger("identity.device")

// ============================================================================
//                              错误定义
// ============================================================================

// 设备管理相关错误
var (
	// ErrDeviceAlreadyExists 设备已存在
	ErrDeviceAlreadyExists = errors.New("device already exists")
	ErrDeviceNotFound      = errors.New("device not found")
	ErrDeviceRevoked       = errors.New("device has been revoked")
	ErrInvalidDeviceCert   = errors.New("invalid device certificate")
	ErrNotMasterIdentity   = errors.New("not a master identity")
	ErrCertExpired         = errors.New("device certificate expired")
)

// ============================================================================
//                              设备身份配置
// ============================================================================

// DeviceConfig 设备身份配置
type DeviceConfig struct {
	// MaxDevices 最大设备数
	MaxDevices int

	// CertValidity 证书有效期
	CertValidity time.Duration

	// AllowSelfSign 允许自签名
	AllowSelfSign bool
}

// DefaultDeviceConfig 返回默认配置
func DefaultDeviceConfig() DeviceConfig {
	return DeviceConfig{
		MaxDevices:    10,
		CertValidity:  365 * 24 * time.Hour, // 1 年
		AllowSelfSign: true,
	}
}

// ============================================================================
//                              DeviceCertificate 设备证书
// ============================================================================

// DeviceCertificate 设备证书
type DeviceCertificate struct {
	// DeviceID 设备 ID
	DeviceID types.NodeID

	// MasterID 主身份 ID
	MasterID types.NodeID

	// DevicePublicKey 设备公钥
	DevicePublicKey ed25519.PublicKey

	// IssuedAt 签发时间
	IssuedAt time.Time

	// ExpiresAt 过期时间
	ExpiresAt time.Time

	// DeviceName 设备名称
	DeviceName string

	// Signature 主身份签名
	Signature []byte
}

// IsExpired 检查证书是否过期
func (c *DeviceCertificate) IsExpired() bool {
	return time.Now().After(c.ExpiresAt)
}

// Bytes 返回证书的字节表示（用于签名）
func (c *DeviceCertificate) Bytes() []byte {
	// 格式: DeviceID(32) + MasterID(32) + DevicePublicKey(32) + IssuedAt(8) + ExpiresAt(8) + DeviceName
	buf := make([]byte, 0, 32+32+32+8+8+len(c.DeviceName))
	buf = append(buf, c.DeviceID[:]...)
	buf = append(buf, c.MasterID[:]...)
	buf = append(buf, c.DevicePublicKey...)

	issuedBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(issuedBytes, uint64(c.IssuedAt.Unix()))
	buf = append(buf, issuedBytes...)

	expiresBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(expiresBytes, uint64(c.ExpiresAt.Unix()))
	buf = append(buf, expiresBytes...)

	buf = append(buf, []byte(c.DeviceName)...)
	return buf
}

// ============================================================================
//                              DeviceIdentity 设备身份
// ============================================================================

// DeviceIdentity 设备身份
type DeviceIdentity struct {
	config DeviceConfig

	// 主身份
	masterIdentity identityif.Identity
	isMaster       bool

	// 设备列表
	devices   map[types.NodeID]*deviceInfo
	devicesMu sync.RWMutex

	// 撤销列表
	revokedDevices   map[types.NodeID]time.Time
	revokedDevicesMu sync.RWMutex
}

// deviceInfo 设备信息
type deviceInfo struct {
	Certificate *DeviceCertificate
	PrivateKey  ed25519.PrivateKey // 仅当是本地设备时有值
	CreatedAt   time.Time
	LastSeen    time.Time
}

// NewDeviceIdentity 创建设备身份管理器
func NewDeviceIdentity(masterIdentity identityif.Identity, config DeviceConfig) (*DeviceIdentity, error) {
	return &DeviceIdentity{
		config:         config,
		masterIdentity: masterIdentity,
		isMaster:       true,
		devices:        make(map[types.NodeID]*deviceInfo),
		revokedDevices: make(map[types.NodeID]time.Time),
	}, nil
}

// NewSubDeviceIdentity 创建从设备身份
func NewSubDeviceIdentity(cert *DeviceCertificate, privateKey ed25519.PrivateKey) (*DeviceIdentity, error) {
	di := &DeviceIdentity{
		config:         DefaultDeviceConfig(),
		isMaster:       false,
		devices:        make(map[types.NodeID]*deviceInfo),
		revokedDevices: make(map[types.NodeID]time.Time),
	}

	// 添加自己作为设备
	di.devices[cert.DeviceID] = &deviceInfo{
		Certificate: cert,
		PrivateKey:  privateKey,
		CreatedAt:   cert.IssuedAt,
		LastSeen:    time.Now(),
	}

	return di, nil
}

// ============================================================================
//                              设备管理
// ============================================================================

// IssueDeviceCertificate 签发设备证书
func (d *DeviceIdentity) IssueDeviceCertificate(deviceName string, devicePubKey ed25519.PublicKey) (*DeviceCertificate, error) {
	if !d.isMaster {
		return nil, ErrNotMasterIdentity
	}

	// 检查设备数量限制
	d.devicesMu.RLock()
	if len(d.devices) >= d.config.MaxDevices {
		d.devicesMu.RUnlock()
		return nil, errors.New("maximum device limit reached")
	}
	d.devicesMu.RUnlock()

	// 生成设备 ID
	deviceID := nodeIDFromEd25519PublicKey(devicePubKey)

	// 检查是否已存在
	d.devicesMu.RLock()
	if _, ok := d.devices[deviceID]; ok {
		d.devicesMu.RUnlock()
		return nil, ErrDeviceAlreadyExists
	}
	d.devicesMu.RUnlock()

	// 检查是否被撤销
	d.revokedDevicesMu.RLock()
	if _, ok := d.revokedDevices[deviceID]; ok {
		d.revokedDevicesMu.RUnlock()
		return nil, ErrDeviceRevoked
	}
	d.revokedDevicesMu.RUnlock()

	now := time.Now()

	// 创建证书
	cert := &DeviceCertificate{
		DeviceID:        deviceID,
		MasterID:        d.masterIdentity.ID(),
		DevicePublicKey: devicePubKey,
		IssuedAt:        now,
		ExpiresAt:       now.Add(d.config.CertValidity),
		DeviceName:      deviceName,
	}

	// 签名证书
	signature, err := d.masterIdentity.Sign(cert.Bytes())
	if err != nil {
		return nil, err
	}
	cert.Signature = signature

	// 添加到设备列表
	d.devicesMu.Lock()
	d.devices[deviceID] = &deviceInfo{
		Certificate: cert,
		CreatedAt:   now,
		LastSeen:    now,
	}
	d.devicesMu.Unlock()

	log.Info("签发设备证书",
		"deviceID", deviceID.ShortString(),
		"deviceName", deviceName)

	return cert, nil
}

// GenerateDeviceKeyPair 生成设备密钥对并签发证书
func (d *DeviceIdentity) GenerateDeviceKeyPair(deviceName string) (*DeviceCertificate, ed25519.PrivateKey, error) {
	// 生成新的 Ed25519 密钥对
	pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, nil, err
	}

	// 签发证书
	cert, err := d.IssueDeviceCertificate(deviceName, pubKey)
	if err != nil {
		return nil, nil, err
	}

	// 存储私钥
	d.devicesMu.Lock()
	if info, ok := d.devices[cert.DeviceID]; ok {
		info.PrivateKey = privKey
	}
	d.devicesMu.Unlock()

	return cert, privKey, nil
}

// VerifyDeviceCertificate 验证设备证书
func (d *DeviceIdentity) VerifyDeviceCertificate(cert *DeviceCertificate) error {
	// 检查过期
	if cert.IsExpired() {
		return ErrCertExpired
	}

	// 检查撤销
	d.revokedDevicesMu.RLock()
	if _, ok := d.revokedDevices[cert.DeviceID]; ok {
		d.revokedDevicesMu.RUnlock()
		return ErrDeviceRevoked
	}
	d.revokedDevicesMu.RUnlock()

	// 验证签名
	var masterPubKeyBytes []byte
	if d.masterIdentity != nil {
		masterPubKeyBytes = d.masterIdentity.PublicKey().Bytes()
	} else {
		// 从设备列表获取主身份公钥（如果是从设备）
		return ErrInvalidDeviceCert
	}

	masterPubKey := ed25519.PublicKey(masterPubKeyBytes)
	if !ed25519.Verify(masterPubKey, cert.Bytes(), cert.Signature) {
		return ErrInvalidDeviceCert
	}

	return nil
}

// RevokeDevice 撤销设备
func (d *DeviceIdentity) RevokeDevice(deviceID types.NodeID) error {
	if !d.isMaster {
		return ErrNotMasterIdentity
	}

	// 从设备列表移除
	d.devicesMu.Lock()
	delete(d.devices, deviceID)
	d.devicesMu.Unlock()

	// 添加到撤销列表
	d.revokedDevicesMu.Lock()
	d.revokedDevices[deviceID] = time.Now()
	d.revokedDevicesMu.Unlock()

	log.Info("撤销设备",
		"deviceID", deviceID.ShortString())

	return nil
}

// IsDeviceRevoked 检查设备是否被撤销
func (d *DeviceIdentity) IsDeviceRevoked(deviceID types.NodeID) bool {
	d.revokedDevicesMu.RLock()
	defer d.revokedDevicesMu.RUnlock()
	_, ok := d.revokedDevices[deviceID]
	return ok
}

// ============================================================================
//                              查询方法
// ============================================================================

// GetDevice 获取设备信息
func (d *DeviceIdentity) GetDevice(deviceID types.NodeID) (*DeviceCertificate, bool) {
	d.devicesMu.RLock()
	defer d.devicesMu.RUnlock()

	info, ok := d.devices[deviceID]
	if !ok {
		return nil, false
	}
	return info.Certificate, true
}

// ListDevices 列出所有设备
func (d *DeviceIdentity) ListDevices() []*DeviceCertificate {
	d.devicesMu.RLock()
	defer d.devicesMu.RUnlock()

	certs := make([]*DeviceCertificate, 0, len(d.devices))
	for _, info := range d.devices {
		certs = append(certs, info.Certificate)
	}
	return certs
}

// ListRevokedDevices 列出撤销的设备
func (d *DeviceIdentity) ListRevokedDevices() []types.NodeID {
	d.revokedDevicesMu.RLock()
	defer d.revokedDevicesMu.RUnlock()

	ids := make([]types.NodeID, 0, len(d.revokedDevices))
	for id := range d.revokedDevices {
		ids = append(ids, id)
	}
	return ids
}

// DeviceCount 返回设备数量
func (d *DeviceIdentity) DeviceCount() int {
	d.devicesMu.RLock()
	defer d.devicesMu.RUnlock()
	return len(d.devices)
}

// IsMaster 检查是否是主身份
func (d *DeviceIdentity) IsMaster() bool {
	return d.isMaster
}

// MasterID 返回主身份 ID
func (d *DeviceIdentity) MasterID() types.NodeID {
	if d.masterIdentity != nil {
		return d.masterIdentity.ID()
	}

	// 从设备证书获取
	d.devicesMu.RLock()
	defer d.devicesMu.RUnlock()

	for _, info := range d.devices {
		return info.Certificate.MasterID
	}

	return types.EmptyNodeID
}

// ============================================================================
//                              更新设备状态
// ============================================================================

// UpdateDeviceLastSeen 更新设备最后活跃时间
func (d *DeviceIdentity) UpdateDeviceLastSeen(deviceID types.NodeID) {
	d.devicesMu.Lock()
	defer d.devicesMu.Unlock()

	if info, ok := d.devices[deviceID]; ok {
		info.LastSeen = time.Now()
	}
}

// CleanupExpiredDevices 清理过期设备证书
func (d *DeviceIdentity) CleanupExpiredDevices() int {
	d.devicesMu.Lock()
	defer d.devicesMu.Unlock()

	count := 0
	for id, info := range d.devices {
		if info.Certificate.IsExpired() {
			delete(d.devices, id)
			count++
		}
	}

	if count > 0 {
		log.Info("清理过期设备证书",
			"count", count)
	}

	return count
}

// ============================================================================
//                              辅助函数
// ============================================================================

// nodeIDFromEd25519PublicKey 从 ed25519 公钥派生 NodeID
func nodeIDFromEd25519PublicKey(pubKey ed25519.PublicKey) types.NodeID {
	hash := sha256.Sum256(pubKey)
	return types.NodeID(hash)
}

