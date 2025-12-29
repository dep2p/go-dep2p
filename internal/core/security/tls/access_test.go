package tls

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dep2p/go-dep2p/internal/core/identity"
	identityif "github.com/dep2p/go-dep2p/pkg/interfaces/identity"
	"github.com/dep2p/go-dep2p/pkg/types"
)

func createTestNodeID(t *testing.T) types.NodeID {
	cfg := identityif.DefaultConfig()
	mgr := identity.NewManager(cfg)
	ident, err := mgr.Create()
	require.NoError(t, err)
	return ident.ID()
}

func TestNewAccessController(t *testing.T) {
	ac := NewAccessController()
	assert.NotNil(t, ac)
	assert.Equal(t, AccessModeAllow, ac.Mode())
	assert.Equal(t, 0, ac.AllowListCount())
	assert.Equal(t, 0, ac.BlockListCount())
}

func TestNewAccessControllerWithMode(t *testing.T) {
	ac := NewAccessControllerWithMode(AccessModeDeny)
	assert.Equal(t, AccessModeDeny, ac.Mode())
}

func TestSetMode(t *testing.T) {
	ac := NewAccessController()
	assert.Equal(t, AccessModeAllow, ac.Mode())

	ac.SetMode(AccessModeDeny)
	assert.Equal(t, AccessModeDeny, ac.Mode())

	ac.SetMode(AccessModeAllow)
	assert.Equal(t, AccessModeAllow, ac.Mode())
}

func TestAllowConnectAllowMode(t *testing.T) {
	ac := NewAccessController()
	nodeID := createTestNodeID(t)

	// 允许模式：默认允许所有
	assert.True(t, ac.AllowConnect(nodeID))

	// 空 NodeID 总是拒绝
	assert.False(t, ac.AllowConnect(types.EmptyNodeID))

	// 加入黑名单后拒绝
	ac.AddToBlockList(nodeID)
	assert.False(t, ac.AllowConnect(nodeID))

	// 从黑名单移除后允许
	ac.RemoveFromBlockList(nodeID)
	assert.True(t, ac.AllowConnect(nodeID))
}

func TestAllowConnectDenyMode(t *testing.T) {
	ac := NewAccessControllerWithMode(AccessModeDeny)
	nodeID := createTestNodeID(t)

	// 拒绝模式：默认拒绝所有
	assert.False(t, ac.AllowConnect(nodeID))

	// 加入白名单后允许
	ac.AddToAllowList(nodeID)
	assert.True(t, ac.AllowConnect(nodeID))

	// 同时在黑名单中则拒绝
	ac.AddToBlockList(nodeID)
	assert.False(t, ac.AllowConnect(nodeID))

	// 从白名单移除后拒绝
	ac.RemoveFromBlockList(nodeID)
	ac.RemoveFromAllowList(nodeID)
	assert.False(t, ac.AllowConnect(nodeID))
}

func TestAllowInboundOutbound(t *testing.T) {
	ac := NewAccessController()
	nodeID := createTestNodeID(t)

	// AllowInbound 和 AllowOutbound 应该与 AllowConnect 行为一致
	assert.True(t, ac.AllowInbound(nodeID))
	assert.True(t, ac.AllowOutbound(nodeID))

	ac.AddToBlockList(nodeID)
	assert.False(t, ac.AllowInbound(nodeID))
	assert.False(t, ac.AllowOutbound(nodeID))
}

func TestAddRemoveAllowList(t *testing.T) {
	ac := NewAccessController()
	nodeID := createTestNodeID(t)

	// 初始为空
	assert.False(t, ac.IsInAllowList(nodeID))
	assert.Equal(t, 0, ac.AllowListCount())

	// 添加到白名单
	ac.AddToAllowList(nodeID)
	assert.True(t, ac.IsInAllowList(nodeID))
	assert.Equal(t, 1, ac.AllowListCount())

	// 重复添加
	ac.AddToAllowList(nodeID)
	assert.Equal(t, 1, ac.AllowListCount())

	// 从白名单移除
	ac.RemoveFromAllowList(nodeID)
	assert.False(t, ac.IsInAllowList(nodeID))
	assert.Equal(t, 0, ac.AllowListCount())

	// 添加空 NodeID 无效
	ac.AddToAllowList(types.EmptyNodeID)
	assert.Equal(t, 0, ac.AllowListCount())
}

