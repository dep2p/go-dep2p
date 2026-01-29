package types

import "testing"

func TestKeyType(t *testing.T) {
	tests := []struct {
		kt   KeyType
		want string
	}{
		{KeyTypeUnknown, "Unknown"},
		{KeyTypeEd25519, "Ed25519"},
		{KeyTypeECDSA, "ECDSA"},
		{KeyTypeECDSAP256, "ECDSA-P256"},
		{KeyTypeECDSAP384, "ECDSA-P384"},
		{KeyTypeRSA, "RSA"},
		{KeyTypeSecp256k1, "Secp256k1"},
		{KeyType(99), "Unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.kt.String(); got != tt.want {
				t.Errorf("KeyType(%d).String() = %q, want %q", tt.kt, got, tt.want)
			}
		})
	}
}

func TestDirection(t *testing.T) {
	tests := []struct {
		d    Direction
		want string
	}{
		{DirUnknown, "unknown"},
		{DirInbound, "inbound"},
		{DirOutbound, "outbound"},
		{Direction(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.d.String(); got != tt.want {
				t.Errorf("Direction(%d).String() = %q, want %q", tt.d, got, tt.want)
			}
		})
	}
}

func TestConnectedness(t *testing.T) {
	tests := []struct {
		c    Connectedness
		want string
	}{
		{NotConnected, "not_connected"},
		{Connected, "connected"},
		{CanConnect, "can_connect"},
		{CannotConnect, "cannot_connect"},
		{Connectedness(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.c.String(); got != tt.want {
				t.Errorf("Connectedness(%d).String() = %q, want %q", tt.c, got, tt.want)
			}
		})
	}
}

func TestReachability(t *testing.T) {
	tests := []struct {
		r    Reachability
		want string
	}{
		{ReachabilityUnknown, "unknown"},
		{ReachabilityPublic, "public"},
		{ReachabilityPrivate, "private"},
		{Reachability(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.r.String(); got != tt.want {
				t.Errorf("Reachability(%d).String() = %q, want %q", tt.r, got, tt.want)
			}
		})
	}
}

func TestNATType(t *testing.T) {
	tests := []struct {
		n    NATType
		want string
	}{
		{NATTypeUnknown, "unknown"},
		{NATTypeNone, "none"},
		{NATTypeFullCone, "full_cone"},
		{NATTypeRestrictedCone, "restricted_cone"},
		{NATTypePortRestricted, "port_restricted"},
		{NATTypeSymmetric, "symmetric"},
		{NATType(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.n.String(); got != tt.want {
				t.Errorf("NATType(%d).String() = %q, want %q", tt.n, got, tt.want)
			}
		})
	}
}

func TestPriority(t *testing.T) {
	tests := []struct {
		p    Priority
		want string
	}{
		{PriorityLow, "low"},
		{PriorityNormal, "normal"},
		{PriorityHigh, "high"},
		{PriorityCritical, "critical"},
		{Priority(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.p.String(); got != tt.want {
				t.Errorf("Priority(%d).String() = %q, want %q", tt.p, got, tt.want)
			}
		})
	}
}

func TestDHTMode(t *testing.T) {
	tests := []struct {
		m    DHTMode
		want string
	}{
		{DHTModeAuto, "auto"},
		{DHTModeClient, "client"},
		{DHTModeServer, "server"},
		{DHTMode(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.m.String(); got != tt.want {
				t.Errorf("DHTMode(%d).String() = %q, want %q", tt.m, got, tt.want)
			}
		})
	}
}

func TestRealmAuthMode(t *testing.T) {
	tests := []struct {
		m    RealmAuthMode
		want string
	}{
		{AuthModePSK, "psk"},
		{AuthModeCert, "cert"},
		{AuthModeCustom, "custom"},
		{RealmAuthMode(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.m.String(); got != tt.want {
				t.Errorf("RealmAuthMode(%d).String() = %q, want %q", tt.m, got, tt.want)
			}
		})
	}
}

func TestRealmRole(t *testing.T) {
	tests := []struct {
		r    RealmRole
		want string
	}{
		{RoleMember, "member"},
		{RoleAdmin, "admin"},
		{RoleRelay, "relay"},
		{RealmRole(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.r.String(); got != tt.want {
				t.Errorf("RealmRole(%d).String() = %q, want %q", tt.r, got, tt.want)
			}
		})
	}
}

func TestDiscoverySource(t *testing.T) {
	tests := []struct {
		s    DiscoverySource
		want string
	}{
		{SourceDHT, "dht"},
		{SourceMDNS, "mdns"},
		{SourceBootstrap, "bootstrap"},
		{SourceRendezvous, "rendezvous"},
		{SourceDNS, "dns"},
		{SourceManual, "manual"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.s.String(); got != tt.want {
				t.Errorf("DiscoverySource.String() = %q, want %q", got, tt.want)
			}
		})
	}
}
