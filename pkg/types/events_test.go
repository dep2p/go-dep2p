package types

import (
	"testing"
	"time"
)

func TestBaseEvent(t *testing.T) {
	evt := NewBaseEvent("test_event")

	if evt.Type() != "test_event" {
		t.Errorf("Type() = %q, want %q", evt.Type(), "test_event")
	}
	if evt.Timestamp().IsZero() {
		t.Error("Timestamp() is zero")
	}
	if time.Since(evt.Timestamp()) > time.Second {
		t.Error("Timestamp() is too old")
	}
}

func TestEvtPeerConnected(t *testing.T) {
	evt := EvtPeerConnected{
		BaseEvent: NewBaseEvent(EventTypePeerConnected),
		PeerID:    PeerID("12D3KooWTest"),
		Direction: DirInbound,
		NumConns:  1,
	}

	if evt.Type() != EventTypePeerConnected {
		t.Errorf("Type() = %q", evt.Type())
	}
	if evt.PeerID != "12D3KooWTest" {
		t.Errorf("PeerID = %q", evt.PeerID)
	}
	if evt.Direction != DirInbound {
		t.Errorf("Direction = %v", evt.Direction)
	}
}

func TestEvtPeerDisconnected(t *testing.T) {
	evt := EvtPeerDisconnected{
		BaseEvent: NewBaseEvent(EventTypePeerDisconnected),
		PeerID:    PeerID("12D3KooWTest"),
		NumConns:  0,
	}

	if evt.Type() != EventTypePeerDisconnected {
		t.Errorf("Type() = %q", evt.Type())
	}
}

func TestEvtConnectionClosed(t *testing.T) {
	evt := EvtConnectionClosed{
		BaseEvent: NewBaseEvent(EventTypeConnectionClosed),
		PeerID:    PeerID("12D3KooWTest"),
		Direction: DirOutbound,
		Duration:  5 * time.Minute,
	}

	if evt.Duration != 5*time.Minute {
		t.Errorf("Duration = %v", evt.Duration)
	}
}

func TestEvtPeerDiscovered(t *testing.T) {
	addr, _ := NewMultiaddr("/ip4/127.0.0.1/tcp/4001")
	evt := EvtPeerDiscovered{
		BaseEvent: NewBaseEvent(EventTypePeerDiscovered),
		PeerID:    PeerID("12D3KooWTest"),
		Addrs:     []Multiaddr{addr},
		Source:    SourceDHT,
	}

	if evt.Source != SourceDHT {
		t.Errorf("Source = %v", evt.Source)
	}
	if len(evt.Addrs) != 1 {
		t.Errorf("Addrs len = %d", len(evt.Addrs))
	}
}

func TestEvtPeerIdentified(t *testing.T) {
	evt := EvtPeerIdentified{
		BaseEvent: NewBaseEvent(EventTypePeerIdentified),
		PeerID:    PeerID("12D3KooWTest"),
		Protocols: []ProtocolID{"/dep2p/sys/ping/1.0.0"},
		AgentVer:  "dep2p/1.0.0",
	}

	if len(evt.Protocols) != 1 {
		t.Errorf("Protocols len = %d", len(evt.Protocols))
	}
	if evt.AgentVer != "dep2p/1.0.0" {
		t.Errorf("AgentVer = %q", evt.AgentVer)
	}
}

func TestEvtProtocolNegotiated(t *testing.T) {
	evt := EvtProtocolNegotiated{
		BaseEvent: NewBaseEvent(EventTypeProtocolNegotiated),
		PeerID:    PeerID("12D3KooWTest"),
		Protocol:  ProtocolID("/dep2p/sys/ping/1.0.0"),
	}

	if evt.Protocol != "/dep2p/sys/ping/1.0.0" {
		t.Errorf("Protocol = %q", evt.Protocol)
	}
}

func TestEvtStreamOpened(t *testing.T) {
	evt := EvtStreamOpened{
		BaseEvent: NewBaseEvent(EventTypeStreamOpened),
		StreamID:  "stream-1",
		PeerID:    PeerID("12D3KooWTest"),
		Protocol:  ProtocolID("/dep2p/sys/ping/1.0.0"),
		Direction: DirOutbound,
	}

	if evt.StreamID != "stream-1" {
		t.Errorf("StreamID = %q", evt.StreamID)
	}
	if evt.Direction != DirOutbound {
		t.Errorf("Direction = %v", evt.Direction)
	}
}

func TestEvtStreamClosed(t *testing.T) {
	evt := EvtStreamClosed{
		BaseEvent: NewBaseEvent(EventTypeStreamClosed),
		StreamID:  "stream-1",
		PeerID:    PeerID("12D3KooWTest"),
		Duration:  30 * time.Second,
	}

	if evt.Duration != 30*time.Second {
		t.Errorf("Duration = %v", evt.Duration)
	}
}

func TestEvtRealmJoined(t *testing.T) {
	evt := EvtRealmJoined{
		BaseEvent: NewBaseEvent(EventTypeRealmJoined),
		RealmID:   RealmID("test-realm"),
		PeerID:    PeerID("12D3KooWTest"),
	}

	if evt.RealmID != "test-realm" {
		t.Errorf("RealmID = %q", evt.RealmID)
	}
}

func TestEvtRealmLeft(t *testing.T) {
	evt := EvtRealmLeft{
		BaseEvent: NewBaseEvent(EventTypeRealmLeft),
		RealmID:   RealmID("test-realm"),
		PeerID:    PeerID("12D3KooWTest"),
	}

	if evt.Type() != EventTypeRealmLeft {
		t.Errorf("Type() = %q", evt.Type())
	}
}