func TestAddRemoveBlockList(t *testing.T) {
	ac := NewAccessController()
	nodeID := createTestNodeID(t)

	// 初始为空
	assert.False(t, ac.IsInBlockList(nodeID))
	assert.Equal(t, 0, ac.BlockListCount())

	// 添加到黑名单
	ac.AddToBlockList(nodeID)
	assert.True(t, ac.IsInBlockList(nodeID))
	assert.Equal(t, 1, ac.BlockListCount())

	// 重复添加
	ac.AddToBlockList(nodeID)
	assert.Equal(t, 1, ac.BlockListCount())

	// 从黑名单移除
	ac.RemoveFromBlockList(nodeID)
	assert.False(t, ac.IsInBlockList(nodeID))
	assert.Equal(t, 0, ac.BlockListCount())

	// 添加空 NodeID 无效
	ac.AddToBlockList(types.EmptyNodeID)
	assert.Equal(t, 0, ac.BlockListCount())
}

func TestClearLists(t *testing.T) {
	ac := NewAccessController()
	nodeID1 := createTestNodeID(t)
	nodeID2 := createTestNodeID(t)

	ac.AddToAllowList(nodeID1)
	ac.AddToBlockList(nodeID2)
	assert.Equal(t, 1, ac.AllowListCount())
	assert.Equal(t, 1, ac.BlockListCount())

	// 清空白名单
	ac.ClearAllowList()
	assert.Equal(t, 0, ac.AllowListCount())
	assert.Equal(t, 1, ac.BlockListCount())

	// 清空黑名单
	ac.ClearBlockList()
	assert.Equal(t, 0, ac.BlockListCount())
}

func TestClearAll(t *testing.T) {
	ac := NewAccessController()
	nodeID1 := createTestNodeID(t)
	nodeID2 := createTestNodeID(t)

	ac.AddToAllowList(nodeID1)
	ac.AddToBlockList(nodeID2)

	// 清空所有
	ac.Clear()
	assert.Equal(t, 0, ac.AllowListCount())
	assert.Equal(t, 0, ac.BlockListCount())
}

func TestAllowedPeers(t *testing.T) {
	ac := NewAccessController()
	nodeID1 := createTestNodeID(t)
	nodeID2 := createTestNodeID(t)

	ac.AddToAllowList(nodeID1)
	ac.AddToAllowList(nodeID2)

	peers := ac.AllowedPeers()
	assert.Len(t, peers, 2)
	assert.Contains(t, peers, nodeID1)
	assert.Contains(t, peers, nodeID2)
}

func TestBlockedPeers(t *testing.T) {
	ac := NewAccessController()
	nodeID1 := createTestNodeID(t)
	nodeID2 := createTestNodeID(t)

	ac.AddToBlockList(nodeID1)
	ac.AddToBlockList(nodeID2)

	peers := ac.BlockedPeers()
	assert.Len(t, peers, 2)
	assert.Contains(t, peers, nodeID1)
	assert.Contains(t, peers, nodeID2)
}

func TestConcurrentAccess(t *testing.T) {
	ac := NewAccessController()
	nodeID := createTestNodeID(t)

	// 并发读写测试
	done := make(chan bool)

	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				ac.AddToAllowList(nodeID)
				ac.AllowConnect(nodeID)
				ac.RemoveFromAllowList(nodeID)
			}
			done <- true
		}()
	}

	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				ac.AddToBlockList(nodeID)
				ac.AllowConnect(nodeID)
				ac.RemoveFromBlockList(nodeID)
			}
			done <- true
		}()
	}

	// 等待所有 goroutine 完成
	for i := 0; i < 20; i++ {
		<-done
	}
}

