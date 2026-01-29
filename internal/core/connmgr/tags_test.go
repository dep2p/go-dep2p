package connmgr

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTagStore_Set 测试设置标签
func TestTagStore_Set(t *testing.T) {
	store := newTagStore()

	peer := "peer-1"

	store.Set(peer, "tag1", 10)
	assert.Equal(t, 10, store.Get(peer, "tag1"))

	// 覆盖旧值
	store.Set(peer, "tag1", 20)
	assert.Equal(t, 20, store.Get(peer, "tag1"))

	t.Log("✅ TagStore Set 正确")
}

// TestTagStore_Get 测试获取标签
func TestTagStore_Get(t *testing.T) {
	store := newTagStore()

	peer := "peer-1"

	// 不存在的标签返回 0
	assert.Equal(t, 0, store.Get(peer, "notexist"))

	store.Set(peer, "tag1", 100)
	assert.Equal(t, 100, store.Get(peer, "tag1"))

	t.Log("✅ TagStore Get 正确")
}

// TestTagStore_Delete 测试删除标签
func TestTagStore_Delete(t *testing.T) {
	store := newTagStore()

	peer := "peer-1"

	store.Set(peer, "tag1", 10)
	store.Set(peer, "tag2", 20)

	// 删除一个标签
	store.Delete(peer, "tag1")
	assert.Equal(t, 0, store.Get(peer, "tag1"))
	assert.Equal(t, 20, store.Get(peer, "tag2"))

	// 删除最后一个标签，节点记录也被删除
	store.Delete(peer, "tag2")
	assert.False(t, store.HasPeer(peer))

	t.Log("✅ TagStore Delete 正确")
}

// TestTagStore_Sum 测试计算总和
func TestTagStore_Sum(t *testing.T) {
	store := newTagStore()

	peer := "peer-1"

	store.Set(peer, "tag1", 10)
	store.Set(peer, "tag2", 20)
	store.Set(peer, "tag3", 30)

	assert.Equal(t, 60, store.Sum(peer))

	// 不存在的节点返回 0
	assert.Equal(t, 0, store.Sum("notexist"))

	t.Log("✅ TagStore Sum 正确")
}

// TestTagStore_GetInfo 测试获取信息
func TestTagStore_GetInfo(t *testing.T) {
	store := newTagStore()

	peer := "peer-1"

	// 不存在的节点返回 nil
	assert.Nil(t, store.GetInfo("notexist"))

	store.Set(peer, "tag1", 10)
	store.Set(peer, "tag2", 20)

	info := store.GetInfo(peer)
	require.NotNil(t, info)
	assert.Equal(t, 30, info.Value)
	assert.Equal(t, 10, info.Tags["tag1"])
	assert.Equal(t, 20, info.Tags["tag2"])
	assert.False(t, info.FirstSeen.IsZero())

	t.Log("✅ TagStore GetInfo 正确")
}

// TestTagStore_Upsert 测试更新或插入
func TestTagStore_Upsert(t *testing.T) {
	store := newTagStore()

	peer := "peer-1"

	// 不存在，插入
	store.Upsert(peer, "count", func(old int) int {
		assert.Equal(t, 0, old)
		return 10
	})
	assert.Equal(t, 10, store.Get(peer, "count"))

	// 存在，更新
	store.Upsert(peer, "count", func(old int) int {
		assert.Equal(t, 10, old)
		return old * 2
	})
	assert.Equal(t, 20, store.Get(peer, "count"))

	t.Log("✅ TagStore Upsert 正确")
}

// TestTagStore_FirstSeen 测试首次发现时间
func TestTagStore_FirstSeen(t *testing.T) {
	store := newTagStore()

	peer := "peer-1"

	before := time.Now()
	store.Set(peer, "tag1", 10)
	firstSeen := store.FirstSeen(peer)
	after := time.Now()

	assert.False(t, firstSeen.IsZero())
	assert.True(t, firstSeen.After(before) || firstSeen.Equal(before))
	assert.True(t, firstSeen.Before(after) || firstSeen.Equal(after))

	t.Log("✅ TagStore FirstSeen 正确")
}

// TestTagStore_HasPeer 测试是否有节点
func TestTagStore_HasPeer(t *testing.T) {
	store := newTagStore()

	peer := "peer-1"

	assert.False(t, store.HasPeer(peer))

	store.Set(peer, "tag1", 10)
	assert.True(t, store.HasPeer(peer))

	store.Delete(peer, "tag1")
	assert.False(t, store.HasPeer(peer))

	t.Log("✅ TagStore HasPeer 正确")
}

// TestTagStore_Clear 测试清空
func TestTagStore_Clear(t *testing.T) {
	store := newTagStore()

	store.Set("peer-1", "tag1", 10)
	store.Set("peer-2", "tag2", 20)

	assert.True(t, store.HasPeer("peer-1"))
	assert.True(t, store.HasPeer("peer-2"))

	store.Clear()

	assert.False(t, store.HasPeer("peer-1"))
	assert.False(t, store.HasPeer("peer-2"))

	t.Log("✅ TagStore Clear 正确")
}
