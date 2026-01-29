// Package tls 实现 TLS 1.3 安全传输
package tls

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	
	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// Config TLS 配置
//
// Config 封装了创建 TLS 连接所需的所有配置参数。
// 它从 Identity 派生证书和密钥，并提供用于客户端和服务端的配置。
type Config struct {
	// Identity 节点身份
	Identity pkgif.Identity
	
	// Certificate TLS 证书（从 Identity 派生）
	Certificate *tls.Certificate
	
	// ServerName TLS SNI 服务器名称
	ServerName string
	
	// NextProtos ALPN 协议列表
	NextProtos []string
	
	// MinVersion 最小 TLS 版本（默认 TLS 1.3）
	MinVersion uint16
	
	// RequireClientCert 服务端是否要求客户端证书
	RequireClientCert bool
}

// NewFromIdentity 从 Identity 创建 TLS 配置
//
// 此函数：
//  1. 从 Identity 获取私钥和公钥
//  2. 生成自签名 TLS 证书（内嵌 Ed25519 公钥）
//  3. 配置 TLS 1.3 参数
//
// 参数：
//   - identity: 节点身份
//
// 返回：
//   - *Config: TLS 配置
//   - error: 创建失败时的错误
func NewFromIdentity(identity pkgif.Identity) (*Config, error) {
	if identity == nil {
		return nil, fmt.Errorf("identity is nil")
	}
	
	// 生成 libp2p 风格的自签名证书
	cert, err := GenerateCert(identity)
	if err != nil {
		return nil, fmt.Errorf("generate certificate: %w", err)
	}
	
	return &Config{
		Identity:          identity,
		Certificate:       cert,
		ServerName:        "dep2p",
		NextProtos:        []string{"dep2p"},
		MinVersion:        tls.VersionTLS13, // 强制 TLS 1.3
		RequireClientCert: true,             // 双向认证
	}, nil
}

// DefaultConfig 返回默认配置
//
// 注意：默认配置没有 Identity 和 Certificate，
// 使用前必须调用 WithIdentity() 设置身份。
func DefaultConfig() *Config {
	return &Config{
		ServerName:        "dep2p",
		NextProtos:        []string{"dep2p"},
		MinVersion:        tls.VersionTLS13,
		RequireClientCert: true,
	}
}

// WithIdentity 设置 Identity 并生成证书
func (c *Config) WithIdentity(identity pkgif.Identity) error {
	if identity == nil {
		return fmt.Errorf("identity is nil")
	}
	
	cert, err := GenerateCert(identity)
	if err != nil {
		return fmt.Errorf("generate certificate: %w", err)
	}
	
	c.Identity = identity
	c.Certificate = cert
	return nil
}

// ServerConfig 生成服务端 TLS 配置
//
// 用于入站连接（服务器端握手）。
//
// 参数：
//   - remotePeer: 期望的远程 PeerID（用于验证）
//
// 返回：
//   - *tls.Config: Go 标准库的 TLS 配置
func (c *Config) ServerConfig(remotePeer types.PeerID) *tls.Config {
	clientAuth := tls.NoClientCert
	if c.RequireClientCert {
		clientAuth = tls.RequireAnyClientCert
	}
	
	return &tls.Config{
		Certificates:       []tls.Certificate{*c.Certificate},
		ClientAuth:         clientAuth,
		MinVersion:         c.MinVersion,
		InsecureSkipVerify: true, //nolint:gosec // G402: P2P 使用自定义 PeerID 验证替代 CA 验证
		NextProtos:         c.NextProtos,
		
		// 自定义证书验证（INV-001）
		VerifyPeerCertificate: func(rawCerts [][]byte, _ [][]*x509.Certificate) error {
			return VerifyPeerCertificate(rawCerts, remotePeer)
		},
	}
}

// ClientConfig 生成客户端 TLS 配置
//
// 用于出站连接（客户端握手）。
//
// 参数：
//   - remotePeer: 期望的远程 PeerID（用于验证）
//
// 返回：
//   - *tls.Config: Go 标准库的 TLS 配置
func (c *Config) ClientConfig(remotePeer types.PeerID) *tls.Config {
	return &tls.Config{
		Certificates:       []tls.Certificate{*c.Certificate},
		MinVersion:         c.MinVersion,
		InsecureSkipVerify: true, //nolint:gosec // G402: P2P 使用自定义 PeerID 验证替代 CA 验证
		ServerName:         c.ServerName,
		NextProtos:         c.NextProtos,
		
		// 自定义证书验证（INV-001）
		VerifyPeerCertificate: func(rawCerts [][]byte, _ [][]*x509.Certificate) error {
			return VerifyPeerCertificate(rawCerts, remotePeer)
		},
	}
}

// Clone 克隆配置
func (c *Config) Clone() *Config {
	return &Config{
		Identity:          c.Identity,
		Certificate:       c.Certificate,
		ServerName:        c.ServerName,
		NextProtos:        append([]string{}, c.NextProtos...),
		MinVersion:        c.MinVersion,
		RequireClientCert: c.RequireClientCert,
	}
}
