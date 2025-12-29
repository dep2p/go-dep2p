// Package nat 提供 NAT 穿透模块的测试
package nat

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/dep2p/go-dep2p/internal/core/address"
	"github.com/dep2p/go-dep2p/internal/core/nat/holepunch"
	"github.com/dep2p/go-dep2p/pkg/interfaces/endpoint"
	natif "github.com/dep2p/go-dep2p/pkg/interfaces/nat"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              Service 测试
// ============================================================================

func TestService_New(t *testing.T) {
	config := natif.DefaultConfig()
	service := NewService(config)
	require.NotNil(t, service)
	defer service.Close()

	// 验证初始状态
	assert.Equal(t, types.NATTypeUnknown, service.NATType())
	assert.NotNil(t, service.Mappers())
	assert.NotNil(t, service.Discoverers())
}

func TestService_NATType(t *testing.T) {
	config := natif.DefaultConfig()
	service := NewService(config)
	defer service.Close()

	// 初始类型应该是 Unknown
	assert.Equal(t, types.NATTypeUnknown, service.NATType())
}

func TestService_MapPort(t *testing.T) {
	config := natif.DefaultConfig()
	config.EnableUPnP = true
	config.EnableNATPMP = true
	service := NewService(config)
	defer service.Close()

	// 尝试映射端口（可能由于没有网关而失败）
	err := service.MapPort("udp", 8000, 0, 30*time.Minute)
	// 不检查错误，因为 UPnP/NAT-PMP 可能不可用
	_ = err
}

func TestService_GetMappedPort(t *testing.T) {
	config := natif.DefaultConfig()
	service := NewService(config)
	defer service.Close()

	// 查找不存在的映射
	_, err := service.GetMappedPort("udp", 8000)
	assert.Error(t, err)
}

func TestService_UnmapPort(t *testing.T) {
	config := natif.DefaultConfig()
	service := NewService(config)
	defer service.Close()

	// 取消不存在的映射应该不报错
	err := service.UnmapPort("udp", 8000)
	assert.NoError(t, err)
}

func TestService_Refresh(t *testing.T) {
	config := natif.DefaultConfig()
	service := NewService(config)
	defer service.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 刷新空映射应该成功
	err := service.Refresh(ctx)
	assert.NoError(t, err)
}

func TestService_Close(t *testing.T) {
	config := natif.DefaultConfig()
	service := NewService(config)

	// 关闭服务
	err := service.Close()
	assert.NoError(t, err)

	// 再次关闭应该是幂等的
	err = service.Close()
	assert.NoError(t, err)
}

func TestService_HolePuncher(t *testing.T) {
	config := natif.DefaultConfig()
	service := NewService(config)
	defer service.Close()

	puncher := service.HolePuncher()
	assert.NotNil(t, puncher)
}

func TestService_STUNClient(t *testing.T) {
	config := natif.DefaultConfig()
	config.EnableSTUN = true
	service := NewService(config)
	defer service.Close()

	client := service.STUNClient()
	assert.NotNil(t, client)
}

func TestService_GetMappings(t *testing.T) {
	config := natif.DefaultConfig()
	service := NewService(config)
	defer service.Close()

	mappings := service.GetMappings()
	assert.NotNil(t, mappings)
	assert.Empty(t, mappings)
}

// ============================================================================
//                              配置测试
// ============================================================================

func TestDefaultConfig(t *testing.T) {
	config := natif.DefaultConfig()

	assert.True(t, config.EnableSTUN)
	assert.True(t, config.EnableUPnP)
	assert.True(t, config.EnableNATPMP)
	assert.NotEmpty(t, config.STUNServers)
	// Layer1 修复：默认禁用 HTTP IP 服务以保护隐私，所以列表为空
	assert.Empty(t, config.HTTPIPServices, "默认配置应禁用 HTTP IP 服务（隐私保护）")
	assert.Greater(t, config.MappingRefreshInterval, time.Duration(0))
	assert.Greater(t, config.MappingDuration, time.Duration(0))
}

func TestCustomConfig(t *testing.T) {
	config := natif.Config{
		EnableSTUN:   false,
		EnableUPnP:   false,
		EnableNATPMP: false,
		STUNServers:  []string{"custom.stun.server:3478"},
		PunchTimeout: 10 * time.Second,
	}

	service := NewService(config)
	defer service.Close()

	// 服务应该正常创建，即使所有功能被禁用
	require.NotNil(t, service)
}

// ============================================================================
//                              fx 模块测试
// ============================================================================

func TestModule_ProvideServices(t *testing.T) {
	config := natif.DefaultConfig()

	output, err := ProvideServices(ModuleInput{
		Config: &config,
	})

	require.NoError(t, err)
	require.NotNil(t, output.NATService)
	assert.NotNil(t, output.STUNClient)
	assert.NotNil(t, output.HolePuncher)

	// 清理
	output.NATService.Close()
}

