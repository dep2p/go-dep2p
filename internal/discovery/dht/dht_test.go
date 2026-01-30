package dht

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

// TestDHT_Creation 测试DHT创建
func TestDHT_Creation(t *testing.T) {
	host := newMockHost("test-peer")

	dht, err := New(host, nil)
	require.NoError(t, err)
	require.NotNil(t, dht)

	assert.Equal(t, types.NodeID("test-peer"), types.NodeID(dht.host.ID()))
	assert.NotNil(t, dht.routingTable)
	assert.NotNil(t, dht.valueStore)
	assert.NotNil(t, dht.providerStore)
}

// TestDHT_Start 测试DHT启动
func TestDHT_Start(t *testing.T) {
	host := newMockHost("test-peer")
	dht, err := New(host, nil)
	require.NoError(t, err)

	ctx := context.Background()
	err = dht.Start(ctx)
	require.NoError(t, err)
	assert.True(t, dht.started.Load())
}

// TestDHT_Stop 测试DHT停止
func TestDHT_Stop(t *testing.T) {
	host := newMockHost("test-peer")
	dht, err := New(host, nil)
	require.NoError(t, err)

	ctx := context.Background()
	err = dht.Start(ctx)
	require.NoError(t, err)

	err = dht.Stop(ctx)
	require.NoError(t, err)
	assert.False(t, dht.started.Load())
}

// TestDHT_Bootstrap 测试引导流程
func TestDHT_Bootstrap(t *testing.T) {
	host := newMockHost("test-peer")

	// 创建带引导节点的配置
	config := DefaultConfig()
	config.BootstrapPeers = []types.PeerInfo{
		{
			ID:    "bootstrap-peer",
			Addrs: []types.Multiaddr{},
		},
	}

	dht, err := New(host, nil, WithBootstrapPeers(config.BootstrapPeers))
	require.NoError(t, err)

	ctx := context.Background()
	err = dht.Start(ctx)
	require.NoError(t, err)

	err = dht.Bootstrap(ctx)
	// Bootstrap 在 started=true 时总是返回 nil（连接失败只是 continue，不返回错误）
	// 代码位置: dht.go:577-597
	require.NoError(t, err, "Bootstrap should succeed even if connections fail")
}

// TestDHT_FindPeer 测试查找节点
func TestDHT_FindPeer(t *testing.T) {
	host := newMockHost("test-peer")
	dht, err := New(host, nil)
	require.NoError(t, err)

	ctx := context.Background()
	err = dht.Start(ctx)
	require.NoError(t, err)

	// 测试查找不存在的节点
	_, err = dht.FindPeer(ctx, "unknown-peer")
	assert.Error(t, err)
}

// TestDHT_FindPeers 测试发现节点
func TestDHT_FindPeers(t *testing.T) {
	host := newMockHost("test-peer")
	dht, err := New(host, nil)
	require.NoError(t, err)

	ctx := context.Background()
	err = dht.Start(ctx)
	require.NoError(t, err)

	ch, err := dht.FindPeers(ctx, "test-namespace")
	require.NoError(t, err)

	// 收集结果
	var peers []types.PeerInfo
	for peer := range ch {
		peers = append(peers, peer)
	}

	// 初始状态应该为空
	assert.Equal(t, 0, len(peers))
}

// TestDHT_Advertise 测试广播
func TestDHT_Advertise(t *testing.T) {
	host := newMockHost("test-peer")
	dht, err := New(host, nil)
	require.NoError(t, err)

	ctx := context.Background()
	err = dht.Start(ctx)
	require.NoError(t, err)

	ttl, err := dht.Advertise(ctx, "test-namespace")
	require.NoError(t, err)
	assert.Greater(t, ttl, int64(0))
}

// TestDHT_Lifecycle 测试生命周期
func TestDHT_Lifecycle(t *testing.T) {
	host := newMockHost("test-peer")
	dht, err := New(host, nil)
	require.NoError(t, err)

	ctx := context.Background()

	// 启动
	err = dht.Start(ctx)
	require.NoError(t, err)
	assert.True(t, dht.started.Load())

	// 重复启动应该失败
	err = dht.Start(ctx)
	assert.Error(t, err)

	// 停止
	err = dht.Stop(ctx)
	require.NoError(t, err)
	assert.False(t, dht.started.Load())
}

