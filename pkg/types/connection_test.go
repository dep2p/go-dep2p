package types

import (
	"testing"
	"time"
)

func TestConnStat(t *testing.T) {
	stat := ConnStat{
		Direction:  DirInbound,
		Opened:     time.Now(),
		Transient:  true,
		NumStreams: 5,
		Limited:    false,
	}

	if stat.Direction != DirInbound {
		t.Errorf("Direction = %v", stat.Direction)
	}
	if !stat.Transient {
		t.Error("Transient = false")
	}
	if stat.NumStreams != 5 {
		t.Errorf("NumStreams = %d", stat.NumStreams)
	}
}

func TestConnInfo(t *testing.T) {
	localAddr, _ := NewMultiaddr("/ip4/127.0.0.1/tcp/4001")
	remoteAddr, _ := NewMultiaddr("/ip4/192.168.1.1/tcp/4001")
	
	info := ConnInfo{
		ID:         "conn-1",
		LocalPeer:  PeerID("local-peer"),
		LocalAddr:  localAddr,
		RemotePeer: PeerID("remote-peer"),
		RemoteAddr: remoteAddr,
		Stat: ConnStat{
			Direction:  DirOutbound,
			Opened:     time.Now(),
			NumStreams: 3,
		},
	}

	if info.ID != "conn-1" {
		t.Errorf("ID = %q", info.ID)
	}
	if info.LocalPeer != "local-peer" {
		t.Errorf("LocalPeer = %q", info.LocalPeer)
	}
	if info.RemotePeer != "remote-peer" {
		t.Errorf("RemotePeer = %q", info.RemotePeer)
	}
	if info.Stat.Direction != DirOutbound {
		t.Errorf("Stat.Direction = %v", info.Stat.Direction)
	}
}

func TestConnState_String(t *testing.T) {
	tests := []struct {
		state ConnState
		want  string
	}{
		{ConnStateConnecting, "connecting"},
		{ConnStateConnected, "connected"},
		{ConnStateDisconnecting, "disconnecting"},
		{ConnStateDisconnected, "disconnected"},
		{ConnState(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := tt.state.String()
			if got != tt.want {
				t.Errorf("ConnState(%d).String() = %q, want %q", tt.state, got, tt.want)
			}
		})
	}
}

func TestConnScope(t *testing.T) {
	scope := ConnScope{
		ConnID:     "conn-1",
		PeerID:     PeerID("peer-1"),
		Direction:  DirInbound,
		Memory:     1024 * 1024,
		NumStreams: 10,
	}

	if scope.ConnID != "conn-1" {
		t.Errorf("ConnID = %q", scope.ConnID)
	}
	if scope.Memory != 1024*1024 {
		t.Errorf("Memory = %d", scope.Memory)
	}
}

func TestConnectionStat(t *testing.T) {
	stat := ConnectionStat{
		NumConns:         10,
		NumConnsInbound:  6,
		NumConnsOutbound: 4,
		NumStreams:       50,
		NumPeers:         8,
	}

	if stat.NumConns != 10 {
		t.Errorf("NumConns = %d", stat.NumConns)
	}
	if stat.NumConnsInbound != 6 {
		t.Errorf("NumConnsInbound = %d", stat.NumConnsInbound)
	}
	if stat.NumConnsOutbound != 4 {
		t.Errorf("NumConnsOutbound = %d", stat.NumConnsOutbound)
	}
	if stat.NumPeers != 8 {
		t.Errorf("NumPeers = %d", stat.NumPeers)
	}
}

func TestConnInfoWithStreams(t *testing.T) {
	info := ConnInfo{
		ID:         "conn-1",
		LocalPeer:  PeerID("local"),
		RemotePeer: PeerID("remote"),
		Streams: []StreamInfo{
			{ID: "stream-1", Protocol: "/ping/1.0.0"},
			{ID: "stream-2", Protocol: "/identify/1.0.0"},
		},
	}

	if len(info.Streams) != 2 {
		t.Errorf("Streams len = %d", len(info.Streams))
	}
	if info.Streams[0].ID != "stream-1" {
		t.Errorf("Streams[0].ID = %q", info.Streams[0].ID)
	}
}
