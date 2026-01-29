package resourcemgr

import (
	"sync"
	"sync/atomic"

	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
)

// resourceScope 资源作用域基础实现
type resourceScope struct {
	limit *pkgif.Limit // 资源限制

	closed atomic.Bool // 关闭状态

	refCnt atomic.Int32 // 引用计数

	// 资源计数器（使用 atomic 保证并发安全）
	nstreamsIn  atomic.Int32 // 入站流数
	nstreamsOut atomic.Int32 // 出站流数
	nconnsIn    atomic.Int32 // 入站连接数
	nconnsOut   atomic.Int32 // 出站连接数
	nfd         atomic.Int32 // 文件描述符数
	memory      atomic.Int64 // 内存使用量

	mu    sync.Mutex              // 保护 spans
	spans []*resourceScopeSpan    // Span 列表
}

// newResourceScope 创建资源作用域
func newResourceScope(limit *pkgif.Limit) *resourceScope {
	return &resourceScope{
		limit: limit,
	}
}

// Stat 返回当前资源使用统计
func (s *resourceScope) Stat() pkgif.ScopeStat {
	return pkgif.ScopeStat{
		NumStreamsInbound:  int(s.nstreamsIn.Load()),
		NumStreamsOutbound: int(s.nstreamsOut.Load()),
		NumConnsInbound:    int(s.nconnsIn.Load()),
		NumConnsOutbound:   int(s.nconnsOut.Load()),
		NumFD:              int(s.nfd.Load()),
		Memory:             s.memory.Load(),
	}
}

// BeginSpan 创建一个临时作用域
func (s *resourceScope) BeginSpan() (pkgif.ResourceScopeSpan, error) {
	if s.closed.Load() {
		return nil, ErrResourceScopeClosed
	}

	span := &resourceScopeSpan{
		resourceScope: s,
	}

	s.mu.Lock()
	s.spans = append(s.spans, span)
	s.mu.Unlock()

	return span, nil
}

// Done 标记作用域完成（基础实现，子类可覆盖）
func (s *resourceScope) Done() {
	// 基础作用域不需要做什么
}

// reserveStreams 预留流资源
func (s *resourceScope) reserveStreams(n, nIn, nOut int) error {
	if s.closed.Load() {
		return ErrResourceScopeClosed
	}

	// 检查总流数限制
	current := int(s.nstreamsIn.Load() + s.nstreamsOut.Load())
	if err := checkLimit(current+n, s.limit.Streams); err != nil {
		return err
	}

	// 检查入站流限制
	if nIn > 0 {
		current := int(s.nstreamsIn.Load())
		if err := checkLimit(current+nIn, s.limit.StreamsInbound); err != nil {
			return err
		}
	}

	// 检查出站流限制
	if nOut > 0 {
		current := int(s.nstreamsOut.Load())
		if err := checkLimit(current+nOut, s.limit.StreamsOutbound); err != nil {
			return err
		}
	}

	// 预留成功，增加计数
	if nIn > 0 {
		s.nstreamsIn.Add(int32(nIn))
	}
	if nOut > 0 {
		s.nstreamsOut.Add(int32(nOut))
	}

	return nil
}

// releaseStreams 释放流资源
func (s *resourceScope) releaseStreams(_ int, nIn, nOut int) {
	if nIn > 0 {
		s.nstreamsIn.Add(-int32(nIn))
	}
	if nOut > 0 {
		s.nstreamsOut.Add(-int32(nOut))
	}
}

// reserveConns 预留连接资源
func (s *resourceScope) reserveConns(n, nIn, nOut, nfd int) error {
	if s.closed.Load() {
		return ErrResourceScopeClosed
	}

	// 检查总连接数限制
	current := int(s.nconnsIn.Load() + s.nconnsOut.Load())
	if err := checkLimit(current+n, s.limit.Conns); err != nil {
		return err
	}

	// 检查入站连接限制
	if nIn > 0 {
		current := int(s.nconnsIn.Load())
		if err := checkLimit(current+nIn, s.limit.ConnsInbound); err != nil {
			return err
		}
	}

	// 检查出站连接限制
	if nOut > 0 {
		current := int(s.nconnsOut.Load())
		if err := checkLimit(current+nOut, s.limit.ConnsOutbound); err != nil {
			return err
		}
	}

	// 检查文件描述符限制
	if nfd > 0 {
		current := int(s.nfd.Load())
		if err := checkLimit(current+nfd, s.limit.FD); err != nil {
			return err
		}
	}

	// 预留成功，增加计数
	if nIn > 0 {
		s.nconnsIn.Add(int32(nIn))
	}
	if nOut > 0 {
		s.nconnsOut.Add(int32(nOut))
	}
	if nfd > 0 {
		s.nfd.Add(int32(nfd))
	}

	return nil
}