// TestDHT_Concurrent 测试并发安全
func TestDHT_Concurrent(t *testing.T) {
	host := newMockHost("test-peer")
	dht, err := New(host, nil)
	require.NoError(t, err)

	ctx := context.Background()
	err = dht.Start(ctx)
	require.NoError(t, err)
	defer dht.Stop(ctx)

	// 并发访问路由表（测试 Race 安全性）
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func() {
			size := dht.routingTable.Size()
			// Size() 应该返回有效值
			assert.GreaterOrEqual(t, size, 0, "Size should return non-negative value")
			done <- true
		}()
	}

	for i := 0; i < 10; i++ {
		<-done
	}
}

// TestDHT_Config 测试配置
func TestDHT_Config(t *testing.T) {
	config := DefaultConfig()
	assert.Equal(t, 20, config.BucketSize)
	assert.Equal(t, 5, config.Alpha) // v2.0.1: Alpha 从 3 增加到 5

	err := config.Validate()
	assert.NoError(t, err)

	// 测试无效配置
	config.BucketSize = 0
	err = config.Validate()
	assert.Error(t, err)
}

// ============================================================================
// 补充测试
// ============================================================================

// TestDHT_Creation_NilHost 测试 nil host 创建
func TestDHT_Creation_NilHost(t *testing.T) {
	_, err := New(nil, nil)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrNilHost)
}

// TestDHT_GetValue 测试获取值
func TestDHT_GetValue(t *testing.T) {
	host := newMockHost("test-peer")
	dht, err := New(host, nil)
	require.NoError(t, err)

	ctx := context.Background()
	err = dht.Start(ctx)
	require.NoError(t, err)
	defer dht.Stop(ctx)

	// 先存储值
	key := "test-key"
	value := []byte("test-value")
	dht.valueStore.Put(key, value, dht.config.MaxRecordAge)

	// 测试获取存在的值
	got, err := dht.GetValue(ctx, key)
	require.NoError(t, err)
	assert.Equal(t, value, got)

	// 测试获取不存在的值
	_, err = dht.GetValue(ctx, "non-existent-key")
	assert.Error(t, err)
}

// TestDHT_PutValue 测试存储值
func TestDHT_PutValue(t *testing.T) {
	host := newMockHost("test-peer")
	dht, err := New(host, nil)
	require.NoError(t, err)

	ctx := context.Background()
	err = dht.Start(ctx)
	require.NoError(t, err)
	defer dht.Stop(ctx)

	key := "test-key"
	value := []byte("test-value")

	// 存储值
	err = dht.PutValue(ctx, key, value)
	require.NoError(t, err)

	// 验证本地存储
	got, ok := dht.valueStore.Get(key)
	assert.True(t, ok)
	assert.Equal(t, value, got)
}

// TestDHT_Provide 测试 Provider 注册
func TestDHT_Provide(t *testing.T) {
	host := newMockHost("test-peer")
	dht, err := New(host, nil)
	require.NoError(t, err)

	ctx := context.Background()
	err = dht.Start(ctx)
	require.NoError(t, err)
	defer dht.Stop(ctx)

	key := "test-content"

	// 注册 Provider
	err = dht.Provide(ctx, key, false)
	require.NoError(t, err)

	// 验证本地存储
	// 注意：Provide 内部会使用 normalizeProviderKey 转换 key
	// 需要使用相同的转换后的 key 进行查询
	normalizedKey := dht.normalizeProviderKey(key)
	providers := dht.providerStore.GetProviders(normalizedKey)
	assert.Len(t, providers, 1)
	assert.Equal(t, types.PeerID("test-peer"), providers[0].PeerID)
}

// TestDHT_FindProviders 测试查找 Providers
func TestDHT_FindProviders(t *testing.T) {
	host := newMockHost("test-peer")
	dht, err := New(host, nil)
	require.NoError(t, err)

	ctx := context.Background()
	err = dht.Start(ctx)
	require.NoError(t, err)
	defer dht.Stop(ctx)

	key := "test-content"

	// 先注册 Provider
	err = dht.Provide(ctx, key, false)
	require.NoError(t, err)

	// 查找 Providers
	ch, err := dht.FindProviders(ctx, key)
	require.NoError(t, err)

	var providers []types.PeerInfo
	for p := range ch {
		providers = append(providers, p)
	}

	assert.Len(t, providers, 1)
	assert.Equal(t, types.PeerID("test-peer"), providers[0].ID)
}

