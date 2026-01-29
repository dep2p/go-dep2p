package coordinator

import (
	"sort"
	"sync"
	"time"

	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              发现策略
// ============================================================================

// DiscoveryStrategy 发现策略接口
type DiscoveryStrategy interface {
	// SelectDiscoveries 选择要使用的发现器
	SelectDiscoveries(discoveries map[string]pkgif.Discovery, ns string) []pkgif.Discovery

	// MergeResults 合并多个发现器的结果
	MergeResults(results []<-chan types.PeerInfo, limit int) <-chan types.PeerInfo
}

// ============================================================================
//                              并行策略
// ============================================================================

// ParallelStrategy 并行发现策略
//
// 同时查询所有发现器，合并并去重结果
type ParallelStrategy struct {
	timeout time.Duration
}

// NewParallelStrategy 创建并行策略
func NewParallelStrategy(timeout time.Duration) *ParallelStrategy {
	return &ParallelStrategy{
		timeout: timeout,
	}
}

// SelectDiscoveries 选择所有发现器
func (s *ParallelStrategy) SelectDiscoveries(discoveries map[string]pkgif.Discovery, _ string) []pkgif.Discovery {
	result := make([]pkgif.Discovery, 0, len(discoveries))
	for _, d := range discoveries {
		result = append(result, d)
	}
	return result
}

// MergeResults 合并结果（并行，去重）
func (s *ParallelStrategy) MergeResults(results []<-chan types.PeerInfo, limit int) <-chan types.PeerInfo {
	out := make(chan types.PeerInfo, limit)

	go func() {
		defer close(out)

		seen := make(map[types.PeerID]bool)
		count := 0

		// 使用 select 从所有通道读取
		cases := make([]any, len(results))
		for i, ch := range results {
			cases[i] = ch
		}

		// 简单实现：轮询所有通道
		for len(results) > 0 {
			for i := len(results) - 1; i >= 0; i-- {
				select {
				case peer, ok := <-results[i]:
					if !ok {
						// 通道关闭，移除
						results = append(results[:i], results[i+1:]...)
						continue
					}

					// 去重
					if !seen[peer.ID] {
						seen[peer.ID] = true
						out <- peer
						count++
						if limit > 0 && count >= limit {
							return
						}
					}
				default:
					// 非阻塞，继续下一个通道
				}
			}

			// 短暂休眠避免 CPU 占用过高
			time.Sleep(time.Millisecond)
		}
	}()

	return out
}

// ============================================================================
//                              优先级策略
// ============================================================================

// PriorityStrategy 优先级发现策略
//
// 按优先级顺序查询发现器，先快后慢
type PriorityStrategy struct {
	priorities map[string]int // 发现器名称 -> 优先级（数字越小优先级越高）
	timeout    time.Duration
}

// NewPriorityStrategy 创建优先级策略
func NewPriorityStrategy(priorities map[string]int, timeout time.Duration) *PriorityStrategy {
	if priorities == nil {
		priorities = map[string]int{
			"mdns":       1, // 局域网最快
			"rendezvous": 2, // 命名空间精确
			"dht":        3, // 全网广泛但慢
			"bootstrap":  4, // 引导节点
			"dns":        5, // DNS 解析
		}
	}

	return &PriorityStrategy{
		priorities: priorities,
		timeout:    timeout,
	}
}

// SelectDiscoveries 按优先级排序选择发现器
func (s *PriorityStrategy) SelectDiscoveries(discoveries map[string]pkgif.Discovery, _ string) []pkgif.Discovery {
	type namedDiscovery struct {
		name      string
		discovery pkgif.Discovery
		priority  int
	}

	named := make([]namedDiscovery, 0, len(discoveries))
	for name, d := range discoveries {
		priority, exists := s.priorities[name]
		if !exists {
			priority = 999 // 未知的发现器优先级最低
		}
		named = append(named, namedDiscovery{
			name:      name,
			discovery: d,
			priority:  priority,
		})
	}

	// 按优先级排序
	sort.Slice(named, func(i, j int) bool {
		return named[i].priority < named[j].priority
	})

	// 提取发现器
	result := make([]pkgif.Discovery, len(named))
	for i, n := range named {
		result[i] = n.discovery
	}

	return result
}

// MergeResults 合并结果（按顺序，去重）
func (s *PriorityStrategy) MergeResults(results []<-chan types.PeerInfo, limit int) <-chan types.PeerInfo {
	out := make(chan types.PeerInfo, limit)

	go func() {
		defer close(out)

		seen := make(map[types.PeerID]bool)
		count := 0

		// 按顺序处理每个结果通道
		for _, ch := range results {
			for peer := range ch {
				// 去重
				if !seen[peer.ID] {
					seen[peer.ID] = true
					out <- peer
					count++
					if limit > 0 && count >= limit {
						return
					}
				}
			}
		}
	}()

	return out
}

// ============================================================================
//                              混合策略
// ============================================================================

// HybridStrategy 混合发现策略
//
// 先查询高优先级的发现器，如果结果不足则并行查询所有发现器
type HybridStrategy struct {
	fastDiscoveries []string // 快速发现器名称（如 mdns）
	minResults      int      // 最少结果数，低于此数则使用所有发现器
	timeout         time.Duration
}

// NewHybridStrategy 创建混合策略
func NewHybridStrategy(fastDiscoveries []string, minResults int, timeout time.Duration) *HybridStrategy {
	if len(fastDiscoveries) == 0 {
		fastDiscoveries = []string{"mdns", "rendezvous"}
	}
	if minResults <= 0 {
		minResults = 3
	}

	return &HybridStrategy{
		fastDiscoveries: fastDiscoveries,
		minResults:      minResults,
		timeout:         timeout,
	}
}

// SelectDiscoveries 智能选择发现器
func (s *HybridStrategy) SelectDiscoveries(discoveries map[string]pkgif.Discovery, _ string) []pkgif.Discovery {
	// 先尝试快速发现器
	fast := make([]pkgif.Discovery, 0)
	for _, name := range s.fastDiscoveries {
		if d, exists := discoveries[name]; exists {
			fast = append(fast, d)
		}
	}

	if len(fast) > 0 {
		return fast
	}

	// 如果没有快速发现器，返回所有发现器
	all := make([]pkgif.Discovery, 0, len(discoveries))
	for _, d := range discoveries {
		all = append(all, d)
	}
	return all
}

// MergeResults 智能合并结果
func (s *HybridStrategy) MergeResults(results []<-chan types.PeerInfo, limit int) <-chan types.PeerInfo {
	out := make(chan types.PeerInfo, limit)

	go func() {
		defer close(out)

		seen := make(map[types.PeerID]bool)
		count := 0
		var mu sync.Mutex

		// 并行读取所有结果
		var wg sync.WaitGroup
		for _, ch := range results {
			wg.Add(1)
			go func(resultCh <-chan types.PeerInfo) {
				defer wg.Done()
				for peer := range resultCh {
					mu.Lock()
					if !seen[peer.ID] {
						seen[peer.ID] = true
						count++
						mu.Unlock()

						select {
						case out <- peer:
							if limit > 0 && count >= limit {
								return
							}
						default:
						}
					} else {
						mu.Unlock()
					}
				}
			}(ch)
		}

		wg.Wait()
	}()

	return out
}

// ============================================================================
//                              策略工厂
// ============================================================================

// NewDefaultStrategy 创建默认策略
func NewDefaultStrategy() DiscoveryStrategy {
	return NewParallelStrategy(30 * time.Second)
}

