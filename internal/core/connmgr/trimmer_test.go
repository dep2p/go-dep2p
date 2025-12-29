package connmgr

import (
	"context"
	"sync"
	"testing"
	"time"

	connmgrif "github.com/dep2p/go-dep2p/pkg/interfaces/connmgr"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                    GAP-015: GracePeriod 行为测试
// ============================================================================

// TestSelectToTrim_GracePeriod 验证新连接在 GracePeriod 内不被裁剪
//
// 设计要求（design/requirements/REQ-CONN-001.md）：
// 新连接在 GracePeriod 内不应被裁剪
func TestSelectToTrim_GracePeriod(t *testing.T) {
	config := connmgrif.DefaultConfig()
	config.GracePeriod = 1 * time.Minute
	config.LowWater = 2
	config.HighWater = 5

	manager := NewConnectionManager(config)

	// 添加一个新连接（在 GracePeriod 内）
	newPeerID := types.NodeID{1, 2, 3, 4}
	manager.mu.Lock()
	manager.peers[newPeerID] = &peerInfo{
		nodeID: newPeerID,
		connInfo: connmgrif.ConnectionInfo{
			NodeID:    newPeerID,
			Direction: types.DirInbound,
		},
		createdAt:  time.Now(), // 刚创建
		lastActive: time.Now(),
		protected:  false,
	}
	manager.mu.Unlock()

	// 添加一个旧连接（超过 GracePeriod）
	oldPeerID := types.NodeID{5, 6, 7, 8}
	manager.mu.Lock()
	manager.peers[oldPeerID] = &peerInfo{
		nodeID: oldPeerID,
		connInfo: connmgrif.ConnectionInfo{
			NodeID:    oldPeerID,
			Direction: types.DirInbound,
		},
		createdAt:  time.Now().Add(-2 * time.Minute), // 2 分钟前创建
		lastActive: time.Now().Add(-2 * time.Minute),
		protected:  false,
	}
	manager.mu.Unlock()

	// 选择要裁剪的连接（只需要裁剪 1 个）
	toTrim := manager.selectToTrim(1)

	// 验证只选择了旧连接
	if len(toTrim) != 1 {
		t.Errorf("Expected 1 connection to trim, got %d", len(toTrim))
		return
	}

	if toTrim[0] != oldPeerID {
		t.Errorf("Expected old peer %v to be trimmed, got %v", oldPeerID, toTrim[0])
	}

	// 验证新连接没有被选中（在 GracePeriod 内）
	for _, id := range toTrim {
		if id == newPeerID {
			t.Error("New peer within GracePeriod should NOT be selected for trimming")
		}
	}

	t.Log("GracePeriod protection verified: new connections are protected from trimming")
}

// TestSelectToTrim_ProtectedConnections 验证受保护的连接不被裁剪
func TestSelectToTrim_ProtectedConnections(t *testing.T) {
	config := connmgrif.DefaultConfig()
	config.GracePeriod = 0 // 禁用 GracePeriod
	config.LowWater = 1
	config.HighWater = 5

	manager := NewConnectionManager(config)

	// 添加一个受保护的连接
	protectedPeerID := types.NodeID{1, 2, 3, 4}
	manager.mu.Lock()
	manager.peers[protectedPeerID] = &peerInfo{
		nodeID: protectedPeerID,
		connInfo: connmgrif.ConnectionInfo{
			NodeID:    protectedPeerID,
			Direction: types.DirInbound,
		},
		createdAt:  time.Now().Add(-10 * time.Minute),
		lastActive: time.Now().Add(-10 * time.Minute),
		protected:  true, // 受保护
	}
	manager.mu.Unlock()

	// 添加一个不受保护的连接
	unprotectedPeerID := types.NodeID{5, 6, 7, 8}
	manager.mu.Lock()
	manager.peers[unprotectedPeerID] = &peerInfo{
		nodeID: unprotectedPeerID,
		connInfo: connmgrif.ConnectionInfo{
			NodeID:    unprotectedPeerID,
			Direction: types.DirInbound,
		},
		createdAt:  time.Now().Add(-10 * time.Minute),
		lastActive: time.Now().Add(-10 * time.Minute),
		protected:  false,
	}
	manager.mu.Unlock()

	// 选择要裁剪的连接
	toTrim := manager.selectToTrim(1)

	// 验证只选择了不受保护的连接
	for _, id := range toTrim {
		if id == protectedPeerID {
			t.Error("Protected connection should NOT be selected for trimming")
		}
	}

	t.Log("Protected connection verified: protected peers are not trimmed")
}

// TestSelectToTrim_IdleConnectionsPriority 验证空闲连接优先被裁剪
func TestSelectToTrim_IdleConnectionsPriority(t *testing.T) {
	config := connmgrif.DefaultConfig()
	config.GracePeriod = 0
	config.IdleTimeout = 5 * time.Minute
	config.LowWater = 1
	config.HighWater = 5

	manager := NewConnectionManager(config)

	// 添加一个活跃的连接
	activePeerID := types.NodeID{1, 2, 3, 4}
	manager.mu.Lock()
	manager.peers[activePeerID] = &peerInfo{
		nodeID: activePeerID,
		connInfo: connmgrif.ConnectionInfo{
			NodeID:    activePeerID,
			Direction: types.DirInbound,
		},
		createdAt:  time.Now().Add(-10 * time.Minute),
		lastActive: time.Now(), // 刚活跃
		protected:  false,
	}
	manager.mu.Unlock()

	// 添加一个空闲的连接
	idlePeerID := types.NodeID{5, 6, 7, 8}
	manager.mu.Lock()
	manager.peers[idlePeerID] = &peerInfo{
		nodeID: idlePeerID,
		connInfo: connmgrif.ConnectionInfo{
			NodeID:    idlePeerID,
			Direction: types.DirInbound,
		},
		createdAt:  time.Now().Add(-10 * time.Minute),
		lastActive: time.Now().Add(-10 * time.Minute), // 10 分钟没活动
		protected:  false,
	}
	manager.mu.Unlock()

	// 选择要裁剪的连接（只裁剪 1 个）
	toTrim := manager.selectToTrim(1)

	// 验证空闲连接被优先选择
	if len(toTrim) != 1 {
		t.Errorf("Expected 1 connection to trim, got %d", len(toTrim))
		return
	}

	if toTrim[0] != idlePeerID {
		t.Errorf("Expected idle peer to be trimmed first")
	}

	t.Log("Idle connection priority verified: idle connections are trimmed first")
}

// TestConnMgrDefaultConfig 验证默认配置
func TestConnMgrDefaultConfig(t *testing.T) {
	config := connmgrif.DefaultConfig()

	if config.GracePeriod <= 0 {
		t.Error("GracePeriod should be positive")
	}

	if config.IdleTimeout <= 0 {
		t.Error("IdleTimeout should be positive")
	}

	if config.LowWater <= 0 {
		t.Error("LowWater should be positive")
	}

	if config.HighWater <= config.LowWater {
		t.Error("HighWater should be greater than LowWater")
	}

	t.Logf("Default config: GracePeriod=%v, IdleTimeout=%v, LowWater=%d, HighWater=%d",
		config.GracePeriod, config.IdleTimeout, config.LowWater, config.HighWater)
}

// ============================================================================
//                              新增测试 - 审查修复验证
// ============================================================================

// TestConnectionManager_CallbacksThreadSafe 测试回调设置的线程安全性
func TestConnectionManager_CallbacksThreadSafe(t *testing.T) {
	config := connmgrif.DefaultConfig()
	manager := NewConnectionManager(config)

	var wg sync.WaitGroup
	const goroutines = 10
	const iterations = 100

	// 并发设置回调
	for i := 0; i < goroutines; i++ {
		wg.Add(2)

		go func(id int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				manager.SetCloseCallback(func(nodeID types.NodeID) error {
					return nil
				})
			}
		}(i)

		go func(id int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				manager.SetReconnectCallback(func(ctx context.Context, nodeID types.NodeID) error {
					return nil
				})
			}
		}(i)
	}

	// 等待所有 goroutine 完成，不应该发生 panic 或数据竞争
	wg.Wait()
}