// TestDHT_SetEventBus 测试设置事件总线
func TestDHT_SetEventBus(t *testing.T) {
	host := newMockHost("test-peer")
	dht, err := New(host, nil)
	require.NoError(t, err)

	// 初始为 nil
	assert.Nil(t, dht.eventBus)

	// 设置 EventBus
	mockEB := &mockEventBus{}
	dht.SetEventBus(mockEB)

	assert.Equal(t, mockEB, dht.eventBus)
}

// TestDHT_RoutingTable 测试路由表接口
func TestDHT_RoutingTable(t *testing.T) {
	host := newMockHost("test-peer")
	dht, err := New(host, nil)
	require.NoError(t, err)

	rt := dht.RoutingTable()
	require.NotNil(t, rt)

	// 初始大小为 0
	assert.Equal(t, 0, rt.Size())
}

// TestDHT_NotStarted 测试未启动时调用方法
func TestDHT_NotStarted(t *testing.T) {
	host := newMockHost("test-peer")
	dht, err := New(host, nil)
	require.NoError(t, err)

	ctx := context.Background()

	// GetValue 应该失败
	_, err = dht.GetValue(ctx, "key")
	assert.ErrorIs(t, err, ErrNotStarted)

	// PutValue 应该失败
	err = dht.PutValue(ctx, "key", []byte("value"))
	assert.ErrorIs(t, err, ErrNotStarted)

	// FindPeer 应该失败
	_, err = dht.FindPeer(ctx, "peer")
	assert.ErrorIs(t, err, ErrNotStarted)

	// Provide 应该失败
	err = dht.Provide(ctx, "key", false)
	assert.ErrorIs(t, err, ErrNotStarted)

	// FindProviders 应该失败
	_, err = dht.FindProviders(ctx, "key")
	assert.ErrorIs(t, err, ErrNotStarted)

	// Bootstrap 应该失败
	err = dht.Bootstrap(ctx)
	assert.ErrorIs(t, err, ErrNotStarted)

	// Stop 应该失败
	err = dht.Stop(ctx)
	assert.ErrorIs(t, err, ErrNotStarted)
}

// TestDHT_UpdateAddrs 测试更新地址
func TestDHT_UpdateAddrs(t *testing.T) {
	host := newMockHost("test-peer")
	dht, err := New(host, nil)
	require.NoError(t, err)

	ctx := context.Background()
	err = dht.Start(ctx)
	require.NoError(t, err)
	defer dht.Stop(ctx)

	// 先注册 Provider
	key := "test-content"
	err = dht.Provide(ctx, key, false)
	require.NoError(t, err)

	// 更新地址
	host.addrs = []string{"/ip4/192.168.1.1/tcp/4001"}
	err = dht.UpdateAddrs(ctx)
	require.NoError(t, err)
}

// TestDHT_InvalidConfig 测试无效配置
func TestDHT_InvalidConfig(t *testing.T) {
	host := newMockHost("test-peer")

	// Alpha 为 0
	_, err := New(host, nil, func(c *Config) { c.Alpha = 0 })
	assert.Error(t, err)
}

// mockEventBus 模拟 EventBus
type mockEventBus struct{}

func (m *mockEventBus) Emitter(eventType interface{}, opts ...pkgif.EmitterOpt) (pkgif.Emitter, error) {
	return nil, nil
}

func (m *mockEventBus) Subscribe(eventType interface{}, opts ...pkgif.SubscriptionOpt) (pkgif.Subscription, error) {
	return nil, nil
}

func (m *mockEventBus) GetAllEventTypes() []interface{} {
	return nil
}

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

// ============================================================================
//                              v2.0 测试：Realm 成员发现与地址查询
// ============================================================================

// TestDHT_ProvideRealmMembership 测试发布 Realm 成员 Provider Record
func TestDHT_ProvideRealmMembership(t *testing.T) {
	host := newMockHost("test-peer")
	dht, err := New(host, nil)
	require.NoError(t, err)

	ctx := context.Background()
	err = dht.Start(ctx)
	require.NoError(t, err)
	defer dht.Stop(ctx)

	// 发布 Provider Record
	testRealmID := types.RealmID("test-realm-id")
	err = dht.ProvideRealmMembership(ctx, testRealmID)
	// 由于底层 DHT 是 mock，这里测试接口调用是否正常
	// 实际功能需要集成测试验证
	assert.NoError(t, err, "ProvideRealmMembership should not return error on mock DHT")
}

