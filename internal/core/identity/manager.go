package identity

import (
	"fmt"

	cryptoif "github.com/dep2p/go-dep2p/pkg/interfaces/identity"
	identityif "github.com/dep2p/go-dep2p/pkg/interfaces/identity"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              Manager 实现
// ============================================================================

// Manager 身份管理器实现
type Manager struct {
	config identityif.Config
}

// 确保实现接口
var _ identityif.IdentityManager = (*Manager)(nil)

// NewManager 创建身份管理器
func NewManager(config identityif.Config) *Manager {
	return &Manager{config: config}
}

// Create 创建新身份
func (m *Manager) Create() (identityif.Identity, error) {
	return m.CreateWithType(m.config.KeyType)
}

// CreateWithType 创建指定类型的身份
func (m *Manager) CreateWithType(keyType types.KeyType) (identityif.Identity, error) {
	var priv cryptoif.PrivateKey
	var err error

	switch keyType {
	case types.KeyTypeEd25519:
		priv, _, err = GenerateEd25519KeyPair()
	default:
		return nil, ErrUnsupportedKeyType
	}

	if err != nil {
		return nil, fmt.Errorf("生成密钥对失败: %w", err)
	}

	return NewIdentity(priv), nil
}

// Load 从文件加载身份
func (m *Manager) Load(path string) (identityif.Identity, error) {
	priv, err := LoadPrivateKeyPEM(path)
	if err != nil {
		return nil, err
	}
	return NewIdentity(priv), nil
}

// Save 保存身份到文件
func (m *Manager) Save(id identityif.Identity, path string) error {
	return SavePrivateKeyPEM(id.PrivateKey(), path)
}

// FromPrivateKey 从私钥创建身份
func (m *Manager) FromPrivateKey(key cryptoif.PrivateKey) (identityif.Identity, error) {
	return NewIdentity(key), nil
}

// FromBytes 从字节创建身份
func (m *Manager) FromBytes(privateKeyBytes []byte, keyType types.KeyType) (identityif.Identity, error) {
	priv, err := PrivateKeyFromBytes(privateKeyBytes, keyType)
	if err != nil {
		return nil, err
	}
	return NewIdentity(priv), nil
}

// GenerateNodeID 从公钥生成节点ID
func (m *Manager) GenerateNodeID(pubKey cryptoif.PublicKey) types.NodeID {
	return NodeIDFromPublicKey(pubKey)
}

