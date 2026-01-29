package resourcemgr

import (
	"testing"

	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
// 限制测试
// ============================================================================

// TestLimit_ExceedConnLimit 测试超出连接限制
func TestLimit_ExceedConnLimit(t *testing.T) {
	limits := DefaultLimitConfig()
	limits.System.Conns = 2           // 最多 2 个连接
	limits.System.ConnsInbound = 1    // 最多 1 个入站连接
	limits.System.ConnsOutbound = 1   // 最多 1 个出站连接

	rm, err := NewResourceManager(limits)
	if err != nil {
		t.Fatalf("NewResourceManager() failed: %v", err)
	}
	defer rm.Close()

	addr := mustMultiaddr("/ip4/127.0.0.1/tcp/4001")

	// 第一个入站连接应该成功
	conn1, err := rm.OpenConnection(pkgif.DirInbound, true, addr)
	if err != nil {
		t.Fatalf("First OpenConnection() failed: %v", err)
	}
	defer conn1.Done()

	// 第二个入站连接应该失败（超出 ConnsInbound 限制）
	_, err = rm.OpenConnection(pkgif.DirInbound, true, addr)
	if err == nil {
		t.Error("Second inbound connection should have failed (exceeded ConnsInbound limit)")
	}

	// 出站连接应该成功
	conn2, err := rm.OpenConnection(pkgif.DirOutbound, true, addr)
	if err != nil {
		t.Fatalf("Outbound OpenConnection() failed: %v", err)
	}
	defer conn2.Done()

	// 第三个连接应该失败（超出总连接限制）
	_, err = rm.OpenConnection(pkgif.DirOutbound, true, addr)
	if err == nil {
		t.Error("Third connection should have failed (exceeded total Conns limit)")
	}
}

// TestLimit_ExceedStreamLimit 测试超出流限制
func TestLimit_ExceedStreamLimit(t *testing.T) {
	limits := DefaultLimitConfig()
	limits.System.Streams = 2         // 最多 2 个流
	limits.System.StreamsInbound = 1  // 最多 1 个入站流
	limits.System.StreamsOutbound = 1 // 最多 1 个出站流

	rm, err := NewResourceManager(limits)
	if err != nil {
		t.Fatalf("NewResourceManager() failed: %v", err)
	}
	defer rm.Close()

	peerID := types.PeerID("QmPeer")

	// 第一个入站流应该成功
	stream1, err := rm.OpenStream(peerID, pkgif.DirInbound)
	if err != nil {
		t.Fatalf("First OpenStream() failed: %v", err)
	}
	defer stream1.Done()

	// 第二个入站流应该失败
	_, err = rm.OpenStream(peerID, pkgif.DirInbound)
	if err == nil {
		t.Error("Second inbound stream should have failed (exceeded StreamsInbound limit)")
	}

	// 出站流应该成功
	stream2, err := rm.OpenStream(peerID, pkgif.DirOutbound)
	if err != nil {
		t.Fatalf("Outbound OpenStream() failed: %v", err)
	}
	defer stream2.Done()

	// 第三个流应该失败
	_, err = rm.OpenStream(peerID, pkgif.DirOutbound)
	if err == nil {
		t.Error("Third stream should have failed (exceeded total Streams limit)")
	}
}

// TestLimit_ExceedMemoryLimit 测试超出内存限制
func TestLimit_ExceedMemoryLimit(t *testing.T) {
	limits := DefaultLimitConfig()
	limits.System.Memory = 1024 // 仅 1KB 内存

	rm, err := NewResourceManager(limits)
	if err != nil {
		t.Fatalf("NewResourceManager() failed: %v", err)
	}
	defer rm.Close()

	err = rm.ViewSystem(func(s pkgif.ResourceScope) error {
		span, err := s.BeginSpan()
		if err != nil {
			return err
		}
		defer span.Done()

		// 预留 512 字节应该成功
		err = span.ReserveMemory(512, pkgif.ReservationPriorityAlways)
		if err != nil {
			t.Errorf("ReserveMemory(512) failed: %v", err)
		}

		// 预留 600 字节应该失败（总共超过 1024）
		err = span.ReserveMemory(600, pkgif.ReservationPriorityAlways)
		if err == nil {
			t.Error("ReserveMemory(600) should have failed (exceeded Memory limit)")
		}

		return nil
	})
	if err != nil {
		t.Fatalf("ViewSystem() failed: %v", err)
	}
}

// TestLimit_ExceedFDLimit 测试超出文件描述符限制
func TestLimit_ExceedFDLimit(t *testing.T) {
	limits := DefaultLimitConfig()
	limits.System.FD = 1     // 最多 1 个 FD
	limits.System.Conns = 10 // 连接数足够多

	rm, err := NewResourceManager(limits)
	if err != nil {
		t.Fatalf("NewResourceManager() failed: %v", err)
	}
	defer rm.Close()

	addr := mustMultiaddr("/ip4/127.0.0.1/tcp/4001")

	// 第一个连接（使用 FD）应该成功
	conn1, err := rm.OpenConnection(pkgif.DirInbound, true, addr)
	if err != nil {
		t.Fatalf("First OpenConnection() failed: %v", err)
	}
	defer conn1.Done()

	// 第二个连接（使用 FD）应该失败
	_, err = rm.OpenConnection(pkgif.DirInbound, true, addr)
	if err == nil {
		t.Error("Second connection with FD should have failed (exceeded FD limit)")
	}

	// 不使用 FD 的连接应该成功
	conn2, err := rm.OpenConnection(pkgif.DirInbound, false, addr)
	if err != nil {
		t.Fatalf("Connection without FD failed: %v", err)
	}
	defer conn2.Done()
}

// TestLimit_Direction 测试方向限制
func TestLimit_Direction(t *testing.T) {
	limits := DefaultLimitConfig()
	limits.System.Conns = 10
	limits.System.ConnsInbound = 2
	limits.System.ConnsOutbound = 3

	rm, err := NewResourceManager(limits)
	if err != nil {
		t.Fatalf("NewResourceManager() failed: %v", err)
	}
	defer rm.Close()

	addr := mustMultiaddr("/ip4/127.0.0.1/tcp/4001")

	// 打开 2 个入站连接
	for i := 0; i < 2; i++ {
		conn, err := rm.OpenConnection(pkgif.DirInbound, false, addr)
		if err != nil {
			t.Fatalf("OpenConnection() inbound %d failed: %v", i, err)
		}
		defer conn.Done()
	}

	// 第三个入站连接应该失败
	_, err = rm.OpenConnection(pkgif.DirInbound, false, addr)
	if err == nil {
		t.Error("Third inbound connection should have failed")
	}

	// 出站连接应该还可以建立（不同方向）
	for i := 0; i < 3; i++ {
		conn, err := rm.OpenConnection(pkgif.DirOutbound, false, addr)
		if err != nil {
			t.Fatalf("OpenConnection() outbound %d failed: %v", i, err)
		}
		defer conn.Done()
	}

	// 第四个出站连接应该失败
	_, err = rm.OpenConnection(pkgif.DirOutbound, false, addr)
	if err == nil {
		t.Error("Fourth outbound connection should have failed")
	}
}