// TestDHT_ProvideRealmMembership_NotStarted 测试未启动时调用
func TestDHT_ProvideRealmMembership_NotStarted(t *testing.T) {
	host := newMockHost("test-peer")
	dht, err := New(host, nil)
	require.NoError(t, err)

	// 不启动 DHT，直接调用
	ctx := context.Background()
	testRealmID := types.RealmID("test-realm-id")
	err = dht.ProvideRealmMembership(ctx, testRealmID)
	assert.Error(t, err, "expected error when DHT is not started")
}

// TestDHT_FindRealmMembers 测试查找 Realm 成员
func TestDHT_FindRealmMembers(t *testing.T) {
	host := newMockHost("test-peer")
	dht, err := New(host, nil)
	require.NoError(t, err)

	ctx := context.Background()
	err = dht.Start(ctx)
	require.NoError(t, err)
	defer dht.Stop(ctx)

	// 查找成员
	testRealmID := types.RealmID("test-realm-id")
	memberCh, err := dht.FindRealmMembers(ctx, testRealmID)
	assert.NoError(t, err, "FindRealmMembers should not return error on mock DHT")
	assert.NotNil(t, memberCh, "expected non-nil channel")

	// 读取并关闭通道
	go func() {
		for range memberCh {
			// 消费通道数据
		}
	}()
}

// TestDHT_FindRealmMembers_NotStarted 测试未启动时调用
func TestDHT_FindRealmMembers_NotStarted(t *testing.T) {
	host := newMockHost("test-peer")
	dht, err := New(host, nil)
	require.NoError(t, err)

	// 不启动 DHT，直接调用
	ctx := context.Background()
	testRealmID := types.RealmID("test-realm-id")
	_, err = dht.FindRealmMembers(ctx, testRealmID)
	assert.Error(t, err, "expected error when DHT is not started")
}

// TestDHT_PublishRealmPeerRecord_v2 测试发布 Realm 成员地址（v2 API）
func TestDHT_PublishRealmPeerRecord_v2(t *testing.T) {
	host := newMockHost("test-peer")
	dht, err := New(host, nil)
	require.NoError(t, err)

	ctx := context.Background()
	err = dht.Start(ctx)
	require.NoError(t, err)
	defer dht.Stop(ctx)

	// 发布 PeerRecord
	testRealmID := types.RealmID("test-realm-id")
	testRecord := []byte(`{"peer_id":"test-peer","addrs":["/ip4/127.0.0.1/tcp/1234"]}`)
	err = dht.PublishRealmPeerRecord(ctx, testRealmID, testRecord)
	assert.NoError(t, err, "PublishRealmPeerRecord should not return error on mock DHT")
}

// TestDHT_FindRealmPeerRecord 测试查询 Realm 成员地址
func TestDHT_FindRealmPeerRecord(t *testing.T) {
	host := newMockHost("test-peer")
	dht, err := New(host, nil)
	require.NoError(t, err)

	ctx := context.Background()
	err = dht.Start(ctx)
	require.NoError(t, err)
	defer dht.Stop(ctx)

	// 查询 PeerRecord
	testRealmID := types.RealmID("test-realm-id")
	testNodeID := types.NodeID("target-node")
	record, err := dht.FindRealmPeerRecord(ctx, testRealmID, testNodeID)
	// 由于底层 DHT 是 mock，GetValue 返回空，这里验证接口调用
	// 实际功能需要集成测试验证
	if err != nil {
		assert.Contains(t, err.Error(), "failed to find realm peer record")
	} else {
		// 如果没有错误，record 应该是有效的（mock 实现可能返回空）
		_ = record
	}
}

// TestDHT_PublishRealmPeerRecord_Success 测试 Realm PeerRecord 发布流程（v2 API）
//
// v2.0 重构：使用新的 PublishRealmPeerRecord(ctx, realmID, record) 签名
func TestDHT_PublishRealmPeerRecord_Success(t *testing.T) {
	host := newMockHost("test-peer")
	dht, err := New(host, nil)
	require.NoError(t, err)

	ctx := context.Background()
	err = dht.Start(ctx)
	require.NoError(t, err)
	defer dht.Stop(ctx)

	// 发布 Realm PeerRecord（v2 API）
	testRealmID := types.RealmID("test-realm-for-publish")
	testRecord := []byte(`{"peer_id":"test-peer","addrs":["/ip4/127.0.0.1/tcp/1234"]}`)
	err = dht.PublishRealmPeerRecord(ctx, testRealmID, testRecord)
	// 在 mock 环境下可能成功（PutValue 不返回错误）
	assert.NoError(t, err, "publish should not return error")
}

