// Package crypto 提供 DeP2P 密码学工具
//
// 本包提供密钥生成、签名验证、序列化和安全存储等核心密码学功能。
//
// # 支持的密钥类型
//
//   - Ed25519（默认推荐）：高性能椭圆曲线签名
//   - Secp256k1（区块链兼容）：比特币/以太坊使用的曲线
//   - ECDSA（P-256）：NIST 标准曲线
//   - RSA（传统兼容）：用于需要 RSA 的场景
//
// # 快速开始
//
// 生成密钥对：
//
//	priv, pub, err := crypto.GenerateKeyPair(crypto.KeyTypeEd25519)
//
// 签名和验证：
//
//	sig, err := crypto.Sign(priv, data)
//	valid, err := crypto.Verify(pub, data, sig)
//
// 从公钥派生 PeerID：
//
//	peerID, err := crypto.PeerIDFromPublicKey(pub)
//
// 密钥存储：
//
//	ks, err := crypto.NewFSKeystore("/path/to/keys", password)
//	err = ks.Put("node-key", priv)
//	priv, err := ks.Get("node-key")
//
// # 安全特性
//
//   - 常量时间比较防止时序攻击
//   - AES-GCM + Argon2id 加密存储
//   - 安全清零敏感数据
//
// # 架构层
//
//   - 层级：pkg（公共包）
//   - 依赖：pkg/types
//   - 位置：Level 0（基础类型，无循环依赖）
//
// # 相关规范
//
//   - design/02_constraints/protocol/L1_identity/key_format.md
//   - design/02_constraints/protocol/L1_identity/signature.md
//   - pkg/proto/key/key.proto
package crypto
