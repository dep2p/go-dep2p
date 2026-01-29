// Package reachability 提供可达性协调模块的实现
package reachability

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dep2p/go-dep2p/pkg/interfaces"
)

func TestNewDirectAddrStore(t *testing.T) {
	store := NewDirectAddrStore("")

	assert.NotNil(t, store)
	assert.NotEmpty(t, store.storePath)
	assert.NotNil(t, store.candidates)
	assert.NotNil(t, store.verified)
}

func TestDirectAddrStore_SetConfig(t *testing.T) {
	store := NewDirectAddrStore("")

	store.SetConfig(500, 1*time.Hour, 12*time.Hour, 2*time.Second)

	assert.Equal(t, 500, store.maxEntries)
	assert.Equal(t, 1*time.Hour, store.candidateTTL)
	assert.Equal(t, 12*time.Hour, store.verifiedTTL)
	assert.Equal(t, 2*time.Second, store.flushDebounce)
}

func TestDirectAddrStore_UpdateCandidate(t *testing.T) {
	store := NewDirectAddrStore("")

	addr := "/ip4/1.2.3.4/udp/4001/quic-v1"
	store.UpdateCandidate(addr, "test", interfaces.PriorityUnverified)

	candidates := store.GetCandidates()
	assert.Len(t, candidates, 1)
	assert.Equal(t, addr, candidates[addr].AddrString)
	assert.Equal(t, "test", candidates[addr].Source)
}

func TestDirectAddrStore_UpdateCandidate_MultipleSource(t *testing.T) {
	store := NewDirectAddrStore("")

	addr := "/ip4/1.2.3.4/udp/4001/quic-v1"
	store.UpdateCandidate(addr, "source1", interfaces.PriorityUnverified)
	store.UpdateCandidate(addr, "source2", interfaces.PriorityUnverified)

	candidates := store.GetCandidates()
	entry := candidates[addr]
	assert.Len(t, entry.Sources, 2)
	assert.Contains(t, entry.Sources, "source1")
	assert.Contains(t, entry.Sources, "source2")
}

func TestDirectAddrStore_UpdateVerified(t *testing.T) {
	store := NewDirectAddrStore("")

	addr := "/ip4/1.2.3.4/udp/4001/quic-v1"

	// 先添加为候选
	store.UpdateCandidate(addr, "test", interfaces.PriorityUnverified)
	assert.Len(t, store.GetCandidates(), 1)

	// 升级为已验证
	store.UpdateVerified(addr, "dial-back", interfaces.PriorityVerifiedDirect)

	// 验证候选已移除，已验证已添加
	assert.Len(t, store.GetCandidates(), 0)
	verified := store.GetVerified()
	assert.Len(t, verified, 1)
	assert.True(t, verified[addr].Verified)
}

func TestDirectAddrStore_RemoveCandidate(t *testing.T) {
	store := NewDirectAddrStore("")

	addr := "/ip4/1.2.3.4/udp/4001/quic-v1"
	store.UpdateCandidate(addr, "test", interfaces.PriorityUnverified)
	assert.Len(t, store.GetCandidates(), 1)

	store.RemoveCandidate(addr)
	assert.Len(t, store.GetCandidates(), 0)
}

func TestDirectAddrStore_RemoveVerified(t *testing.T) {
	store := NewDirectAddrStore("")

	addr := "/ip4/1.2.3.4/udp/4001/quic-v1"
	store.UpdateVerified(addr, "test", interfaces.PriorityVerifiedDirect)
	assert.Len(t, store.GetVerified(), 1)

	store.RemoveVerified(addr)
	assert.Len(t, store.GetVerified(), 0)
}

func TestDirectAddrStore_SaveLoad(t *testing.T) {
	// 使用临时目录
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "test_addrs.json")

	// 创建并写入数据
	store := NewDirectAddrStore(storePath)
	store.UpdateCandidate("/ip4/1.1.1.1/udp/4001/quic-v1", "stun", interfaces.PriorityUnverified)
	store.UpdateVerified("/ip4/2.2.2.2/udp/4001/quic-v1", "dial-back", interfaces.PriorityVerifiedDirect)

	err := store.Save()
	require.NoError(t, err)

	// 验证文件存在
	_, err = os.Stat(storePath)
	require.NoError(t, err)

	// 创建新实例并加载
	store2 := NewDirectAddrStore(storePath)
	err = store2.Load()
	require.NoError(t, err)

	// 验证数据
	candidates := store2.GetCandidates()
	verified := store2.GetVerified()

	assert.Len(t, candidates, 1)
	assert.Len(t, verified, 1)
	assert.Contains(t, candidates, "/ip4/1.1.1.1/udp/4001/quic-v1")
	assert.Contains(t, verified, "/ip4/2.2.2.2/udp/4001/quic-v1")
}

func TestDirectAddrStore_Load_NonExistent(t *testing.T) {
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "non_existent.json")

	store := NewDirectAddrStore(storePath)
	err := store.Load()
	assert.NoError(t, err)

	// 应该使用空状态
	assert.Len(t, store.GetCandidates(), 0)
	assert.Len(t, store.GetVerified(), 0)
}

