package pubsub

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMeshPeers_Add(t *testing.T) {
	mesh := newMeshPeers(6, 4, 12)

	// 添加节点
	ok := mesh.Add("topic1", "peer-1")
	assert.True(t, ok)

	// 验证已添加
	assert.True(t, mesh.Has("topic1", "peer-1"))
	assert.Equal(t, 1, mesh.Count("topic1"))
}

func TestMeshPeers_Add_Full(t *testing.T) {
	mesh := newMeshPeers(6, 4, 12)

	// 添加到上限
	for i := 0; i < 12; i++ {
		ok := mesh.Add("topic1", string(rune('a'+i)))
		assert.True(t, ok)
	}

	// 再添加应失败
	ok := mesh.Add("topic1", "peer-overflow")
	assert.False(t, ok)
}

func TestMeshPeers_Remove(t *testing.T) {
	mesh := newMeshPeers(6, 4, 12)

	mesh.Add("topic1", "peer-1")
	assert.True(t, mesh.Has("topic1", "peer-1"))

	// 移除
	mesh.Remove("topic1", "peer-1")
	assert.False(t, mesh.Has("topic1", "peer-1"))
	assert.Equal(t, 0, mesh.Count("topic1"))
}

func TestMeshPeers_List(t *testing.T) {
	mesh := newMeshPeers(6, 4, 12)

	// 添加多个节点
	mesh.Add("topic1", "peer-1")
	mesh.Add("topic1", "peer-2")
	mesh.Add("topic1", "peer-3")

	peers := mesh.List("topic1")
	assert.Len(t, peers, 3)
	assert.Contains(t, peers, "peer-1")
	assert.Contains(t, peers, "peer-2")
	assert.Contains(t, peers, "peer-3")
}

func TestMeshPeers_NeedMorePeers(t *testing.T) {
	mesh := newMeshPeers(6, 4, 12)

	// 空 Mesh 需要更多节点
	assert.True(t, mesh.NeedMorePeers("topic1"))

	// 添加节点
	for i := 0; i < 6; i++ {
		mesh.Add("topic1", string(rune('a'+i)))
	}

	// 达到目标度数,不需要更多
	assert.False(t, mesh.NeedMorePeers("topic1"))
}

func TestMeshPeers_TooManyPeers(t *testing.T) {
	mesh := newMeshPeers(6, 4, 12)

	// 添加到上限以上
	for i := 0; i < 12; i++ {
		mesh.Add("topic1", string(rune('a'+i)))
	}

	// 不应该过多(等于上限)
	assert.False(t, mesh.TooManyPeers("topic1"))

	// 手动添加超过上限(绕过 Add 的检查)
	mesh.mu.Lock()
	mesh.peers["topic1"]["peer-overflow"] = true
	mesh.mu.Unlock()

	// 现在应该过多
	assert.True(t, mesh.TooManyPeers("topic1"))
}

func TestMeshPeers_TooFewPeers(t *testing.T) {
	mesh := newMeshPeers(6, 4, 12)

	// 空 Mesh 太少
	assert.True(t, mesh.TooFewPeers("topic1"))

	// 添加到下限
	for i := 0; i < 4; i++ {
		mesh.Add("topic1", string(rune('a'+i)))
	}

	// 等于下限,不算太少
	assert.False(t, mesh.TooFewPeers("topic1"))
}

func TestMeshPeers_SelectPeersToGraft(t *testing.T) {
	mesh := newMeshPeers(6, 4, 12)

	// 已在 Mesh 中的节点
	mesh.Add("topic1", "peer-1")
	mesh.Add("topic1", "peer-2")

	// 候选节点
	candidates := []string{"peer-1", "peer-2", "peer-3", "peer-4", "peer-5"}

	// 选择2个节点
	selected := mesh.SelectPeersToGraft("topic1", candidates, 2)

	// 应该只选择不在 Mesh 中的节点
	assert.Len(t, selected, 2)
	for _, peer := range selected {
		assert.NotContains(t, []string{"peer-1", "peer-2"}, peer)
	}
}

func TestMeshPeers_SelectPeersToPrune(t *testing.T) {
	mesh := newMeshPeers(6, 4, 12)

	// 添加节点
	for i := 0; i < 8; i++ {
		mesh.Add("topic1", string(rune('a'+i)))
	}

	// 选择2个节点
	selected := mesh.SelectPeersToPrune("topic1", 2)
	assert.Len(t, selected, 2)
}

func TestMeshPeers_Clear(t *testing.T) {
	mesh := newMeshPeers(6, 4, 12)

	// 添加节点
	mesh.Add("topic1", "peer-1")
	mesh.Add("topic1", "peer-2")
	assert.Equal(t, 2, mesh.Count("topic1"))

	// 清空
	mesh.Clear("topic1")
	assert.Equal(t, 0, mesh.Count("topic1"))
}
