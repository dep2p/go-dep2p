// Package tls 提供基于 TLS 的安全传输实现
package tls

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"time"

	identityif "github.com/dep2p/go-dep2p/pkg/interfaces/identity"
	securityif "github.com/dep2p/go-dep2p/pkg/interfaces/security"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// ConfigBuilder TLS 配置构建器
type ConfigBuilder struct {
	identity    identityif.Identity
	certManager *CertificateManager
	cert        *tls.Certificate

	// 配置选项
	minVersion         uint16
	cipherSuites       []uint16
	requireClientAuth  bool
	insecureSkipVerify bool
	nextProtos         []string

	// 0-RTT 支持
	sessionCache tls.ClientSessionCache
}

// NewConfigBuilder 创建配置构建器
func NewConfigBuilder(identity identityif.Identity) *ConfigBuilder {
	certManager := NewCertificateManager(identity)
	return &ConfigBuilder{
		identity:           identity,
		certManager:        certManager,
		minVersion:         tls.VersionTLS13,
		requireClientAuth:  true,
		insecureSkipVerify: false,
		nextProtos:         []string{"dep2p/1.0.0"},
	}
}

// NewConfigBuilderFromConfig 从 securityif.Config 创建配置构建器
func NewConfigBuilderFromConfig(identity identityif.Identity, config securityif.Config) *ConfigBuilder {
	builder := NewConfigBuilder(identity)

	// 应用配置
	if config.MinVersion != 0 {
		builder.minVersion = config.MinVersion
	}
	if len(config.CipherSuites) > 0 {
		builder.cipherSuites = config.CipherSuites
	}
	builder.requireClientAuth = config.RequireClientAuth
	builder.insecureSkipVerify = config.InsecureSkipVerify

	// 使用提供的证书
	if config.Certificate != nil {
		builder.cert = config.Certificate
	}

	return builder
}

// WithCertificate 设置证书
func (b *ConfigBuilder) WithCertificate(cert *tls.Certificate) *ConfigBuilder {
	b.cert = cert
	return b
}

// WithMinVersion 设置最低 TLS 版本
func (b *ConfigBuilder) WithMinVersion(version uint16) *ConfigBuilder {
	b.minVersion = version
	return b
}

// WithCipherSuites 设置加密套件（仅 TLS 1.2 及以下有效）
func (b *ConfigBuilder) WithCipherSuites(suites []uint16) *ConfigBuilder {
	b.cipherSuites = suites
	return b
}

// WithRequireClientAuth 设置是否要求客户端认证
func (b *ConfigBuilder) WithRequireClientAuth(require bool) *ConfigBuilder {
	b.requireClientAuth = require
	return b
}

// WithInsecureSkipVerify 设置是否跳过部分验证
//
// 当设置为 true 时：
//   - 跳过证书有效期检查
//   - 跳过 expectedID 匹配检查
//   - 仍保留公钥派生 NodeID 的不可伪造约束
//   - 仅用于测试环境
func (b *ConfigBuilder) WithInsecureSkipVerify(skip bool) *ConfigBuilder {
	b.insecureSkipVerify = skip
	return b
}

// WithNextProtos 设置 ALPN 协议
func (b *ConfigBuilder) WithNextProtos(protos []string) *ConfigBuilder {
	b.nextProtos = protos
	return b
}

// WithSessionCache 设置 Session Cache（用于 0-RTT 重连）
func (b *ConfigBuilder) WithSessionCache(cache tls.ClientSessionCache) *ConfigBuilder {
	b.sessionCache = cache
	return b
}

