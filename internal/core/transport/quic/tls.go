// Package quic 实现 QUIC 传输
package quic

import (
	"crypto/tls"
	"fmt"

	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	securitytls "github.com/dep2p/go-dep2p/internal/core/security/tls"
)

// NewTLSConfig 从 Identity 创建 TLS 配置
//
// QUIC 使用 TLS 1.3 作为加密层，此函数从 core_identity 获取密钥
// 并生成符合 libp2p 规范的 TLS 证书。
//
// 参数：
//   - id: 节点身份
//
// 返回：
//   - *tls.Config: 服务器 TLS 配置
//   - *tls.Config: 客户端 TLS 配置
//   - error: 创建失败时的错误
func NewTLSConfig(id pkgif.Identity) (serverConf *tls.Config, clientConf *tls.Config, err error) {
	if id == nil {
		return nil, nil, fmt.Errorf("identity is nil")
	}
	
	// 使用 core_security/tls 的配置生成器
	cfg, err := securitytls.NewFromIdentity(id)
	if err != nil {
		return nil, nil, fmt.Errorf("create tls config: %w", err)
	}
	
	// 生成服务器配置（用于监听）
	serverConf = cfg.ServerConfig("")
	
	// 生成客户端配置（用于拨号）
	clientConf = cfg.ClientConfig("")
	
	return serverConf, clientConf, nil
}
