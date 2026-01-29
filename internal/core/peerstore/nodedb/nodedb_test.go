package nodedb

import (
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	assert.Equal(t, 10000, config.MaxNodes)
	assert.Equal(t, 7*24*time.Hour, config.NodeExpiry)
	assert.Equal(t, 1*time.Hour, config.CleanupInterval)
	assert.Equal(t, 5, config.MaxFailedDials)
}

func TestNodeRecord_Clone(t *testing.T) {
	original := &NodeRecord{
		ID:          "peer1",
		IP:          net.ParseIP("192.168.1.1"),
		UDP:         4000,
		TCP:         4001,
		Addrs:       []string{"/ip4/192.168.1.1/tcp/4001"},
		LastPong:    time.Now().Add(-1 * time.Hour),
		LastSeen:    time.Now(),
		FailedDials: 2,
		LastDial:    time.Now().Add(-30 * time.Minute),
	}

	clone := original.Clone()

	// 验证值相等
	assert.Equal(t, original.ID, clone.ID)
	assert.Equal(t, original.UDP, clone.UDP)
	assert.Equal(t, original.TCP, clone.TCP)
	assert.Equal(t, original.LastPong, clone.LastPong)
	assert.Equal(t, original.LastSeen, clone.LastSeen)
	assert.Equal(t, original.FailedDials, clone.FailedDials)

	// 验证深拷贝
	clone.IP[0] = 10
	assert.NotEqual(t, original.IP[0], clone.IP[0])

	clone.Addrs[0] = "modified"
	assert.NotEqual(t, original.Addrs[0], clone.Addrs[0])
}

func TestNewMemoryDB(t *testing.T) {
	config := DefaultConfig()
	config.CleanupInterval = 1 * time.Hour // 避免测试期间清理

	db := NewMemoryDB(config)
	require.NotNil(t, db)
	defer db.Close()

	assert.Equal(t, config.MaxNodes, db.config.MaxNodes)
	assert.Equal(t, 0, db.Size())
}

func TestMemoryDB_UpdateNode(t *testing.T) {
	config := DefaultConfig()
	config.CleanupInterval = 1 * time.Hour

	db := NewMemoryDB(config)
	defer db.Close()

	// 插入节点
	node := &NodeRecord{
		ID:    "peer1",
		IP:    net.ParseIP("192.168.1.1"),
		TCP:   4001,
		Addrs: []string{"/ip4/192.168.1.1/tcp/4001"},
	}

	err := db.UpdateNode(node)
	require.NoError(t, err)
	assert.Equal(t, 1, db.Size())

	// 获取节点
	retrieved := db.GetNode("peer1")
	require.NotNil(t, retrieved)
	assert.Equal(t, "peer1", retrieved.ID)
	assert.Equal(t, 4001, retrieved.TCP)

	// 更新节点
	node.TCP = 4002
	err = db.UpdateNode(node)
	require.NoError(t, err)

	retrieved = db.GetNode("peer1")
	assert.Equal(t, 4002, retrieved.TCP)
}

func TestMemoryDB_UpdateNode_InvalidNode(t *testing.T) {
	config := DefaultConfig()
	config.CleanupInterval = 1 * time.Hour

	db := NewMemoryDB(config)
	defer db.Close()

	// nil 节点
	err := db.UpdateNode(nil)
	assert.Equal(t, ErrInvalidNode, err)

	// 空 ID
	err = db.UpdateNode(&NodeRecord{})
	assert.Equal(t, ErrInvalidNode, err)
}

func TestMemoryDB_RemoveNode(t *testing.T) {
	config := DefaultConfig()
	config.CleanupInterval = 1 * time.Hour

	db := NewMemoryDB(config)
	defer db.Close()

	// 插入节点
	err := db.UpdateNode(&NodeRecord{ID: "peer1"})
	require.NoError(t, err)
	assert.Equal(t, 1, db.Size())

	// 删除节点
	err = db.RemoveNode("peer1")
	require.NoError(t, err)
	assert.Equal(t, 0, db.Size())

	// 获取已删除节点
	retrieved := db.GetNode("peer1")
	assert.Nil(t, retrieved)
}

func TestMemoryDB_QuerySeeds(t *testing.T) {
	config := DefaultConfig()
	config.CleanupInterval = 1 * time.Hour

	db := NewMemoryDB(config)
	defer db.Close()

	now := time.Now()

	// 插入多个节点
	for i := 0; i < 10; i++ {
		err := db.UpdateNode(&NodeRecord{
			ID:       "peer" + string(rune('0'+i)),
			LastSeen: now.Add(-time.Duration(i) * time.Hour),
		})
		require.NoError(t, err)
	}

	// 查询种子节点
	seeds := db.QuerySeeds(5, 6*time.Hour)
	assert.Len(t, seeds, 5)

	// 验证排序（最近活跃的优先）
	for i := 0; i < len(seeds)-1; i++ {
		assert.True(t, seeds[i].LastSeen.After(seeds[i+1].LastSeen) ||
			seeds[i].LastSeen.Equal(seeds[i+1].LastSeen))
	}
}