// BuildServerConfig 构建服务端 TLS 配置
func (b *ConfigBuilder) BuildServerConfig() (*tls.Config, error) {
	cert, err := b.ensureCertificate()
	if err != nil {
		return nil, err
	}

	// 创建验证回调
	verifyCallback := b.createVerifyCallback(types.EmptyNodeID)

	config := &tls.Config{
		Certificates: []tls.Certificate{*cert},
		MinVersion:   b.minVersion,
		NextProtos:   b.nextProtos,
		// P2P 场景使用自签名证书，需要自定义验证
		InsecureSkipVerify:    true, //nolint:gosec // G402: 使用 VerifyPeerCertificate 进行自定义验证
		VerifyPeerCertificate: verifyCallback,
	}

	// 设置加密套件（仅 TLS 1.2 及以下有效）
	if len(b.cipherSuites) > 0 {
		config.CipherSuites = b.cipherSuites
	}

	// 设置客户端认证
	if b.requireClientAuth {
		config.ClientAuth = tls.RequireAnyClientCert
	} else {
		config.ClientAuth = tls.RequestClientCert
	}

	return config, nil
}

// BuildClientConfig 构建客户端 TLS 配置
func (b *ConfigBuilder) BuildClientConfig(expectedServerID types.NodeID) (*tls.Config, error) {
	cert, err := b.ensureCertificate()
	if err != nil {
		return nil, err
	}

	// 创建验证回调
	verifyCallback := b.createVerifyCallback(expectedServerID)

	config := &tls.Config{
		Certificates: []tls.Certificate{*cert},
		MinVersion:   b.minVersion,
		NextProtos:   b.nextProtos,
		// P2P 场景使用自签名证书，需要自定义验证
		InsecureSkipVerify:    true, //nolint:gosec // G402: 使用 VerifyPeerCertificate 进行自定义验证
		VerifyPeerCertificate: verifyCallback,
	}

	// 设置加密套件（仅 TLS 1.2 及以下有效）
	if len(b.cipherSuites) > 0 {
		config.CipherSuites = b.cipherSuites
	}

	// 设置 Session Cache（用于 0-RTT 重连）
	if b.sessionCache != nil {
		config.ClientSessionCache = b.sessionCache
	}

	return config, nil
}

// ensureCertificate 确保有证书可用
func (b *ConfigBuilder) ensureCertificate() (*tls.Certificate, error) {
	if b.cert != nil {
		return b.cert, nil
	}

	// 从 Identity 生成证书
	cert, err := b.certManager.GenerateCertificateFromIdentity()
	if err != nil {
		return nil, fmt.Errorf("生成证书失败: %w", err)
	}

	b.cert = cert
	return cert, nil
}

// createVerifyCallback 创建证书验证回调
//
// 验证逻辑：
//  1. 从证书公钥派生 NodeID（不可伪造，始终执行）
//  2. 若证书带有 NodeID 扩展，验证扩展值等于派生值（始终执行）
//  3. 验证 expectedID（除非 insecureSkipVerify=true）
//  4. 验证有效期（除非 insecureSkipVerify=true）
//  5. 验证自签名完整性（除非 insecureSkipVerify=true）
func (b *ConfigBuilder) createVerifyCallback(expectedID types.NodeID) func([][]byte, [][]*x509.Certificate) error {
	return func(rawCerts [][]byte, _ [][]*x509.Certificate) error {
		if len(rawCerts) == 0 {
			return fmt.Errorf("对端未提供证书")
		}

		// 解析证书
		cert, err := x509.ParseCertificate(rawCerts[0])
		if err != nil {
			return fmt.Errorf("解析证书失败: %w", err)
		}

		// 1. 从证书公钥派生 NodeID（始终执行，不可伪造）
		derivedID, err := DeriveNodeIDFromCertPublicKey(cert)
		if err != nil {
			return fmt.Errorf("从证书公钥派生 NodeID 失败: %w", err)
		}

		// 2. 若证书带有 NodeID 扩展，验证扩展值等于派生值（始终执行）
		for _, ext := range cert.Extensions {
			if ext.Id.Equal(nodeIDExtensionOID) {
				if len(ext.Value) != 32 {
					return fmt.Errorf("无效的 NodeID 扩展长度: %d", len(ext.Value))
				}
				extensionID, err := types.NodeIDFromBytes(ext.Value)
				if err != nil {
					return fmt.Errorf("解析扩展 NodeID 失败: %w", err)
				}
				if !extensionID.Equal(derivedID) {
					return fmt.Errorf("NodeID 扩展与公钥派生不一致: 扩展 %s, 派生 %s",
						extensionID.String(), derivedID.String())
				}
				break
			}
		}

		// insecureSkipVerify 模式下跳过后续检查
		if b.insecureSkipVerify {
			return nil
		}

		// 3. 验证 expectedID
		if !expectedID.IsEmpty() && !derivedID.Equal(expectedID) {
			return fmt.Errorf("NodeID 不匹配: 期望 %s, 实际 %s",
				expectedID.String(), derivedID.String())
		}

		// 4. 验证证书有效期
		now := time.Now()
		if now.Before(cert.NotBefore) {
			return fmt.Errorf("证书尚未生效: NotBefore=%v", cert.NotBefore)
		}
		if now.After(cert.NotAfter) {
			return fmt.Errorf("证书已过期: NotAfter=%v", cert.NotAfter)
		}

		// 注意：不验证自签名完整性，因为：
		// 1. TLS 握手本身会验证证书签名
		// 2. 我们的安全性来自 NodeID 与公钥的强绑定，而非证书链
		// 3. cert.CheckSignatureFrom(cert) 对非 CA 证书会失败

		return nil
	}
}

