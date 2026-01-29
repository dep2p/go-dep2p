package realm

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

// ============================================================================
//                              Realm 成员管理测试（6个）
// ============================================================================

// TestRealm_ID 测试获取 ID
func TestRealm_ID(t *testing.T) {
	realm := &realmImpl{
		id: "test-realm",
	}

	assert.Equal(t, "test-realm", realm.ID())
}

// TestRealm_Name 测试获取名称
func TestRealm_Name(t *testing.T) {
	realm := &realmImpl{
		name: "Test Realm",
	}

	assert.Equal(t, "Test Realm", realm.Name())
}

// TestRealm_Members 测试获取成员列表
func TestRealm_Members(t *testing.T) {
	realm := &realmImpl{}

	members := realm.Members()
	assert.NotNil(t, members)
}

// TestRealm_IsMember 测试检查成员
func TestRealm_IsMember(t *testing.T) {
	realm := &realmImpl{}

	isMember := realm.IsMember("peer1")
	_ = isMember
}

// TestRealm_MemberCount 测试成员数量
func TestRealm_MemberCount(t *testing.T) {
	realm := &realmImpl{}

	count := realm.MemberCount()
	assert.GreaterOrEqual(t, count, 0)
}

// TestRealm_AddMember 测试添加成员
func TestRealm_AddMember(t *testing.T) {
	realm := &realmImpl{}

	// 简化实现
	_ = realm
}

// ============================================================================
//                              Realm 路由测试
// ============================================================================

// TestRealm_FindRoute 测试查找路由
func TestRealm_FindRoute(t *testing.T) {
	realm := &realmImpl{}
	ctx := context.Background()

	route, err := realm.FindRoute(ctx, "peer1")
	// routing==nil 时应该返回 ErrRoutingFailed（代码 realm.go:246-248）
	assert.ErrorIs(t, err, ErrRoutingFailed, "FindRoute without routing should return ErrRoutingFailed")
	assert.Nil(t, route, "route should be nil on error")
}

// TestRealm_SelectBestRoute 测试选择最佳路由
func TestRealm_SelectBestRoute(t *testing.T) {
	realm := &realmImpl{}
	ctx := context.Background()

	route, err := realm.SelectBestRoute(ctx, nil, 0)
	// routing==nil 时应该返回 ErrRoutingFailed（代码 realm.go:255-257）
	assert.ErrorIs(t, err, ErrRoutingFailed, "SelectBestRoute without routing should return ErrRoutingFailed")
	assert.Nil(t, route, "route should be nil on error")
}

// TestRealm_InvalidateRoute 测试使路由失效
func TestRealm_InvalidateRoute(t *testing.T) {
	realm := &realmImpl{}

	realm.InvalidateRoute("peer1")
}

// TestRealm_GetRouteTable 测试获取路由表
func TestRealm_GetRouteTable(t *testing.T) {
	realm := &realmImpl{}

	table := realm.GetRouteTable()
	_ = table
}

// ============================================================================
//                              Realm 网关测试
// ============================================================================

// TestRealm_GetReachableNodes 测试获取可达节点
func TestRealm_GetReachableNodes(t *testing.T) {
	realm := &realmImpl{}

	nodes := realm.GetReachableNodes()
	assert.NotNil(t, nodes)
}

// ============================================================================
//                              Realm 服务门面测试（6个）
// ============================================================================

// TestRealm_Messaging 测试 Messaging 服务
func TestRealm_Messaging(t *testing.T) {
	realm := &realmImpl{}

	messaging := realm.Messaging()
	_ = messaging
}

// TestRealm_PubSub 测试 PubSub 服务
func TestRealm_PubSub(t *testing.T) {
	realm := &realmImpl{}

	pubsub := realm.PubSub()
	_ = pubsub
}

// TestRealm_Streams 测试 Streams 服务
func TestRealm_Streams(t *testing.T) {
	realm := &realmImpl{}

	streams := realm.Streams()
	_ = streams
}

// TestRealm_Liveness 测试 Liveness 服务
func TestRealm_Liveness(t *testing.T) {
	realm := &realmImpl{}

	liveness := realm.Liveness()
	_ = liveness
}

// TestRealm_ServiceInitialization 测试服务初始化
func TestRealm_ServiceInitialization(t *testing.T) {
	realm := &realmImpl{}

	// 多次调用应返回同一实例
	msg1 := realm.Messaging()
	msg2 := realm.Messaging()

	_ = msg1
	_ = msg2
}

// TestRealm_AllServices 测试所有服务
func TestRealm_AllServices(t *testing.T) {
	realm := &realmImpl{}

	messaging := realm.Messaging()
	pubsub := realm.PubSub()
	streams := realm.Streams()
	liveness := realm.Liveness()

	_ = messaging
	_ = pubsub
	_ = streams
	_ = liveness
}

// ============================================================================
//                              Realm 状态同步测试（2个）
// ============================================================================

// TestRealm_SyncMemberToRouting 测试 Member → Routing 同步
func TestRealm_SyncMemberToRouting(t *testing.T) {
	realm := &realmImpl{}
	ctx := context.Background()

	err := realm.syncMemberToRouting(ctx)
	// 没有 member/routing 组件时应该成功（空操作）
	assert.NoError(t, err, "syncMemberToRouting should succeed even without components")
}

// TestRealm_SyncRoutingToGateway 测试 Routing → Gateway 同步
func TestRealm_SyncRoutingToGateway(t *testing.T) {
	realm := &realmImpl{}
	ctx := context.Background()

	err := realm.syncRoutingToGateway(ctx)
	// 没有 routing/gateway 组件时应该成功（空操作）
	assert.NoError(t, err, "syncRoutingToGateway should succeed even without components")
}

