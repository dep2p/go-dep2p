package protocol

import (
	"testing"
)

func TestSystemProtocolConstants(t *testing.T) {
	tests := []struct {
		name     string
		protocol ID
		expected string
	}{
		{"Identify", Identify, "/dep2p/sys/identify/1.0.0"},
		{"IdentifyPush", IdentifyPush, "/dep2p/sys/identify/push/1.0.0"},
		{"Ping", Ping, "/dep2p/sys/ping/1.0.0"},
		{"Heartbeat", Heartbeat, "/dep2p/sys/heartbeat/1.0.0"},
		{"AutoNAT", AutoNAT, "/dep2p/sys/autonat/1.0.0"},
		{"HolePunch", HolePunch, "/dep2p/sys/holepunch/1.0.0"},
		{"DHT", DHT, "/dep2p/sys/dht/1.0.0"},
		{"Rendezvous", Rendezvous, "/dep2p/sys/rendezvous/1.0.0"},
		{"Reachability", Reachability, "/dep2p/sys/reachability/1.0.0"},
		{"ReachabilityWitness", ReachabilityWitness, "/dep2p/sys/reachability/witness/1.0.0"},
		{"AddrMgmt", AddrMgmt, "/dep2p/sys/addr-mgmt/1.0.0"},
		{"DeliveryAck", DeliveryAck, "/dep2p/sys/delivery/ack/1.0.0"},
		{"GatewayRelay", GatewayRelay, "/dep2p/sys/gateway/relay/1.0.0"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.protocol) != tt.expected {
				t.Errorf("got %q, want %q", tt.protocol, tt.expected)
			}
		})
	}
}

func TestRelayProtocolConstants(t *testing.T) {
	tests := []struct {
		name     string
		protocol ID
		expected string
	}{
		{"RelayHop", RelayHop, "/dep2p/relay/1.0.0/hop"},
		{"RelayStop", RelayStop, "/dep2p/relay/1.0.0/stop"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.protocol) != tt.expected {
				t.Errorf("got %q, want %q", tt.protocol, tt.expected)
			}
		})
	}
}

func TestRealmBuilder(t *testing.T) {
	realmID := "my-realm"
	builder := NewRealmBuilder(realmID)

	tests := []struct {
		name     string
		protocol ID
		expected string
	}{
		{"Auth", builder.Auth(), "/dep2p/realm/my-realm/auth/1.0.0"},
		{"Sync", builder.Sync(), "/dep2p/realm/my-realm/sync/1.0.0"},
		{"Announce", builder.Announce(), "/dep2p/realm/my-realm/announce/1.0.0"},
		{"Addressbook", builder.Addressbook(), "/dep2p/realm/my-realm/addressbook/1.0.0"},
		{"Join", builder.Join(), "/dep2p/realm/my-realm/join/1.0.0"},
		{"Route", builder.Route(), "/dep2p/realm/my-realm/route/1.0.0"},
		{"Custom", builder.Custom("test", "2.0.0"), "/dep2p/realm/my-realm/test/2.0.0"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.protocol) != tt.expected {
				t.Errorf("got %q, want %q", tt.protocol, tt.expected)
			}
		})
	}

	// 验证 RealmID 方法
	if builder.RealmID() != realmID {
		t.Errorf("RealmID() = %q, want %q", builder.RealmID(), realmID)
	}
}

func TestAppBuilder(t *testing.T) {
	realmID := "test-realm"
	builder := NewAppBuilder(realmID)

	tests := []struct {
		name     string
		protocol ID
		expected string
	}{
		{"Messaging", builder.Messaging(), "/dep2p/app/test-realm/messaging/1.0.0"},
		{"PubSub", builder.PubSub(), "/dep2p/app/test-realm/pubsub/1.0.0"},
		{"Streams", builder.Streams(), "/dep2p/app/test-realm/streams/1.0.0"},
		{"Liveness", builder.Liveness(), "/dep2p/app/test-realm/liveness/1.0.0"},
		{"Custom", builder.Custom("rpc", "1.5.0"), "/dep2p/app/test-realm/rpc/1.5.0"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.protocol) != tt.expected {
				t.Errorf("got %q, want %q", tt.protocol, tt.expected)
			}
		})
	}

	// 验证 RealmID 方法
	if builder.RealmID() != realmID {
		t.Errorf("RealmID() = %q, want %q", builder.RealmID(), realmID)
	}
}

