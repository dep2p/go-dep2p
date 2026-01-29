package mocks

import (
	"github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// MockIdentity 模拟 Identity 接口实现
type MockIdentity struct {
	// 基本属性
	PeerIDValue types.PeerID
	PubKeyValue interfaces.PublicKey
	PrivKeyValue interfaces.PrivateKey

	// 可覆盖的方法
	PeerIDFunc     func() string
	PublicKeyFunc  func() interfaces.PublicKey
	PrivateKeyFunc func() interfaces.PrivateKey
	SignFunc       func(data []byte) ([]byte, error)
	VerifyFunc     func(data []byte, sig []byte) (bool, error)
	IDFromPublicKeyFunc func() (string, error)
}

// NewMockIdentity 创建带有默认值的 MockIdentity
func NewMockIdentity(id types.PeerID) *MockIdentity {
	pubKey := &MockPublicKey{Data: []byte("mock-public-key")}
	privKey := &MockPrivateKey{PubKey: pubKey}
	return &MockIdentity{
		PeerIDValue:  id,
		PubKeyValue:  pubKey,
		PrivKeyValue: privKey,
	}
}

// PeerID 返回节点 ID
func (m *MockIdentity) PeerID() string {
	if m.PeerIDFunc != nil {
		return m.PeerIDFunc()
	}
	return string(m.PeerIDValue)
}

// PublicKey 返回公钥
func (m *MockIdentity) PublicKey() interfaces.PublicKey {
	if m.PublicKeyFunc != nil {
		return m.PublicKeyFunc()
	}
	return m.PubKeyValue
}

// PrivateKey 返回私钥
func (m *MockIdentity) PrivateKey() interfaces.PrivateKey {
	if m.PrivateKeyFunc != nil {
		return m.PrivateKeyFunc()
	}
	return m.PrivKeyValue
}

// Sign 签名数据
func (m *MockIdentity) Sign(data []byte) ([]byte, error) {
	if m.SignFunc != nil {
		return m.SignFunc(data)
	}
	return []byte("mock-signature"), nil
}

// Verify 验证签名
func (m *MockIdentity) Verify(data []byte, sig []byte) (bool, error) {
	if m.VerifyFunc != nil {
		return m.VerifyFunc(data, sig)
	}
	return true, nil
}

// IDFromPublicKey 从公钥导出 ID
func (m *MockIdentity) IDFromPublicKey() (string, error) {
	if m.IDFromPublicKeyFunc != nil {
		return m.IDFromPublicKeyFunc()
	}
	return string(m.PeerIDValue), nil
}

// 确保实现接口
var _ interfaces.Identity = (*MockIdentity)(nil)

// ============================================================================
// MockPublicKey
// ============================================================================

// MockPublicKey 模拟 PublicKey 接口
type MockPublicKey struct {
	Data []byte
	KeyType interfaces.KeyType

	// 可覆盖的方法
	RawFunc    func() ([]byte, error)
	TypeFunc   func() interfaces.KeyType
	EqualsFunc func(other interfaces.PublicKey) bool
	VerifyFunc func(data, sig []byte) (bool, error)
}

// Raw 返回原始字节
func (m *MockPublicKey) Raw() ([]byte, error) {
	if m.RawFunc != nil {
		return m.RawFunc()
	}
	return m.Data, nil
}

// Type 返回密钥类型
func (m *MockPublicKey) Type() interfaces.KeyType {
	if m.TypeFunc != nil {
		return m.TypeFunc()
	}
	if m.KeyType != 0 {
		return m.KeyType
	}
	return interfaces.KeyTypeEd25519
}

// Equals 比较是否相等
func (m *MockPublicKey) Equals(other interfaces.PublicKey) bool {
	if m.EqualsFunc != nil {
		return m.EqualsFunc(other)
	}
	return false
}

// Verify 验证签名
func (m *MockPublicKey) Verify(data, sig []byte) (bool, error) {
	if m.VerifyFunc != nil {
		return m.VerifyFunc(data, sig)
	}
	return true, nil
}

// 确保实现接口
var _ interfaces.PublicKey = (*MockPublicKey)(nil)

// ============================================================================
// MockPrivateKey
// ============================================================================

// MockPrivateKey 模拟 PrivateKey 接口
type MockPrivateKey struct {
	Data    []byte
	PubKey  *MockPublicKey
	KeyType interfaces.KeyType

	// 可覆盖的方法
	RawFunc       func() ([]byte, error)
	TypeFunc      func() interfaces.KeyType
	PublicKeyFunc func() interfaces.PublicKey
	EqualsFunc    func(other interfaces.PrivateKey) bool
	SignFunc      func(data []byte) ([]byte, error)
}

// Raw 返回原始字节
func (m *MockPrivateKey) Raw() ([]byte, error) {
	if m.RawFunc != nil {
		return m.RawFunc()
	}
	if m.Data != nil {
		return m.Data, nil
	}
	return []byte("mock-private-key"), nil
}

// Type 返回密钥类型
func (m *MockPrivateKey) Type() interfaces.KeyType {
	if m.TypeFunc != nil {
		return m.TypeFunc()
	}
	if m.KeyType != 0 {
		return m.KeyType
	}
	return interfaces.KeyTypeEd25519
}

// PublicKey 返回对应的公钥
func (m *MockPrivateKey) PublicKey() interfaces.PublicKey {
	if m.PublicKeyFunc != nil {
		return m.PublicKeyFunc()
	}
	if m.PubKey != nil {
		return m.PubKey
	}
	return &MockPublicKey{Data: []byte("mock-public-key")}
}

// Equals 比较是否相等
func (m *MockPrivateKey) Equals(other interfaces.PrivateKey) bool {
	if m.EqualsFunc != nil {
		return m.EqualsFunc(other)
	}
	return false
}

// Sign 签名数据
func (m *MockPrivateKey) Sign(data []byte) ([]byte, error) {
	if m.SignFunc != nil {
		return m.SignFunc(data)
	}
	return []byte("mock-signature"), nil
}

// 确保实现接口
var _ interfaces.PrivateKey = (*MockPrivateKey)(nil)
