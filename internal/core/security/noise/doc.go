// Package noise 实现 Noise 协议安全传输
//
// Noise 协议提供轻量级、高性能的安全传输，是 TLS 的替代方案。
// 本实现遵循 libp2p-noise 规范：
// https://github.com/libp2p/specs/blob/master/noise/README.md
//
// # 协议
//
// 使用 Noise_XX_25519_ChaChaPoly_SHA256 模式：
//   - XX: 三轮握手，双方相互认证
//   - 25519: Curve25519 用于 DH 密钥交换
//   - ChaChaPoly: ChaCha20-Poly1305 用于对称加密
//   - SHA256: 用于 HKDF 密钥派生
//
// # 握手流程
//
// Noise XX 三轮握手：
//
//	-> e                              (发起者发送临时公钥)
//	<- e, ee, s, es, payload          (响应者发送临时公钥、静态公钥、payload)
//	-> s, se, payload                 (发起者发送静态公钥、payload)
//
// payload 包含 Ed25519 身份公钥和签名，用于将 Noise 静态公钥
// 绑定到 libp2p 身份密钥。
//
// # 安全特性
//
//   - 前向保密（Forward Secrecy）
//   - 身份隐藏（Identity Hiding）
//   - 相互认证（Mutual Authentication）
//   - 抵抗重放攻击
//
// # 使用示例
//
//	import "github.com/dep2p/go-dep2p/internal/core/security/noise"
//
//	// 创建 Noise 传输
//	transport, err := noise.New(identity)
//	if err != nil {
//	    return err
//	}
//
//	// 作为客户端连接
//	secureConn, err := transport.SecureOutbound(ctx, conn, remotePeerID)
//
//	// 作为服务器接受连接
//	secureConn, err := transport.SecureInbound(ctx, conn, "")
package noise