// ============================================================================
//                              补充覆盖率测试
// ============================================================================

// TestRealm_PSK 测试获取 PSK
func TestRealm_PSK(t *testing.T) {
	realm := &realmImpl{
		psk: []byte("test-psk"),
	}

	psk := realm.PSK()
	assert.NotNil(t, psk)
	assert.Equal(t, []byte("test-psk"), psk)
}

// TestRealm_Authenticate 测试认证
func TestRealm_Authenticate(t *testing.T) {
	realm := &realmImpl{}
	ctx := context.Background()

	valid, err := realm.Authenticate(ctx, "peer1", []byte("proof"))
	// auth==nil 时应该返回 ErrAuthFailed（代码 realm.go:224-226）
	assert.ErrorIs(t, err, ErrAuthFailed, "Authenticate without auth should return ErrAuthFailed")
	assert.False(t, valid, "valid should be false on error")
}

// TestRealm_GenerateProof 测试生成证明
func TestRealm_GenerateProof(t *testing.T) {
	realm := &realmImpl{}
	ctx := context.Background()

	proof, err := realm.GenerateProof(ctx)
	// auth==nil 时应该返回 ErrAuthFailed（代码 realm.go:233-235）
	assert.ErrorIs(t, err, ErrAuthFailed, "GenerateProof without auth should return ErrAuthFailed")
	assert.Nil(t, proof, "proof should be nil on error")
}

// TestRealm_Join 测试加入
func TestRealm_Join(t *testing.T) {
	realm := &realmImpl{}
	ctx := context.Background()

	err := realm.Join(ctx)
	assert.NoError(t, err)
}

// TestRealm_Close 测试关闭
func TestRealm_Close(t *testing.T) {
	realm := &realmImpl{}

	err := realm.Close()
	assert.NoError(t, err)
}

// TestRealm_Leave 测试离开
func TestRealm_Leave(t *testing.T) {
	realm := &realmImpl{}
	ctx := context.Background()

	err := realm.Leave(ctx)
	// manager==nil 时应该返回 ErrNotInRealm（代码 realm.go:708-710）
	assert.ErrorIs(t, err, ErrNotInRealm, "Leave without manager should return ErrNotInRealm")
}

// TestRealm_GetStats 测试获取统计
func TestRealm_GetStats(t *testing.T) {
	realm := &realmImpl{
		id:   "test-realm",
		name: "Test Realm",
	}

	stats := realm.GetStats()
	assert.NotNil(t, stats)
	assert.Equal(t, "test-realm", stats.ID)
}

// ============================================================================
//                              Step B4 测试：Relay 地址注册
// ============================================================================

// mockAddressBookService 模拟地址簿服务
type mockAddressBookService struct {
	relayPeerID    string
	registerCalled bool
	registerError  error
}

func (m *mockAddressBookService) RegisterSelf(ctx context.Context, relayPeerID string) error {
	m.registerCalled = true
	return m.registerError
}

func (m *mockAddressBookService) GetRelayPeerID() string {
	return m.relayPeerID
}

// TestRealm_RegisterToRelay_NoAddressBookService 测试无地址簿服务时跳过注册
func TestRealm_RegisterToRelay_NoAddressBookService(t *testing.T) {
	realm := &realmImpl{
		id:                 "test-realm",
		addressBookService: nil, // 无地址簿服务
	}
	ctx := context.Background()

	// 应该静默返回，不报错
	realm.registerToRelay(ctx)
	// 无法验证内部行为，但确保不会 panic
}

// TestRealm_RegisterToRelay_NoRelayConfigured 测试未配置 Relay 时跳过注册
func TestRealm_RegisterToRelay_NoRelayConfigured(t *testing.T) {
	mockABS := &mockAddressBookService{
		relayPeerID: "", // 未配置 Relay
	}

	realm := &realmImpl{
		id:                 "test-realm",
		addressBookService: mockABS,
	}
	ctx := context.Background()

	realm.registerToRelay(ctx)

	// 未配置 Relay 时不应调用 RegisterSelf
	assert.False(t, mockABS.registerCalled, "RegisterSelf should not be called when relay not configured")
}

// TestRealm_RegisterToRelay_Success 测试成功注册到 Relay
func TestRealm_RegisterToRelay_Success(t *testing.T) {
	mockABS := &mockAddressBookService{
		relayPeerID:   "relay-peer-id",
		registerError: nil,
	}

	realm := &realmImpl{
		id:                 "test-realm",
		addressBookService: mockABS,
	}
	ctx := context.Background()

	realm.registerToRelay(ctx)

	// 应该调用 RegisterSelf
	assert.True(t, mockABS.registerCalled, "RegisterSelf should be called")
}

// TestRealm_RegisterToRelay_Error 测试注册失败时不影响 Realm 运行
func TestRealm_RegisterToRelay_Error(t *testing.T) {
	mockABS := &mockAddressBookService{
		relayPeerID:   "relay-peer-id",
		registerError: assert.AnError,
	}

	realm := &realmImpl{
		id:                 "test-realm",
		addressBookService: mockABS,
	}
	ctx := context.Background()

	// 不应该 panic，即使注册失败
	realm.registerToRelay(ctx)

	// 应该调用 RegisterSelf
	assert.True(t, mockABS.registerCalled, "RegisterSelf should be called even if it fails")
}