func TestEvtRealmMemberJoined(t *testing.T) {
	evt := EvtRealmMemberJoined{
		BaseEvent: NewBaseEvent(EventTypeRealmMemberJoined),
		RealmID:   RealmID("test-realm"),
		MemberID:  PeerID("new-member"),
	}

	if evt.MemberID != "new-member" {
		t.Errorf("MemberID = %q", evt.MemberID)
	}
}

func TestEvtRealmMemberLeft(t *testing.T) {
	evt := EvtRealmMemberLeft{
		BaseEvent: NewBaseEvent(EventTypeRealmMemberLeft),
		RealmID:   RealmID("test-realm"),
		MemberID:  PeerID("left-member"),
	}

	if evt.MemberID != "left-member" {
		t.Errorf("MemberID = %q", evt.MemberID)
	}
}

func TestEvtNATTypeDetected(t *testing.T) {
	evt := EvtNATTypeDetected{
		BaseEvent:    NewBaseEvent(EventTypeNATTypeDetected),
		NATType:      NATTypeFullCone,
		ExternalIP:   "1.2.3.4",
		ExternalPort: 4001,
		Reachability: ReachabilityPublic,
	}

	if evt.NATType != NATTypeFullCone {
		t.Errorf("NATType = %v", evt.NATType)
	}
	if evt.ExternalIP != "1.2.3.4" {
		t.Errorf("ExternalIP = %q", evt.ExternalIP)
	}
	if evt.Reachability != ReachabilityPublic {
		t.Errorf("Reachability = %v", evt.Reachability)
	}
}

func TestEvtHolePunchAttempt(t *testing.T) {
	evt := EvtHolePunchAttempt{
		BaseEvent: NewBaseEvent(EventTypeHolePunchAttempt),
		PeerID:    PeerID("12D3KooWTest"),
		Success:   true,
		RTT:       100 * time.Millisecond,
	}

	if !evt.Success {
		t.Error("Success = false")
	}
	if evt.RTT != 100*time.Millisecond {
		t.Errorf("RTT = %v", evt.RTT)
	}
}

func TestEvtHolePunchComplete(t *testing.T) {
	evt := EvtHolePunchComplete{
		BaseEvent: NewBaseEvent(EventTypeHolePunchComplete),
		PeerID:    PeerID("12D3KooWTest"),
		Success:   true,
		Direct:    true,
	}

	if !evt.Direct {
		t.Error("Direct = false")
	}
}

func TestEvtPeerAlive(t *testing.T) {
	evt := EvtPeerAlive{
		BaseEvent: NewBaseEvent(EventTypePeerAlive),
		PeerID:    PeerID("12D3KooWTest"),
		RTT:       50 * time.Millisecond,
	}

	if evt.RTT != 50*time.Millisecond {
		t.Errorf("RTT = %v", evt.RTT)
	}
}

func TestEvtPeerDead(t *testing.T) {
	lastSeen := time.Now().Add(-5 * time.Minute)
	evt := EvtPeerDead{
		BaseEvent:   NewBaseEvent(EventTypePeerDead),
		PeerID:      PeerID("12D3KooWTest"),
		LastSeen:    lastSeen,
		FailedPings: 3,
	}

	if evt.FailedPings != 3 {
		t.Errorf("FailedPings = %d", evt.FailedPings)
	}
	if !evt.LastSeen.Equal(lastSeen) {
		t.Errorf("LastSeen = %v", evt.LastSeen)
	}
}

func TestEvtRelayReservation(t *testing.T) {
	expiry := time.Now().Add(time.Hour)
	evt := EvtRelayReservation{
		BaseEvent: NewBaseEvent(EventTypeRelayReservation),
		RelayPeer: PeerID("relay-peer"),
		Success:   true,
		Expiry:    expiry,
	}

	if !evt.Success {
		t.Error("Success = false")
	}
	if !evt.Expiry.Equal(expiry) {
		t.Errorf("Expiry = %v", evt.Expiry)
	}
}

func TestEvtRelayConnection(t *testing.T) {
	evt := EvtRelayConnection{
		BaseEvent:  NewBaseEvent(EventTypeRelayConnection),
		RelayPeer:  PeerID("relay-peer"),
		RemotePeer: PeerID("remote-peer"),
		Direction:  DirInbound,
	}

	if evt.RemotePeer != "remote-peer" {
		t.Errorf("RemotePeer = %q", evt.RemotePeer)
	}
}

func TestEventTypeConstants(t *testing.T) {
	// Verify all event type constants are defined and unique
	types := []string{
		EventTypePeerConnected,
		EventTypePeerDisconnected,
		EventTypeConnectionClosed,
		EventTypePeerDiscovered,
		EventTypePeerIdentified,
		EventTypeProtocolNegotiated,
		EventTypeStreamOpened,
		EventTypeStreamClosed,
		EventTypeRealmJoined,
		EventTypeRealmLeft,
		EventTypeRealmMemberJoined,
		EventTypeRealmMemberLeft,
		EventTypeNATTypeDetected,
		EventTypeHolePunchAttempt,
		EventTypeHolePunchComplete,
		EventTypePeerAlive,
		EventTypePeerDead,
		EventTypeRelayReservation,
		EventTypeRelayConnection,
	}

	seen := make(map[string]bool)
	for _, typ := range types {
		if typ == "" {
			t.Error("Event type constant is empty")
		}
		if seen[typ] {
			t.Errorf("Duplicate event type: %q", typ)
		}
		seen[typ] = true
	}
}
