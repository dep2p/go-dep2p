package protocolids

import (
	"testing"

	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              协议命名空间测试（IMPL-1227 Phase 7）
// ============================================================================

// TestFullAppProtocol 测试应用协议生成
func TestFullAppProtocol(t *testing.T) {
	realmKey := types.GenerateRealmKey()
	realmID := types.DeriveRealmID(realmKey)

	testCases := []struct {
		name      string
		userProto string
	}{
		{"simple", "chat/1.0.0"},
		{"nested", "file/transfer/1.0.0"},
		{"version only", "myproto/2.0.0"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			proto := FullAppProtocol(realmID, tc.userProto)
			expected := "/dep2p/app/" + string(realmID) + "/" + tc.userProto
			if string(proto) != expected {
				t.Errorf("expected %s, got %s", expected, proto)
			}
		})
	}
}

// TestFullRealmProtocol 测试 Realm 协议生成
func TestFullRealmProtocol(t *testing.T) {
	realmKey := types.GenerateRealmKey()
	realmID := types.DeriveRealmID(realmKey)

	proto := FullRealmProtocol(realmID, "membership/1.0.0")
	expected := "/dep2p/realm/" + string(realmID) + "/membership/1.0.0"
	if string(proto) != expected {
		t.Errorf("expected %s, got %s", expected, proto)
	}
}

// TestValidateUserProtocol 测试用户协议验证
func TestValidateUserProtocol(t *testing.T) {
	realmKey := types.GenerateRealmKey()
	realmID := types.DeriveRealmID(realmKey)
	realmKey2 := types.GenerateRealmKey()
	realmID2 := types.DeriveRealmID(realmKey2)

	testCases := []struct {
		name      string
		proto     string
		wantErr   error
		errName   string
	}{
		{"valid simple", "chat/1.0.0", nil, ""},
		{"valid nested", "file/transfer/1.0.0", nil, ""},
		{"reserved sys", "/dep2p/sys/echo/1.0.0", ErrReservedProtocol, "ErrReservedProtocol"},
		{"reserved realm", "/dep2p/realm/xxx/proto", ErrReservedProtocol, "ErrReservedProtocol"},
		{"reserved app", "/dep2p/app/" + string(realmID) + "/chat", ErrReservedProtocol, "ErrReservedProtocol"},
		{"cross realm", "/dep2p/app/" + string(realmID2) + "/chat", ErrCrossRealmProtocol, "ErrCrossRealmProtocol"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateUserProtocol(tc.proto, realmID)
			if err != tc.wantErr {
				t.Errorf("ValidateUserProtocol(%q) = %v, want %v (%s)", tc.proto, err, tc.wantErr, tc.errName)
			}
		})
	}
}

// TestExtractRealmID 测试从协议中提取 RealmID
func TestExtractRealmID(t *testing.T) {
	realmKey := types.GenerateRealmKey()
	realmID := types.DeriveRealmID(realmKey)

	testCases := []struct {
		name    string
		proto   types.ProtocolID
		wantID  types.RealmID
		wantErr error
	}{
		{"app protocol", FullAppProtocol(realmID, "chat/1.0.0"), realmID, nil},
		{"realm protocol", FullRealmProtocol(realmID, "sync/1.0.0"), realmID, nil},
		{"system protocol", SysEcho, "", ErrNoRealmInProtocol},
		{"invalid app", "/dep2p/app/", "", ErrInvalidProtocolFormat},
		{"invalid realm", "/dep2p/realm/", "", ErrInvalidProtocolFormat},
		{"random protocol", "random/proto", "", ErrNoRealmInProtocol},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			gotID, err := ExtractRealmID(tc.proto)
			if err != tc.wantErr {
				t.Errorf("ExtractRealmID(%q) error = %v, want %v", tc.proto, err, tc.wantErr)
				return
			}
			if gotID != tc.wantID {
				t.Errorf("ExtractRealmID(%q) = %q, want %q", tc.proto, gotID, tc.wantID)
			}
		})
	}
}

// TestIsSystemProtocol 测试系统协议识别
func TestIsSystemProtocol(t *testing.T) {
	testCases := []struct {
		proto    types.ProtocolID
		expected bool
	}{
		{SysEcho, true},
		{SysPing, true},
		{SysDHT, true},
		{SysRelay, true},
		{SysNoise, true},
		{SysRealmAuth, true},
		{"/dep2p/sys/custom/1.0.0", true},
		{"/dep2p/app/xxx/chat", false},
		{"/dep2p/realm/xxx/sync", false},
		{"random/proto", false},
	}

	for _, tc := range testCases {
		t.Run(string(tc.proto), func(t *testing.T) {
			result := IsSystemProtocol(tc.proto)
			if result != tc.expected {
				t.Errorf("IsSystemProtocol(%q) = %v, want %v", tc.proto, result, tc.expected)
			}
		})
	}
}

