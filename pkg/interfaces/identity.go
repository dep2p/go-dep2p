// Package interfaces 定义 DeP2P 公共接口
//
// 本文件定义 Identity 接口，管理节点身份和密钥。
package interfaces

// Identity 定义身份管理接口
//
// Identity 提供密钥生成、签名和验证功能。
type Identity interface {
	// PeerID 返回节点 ID（从公钥派生）
	PeerID() string

	// PublicKey 返回公钥
	PublicKey() PublicKey

	// PrivateKey 返回私钥
	PrivateKey() PrivateKey

	// Sign 使用私钥签名数据
	Sign(data []byte) ([]byte, error)

	// Verify 验证签名
	Verify(data []byte, sig []byte) (bool, error)
}

// PublicKey 定义公钥接口
type PublicKey interface {
	// Raw 返回原始公钥字节
	Raw() ([]byte, error)

	// Type 返回密钥类型
	Type() KeyType

	// Equals 比较两个公钥是否相等
	Equals(other PublicKey) bool

	// Verify 使用此公钥验证签名
	Verify(data, sig []byte) (bool, error)
}

// PrivateKey 定义私钥接口
type PrivateKey interface {
	// Raw 返回原始私钥字节
	Raw() ([]byte, error)

	// Type 返回密钥类型
	Type() KeyType

	// PublicKey 返回对应的公钥
	PublicKey() PublicKey

	// Equals 比较两个私钥是否相等
	Equals(other PrivateKey) bool

	// Sign 使用此私钥签名数据
	Sign(data []byte) ([]byte, error)
}

// KeyType 密钥类型
//
// 值与 pkg/crypto 和 pkg/proto/key/key.proto 保持一致：
//   - KEY_TYPE_UNSPECIFIED = 0
//   - RSA = 1
//   - Ed25519 = 2
//   - Secp256k1 = 3
//   - ECDSA = 4
type KeyType int

const (
	// KeyTypeUnspecified 未指定密钥类型
	KeyTypeUnspecified KeyType = 0
	// KeyTypeRSA RSA 密钥
	KeyTypeRSA KeyType = 1
	// KeyTypeEd25519 Ed25519 密钥（默认推荐）
	KeyTypeEd25519 KeyType = 2
	// KeyTypeSecp256k1 Secp256k1 密钥（区块链兼容）
	KeyTypeSecp256k1 KeyType = 3
	// KeyTypeECDSA ECDSA 密钥
	KeyTypeECDSA KeyType = 4
)

// String 返回密钥类型名称
func (kt KeyType) String() string {
	switch kt {
	case KeyTypeUnspecified:
		return "Unspecified"
	case KeyTypeRSA:
		return "RSA"
	case KeyTypeEd25519:
		return "Ed25519"
	case KeyTypeSecp256k1:
		return "Secp256k1"
	case KeyTypeECDSA:
		return "ECDSA"
	default:
		return "Unknown"
	}
}
