package types

import (
	"testing"
)

func TestProtocolID_String(t *testing.T) {
	proto := ProtocolID("/dep2p/sys/ping/1.0.0")
	if proto.String() != "/dep2p/sys/ping/1.0.0" {
		t.Errorf("String() = %q", proto.String())
	}
}

func TestProtocolID_IsEmpty(t *testing.T) {
	empty := ProtocolID("")
	if !empty.IsEmpty() {
		t.Error("IsEmpty() = false for empty")
	}

	proto := ProtocolID("/test")
	if proto.IsEmpty() {
		t.Error("IsEmpty() = true for non-empty")
	}
}

func TestProtocolID_Version(t *testing.T) {
	tests := []struct {
		proto ProtocolID
		want  string
	}{
		{"/dep2p/sys/ping/1.0.0", "1.0.0"},
		{"/dep2p/sys/identify/2.0.0", "2.0.0"},
		{"/simple", "simple"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(string(tt.proto), func(t *testing.T) {
			got := tt.proto.Version()
			if got != tt.want {
				t.Errorf("Version() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestProtocolID_Name(t *testing.T) {
	tests := []struct {
		proto ProtocolID
		want  string
	}{
		{"/dep2p/sys/ping/1.0.0", "/dep2p/sys/ping"},
		{"/dep2p/sys/identify/2.0.0", "/dep2p/sys/identify"},
		{"/simple", "/simple"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(string(tt.proto), func(t *testing.T) {
			got := tt.proto.Name()
			if got != tt.want {
				t.Errorf("Name() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestProtocolID_IsSystem(t *testing.T) {
	tests := []struct {
		proto ProtocolID
		want  bool
	}{
		{"/dep2p/sys/ping/1.0.0", true},
		{"/dep2p/sys/identify/1.0.0", true},
		{"/dep2p/realm/xxx/foo/1.0.0", false},
		{"/dep2p/app/xxx/bar/1.0.0", false},
		{"/other/proto", false},
	}

	for _, tt := range tests {
		t.Run(string(tt.proto), func(t *testing.T) {
			got := tt.proto.IsSystem()
			if got != tt.want {
				t.Errorf("IsSystem() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestProtocolID_IsRealm(t *testing.T) {
	tests := []struct {
		proto ProtocolID
		want  bool
	}{
		{"/dep2p/realm/xxx/foo/1.0.0", true},
		{"/dep2p/realm/yyy/bar/1.0.0", true},
		{"/dep2p/sys/ping/1.0.0", false},
		{"/dep2p/app/xxx/baz/1.0.0", false},
	}

	for _, tt := range tests {
		t.Run(string(tt.proto), func(t *testing.T) {
			got := tt.proto.IsRealm()
			if got != tt.want {
				t.Errorf("IsRealm() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestProtocolID_IsApp(t *testing.T) {
	tests := []struct {
		proto ProtocolID
		want  bool
	}{
		{"/dep2p/app/xxx/foo/1.0.0", true},
		{"/dep2p/app/yyy/bar/1.0.0", true},
		{"/dep2p/sys/ping/1.0.0", false},
		{"/dep2p/realm/xxx/baz/1.0.0", false},
	}

	for _, tt := range tests {
		t.Run(string(tt.proto), func(t *testing.T) {
			got := tt.proto.IsApp()
			if got != tt.want {
				t.Errorf("IsApp() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestProtocolID_RealmID(t *testing.T) {
	tests := []struct {
		proto ProtocolID
		want  string
	}{
		{"/dep2p/realm/realm123/foo/1.0.0", "realm123"},
		{"/dep2p/app/realm456/bar/1.0.0", "realm456"},
		{"/dep2p/sys/ping/1.0.0", ""},
		{"/dep2p/realm/", ""},
	}

	for _, tt := range tests {
		t.Run(string(tt.proto), func(t *testing.T) {
			got := tt.proto.RealmID()
			if got != tt.want {
				t.Errorf("RealmID() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBuildRealmProtocolID(t *testing.T) {
	proto := BuildRealmProtocolID("realm123", "chat", "1.0.0")
	expected := ProtocolID("/dep2p/realm/realm123/chat/1.0.0")

	if proto != expected {
		t.Errorf("BuildRealmProtocolID() = %q, want %q", proto, expected)
	}

	if !proto.IsRealm() {
		t.Error("BuildRealmProtocolID() should return realm protocol")
	}
}

func TestBuildAppProtocolID(t *testing.T) {
	proto := BuildAppProtocolID("realm123", "custom", "2.0.0")
	expected := ProtocolID("/dep2p/app/realm123/custom/2.0.0")

	if proto != expected {
		t.Errorf("BuildAppProtocolID() = %q, want %q", proto, expected)
	}

	if !proto.IsApp() {
		t.Error("BuildAppProtocolID() should return app protocol")
	}
}

func TestProtocolPrefixConstants(t *testing.T) {
	if ProtocolPrefixSys != "/dep2p/sys" {
		t.Errorf("ProtocolPrefixSys = %q", ProtocolPrefixSys)
	}
	if ProtocolPrefixRealm != "/dep2p/realm" {
		t.Errorf("ProtocolPrefixRealm = %q", ProtocolPrefixRealm)
	}
	if ProtocolPrefixApp != "/dep2p/app" {
		t.Errorf("ProtocolPrefixApp = %q", ProtocolPrefixApp)
	}
}

func TestSystemProtocolConstants(t *testing.T) {
	// 系统协议常量已移至 pkg/protocol 包
	// 此处仅测试 IsSystem() 方法对系统协议格式的判断
	systemProtocols := []ProtocolID{
		"/dep2p/sys/ping/1.0.0",
		"/dep2p/sys/identify/1.0.0",
		"/dep2p/relay/1.0.0/hop",
		"/dep2p/relay/1.0.0/stop",
	}

	for _, proto := range systemProtocols {
		if !proto.IsSystem() {
			t.Errorf("%q should be a system protocol", proto)
		}
		if proto.IsEmpty() {
			t.Error("Protocol should not be empty")
		}
	}
}

func TestProtocolNegotiation(t *testing.T) {
	pingProto := ProtocolID("/dep2p/sys/ping/1.0.0")
	identifyProto := ProtocolID("/dep2p/sys/identify/1.0.0")

	neg := ProtocolNegotiation{
		Selected: pingProto,
		Candidates: []ProtocolID{
			pingProto,
			identifyProto,
		},
	}

	if neg.Selected != pingProto {
		t.Errorf("Selected = %q", neg.Selected)
	}
	if len(neg.Candidates) != 2 {
		t.Errorf("Candidates len = %d", len(neg.Candidates))
	}
}
