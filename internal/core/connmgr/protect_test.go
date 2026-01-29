package connmgr

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestProtectStore_Protect 测试保护
func TestProtectStore_Protect(t *testing.T) {
	store := newProtectStore()

	peer := "peer-1"

	store.Protect(peer, "tag1")
	assert.True(t, store.IsProtected(peer, "tag1"))

	// 添加多个保护标签
	store.Protect(peer, "tag2")
	assert.True(t, store.IsProtected(peer, "tag1"))
	assert.True(t, store.IsProtected(peer, "tag2"))

	t.Log("✅ ProtectStore Protect 正确")
}

// TestProtectStore_Unprotect 测试取消保护
func TestProtectStore_Unprotect(t *testing.T) {
	store := newProtectStore()

	peer := "peer-1"

	// 不存在的保护
	hasMore := store.Unprotect(peer, "tag1")
	assert.False(t, hasMore)

	// 添加两个保护标签
	store.Protect(peer, "tag1")
	store.Protect(peer, "tag2")

	// 移除一个，还有剩余
	hasMore = store.Unprotect(peer, "tag1")
	assert.True(t, hasMore)
	assert.False(t, store.IsProtected(peer, "tag1"))
	assert.True(t, store.IsProtected(peer, "tag2"))

	// 移除最后一个
	hasMore = store.Unprotect(peer, "tag2")
	assert.False(t, hasMore)
	assert.False(t, store.HasAnyProtection(peer))

	t.Log("✅ ProtectStore Unprotect 正确")
}

// TestProtectStore_IsProtected 测试检查保护
func TestProtectStore_IsProtected(t *testing.T) {
	store := newProtectStore()

	peer := "peer-1"

	// 不存在的保护
	assert.False(t, store.IsProtected(peer, "tag1"))

	store.Protect(peer, "tag1")
	assert.True(t, store.IsProtected(peer, "tag1"))
	assert.False(t, store.IsProtected(peer, "tag2"))

	// 空标签检查是否有任何保护
	assert.True(t, store.IsProtected(peer, ""))

	t.Log("✅ ProtectStore IsProtected 正确")
}

// TestProtectStore_HasAnyProtection 测试是否有任何保护
func TestProtectStore_HasAnyProtection(t *testing.T) {
	store := newProtectStore()

	peer := "peer-1"

	assert.False(t, store.HasAnyProtection(peer))

	store.Protect(peer, "tag1")
	assert.True(t, store.HasAnyProtection(peer))

	store.Protect(peer, "tag2")
	assert.True(t, store.HasAnyProtection(peer))

	store.Unprotect(peer, "tag1")
	assert.True(t, store.HasAnyProtection(peer))

	store.Unprotect(peer, "tag2")
	assert.False(t, store.HasAnyProtection(peer))

	t.Log("✅ ProtectStore HasAnyProtection 正确")
}

// TestProtectStore_GetProtections 测试获取保护列表
func TestProtectStore_GetProtections(t *testing.T) {
	store := newProtectStore()

	peer := "peer-1"

	// 不存在的节点
	tags := store.GetProtections(peer)
	assert.Nil(t, tags)

	// 添加保护标签
	store.Protect(peer, "tag1")
	store.Protect(peer, "tag2")

	tags = store.GetProtections(peer)
	assert.Len(t, tags, 2)
	assert.Contains(t, tags, "tag1")
	assert.Contains(t, tags, "tag2")

	t.Log("✅ ProtectStore GetProtections 正确")
}

// TestProtectStore_Clear 测试清空
func TestProtectStore_Clear(t *testing.T) {
	store := newProtectStore()

	store.Protect("peer-1", "tag1")
	store.Protect("peer-2", "tag2")

	assert.True(t, store.HasAnyProtection("peer-1"))
	assert.True(t, store.HasAnyProtection("peer-2"))

	store.Clear()

	assert.False(t, store.HasAnyProtection("peer-1"))
	assert.False(t, store.HasAnyProtection("peer-2"))

	t.Log("✅ ProtectStore Clear 正确")
}
