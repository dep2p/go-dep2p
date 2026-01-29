package eventbus

import (
	"testing"
	"time"

	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
)

// 测试事件类型定义
type EvtPeerConnectedness struct {
	Peer          string
	Connectedness int
}

type EvtPeerIdentified struct {
	Peer      string
	Protocols []string
	AgentVer  string
}

type EvtPeerDiscovered struct {
	Peer   string
	Addrs  []string
	Source string
}

type EvtLocalAddrsUpdated struct {
	Current []string
	Added   []string
	Removed []string
}

// ============================================================================
// 接口集成测试
// ============================================================================

// TestIntegration_InterfaceCompliance 验证接口实现
func TestIntegration_InterfaceCompliance(t *testing.T) {
	var _ pkgif.EventBus = (*Bus)(nil)
	var _ pkgif.Subscription = (*Subscription)(nil)
	var _ pkgif.Emitter = (*Emitter)(nil)
}

// ============================================================================
// 事件类型集成测试
// ============================================================================

// TestIntegration_PeerConnectednessEvent 测试连接事件
func TestIntegration_PeerConnectednessEvent(t *testing.T) {
	bus := NewBus()

	// 订阅连接事件
	sub, err := bus.Subscribe(new(EvtPeerConnectedness))
	if err != nil {
		t.Fatalf("Subscribe() failed: %v", err)
	}
	defer sub.Close()

	// 发射连接事件
	em, err := bus.Emitter(new(EvtPeerConnectedness))
	if err != nil {
		t.Fatalf("Emitter() failed: %v", err)
	}
	defer em.Close()

	testEvent := EvtPeerConnectedness{
		Peer:          "QmTest123",
		Connectedness: 1,
	}

	err = em.Emit(testEvent)
	if err != nil {
		t.Errorf("Emit() failed: %v", err)
	}

	// 接收事件
	select {
	case evt := <-sub.Out():
		received, ok := evt.(EvtPeerConnectedness)
		if !ok {
			t.Fatalf("Received wrong event type: %T", evt)
		}
		if received.Peer != testEvent.Peer {
			t.Errorf("Peer = %s, want %s", received.Peer, testEvent.Peer)
		}
		if received.Connectedness != testEvent.Connectedness {
			t.Errorf("Connectedness = %d, want %d", received.Connectedness, testEvent.Connectedness)
		}
	case <-time.After(time.Second):
		t.Error("Timeout waiting for event")
	}
}

// TestIntegration_PeerIdentifiedEvent 测试身份识别事件
func TestIntegration_PeerIdentifiedEvent(t *testing.T) {
	bus := NewBus()

	sub, _ := bus.Subscribe(new(EvtPeerIdentified))
	defer sub.Close()

	em, _ := bus.Emitter(new(EvtPeerIdentified))
	defer em.Close()

	testEvent := EvtPeerIdentified{
		Peer:      "QmPeer456",
		Protocols: []string{"/ipfs/id/1.0.0", "/ipfs/ping/1.0.0"},
		AgentVer:  "dep2p/1.0.0",
	}

	em.Emit(testEvent)

	select {
	case evt := <-sub.Out():
		received := evt.(EvtPeerIdentified)
		if received.Peer != testEvent.Peer {
			t.Errorf("Peer mismatch")
		}
		if len(received.Protocols) != len(testEvent.Protocols) {
			t.Errorf("Protocols count mismatch")
		}
	case <-time.After(time.Second):
		t.Error("Timeout")
	}
}

// TestIntegration_PeerDiscoveredEvent 测试发现事件
func TestIntegration_PeerDiscoveredEvent(t *testing.T) {
	bus := NewBus()

	sub, _ := bus.Subscribe(new(EvtPeerDiscovered))
	defer sub.Close()

	em, _ := bus.Emitter(new(EvtPeerDiscovered))
	defer em.Close()

	testEvent := EvtPeerDiscovered{
		Peer:   "QmDiscovered",
		Addrs:  []string{"/ip4/127.0.0.1/tcp/4001"},
		Source: "mdns",
	}

	em.Emit(testEvent)

	select {
	case evt := <-sub.Out():
		received := evt.(EvtPeerDiscovered)
		if received.Peer != testEvent.Peer {
			t.Error("Peer mismatch")
		}
		if received.Source != testEvent.Source {
			t.Error("Source mismatch")
		}
	case <-time.After(time.Second):
		t.Error("Timeout")
	}
}