// TestSelectToTrim_AgeOverflow 测试连接年龄计算不会整数溢出
func TestSelectToTrim_AgeOverflow(t *testing.T) {
	config := connmgrif.DefaultConfig()
	config.GracePeriod = 0 // 禁用保护期
	manager := NewConnectionManager(config)

	// 创建一个非常"老"的连接 (模拟)
	nodeID := types.NodeID{1, 2, 3, 4, 5, 6, 7, 8}
	manager.mu.Lock()
	manager.peers[nodeID] = &peerInfo{
		nodeID:     nodeID,
		tags:       make(map[string]struct{}),
		protected:  false,
		createdAt:  time.Now().Add(-365 * 24 * time.Hour * 100), // 100年前
		lastActive: time.Now().Add(-time.Hour),
	}
	manager.mu.Unlock()

	// 调用 selectToTrim 不应该 panic
	toTrim := manager.selectToTrim(1)

	// 应该返回这个连接
	if len(toTrim) != 1 {
		t.Errorf("selectToTrim 返回 %d 个，期望 1", len(toTrim))
	}
}

// ============================================================================
//                    多维度裁剪策略测试（GAP-C2）
// ============================================================================

// TestSelectToTrim_ShortTermPriority 验证短期连接优先被裁剪
func TestSelectToTrim_ShortTermPriority(t *testing.T) {
	config := connmgrif.DefaultConfig()
	config.GracePeriod = 0
	config.IdleTimeout = 30 * time.Minute // 设置较长的空闲超时
	manager := NewConnectionManager(config)

	// 添加一个长期稳定的连接（20分钟前创建）
	longTermPeerID := types.NodeID{1, 2, 3, 4}
	manager.mu.Lock()
	manager.peers[longTermPeerID] = &peerInfo{
		nodeID: longTermPeerID,
		connInfo: connmgrif.ConnectionInfo{
			NodeID:    longTermPeerID,
			Direction: types.DirInbound,
		},
		createdAt:  time.Now().Add(-20 * time.Minute),
		lastActive: time.Now().Add(-1 * time.Minute), // 近期有活动
		protected:  false,
	}
	manager.mu.Unlock()

	// 添加一个短期连接（1分钟前创建）
	shortTermPeerID := types.NodeID{5, 6, 7, 8}
	manager.mu.Lock()
	manager.peers[shortTermPeerID] = &peerInfo{
		nodeID: shortTermPeerID,
		connInfo: connmgrif.ConnectionInfo{
			NodeID:    shortTermPeerID,
			Direction: types.DirInbound,
		},
		createdAt:  time.Now().Add(-1 * time.Minute),
		lastActive: time.Now().Add(-1 * time.Minute), // 同样的活跃时间
		protected:  false,
	}
	manager.mu.Unlock()

	// 选择要裁剪的连接（只裁剪 1 个）
	toTrim := manager.selectToTrim(1)

	// 验证短期连接被优先选择（因为短期连接分数更高）
	if len(toTrim) != 1 {
		t.Errorf("Expected 1 connection to trim, got %d", len(toTrim))
		return
	}

	if toTrim[0] != shortTermPeerID {
		t.Errorf("Expected short-term peer to be trimmed first, got %v", toTrim[0])
	}

	t.Log("Short-term connection priority verified")
}

