package rendezvous

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/lib/crypto"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// TestRendezvous_Creation 测试创建
func TestRendezvous_Creation(t *testing.T) {
	host := newMockHost("test-peer")
	config := DefaultDiscovererConfig()

	discoverer := NewDiscoverer(host, config)
	require.NotNil(t, discoverer)
	assert.Equal(t, types.PeerID("test-peer"), discoverer.localID)
}

// TestRendezvous_Start 测试启动
func TestRendezvous_Start(t *testing.T) {
	host := newMockHost("test-peer")
	config := DefaultDiscovererConfig()
	discoverer := NewDiscoverer(host, config)

	ctx := context.Background()
	err := discoverer.Start(ctx)
	require.NoError(t, err)
	assert.True(t, discoverer.started.Load())
}

// TestRendezvous_Register 测试注册
func TestRendezvous_Register(t *testing.T) {
	host := newMockHost("test-peer")
	config := DefaultDiscovererConfig()
	config.Points = []types.PeerID{"point-1"}
	discoverer := NewDiscoverer(host, config)

	ctx := context.Background()
	err := discoverer.Start(ctx)
	require.NoError(t, err)

	// Register 会失败（因为 mock stream 返回 nil）
	err = discoverer.Register(ctx, "test-ns", 1*time.Hour)
	assert.Error(t, err) // 预期失败
}

// TestRendezvous_Discover 测试发现
func TestRendezvous_Discover(t *testing.T) {
	host := newMockHost("test-peer")
	config := DefaultDiscovererConfig()
	config.Points = []types.PeerID{"point-1"}
	discoverer := NewDiscoverer(host, config)

	ctx := context.Background()
	err := discoverer.Start(ctx)
	require.NoError(t, err)

	// Discover 会失败（因为 mock stream 返回 nil）
	// 根据代码分析：sendRequest 检查 stream == nil 并返回 errors.New("stream is nil")
	peers, err := discoverer.Discover(ctx, "test-ns", 10)
	assert.Error(t, err, "Discover should fail when stream is nil")
	assert.Contains(t, err.Error(), "stream is nil", "error should indicate nil stream")
	assert.Empty(t, peers, "peers should be empty on error")
}

// TestRendezvous_Advertise 测试广播
func TestRendezvous_Advertise(t *testing.T) {
	host := newMockHost("test-peer")
	config := DefaultDiscovererConfig()
	config.Points = []types.PeerID{"point-1"}
	discoverer := NewDiscoverer(host, config)

	ctx := context.Background()
	err := discoverer.Start(ctx)
	require.NoError(t, err)

	// Advertise 映射到 Register，会失败（mock）
	_, err = discoverer.Advertise(ctx, "test-ns")
	assert.Error(t, err) // 预期失败
}

// TestRendezvous_TTLExpiration 测试过期
func TestRendezvous_TTLExpiration(t *testing.T) {
	store := NewStore(DefaultStoreConfig())

	peerInfo := types.PeerInfo{
		ID:    "peer-1",
		Addrs: []types.Multiaddr{},
	}

	// 添加短 TTL 注册
	err := store.Add("test-ns", peerInfo, 100*time.Millisecond)
	require.NoError(t, err)

	// 立即查询应该能找到
	regs, _, err := store.Get("test-ns", 10, nil)
	require.NoError(t, err)
	assert.Equal(t, 1, len(regs))

	// 等待过期
	time.Sleep(150 * time.Millisecond)

	// 清理过期
	count := store.CleanupExpired()
	assert.Equal(t, 1, count)

	// 再次查询应该为空
	regs, _, err = store.Get("test-ns", 10, nil)
	require.NoError(t, err)
	assert.Equal(t, 0, len(regs))
}

// TestPoint_HandleRegister 测试服务端注册处理
func TestPoint_HandleRegister(t *testing.T) {
	host := newMockHost("point-peer")
	config := DefaultPointConfig()
	point := NewPoint(host, config)

	ctx := context.Background()
	err := point.Start(ctx)
	require.NoError(t, err)
	defer point.Stop()

	// 测试点已启动
	assert.NotNil(t, point.store)
}