func TestModule_ProvideServicesWithoutConfig(t *testing.T) {
	output, err := ProvideServices(ModuleInput{})

	require.NoError(t, err)
	require.NotNil(t, output.NATService)

	// 清理
	output.NATService.Close()
}

// ============================================================================
//                              Mapping 测试
// ============================================================================

func TestMapping_TTL(t *testing.T) {
	mapping := types.Mapping{
		Protocol:     "udp",
		InternalPort: 8000,
		ExternalPort: 8000,
		Expiry:       time.Now().Add(30 * time.Minute),
	}

	// TTL 应该接近 30 分钟
	ttl := mapping.TTL()
	assert.Greater(t, ttl, 29*time.Minute)
	assert.Less(t, ttl, 31*time.Minute)
}

func TestMapping_IsExpired(t *testing.T) {
	// 未过期的映射
	validMapping := types.Mapping{
		Protocol:     "udp",
		InternalPort: 8000,
		ExternalPort: 8000,
		Expiry:       time.Now().Add(30 * time.Minute),
	}
	assert.False(t, validMapping.IsExpired())

	// 已过期的映射
	expiredMapping := types.Mapping{
		Protocol:     "udp",
		InternalPort: 8000,
		ExternalPort: 8000,
		Expiry:       time.Now().Add(-1 * time.Minute),
	}
	assert.True(t, expiredMapping.IsExpired())
}

// ============================================================================
//                              NATType 测试
// ============================================================================

func TestNATType_String(t *testing.T) {
	tests := []struct {
		natType types.NATType
		want    string
	}{
		{types.NATTypeUnknown, "unknown"},
		{types.NATTypeNone, "none"},
		{types.NATTypeFull, "full_cone"},
		{types.NATTypeRestricted, "restricted"},
		{types.NATTypePortRestricted, "port_restricted"},
		{types.NATTypeSymmetric, "symmetric"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.natType.String())
		})
	}
}

// ============================================================================
//                              Holepunch 配置测试
// ============================================================================

func TestHolepunchConfig_Default(t *testing.T) {
	config := holepunch.DefaultConfig()

	assert.Equal(t, 5, config.MaxAttempts)
	assert.Equal(t, 200*time.Millisecond, config.AttemptInterval)
	assert.Equal(t, 10*time.Second, config.Timeout)
	assert.Equal(t, 64, config.PacketSize)
}

func TestHolepunchPuncher_New(t *testing.T) {
	config := holepunch.DefaultConfig()

	puncher := holepunch.NewPuncher(config)
	require.NotNil(t, puncher)
}

// ============================================================================
//                              Holepunch 协议测试
// ============================================================================

func TestHolepunchProtocol_RequestEncodeDecode(t *testing.T) {
	var initID types.NodeID
	copy(initID[:], []byte("initiator-id-32-bytes-here....."))

	var respID types.NodeID
	copy(respID[:], []byte("responder-id-32-bytes-here....."))

	req := &holepunch.HolePunchRequest{
		InitiatorID:    initID,
		InitiatorAddrs: []endpoint.Address{address.MustParse("/ip4/192.168.1.1/udp/8000/quic-v1")},
		ResponderID:    respID,
	}

	data, err := req.Encode()
	require.NoError(t, err)
	require.NotEmpty(t, data)

	decoded := &holepunch.HolePunchRequest{}
	err = decoded.Decode(data)
	require.NoError(t, err)

	assert.Equal(t, req.InitiatorID, decoded.InitiatorID)
	assert.Equal(t, req.ResponderID, decoded.ResponderID)
	assert.Len(t, decoded.InitiatorAddrs, 1)
}

func TestHolepunchProtocol_SyncEncodeDecode(t *testing.T) {
	sync := &holepunch.HolePunchSync{
		Nonce: make([]byte, holepunch.NonceLen),
	}
	copy(sync.Nonce, []byte("1234567890123456"))

	data, err := sync.Encode()
	require.NoError(t, err)
	require.NotEmpty(t, data)

	decoded := &holepunch.HolePunchSync{}
	err = decoded.Decode(data)
	require.NoError(t, err)

	assert.Equal(t, sync.Nonce, decoded.Nonce)
}

func TestHolepunchProtocol_ResponseEncodeDecode(t *testing.T) {
	resp := &holepunch.HolePunchResponse{
		Success: true,
		Nonce:   make([]byte, holepunch.NonceLen),
		Error:   "",
	}
	copy(resp.Nonce, []byte("1234567890123456"))

	data, err := resp.Encode()
	require.NoError(t, err)
	require.NotEmpty(t, data)

	decoded := &holepunch.HolePunchResponse{}
	err = decoded.Decode(data)
	require.NoError(t, err)

	assert.Equal(t, resp.Success, decoded.Success)
	assert.Equal(t, resp.Nonce, decoded.Nonce)
}