// TestDHT_UnpublishPeerRecord 测试取消发布 PeerRecord
//
// Phase D Step D3 对齐：验证优雅关闭时取消发布功能
func TestDHT_UnpublishPeerRecord(t *testing.T) {
	host := newMockHost("test-peer")
	dht, err := New(host, nil)
	require.NoError(t, err)

	ctx := context.Background()
	err = dht.Start(ctx)
	require.NoError(t, err)
	defer dht.Stop(ctx)

	// 初始化 LocalRecordManager
	privKey, pubKey, err := crypto.GenerateKeyPair(crypto.KeyTypeEd25519)
	require.NoError(t, err)
	err = dht.InitializeLocalRecordManager(privKey, "")
	require.NoError(t, err)

	// v2.0: 直接设置 RealmID（不使用已删除的 SetRealmForPublish）
	testRealmID := types.RealmID("test-realm-for-unpublish")
	dht.localRecordManager.SetRealmID(testRealmID)

	// 验证初始状态
	assert.True(t, dht.localRecordManager.IsInitialized())

	// 手动添加一些记录到 peerRecordStore（模拟已发布状态）
	nodeID := dht.localRecordManager.NodeID()
	globalKey := GlobalPeerKey(nodeID)
	realmKey := RealmPeerKey(testRealmID, nodeID)

	// 创建一个 mock 记录（使用真实的 PublicKey）
	record := &RealmPeerRecord{
		NodeID:    nodeID,
		RealmID:   testRealmID,
		Seq:       1,
		Timestamp: time.Now().UnixNano(),
		TTL:       int64(time.Hour / time.Millisecond),
	}
	signed := &SignedRealmPeerRecord{
		Record:    record,
		Signature: []byte("mock-signature"),
		PublicKey: pubKey,
	}

	// 直接存入 peerRecordStore（绕过验证）
	dht.peerRecordStore.mu.Lock()
	dht.peerRecordStore.records[globalKey] = signed
	dht.peerRecordStore.records[realmKey] = signed
	dht.peerRecordStore.mu.Unlock()

	// 验证记录存在
	_, exists := dht.peerRecordStore.GetWithExpired(globalKey)
	assert.True(t, exists, "global key should exist before unpublish")
	_, exists = dht.peerRecordStore.GetWithExpired(realmKey)
	assert.True(t, exists, "realm key should exist before unpublish")

	// 取消发布
	err = dht.UnpublishPeerRecord(ctx)
	assert.NoError(t, err)

	// 验证 LocalRecordManager 已清理
	assert.False(t, dht.localRecordManager.IsInitialized(), "manager should not be initialized after unpublish")

	// 验证记录已删除
	_, exists = dht.peerRecordStore.GetWithExpired(globalKey)
	assert.False(t, exists, "global key should not exist after unpublish")
	_, exists = dht.peerRecordStore.GetWithExpired(realmKey)
	assert.False(t, exists, "realm key should not exist after unpublish")
}

// TestDHT_UnpublishPeerRecord_NotInitialized 测试未初始化时取消发布
func TestDHT_UnpublishPeerRecord_NotInitialized(t *testing.T) {
	host := newMockHost("test-peer")
	dht, err := New(host, nil)
	require.NoError(t, err)

	ctx := context.Background()
	err = dht.Start(ctx)
	require.NoError(t, err)
	defer dht.Stop(ctx)

	// 不初始化 LocalRecordManager，直接取消发布
	// 应该不报错（幂等操作）
	err = dht.UnpublishPeerRecord(ctx)
	assert.NoError(t, err, "unpublish should not error when not initialized")
}

// ============================================================================
//                              私钥类型转换测试
// ============================================================================

// mockPkgifPrivateKey 模拟 pkgif.PrivateKey 接口
// 用于测试 InitializeLocalRecordManager 的类型转换
type mockPkgifPrivateKey struct {
	raw     []byte
	keyType pkgif.KeyType
	pubKey  pkgif.PublicKey
}

func (m *mockPkgifPrivateKey) Raw() ([]byte, error) {
	return m.raw, nil
}

func (m *mockPkgifPrivateKey) Type() pkgif.KeyType {
	return m.keyType
}

func (m *mockPkgifPrivateKey) PublicKey() pkgif.PublicKey {
	return m.pubKey
}

func (m *mockPkgifPrivateKey) Equals(other pkgif.PrivateKey) bool {
	if other == nil {
		return false
	}
	otherRaw, err := other.Raw()
	if err != nil {
		return false
	}
	return string(m.raw) == string(otherRaw)
}