// TestStore_AddGetRemove 测试存储操作
func TestStore_AddGetRemove(t *testing.T) {
	store := NewStore(DefaultStoreConfig())

	peerInfo := types.PeerInfo{
		ID:    "peer-1",
		Addrs: []types.Multiaddr{},
	}

	// 添加
	err := store.Add("test-ns", peerInfo, 1*time.Hour)
	require.NoError(t, err)

	// 查询
	regs, _, err := store.Get("test-ns", 10, nil)
	require.NoError(t, err)
	assert.Equal(t, 1, len(regs))
	assert.Equal(t, types.PeerID("peer-1"), regs[0].PeerInfo.ID)

	// 移除
	store.Remove("test-ns", "peer-1")

	// 再次查询应该为空
	regs, _, err = store.Get("test-ns", 10, nil)
	require.NoError(t, err)
	assert.Equal(t, 0, len(regs))
}

// TestConfig_Validate 测试配置验证
func TestConfig_Validate(t *testing.T) {
	config := DefaultDiscovererConfig()
	assert.Greater(t, config.DefaultTTL, time.Duration(0))
	assert.Greater(t, config.RenewalInterval, time.Duration(0))

	pointConfig := DefaultPointConfig()
	assert.Greater(t, pointConfig.MaxRegistrations, 0)
	assert.Greater(t, pointConfig.MaxNamespaces, 0)
}

// TestLifecycle 测试完整生命周期
func TestLifecycle(t *testing.T) {
	host := newMockHost("test-peer")
	config := DefaultDiscovererConfig()
	discoverer := NewDiscoverer(host, config)

	ctx := context.Background()

	// 启动
	err := discoverer.Start(ctx)
	require.NoError(t, err)
	assert.True(t, discoverer.started.Load())

	// 重复启动应该失败
	err = discoverer.Start(ctx)
	assert.Error(t, err)

	// 停止
	err = discoverer.Stop(ctx)
	require.NoError(t, err)
	assert.False(t, discoverer.started.Load())
}

// ============================================================================
//                              SignedPeerRecord 测试
// ============================================================================

// TestSignedPeerRecord_CreateAndVerify 测试签名创建和验证
func TestSignedPeerRecord_CreateAndVerify(t *testing.T) {
	// 生成密钥对
	privKey, _, err := crypto.GenerateKeyPair(crypto.KeyTypeEd25519)
	require.NoError(t, err)

	// 创建 PeerRecord
	record := &PeerRecord{
		PeerID:    "test-peer-id",
		Addrs:     []types.Multiaddr{},
		Seq:       1,
		Timestamp: time.Now(),
	}

	// 签名
	signed, err := SignPeerRecord(privKey, record)
	require.NoError(t, err)
	require.NotNil(t, signed)

	// 验证
	valid, err := VerifySignedPeerRecord(signed)
	require.NoError(t, err)
	assert.True(t, valid)
}

// TestSignedPeerRecord_MarshalUnmarshal 测试序列化反序列化
func TestSignedPeerRecord_MarshalUnmarshal(t *testing.T) {
	privKey, _, err := crypto.GenerateKeyPair(crypto.KeyTypeEd25519)
	require.NoError(t, err)

	record := &PeerRecord{
		PeerID:    "test-peer",
		Addrs:     []types.Multiaddr{},
		Seq:       42,
		Timestamp: time.Now(),
	}

	signed, err := SignPeerRecord(privKey, record)
	require.NoError(t, err)

	// 序列化
	data, err := signed.Marshal()
	require.NoError(t, err)

	// 反序列化
	decoded, err := UnmarshalSignedPeerRecord(data)
	require.NoError(t, err)

	assert.Equal(t, record.PeerID, decoded.PeerRecord.PeerID)
	assert.Equal(t, record.Seq, decoded.PeerRecord.Seq)

	// 验证反序列化后的签名
	valid, err := VerifySignedPeerRecord(decoded)
	require.NoError(t, err)
	assert.True(t, valid)
}

// TestPeerRecordManager 测试 PeerRecord 管理器
func TestPeerRecordManager(t *testing.T) {
	privKey, _, err := crypto.GenerateKeyPair(crypto.KeyTypeEd25519)
	require.NoError(t, err)

	manager := NewPeerRecordManager(privKey, "my-peer-id")

	// 创建第一个记录
	signed1, err := manager.CreateSignedRecord(nil)
	require.NoError(t, err)
	assert.Equal(t, uint64(1), signed1.PeerRecord.Seq)

	// 创建第二个记录，序列号应该递增
	signed2, err := manager.CreateSignedRecord(nil)
	require.NoError(t, err)
	assert.Equal(t, uint64(2), signed2.PeerRecord.Seq)
}

// ============================================================================
//                              PersistentStore 测试
// ============================================================================

