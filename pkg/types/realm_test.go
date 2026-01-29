package types

import (
	"testing"
	"time"
)

func TestRealmMember_IsOnline(t *testing.T) {
	m := RealmMember{Online: true}
	if !m.IsOnline() {
		t.Error("IsOnline() = false for online member")
	}

	m.Online = false
	if m.IsOnline() {
		t.Error("IsOnline() = true for offline member")
	}
}

func TestRealmMember_IsAdmin(t *testing.T) {
	m := RealmMember{Role: RoleAdmin}
	if !m.IsAdmin() {
		t.Error("IsAdmin() = false for admin")
	}

	m.Role = RoleMember
	if m.IsAdmin() {
		t.Error("IsAdmin() = true for non-admin")
	}
}

func TestRealmMember_IsRelay(t *testing.T) {
	m := RealmMember{Role: RoleRelay}
	if !m.IsRelay() {
		t.Error("IsRelay() = false for relay")
	}

	m.Role = RoleMember
	if m.IsRelay() {
		t.Error("IsRelay() = true for non-relay")
	}
}

func TestRealmConfig_Validate(t *testing.T) {
	// Valid config
	config := RealmConfig{
		PSK: GeneratePSK(),
	}
	if err := config.Validate(); err != nil {
		t.Errorf("Validate() error = %v for valid config", err)
	}

	// Empty PSK
	config2 := RealmConfig{}
	if err := config2.Validate(); err == nil {
		t.Error("Validate() should return error for empty PSK")
	}

	// Invalid PSK length
	config3 := RealmConfig{
		PSK: PSK([]byte{1, 2, 3}),
	}
	if err := config3.Validate(); err == nil {
		t.Error("Validate() should return error for invalid PSK length")
	}
}

func TestRealmConfig_DeriveRealmID(t *testing.T) {
	psk := GeneratePSK()
	config := RealmConfig{PSK: psk}

	realmID := config.DeriveRealmID()
	if realmID.IsEmpty() {
		t.Error("DeriveRealmID() returned empty")
	}

	// Same PSK should give same RealmID
	realmID2 := config.DeriveRealmID()
	if realmID != realmID2 {
		t.Error("DeriveRealmID() not deterministic")
	}
}

func TestDefaultRealmConfig(t *testing.T) {
	config := DefaultRealmConfig()

	if config.AuthMode != AuthModePSK {
		t.Errorf("AuthMode = %v, want AuthModePSK", config.AuthMode)
	}
	if config.MaxMembers != 0 {
		t.Errorf("MaxMembers = %d, want 0 (unlimited)", config.MaxMembers)
	}
	if config.RelayEnabled {
		t.Error("RelayEnabled = true, want false")
	}
}

func TestRealmStats(t *testing.T) {
	stats := RealmStats{
		RealmID:       RealmID("test"),
		MemberCount:   10,
		OnlineCount:   5,
		MessageCount:  100,
		BytesSent:     1024,
		BytesReceived: 2048,
		Uptime:        time.Hour,
	}

	if stats.MemberCount != 10 {
		t.Errorf("MemberCount = %d", stats.MemberCount)
	}
	if stats.OnlineCount != 5 {
		t.Errorf("OnlineCount = %d", stats.OnlineCount)
	}
}

func TestRelayConfig(t *testing.T) {
	config := RelayConfig{
		Enabled:      true,
		RelayPeer:    PeerID("relay-peer"),
		MaxBandwidth: 1024 * 1024, // 1 MB/s
		MaxDuration:  time.Hour,
	}

	if !config.Enabled {
		t.Error("Enabled = false")
	}
	if config.RelayPeer != "relay-peer" {
		t.Errorf("RelayPeer = %q", config.RelayPeer)
	}
}

func TestDefaultRealmJoinOptions(t *testing.T) {
	opts := DefaultRealmJoinOptions()

	if opts.Timeout != 30*time.Second {
		t.Errorf("Timeout = %v, want 30s", opts.Timeout)
	}
	if opts.Role != RoleMember {
		t.Errorf("Role = %v, want RoleMember", opts.Role)
	}
}

func TestDefaultRealmFindOptions(t *testing.T) {
	opts := DefaultRealmFindOptions()

	if opts.Limit != 100 {
		t.Errorf("Limit = %d, want 100", opts.Limit)
	}
	if opts.OnlineOnly {
		t.Error("OnlineOnly = true, want false")
	}
}

func TestRealmInfo(t *testing.T) {
	info := RealmInfo{
		ID:           RealmID("test-realm"),
		Name:         "Test Realm",
		Created:      time.Now(),
		MemberCount:  10,
		OnlineCount:  5,
		AuthMode:     AuthModePSK,
		RelayEnabled: true,
	}

	if info.ID != "test-realm" {
		t.Errorf("ID = %q", info.ID)
	}
	if info.Name != "Test Realm" {
		t.Errorf("Name = %q", info.Name)
	}
	if info.MemberCount != 10 {
		t.Errorf("MemberCount = %d", info.MemberCount)
	}
}