func TestBuildFunctions(t *testing.T) {
	t.Run("BuildRealmProtocol", func(t *testing.T) {
		proto := BuildRealmProtocol("r1", "auth", "1.0.0")
		expected := "/dep2p/realm/r1/auth/1.0.0"
		if string(proto) != expected {
			t.Errorf("got %q, want %q", proto, expected)
		}
	})

	t.Run("BuildAppProtocol", func(t *testing.T) {
		proto := BuildAppProtocol("r1", "messaging", "1.0.0")
		expected := "/dep2p/app/r1/messaging/1.0.0"
		if string(proto) != expected {
			t.Errorf("got %q, want %q", proto, expected)
		}
	})
}

func TestIsSystem(t *testing.T) {
	tests := []struct {
		protocol ID
		expected bool
	}{
		{Ping, true},
		{DHT, true},
		{RelayHop, true},
		{RelayStop, true},
		{ID("/dep2p/realm/r1/auth/1.0.0"), false},
		{ID("/dep2p/app/r1/messaging/1.0.0"), false},
	}

	for _, tt := range tests {
		t.Run(string(tt.protocol), func(t *testing.T) {
			if got := IsSystem(tt.protocol); got != tt.expected {
				t.Errorf("IsSystem(%q) = %v, want %v", tt.protocol, got, tt.expected)
			}
		})
	}
}

func TestIsRelay(t *testing.T) {
	tests := []struct {
		protocol ID
		expected bool
	}{
		{RelayHop, true},
		{RelayStop, true},
		{Ping, false},
		{DHT, false},
	}

	for _, tt := range tests {
		t.Run(string(tt.protocol), func(t *testing.T) {
			if got := IsRelay(tt.protocol); got != tt.expected {
				t.Errorf("IsRelay(%q) = %v, want %v", tt.protocol, got, tt.expected)
			}
		})
	}
}

func TestIsRealm(t *testing.T) {
	tests := []struct {
		protocol ID
		expected bool
	}{
		{ID("/dep2p/realm/r1/auth/1.0.0"), true},
		{ID("/dep2p/realm/r1/sync/1.0.0"), true},
		{Ping, false},
		{RelayHop, false},
		{ID("/dep2p/app/r1/messaging/1.0.0"), false},
	}

	for _, tt := range tests {
		t.Run(string(tt.protocol), func(t *testing.T) {
			if got := IsRealm(tt.protocol); got != tt.expected {
				t.Errorf("IsRealm(%q) = %v, want %v", tt.protocol, got, tt.expected)
			}
		})
	}
}

func TestIsApp(t *testing.T) {
	tests := []struct {
		protocol ID
		expected bool
	}{
		{ID("/dep2p/app/r1/messaging/1.0.0"), true},
		{ID("/dep2p/app/r1/pubsub/1.0.0"), true},
		{Ping, false},
		{ID("/dep2p/realm/r1/auth/1.0.0"), false},
	}

	for _, tt := range tests {
		t.Run(string(tt.protocol), func(t *testing.T) {
			if got := IsApp(tt.protocol); got != tt.expected {
				t.Errorf("IsApp(%q) = %v, want %v", tt.protocol, got, tt.expected)
			}
		})
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		protocol ID
		wantErr  error
	}{
		// 有效协议
		{Ping, nil},
		{RelayHop, nil},
		{ID("/dep2p/realm/r1/auth/1.0.0"), nil},
		{ID("/dep2p/app/r1/messaging/1.0.0"), nil},

		// 无效协议
		{ID(""), ErrEmptyProtocol},
		{ID("/invalid/protocol"), ErrInvalidPrefix},
		{ID("/dep2p"), ErrInvalidPrefix},           // 只有前缀，类型部分缺失
		{ID("/dep2p/sys"), ErrInvalidProtocol},     // 缺少协议名
		{ID("/dep2p/sys/ping"), ErrMissingVersion}, // 缺少版本
		{ID("/dep2p/realm/r1"), ErrMissingVersion}, // 缺少协议名和版本
		{ID("/dep2p/realm/r1/auth"), ErrMissingVersion},
		{ID("/dep2p/unknown/test/1.0.0"), ErrInvalidPrefix},
	}

	for _, tt := range tests {
		t.Run(string(tt.protocol), func(t *testing.T) {
			err := Validate(tt.protocol)
			if err != tt.wantErr {
				t.Errorf("Validate(%q) = %v, want %v", tt.protocol, err, tt.wantErr)
			}
		})
	}
}