// TestPersistentStore_AddAndGet 测试持久化存储
func TestPersistentStore_AddAndGet(t *testing.T) {
	// 创建临时目录
	tmpDir := t.TempDir()

	config := DefaultPersistentStoreConfig()
	config.DataDir = tmpDir
	config.SaveInterval = 0 // 禁用自动保存

	store, err := NewPersistentStore(config)
	require.NoError(t, err)
	defer store.Close()

	peerInfo := types.PeerInfo{
		ID:    "peer-1",
		Addrs: []types.Multiaddr{},
	}

	// 添加
	err = store.Add("test-ns", peerInfo, 1*time.Hour)
	require.NoError(t, err)

	// 查询
	regs, _, err := store.Get("test-ns", 10, nil)
	require.NoError(t, err)
	assert.Equal(t, 1, len(regs))

	// 保存
	err = store.Save()
	require.NoError(t, err)
}

// TestPersistentStore_Reload 测试重新加载
func TestPersistentStore_Reload(t *testing.T) {
	tmpDir := t.TempDir()

	config := DefaultPersistentStoreConfig()
	config.DataDir = tmpDir
	config.SaveInterval = 0

	// 第一个实例
	store1, err := NewPersistentStore(config)
	require.NoError(t, err)

	peerInfo := types.PeerInfo{
		ID:    "peer-reload",
		Addrs: []types.Multiaddr{},
	}

	err = store1.Add("reload-ns", peerInfo, 1*time.Hour)
	require.NoError(t, err)
	err = store1.Save()
	require.NoError(t, err)
	store1.Close()

	// 第二个实例应该加载数据
	store2, err := NewPersistentStore(config)
	require.NoError(t, err)
	defer store2.Close()

	regs, _, err := store2.Get("reload-ns", 10, nil)
	require.NoError(t, err)
	assert.Equal(t, 1, len(regs))
	assert.Equal(t, types.PeerID("peer-reload"), regs[0].PeerInfo.ID)
}

// ============================================================================
//                              PointDiscovery 测试
// ============================================================================

// TestPointDiscovery_Creation 测试 PointDiscovery 创建
func TestPointDiscovery_Creation(t *testing.T) {
	host := newMockHost("test-peer")
	config := DefaultPointDiscoveryConfig()

	pd := NewPointDiscovery(nil, host, config)
	require.NotNil(t, pd)

	points := pd.GetPoints()
	assert.Equal(t, 0, len(points))
}

// ============================================================================
//                              Mock 实现
// ============================================================================

// mockHost 模拟Host接口
type mockHost struct {
	id     string
	addrs  []string
	closed bool
}

func newMockHost(id string) *mockHost {
	return &mockHost{
		id:    id,
		addrs: []string{"/ip4/127.0.0.1/tcp/4001"},
	}
}

func (m *mockHost) ID() string {
	return m.id
}

func (m *mockHost) Addrs() []string {
	return m.addrs
}

func (m *mockHost) Listen(addrs ...string) error {
	return nil
}

func (m *mockHost) Connect(ctx context.Context, peerID string, addrs []string) error {
	return nil
}

func (m *mockHost) SetStreamHandler(protocolID string, handler pkgif.StreamHandler) {
}

func (m *mockHost) RemoveStreamHandler(protocolID string) {
}

func (m *mockHost) NewStream(ctx context.Context, peerID string, protocolIDs ...string) (pkgif.Stream, error) {
	return nil, nil
}

func (m *mockHost) NewStreamWithPriority(ctx context.Context, peerID string, protocolID string, priority int) (pkgif.Stream, error) {
	return m.NewStream(ctx, peerID, protocolID)
}

func (m *mockHost) Peerstore() pkgif.Peerstore {
	return nil
}

func (m *mockHost) EventBus() pkgif.EventBus {
	return nil
}

func (m *mockHost) Close() error {
	m.closed = true
	return nil
}

func (m *mockHost) AdvertisedAddrs() []string {
	return m.Addrs()
}

func (m *mockHost) ShareableAddrs() []string {
	return nil
}

func (m *mockHost) HolePunchAddrs() []string {
	return nil
}

func (m *mockHost) SetReachabilityCoordinator(coordinator pkgif.ReachabilityCoordinator) {
	// no-op for mock
}

func (m *mockHost) Network() pkgif.Swarm {
	return nil
}

func (m *mockHost) HandleInboundStream(stream pkgif.Stream) {
	// Mock implementation: no-op
}
