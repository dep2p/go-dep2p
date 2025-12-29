// Package identity 定义密钥和身份相关接口
//
// 这些接口从原 crypto 包合并而来，用于密钥管理。
// 提供公钥和私钥的抽象接口，支持多种密钥算法。
package identity

import (
	"crypto"
	"crypto/sha256"

	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              PublicKey 接口
// ============================================================================

// PublicKey 公钥接口
//
// 公钥用于:
// - 验证签名
// - 派生 NodeID
// - 加密数据（某些算法）
//
// 安全边界说明（v1.1）：
// - Bytes()/Raw() 暴露密钥材料，允许用于：TLS 证书生成、签名验证、消息认证
// - 禁止用途：将公钥字节写入日志（可能泄露关联身份信息）
type PublicKey interface {
	// Bytes 返回公钥的字节表示
	//
	// 允许用途：NodeID 派生、签名验证、消息认证（如 gossipsub 消息签名）
	Bytes() []byte

	// Equal 比较两个公钥是否相等
	Equal(other PublicKey) bool

	// Verify 使用公钥验证签名
	// data: 原始数据
	// signature: 签名
	// 返回 true 表示签名有效
	Verify(data, signature []byte) (bool, error)

	// Type 返回密钥类型
	Type() types.KeyType

	// Raw 返回底层密钥（如 *ecdsa.PublicKey）
	//
	// 允许用途：与标准库或其他库互操作（如 TLS 证书生成）
	// 禁止用途：在 API 响应或日志中暴露
	Raw() crypto.PublicKey
}

// ============================================================================
//                              PrivateKey 接口
// ============================================================================

// PrivateKey 私钥接口
//
// 私钥用于:
// - 签名数据
// - 证明身份
// - 解密数据（某些算法）
//
// ⚠️ 安全边界说明（v1.1）：
// 私钥材料是系统最敏感的资产。以下方法暴露原始密钥材料，必须严格控制调用场景：
// - Bytes()/Raw() 允许用途：TLS 证书生成、身份持久化（PEM 序列化）、签名
// - 禁止用途：日志输出、网络传输、业务层缓存、传递给不受信任的第三方库
// - 推荐做法：获取后立即使用，不存储到长生命周期结构体；使用完毕后（如签名）不主动置零（Go 无法保证内存清除）
type PrivateKey interface {
	// PublicKey 返回对应的公钥
	PublicKey() PublicKey

	// Sign 使用私钥签名数据
	// 返回签名字节
	Sign(data []byte) ([]byte, error)

	// Bytes 返回私钥的字节表示
	//
	// ⚠️ 安全敏感：私钥字节是系统最敏感的数据。
	// 允许用途：身份持久化（PEM 序列化）、测试断言
	// 禁止用途：日志、网络传输、业务层缓存
	Bytes() []byte

	// Type 返回密钥类型
	Type() types.KeyType

	// Raw 返回底层密钥（如 *ecdsa.PrivateKey）
	//
	// ⚠️ 安全敏感：暴露底层密钥对象。
	// 允许用途：TLS 证书生成（需要 crypto.Signer）、与标准库互操作
	// 禁止用途：日志、网络传输、传递给不受信任的第三方库
	Raw() crypto.PrivateKey
}

// ============================================================================
//                              KeyGenerator（v1.1 已删除）
// ============================================================================

// 注意：KeyPair 接口已删除（v1.1 清理）。
// PrivateKey.PublicKey() 已满足密钥对需求。

// 注意：KeyGenerator 接口已删除（v1.1 清理）。
// 原因：仅测试使用，核心路径使用 GenerateEd25519KeyPair() 函数。
// 测试可直接使用 internal/core/identity.Ed25519KeyGenerator。

// ============================================================================
//                              辅助函数类型
// ============================================================================

// GenerateKeyPairFunc 密钥对生成函数类型
type GenerateKeyPairFunc func(keyType types.KeyType) (PrivateKey, PublicKey, error)

// NodeIDFromPublicKeyFunc 从公钥派生 NodeID 的函数类型
type NodeIDFromPublicKeyFunc func(pubKey PublicKey) types.NodeID

// ============================================================================
//                              KeyFactory 接口
// ============================================================================

// KeyFactory 密钥工厂接口
//
// 用于从字节创建公钥/私钥对象。
// 通过 Fx 依赖注入提供，避免跨组件直接依赖实现包。
type KeyFactory interface {
	// PublicKeyFromBytes 从字节创建公钥
	//
	// 根据 keyType 自动选择对应的密钥实现。
	// 支持：Ed25519、ECDSA P-256、ECDSA P-384
	PublicKeyFromBytes(keyBytes []byte, keyType types.KeyType) (PublicKey, error)

	// PrivateKeyFromBytes 从字节创建私钥
	//
	// 根据 keyType 自动选择对应的密钥实现。
	PrivateKeyFromBytes(keyBytes []byte, keyType types.KeyType) (PrivateKey, error)
}

// ============================================================================
//                              公共工具函数
// ============================================================================

// NodeIDFromPublicKey 从公钥派生 NodeID
//
// 使用 SHA256(PublicKeyBytes) 作为 NodeID。
// 这确保了 NodeID 与公钥之间的唯一对应关系。
//
// 这是一个纯函数，不依赖具体密钥实现，可安全用于任何组件。
func NodeIDFromPublicKey(pubKey PublicKey) types.NodeID {
	hash := sha256.Sum256(pubKey.Bytes())
	return types.NodeID(hash)
}

