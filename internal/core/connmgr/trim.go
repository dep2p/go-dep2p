package connmgr

import (
	"context"
	"sort"
)

// peerScore 节点评分
type peerScore struct {
	peer  string
	score int
}

// trimToTarget 回收连接至目标数量
func (m *Manager) trimToTarget(ctx context.Context, target int) {
	// 获取当前连接
	conns := m.host.Connections()

	if len(conns) <= target {
		// 连接数低于目标，不需要回收
		return
	}

	// 收集可回收候选（排除受保护的）
	candidates := make([]peerScore, 0)
	seen := make(map[string]bool) // 去重

	for _, conn := range conns {
		peer := conn.RemotePeer()

		// 跳过已处理的节点
		if seen[peer] {
			continue
		}
		seen[peer] = true

		// 跳过受保护的节点
		if m.protects.HasAnyProtection(peer) {
			continue
		}

		// 计算分数
		candidates = append(candidates, peerScore{
			peer:  peer,
			score: m.calculateScore(peer),
		})
	}

	// 按分数排序（升序，低分优先关闭）
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].score < candidates[j].score
	})

	// 计算需要关闭的连接数
	toClose := len(conns) - target
	if toClose > len(candidates) {
		toClose = len(candidates)
	}

	// 关闭低分连接
	for i := 0; i < toClose; i++ {
		select {
		case <-ctx.Done():
			// 上下文取消，停止回收
			return
		default:
			// 关闭连接
			_ = m.host.CloseConnection(candidates[i].peer)
		}
	}
}
