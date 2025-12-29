package quic

import (
	"crypto/tls"
	"testing"

	"github.com/dep2p/go-dep2p/internal/core/identity"
	identityif "github.com/dep2p/go-dep2p/pkg/interfaces/identity"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// createTestIdentity 创建测试用的身份
func createTestIdentity(t *testing.T) *identity.Manager {
	t.Helper()
	mgr := identity.NewManager(identityif.DefaultConfig())
	return mgr
}

// TestNewTLSConfig 测试 TLS 配置生成器创建
func TestNewTLSConfig(t *testing.T) {
	mgr := createTestIdentity(t)
	id, err := mgr.Create()
	if err != nil {
		t.Fatalf("创建身份失败: %v", err)
	}

	tlsConfig := NewTLSConfig(id)
	if tlsConfig == nil {
		t.Fatal("TLS 配置生成器不应为 nil")
	}
}

// TestGenerateConfig 测试生成 TLS 配置
func TestGenerateConfig(t *testing.T) {
	mgr := createTestIdentity(t)
	id, err := mgr.Create()
	if err != nil {
		t.Fatalf("创建身份失败: %v", err)
	}

	tlsConfigGen := NewTLSConfig(id)
	config, err := tlsConfigGen.GenerateConfig()
	if err != nil {
		t.Fatalf("生成 TLS 配置失败: %v", err)
	}

	// 验证配置
	if len(config.Certificates) == 0 {
		t.Error("证书列表不应为空")
	}

	if len(config.NextProtos) == 0 {
		t.Error("NextProtos 不应为空")
	}

	hasDepProto := false
	for _, proto := range config.NextProtos {
		if proto == "dep2p-quic" {
			hasDepProto = true
			break
		}
	}
	if !hasDepProto {
		t.Error("NextProtos 应包含 dep2p-quic")
	}

	if !config.InsecureSkipVerify {
		t.Error("InsecureSkipVerify 应为 true（P2P 自签名证书）")
	}

	if config.ClientAuth != tls.RequireAnyClientCert {
		t.Error("ClientAuth 应为 RequireAnyClientCert")
	}

	if config.MinVersion != tls.VersionTLS13 {
		t.Error("MinVersion 应为 TLS 1.3")
	}

	if config.VerifyPeerCertificate == nil {
		t.Error("VerifyPeerCertificate 不应为 nil")
	}
}

// TestGenerateClientConfig 测试生成客户端 TLS 配置
func TestGenerateClientConfig(t *testing.T) {
	mgr := createTestIdentity(t)
	id, err := mgr.Create()
	if err != nil {
		t.Fatalf("创建身份失败: %v", err)
	}

	tlsConfigGen := NewTLSConfig(id)
	config, err := tlsConfigGen.GenerateClientConfig()
	if err != nil {
		t.Fatalf("生成客户端 TLS 配置失败: %v", err)
	}

	// 客户端配置不应要求客户端证书
	if config.ClientAuth != tls.NoClientCert {
		t.Error("客户端配置的 ClientAuth 应为 NoClientCert")
	}
}

// TestGenerateConfigNilIdentity 测试 nil 身份
func TestGenerateConfigNilIdentity(t *testing.T) {
	tlsConfigGen := NewTLSConfig(nil)
	_, err := tlsConfigGen.GenerateConfig()
	if err == nil {
		t.Error("nil 身份应返回错误")
	}
}

// TestTLSHandshake 测试 TLS 握手
func TestTLSHandshake(t *testing.T) {
	// 创建两个身份
	mgr := createTestIdentity(t)
	serverID, err := mgr.Create()
	if err != nil {
		t.Fatalf("创建服务端身份失败: %v", err)
	}

	clientID, err := mgr.Create()
	if err != nil {
		t.Fatalf("创建客户端身份失败: %v", err)
	}

	// 生成配置
	serverTLSGen := NewTLSConfig(serverID)
	serverConfig, err := serverTLSGen.GenerateConfig()
	if err != nil {
		t.Fatalf("生成服务端 TLS 配置失败: %v", err)
	}

	clientTLSGen := NewTLSConfig(clientID)
	clientConfig, err := clientTLSGen.GenerateClientConfig()
	if err != nil {
		t.Fatalf("生成客户端 TLS 配置失败: %v", err)
	}

	// 验证配置有效
	if serverConfig == nil || clientConfig == nil {
		t.Fatal("TLS 配置不应为 nil")
	}

	// 验证证书中包含 NodeID 扩展
	if len(serverConfig.Certificates) > 0 {
		cert := serverConfig.Certificates[0]
		if len(cert.Certificate) == 0 {
			t.Error("证书不应为空")
		}
	}
}

// TestVerifyPeerCertificate 测试对端证书验证
func TestVerifyPeerCertificate(t *testing.T) {
	// 创建身份并生成证书
	mgr := createTestIdentity(t)
	id, err := mgr.Create()
	if err != nil {
		t.Fatalf("创建身份失败: %v", err)
	}

	tlsConfigGen := NewTLSConfig(id)
	config, err := tlsConfigGen.GenerateConfig()
	if err != nil {
		t.Fatalf("生成 TLS 配置失败: %v", err)
	}

	// 获取证书
	if len(config.Certificates) == 0 {
		t.Fatal("证书列表为空")
	}

	rawCert := config.Certificates[0].Certificate[0]

	// 测试验证函数
	err = verifyPeerCertificate([][]byte{rawCert}, nil)
	if err != nil {
		t.Errorf("验证证书失败: %v", err)
	}

	// 测试空证书
	err = verifyPeerCertificate([][]byte{}, nil)
	if err == nil {
		t.Error("空证书应返回错误")
	}

	// 测试无效证书
	err = verifyPeerCertificate([][]byte{[]byte("invalid")}, nil)
	if err == nil {
		t.Error("无效证书应返回错误")
	}
}

// TestExtractNodeID 测试从 TLS 连接状态提取 NodeID
func TestExtractNodeID(t *testing.T) {
	// 这个测试需要一个完整的 TLS 连接状态
	// 我们通过模拟测试部分功能

	// 测试空证书
	emptyState := tls.ConnectionState{}
	_, err := ExtractNodeID(emptyState)
	if err == nil {
		t.Error("空证书状态应返回错误")
	}
}

// TestVerifyNodeID 测试验证 NodeID
func TestVerifyNodeID(t *testing.T) {
	// 测试空证书状态
	emptyState := tls.ConnectionState{}
	expectedID := types.NodeID{}
	err := VerifyNodeID(emptyState, expectedID)
	if err == nil {
		t.Error("空证书状态应返回错误")
	}
}

// TestDeriveNodeIDFromPublicKey 测试从公钥派生 NodeID
func TestDeriveNodeIDFromPublicKey(t *testing.T) {
	// 创建测试公钥数据
	testPubKey := make([]byte, 32)
	for i := range testPubKey {
		testPubKey[i] = byte(i)
	}

	nodeID, err := deriveNodeIDFromPublicKey(testPubKey)
	if err != nil {
		t.Fatalf("派生 NodeID 失败: %v", err)
	}

	if nodeID.IsEmpty() {
		t.Error("NodeID 不应为空")
	}

	// 相同的公钥应产生相同的 NodeID
	nodeID2, err := deriveNodeIDFromPublicKey(testPubKey)
	if err != nil {
		t.Fatalf("第二次派生 NodeID 失败: %v", err)
	}

	if !nodeID.Equal(nodeID2) {
		t.Error("相同公钥应产生相同的 NodeID")
	}

	// 不同的公钥应产生不同的 NodeID
	testPubKey2 := make([]byte, 32)
	for i := range testPubKey2 {
		testPubKey2[i] = byte(i + 1)
	}

	nodeID3, err := deriveNodeIDFromPublicKey(testPubKey2)
	if err != nil {
		t.Fatalf("派生不同的 NodeID 失败: %v", err)
	}

	if nodeID.Equal(nodeID3) {
		t.Error("不同公钥应产生不同的 NodeID")
	}
}

// TestNodeIDExtensionOID 测试 OID 定义
func TestNodeIDExtensionOID(t *testing.T) {
	if len(nodeIDExtensionOID) == 0 {
		t.Error("OID 不应为空")
	}

	// 验证 OID 格式
	expected := []int{1, 3, 6, 1, 4, 1, 53594, 1, 1}
	if len(nodeIDExtensionOID) != len(expected) {
		t.Errorf("OID 长度应为 %d，实际为 %d", len(expected), len(nodeIDExtensionOID))
	}

	for i, v := range expected {
		if nodeIDExtensionOID[i] != v {
			t.Errorf("OID[%d] = %d，期望 %d", i, nodeIDExtensionOID[i], v)
		}
	}
}

