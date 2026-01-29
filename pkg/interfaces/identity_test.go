package interfaces_test

import (
	"testing"

	"github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
// Mock 实现
// ============================================================================

// MockIdentity 模拟 Identity 接口实现
type MockIdentity struct {
	id     types.PeerID
	pubKey []byte
}

func NewMockIdentity(id types.PeerID) *MockIdentity {
	return &MockIdentity{
		id:     id,
		pubKey: []byte("mock-public-key"),
	}
}

func (m *MockIdentity) PeerID() string {
	return string(m.id)
}

func (m *MockIdentity) PublicKey() interfaces.PublicKey {
	return &MockPublicKey{data: m.pubKey}
}

func (m *MockIdentity) PrivateKey() interfaces.PrivateKey {
	return &MockPrivateKey{}
}

func (m *MockIdentity) Sign(data []byte) ([]byte, error) {
	return []byte("mock-signature"), nil
}

func (m *MockIdentity) Verify(data []byte, sig []byte) (bool, error) {
	return true, nil
}

// MockPublicKey 模拟 PublicKey 接口
type MockPublicKey struct {
	data []byte
}

func (m *MockPublicKey) Raw() ([]byte, error) {
	return m.data, nil
}

func (m *MockPublicKey) Type() interfaces.KeyType {
	return interfaces.KeyTypeEd25519
}

func (m *MockPublicKey) Equals(other interfaces.PublicKey) bool {
	return false
}

func (m *MockPublicKey) Verify(data, sig []byte) (bool, error) {
	return true, nil
}

// MockPrivateKey 模拟 PrivateKey 接口
type MockPrivateKey struct{}

func (m *MockPrivateKey) Raw() ([]byte, error) {
	return []byte("mock-private-key"), nil
}

func (m *MockPrivateKey) Type() interfaces.KeyType {
	return interfaces.KeyTypeEd25519
}

func (m *MockPrivateKey) PublicKey() interfaces.PublicKey {
	return &MockPublicKey{data: []byte("mock-public-key")}
}

func (m *MockPrivateKey) Equals(other interfaces.PrivateKey) bool {
	return false
}

func (m *MockPrivateKey) Sign(data []byte) ([]byte, error) {
	return []byte("signature"), nil
}


// ============================================================================
// 接口契约测试
// ============================================================================

// TestIdentityInterface 验证 Identity 接口存在
func TestIdentityInterface(t *testing.T) {
	var _ interfaces.Identity = (*MockIdentity)(nil)
}

// TestIdentity_PeerID 测试 PeerID 方法
func TestIdentity_PeerID(t *testing.T) {
	id := types.PeerID("test-peer-id")
	mock := NewMockIdentity(id)

	if mock.PeerID() != string(id) {
		t.Errorf("PeerID() = %v, want %v", mock.PeerID(), id)
	}
}

// TestIdentity_PublicKey 测试 PublicKey 方法
func TestIdentity_PublicKey(t *testing.T) {
	mock := NewMockIdentity(types.PeerID("test"))

	pubKey := mock.PublicKey()
	if pubKey == nil {
		t.Error("PublicKey() returned nil")
	}
}

// TestIdentity_Sign 测试 Sign 方法
func TestIdentity_Sign(t *testing.T) {
	mock := NewMockIdentity(types.PeerID("test"))

	sig, err := mock.Sign([]byte("data"))
	if err != nil {
		t.Errorf("Sign() failed: %v", err)
	}

	if len(sig) == 0 {
		t.Error("Sign() returned empty signature")
	}
}

// TestPublicKey_Raw 测试 PublicKey.Raw 方法
func TestPublicKey_Raw(t *testing.T) {
	pk := &MockPublicKey{data: []byte("test")}

	raw, err := pk.Raw()
	if err != nil {
		t.Errorf("Raw() failed: %v", err)
	}

	if len(raw) == 0 {
		t.Error("Raw() returned empty data")
	}
}
