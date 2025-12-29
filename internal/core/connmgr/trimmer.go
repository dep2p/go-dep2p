// Package connmgr 提供连接管理模块的实现
package connmgr

import (
	"sort"
	"time"

	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              裁剪
// ============================================================================

// TriggerTrim 触发裁剪
func (m *ConnectionManager) TriggerTrim() {
	select {
	case m.trimCh <- struct{}{}:
		log.Debug("裁剪已触发")
	default:
		// 已有裁剪任务在队列中
	}
}

// trimLoop 裁剪循环
func (m *ConnectionManager) trimLoop() {
	ticker := time.NewTicker(m.config.TrimInterval)
	defer ticker.Stop()

	for {
		select {
		case <-m.stopCh:
			return

		case <-ticker.C:
			m.checkAndTrim()

		case <-m.trimCh:
			m.checkAndTrim()
		}
	}
}

// checkAndTrim 检查并执行裁剪
func (m *ConnectionManager) checkAndTrim() {
	m.mu.RLock()
	connCount := len(m.peers)
	m.mu.RUnlock()

	// 只有超过高水位线才裁剪
	if connCount <= m.config.HighWater {
		return
	}

	// 计算需要裁剪的数量
	targetCount := m.config.LowWater
	trimCount := connCount - targetCount

	if trimCount <= 0 {
		return
	}

	log.Info("开始裁剪",
		"current", connCount,
		"target", targetCount,
		"toTrim", trimCount)

	start := time.Now()

	// 选择要裁剪的连接
	toTrim := m.selectToTrim(trimCount)

	// 获取回调函数（线程安全）
	m.callbackMu.RLock()
	closeCallback := m.closeCallback
	m.callbackMu.RUnlock()

	// 执行裁剪
	trimmed := 0
	for _, nodeID := range toTrim {
		if closeCallback != nil {
			if err := closeCallback(nodeID); err != nil {
				log.Debug("关闭连接失败",
					"peer", nodeID.ShortString(),
					"err", err)
				continue
			}
		}

		m.mu.Lock()
		delete(m.peers, nodeID)
		m.mu.Unlock()

		trimmed++
	}

	log.Info("裁剪完成",
		"trimmed", trimmed,
		"current", m.ConnCount(),
		"duration", time.Since(start))
}

// selectToTrim 选择要裁剪的连接
//
// 裁剪优先级（按设计文档）：
// 1. 空闲连接（长时间无数据传输）- 最先裁剪
// 2. 短期连接（连接时间短，关系不稳定）
// 3. 低活跃连接（数据传输量少）
// 4. 高延迟连接（RTT 较高）
// 5. 活跃连接（近期有数据传输）- 最后裁剪
// 6. 受保护连接 - 不裁剪
func (m *ConnectionManager) selectToTrim(count int) []types.NodeID {
	m.mu.RLock()
	defer m.mu.RUnlock()

	now := time.Now()
	gracePeriod := m.config.GracePeriod
	idleTimeout := m.config.IdleTimeout

	// 收集候选连接
	type candidate struct {
		nodeID     types.NodeID
		pruneScore float64 // 越高越优先被裁剪
	}

	candidates := make([]candidate, 0, len(m.peers))

	// 计算活跃度的参考值（用于归一化）
	var maxBytes uint64
	for _, peer := range m.peers {
		totalBytes := peer.bytesSent + peer.bytesRecv
		if totalBytes > maxBytes {
			maxBytes = totalBytes
		}
	}
	if maxBytes == 0 {
		maxBytes = 1 // 避免除零
	}

	for nodeID, peer := range m.peers {
		// 跳过受保护的连接
		if peer.protected {
			continue
		}

		// 跳过处于保护期的新连接
		if now.Sub(peer.createdAt) < gracePeriod {
			continue
		}

		// 计算裁剪分数（越高越优先被裁剪）
		// 基础公式：PruneScore = IdleScore + ShortTermScore + InactiveScore + LatencyScore - ActivityBonus
		var pruneScore float64

		// 1. 空闲分数（最高权重 100）：空闲时间越长，分数越高
		idleDuration := now.Sub(peer.lastActive)
		if idleDuration > idleTimeout {
			// 超过空闲阈值，高分裁剪
			pruneScore += 100.0
		} else {
			// 空闲时间比例（0-50 分）
			idleRatio := float64(idleDuration) / float64(idleTimeout)
			pruneScore += idleRatio * 50.0
		}

		// 2. 短期连接分数（权重 30）：连接时间越短，分数越高
		age := now.Sub(peer.createdAt)
		// 连接稳定期设定为 10 分钟，短于此的连接优先裁剪
		stableThreshold := 10 * time.Minute
		if age < stableThreshold {
			shortTermRatio := 1.0 - float64(age)/float64(stableThreshold)
			pruneScore += shortTermRatio * 30.0
		}

		// 3. 低活跃分数（权重 40）：传输数据越少，分数越高
		totalBytes := peer.bytesSent + peer.bytesRecv
		if totalBytes == 0 {
			pruneScore += 40.0
		} else {
			// 活跃度越低，分数越高
			activityRatio := float64(totalBytes) / float64(maxBytes)
			pruneScore += (1.0 - activityRatio) * 40.0
		}

		// 4. 高延迟分数（权重 20）：RTT 越高，分数越高
		if peer.rtt > 0 {
			// 假设 500ms 以上的 RTT 为高延迟
			highLatencyThreshold := 500 * time.Millisecond
			if peer.rtt > highLatencyThreshold {
				pruneScore += 20.0
			} else {
				latencyRatio := float64(peer.rtt) / float64(highLatencyThreshold)
				pruneScore += latencyRatio * 20.0
			}
		}

		// 5. 入站连接稍微优先裁剪（权重 5）
		if peer.connInfo.Direction == types.DirInbound {
			pruneScore += 5.0
		}

		candidates = append(candidates, candidate{
			nodeID:     nodeID,
			pruneScore: pruneScore,
		})
	}

	// 按裁剪分数降序排序（分数高的先被裁剪）
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].pruneScore > candidates[j].pruneScore
	})

	// 选择前 count 个
	result := make([]types.NodeID, 0, count)
	for i := 0; i < count && i < len(candidates); i++ {
		result = append(result, candidates[i].nodeID)
	}

	log.Debug("裁剪候选评分",
		"candidates", len(candidates),
		"selected", len(result))

	return result
}

