// Package security 定义安全层接口
//
// 安全模块负责连接的加密和身份验证，包括：
// - 安全握手
// - 加密通道建立
// - 身份验证
// - 安全事件通知（REQ-SEC-002）
package security

import (
	"context"
	"crypto/tls"
	"time"

	"github.com/dep2p/go-dep2p/pkg/interfaces/identity"
	"github.com/dep2p/go-dep2p/pkg/interfaces/transport"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              SecureTransport 接口
// ============================================================================

// SecureTransport 安全传输接口
//
// SecureTransport 提供连接的安全升级能力，
// 将普通连接升级为加密连接。
type SecureTransport interface {
	// SecureInbound 对入站连接进行安全握手
	//
	// 用于服务端，被动接受安全握手。
	SecureInbound(ctx context.Context, conn transport.Conn) (SecureConn, error)

	// SecureOutbound 对出站连接进行安全握手
	//
	// 用于客户端，主动发起安全握手。
	// remotePeer 是期望的远程节点 ID，用于身份验证。
	SecureOutbound(ctx context.Context, conn transport.Conn, remotePeer types.NodeID) (SecureConn, error)

	// Protocol 返回安全协议名称
	// 如 "tls", "noise"
	Protocol() string
}

// ============================================================================
//                              SecureConn 接口
// ============================================================================

// SecureConn 安全连接接口
//
// SecureConn 是经过安全握手后的加密连接，
// 提供身份信息和加密保证。
type SecureConn interface {
	transport.Conn

	// LocalIdentity 返回本地身份
	LocalIdentity() types.NodeID

	// LocalPublicKey 返回本地公钥
	LocalPublicKey() identity.PublicKey

	// RemoteIdentity 返回远程节点 ID
	RemoteIdentity() types.NodeID

	// RemotePublicKey 返回远程公钥
	RemotePublicKey() identity.PublicKey

	// ConnectionState 返回连接状态（如 TLS 状态）
	ConnectionState() ConnectionState
}

// ============================================================================
//                              连接状态
// ============================================================================

// ConnectionState 安全连接状态（类型别名，实际定义在 types 包）
type ConnectionState = types.ConnectionState

// ============================================================================
//                              TLS 配置（v1.1 已删除）
// ============================================================================

// 注意：TLSConfig 接口已删除（v1.1 清理）。
// 原因：public 接口签名与实现不一致，且无外部使用。
// TLS 配置通过 internal/core/security/tls/config.go 的 TLSConfigProvider 直接使用。

// ============================================================================
//                              证书管理
// ============================================================================

// CertificateManager 证书管理器接口
type CertificateManager interface {
	// GenerateCertificate 生成自签名证书
	GenerateCertificate(identity types.NodeID, privateKey identity.PrivateKey) (*tls.Certificate, error)

	// VerifyPeerCertificate 验证对端证书
	VerifyPeerCertificate(certs [][]byte, expectedID types.NodeID) error

	// ExtractNodeID 从证书中提取节点 ID
	ExtractNodeID(cert []byte) (types.NodeID, error)

	// 注意：LoadCertificate 已删除（v1.1 清理）- 无生产使用。
	// 实现仍保留在 internal/core/security/tls/cert.go 中以备内部需要。
}

// ============================================================================
//                              访问控制
// ============================================================================

// AccessController 访问控制器接口
type AccessController interface {
	// AllowConnect 检查是否允许连接
	AllowConnect(nodeID types.NodeID) bool

	// AllowInbound 检查是否允许入站连接
	AllowInbound(nodeID types.NodeID) bool

	// AllowOutbound 检查是否允许出站连接
	AllowOutbound(nodeID types.NodeID) bool

	// AddToAllowList 添加到白名单
	AddToAllowList(nodeID types.NodeID)

	// AddToBlockList 添加到黑名单
	AddToBlockList(nodeID types.NodeID)

	// RemoveFromAllowList 从白名单移除
	RemoveFromAllowList(nodeID types.NodeID)

	// RemoveFromBlockList 从黑名单移除
	RemoveFromBlockList(nodeID types.NodeID)
}

// ============================================================================
//                              配置
// ============================================================================

// Config 安全模块配置
type Config struct {
	// Protocol 安全协议: "tls", "noise"
	Protocol string

	// PrivateKey 节点私钥
	PrivateKey identity.PrivateKey

	// Certificate 证书（可选，会自动生成）
	Certificate *tls.Certificate

	// InsecureSkipVerify 跳过证书验证（仅测试用）
	InsecureSkipVerify bool

	// RequireClientAuth 是否要求客户端认证
	RequireClientAuth bool

	// MinVersion TLS 最低版本
	MinVersion uint16

	// CipherSuites 加密套件
	CipherSuites []uint16

	// NoiseConfig Noise 协议配置（当 Protocol="noise" 时使用）
	NoiseConfig *NoiseConfig
}

// NoiseConfig Noise 协议配置
type NoiseConfig struct {
	// HandshakePattern 握手模式 (默认 "XX")
	// 支持: XX (双向认证), IK (已知响应者), NK (仅验证响应者)
	HandshakePattern string

	// DHCurve DH 曲线选择
	// 支持: "25519" (Curve25519)
	DHCurve string

	// CipherSuite 加密套件
	// 支持: "ChaChaPoly" (ChaCha20-Poly1305), "AESGCM" (AES-256-GCM)
	CipherSuite string

	// HashFunction 哈希函数
	// 支持: "SHA256", "BLAKE2b", "BLAKE2s"
	HashFunction string

	// Prologue 可选的协议标识符（用于协议绑定）
	Prologue []byte

	// StaticKeypair 静态密钥对（可选，不提供则自动生成）
	StaticKeypair *NoiseKeypair

	// MaxMessageSize 最大消息大小 (默认 65535)
	MaxMessageSize int

	// HandshakeTimeout 握手超时 (默认 10s)
	HandshakeTimeout int // 秒
}

// NoiseKeypair Noise 密钥对
type NoiseKeypair struct {
	// PublicKey 公钥 (32 bytes for Curve25519)
	PublicKey []byte

	// PrivateKey 私钥 (32 bytes for Curve25519)
	PrivateKey []byte
}

// DefaultNoiseConfig 返回默认 Noise 配置
func DefaultNoiseConfig() *NoiseConfig {
	return &NoiseConfig{
		HandshakePattern: "XX",
		DHCurve:          "25519",
		CipherSuite:      "ChaChaPoly",
		HashFunction:     "SHA256",
		Prologue:         []byte("dep2p/noise/1.0"),
		MaxMessageSize:   65535,
		HandshakeTimeout: 10,
	}
}

// DefaultConfig 返回默认配置
func DefaultConfig() Config {
	return Config{
		Protocol:           "tls",
		InsecureSkipVerify: false,
		RequireClientAuth:  true,
		MinVersion:         tls.VersionTLS13,
		NoiseConfig:        DefaultNoiseConfig(),
	}
}

// ============================================================================
//                              安全事件（REQ-SEC-002）
// ============================================================================

// SecurityEventType 安全事件类型
type SecurityEventType int

const (
	// SecurityEventUnknown 未知事件
	SecurityEventUnknown SecurityEventType = iota

	// SecurityEventHandshakeFailed 安全握手失败
	// 发生场景：TLS/Noise 握手过程中出错
	SecurityEventHandshakeFailed

	// SecurityEventHandshakeSuccess 安全握手成功
	SecurityEventHandshakeSuccess

	// SecurityEventIdentityMismatch 身份不匹配
	// 发生场景：远程节点 ID 与预期不符
	SecurityEventIdentityMismatch

	// SecurityEventCertificateExpired 证书过期
	// 发生场景：TLS 证书已过期
	SecurityEventCertificateExpired

	// SecurityEventCertificateInvalid 证书无效
	// 发生场景：证书格式错误、签名无效等
	SecurityEventCertificateInvalid

	// SecurityEventConnectionRejected 连接被拒绝
	// 发生场景：访问控制拒绝连接
	SecurityEventConnectionRejected

	// SecurityEventUnauthorizedAccess 未授权访问
	// 发生场景：尝试访问未授权的资源或协议
	SecurityEventUnauthorizedAccess

	// SecurityEventReplayAttackDetected 检测到重放攻击
	// 发生场景：Nonce 重复或时间戳异常
	SecurityEventReplayAttackDetected

	// SecurityEventSignatureInvalid 签名无效
	// 发生场景：消息或记录签名验证失败
	SecurityEventSignatureInvalid

	// SecurityEventBlockedPeer 被阻止的节点尝试连接
	// 发生场景：黑名单中的节点尝试建立连接
	SecurityEventBlockedPeer
)

// String 返回安全事件类型的字符串表示
func (e SecurityEventType) String() string {
	switch e {
	case SecurityEventHandshakeFailed:
		return "HandshakeFailed"
	case SecurityEventHandshakeSuccess:
		return "HandshakeSuccess"
	case SecurityEventIdentityMismatch:
		return "IdentityMismatch"
	case SecurityEventCertificateExpired:
		return "CertificateExpired"
	case SecurityEventCertificateInvalid:
		return "CertificateInvalid"
	case SecurityEventConnectionRejected:
		return "ConnectionRejected"
	case SecurityEventUnauthorizedAccess:
		return "UnauthorizedAccess"
	case SecurityEventReplayAttackDetected:
		return "ReplayAttackDetected"
	case SecurityEventSignatureInvalid:
		return "SignatureInvalid"
	case SecurityEventBlockedPeer:
		return "BlockedPeer"
	default:
		return "Unknown"
	}
}

// Severity 返回事件严重程度
func (e SecurityEventType) Severity() SecurityEventSeverity {
	switch e {
	case SecurityEventHandshakeSuccess:
		return SeverityInfo
	case SecurityEventHandshakeFailed, SecurityEventConnectionRejected:
		return SeverityWarning
	case SecurityEventIdentityMismatch, SecurityEventCertificateExpired,
		SecurityEventCertificateInvalid, SecurityEventBlockedPeer:
		return SeverityError
	case SecurityEventUnauthorizedAccess, SecurityEventReplayAttackDetected,
		SecurityEventSignatureInvalid:
		return SeverityCritical
	default:
		return SeverityInfo
	}
}

// SecurityEventSeverity 安全事件严重程度
type SecurityEventSeverity int

const (
	// SeverityInfo 信息级别（正常操作）
	SeverityInfo SecurityEventSeverity = iota
	// SeverityWarning 警告级别（需要关注）
	SeverityWarning
	// SeverityError 错误级别（需要处理）
	SeverityError
	// SeverityCritical 严重级别（可能存在攻击）
	SeverityCritical
)

// String 返回严重程度的字符串表示
func (s SecurityEventSeverity) String() string {
	switch s {
	case SeverityInfo:
		return "INFO"
	case SeverityWarning:
		return "WARNING"
	case SeverityError:
		return "ERROR"
	case SeverityCritical:
		return "CRITICAL"
	default:
		return "UNKNOWN"
	}
}

// SecurityEvent 安全事件
type SecurityEvent struct {
	// Type 事件类型
	Type SecurityEventType

	// Severity 严重程度（自动从 Type 推断，也可覆盖）
	Severity SecurityEventSeverity

	// Timestamp 事件发生时间
	Timestamp time.Time

	// RemoteAddr 远程地址（如有）
	RemoteAddr string

	// RemoteNodeID 远程节点 ID（如有）
	RemoteNodeID types.NodeID

	// ExpectedNodeID 预期节点 ID（用于身份不匹配场景）
	ExpectedNodeID types.NodeID

	// Protocol 相关协议（tls, noise, etc.）
	Protocol string

	// Error 相关错误（如有）
	Error error

	// Message 事件描述信息
	Message string

	// Details 额外详情（可选，用于诊断）
	Details map[string]interface{}
}

// SecurityEventCallback 安全事件回调函数类型
type SecurityEventCallback func(event SecurityEvent)

// SecurityEventEmitter 安全事件发射器接口
type SecurityEventEmitter interface {
	// OnSecurityEvent 注册安全事件回调
	// 可注册多个回调，按注册顺序调用
	OnSecurityEvent(callback SecurityEventCallback)

	// EmitSecurityEvent 发射安全事件（供内部使用）
	EmitSecurityEvent(event SecurityEvent)
}

// SecurityEventFilter 安全事件过滤器
// 用于仅订阅特定类型或严重程度的事件
type SecurityEventFilter struct {
	// Types 要过滤的事件类型（为空表示接收所有类型）
	Types []SecurityEventType

	// MinSeverity 最低严重程度（低于此级别的事件将被忽略）
	MinSeverity SecurityEventSeverity
}

// Match 检查事件是否匹配过滤器
func (f *SecurityEventFilter) Match(event SecurityEvent) bool {
	// 检查严重程度
	if event.Severity < f.MinSeverity {
		return false
	}

	// 如果未指定类型，则匹配所有
	if len(f.Types) == 0 {
		return true
	}

	// 检查类型
	for _, t := range f.Types {
		if event.Type == t {
			return true
		}
	}
	return false
}

// WithSecurityEventFilter 创建带过滤器的回调
func WithSecurityEventFilter(filter SecurityEventFilter, callback SecurityEventCallback) SecurityEventCallback {
	return func(event SecurityEvent) {
		if filter.Match(event) {
			callback(event)
		}
	}
}
