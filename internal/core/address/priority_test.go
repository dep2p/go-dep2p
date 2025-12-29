package address

import (
	"sort"
	"testing"
)

// ============================================================================
//                    GAP-013: Address 优先级排序测试
// ============================================================================

// TestAddressTypePriority 验证地址类型优先级
//
// 设计要求（docs/01-design/protocols/foundation/02-address.md 第 140-145 行）：
// | 优先级 | 类型           |
// |--------|----------------|
// | 1      | 公网直连地址    |
// | 2      | 局域网地址      |
// | 3      | NAT 映射地址    |
// | 4      | 中继地址        |
func TestAddressTypePriority(t *testing.T) {
	// 验证优先级数值：Public > LAN > NATMapped > Relay
	publicPriority := AddressTypePublic.BasePriority()
	lanPriority := AddressTypeLAN.BasePriority()
	natPriority := AddressTypeNATMapped.BasePriority()
	relayPriority := AddressTypeRelay.BasePriority()

	if publicPriority <= lanPriority {
		t.Errorf("Public priority (%d) should be > LAN priority (%d)", publicPriority, lanPriority)
	}

	if lanPriority <= natPriority {
		t.Errorf("LAN priority (%d) should be > NAT priority (%d)", lanPriority, natPriority)
	}

	if natPriority <= relayPriority {
		t.Errorf("NAT priority (%d) should be > Relay priority (%d)", natPriority, relayPriority)
	}

	t.Logf("Priority order verified: Public(%d) > LAN(%d) > NAT(%d) > Relay(%d)",
		publicPriority, lanPriority, natPriority, relayPriority)
}

// TestAddressTypePriorityValues 验证具体优先级值
func TestAddressTypePriorityValues(t *testing.T) {
	tests := []struct {
		addrType     AddressType
		expectedBase int
	}{
		{AddressTypePublic, 80},
		{AddressTypeLAN, 70},
		{AddressTypeNATMapped, 60},
		{AddressTypeRelay, 40},
	}

	for _, tt := range tests {
		priority := tt.addrType.BasePriority()
		if priority != tt.expectedBase {
			t.Errorf("%s.BasePriority() = %d, want %d",
				tt.addrType.String(), priority, tt.expectedBase)
		}
	}
}

// TestAddressTypeSorting 验证地址类型排序
func TestAddressTypeSorting(t *testing.T) {
	// 乱序的地址类型
	types := []AddressType{
		AddressTypeRelay,
		AddressTypePublic,
		AddressTypeNATMapped,
		AddressTypeLAN,
	}

	// 按优先级排序（高优先级在前）
	sort.Slice(types, func(i, j int) bool {
		return types[i].BasePriority() > types[j].BasePriority()
	})

	// 验证排序结果
	expected := []AddressType{
		AddressTypePublic,
		AddressTypeLAN,
		AddressTypeNATMapped,
		AddressTypeRelay,
	}

	for i, typ := range types {
		if typ != expected[i] {
			t.Errorf("Sorted position %d: got %s, want %s",
				i, typ.String(), expected[i].String())
		}
	}

	t.Log("Address type sorting verified: Public → LAN → NAT → Relay")
}

// TestAddressTypeString 验证类型字符串表示
func TestAddressTypeString(t *testing.T) {
	tests := []struct {
		addrType AddressType
		expected string
	}{
		{AddressTypePublic, "public"},
		{AddressTypeLAN, "lan"},
		{AddressTypeNATMapped, "nat-mapped"},
		{AddressTypeRelay, "relay"},
		{AddressType(999), "unknown"},
	}

	for _, tt := range tests {
		got := tt.addrType.String()
		if got != tt.expected {
			t.Errorf("%v.String() = %s, want %s", tt.addrType, got, tt.expected)
		}
	}
}

// TestAddressStateUsable 验证地址状态可用性
func TestAddressStateUsable(t *testing.T) {
	tests := []struct {
		state  AddressState
		usable bool
	}{
		{AddressStateUnknown, true},
		{AddressStatePending, false},
		{AddressStateAvailable, true},
		{AddressStateDegraded, true},
		{AddressStateUnreachable, false},
		{AddressStateInvalid, false},
	}

	for _, tt := range tests {
		got := tt.state.IsUsable()
		if got != tt.usable {
			t.Errorf("%s.IsUsable() = %v, want %v", tt.state.String(), got, tt.usable)
		}
	}
}