// TestIsAppProtocol 测试应用协议识别
func TestIsAppProtocol(t *testing.T) {
	realmKey := types.GenerateRealmKey()
	realmID := types.DeriveRealmID(realmKey)
	appProto := FullAppProtocol(realmID, "chat/1.0.0")

	testCases := []struct {
		proto    types.ProtocolID
		expected bool
	}{
		{appProto, true},
		{"/dep2p/app/any-realm-id/proto", true},
		{SysEcho, false},
		{"/dep2p/realm/xxx/sync", false},
		{"random/proto", false},
	}

	for _, tc := range testCases {
		t.Run(string(tc.proto), func(t *testing.T) {
			result := IsAppProtocol(tc.proto)
			if result != tc.expected {
				t.Errorf("IsAppProtocol(%q) = %v, want %v", tc.proto, result, tc.expected)
			}
		})
	}
}

// TestIsRealmProtocol 测试 Realm 控制协议识别
func TestIsRealmProtocol(t *testing.T) {
	realmKey := types.GenerateRealmKey()
	realmID := types.DeriveRealmID(realmKey)
	realmProto := FullRealmProtocol(realmID, "sync/1.0.0")

	testCases := []struct {
		proto    types.ProtocolID
		expected bool
	}{
		{realmProto, true},
		{"/dep2p/realm/any-realm-id/proto", true},
		{SysEcho, false},
		{"/dep2p/app/xxx/chat", false},
		{"random/proto", false},
	}

	for _, tc := range testCases {
		t.Run(string(tc.proto), func(t *testing.T) {
			result := IsRealmProtocol(tc.proto)
			if result != tc.expected {
				t.Errorf("IsRealmProtocol(%q) = %v, want %v", tc.proto, result, tc.expected)
			}
		})
	}
}

// TestBelongsToRealm 测试协议所属 Realm 判断
func TestBelongsToRealm(t *testing.T) {
	realmKey1 := types.GenerateRealmKey()
	realmKey2 := types.GenerateRealmKey()
	realmID1 := types.DeriveRealmID(realmKey1)
	realmID2 := types.DeriveRealmID(realmKey2)

	appProto1 := FullAppProtocol(realmID1, "chat/1.0.0")
	realmProto1 := FullRealmProtocol(realmID1, "sync/1.0.0")

	testCases := []struct {
		name     string
		proto    types.ProtocolID
		realmID  types.RealmID
		expected bool
	}{
		{"app proto belongs", appProto1, realmID1, true},
		{"app proto not belongs", appProto1, realmID2, false},
		{"realm proto belongs", realmProto1, realmID1, true},
		{"realm proto not belongs", realmProto1, realmID2, false},
		{"sys proto never belongs", SysEcho, realmID1, false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := BelongsToRealm(tc.proto, tc.realmID)
			if result != tc.expected {
				t.Errorf("BelongsToRealm(%q, %q) = %v, want %v", tc.proto, tc.realmID, result, tc.expected)
			}
		})
	}
}

// TestSystemProtocolConstants 测试系统协议常量格式
func TestSystemProtocolConstants(t *testing.T) {
	sysProtocols := []types.ProtocolID{
		SysPing,
		SysGoodbye,
		SysHeartbeat,
		SysIdentify,
		SysIdentifyPush,
		SysEcho,
		SysNoise,
		SysDHT,
		SysRendezvous,
		SysRelay,
		SysRelayHop,
		SysRelayStop,
		SysHolepunch,
		SysReachability,
		SysReachabilityWitness,
		SysDeviceID,
		SysAddrMgmt,
		SysAddrExchange,
		SysRealmAuth,
		SysRealmSync,
		SysGossipSub,
		SysTestEcho,
		SysTestBackpressure,
	}

	for _, proto := range sysProtocols {
		t.Run(string(proto), func(t *testing.T) {
			// 所有系统协议应该以 /dep2p/sys/ 开头
			if !IsSystemProtocol(proto) {
				t.Errorf("%q should be recognized as system protocol", proto)
			}
			// 系统协议不应该属于任何 Realm
			realmKey := types.GenerateRealmKey()
			realmID := types.DeriveRealmID(realmKey)
			if BelongsToRealm(proto, realmID) {
				t.Errorf("%q should not belong to any realm", proto)
			}
		})
	}
}