// TestIntegration_LocalAddrsUpdatedEvent 测试地址更新事件
func TestIntegration_LocalAddrsUpdatedEvent(t *testing.T) {
	bus := NewBus()

	sub, _ := bus.Subscribe(new(EvtLocalAddrsUpdated))
	defer sub.Close()

	em, _ := bus.Emitter(new(EvtLocalAddrsUpdated))
	defer em.Close()

	testEvent := EvtLocalAddrsUpdated{
		Current: []string{"/ip4/192.168.1.1/tcp/4001"},
		Added:   []string{"/ip4/192.168.1.1/tcp/4001"},
		Removed: []string{},
	}

	em.Emit(testEvent)

	select {
	case evt := <-sub.Out():
		received := evt.(EvtLocalAddrsUpdated)
		if len(received.Current) != len(testEvent.Current) {
			t.Error("Current addrs mismatch")
		}
	case <-time.After(time.Second):
		t.Error("Timeout")
	}
}

// TestIntegration_MultipleEventTypes 测试多种事件类型
func TestIntegration_MultipleEventTypes(t *testing.T) {
	bus := NewBus()

	// 订阅多种事件
	sub1, _ := bus.Subscribe(new(EvtPeerConnectedness))
	defer sub1.Close()

	sub2, _ := bus.Subscribe(new(EvtPeerDiscovered))
	defer sub2.Close()

	// 发射不同类型的事件
	em1, _ := bus.Emitter(new(EvtPeerConnectedness))
	defer em1.Close()

	em2, _ := bus.Emitter(new(EvtPeerDiscovered))
	defer em2.Close()

	em1.Emit(EvtPeerConnectedness{Peer: "peer1"})
	em2.Emit(EvtPeerDiscovered{Peer: "peer2"})

	// sub1 应该只收到 EvtPeerConnectedness
	select {
	case evt := <-sub1.Out():
		if _, ok := evt.(EvtPeerConnectedness); !ok {
			t.Errorf("sub1 received wrong event type: %T", evt)
		}
	case <-time.After(time.Second):
		t.Error("sub1 timeout")
	}

	// sub2 应该只收到 EvtPeerDiscovered
	select {
	case evt := <-sub2.Out():
		if _, ok := evt.(EvtPeerDiscovered); !ok {
			t.Errorf("sub2 received wrong event type: %T", evt)
		}
	case <-time.After(time.Second):
		t.Error("sub2 timeout")
	}
}

// TestIntegration_GetAllEventTypes 测试获取所有事件类型
func TestIntegration_GetAllEventTypes(t *testing.T) {
	bus := NewBus()

	// 订阅几种事件类型
	sub1, _ := bus.Subscribe(new(EvtPeerConnectedness))
	defer sub1.Close()

	sub2, _ := bus.Subscribe(new(EvtPeerDiscovered))
	defer sub2.Close()

	sub3, _ := bus.Subscribe(new(EvtLocalAddrsUpdated))
	defer sub3.Close()

	// 获取所有事件类型
	types := bus.GetAllEventTypes()

	if len(types) != 3 {
		t.Errorf("GetAllEventTypes() returned %d types, want 3", len(types))
	}

	// 验证类型包含预期的事件
	hasConnectedness := false
	hasDiscovered := false
	hasAddrsUpdated := false

	for _, typ := range types {
		switch typ.(type) {
		case EvtPeerConnectedness:
			hasConnectedness = true
		case EvtPeerDiscovered:
			hasDiscovered = true
		case EvtLocalAddrsUpdated:
			hasAddrsUpdated = true
		}
	}

	if !hasConnectedness {
		t.Error("Missing EvtPeerConnectedness in event types")
	}
	if !hasDiscovered {
		t.Error("Missing EvtPeerDiscovered in event types")
	}
	if !hasAddrsUpdated {
		t.Error("Missing EvtLocalAddrsUpdated in event types")
	}
}