// TestSelectToTrim_LowActivityPriority 验证低活跃连接优先被裁剪
func TestSelectToTrim_LowActivityPriority(t *testing.T) {
	config := connmgrif.DefaultConfig()
	config.GracePeriod = 0
	config.IdleTimeout = 30 * time.Minute
	manager := NewConnectionManager(config)

	// 添加一个高活跃的连接（传输了大量数据）
	highActivityPeerID := types.NodeID{1, 2, 3, 4}
	manager.mu.Lock()
	manager.peers[highActivityPeerID] = &peerInfo{
		nodeID: highActivityPeerID,
		connInfo: connmgrif.ConnectionInfo{
			NodeID:    highActivityPeerID,
			Direction: types.DirInbound,
		},
		createdAt:  time.Now().Add(-15 * time.Minute),
		lastActive: time.Now().Add(-1 * time.Minute),
		protected:  false,
		bytesSent:  1000000, // 1MB 数据发送
		bytesRecv:  1000000, // 1MB 数据接收
	}
	manager.mu.Unlock()

	// 添加一个低活跃的连接（几乎没有数据传输）
	lowActivityPeerID := types.NodeID{5, 6, 7, 8}
	manager.mu.Lock()
	manager.peers[lowActivityPeerID] = &peerInfo{
		nodeID: lowActivityPeerID,
		connInfo: connmgrif.ConnectionInfo{
			NodeID:    lowActivityPeerID,
			Direction: types.DirInbound,
		},
		createdAt:  time.Now().Add(-15 * time.Minute), // 同样的创建时间
		lastActive: time.Now().Add(-1 * time.Minute),  // 同样的活跃时间
		protected:  false,
		bytesSent:  100, // 很少的数据
		bytesRecv:  100,
	}
	manager.mu.Unlock()

	// 选择要裁剪的连接（只裁剪 1 个）
	toTrim := manager.selectToTrim(1)

	// 验证低活跃连接被优先选择
	if len(toTrim) != 1 {
		t.Errorf("Expected 1 connection to trim, got %d", len(toTrim))
		return
	}

	if toTrim[0] != lowActivityPeerID {
		t.Errorf("Expected low-activity peer to be trimmed first, got %v", toTrim[0])
	}

	t.Log("Low-activity connection priority verified")
}

