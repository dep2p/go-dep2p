package upnp

import (
	"context"
	"testing"
	"time"
)

// MockIGDClient Mock UPnP IGD 客户端
type MockIGDClient struct {
	addMappingErr    error
	deleteMappingErr error
	externalIPErr    error
	externalIP       string
	mappings         map[uint16]bool
}

func (m *MockIGDClient) AddPortMapping(host string, extPort uint16, proto string, intPort uint16, client string, enabled bool, desc string, duration uint32) error {
	if m.addMappingErr != nil {
		return m.addMappingErr
	}
	if m.mappings == nil {
		m.mappings = make(map[uint16]bool)
	}
	m.mappings[extPort] = true
	return nil
}

func (m *MockIGDClient) DeletePortMapping(host string, extPort uint16, proto string) error {
	if m.deleteMappingErr != nil {
		return m.deleteMappingErr
	}
	delete(m.mappings, extPort)
	return nil
}

func (m *MockIGDClient) GetExternalIPAddress() (string, error) {
	if m.externalIPErr != nil {
		return "", m.externalIPErr
	}
	if m.externalIP == "" {
		return "203.0.113.1", nil // 默认返回测试用公网 IP
	}
	return m.externalIP, nil
}

// TestGetLocalIP 测试获取本地 IP
func TestGetLocalIP(t *testing.T) {
	ip, err := getLocalIP()
	if err != nil {
		t.Fatalf("getLocalIP failed: %v", err)
	}
	
	if ip == nil {
		t.Fatal("getLocalIP returned nil")
	}
	
	t.Logf("✅ Local IP: %s", ip)
}

// TestMapping_Creation 测试映射创建
func TestMapping_Creation(t *testing.T) {
	m := &Mapping{
		Protocol:     "UDP",
		InternalPort: 4001,
		ExternalPort: 4001,
		Duration:     3600,
		CreatedAt:    time.Now(),
	}
	
	if m.Protocol != "UDP" {
		t.Errorf("Protocol = %s, want UDP", m.Protocol)
	}
	
	t.Log("✅ Mapping 结构正确")
}

// TestUPnPMapper_MockMapPort 测试 Mock 映射
func TestUPnPMapper_MockMapPort(t *testing.T) {
	mapper := &UPnPMapper{
		client:   &MockIGDClient{},
		mappings: make(map[int]*Mapping),
	}
	
	ctx := context.Background()
	port, err := mapper.MapPort(ctx, "UDP", 4001)
	if err != nil {
		t.Fatalf("MapPort failed: %v", err)
	}
	
	if port != 4001 {
		t.Errorf("Mapped port = %d, want 4001", port)
	}
	
	// 检查映射记录
	if _, ok := mapper.mappings[4001]; !ok {
		t.Error("Mapping not recorded")
	}
	
	t.Log("✅ UPnP MapPort 正确")
}

// TestUPnPMapper_MockUnmapPort 测试 Mock 取消映射
func TestUPnPMapper_MockUnmapPort(t *testing.T) {
	mockClient := &MockIGDClient{
		mappings: make(map[uint16]bool),
	}
	mockClient.mappings[4001] = true
	
	mapper := &UPnPMapper{
		client:   mockClient,
		mappings: make(map[int]*Mapping),
	}
	
	err := mapper.UnmapPort("UDP", 4001)
	if err != nil {
		t.Fatalf("UnmapPort failed: %v", err)
	}
	
	// 检查是否被删除
	if mockClient.mappings[4001] {
		t.Error("Mapping was not deleted")
	}
	
	t.Log("✅ UPnP UnmapPort 正确")
}

// TestUPnPMapper_RenewMappings 测试续期
func TestUPnPMapper_RenewMappings(t *testing.T) {
	mapper := &UPnPMapper{
		client:   &MockIGDClient{},
		mappings: make(map[int]*Mapping),
	}
	
	// 添加一个旧映射（超过 2/3 租期）
	mapper.mappings[4001] = &Mapping{
		Protocol:     "UDP",
		InternalPort: 4001,
		ExternalPort: 4001,
		Duration:     3600,
		CreatedAt:    time.Now().Add(-2500 * time.Second), // 2500秒前
	}
	
	ctx := context.Background()
	mapper.renewMappings(ctx)
	
	// 续期后 CreatedAt 应该被更新
	if time.Since(mapper.mappings[4001].CreatedAt) > time.Second {
		t.Error("Mapping was not renewed")
	}
	
	t.Log("✅ UPnP 续期机制正确")
}
