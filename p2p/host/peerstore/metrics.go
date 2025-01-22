package peerstore

import (
	"sync"
	"time"

	"github.com/dep2p/go-dep2p/core/peer"
)

// LatencyEWMASmoothing 控制EWMA的衰减(变化速度)
// 这必须是一个归一化的(0-1)值
// 1表示100%变化,0表示无变化
var LatencyEWMASmoothing = 0.1

// metrics 指标结构体
type metrics struct {
	mutex  sync.RWMutex              // 读写锁
	latmap map[peer.ID]time.Duration // 延迟映射表
}

// NewMetrics 创建新的指标实例
// 返回:
//   - *metrics: 指标实例
func NewMetrics() *metrics {
	return &metrics{
		latmap: make(map[peer.ID]time.Duration), // 初始化延迟映射表
	}
}

// RecordLatency 记录新的延迟测量值
// 参数:
//   - p: 对等节点ID
//   - next: 新的延迟值
func (m *metrics) RecordLatency(p peer.ID, next time.Duration) {
	nextf := float64(next)
	s := LatencyEWMASmoothing
	if s > 1 || s < 0 {
		s = 0.1 // 忽略这个参数,它已损坏
	}

	m.mutex.Lock()
	ewma, found := m.latmap[p]
	ewmaf := float64(ewma)
	if !found {
		m.latmap[p] = next // 没有数据时,直接作为平均值
	} else {
		nextf = ((1.0 - s) * ewmaf) + (s * nextf)
		m.latmap[p] = time.Duration(nextf)
	}
	m.mutex.Unlock()
}

// LatencyEWMA 返回对等节点延迟的指数加权移动平均值
// 参数:
//   - p: 对等节点ID
//
// 返回:
//   - time.Duration: 延迟的EWMA值
func (m *metrics) LatencyEWMA(p peer.ID) time.Duration {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return m.latmap[p]
}

// RemovePeer 移除对等节点的指标数据
// 参数:
//   - p: 对等节点ID
func (m *metrics) RemovePeer(p peer.ID) {
	m.mutex.Lock()
	delete(m.latmap, p)
	m.mutex.Unlock()
}
