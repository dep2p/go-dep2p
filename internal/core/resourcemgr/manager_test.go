package resourcemgr

import (
	"testing"

	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
// 接口契约测试
// ============================================================================

// TestManager_ImplementsInterface 验证 ResourceManager 实现接口
func TestManager_ImplementsInterface(t *testing.T) {
	var _ pkgif.ResourceManager = (*resourceManager)(nil)
}

// ============================================================================
// 基础功能测试
// ============================================================================

// TestManager_NewResourceManager 测试创建资源管理器
func TestManager_NewResourceManager(t *testing.T) {
	limits := DefaultLimitConfig()
	rm, err := NewResourceManager(limits)
	if err != nil {
		t.Fatalf("NewResourceManager() failed: %v", err)
	}
	defer rm.Close()

	if rm == nil {
		t.Fatal("NewResourceManager() returned nil")
	}
}

// TestManager_ViewSystem 测试查看系统作用域
func TestManager_ViewSystem(t *testing.T) {
	limits := DefaultLimitConfig()
	rm, err := NewResourceManager(limits)
	if err != nil {
		t.Fatalf("NewResourceManager() failed: %v", err)
	}
	defer rm.Close()

	err = rm.ViewSystem(func(s pkgif.ResourceScope) error {
		stat := s.Stat()
		if stat.NumConnsInbound != 0 {
			t.Errorf("Initial NumConnsInbound = %d, want 0", stat.NumConnsInbound)
		}
		if stat.Memory != 0 {
			t.Errorf("Initial Memory = %d, want 0", stat.Memory)
		}
		return nil
	})
	if err != nil {
		t.Errorf("ViewSystem() failed: %v", err)
	}
}

// TestManager_ViewTransient 测试查看临时作用域
func TestManager_ViewTransient(t *testing.T) {
	limits := DefaultLimitConfig()
	rm, err := NewResourceManager(limits)
	if err != nil {
		t.Fatalf("NewResourceManager() failed: %v", err)
	}
	defer rm.Close()

	err = rm.ViewTransient(func(s pkgif.ResourceScope) error {
		stat := s.Stat()
		if stat.NumStreamsInbound != 0 {
			t.Errorf("Initial NumStreamsInbound = %d, want 0", stat.NumStreamsInbound)
		}
		return nil
	})
	if err != nil {
		t.Errorf("ViewTransient() failed: %v", err)
	}
}

// TestManager_ViewPeer 测试查看节点作用域
func TestManager_ViewPeer(t *testing.T) {
	limits := DefaultLimitConfig()
	rm, err := NewResourceManager(limits)
	if err != nil {
		t.Fatalf("NewResourceManager() failed: %v", err)
	}
	defer rm.Close()

	peerID := types.PeerID("QmTest123")

	err = rm.ViewPeer(peerID, func(s pkgif.PeerScope) error {
		if s.Peer() != peerID {
			t.Errorf("Peer() = %v, want %v", s.Peer(), peerID)
		}

		stat := s.Stat()
		if stat.NumConnsInbound != 0 {
			t.Errorf("Initial NumConnsInbound = %d, want 0", stat.NumConnsInbound)
		}
		return nil
	})
	if err != nil {
		t.Errorf("ViewPeer() failed: %v", err)
	}
}

// TestManager_OpenConnection 测试打开连接作用域
func TestManager_OpenConnection(t *testing.T) {
	limits := DefaultLimitConfig()
	rm, err := NewResourceManager(limits)
	if err != nil {
		t.Fatalf("NewResourceManager() failed: %v", err)
	}
	defer rm.Close()

	addr := mustMultiaddr("/ip4/127.0.0.1/tcp/4001")

	connScope, err := rm.OpenConnection(pkgif.DirInbound, true, addr)
	if err != nil {
		t.Fatalf("OpenConnection() failed: %v", err)
	}
	defer connScope.Done()

	stat := connScope.Stat()
	if stat.NumConnsInbound != 1 {
		t.Errorf("NumConnsInbound = %d, want 1", stat.NumConnsInbound)
	}
	if stat.NumFD != 1 {
		t.Errorf("NumFD = %d, want 1", stat.NumFD)
	}
}

// TestManager_OpenStream 测试打开流作用域
func TestManager_OpenStream(t *testing.T) {
	limits := DefaultLimitConfig()
	rm, err := NewResourceManager(limits)
	if err != nil {
		t.Fatalf("NewResourceManager() failed: %v", err)
	}
	defer rm.Close()

	peerID := types.PeerID("QmPeer456")

	streamScope, err := rm.OpenStream(peerID, pkgif.DirOutbound)
	if err != nil {
		t.Fatalf("OpenStream() failed: %v", err)
	}
	defer streamScope.Done()

	stat := streamScope.Stat()
	if stat.NumStreamsOutbound != 1 {
		t.Errorf("NumStreamsOutbound = %d, want 1", stat.NumStreamsOutbound)
	}
}

// TestManager_Close 测试关闭资源管理器
func TestManager_Close(t *testing.T) {
	limits := DefaultLimitConfig()
	rm, err := NewResourceManager(limits)
	if err != nil {
		t.Fatalf("NewResourceManager() failed: %v", err)
	}

	err = rm.Close()
	if err != nil {
		t.Errorf("Close() failed: %v", err)
	}

	// 关闭后再次关闭应该成功（幂等）
	err = rm.Close()
	if err != nil {
		t.Errorf("Close() second time failed: %v", err)
	}
}
