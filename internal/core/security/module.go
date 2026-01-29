// Package security 实现安全传输
package security

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/dep2p/go-dep2p/config"
	"github.com/dep2p/go-dep2p/internal/core/security/noise"
	"github.com/dep2p/go-dep2p/internal/core/security/tls"
	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/lib/log"
	"github.com/dep2p/go-dep2p/pkg/types"
	mss "github.com/multiformats/go-multistream"
	"go.uber.org/fx"
)

var logger = log.Logger("core/security")

const (
	// defaultNegotiateTimeout 默认协商超时
	defaultNegotiateTimeout = 60 * time.Second
)

// Config 安全传输配置
type Config struct {
	// Transports 启用的安全传输协议
	Transports []string // ["tls", "noise"]
	// Preferred 首选协议
	Preferred string // "tls" or "noise"
	// NegotiateTimeout 协商超时
	NegotiateTimeout time.Duration
}

// ConfigFromUnified 从统一配置创建安全配置
func ConfigFromUnified(cfg *config.Config) Config {
	if cfg == nil {
		return NewConfig()
	}

	transports := make([]string, 0, 2)
	if cfg.Security.EnableTLS {
		transports = append(transports, "tls")
	}
	if cfg.Security.EnableNoise {
		transports = append(transports, "noise")
	}

	// 如果都没启用，至少启用 TLS
	if len(transports) == 0 {
		transports = append(transports, "tls")
	}

	return Config{
		Transports:       transports,
		Preferred:        cfg.Security.PreferredProtocol,
		NegotiateTimeout: cfg.Security.NegotiateTimeout.Duration(),
	}
}

// NewConfig 创建默认配置
func NewConfig() Config {
	return Config{
		Transports:       []string{"tls", "noise"}, // 默认启用 TLS 和 Noise
		Preferred:        "tls",                    // 默认首选 TLS
		NegotiateTimeout: defaultNegotiateTimeout,
	}
}

// Module 返回 Fx 模块
func Module() fx.Option {
	return fx.Module("security",
		fx.Provide(
			ProvideConfig,
			NewSecurityMux,
			// 将 *SecurityMux 作为 pkgif.SecureTransport 提供
			func(mux *SecurityMux) pkgif.SecureTransport {
				return mux
			},
			// P4 新增：提供身份绑定验证器
			ProvideIdentityBinding,
			// P4 新增：提供 TLS 访问控制
			ProvideAccessControl,
		),
	)
}

// ProvideIdentityBinding 提供身份绑定验证器（P4 新增）
func ProvideIdentityBinding(id pkgif.Identity) (*noise.IdentityBinding, error) {
	return noise.NewIdentityBindingFromIdentity(id)
}

// AccessControlConfig 访问控制配置
type AccessControlConfig struct {
	Mode      tls.AccessMode
	Whitelist []types.PeerID
	Blacklist []types.PeerID
}

// ProvideAccessControl 提供 TLS 访问控制（P4 新增）
func ProvideAccessControl(_ *config.Config) *tls.AccessControl {
	// 从配置中读取访问控制设置（如果有）
	acConfig := tls.DefaultAccessControlConfig()
	// 可根据 cfg 调整 acConfig
	return tls.NewAccessControl(acConfig)
}

// ProvideConfig 从统一配置提供安全配置
func ProvideConfig(cfg *config.Config) Config {
	return ConfigFromUnified(cfg)
}

// ProvideTLSTransport 提供 TLS 传输
func ProvideTLSTransport(id pkgif.Identity) (pkgif.SecureTransport, error) {
	return tls.New(id)
}

// ProvideNoiseTransport 提供 Noise 传输
func ProvideNoiseTransport(id pkgif.Identity) (pkgif.SecureTransport, error) {
	return noise.New(id)
}

// ============================================================================
// SecurityMux - 多协议安全选择器
// ============================================================================

// SecurityMux 多协议安全选择器
//
// SecurityMux 实现了 multistream-select 协议协商，
// 支持多个安全传输协议（TLS, Noise）并动态选择。
type SecurityMux struct {
	transports       map[string]pkgif.SecureTransport
	preferred        string
	identity         pkgif.Identity
	negotiateTimeout time.Duration
}

// 确保实现接口
var _ pkgif.SecureTransport = (*SecurityMux)(nil)

// SecurityMuxParams NewSecurityMux 的 Fx 参数
type SecurityMuxParams struct {
	fx.In

	Identity        pkgif.Identity
	Config          Config
	IdentityBinding *noise.IdentityBinding `optional:"true"` // 可选的身份绑定
	AccessControl   *tls.AccessControl     `optional:"true"` // 可选的访问控制
}

