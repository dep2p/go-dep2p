package resourcemgr

import (
	"errors"
	"testing"

	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
// 边界条件和错误路径测试
// ============================================================================

// TestEdge_CloseAfterUse 测试关闭后使用
func TestEdge_CloseAfterUse(t *testing.T) {
	limits := DefaultLimitConfig()
	rm, err := NewResourceManager(limits)
	if err != nil {
		t.Fatalf("NewResourceManager() failed: %v", err)
	}

	// 关闭资源管理器
	rm.Close()

	// 尝试打开连接应该失败
	_, err = rm.OpenConnection(pkgif.DirInbound, false, mustMultiaddr("/ip4/127.0.0.1/tcp/4001"))
	if err == nil {
		t.Error("OpenConnection() after Close() should fail")
	}

	// 尝试打开流应该失败
	_, err = rm.OpenStream(types.PeerID("QmPeer"), pkgif.DirOutbound)
	if err == nil {
		t.Error("OpenStream() after Close() should fail")
	}
}

// TestEdge_ViewWithError 测试 View 函数返回错误
func TestEdge_ViewWithError(t *testing.T) {
	limits := DefaultLimitConfig()
	rm, err := NewResourceManager(limits)
	if err != nil {
		t.Fatalf("NewResourceManager() failed: %v", err)
	}
	defer rm.Close()

	testErr := errors.New("test error")

	err = rm.ViewSystem(func(s pkgif.ResourceScope) error {
		return testErr
	})
	if err != testErr {
		t.Errorf("ViewSystem() returned %v, want %v", err, testErr)
	}
}

// TestEdge_SetPeerTwice 测试多次设置节点
func TestEdge_SetPeerTwice(t *testing.T) {
	limits := DefaultLimitConfig()
	rm, err := NewResourceManager(limits)
	if err != nil {
		t.Fatalf("NewResourceManager() failed: %v", err)
	}
	defer rm.Close()

	addr := mustMultiaddr("/ip4/127.0.0.1/tcp/4001")
	peerID := types.PeerID("QmPeer123")

	connScope, err := rm.OpenConnection(pkgif.DirInbound, false, addr)
	if err != nil {
		t.Fatalf("OpenConnection() failed: %v", err)
	}
	defer connScope.Done()

	// 第一次设置节点
	err = connScope.SetPeer(peerID)
	if err != nil {
		t.Fatalf("SetPeer() first time failed: %v", err)
	}

	// 第二次设置节点应该是幂等的
	err = connScope.SetPeer(peerID)
	if err != nil {
		t.Errorf("SetPeer() second time failed: %v", err)
	}

	// 尝试设置不同的节点应该成功（幂等）
	err = connScope.SetPeer(types.PeerID("QmOtherPeer"))
	if err != nil {
		t.Errorf("SetPeer() with different peer failed: %v", err)
	}
}

// TestEdge_SetProtocolTwice 测试多次设置协议
func TestEdge_SetProtocolTwice(t *testing.T) {
	limits := DefaultLimitConfig()
	rm, err := NewResourceManager(limits)
	if err != nil {
		t.Fatalf("NewResourceManager() failed: %v", err)
	}
	defer rm.Close()

	peerID := types.PeerID("QmPeer")
	protoID := types.ProtocolID("/test/1.0.0")

	streamScope, err := rm.OpenStream(peerID, pkgif.DirOutbound)
	if err != nil {
		t.Fatalf("OpenStream() failed: %v", err)
	}
	defer streamScope.Done()

	// 第一次设置协议
	err = streamScope.SetProtocol(protoID)
	if err != nil {
		t.Fatalf("SetProtocol() first time failed: %v", err)
	}

	// 第二次设置协议应该是幂等的
	err = streamScope.SetProtocol(protoID)
	if err != nil {
		t.Errorf("SetProtocol() second time failed: %v", err)
	}
}

// TestEdge_SetServiceTwice 测试多次设置服务
func TestEdge_SetServiceTwice(t *testing.T) {
	limits := DefaultLimitConfig()
	rm, err := NewResourceManager(limits)
	if err != nil {
		t.Fatalf("NewResourceManager() failed: %v", err)
	}
	defer rm.Close()

	peerID := types.PeerID("QmPeer")
	serviceName := "test-service"

	streamScope, err := rm.OpenStream(peerID, pkgif.DirOutbound)
	if err != nil {
		t.Fatalf("OpenStream() failed: %v", err)
	}
	defer streamScope.Done()

	// 第一次设置服务
	err = streamScope.SetService(serviceName)
	if err != nil {
		t.Fatalf("SetService() first time failed: %v", err)
	}

	// 第二次设置服务应该是幂等的
	err = streamScope.SetService(serviceName)
	if err != nil {
		t.Errorf("SetService() second time failed: %v", err)
	}
}

// TestEdge_NilLimitConfig 测试 nil 配置
func TestEdge_NilLimitConfig(t *testing.T) {
	rm, err := NewResourceManager(nil)
	if err != nil {
		t.Fatalf("NewResourceManager(nil) failed: %v", err)
	}
	defer rm.Close()

	// 应该使用默认配置
	err = rm.ViewSystem(func(s pkgif.ResourceScope) error {
		stat := s.Stat()
		if stat.Memory != 0 {
			t.Errorf("Initial memory = %d, want 0", stat.Memory)
		}
		return nil
	})
	if err != nil {
		t.Errorf("ViewSystem() failed: %v", err)
	}
}

// TestEdge_ConnScopeMemory 测试连接作用域内存操作
func TestEdge_ConnScopeMemory(t *testing.T) {
	limits := DefaultLimitConfig()
	rm, err := NewResourceManager(limits)
	if err != nil {
		t.Fatalf("NewResourceManager() failed: %v", err)
	}
	defer rm.Close()

	addr := mustMultiaddr("/ip4/127.0.0.1/tcp/4001")

	connScope, err := rm.OpenConnection(pkgif.DirInbound, false, addr)
	if err != nil {
		t.Fatalf("OpenConnection() failed: %v", err)
	}
	defer connScope.Done()

	// 连接作用域应该能预留内存
	err = connScope.ReserveMemory(512, pkgif.ReservationPriorityAlways)
	if err != nil {
		t.Errorf("ReserveMemory() failed: %v", err)
	}

	// 释放内存
	connScope.ReleaseMemory(512)

	stat := connScope.Stat()
	if stat.Memory != 0 {
		t.Errorf("Memory after release = %d, want 0", stat.Memory)
	}
}

// TestEdge_StreamScopeMemory 测试流作用域内存操作
func TestEdge_StreamScopeMemory(t *testing.T) {
	limits := DefaultLimitConfig()
	rm, err := NewResourceManager(limits)
	if err != nil {
		t.Fatalf("NewResourceManager() failed: %v", err)
	}
	defer rm.Close()

	peerID := types.PeerID("QmPeer")

	streamScope, err := rm.OpenStream(peerID, pkgif.DirOutbound)
	if err != nil {
		t.Fatalf("OpenStream() failed: %v", err)
	}
	defer streamScope.Done()

	// 流作用域应该能预留内存
	err = streamScope.ReserveMemory(256, pkgif.ReservationPriorityAlways)
	if err != nil {
		t.Errorf("ReserveMemory() failed: %v", err)
	}

	// 释放内存
	streamScope.ReleaseMemory(256)

	stat := streamScope.Stat()
	if stat.Memory != 0 {
		t.Errorf("Memory after release = %d, want 0", stat.Memory)
	}
}

// TestEdge_ProtectPeer 测试保护节点
func TestEdge_ProtectPeer(t *testing.T) {
	limits := DefaultLimitConfig()
	rm, err := NewResourceManager(limits)
	if err != nil {
		t.Fatalf("NewResourceManager() failed: %v", err)
	}
	defer rm.Close()

	addr := mustMultiaddr("/ip4/127.0.0.1/tcp/4001")
	peerID := types.PeerID("QmProtected")

	connScope, err := rm.OpenConnection(pkgif.DirInbound, false, addr)
	if err != nil {
		t.Fatalf("OpenConnection() failed: %v", err)
	}
	defer connScope.Done()

	// ProtectPeer 应该不会 panic（即使是占位符）
	connScope.ProtectPeer(peerID)
}

// TestEdge_ServiceScopeWithProtocol 测试服务和协议同时设置
func TestEdge_ServiceScopeWithProtocol(t *testing.T) {
	limits := DefaultLimitConfig()
	rm, err := NewResourceManager(limits)
	if err != nil {
		t.Fatalf("NewResourceManager() failed: %v", err)
	}
	defer rm.Close()

	peerID := types.PeerID("QmPeer")
	serviceName := "my-service"
	protoID := types.ProtocolID("/my/proto/1.0.0")

	streamScope, err := rm.OpenStream(peerID, pkgif.DirInbound)
	if err != nil {
		t.Fatalf("OpenStream() failed: %v", err)
	}
	defer streamScope.Done()

	// 先设置协议
	err = streamScope.SetProtocol(protoID)
	if err != nil {
		t.Fatalf("SetProtocol() failed: %v", err)
	}

	// 然后设置服务
	err = streamScope.SetService(serviceName)
	if err != nil {
		t.Fatalf("SetService() failed: %v", err)
	}

	// 验证服务作用域
	svcScope := streamScope.ServiceScope()
	if svcScope == nil {
		t.Fatal("ServiceScope() returned nil")
	}
	if svcScope.Name() != serviceName {
		t.Errorf("Name() = %s, want %s", svcScope.Name(), serviceName)
	}

	// 验证协议作用域
	protoScope := streamScope.ProtocolScope()
	if protoScope == nil {
		t.Fatal("ProtocolScope() returned nil")
	}
	if protoScope.Protocol() != protoID {
		t.Errorf("Protocol() = %s, want %s", protoScope.Protocol(), protoID)
	}
}