// releaseConns 释放连接资源
func (s *resourceScope) releaseConns(_ int, nIn, nOut, nfd int) {
	if nIn > 0 {
		s.nconnsIn.Add(-int32(nIn))
	}
	if nOut > 0 {
		s.nconnsOut.Add(-int32(nOut))
	}
	if nfd > 0 {
		s.nfd.Add(-int32(nfd))
	}
}

// reserveMemory 预留内存
func (s *resourceScope) reserveMemory(size int, prio uint8) error {
	if s.closed.Load() {
		return ErrResourceScopeClosed
	}

	if size < 0 {
		return nil // 忽略负值
	}

	current := s.memory.Load()
	return checkMemoryLimit(current, int64(size), s.limit.Memory, prio)
}

// addMemory 添加内存（预留成功后调用）
func (s *resourceScope) addMemory(size int) {
	if size > 0 {
		s.memory.Add(int64(size))
	}
}

// releaseMemory 释放内存
func (s *resourceScope) releaseMemory(size int) {
	if size > 0 {
		s.memory.Add(-int64(size))
	}
}

// IncRef 增加引用计数
func (s *resourceScope) IncRef() {
	s.refCnt.Add(1)
}

// DecRef 减少引用计数
func (s *resourceScope) DecRef() {
	s.refCnt.Add(-1)
}

// ============================================================================
// resourceScopeSpan - 临时作用域实现
// ============================================================================

// resourceScopeSpan 临时资源作用域
type resourceScopeSpan struct {
	*resourceScope // 继承基础作用域

	done   sync.Once   // 确保 Done() 只执行一次
	closed atomic.Bool // 关闭状态

	// Span 自己的资源计数
	spanStreamsIn  atomic.Int32
	spanStreamsOut atomic.Int32
	spanConnsIn    atomic.Int32
	spanConnsOut   atomic.Int32
	spanFD         atomic.Int32
	spanMemory     atomic.Int64
}

// ReserveMemory 预留内存
func (s *resourceScopeSpan) ReserveMemory(size int, prio uint8) error {
	if s.closed.Load() {
		return ErrResourceScopeClosed
	}

	if size <= 0 {
		return nil
	}

	// 检查父作用域限制
	current := s.memory.Load()
	limit := s.limit.Memory
	
	// 检查是否超限
	if err := checkMemoryLimit(current, int64(size), limit, prio); err != nil {
		return err
	}

	// 预留成功，增加计数
	s.memory.Add(int64(size))
	s.spanMemory.Add(int64(size))

	return nil
}

// ReleaseMemory 释放内存
func (s *resourceScopeSpan) ReleaseMemory(size int) {
	if size <= 0 {
		return
	}

	// 减少计数（不能低于 0）
	for {
		current := s.memory.Load()
		newVal := current - int64(size)
		if newVal < 0 {
			newVal = 0
		}
		if s.memory.CompareAndSwap(current, newVal) {
			break
		}
	}

	for {
		current := s.spanMemory.Load()
		newVal := current - int64(size)
		if newVal < 0 {
			newVal = 0
		}
		if s.spanMemory.CompareAndSwap(current, newVal) {
			break
		}
	}
}

// Done 结束 Span 并释放所有资源
func (s *resourceScopeSpan) Done() {
	s.done.Do(func() {
		s.closed.Store(true)

		// 释放所有 Span 持有的资源
		mem := s.spanMemory.Load()
		if mem > 0 {
			for {
				current := s.memory.Load()
				newVal := current - mem
				if newVal < 0 {
					newVal = 0
				}
				if s.memory.CompareAndSwap(current, newVal) {
					break
				}
			}
		}

		if nIn := s.spanStreamsIn.Load(); nIn > 0 {
			s.releaseStreams(0, int(nIn), 0)
		}
		if nOut := s.spanStreamsOut.Load(); nOut > 0 {
			s.releaseStreams(0, 0, int(nOut))
		}

		if nIn := s.spanConnsIn.Load(); nIn > 0 {
			s.releaseConns(0, int(nIn), 0, 0)
		}
		if nOut := s.spanConnsOut.Load(); nOut > 0 {
			s.releaseConns(0, 0, int(nOut), 0)
		}
		if nfd := s.spanFD.Load(); nfd > 0 {
			s.releaseConns(0, 0, 0, int(nfd))
		}

		// 从父作用域的 spans 列表中移除
		s.mu.Lock()
		spans := s.spans
		for i, span := range spans {
			if span == s {
				// 删除该 span
				s.spans = append(spans[:i], spans[i+1:]...)
				break
			}
		}
		s.mu.Unlock()
	})
}
