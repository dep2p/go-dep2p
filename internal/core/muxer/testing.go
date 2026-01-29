package muxer

import (
	"net"
	"testing"

	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// testConnPair 创建测试用的连接对
func testConnPair(t *testing.T) (net.Conn, net.Conn) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	var serverConn net.Conn
	done := make(chan struct{})
	go func() {
		serverConn, _ = ln.Accept()
		close(done)
	}()

	clientConn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	<-done

	return clientConn, serverConn
}

// mockPeerScope 模拟 PeerScope（用于测试）
type mockPeerScope struct{}

func (m *mockPeerScope) BeginSpan() (pkgif.ResourceScopeSpan, error) {
	return &mockSpan{}, nil
}

func (m *mockPeerScope) Stat() pkgif.ScopeStat {
	return pkgif.ScopeStat{}
}

func (m *mockPeerScope) Peer() types.PeerID {
	return types.PeerID("mock-peer")
}

// mockSpan 模拟 ResourceScopeSpan（用于测试）
type mockSpan struct{}

func (m *mockSpan) ReserveMemory(_ int, _ uint8) error {
	return nil
}

func (m *mockSpan) ReleaseMemory(_ int) {}

func (m *mockSpan) Done() {}

func (m *mockSpan) Stat() pkgif.ScopeStat {
	return pkgif.ScopeStat{}
}

func (m *mockSpan) BeginSpan() (pkgif.ResourceScopeSpan, error) {
	return &mockSpan{}, nil
}

// errorPeerScope 模拟返回错误的 PeerScope（用于测试错误路径）
type errorPeerScope struct {
	err error
}

func (m *errorPeerScope) BeginSpan() (pkgif.ResourceScopeSpan, error) {
	return nil, m.err
}

func (m *errorPeerScope) Stat() pkgif.ScopeStat {
	return pkgif.ScopeStat{}
}

func (m *errorPeerScope) Peer() types.PeerID {
	return types.PeerID("error-peer")
}
