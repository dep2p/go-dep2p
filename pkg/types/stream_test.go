package types

import (
	"testing"
	"time"
)

func TestStreamStat(t *testing.T) {
	stat := StreamStat{
		Direction:    DirOutbound,
		Opened:       time.Now(),
		Protocol:     ProtocolID("/dep2p/sys/ping/1.0.0"),
		BytesRead:    1024,
		BytesWritten: 2048,
	}

	if stat.Direction != DirOutbound {
		t.Errorf("Direction = %v", stat.Direction)
	}
	if stat.Protocol != "/dep2p/sys/ping/1.0.0" {
		t.Errorf("Protocol = %q", stat.Protocol)
	}
	if stat.BytesRead != 1024 {
		t.Errorf("BytesRead = %d", stat.BytesRead)
	}
	if stat.BytesWritten != 2048 {
		t.Errorf("BytesWritten = %d", stat.BytesWritten)
	}
}

func TestStreamInfo(t *testing.T) {
	now := time.Now()
	info := StreamInfo{
		ID:         "stream-1",
		Protocol:   ProtocolID("/dep2p/sys/ping/1.0.0"),
		Direction:  DirInbound,
		LocalPeer:  PeerID("local-peer"),
		RemotePeer: PeerID("remote-peer"),
		Opened:     now,
		Stat: StreamStat{
			Direction: DirInbound,
			Opened:    now,
			Protocol:  ProtocolID("/dep2p/sys/ping/1.0.0"),
		},
	}

	if info.ID != "stream-1" {
		t.Errorf("ID = %q", info.ID)
	}
	if info.Protocol != "/dep2p/sys/ping/1.0.0" {
		t.Errorf("Protocol = %q", info.Protocol)
	}
	if info.Direction != DirInbound {
		t.Errorf("Direction = %v", info.Direction)
	}
	if info.LocalPeer != "local-peer" {
		t.Errorf("LocalPeer = %q", info.LocalPeer)
	}
	if info.RemotePeer != "remote-peer" {
		t.Errorf("RemotePeer = %q", info.RemotePeer)
	}
}

func TestStreamState_String(t *testing.T) {
	tests := []struct {
		state StreamState
		want  string
	}{
		{StreamStateOpen, "open"},
		{StreamStateReadClosed, "read-closed"},
		{StreamStateWriteClosed, "write-closed"},
		{StreamStateClosed, "closed"},
		{StreamStateReset, "reset"},
		{StreamState(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := tt.state.String()
			if got != tt.want {
				t.Errorf("StreamState(%d).String() = %q, want %q", tt.state, got, tt.want)
			}
		})
	}
}

func TestStreamScope(t *testing.T) {
	scope := StreamScope{
		StreamID:  "stream-1",
		PeerID:    PeerID("peer-1"),
		Protocol:  ProtocolID("/dep2p/sys/ping/1.0.0"),
		Direction: DirOutbound,
		Memory:    4096,
	}

	if scope.StreamID != "stream-1" {
		t.Errorf("StreamID = %q", scope.StreamID)
	}
	if scope.PeerID != "peer-1" {
		t.Errorf("PeerID = %q", scope.PeerID)
	}
	if scope.Protocol != "/dep2p/sys/ping/1.0.0" {
		t.Errorf("Protocol = %q", scope.Protocol)
	}
	if scope.Direction != DirOutbound {
		t.Errorf("Direction = %v", scope.Direction)
	}
	if scope.Memory != 4096 {
		t.Errorf("Memory = %d", scope.Memory)
	}
}

func TestStreamStatWithData(t *testing.T) {
	stat := StreamStat{
		Direction:    DirInbound,
		Opened:       time.Now().Add(-time.Minute),
		Protocol:     ProtocolID("/test/1.0.0"),
		BytesRead:    10 * 1024 * 1024, // 10 MB
		BytesWritten: 5 * 1024 * 1024,  // 5 MB
	}

	// Verify large values work
	if stat.BytesRead != 10*1024*1024 {
		t.Errorf("BytesRead = %d", stat.BytesRead)
	}
	if stat.BytesWritten != 5*1024*1024 {
		t.Errorf("BytesWritten = %d", stat.BytesWritten)
	}
}
