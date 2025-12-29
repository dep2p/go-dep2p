package quic

import (
	"testing"

	"github.com/dep2p/go-dep2p/internal/core/identity"
	identityif "github.com/dep2p/go-dep2p/pkg/interfaces/identity"
	transportif "github.com/dep2p/go-dep2p/pkg/interfaces/transport"
)

// TestListenerBasicProperties 测试监听器基本属性
func TestListenerBasicProperties(t *testing.T) {
	mgr := identity.NewManager(identityif.DefaultConfig())
	id, _ := mgr.Create()
	transport, _ := NewTransport(transportif.DefaultConfig(), id)
	defer transport.Close()

	listener, err := transport.Listen(NewAddress("127.0.0.1", 0))
	if err != nil {
		t.Fatalf("监听失败: %v", err)
	}
	defer func() { _ = listener.Close() }()

	// 测试地址
	addr := listener.Addr()
	if addr == nil {
		t.Error("地址不应为 nil")
	}
	t.Logf("监听地址: %s", addr.String())

	// 测试多地址
	multiaddr := listener.Multiaddr()
	if multiaddr == "" {
		t.Error("多地址不应为空")
	}
	t.Logf("多地址: %s", multiaddr)

	// 测试 QuicListener
	quicListener := listener.(*Listener)
	if quicListener.QuicListener() == nil {
		t.Error("QuicListener 不应为 nil")
	}

	// 测试关闭状态
	if quicListener.IsClosed() {
		t.Error("新监听器不应处于关闭状态")
	}
}

// TestListenerClose 测试监听器关闭
func TestListenerClose(t *testing.T) {
	mgr := identity.NewManager(identityif.DefaultConfig())
	id, _ := mgr.Create()
	transport, _ := NewTransport(transportif.DefaultConfig(), id)
	defer transport.Close()

	listener, err := transport.Listen(NewAddress("127.0.0.1", 0))
	if err != nil {
		t.Fatalf("监听失败: %v", err)
	}

	quicListener := listener.(*Listener)

	// 关闭监听器
	if err := listener.Close(); err != nil {
		t.Errorf("关闭监听器失败: %v", err)
	}

	// 验证已关闭
	if !quicListener.IsClosed() {
		t.Error("监听器应处于关闭状态")
	}

	// 重复关闭不应报错
	if err := listener.Close(); err != nil {
		t.Errorf("重复关闭不应报错: %v", err)
	}
}

// TestListenerAcceptAfterClose 测试关闭后接受连接
func TestListenerAcceptAfterClose(t *testing.T) {
	mgr := identity.NewManager(identityif.DefaultConfig())
	id, _ := mgr.Create()
	transport, _ := NewTransport(transportif.DefaultConfig(), id)
	defer transport.Close()

	listener, err := transport.Listen(NewAddress("127.0.0.1", 0))
	if err != nil {
		t.Fatalf("监听失败: %v", err)
	}

	// 关闭监听器
	listener.Close()

	// 尝试接受连接
	_, err = listener.Accept()
	if err == nil {
		t.Error("在关闭的监听器上接受连接应返回错误")
	}
}

// TestListenerMultipleAddresses 测试监听多个地址
func TestListenerMultipleAddresses(t *testing.T) {
	mgr := identity.NewManager(identityif.DefaultConfig())
	id, _ := mgr.Create()
	transport, _ := NewTransport(transportif.DefaultConfig(), id)
	defer transport.Close()

	// 监听第一个地址
	listener1, err := transport.Listen(NewAddress("127.0.0.1", 0))
	if err != nil {
		t.Fatalf("监听失败: %v", err)
	}
	defer listener1.Close()

	// 监听第二个地址
	listener2, err := transport.Listen(NewAddress("127.0.0.1", 0))
	if err != nil {
		t.Fatalf("监听第二个地址失败: %v", err)
	}
	defer listener2.Close()

	// 验证两个地址不同
	addr1 := listener1.Addr().String()
	addr2 := listener2.Addr().String()

	if addr1 == addr2 {
		t.Error("两个监听器的地址应该不同")
	}

	t.Logf("监听器1: %s", addr1)
	t.Logf("监听器2: %s", addr2)

	// 验证监听器数量
	if transport.ListenersCount() != 2 {
		t.Errorf("应有 2 个监听器，实际有 %d 个", transport.ListenersCount())
	}
}