// TestSelectToTrim_HighLatencyPriority 验证高延迟连接优先被裁剪
func TestSelectToTrim_HighLatencyPriority(t *testing.T) {
	config := connmgrif.DefaultConfig()
	config.GracePeriod = 0
	config.IdleTimeout = 30 * time.Minute
	manager := NewConnectionManager(config)

	// 添加一个低延迟的连接
	lowLatencyPeerID := types.NodeID{1, 2, 3, 4}
	manager.mu.Lock()
	manager.peers[lowLatencyPeerID] = &peerInfo{
		nodeID: lowLatencyPeerID,
		connInfo: connmgrif.ConnectionInfo{
			NodeID:    lowLatencyPeerID,
			Direction: types.DirInbound,
		},
		createdAt:  time.Now().Add(-15 * time.Minute),
		lastActive: time.Now().Add(-1 * time.Minute),
		protected:  false,
		bytesSent:  10000,
		bytesRecv:  10000,
		rtt:        50 * time.Millisecond, // 低延迟
	}
	manager.mu.Unlock()

	// 添加一个高延迟的连接
	highLatencyPeerID := types.NodeID{5, 6, 7, 8}
	manager.mu.Lock()
	manager.peers[highLatencyPeerID] = &peerInfo{
		nodeID: highLatencyPeerID,
		connInfo: connmgrif.ConnectionInfo{
			NodeID:    highLatencyPeerID,
			Direction: types.DirInbound,
		},
		createdAt:  time.Now().Add(-15 * time.Minute), // 同样的创建时间
		lastActive: time.Now().Add(-1 * time.Minute),  // 同样的活跃时间
		protected:  false,
		bytesSent:  10000, // 同样的活跃度
		bytesRecv:  10000,
		rtt:        800 * time.Millisecond, // 高延迟
	}
	manager.mu.Unlock()

	// 选择要裁剪的连接（只裁剪 1 个）
	toTrim := manager.selectToTrim(1)

	// 验证高延迟连接被优先选择
	if len(toTrim) != 1 {
		t.Errorf("Expected 1 connection to trim, got %d", len(toTrim))
		return
	}

	if toTrim[0] != highLatencyPeerID {
		t.Errorf("Expected high-latency peer to be trimmed first, got %v", toTrim[0])
	}

	t.Log("High-latency connection priority verified")
}

// TestUpdateStats 测试统计信息更新
func TestUpdateStats(t *testing.T) {
	config := connmgrif.DefaultConfig()
	manager := NewConnectionManager(config)

	nodeID := types.NodeID{1, 2, 3, 4}
	manager.mu.Lock()
	manager.peers[nodeID] = &peerInfo{
		nodeID:     nodeID,
		createdAt:  time.Now(),
		lastActive: time.Now().Add(-10 * time.Minute),
	}
	manager.mu.Unlock()

	// 更新统计信息
	manager.UpdateStats(nodeID, 1000, 2000, 100*time.Millisecond)

	manager.mu.RLock()
	peer := manager.peers[nodeID]
	manager.mu.RUnlock()

	if peer.bytesSent != 1000 {
		t.Errorf("Expected bytesSent=1000, got %d", peer.bytesSent)
	}
	if peer.bytesRecv != 2000 {
		t.Errorf("Expected bytesRecv=2000, got %d", peer.bytesRecv)
	}
	if peer.rtt != 100*time.Millisecond {
		t.Errorf("Expected rtt=100ms, got %v", peer.rtt)
	}
	// UpdateStats 应该同时更新 lastActive
	if time.Since(peer.lastActive) > time.Second {
		t.Error("UpdateStats should update lastActive")
	}

	t.Log("UpdateStats verified")
}

