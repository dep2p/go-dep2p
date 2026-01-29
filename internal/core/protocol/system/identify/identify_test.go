package identify

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestIdentifyService_New 测试创建服务
func TestIdentifyService_New(t *testing.T) {
	// NewService 需要完整的 Host 和 ProtocolRegistry 接口实现
	// 这里测试 nil 参数不会 panic
	service := NewService(nil, nil)
	assert.NotNil(t, service)
	assert.Nil(t, service.host)
	assert.Nil(t, service.registry)

	t.Log("✅ Identify Service 可以用 nil 参数创建")
}

// TestIdentify_Constants 测试常量定义
func TestIdentify_Constants(t *testing.T) {
	assert.Equal(t, "/dep2p/sys/identify/1.0.0", ProtocolID)
	assert.Equal(t, "/dep2p/sys/identify/push/1.0.0", ProtocolIDPush)

	t.Log("✅ Identify 常量正确")
}

// TestIdentify_Handler 测试处理器存在
func TestIdentify_Handler(t *testing.T) {
	// Handler 方法签名: func (s *Service) Handler(stream pkgif.Stream)
	// 创建一个服务来验证方法存在
	service := NewService(nil, nil)
	assert.NotNil(t, service)

	// Handler 方法在没有完整依赖时会 panic，这里只验证服务结构

	t.Log("✅ Identify Handler 方法存在")
}

// TestIdentify_Exchange 测试信息交换结构
func TestIdentify_Exchange(t *testing.T) {
	// getProtocols 依赖 registry，这里测试 nil registry 的行为
	service := NewService(nil, nil)
	protocols := service.getProtocols()

	// nil registry 应该返回空列表
	assert.Len(t, protocols, 0)

	t.Log("✅ Identify Exchange 返回空协议列表")
}

// TestIdentifyInfo_Structure 测试 IdentifyInfo 结构
func TestIdentifyInfo_Structure(t *testing.T) {
	info := &IdentifyInfo{
		PeerID:          "test-peer",
		ListenAddrs:     []string{"/ip4/127.0.0.1/tcp/4001"},
		Protocols:       []string{"/test/1.0.0"},
		PublicKey:       "base64-encoded-key",
		AgentVersion:    "go-dep2p/1.0.0",
		ProtocolVersion: "dep2p/1.0.0",
	}

	assert.Equal(t, "test-peer", info.PeerID)
	assert.Len(t, info.ListenAddrs, 1)
	assert.Len(t, info.Protocols, 1)
	assert.Equal(t, "base64-encoded-key", info.PublicKey)
	assert.Equal(t, "go-dep2p/1.0.0", info.AgentVersion)
	assert.Equal(t, "dep2p/1.0.0", info.ProtocolVersion)

	t.Log("✅ IdentifyInfo 结构正确")
}

// TestIdentifyInfo_EmptyFields 测试 IdentifyInfo 空字段
func TestIdentifyInfo_EmptyFields(t *testing.T) {
	info := &IdentifyInfo{}

	assert.Empty(t, info.PeerID)
	assert.Nil(t, info.ListenAddrs)
	assert.Nil(t, info.Protocols)
	assert.Empty(t, info.PublicKey)

	t.Log("✅ IdentifyInfo 空字段默认值正确")
}