func TestMemoryDB_QuerySeeds_FailedDials(t *testing.T) {
	config := DefaultConfig()
	config.CleanupInterval = 1 * time.Hour
	config.MaxFailedDials = 3

	db := NewMemoryDB(config)
	defer db.Close()

	// 插入节点，有些失败次数超过阈值
	err := db.UpdateNode(&NodeRecord{
		ID:          "peer1",
		LastSeen:    time.Now(),
		FailedDials: 0,
	})
	require.NoError(t, err)

	err = db.UpdateNode(&NodeRecord{
		ID:          "peer2",
		LastSeen:    time.Now(),
		FailedDials: 5, // 超过阈值
	})
	require.NoError(t, err)

	// 查询种子节点
	seeds := db.QuerySeeds(10, 1*time.Hour)
	assert.Len(t, seeds, 1)
	assert.Equal(t, "peer1", seeds[0].ID)
}

func TestMemoryDB_LastPong(t *testing.T) {
	config := DefaultConfig()
	config.CleanupInterval = 1 * time.Hour

	db := NewMemoryDB(config)
	defer db.Close()

	// 不存在的节点
	pongTime := db.LastPongReceived("peer1")
	assert.True(t, pongTime.IsZero())

	// 更新 Pong 时间
	now := time.Now()
	err := db.UpdateLastPong("peer1", now)
	require.NoError(t, err)

	// 获取 Pong 时间
	pongTime = db.LastPongReceived("peer1")
	assert.Equal(t, now.Unix(), pongTime.Unix())

	// 验证节点已创建
	node := db.GetNode("peer1")
	require.NotNil(t, node)
	assert.Equal(t, now.Unix(), node.LastSeen.Unix())
}

func TestMemoryDB_UpdateDialAttempt(t *testing.T) {
	config := DefaultConfig()
	config.CleanupInterval = 1 * time.Hour

	db := NewMemoryDB(config)
	defer db.Close()

	// 插入节点
	err := db.UpdateNode(&NodeRecord{ID: "peer1"})
	require.NoError(t, err)

	// 拨号失败
	err = db.UpdateDialAttempt("peer1", false)
	require.NoError(t, err)

	node := db.GetNode("peer1")
	assert.Equal(t, 1, node.FailedDials)

	// 再次失败
	err = db.UpdateDialAttempt("peer1", false)
	require.NoError(t, err)

	node = db.GetNode("peer1")
	assert.Equal(t, 2, node.FailedDials)

	// 拨号成功
	err = db.UpdateDialAttempt("peer1", true)
	require.NoError(t, err)

	node = db.GetNode("peer1")
	assert.Equal(t, 0, node.FailedDials) // 重置
}

func TestMemoryDB_MaxNodes(t *testing.T) {
	config := DefaultConfig()
	config.MaxNodes = 5
	config.CleanupInterval = 1 * time.Hour
	config.NodeExpiry = 24 * time.Hour

	db := NewMemoryDB(config)
	defer db.Close()

	// 插入超过最大数量的节点
	for i := 0; i < 10; i++ {
		err := db.UpdateNode(&NodeRecord{
			ID:       "peer" + string(rune('0'+i)),
			LastSeen: time.Now().Add(-time.Duration(i) * time.Minute),
		})
		require.NoError(t, err)
	}

	// 应该只保留最大数量
	assert.Equal(t, 5, db.Size())
}

func TestMemoryDB_Stats(t *testing.T) {
	config := DefaultConfig()
	config.CleanupInterval = 1 * time.Hour

	db := NewMemoryDB(config)
	defer db.Close()

	// 插入节点
	err := db.UpdateNode(&NodeRecord{
		ID:       "peer1",
		LastSeen: time.Now(),
	})
	require.NoError(t, err)

	err = db.UpdateNode(&NodeRecord{
		ID:       "peer2",
		LastSeen: time.Now().Add(-2 * time.Hour),
	})
	require.NoError(t, err)

	err = db.UpdateNode(&NodeRecord{
		ID:          "peer3",
		LastSeen:    time.Now(),
		FailedDials: 10,
	})
	require.NoError(t, err)

	stats := db.Stats()

	assert.Equal(t, 3, stats.TotalNodes)
	assert.Equal(t, 2, stats.ActiveNodes)      // peer1 和 peer3
	assert.Equal(t, 1, stats.UnreachableNodes) // peer3
}

func TestMemoryDB_Close(t *testing.T) {
	config := DefaultConfig()
	config.CleanupInterval = 1 * time.Hour

	db := NewMemoryDB(config)

	// 插入节点
	err := db.UpdateNode(&NodeRecord{ID: "peer1"})
	require.NoError(t, err)

	// 关闭
	err = db.Close()
	require.NoError(t, err)

	// 重复关闭
	err = db.Close()
	require.NoError(t, err)

	// 关闭后操作应该失败
	err = db.UpdateNode(&NodeRecord{ID: "peer2"})
	assert.Equal(t, ErrDatabaseClosed, err)

	node := db.GetNode("peer1")
	assert.Nil(t, node)
}

func TestMemoryDB_CleanupExpired(t *testing.T) {
	config := DefaultConfig()
	config.NodeExpiry = 500 * time.Millisecond
	config.CleanupInterval = 100 * time.Millisecond

	db := NewMemoryDB(config)
	defer db.Close()

	// 插入节点
	err := db.UpdateNode(&NodeRecord{
		ID:       "peer1",
		LastSeen: time.Now().Add(-2 * time.Second), // 已过期
	})
	require.NoError(t, err)

	err = db.UpdateNode(&NodeRecord{
		ID:       "peer2",
		LastSeen: time.Now(), // 未过期
	})
	require.NoError(t, err)

	// 等待清理
	time.Sleep(200 * time.Millisecond)

	// peer1 应该被清理
	node := db.GetNode("peer1")
	assert.Nil(t, node)

	// peer2 应该还在
	node = db.GetNode("peer2")
	assert.NotNil(t, node)
}