func (m *mockPkgifPrivateKey) Sign(data []byte) ([]byte, error) {
	// 对于测试，签名功能由底层 crypto 包处理
	// 这里只是模拟接口
	return nil, nil
}

// mockPkgifPublicKey 模拟 pkgif.PublicKey 接口
type mockPkgifPublicKey struct {
	raw     []byte
	keyType pkgif.KeyType
}

func (m *mockPkgifPublicKey) Raw() ([]byte, error) {
	return m.raw, nil
}

func (m *mockPkgifPublicKey) Type() pkgif.KeyType {
	return m.keyType
}

func (m *mockPkgifPublicKey) Equals(other pkgif.PublicKey) bool {
	if other == nil {
		return false
	}
	otherRaw, err := other.Raw()
	if err != nil {
		return false
	}
	return string(m.raw) == string(otherRaw)
}

func (m *mockPkgifPublicKey) Verify(data, sig []byte) (bool, error) {
	return true, nil
}

// TestDHT_InitializeLocalRecordManager_PkgifPrivateKey 测试从 pkgif.PrivateKey 初始化
func TestDHT_InitializeLocalRecordManager_PkgifPrivateKey(t *testing.T) {
	host := newMockHost("test-peer")
	dht, err := New(host, nil)
	require.NoError(t, err)

	ctx := context.Background()
	err = dht.Start(ctx)
	require.NoError(t, err)
	defer dht.Stop(ctx)

	// 生成真实的私钥
	realPrivKey, realPubKey, err := crypto.GenerateKeyPair(crypto.KeyTypeEd25519)
	require.NoError(t, err)

	// 获取原始字节
	privRaw, err := realPrivKey.Raw()
	require.NoError(t, err)
	pubRaw, err := realPubKey.Raw()
	require.NoError(t, err)

	// 创建模拟的 pkgif.PrivateKey（模拟 identity.privateKeyAdapter）
	mockPrivKey := &mockPkgifPrivateKey{
		raw:     privRaw,
		keyType: pkgif.KeyTypeEd25519,
		pubKey: &mockPkgifPublicKey{
			raw:     pubRaw,
			keyType: pkgif.KeyTypeEd25519,
		},
	}

	// 使用 pkgif.PrivateKey 初始化
	err = dht.InitializeLocalRecordManager(mockPrivKey, "test-realm")
	require.NoError(t, err, "InitializeLocalRecordManager should accept pkgif.PrivateKey")

	// 验证初始化成功
	assert.True(t, dht.localRecordManager.IsInitialized())

	// 验证 Realm 设置正确
	realmID, hasRealm := dht.localRecordManager.RealmID()
	assert.True(t, hasRealm)
	assert.Equal(t, types.RealmID("test-realm"), realmID)
}

// TestDHT_InitializeLocalRecordManager_CryptoPrivateKey 测试从 crypto.PrivateKey 初始化
func TestDHT_InitializeLocalRecordManager_CryptoPrivateKey(t *testing.T) {
	host := newMockHost("test-peer")
	dht, err := New(host, nil)
	require.NoError(t, err)

	ctx := context.Background()
	err = dht.Start(ctx)
	require.NoError(t, err)
	defer dht.Stop(ctx)

	// 生成私钥
	privKey, _, err := crypto.GenerateKeyPair(crypto.KeyTypeEd25519)
	require.NoError(t, err)

	// 使用 crypto.PrivateKey 初始化
	err = dht.InitializeLocalRecordManager(privKey, "test-realm")
	require.NoError(t, err, "InitializeLocalRecordManager should accept crypto.PrivateKey")

	// 验证初始化成功
	assert.True(t, dht.localRecordManager.IsInitialized())
}

// TestDHT_InitializeLocalRecordManager_InvalidType 测试无效类型
func TestDHT_InitializeLocalRecordManager_InvalidType(t *testing.T) {
	host := newMockHost("test-peer")
	dht, err := New(host, nil)
	require.NoError(t, err)

	ctx := context.Background()
	err = dht.Start(ctx)
	require.NoError(t, err)
	defer dht.Stop(ctx)

	// 使用无效类型初始化
	err = dht.InitializeLocalRecordManager("invalid-type", "test-realm")
	assert.Error(t, err, "InitializeLocalRecordManager should reject invalid types")
	assert.Contains(t, err.Error(), "invalid private key type")
}