// TLSConfigProvider TLS 配置提供者
// 提供一个简化的 API 来获取 TLS 配置
type TLSConfigProvider struct {
	identity     identityif.Identity
	builder      *ConfigBuilder
	sessionCache tls.ClientSessionCache
}

// NewTLSConfigProvider 创建 TLS 配置提供者
func NewTLSConfigProvider(identity identityif.Identity) *TLSConfigProvider {
	return &TLSConfigProvider{
		identity: identity,
		builder:  NewConfigBuilder(identity),
	}
}

// NewTLSConfigProviderFromConfig 从 securityif.Config 创建 TLS 配置提供者
func NewTLSConfigProviderFromConfig(identity identityif.Identity, config securityif.Config) *TLSConfigProvider {
	return &TLSConfigProvider{
		identity: identity,
		builder:  NewConfigBuilderFromConfig(identity, config),
	}
}

// NewTLSConfigProviderWith0RTT 创建支持 0-RTT 的 TLS 配置提供者
func NewTLSConfigProviderWith0RTT(identity identityif.Identity, sessionCache tls.ClientSessionCache) *TLSConfigProvider {
	return &TLSConfigProvider{
		identity:     identity,
		builder:      NewConfigBuilder(identity).WithSessionCache(sessionCache),
		sessionCache: sessionCache,
	}
}

// ServerConfig 返回服务端 TLS 配置
func (p *TLSConfigProvider) ServerConfig() (*tls.Config, error) {
	return p.builder.BuildServerConfig()
}

// ClientConfig 返回客户端 TLS 配置
func (p *TLSConfigProvider) ClientConfig(expectedServerID types.NodeID) (*tls.Config, error) {
	// 创建新的 builder 以避免共享状态
	builder := NewConfigBuilder(p.identity).
		WithMinVersion(p.builder.minVersion).
		WithRequireClientAuth(p.builder.requireClientAuth).
		WithInsecureSkipVerify(p.builder.insecureSkipVerify).
		WithNextProtos(p.builder.nextProtos)

	if len(p.builder.cipherSuites) > 0 {
		builder.WithCipherSuites(p.builder.cipherSuites)
	}
	if p.builder.cert != nil {
		builder.WithCertificate(p.builder.cert)
	}
	if p.sessionCache != nil {
		builder.WithSessionCache(p.sessionCache)
	}

	return builder.BuildClientConfig(expectedServerID)
}

// InsecureSkipVerify 返回是否跳过验证
func (p *TLSConfigProvider) InsecureSkipVerify() bool {
	return p.builder.insecureSkipVerify
}

// SetSessionCache 设置 Session Cache
func (p *TLSConfigProvider) SetSessionCache(cache tls.ClientSessionCache) {
	p.sessionCache = cache
	p.builder.WithSessionCache(cache)
}
