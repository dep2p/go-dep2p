package resourcemgr

import (
	"testing"

	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
)

// ============================================================================
// 作用域测试
// ============================================================================

// TestScope_ReserveMemory 测试内存预留
func TestScope_ReserveMemory(t *testing.T) {
	limits := DefaultLimitConfig()
	rm, err := NewResourceManager(limits)
	if err != nil {
		t.Fatalf("NewResourceManager() failed: %v", err)
	}
	defer rm.Close()

	err = rm.ViewSystem(func(s pkgif.ResourceScope) error {
		// 检查系统作用域的初始状态
		sysStat := s.Stat()
		t.Logf("System scope initial: Memory=%d", sysStat.Memory)

		// 检查限制
		if rs, ok := s.(*systemScope); ok {
			t.Logf("System limit: Memory=%d", rs.limit.Memory)
		}

		span, err := s.BeginSpan()
		if err != nil {
			t.Fatalf("BeginSpan() failed: %v", err)
		}
		defer span.Done()

		// 预留 1KB 内存
		err = span.ReserveMemory(1024, pkgif.ReservationPriorityAlways)
		if err != nil {
			t.Fatalf("ReserveMemory() failed: %v", err)
		}

		stat := span.Stat()
		t.Logf("Span stat after reserve: Memory=%d", stat.Memory)
		if stat.Memory != 1024 {
			t.Errorf("Memory = %d, want 1024", stat.Memory)
		}

		return nil
	})
	if err != nil {
		t.Fatalf("ViewSystem() failed: %v", err)
	}
}

// TestScope_ReleaseMemory 测试内存释放
func TestScope_ReleaseMemory(t *testing.T) {
	limits := DefaultLimitConfig()
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

		// 预留然后释放
		err = span.ReserveMemory(2048, pkgif.ReservationPriorityAlways)
		if err != nil {
			t.Errorf("ReserveMemory() failed: %v", err)
		}

		span.ReleaseMemory(2048)

		stat := span.Stat()
		if stat.Memory != 0 {
			t.Errorf("Memory after release = %d, want 0", stat.Memory)
		}

		return nil
	})
	if err != nil {
		t.Fatalf("ViewSystem() failed: %v", err)
	}
}

// TestScope_ReserveMemoryPriority 测试内存预留优先级
func TestScope_ReserveMemoryPriority(t *testing.T) {
	// 创建限制很小的资源管理器
	limits := DefaultLimitConfig()
	limits.System.Memory = 1000 // 仅 1000 字节

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

		// 先预留 500 字节（50% 利用率）
		err = span.ReserveMemory(500, pkgif.ReservationPriorityAlways)
		if err != nil {
			t.Fatalf("ReserveMemory(500) failed: %v", err)
		}

		// 低优先级预留 100 字节应该失败
		// 阈值 = 1000 * 102 / 256 ≈ 398
		// 当前 500 + 100 = 600 > 398，应该失败
		err = span.ReserveMemory(100, pkgif.ReservationPriorityLow)
		if err == nil {
			t.Error("ReserveMemory() with low priority should have failed (exceeds 40% threshold)")
		}

		// 中优先级预留 100 字节应该成功
		// 阈值 = 1000 * 153 / 256 ≈ 597
		// 当前 500 + 100 = 600 > 597，应该失败
		err = span.ReserveMemory(100, pkgif.ReservationPriorityMedium)
		if err == nil {
			t.Error("ReserveMemory() with medium priority should have failed (exceeds 60% threshold)")
		}

		// 高优先级预留 100 字节应该成功
		// 阈值 = 1000 * 204 / 256 ≈ 796
		// 当前 500 + 100 = 600 < 796，应该成功
		err = span.ReserveMemory(100, pkgif.ReservationPriorityHigh)
		if err != nil {
			t.Errorf("ReserveMemory() with high priority failed: %v", err)
		}

		// Always 优先级应该可以预留到限制
		err = span.ReserveMemory(400, pkgif.ReservationPriorityAlways)
		if err != nil {
			t.Errorf("ReserveMemory() with always priority failed: %v", err)
		}

		return nil
	})
	if err != nil {
		t.Fatalf("ViewSystem() failed: %v", err)
	}
}

// TestScope_Stat 测试作用域统计
func TestScope_Stat(t *testing.T) {
	limits := DefaultLimitConfig()
	rm, err := NewResourceManager(limits)
	if err != nil {
		t.Fatalf("NewResourceManager() failed: %v", err)
	}
	defer rm.Close()

	err = rm.ViewSystem(func(s pkgif.ResourceScope) error {
		stat := s.Stat()

		if stat.NumConnsInbound != 0 {
			t.Errorf("NumConnsInbound = %d, want 0", stat.NumConnsInbound)
		}
		if stat.NumConnsOutbound != 0 {
			t.Errorf("NumConnsOutbound = %d, want 0", stat.NumConnsOutbound)
		}
		if stat.NumStreamsInbound != 0 {
			t.Errorf("NumStreamsInbound = %d, want 0", stat.NumStreamsInbound)
		}
		if stat.NumStreamsOutbound != 0 {
			t.Errorf("NumStreamsOutbound = %d, want 0", stat.NumStreamsOutbound)
		}
		if stat.NumFD != 0 {
			t.Errorf("NumFD = %d, want 0", stat.NumFD)
		}
		if stat.Memory != 0 {
			t.Errorf("Memory = %d, want 0", stat.Memory)
		}

		return nil
	})
	if err != nil {
		t.Fatalf("ViewSystem() failed: %v", err)
	}
}

// TestScope_BeginSpan 测试创建 Span
func TestScope_BeginSpan(t *testing.T) {
	limits := DefaultLimitConfig()
	rm, err := NewResourceManager(limits)
	if err != nil {
		t.Fatalf("NewResourceManager() failed: %v", err)
	}
	defer rm.Close()

	err = rm.ViewSystem(func(s pkgif.ResourceScope) error {
		span, err := s.BeginSpan()
		if err != nil {
			t.Fatalf("BeginSpan() failed: %v", err)
		}

		if span == nil {
			t.Fatal("BeginSpan() returned nil")
		}

		span.Done()
		return nil
	})
	if err != nil {
		t.Fatalf("ViewSystem() failed: %v", err)
	}
}

// TestScope_DoneMultipleTimes 测试多次调用 Done()
func TestScope_DoneMultipleTimes(t *testing.T) {
	limits := DefaultLimitConfig()
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

		// 第一次 Done() 应该成功
		span.Done()

		// 第二次 Done() 应该是幂等的（不应该 panic）
		span.Done()

		return nil
	})
	if err != nil {
		t.Fatalf("ViewSystem() failed: %v", err)
	}
}
