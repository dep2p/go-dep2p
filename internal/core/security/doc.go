// Package security 实现安全传输
//
// Security 提供连接加密和身份验证功能，支持 TLS 1.3 协议。
//
// # 核心功能
//
//   - TLS 1.3 握手：使用自签名证书建立加密通道
//   - 证书生成：嵌入 Ed25519 公钥到证书扩展
//   - 身份验证：实现 INV-001（验证 PeerID 匹配）
//   - 前向保密：TLS 1.3 强制 ECDHE
//
// # 使用示例
//
//	import (
//	    "context"
//	    "github.com/dep2p/go-dep2p/internal/core/identity"
//	    "github.com/dep2p/go-dep2p/internal/core/security/tls"
//	)
//
//	// 创建身份
//	id, _ := identity.Generate()
//
//	// 创建 TLS 传输
//	transport, _ := tls.New(id)
//
//	// 服务器端握手
//	secureConn, _ := transport.SecureInbound(ctx, conn, remotePeer)
//
//	// 客户端握手
//	secureConn, _ := transport.SecureOutbound(ctx, conn, remotePeer)
//
// # Fx 模块
//
//	import "github.com/dep2p/go-dep2p/internal/core/security"
//
//	app := fx.New(
//	    identity.Module(),
//	    security.Module(),
//	)
//
// # INV-001 验证
//
// 所有 TLS 握手都强制执行 INV-001 身份验证：
//  1. 从证书提取 Ed25519 公钥
//  2. 派生 PeerID = Hash(PublicKey)
//  3. 验证 PeerID == ExpectedPeer
//  4. 不匹配则拒绝连接（返回 ErrPeerIDMismatch）
//
// # 架构设计
//
// 详见：design/03_architecture/L6_domains/core_security/
//
// # 相关模块
//
//   - internal/core/identity: 提供密钥和 PeerID
//   - internal/core/transport: 使用 TLS 配置保护 QUIC/TCP
//
// 架构层：Core Layer
// 公共接口：pkg/interfaces/security.go
package security
