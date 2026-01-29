// Package identity 实现 DeP2P 的身份管理功能
//
// 本包提供密钥对管理、PeerID 派生和签名验证等核心功能。
//
// # 核心功能
//
// 1. 密钥对管理：
//   - Ed25519 密钥生成（默认）
//   - 密钥序列化（Raw 和 PEM 格式）
//   - 密钥持久化
//
// 2. PeerID 派生：
//   - 从公钥派生 PeerID（Base58 编码的 Multihash）
//   - PeerID 验证
//
// 3. 签名与验证：
//   - 数据签名（Ed25519）
//   - 签名验证
//   - 批量签名/验证
//
// # 快速开始
//
//	// 生成新身份
//	priv, pub, _ := identity.GenerateEd25519Key()
//	id, _ := identity.New(priv)
//
//	// 获取 PeerID
//	peerID := id.PeerID()
//
//	// 签名和验证
//	sig, _ := id.Sign([]byte("data"))
//	valid, _ := id.Verify([]byte("data"), sig)
//
// # Fx 模块
//
//	import "go.uber.org/fx"
//
//	app := fx.New(
//	    identity.Module(),
//	    fx.Invoke(func(id pkgif.Identity) {
//	        fmt.Printf("PeerID: %s\n", id.PeerID())
//	    }),
//	)
//
// # 架构定位
//
// Tier: Core Layer Level 1（无依赖）
//
// 依赖关系：
//   - 依赖：pkg/types, pkg/interfaces
//   - 被依赖：peerstore, security, host
//
// # 性能
//
//   - 密钥生成：~24µs
//   - PeerID 派生：~1.8µs
//   - 签名：~28µs（< 1ms 要求）
//   - 验证：~65µs（< 1ms 要求）
//
// # 相关文档
//
//   - 设计文档：design/03_architecture/L6_domains/core_identity/
//   - 接口定义：pkg/interfaces/identity.go
package identity
