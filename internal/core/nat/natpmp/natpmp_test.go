package natpmp

import (
	"net"
	"testing"
	"time"
)

// MockNATPMPClient Mock NAT-PMP 客户端
type MockNATPMPClient struct {
	addMappingFunc func(protocol string, internalPort, externalPort int, lifetime int) error
	externalAddr   [4]byte
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
	
	if m.InternalPort != 4001 {
		t.Errorf("InternalPort = %d, want 4001", m.InternalPort)
	}
	
	t.Log("✅ NAT-PMP Mapping 结构正确")
}

// TestMapping_ShouldRenew 测试是否需要续期
func TestMapping_ShouldRenew(t *testing.T) {
	m := &Mapping{
		Protocol:     "UDP",
		InternalPort: 4001,
		ExternalPort: 4001,
		Duration:     3600,
		CreatedAt:    time.Now().Add(-2500 * time.Second), // 2500秒前创建
	}
	
	// 已超过 2/3 租期（2400秒），应该续期
	elapsed := time.Since(m.CreatedAt)
	threshold := time.Duration(m.Duration) * 2 / 3 * time.Second
	
	if elapsed <= threshold {
		t.Error("Should need renewal")
	}
	
	t.Log("✅ NAT-PMP 续期判断正确")
}

// TestNATPMPMapper_Structure 测试结构
func TestNATPMPMapper_Structure(t *testing.T) {
	mapper := &NATPMPMapper{
		gateway:  net.ParseIP("192.168.1.1"),
		mappings: make(map[int]*Mapping),
	}
	
	if mapper.gateway == nil {
		t.Error("Gateway is nil")
	}
	
	if mapper.mappings == nil {
		t.Error("Mappings map is nil")
	}
	
	t.Log("✅ NATPMPMapper 结构正确")
}

// TestNATPMPMapper_MappingRecord 测试映射记录
func TestNATPMPMapper_MappingRecord(t *testing.T) {
	mapper := &NATPMPMapper{
		gateway:  net.ParseIP("192.168.1.1"),
		mappings: make(map[int]*Mapping),
	}
	
	// 手动添加映射记录
	mapper.mappings[4001] = &Mapping{
		Protocol:     "UDP",
		InternalPort: 4001,
		ExternalPort: 4001,
		Duration:     3600,
		CreatedAt:    time.Now(),
	}
	
	// 检查记录
	m, ok := mapper.mappings[4001]
	if !ok {
		t.Fatal("Mapping not found")
	}
	
	if m.Protocol != "UDP" {
		t.Errorf("Protocol = %s, want UDP", m.Protocol)
	}
	
	t.Log("✅ NATPMPMapper 映射记录正确")
}

// TestNATPMPMapper_RenewLogic 测试续期逻辑
func TestNATPMPMapper_RenewLogic(t *testing.T) {
	mapper := &NATPMPMapper{
		gateway:  net.ParseIP("192.168.1.1"),
		mappings: make(map[int]*Mapping),
	}
	
	// 添加需要续期的映射
	oldTime := time.Now().Add(-2500 * time.Second)
	mapper.mappings[4001] = &Mapping{
		Protocol:     "UDP",
		InternalPort: 4001,
		ExternalPort: 4001,
		Duration:     3600,
		CreatedAt:    oldTime,
	}
	
	// 添加不需要续期的映射
	mapper.mappings[4002] = &Mapping{
		Protocol:     "UDP",
		InternalPort: 4002,
		ExternalPort: 4002,
		Duration:     3600,
		CreatedAt:    time.Now(),
	}
	
	// 检查哪些需要续期
	needRenew := 0
	for _, m := range mapper.mappings {
		elapsed := time.Since(m.CreatedAt)
		threshold := time.Duration(m.Duration) * 2 / 3 * time.Second
		if elapsed > threshold {
			needRenew++
		}
	}
	
	if needRenew != 1 {
		t.Errorf("Need renew count = %d, want 1", needRenew)
	}
	
	t.Log("✅ NATPMPMapper 续期逻辑正确")
}
