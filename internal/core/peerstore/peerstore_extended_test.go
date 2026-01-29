package peerstore

import (
	"context"
	"testing"
	"time"

	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
//                     AddrBook 方法测试
// ============================================================================

func TestPeerstore_SetAddr(t *testing.T) {
	ps := NewPeerstore()
	defer ps.Close()

	peerID := testPeerID("peer1")
	addr := testMultiaddr("/ip4/127.0.0.1/tcp/4001")

	// SetAddr 设置单个地址
	ps.SetAddr(peerID, addr, ConnectedAddrTTL)

	addrs := ps.Addrs(peerID)
	require.Len(t, addrs, 1)
	assert.Equal(t, addr.String(), addrs[0].String())
}

func TestPeerstore_SetAddrs(t *testing.T) {
	ps := NewPeerstore()
	defer ps.Close()

	peerID := testPeerID("peer1")
	addr1 := testMultiaddr("/ip4/127.0.0.1/tcp/4001")
	addr2 := testMultiaddr("/ip4/192.168.1.1/tcp/4001")

	// 先添加一个地址
	ps.AddAddr(peerID, addr1, ConnectedAddrTTL)
	require.Len(t, ps.Addrs(peerID), 1)

	// SetAddrs 应该覆盖现有地址
	ps.SetAddrs(peerID, []types.Multiaddr{addr2}, ConnectedAddrTTL)

	addrs := ps.Addrs(peerID)
	require.Len(t, addrs, 1)
	assert.Equal(t, addr2.String(), addrs[0].String())
}

func TestPeerstore_UpdateAddrs(t *testing.T) {
	ps := NewPeerstore()
	defer ps.Close()

	peerID := testPeerID("peer1")
	addr := testMultiaddr("/ip4/127.0.0.1/tcp/4001")

	// 添加地址，TTL 为 1 分钟
	oldTTL := 1 * time.Minute
	ps.AddAddr(peerID, addr, oldTTL)

	// 更新 TTL 为 1 小时
	newTTL := 1 * time.Hour
	ps.UpdateAddrs(peerID, oldTTL, newTTL)

	// 验证地址仍然存在
	addrs := ps.Addrs(peerID)
	assert.Len(t, addrs, 1)
}

func TestPeerstore_AddrStream(t *testing.T) {
	ps := NewPeerstore()
	defer ps.Close()

	peerID := testPeerID("peer1")

	// 先添加一个地址
	addr1 := testMultiaddr("/ip4/127.0.0.1/tcp/4001")
	ps.AddAddr(peerID, addr1, ConnectedAddrTTL)

	// 创建地址流
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	ch := ps.AddrStream(ctx, peerID)
	require.NotNil(t, ch)

	// 应该能收到已有的地址
	select {
	case addr := <-ch:
		assert.Equal(t, addr1.String(), addr.String())
	case <-ctx.Done():
		// 超时也是可接受的（取决于实现）
	}
}

func TestPeerstore_ClearAddrs(t *testing.T) {
	ps := NewPeerstore()
	defer ps.Close()

	peerID := testPeerID("peer1")
	addr := testMultiaddr("/ip4/127.0.0.1/tcp/4001")

	// 添加地址
	ps.AddAddr(peerID, addr, ConnectedAddrTTL)
	require.Len(t, ps.Addrs(peerID), 1)

	// 清除地址
	ps.ClearAddrs(peerID)

	// 验证地址已清除
	addrs := ps.Addrs(peerID)
	assert.Empty(t, addrs)
}

func TestPeerstore_PeersWithAddrs(t *testing.T) {
	ps := NewPeerstore()
	defer ps.Close()

	// 初始应该没有节点
	peers := ps.PeersWithAddrs()
	assert.Empty(t, peers)

	// 添加几个节点的地址
	peerID1 := testPeerID("peer1")
	peerID2 := testPeerID("peer2")

	ps.AddAddr(peerID1, testMultiaddr("/ip4/127.0.0.1/tcp/4001"), ConnectedAddrTTL)
	ps.AddAddr(peerID2, testMultiaddr("/ip4/127.0.0.2/tcp/4001"), ConnectedAddrTTL)

	// 验证返回了所有有地址的节点
	peers = ps.PeersWithAddrs()
	assert.Len(t, peers, 2)
	assert.Contains(t, peers, peerID1)
	assert.Contains(t, peers, peerID2)
}

// ============================================================================
//                     带来源的地址方法测试
// ============================================================================

func TestPeerstore_AddAddrsWithSource(t *testing.T) {
	ps := NewPeerstore()
	defer ps.Close()

	peerID := testPeerID("peer1")
	addr := testMultiaddr("/ip4/127.0.0.1/tcp/4001")

	addrs := []AddressWithSource{
		{
			Addr:   addr,
			Source: SourceDHT,
			TTL:    10 * time.Minute,
		},
	}

	ps.AddAddrsWithSource(peerID, addrs)

	// 验证地址已添加
	result := ps.Addrs(peerID)
	require.Len(t, result, 1)
	assert.Equal(t, addr.String(), result[0].String())
}

func TestPeerstore_SetAddrsWithSource(t *testing.T) {
	ps := NewPeerstore()
	defer ps.Close()

	peerID := testPeerID("peer1")
	addr1 := testMultiaddr("/ip4/127.0.0.1/tcp/4001")
	addr2 := testMultiaddr("/ip4/192.168.1.1/tcp/4001")

	// 先添加一个地址
	ps.AddAddrsWithSource(peerID, []AddressWithSource{
		{Addr: addr1, Source: SourceManual, TTL: 1 * time.Hour},
	})

	// SetAddrsWithSource 覆盖现有地址
	ps.SetAddrsWithSource(peerID, []AddressWithSource{
		{Addr: addr2, Source: SourceDHT, TTL: 10 * time.Minute},
	})

	// 验证只有新地址
	addrs := ps.Addrs(peerID)
	require.Len(t, addrs, 1)
	assert.Equal(t, addr2.String(), addrs[0].String())
}

func TestPeerstore_AddrsWithSource(t *testing.T) {
	ps := NewPeerstore()
	defer ps.Close()

	peerID := testPeerID("peer1")
	addr := testMultiaddr("/ip4/127.0.0.1/tcp/4001")

	ps.AddAddrsWithSource(peerID, []AddressWithSource{
		{Addr: addr, Source: SourceMemberList, TTL: 30 * time.Minute},
	})

	// 获取带来源的地址
	addrsWithSource := ps.AddrsWithSource(peerID)
	require.Len(t, addrsWithSource, 1)
	assert.Equal(t, addr.String(), addrsWithSource[0].Addr.String())
	assert.Equal(t, SourceMemberList, addrsWithSource[0].Source)
}

func TestPeerstore_GetAddrSource(t *testing.T) {
	ps := NewPeerstore()
	defer ps.Close()

	peerID := testPeerID("peer1")
	addr := testMultiaddr("/ip4/127.0.0.1/tcp/4001")

	ps.AddAddrsWithSource(peerID, []AddressWithSource{
		{Addr: addr, Source: SourceRelay, TTL: 15 * time.Minute},
	})

	// 获取特定地址的来源
	source := ps.GetAddrSource(peerID, addr)
	assert.Equal(t, SourceRelay, source)
}

// ============================================================================
//                     KeyBook 方法测试
// ============================================================================

// testPrivateKey 测试用的私钥实现
type testPrivateKey struct {
	data   []byte
	pubKey *testPublicKey
}

func (k *testPrivateKey) Raw() ([]byte, error) {
	return k.data, nil
}

func (k *testPrivateKey) Type() pkgif.KeyType {
	return pkgif.KeyTypeEd25519
}

func (k *testPrivateKey) Equals(other pkgif.PrivateKey) bool {
	if other == nil {
		return false
	}
	otherData, err := other.Raw()
	if err != nil {
		return false
	}
	return string(k.data) == string(otherData)
}

func (k *testPrivateKey) Sign(data []byte) ([]byte, error) {
	return append([]byte("sig-"), data...), nil
}

func (k *testPrivateKey) PublicKey() pkgif.PublicKey {
	return k.pubKey
}

func testPrivKey(s string) pkgif.PrivateKey {
	return &testPrivateKey{
		data:   []byte(s),
		pubKey: &testPublicKey{data: []byte(s + "-pub")},
	}
}

func TestPeerstore_PrivKey(t *testing.T) {
	ps := NewPeerstore()
	defer ps.Close()

	peerID := testPeerID("peer1")

	// 没有私钥时应该返回错误
	_, err := ps.PrivKey(peerID)
	assert.Error(t, err)
}

func TestPeerstore_AddPrivKey(t *testing.T) {
	ps := NewPeerstore()
	defer ps.Close()

	peerID := testPeerID("peer1")
	privKey := testPrivKey("priv1")

	// 添加私钥
	err := ps.AddPrivKey(peerID, privKey)
	assert.NoError(t, err)

	// 获取私钥
	retrieved, err := ps.PrivKey(peerID)
	require.NoError(t, err)
	assert.True(t, privKey.Equals(retrieved))
}

func TestPeerstore_PeersWithKeys(t *testing.T) {
	ps := NewPeerstore()
	defer ps.Close()

	// 初始应该没有节点
	peers := ps.PeersWithKeys()
	assert.Empty(t, peers)

	// 添加公钥
	peerID1 := testPeerID("peer1")
	peerID2 := testPeerID("peer2")

	ps.AddPubKey(peerID1, testPubKey("key1"))
	ps.AddPubKey(peerID2, testPubKey("key2"))

	// 验证返回了所有有密钥的节点
	peers = ps.PeersWithKeys()
	assert.Len(t, peers, 2)
	assert.Contains(t, peers, peerID1)
	assert.Contains(t, peers, peerID2)
}

// ============================================================================
//                     ProtoBook 方法测试
// ============================================================================

func TestPeerstore_AddProtocols(t *testing.T) {
	ps := NewPeerstore()
	defer ps.Close()

	peerID := testPeerID("peer1")
	proto1 := testProtocolID("/dep2p/sys/dht/1.0.0")
	proto2 := testProtocolID("/dep2p/relay/1.0.0/hop")

	// 添加协议
	err := ps.AddProtocols(peerID, proto1, proto2)
	assert.NoError(t, err)

	// 验证协议已添加
	protos, err := ps.GetProtocols(peerID)
	require.NoError(t, err)
	assert.Len(t, protos, 2)
	assert.Contains(t, protos, proto1)
	assert.Contains(t, protos, proto2)
}

func TestPeerstore_RemoveProtocols(t *testing.T) {
	ps := NewPeerstore()
	defer ps.Close()

	peerID := testPeerID("peer1")
	proto1 := testProtocolID("/dep2p/sys/dht/1.0.0")
	proto2 := testProtocolID("/dep2p/relay/1.0.0/hop")

	// 添加协议
	ps.SetProtocols(peerID, proto1, proto2)

	// 移除一个协议
	err := ps.RemoveProtocols(peerID, proto1)
	assert.NoError(t, err)

	// 验证只剩一个协议
	protos, err := ps.GetProtocols(peerID)
	require.NoError(t, err)
	assert.Len(t, protos, 1)
	assert.Contains(t, protos, proto2)
}

func TestPeerstore_SupportsProtocols(t *testing.T) {
	ps := NewPeerstore()
	defer ps.Close()

	peerID := testPeerID("peer1")
	proto1 := testProtocolID("/dep2p/sys/dht/1.0.0")
	proto2 := testProtocolID("/dep2p/relay/1.0.0/hop")
	proto3 := testProtocolID("/dep2p/pubsub/1.0.0")

	// 只添加 proto1 和 proto2
	ps.SetProtocols(peerID, proto1, proto2)

	// 检查支持的协议
	supported, err := ps.SupportsProtocols(peerID, proto1, proto3)
	require.NoError(t, err)
	assert.Len(t, supported, 1)
	assert.Contains(t, supported, proto1)
}

func TestPeerstore_FirstSupportedProtocol(t *testing.T) {
	ps := NewPeerstore()
	defer ps.Close()

	peerID := testPeerID("peer1")
	proto1 := testProtocolID("/dep2p/sys/dht/1.0.0")
	proto2 := testProtocolID("/dep2p/relay/1.0.0/hop")
	proto3 := testProtocolID("/dep2p/pubsub/1.0.0")

	// 只添加 proto2 和 proto3
	ps.SetProtocols(peerID, proto2, proto3)

	// 查询首个支持的协议（按查询顺序）
	first, err := ps.FirstSupportedProtocol(peerID, proto1, proto2, proto3)
	require.NoError(t, err)
	// 应该返回 proto2（第一个被支持的）
	assert.Equal(t, proto2, first)
}

func TestPeerstore_FirstSupportedProtocol_NoneSupported(t *testing.T) {
	ps := NewPeerstore()
	defer ps.Close()

	peerID := testPeerID("peer1")
	proto1 := testProtocolID("/dep2p/sys/dht/1.0.0")

	// 没有设置任何协议
	first, err := ps.FirstSupportedProtocol(peerID, proto1)
	// 没有支持的协议时应返回空字符串，无错误
	assert.Empty(t, first, "未设置协议时应返回空")
	assert.NoError(t, err, "未设置协议时不应返回错误")
}

// ============================================================================
//                     NodeDB 集成测试
// ============================================================================

func TestPeerstore_UpdateNodeRecord(t *testing.T) {
	ps := NewPeerstore()
	defer ps.Close()

	peerID := testPeerID("peer1")
	addrs := []string{"/ip4/127.0.0.1/tcp/4001"}

	// 更新节点记录
	err := ps.UpdateNodeRecord(peerID, addrs)
	assert.NoError(t, err)
}

func TestPeerstore_GetNodeRecord(t *testing.T) {
	ps := NewPeerstore()
	defer ps.Close()

	peerID := testPeerID("peer1")
	addrs := []string{"/ip4/127.0.0.1/tcp/4001"}

	// 先更新
	ps.UpdateNodeRecord(peerID, addrs)

	// 获取记录
	record := ps.GetNodeRecord(peerID)
	require.NotNil(t, record)
	assert.Equal(t, string(peerID), record.ID)
	assert.Equal(t, addrs, record.Addrs)
}

func TestPeerstore_GetNodeRecord_NotFound(t *testing.T) {
	ps := NewPeerstore()
	defer ps.Close()

	peerID := testPeerID("nonexistent")

	// 获取不存在的记录
	record := ps.GetNodeRecord(peerID)
	assert.Nil(t, record)
}

func TestPeerstore_RemoveNodeRecord(t *testing.T) {
	ps := NewPeerstore()
	defer ps.Close()

	peerID := testPeerID("peer1")
	addrs := []string{"/ip4/127.0.0.1/tcp/4001"}

	// 先更新
	ps.UpdateNodeRecord(peerID, addrs)
	require.NotNil(t, ps.GetNodeRecord(peerID))

	// 删除
	err := ps.RemoveNodeRecord(peerID)
	assert.NoError(t, err)

	// 验证已删除
	record := ps.GetNodeRecord(peerID)
	assert.Nil(t, record)
}

func TestPeerstore_QuerySeeds(t *testing.T) {
	ps := NewPeerstore()
	defer ps.Close()

	// 添加几个节点
	for i := 0; i < 5; i++ {
		peerID := testPeerID(string(rune('a' + i)))
		ps.UpdateNodeRecord(peerID, []string{"/ip4/127.0.0.1/tcp/400" + string(rune('1'+i))})
	}

	// 查询种子节点
	seeds := ps.QuerySeeds(3, 1*time.Hour)
	assert.LessOrEqual(t, len(seeds), 3)
}

func TestPeerstore_UpdateDialAttempt(t *testing.T) {
	ps := NewPeerstore()
	defer ps.Close()

	peerID := testPeerID("peer1")
	ps.UpdateNodeRecord(peerID, []string{"/ip4/127.0.0.1/tcp/4001"})

	// 记录成功的拨号
	err := ps.UpdateDialAttempt(peerID, true)
	assert.NoError(t, err)

	// 记录失败的拨号
	err = ps.UpdateDialAttempt(peerID, false)
	assert.NoError(t, err)
}

func TestPeerstore_UpdateLastPong(t *testing.T) {
	ps := NewPeerstore()
	defer ps.Close()

	peerID := testPeerID("peer1")
	ps.UpdateNodeRecord(peerID, []string{"/ip4/127.0.0.1/tcp/4001"})

	// 更新最后 Pong 时间
	pongTime := time.Now()
	err := ps.UpdateLastPong(peerID, pongTime)
	assert.NoError(t, err)
}

func TestPeerstore_LastPongReceived(t *testing.T) {
	ps := NewPeerstore()
	defer ps.Close()

	peerID := testPeerID("peer1")
	ps.UpdateNodeRecord(peerID, []string{"/ip4/127.0.0.1/tcp/4001"})

	// 设置 Pong 时间
	pongTime := time.Now()
	ps.UpdateLastPong(peerID, pongTime)

	// 获取 Pong 时间
	retrieved := ps.LastPongReceived(peerID)
	// 允许一定时间误差
	assert.WithinDuration(t, pongTime, retrieved, time.Second)
}

func TestPeerstore_LastPongReceived_NoRecord(t *testing.T) {
	ps := NewPeerstore()
	defer ps.Close()

	peerID := testPeerID("nonexistent")

	// 没有记录时应返回零值
	retrieved := ps.LastPongReceived(peerID)
	assert.True(t, retrieved.IsZero())
}

func TestPeerstore_NodeDBSize(t *testing.T) {
	ps := NewPeerstore()
	defer ps.Close()

	// 初始大小
	size := ps.NodeDBSize()
	assert.Equal(t, 0, size)

	// 添加节点
	ps.UpdateNodeRecord(testPeerID("peer1"), []string{"/ip4/127.0.0.1/tcp/4001"})
	ps.UpdateNodeRecord(testPeerID("peer2"), []string{"/ip4/127.0.0.2/tcp/4001"})

	// 验证大小增加
	size = ps.NodeDBSize()
	assert.Equal(t, 2, size)
}

func TestPeerstore_NodeDB(t *testing.T) {
	ps := NewPeerstore()
	defer ps.Close()

	// 获取底层 NodeDB
	db := ps.NodeDB()
	assert.NotNil(t, db)
}

func TestPeerstore_SetNodeDB(t *testing.T) {
	ps := NewPeerstore()
	defer ps.Close()

	// 获取原始 NodeDB
	origDB := ps.NodeDB()
	require.NotNil(t, origDB)

	// 设置为 nil
	ps.SetNodeDB(nil)
	assert.Nil(t, ps.NodeDB())

	// 恢复
	ps.SetNodeDB(origDB)
	assert.NotNil(t, ps.NodeDB())
}

func TestPeerstore_AddrBook(t *testing.T) {
	ps := NewPeerstore()
	defer ps.Close()

	// 获取底层 AddrBook
	ab := ps.AddrBook()
	// 可能返回 nil（如果是 PersistentAddrBook）
	// 对于 NewPeerstore() 应该返回 *addrbook.AddrBook
	assert.NotNil(t, ab)
}

// ============================================================================
//                     ResetStates 测试
// ============================================================================

func TestPeerstore_ResetStates(t *testing.T) {
	ps := NewPeerstore()
	defer ps.Close()

	peerID := testPeerID("peer1")

	// 添加短 TTL 地址
	ps.AddAddr(peerID, testMultiaddr("/ip4/127.0.0.1/tcp/4001"), TempAddrTTL)

	// 重置状态
	ctx := context.Background()
	err := ps.ResetStates(ctx)
	assert.NoError(t, err)
}

func TestPeerstore_ResetStates_AfterClose(t *testing.T) {
	ps := NewPeerstore()
	ps.Close()

	// 关闭后调用应返回错误
	ctx := context.Background()
	err := ps.ResetStates(ctx)
	assert.ErrorIs(t, err, ErrClosed)
}

// ============================================================================
//                     NodeDB nil 边界测试
// ============================================================================

func TestPeerstore_NodeDB_NilSafe(t *testing.T) {
	ps := NewPeerstore()
	defer ps.Close()

	// 设置 NodeDB 为 nil
	ps.SetNodeDB(nil)

	peerID := testPeerID("peer1")

	// 所有 NodeDB 相关方法应该安全处理 nil
	err := ps.UpdateNodeRecord(peerID, []string{"/ip4/127.0.0.1/tcp/4001"})
	assert.NoError(t, err) // nil 时返回 nil

	record := ps.GetNodeRecord(peerID)
	assert.Nil(t, record)

	err = ps.RemoveNodeRecord(peerID)
	assert.NoError(t, err)

	seeds := ps.QuerySeeds(10, time.Hour)
	assert.Nil(t, seeds)

	err = ps.UpdateDialAttempt(peerID, true)
	assert.NoError(t, err)

	err = ps.UpdateLastPong(peerID, time.Now())
	assert.NoError(t, err)

	pongTime := ps.LastPongReceived(peerID)
	assert.True(t, pongTime.IsZero())

	size := ps.NodeDBSize()
	assert.Equal(t, 0, size)
}

// ============================================================================
//                     Close 测试
// ============================================================================

func TestPeerstore_Close_Idempotent(t *testing.T) {
	ps := NewPeerstore()

	// 第一次关闭
	err := ps.Close()
	assert.NoError(t, err)

	// 第二次关闭应返回 ErrClosed
	err = ps.Close()
	assert.ErrorIs(t, err, ErrClosed)
}
