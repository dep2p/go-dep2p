package muxer

import (
	"testing"

	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
)

// ============================================================================
// 接口契约测试
// ============================================================================

// TestTransport_ImplementsInterface 验证 Transport 实现接口
func TestTransport_ImplementsInterface(t *testing.T) {
	var _ pkgif.StreamMuxer = (*Transport)(nil)
}

// ============================================================================
// 基础功能测试
// ============================================================================

// TestTransport_NewConn_AsServer 测试服务端创建连接
func TestTransport_NewConn_AsServer(t *testing.T) {
	transport := NewTransport()

	clientConn, serverConn := testConnPair(t)
	defer clientConn.Close()
	defer serverConn.Close()

	muxedConn, err := transport.NewConn(serverConn, true, nil)
	if err != nil {
		t.Fatalf("NewConn() as server failed: %v", err)
	}
	defer muxedConn.Close()

	if muxedConn == nil {
		t.Fatal("NewConn() returned nil")
	}
}

// TestTransport_NewConn_AsClient 测试客户端创建连接
func TestTransport_NewConn_AsClient(t *testing.T) {
	transport := NewTransport()

	clientConn, serverConn := testConnPair(t)
	defer clientConn.Close()
	defer serverConn.Close()

	muxedConn, err := transport.NewConn(clientConn, false, nil)
	if err != nil {
		t.Fatalf("NewConn() as client failed: %v", err)
	}
	defer muxedConn.Close()

	if muxedConn == nil {
		t.Fatal("NewConn() returned nil")
	}
}

// TestTransport_ID 测试获取协议 ID
func TestTransport_ID(t *testing.T) {
	transport := NewTransport()

	id := transport.ID()
	expected := "/yamux/1.0.0"

	if id != expected {
		t.Errorf("ID() = %s, want %s", id, expected)
	}
}

// TestTransport_WithPeerScope 测试带 PeerScope 创建连接
func TestTransport_WithPeerScope(t *testing.T) {
	transport := NewTransport()

	clientConn, serverConn := testConnPair(t)
	defer clientConn.Close()
	defer serverConn.Close()

	scope := &mockPeerScope{}

	muxedConn, err := transport.NewConn(serverConn, true, scope)
	if err != nil {
		t.Fatalf("NewConn() with scope failed: %v", err)
	}
	defer muxedConn.Close()

	if muxedConn == nil {
		t.Fatal("NewConn() returned nil")
	}
}