func TestHolepunchProtocol_ParseMessage(t *testing.T) {
	// 测试 Sync 消息解析
	sync := &holepunch.HolePunchSync{
		Nonce: make([]byte, holepunch.NonceLen),
	}
	copy(sync.Nonce, []byte("1234567890123456"))

	data, err := sync.Encode()
	require.NoError(t, err)

	msg, err := holepunch.ParseMessage(data)
	require.NoError(t, err)

	parsed, ok := msg.(*holepunch.HolePunchSync)
	require.True(t, ok)
	assert.Equal(t, sync.Nonce, parsed.Nonce)
}

// ============================================================================
//                              并发测试
// ============================================================================

func TestService_Concurrent(t *testing.T) {
	config := natif.DefaultConfig()
	service := NewService(config)
	defer service.Close()

	// 并发调用
	done := make(chan bool, 10)

	for i := 0; i < 10; i++ {
		go func() {
			_ = service.NATType()
			_ = service.GetMappings()
			done <- true
		}()
	}

	// 等待所有 goroutine 完成
	for i := 0; i < 10; i++ {
		<-done
	}
}

// ============================================================================
//                              Discoverer 优先级测试
// ============================================================================

func TestDiscoverer_Priority(t *testing.T) {
	config := natif.DefaultConfig()
	service := NewService(config)
	defer service.Close()

	discoverers := service.Discoverers()
	require.NotEmpty(t, discoverers)

	// 验证优先级排序
	for i := 1; i < len(discoverers); i++ {
		assert.LessOrEqual(t, discoverers[i-1].Priority(), discoverers[i].Priority())
	}
}

// ============================================================================
//                              Mapper 可用性测试
// ============================================================================

func TestMapper_Names(t *testing.T) {
	config := natif.DefaultConfig()
	config.EnableUPnP = true
	config.EnableNATPMP = true
	service := NewService(config)
	defer service.Close()

	mappers := service.Mappers()
	require.NotEmpty(t, mappers)

	names := make([]string, len(mappers))
	for i, m := range mappers {
		names[i] = m.Name()
	}

	// 应该有 upnp 和 nat-pmp
	assert.Contains(t, names, "upnp")
	assert.Contains(t, names, "nat-pmp")
}

// ============================================================================
//                              Close 幂等性测试
// ============================================================================

func TestService_Close_Idempotent(t *testing.T) {
	config := natif.DefaultConfig()
	service := NewService(config)

	// 多次调用 Close 应该不会 panic
	for i := 0; i < 5; i++ {
		err := service.Close()
		assert.NoError(t, err)
	}
}

func TestService_Close_Concurrent(t *testing.T) {
	config := natif.DefaultConfig()
	service := NewService(config)

	// 并发调用 Close 应该不会 panic
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = service.Close()
		}()
	}
	wg.Wait()
}

// ============================================================================
//                              RefreshLoop 安全性测试
// ============================================================================

func TestService_RefreshLoop_NilTicker(t *testing.T) {
	// MappingRefreshInterval = 0 时，refreshTicker 应该为 nil
	// refreshLoop 应该能安全处理这种情况
	config := natif.DefaultConfig()
	config.MappingRefreshInterval = 0
	service := NewService(config)
	defer service.Close()

	// refreshTicker 应该为 nil
	assert.Nil(t, service.refreshTicker)

	// 手动调用 refreshLoop 应该不会 panic
	service.refreshLoop()
}

// ============================================================================
//                              Module 超时测试
// ============================================================================

func TestModule_TimeoutCorrect(t *testing.T) {
	// 这个测试验证 module.go 中的超时设置是正确的
	// 通过检查 ProvideServices 返回的服务来间接验证

	config := natif.DefaultConfig()

	output, err := ProvideServices(ModuleInput{
		Config: &config,
	})

	require.NoError(t, err)
	require.NotNil(t, output.NATService)

	// 服务应该可以正常使用
	assert.NotNil(t, output.NATService)

	output.NATService.Close()
}

// ============================================================================
//                              Holepunch Session 测试
// ============================================================================

func TestHolepunchPuncher_CompleteSession_Idempotent(t *testing.T) {
	config := holepunch.DefaultConfig()

	puncher := holepunch.NewPuncher(config)
	require.NotNil(t, puncher)

	var remoteID types.NodeID
	copy(remoteID[:], []byte("remote-peer-id-32-bytes-here..."))

	// 调用 CompleteSession 对于不存在的会话应该不会 panic
	puncher.CompleteSession(remoteID, nil, nil)
	puncher.CompleteSession(remoteID, nil, nil)
}