func TestExtractRealmID(t *testing.T) {
	tests := []struct {
		protocol ID
		expected string
	}{
		{ID("/dep2p/realm/my-realm/auth/1.0.0"), "my-realm"},
		{ID("/dep2p/app/test-realm/messaging/1.0.0"), "test-realm"},
		{Ping, ""},
		{RelayHop, ""},
	}

	for _, tt := range tests {
		t.Run(string(tt.protocol), func(t *testing.T) {
			if got := ExtractRealmID(tt.protocol); got != tt.expected {
				t.Errorf("ExtractRealmID(%q) = %q, want %q", tt.protocol, got, tt.expected)
			}
		})
	}
}

func TestExtractNameAndVersion(t *testing.T) {
	tests := []struct {
		protocol        ID
		expectedName    string
		expectedVersion string
	}{
		{Ping, "/dep2p/sys/ping", "1.0.0"},
		{RelayHop, "/dep2p/relay/1.0.0", "hop"},
		{ID("/dep2p/realm/r1/auth/1.0.0"), "/dep2p/realm/r1/auth", "1.0.0"},
	}

	for _, tt := range tests {
		t.Run(string(tt.protocol), func(t *testing.T) {
			if got := ExtractName(tt.protocol); got != tt.expectedName {
				t.Errorf("ExtractName(%q) = %q, want %q", tt.protocol, got, tt.expectedName)
			}
			if got := ExtractVersion(tt.protocol); got != tt.expectedVersion {
				t.Errorf("ExtractVersion(%q) = %q, want %q", tt.protocol, got, tt.expectedVersion)
			}
		})
	}
}

func TestMatch(t *testing.T) {
	tests := []struct {
		a, b     ID
		expected bool
	}{
		{ID("/dep2p/sys/ping/1.0.0"), ID("/dep2p/sys/ping/2.0.0"), true},
		{ID("/dep2p/sys/ping/1.0.0"), ID("/dep2p/sys/pong/1.0.0"), false},
		{Ping, Ping, true},
	}

	for _, tt := range tests {
		t.Run(string(tt.a)+"_"+string(tt.b), func(t *testing.T) {
			if got := Match(tt.a, tt.b); got != tt.expected {
				t.Errorf("Match(%q, %q) = %v, want %v", tt.a, tt.b, got, tt.expected)
			}
		})
	}
}

func TestSystemProtocols(t *testing.T) {
	protocols := SystemProtocols()

	// 确保所有系统协议都包含在列表中
	expectedCount := 14 // 不含 Relay（包含 AddrExchange）
	if len(protocols) != expectedCount {
		t.Errorf("SystemProtocols() returned %d protocols, want %d", len(protocols), expectedCount)
	}

	// 验证所有返回的协议都是系统协议
	for _, p := range protocols {
		if !IsSystem(p) {
			t.Errorf("SystemProtocols() contains non-system protocol: %q", p)
		}
		if IsRelay(p) {
			t.Errorf("SystemProtocols() contains relay protocol: %q", p)
		}
	}
}

func TestRelayProtocols(t *testing.T) {
	protocols := RelayProtocols()

	if len(protocols) != 2 {
		t.Errorf("RelayProtocols() returned %d protocols, want 2", len(protocols))
	}

	for _, p := range protocols {
		if !IsRelay(p) {
			t.Errorf("RelayProtocols() contains non-relay protocol: %q", p)
		}
	}
}

func TestAllSystemProtocols(t *testing.T) {
	protocols := AllSystemProtocols()
	sysCount := len(SystemProtocols())
	relayCount := len(RelayProtocols())

	if len(protocols) != sysCount+relayCount {
		t.Errorf("AllSystemProtocols() returned %d protocols, want %d", len(protocols), sysCount+relayCount)
	}
}
