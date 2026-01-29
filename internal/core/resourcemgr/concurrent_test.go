package resourcemgr

import (
	"sync"
	"testing"

	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
// 并发测试
// ============================================================================

// TestConcurrent_OpenConnections 测试并发打开连接
func TestConcurrent_OpenConnections(t *testing.T) {
	limits := DefaultLimitConfig()
	limits.System.Conns = 100

	rm, err := NewResourceManager(limits)
	if err != nil {
		t.Fatalf("NewResourceManager() failed: %v", err)
	}
	defer rm.Close()

	addr := mustMultiaddr("/ip4/127.0.0.1/tcp/4001")

	numGoroutines := 50
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	errors := make(chan error, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()

			connScope, err := rm.OpenConnection(pkgif.DirInbound, false, addr)
			if err != nil {
				errors <- err
				return
			}
			defer connScope.Done()

			// 模拟一些工作
			stat := connScope.Stat()
			if stat.NumConnsInbound < 1 {
				errors <- err
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	// 检查错误
	for err := range errors {
		if err != nil {
			t.Errorf("Concurrent OpenConnection() failed: %v", err)
		}
	}
}

// TestConcurrent_OpenStreams 测试并发打开流
func TestConcurrent_OpenStreams(t *testing.T) {
	limits := DefaultLimitConfig()
	limits.System.Streams = 100

	rm, err := NewResourceManager(limits)
	if err != nil {
		t.Fatalf("NewResourceManager() failed: %v", err)
	}
	defer rm.Close()

	peerID := types.PeerID("QmPeerConcurrent")

	numGoroutines := 50
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	errors := make(chan error, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()

			streamScope, err := rm.OpenStream(peerID, pkgif.DirOutbound)
			if err != nil {
				errors <- err
				return
			}
			defer streamScope.Done()

			// 模拟一些工作
			stat := streamScope.Stat()
			if stat.NumStreamsOutbound < 1 {
				errors <- err
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	// 检查错误
	for err := range errors {
		if err != nil {
			t.Errorf("Concurrent OpenStream() failed: %v", err)
		}
	}
}

// TestConcurrent_ReserveMemory 测试并发预留内存
func TestConcurrent_ReserveMemory(t *testing.T) {
	limits := DefaultLimitConfig()
	limits.System.Memory = 1 << 20 // 1MB

	rm, err := NewResourceManager(limits)
	if err != nil {
		t.Fatalf("NewResourceManager() failed: %v", err)
	}
	defer rm.Close()

	numGoroutines := 50
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	errors := make(chan error, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()

			err := rm.ViewSystem(func(s pkgif.ResourceScope) error {
				span, err := s.BeginSpan()
				if err != nil {
					return err
				}
				defer span.Done()

				// 每个 goroutine 预留 1KB
				err = span.ReserveMemory(1024, pkgif.ReservationPriorityAlways)
				if err != nil {
					return err
				}

				// 然后释放
				span.ReleaseMemory(1024)
				return nil
			})

			if err != nil {
				errors <- err
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	// 检查错误
	for err := range errors {
		if err != nil {
			t.Errorf("Concurrent ReserveMemory() failed: %v", err)
		}
	}
}

// TestConcurrent_RaceDetection 测试竞态条件
// 运行 go test -race 时检测竞态
func TestConcurrent_RaceDetection(t *testing.T) {
	limits := DefaultLimitConfig()
	rm, err := NewResourceManager(limits)
	if err != nil {
		t.Fatalf("NewResourceManager() failed: %v", err)
	}
	defer rm.Close()

	addr := mustMultiaddr("/ip4/127.0.0.1/tcp/4001")
	peerID := types.PeerID("QmRacePeer")

	numOps := 20
	var wg sync.WaitGroup
	wg.Add(numOps * 3) // 连接 + 流 + 内存操作

	// 并发打开连接
	for i := 0; i < numOps; i++ {
		go func() {
			defer wg.Done()
			connScope, err := rm.OpenConnection(pkgif.DirInbound, false, addr)
			if err != nil {
				return
			}
			defer connScope.Done()
		}()
	}

	// 并发打开流
	for i := 0; i < numOps; i++ {
		go func() {
			defer wg.Done()
			streamScope, err := rm.OpenStream(peerID, pkgif.DirOutbound)
			if err != nil {
				return
			}
			defer streamScope.Done()
		}()
	}

	// 并发预留内存
	for i := 0; i < numOps; i++ {
		go func() {
			defer wg.Done()
			_ = rm.ViewSystem(func(s pkgif.ResourceScope) error {
				span, err := s.BeginSpan()
				if err != nil {
					return err
				}
				defer span.Done()

				_ = span.ReserveMemory(100, pkgif.ReservationPriorityAlways)
				span.ReleaseMemory(100)
				return nil
			})
		}()
	}

	wg.Wait()
}