// NewSecurityMux 创建多协议安全选择器
func NewSecurityMux(params SecurityMuxParams) (*SecurityMux, error) {
	id := params.Identity
	cfg := params.Config

	if id == nil {
		return nil, fmt.Errorf("identity is nil")
	}

	timeout := cfg.NegotiateTimeout
	if timeout <= 0 {
		timeout = defaultNegotiateTimeout
	}

	mux := &SecurityMux{
		transports:       make(map[string]pkgif.SecureTransport),
		preferred:        cfg.Preferred,
		identity:         id,
		negotiateTimeout: timeout,
	}

	// 注册启用的传输协议
	for _, proto := range cfg.Transports {
		switch proto {
		case "tls":
			t, err := tls.New(id)
			if err != nil {
				return nil, fmt.Errorf("create tls transport: %w", err)
			}
			// 集成 AccessControl（如果存在）
			if params.AccessControl != nil {
				t.SetAccessControl(params.AccessControl)
				logger.Debug("TLS Transport 已集成 AccessControl")
			}
			mux.transports[proto] = t

		case "noise":
			t, err := noise.New(id)
			if err != nil {
				return nil, fmt.Errorf("create noise transport: %w", err)
			}
			// 集成 IdentityBinding（如果存在）
			if params.IdentityBinding != nil {
				t.SetIdentityBinding(params.IdentityBinding)
				logger.Debug("Noise Transport 已集成 IdentityBinding")
			}
			mux.transports[proto] = t

		default:
			return nil, fmt.Errorf("unknown transport protocol: %s", proto)
		}
	}

	// 确保至少有一个传输协议
	if len(mux.transports) == 0 {
		return nil, fmt.Errorf("no transport protocols enabled")
	}

	// 确保首选协议存在
	if mux.preferred != "" {
		if _, ok := mux.transports[mux.preferred]; !ok {
			return nil, fmt.Errorf("preferred protocol %s not enabled", mux.preferred)
		}
	} else {
		// 如果没有指定首选，使用第一个
		for proto := range mux.transports {
			mux.preferred = proto
			break
		}
	}

	return mux, nil
}

// ID 返回协议标识
func (m *SecurityMux) ID() types.ProtocolID {
	return types.ProtocolID("/security/multistream/1.0.0")
}

// SecureInbound 保护入站连接（服务器端）
func (m *SecurityMux) SecureInbound(ctx context.Context, conn net.Conn, remotePeer types.PeerID) (pkgif.SecureConn, error) {
	logger.Debug("安全协商入站连接", "remotePeer", string(remotePeer)[:8])
	
	// 设置协商超时
	deadline := time.Now().Add(m.negotiateTimeout)
	if d, ok := ctx.Deadline(); ok {
		deadline = d
	}
	if err := conn.SetDeadline(deadline); err != nil {
		return nil, fmt.Errorf("set deadline: %w", err)
	}
	defer conn.SetDeadline(time.Time{}) // 清除超时

	// 创建 multistream muxer 并注册所有支持的协议
	muxer := mss.NewMultistreamMuxer[string]()
	for proto := range m.transports {
		muxer.AddHandler(proto, nil)
	}

	// 服务端协商：从客户端提议中选择
	selectedProto, _, err := muxer.Negotiate(conn)
	if err != nil {
		logger.Warn("安全协议协商失败", "error", err)
		return nil, fmt.Errorf("multistream negotiation failed: %w", err)
	}

	logger.Debug("安全协议协商成功", "protocol", selectedProto)

	// 查找对应的传输协议
	transport, ok := m.transports[selectedProto]
	if !ok {
		logger.Error("协商的协议未找到", "protocol", selectedProto)
		return nil, fmt.Errorf("negotiated protocol %s not found", selectedProto)
	}

	// 使用选定的传输协议进行握手
	secConn, err := transport.SecureInbound(ctx, conn, remotePeer)
	if err != nil {
		logger.Warn("安全握手失败", "protocol", selectedProto, "error", err)
	} else {
		logger.Debug("安全握手成功", "protocol", selectedProto)
	}
	return secConn, err
}

// SecureOutbound 保护出站连接（客户端端）
func (m *SecurityMux) SecureOutbound(ctx context.Context, conn net.Conn, remotePeer types.PeerID) (pkgif.SecureConn, error) {
	logger.Debug("安全协商出站连接", "remotePeer", string(remotePeer)[:8], "preferred", m.preferred)
	
	// 设置协商超时
	deadline := time.Now().Add(m.negotiateTimeout)
	if d, ok := ctx.Deadline(); ok {
		deadline = d
	}
	if err := conn.SetDeadline(deadline); err != nil {
		return nil, fmt.Errorf("set deadline: %w", err)
	}
	defer conn.SetDeadline(time.Time{}) // 清除超时

	// 构建协议列表（首选协议在前）
	protocols := make([]string, 0, len(m.transports))

	// 首选协议放在第一位
	if m.preferred != "" {
		protocols = append(protocols, m.preferred)
	}

	// 添加其他协议
	for proto := range m.transports {
		if proto != m.preferred {
			protocols = append(protocols, proto)
		}
	}

	// 客户端协商：提议协议列表，服务端选择
	selectedProto, err := mss.SelectOneOf(protocols, conn)
	if err != nil {
		logger.Warn("安全协议选择失败", "error", err)
		return nil, fmt.Errorf("multistream selection failed: %w", err)
	}

	logger.Debug("安全协议选择成功", "protocol", selectedProto)

	// 查找对应的传输协议
	transport, ok := m.transports[selectedProto]
	if !ok {
		logger.Error("选择的协议未找到", "protocol", selectedProto)
		return nil, fmt.Errorf("selected protocol %s not found", selectedProto)
	}

	// 使用选定的传输协议进行握手
	secConn, err := transport.SecureOutbound(ctx, conn, remotePeer)
	if err != nil {
		logger.Warn("安全握手失败", "protocol", selectedProto, "error", err)
	} else {
		logger.Debug("安全握手成功", "protocol", selectedProto)
	}
	return secConn, err
}

// ListProtocols 列出所有支持的协议
func (m *SecurityMux) ListProtocols() []string {
	protocols := make([]string, 0, len(m.transports))
	for proto := range m.transports {
		protocols = append(protocols, proto)
	}
	return protocols
}

// SetPreferred 设置首选协议
func (m *SecurityMux) SetPreferred(proto string) error {
	if _, ok := m.transports[proto]; !ok {
		return fmt.Errorf("protocol %s not enabled", proto)
	}
	m.preferred = proto
	return nil
}
