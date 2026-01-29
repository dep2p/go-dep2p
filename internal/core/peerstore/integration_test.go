package peerstore

import (
	"context"
	"testing"
	"time"

	"github.com/dep2p/go-dep2p/config"
	"github.com/dep2p/go-dep2p/internal/core/storage"
	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"
	"go.uber.org/fx/fxtest"
)

func TestPeerstoreFxModule(t *testing.T) {
	// 创建临时目录和配置
	tmpDir := t.TempDir()
	cfg := config.NewConfig()
	cfg.Storage.DataDir = tmpDir

	app := fxtest.New(t,
		fx.Supply(cfg),
		storage.Module(),
		Module(),
		fx.Invoke(func(ps pkgif.Peerstore) {
			// 验证依赖注入成功
			require.NotNil(t, ps)
			// 验证接口方法可用
			peers := ps.Peers()
			require.NotNil(t, peers)
		}),
	)

	defer app.RequireStart().RequireStop()
}

func TestPeerstoreLifecycle(t *testing.T) {
	// 创建临时目录和配置
	tmpDir := t.TempDir()
	cfg := config.NewConfig()
	cfg.Storage.DataDir = tmpDir

	var ps pkgif.Peerstore

	app := fxtest.New(t,
		fx.Supply(cfg),
		storage.Module(),
		Module(),
		fx.Populate(&ps),
	)

	ctx := context.Background()
	app.RequireStart()

	// 在运行时添加一些数据
	peerID := testPeerID("peer1")
	ps.AddAddrs(peerID, []types.Multiaddr{testMultiaddr("/ip4/127.0.0.1/tcp/4001")}, ConnectedAddrTTL)

	// 验证数据存在
	addrs := ps.Addrs(peerID)
	require.Len(t, addrs, 1)

	// 停止应用
	require.NoError(t, app.Stop(ctx))
}

func TestPeerstore_MultiSubbookIntegration(t *testing.T) {
	ps := NewPeerstore()
	defer ps.Close()

	peerID := testPeerID("peer1")

	// 添加地址
	addrs := []types.Multiaddr{
		testMultiaddr("/ip4/127.0.0.1/tcp/4001"),
		testMultiaddr("/ip4/192.168.1.1/tcp/4001"),
	}
	ps.AddAddrs(peerID, addrs, ConnectedAddrTTL)

	// 添加公钥
	pubKey := testPubKey("key1")
	err := ps.AddPubKey(peerID, pubKey)
	require.NoError(t, err)

	// 添加协议
	err = ps.SetProtocols(peerID, types.ProtocolID("/dep2p/sys/dht/1.0.0"))
	require.NoError(t, err)

	// 添加元数据
	err = ps.Put(peerID, "agent", "dep2p/v1.0.0")
	require.NoError(t, err)

	// 验证 PeerInfo
	info := ps.PeerInfo(peerID)
	require.Equal(t, peerID, info.ID)
	require.Len(t, info.Addrs, 2)

	// 验证所有数据都能查询到
	retrievedAddrs := ps.Addrs(peerID)
	require.Len(t, retrievedAddrs, 2)

	retrievedKey, err := ps.PubKey(peerID)
	require.NoError(t, err)
	require.True(t, pubKey.Equals(retrievedKey))

	protocols, err := ps.GetProtocols(peerID)
	require.NoError(t, err)
	require.Len(t, protocols, 1)

	agent, err := ps.Get(peerID, "agent")
	require.NoError(t, err)
	require.Equal(t, "dep2p/v1.0.0", agent)
}

func TestPeerstore_GCIntegration(t *testing.T) {
	// 创建 Peerstore
	ps := NewPeerstore()
	defer ps.Close()

	peerID := types.PeerID("gc-integration-peer")
	addr, _ := types.NewMultiaddr("/ip4/127.0.0.1/tcp/4001")

	// 添加短 TTL 地址
	ps.AddAddr(peerID, addr, 1*time.Millisecond)

	// 验证地址存在
	addrs := ps.Addrs(peerID)
	assert.Len(t, addrs, 1, "地址应该被添加")

	// 等待过期
	time.Sleep(10 * time.Millisecond)

	// 触发 GC（如果 Peerstore 有 GC 方法）
	// 或直接检查地址是否仍存在

	// 验证地址已过期（获取时会自动过滤过期地址）
	addrs = ps.Addrs(peerID)
	// 过期后应该返回空列表
	assert.Empty(t, addrs, "过期地址应被清理")
}
