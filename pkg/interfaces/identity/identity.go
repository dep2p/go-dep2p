// Package identity 定义身份管理相关接口
//
// 身份模块负责节点的密码学身份管理，包括：
// - 密钥对生成
// - 签名和验证
// - 身份持久化
// - 密钥接口定义（合并自原 crypto 包）
package identity

import (
	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              Identity 接口
// ============================================================================

// Identity 节点身份接口
//
// Identity 代表节点的密码学身份，包含公钥和私钥。
// 节点 ID 由公钥派生，是节点在网络中的唯一标识。
//
// ⚠️ 安全边界说明（v1.1）：
// - PrivateKey() 方法返回私钥对象，是系统最敏感的 API
// - 允许用途：TLS 证书生成、消息签名、身份持久化
// - 禁止用途：日志输出、API 响应、传递给不受信任的组件
// - 推荐做法：优先使用 Sign() 方法进行签名，避免直接获取 PrivateKey
type Identity interface {
	// ID 返回节点 ID
	// NodeID 由公钥派生，是节点的唯一标识
	ID() types.NodeID

	// PublicKey 返回公钥
	PublicKey() PublicKey

	// PrivateKey 返回私钥
	//
	// ⚠️ 安全敏感：返回私钥对象，调用方应严格控制使用场景。
	// 允许用途：TLS 证书生成（需要 crypto.Signer）、ed25519 原始签名
	// 禁止用途：日志、网络传输、业务层缓存
	// 推荐做法：优先使用 Identity.Sign() 进行签名操作
	PrivateKey() PrivateKey

	// Sign 签名数据
	// 使用私钥对数据进行签名
	// 推荐：优先使用此方法，而非 PrivateKey().Sign()
	Sign(data []byte) ([]byte, error)

	// Verify 验证签名
	// 验证指定公钥对数据的签名是否有效
	Verify(data, signature []byte, pubKey PublicKey) (bool, error)

	// KeyType 返回密钥类型
	KeyType() types.KeyType
}

// ============================================================================
//                              IdentityManager 接口
// ============================================================================

// IdentityManager 身份管理器接口
//
// 提供身份的创建、加载、保存等管理功能。
type IdentityManager interface {
	// Create 创建新身份
	// 生成新的密钥对并创建身份
	Create() (Identity, error)

	// CreateWithType 创建指定类型的身份
	CreateWithType(keyType types.KeyType) (Identity, error)

	// Load 从文件加载身份
	// path 为身份文件路径
	Load(path string) (Identity, error)

	// Save 保存身份到文件
	// 将身份（私钥）保存到指定路径
	Save(identity Identity, path string) error

	// 注意：以下方法已删除（v1.1 清理）- 仅测试使用，核心路径不需要：
	// - FromPrivateKey(key PrivateKey) (Identity, error)
	// - FromBytes(privateKeyBytes []byte, keyType types.KeyType) (Identity, error)
	// - GenerateNodeID(pubKey PublicKey) types.NodeID
	// 测试可直接使用 internal/core/identity.Manager 的对应方法
}

// ============================================================================
//                              配置选项
// ============================================================================

// 注意：KeyStore 接口已被删除（无实现、无使用）。
// 密钥存储功能由 IdentityManager 的 Load/Save 方法提供。

// Config 身份模块配置
type Config struct {
	// KeyType 默认密钥类型
	KeyType types.KeyType

	// IdentityPath 身份文件路径（私钥 PEM 文件）
	// 如果为空，将在内存中生成临时身份
	IdentityPath string

	// AutoCreate 如果不存在是否自动创建身份
	AutoCreate bool

	// PrivateKey 直接注入的私钥（可选）
	//
	// 如果提供，将跳过文件加载/生成，直接使用此私钥创建身份。
	// 用于 WithIdentity(key) 场景。
	//
	// 优先级：PrivateKey > IdentityPath > AutoCreate
	PrivateKey PrivateKey
}

// DefaultConfig 返回默认配置
func DefaultConfig() Config {
	return Config{
		KeyType:      types.KeyTypeEd25519,
		IdentityPath: "",
		AutoCreate:   true,
		PrivateKey:   nil,
	}
}
