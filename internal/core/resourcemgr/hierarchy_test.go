package resourcemgr

import (
	"testing"

	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
// 层次测试
// ============================================================================

// TestHierarchy_SystemToConnection 测试从系统到连接的层次
func TestHierarchy_SystemToConnection(t *testing.T) {
	limits := DefaultLimitConfig()
	rm, err := NewResourceManager(limits)
	if err != nil {
		t.Fatalf("NewResourceManager() failed: %v", err)
	}
	defer rm.Close()

	addr := mustMultiaddr("/ip4/127.0.0.1/tcp/4001")

	// 打开连接
	connScope, err := rm.OpenConnection(pkgif.DirInbound, true, addr)
	if err != nil {
		t.Fatalf("OpenConnection() failed: %v", err)
	}
	defer connScope.Done()

	// 检查系统作用域的统计
	err = rm.ViewSystem(func(s pkgif.ResourceScope) error {
		stat := s.Stat()
		if stat.NumConnsInbound != 1 {
			t.Errorf("System NumConnsInbound = %d, want 1", stat.NumConnsInbound)
		}
		if stat.NumFD != 1 {
			t.Errorf("System NumFD = %d, want 1", stat.NumFD)
		}
		return nil
	})
	if err != nil {
		t.Errorf("ViewSystem() failed: %v", err)
	}

	// 检查连接作用域的统计
	connStat := connScope.Stat()
	if connStat.NumConnsInbound != 1 {
		t.Errorf("Connection NumConnsInbound = %d, want 1", connStat.NumConnsInbound)
	}
}

// TestHierarchy_PeerScope 测试节点作用域层次
func TestHierarchy_PeerScope(t *testing.T) {
	limits := DefaultLimitConfig()
	rm, err := NewResourceManager(limits)
	if err != nil {
		t.Fatalf("NewResourceManager() failed: %v", err)
	}
	defer rm.Close()

	peerID := types.PeerID("QmPeer123")
	addr := mustMultiaddr("/ip4/127.0.0.1/tcp/4001")

	// 打开连接并设置节点
	connScope, err := rm.OpenConnection(pkgif.DirInbound, false, addr)
	if err != nil {
		t.Fatalf("OpenConnection() failed: %v", err)
	}
	defer connScope.Done()

	err = connScope.SetPeer(peerID)
	if err != nil {
		t.Fatalf("SetPeer() failed: %v", err)
	}

	// 检查节点作用域的统计
	err = rm.ViewPeer(peerID, func(s pkgif.PeerScope) error {
		if s.Peer() != peerID {
			t.Errorf("Peer() = %v, want %v", s.Peer(), peerID)
		}

		stat := s.Stat()
		if stat.NumConnsInbound != 1 {
			t.Errorf("Peer NumConnsInbound = %d, want 1", stat.NumConnsInbound)
		}
		return nil
	})
	if err != nil {
		t.Errorf("ViewPeer() failed: %v", err)
	}

	// 检查连接作用域能访问节点作用域
	peerScope := connScope.PeerScope()
	if peerScope == nil {
		t.Fatal("PeerScope() returned nil")
	}
	if peerScope.Peer() != peerID {
		t.Errorf("PeerScope().Peer() = %v, want %v", peerScope.Peer(), peerID)
	}
}

// TestHierarchy_ServiceScope 测试服务作用域层次
func TestHierarchy_ServiceScope(t *testing.T) {
	limits := DefaultLimitConfig()
	rm, err := NewResourceManager(limits)
	if err != nil {
		t.Fatalf("NewResourceManager() failed: %v", err)
	}
	defer rm.Close()

	serviceName := "test-service"
	peerID := types.PeerID("QmPeer456")

	// 打开流并设置服务
	streamScope, err := rm.OpenStream(peerID, pkgif.DirOutbound)
	if err != nil {
		t.Fatalf("OpenStream() failed: %v", err)
	}
	defer streamScope.Done()

	err = streamScope.SetService(serviceName)
	if err != nil {
		t.Fatalf("SetService() failed: %v", err)
	}

	// 检查服务作用域的统计
	err = rm.ViewService(serviceName, func(s pkgif.ServiceScope) error {
		if s.Name() != serviceName {
			t.Errorf("Name() = %v, want %v", s.Name(), serviceName)
		}

		stat := s.Stat()
		if stat.NumStreamsOutbound != 1 {
			t.Errorf("Service NumStreamsOutbound = %d, want 1", stat.NumStreamsOutbound)
		}
		return nil
	})
	if err != nil {
		t.Errorf("ViewService() failed: %v", err)
	}

	// 检查流作用域能访问服务作用域
	svcScope := streamScope.ServiceScope()
	if svcScope == nil {
		t.Fatal("ServiceScope() returned nil")
	}
	if svcScope.Name() != serviceName {
		t.Errorf("ServiceScope().Name() = %v, want %v", svcScope.Name(), serviceName)
	}
}

// TestHierarchy_ProtocolScope 测试协议作用域层次
func TestHierarchy_ProtocolScope(t *testing.T) {
	limits := DefaultLimitConfig()
	rm, err := NewResourceManager(limits)
	if err != nil {
		t.Fatalf("NewResourceManager() failed: %v", err)
	}
	defer rm.Close()

	protoID := types.ProtocolID("/test/protocol/1.0.0")
	peerID := types.PeerID("QmPeer789")

	// 打开流并设置协议
	streamScope, err := rm.OpenStream(peerID, pkgif.DirInbound)
	if err != nil {
		t.Fatalf("OpenStream() failed: %v", err)
	}
	defer streamScope.Done()

	err = streamScope.SetProtocol(protoID)
	if err != nil {
		t.Fatalf("SetProtocol() failed: %v", err)
	}

	// 检查协议作用域的统计
	err = rm.ViewProtocol(protoID, func(s pkgif.ProtocolScope) error {
		if s.Protocol() != protoID {
			t.Errorf("Protocol() = %v, want %v", s.Protocol(), protoID)
		}

		stat := s.Stat()
		if stat.NumStreamsInbound != 1 {
			t.Errorf("Protocol NumStreamsInbound = %d, want 1", stat.NumStreamsInbound)
		}
		return nil
	})
	if err != nil {
		t.Errorf("ViewProtocol() failed: %v", err)
	}

	// 检查流作用域能访问协议作用域
	protoScope := streamScope.ProtocolScope()
	if protoScope == nil {
		t.Fatal("ProtocolScope() returned nil")
	}
	if protoScope.Protocol() != protoID {
		t.Errorf("ProtocolScope().Protocol() = %v, want %v", protoScope.Protocol(), protoID)
	}
}