func TestDirectAddrStore_CleanExpired(t *testing.T) {
	store := NewDirectAddrStore("")
	store.SetConfig(100, 100*time.Millisecond, 100*time.Millisecond, time.Second)

	// 添加地址
	store.UpdateCandidate("/ip4/1.1.1.1/udp/4001/quic-v1", "test", interfaces.PriorityUnverified)
	store.UpdateVerified("/ip4/2.2.2.2/udp/4001/quic-v1", "test", interfaces.PriorityVerifiedDirect)

	// 等待过期
	time.Sleep(200 * time.Millisecond)

	// 清理
	candidatesRemoved, verifiedRemoved := store.CleanExpired()

	assert.Equal(t, 1, candidatesRemoved)
	assert.Equal(t, 1, verifiedRemoved)
	assert.Len(t, store.GetCandidates(), 0)
	assert.Len(t, store.GetVerified(), 0)
}

func TestDirectAddrStore_MaxEntries_Candidates(t *testing.T) {
	store := NewDirectAddrStore("")
	store.SetConfig(2, time.Hour, time.Hour, time.Second)

	// 直接设置候选地址以确保时间顺序
	store.mu.Lock()
	store.candidates["/ip4/1.1.1.1/udp/4001/quic-v1"] = &storedAddressEntry{
		AddrString: "/ip4/1.1.1.1/udp/4001/quic-v1",
		Priority:   interfaces.PriorityUnverified,
		LastSeen:   time.Now().Add(-10 * time.Second).Unix(),
	}
	store.candidates["/ip4/2.2.2.2/udp/4001/quic-v1"] = &storedAddressEntry{
		AddrString: "/ip4/2.2.2.2/udp/4001/quic-v1",
		Priority:   interfaces.PriorityUnverified,
		LastSeen:   time.Now().Add(-5 * time.Second).Unix(),
	}
	store.mu.Unlock()

	// 添加第三个地址会触发 evict
	store.UpdateCandidate("/ip4/3.3.3.3/udp/4001/quic-v1", "test", interfaces.PriorityUnverified)

	// 应该只保留 2 个
	candidates := store.GetCandidates()
	assert.Equal(t, 2, len(candidates))

	// 最旧的应该被淘汰
	assert.NotContains(t, candidates, "/ip4/1.1.1.1/udp/4001/quic-v1")
}

func TestDirectAddrStore_MaxEntries_Verified(t *testing.T) {
	store := NewDirectAddrStore("")
	store.SetConfig(2, time.Hour, time.Hour, time.Second)

	// 添加超过限制的已验证地址
	// 注意：第三个地址添加时会触发 evict
	store.mu.Lock()
	store.verified["/ip4/1.1.1.1/udp/4001/quic-v1"] = &storedAddressEntry{
		AddrString: "/ip4/1.1.1.1/udp/4001/quic-v1",
		Priority:   interfaces.PriorityVerifiedDirect,
		LastSeen:   time.Now().Add(-10 * time.Second).Unix(),
	}
	store.verified["/ip4/2.2.2.2/udp/4001/quic-v1"] = &storedAddressEntry{
		AddrString: "/ip4/2.2.2.2/udp/4001/quic-v1",
		Priority:   interfaces.PriorityVerifiedDirect,
		LastSeen:   time.Now().Add(-5 * time.Second).Unix(),
	}
	store.mu.Unlock()

	// 添加第三个地址会触发 evict
	store.UpdateVerified("/ip4/3.3.3.3/udp/4001/quic-v1", "test", interfaces.PriorityVerifiedDirect)

	// 应该只保留 2 个
	verified := store.GetVerified()
	assert.Equal(t, 2, len(verified))

	// 最旧的应该被淘汰
	assert.NotContains(t, verified, "/ip4/1.1.1.1/udp/4001/quic-v1")
}

func TestDirectAddrStore_Close(t *testing.T) {
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "test_close.json")

	store := NewDirectAddrStore(storePath)
	store.UpdateCandidate("/ip4/1.1.1.1/udp/4001/quic-v1", "test", interfaces.PriorityUnverified)

	err := store.Close()
	require.NoError(t, err)

	// 验证文件已保存
	_, err = os.Stat(storePath)
	assert.NoError(t, err)
}

func TestDirectAddrStore_ScheduleFlush(t *testing.T) {
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "test_flush.json")

	store := NewDirectAddrStore(storePath)
	store.SetConfig(100, time.Hour, time.Hour, 50*time.Millisecond)

	// 添加地址（会触发 ScheduleFlush）
	store.UpdateCandidate("/ip4/1.1.1.1/udp/4001/quic-v1", "test", interfaces.PriorityUnverified)

	// 等待 flush
	time.Sleep(100 * time.Millisecond)

	// 验证文件已保存
	_, err := os.Stat(storePath)
	assert.NoError(t, err)
}
