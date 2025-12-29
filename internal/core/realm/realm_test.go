package realm

import (
	"bytes"
	"context"
	"sync"
	"testing"
	"time"

	"github.com/dep2p/go-dep2p/internal/config"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              Manager 测试
// ============================================================================

// TestManager_JoinLeaveRealm 测试加入/离开 Realm
//
// IMPL-1227: 使用新 API（JoinRealmWithKey）
func TestManager_JoinLeaveRealm(t *testing.T) {
	cfg := config.RealmConfig{
		RealmAuthEnabled: false,
	}

	manager := NewManager(cfg, nil)
	// 必须先 Start() 以初始化 ctx
	if err := manager.Start(context.Background()); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	ctx := context.Background()

	// IMPL-1227: 使用新 API 加入 Realm
	realmKey := types.GenerateRealmKey()
	realm, err := manager.JoinRealmWithKey(ctx, "test-realm-1", realmKey)
	if err != nil {
		t.Fatalf("JoinRealmWithKey failed: %v", err)
	}

	// 验证是成员
	if !manager.IsMember() {
		t.Error("should be member after joining")
	}

	// 验证当前 Realm
	currentRealm := manager.CurrentRealm()
	if currentRealm == nil {
		t.Fatal("CurrentRealm should not be nil")
	}
	if currentRealm.ID() != realm.ID() {
		t.Error("current realm should be the joined realm")
	}

	// IMPL-1227: 尝试加入第二个 Realm 应该返回 ErrAlreadyJoined
	realmKey2 := types.GenerateRealmKey()
	_, err = manager.JoinRealmWithKey(ctx, "test-realm-2", realmKey2)
	if err != ErrAlreadyJoined {
		t.Errorf("expected ErrAlreadyJoined, got %v", err)
	}

	// 离开 Realm
	err = manager.LeaveRealm()
	if err != nil {
		t.Fatalf("LeaveRealm failed: %v", err)
	}

	// 验证不再是成员
	if manager.IsMember() {
		t.Error("should not be member after leaving")
	}

	// 离开后可以加入新的 Realm
	realm2, err := manager.JoinRealmWithKey(ctx, "test-realm-2", realmKey2)
	if err != nil {
		t.Fatalf("JoinRealmWithKey after leave failed: %v", err)
	}

	currentRealm = manager.CurrentRealm()
	if currentRealm == nil || currentRealm.ID() != realm2.ID() {
		t.Error("current realm should be the new joined realm")
	}
}

// TestManager_StrictSingleRealm 测试严格单 Realm 模型
//
// IMPL-1227: 验证已加入 Realm 后再次 JoinRealm 返回 ErrAlreadyJoined
func TestManager_StrictSingleRealm(t *testing.T) {
	cfg := config.RealmConfig{
		RealmAuthEnabled: false,
	}

	manager := NewManager(cfg, nil)
	// 必须先 Start() 以初始化 ctx
	if err := manager.Start(context.Background()); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	ctx := context.Background()

	// 加入第一个 Realm
	realmKey1 := types.GenerateRealmKey()
	_, err := manager.JoinRealmWithKey(ctx, "realm-1", realmKey1)
	if err != nil {
		t.Fatalf("first join failed: %v", err)
	}

	// 尝试加入第二个应该返回 ErrAlreadyJoined
	realmKey2 := types.GenerateRealmKey()
	_, err = manager.JoinRealmWithKey(ctx, "realm-2", realmKey2)
	if err != ErrAlreadyJoined {
		t.Errorf("expected ErrAlreadyJoined, got %v", err)
	}

	// 尝试加入同一个 Realm 也应该返回 ErrAlreadyJoined
	_, err = manager.JoinRealmWithKey(ctx, "realm-1", realmKey1)
	if err != ErrAlreadyJoined {
		t.Errorf("expected ErrAlreadyJoined for same realm, got %v", err)
	}
}

func TestManager_RealmPeers(t *testing.T) {
	cfg := config.RealmConfig{
		RealmAuthEnabled: false,
	}

	manager := NewManager(cfg, nil)
	// 必须先 Start() 以初始化 ctx
	if err := manager.Start(context.Background()); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	ctx := context.Background()

	// IMPL-1227: 使用新 API
	realmKey := types.GenerateRealmKey()
	realm, err := manager.JoinRealmWithKey(ctx, "test-realm", realmKey)
	if err != nil {
		t.Fatalf("JoinRealmWithKey failed: %v", err)
	}

	realmID := realm.ID()

	// 添加节点
	nodeID1 := types.NodeID{1, 2, 3}
	nodeID2 := types.NodeID{4, 5, 6}

	manager.addRealmPeer(realmID, nodeID1, nil)
	manager.addRealmPeer(realmID, nodeID2, nil)

	// 验证节点数量（通过 Realm 对象）
	if realm.MemberCount() != 2 {
		t.Errorf("expected 2 peers, got %d", realm.MemberCount())
	}

	// 验证节点列表
	members := realm.Members()
	if len(members) != 2 {
		t.Errorf("expected 2 peers in list, got %d", len(members))
	}

	// 移除节点
	manager.removeRealmPeer(realmID, nodeID1)
	if realm.MemberCount() != 1 {
		t.Errorf("expected 1 peer after removal, got %d", realm.MemberCount())
	}
}

// ============================================================================
//                              AccessController 测试
// ============================================================================

func TestAccessController_JoinKey(t *testing.T) {
	cfg := config.RealmConfig{
		RealmAuthEnabled: false,
	}
	manager := NewManager(cfg, nil)
	ac := NewAccessController(manager)

	realmID := types.RealmID("protected-realm")
	joinKey := []byte("secret-key-123")

	// 设置 JoinKey
	err := ac.SetJoinKey(realmID, joinKey)
	if err != nil {
		t.Fatalf("SetJoinKey failed: %v", err)
	}

	// 验证正确的 JoinKey
	if !ac.ValidateJoinKey(realmID, joinKey) {
		t.Error("valid join key should pass validation")
	}

	// 验证错误的 JoinKey
	if ac.ValidateJoinKey(realmID, []byte("wrong-key")) {
		t.Error("invalid join key should fail validation")
	}
}

func TestAccessController_AccessLevels(t *testing.T) {
	ac := NewAccessController(nil)

	realmID := types.RealmID("test-realm")

	// 默认应该是公开
	if ac.GetAccess(realmID) != types.AccessLevelPublic {
		t.Error("default access level should be public")
	}

	// 设置为保护
	ac.SetAccess(realmID, types.AccessLevelProtected)
	if ac.GetAccess(realmID) != types.AccessLevelProtected {
		t.Error("access level should be protected")
	}

	// 设置为私有
	ac.SetAccess(realmID, types.AccessLevelPrivate)
	if ac.GetAccess(realmID) != types.AccessLevelPrivate {
		t.Error("access level should be private")
	}
}

func TestAccessController_Invite(t *testing.T) {
	cfg := config.RealmConfig{
		RealmAuthEnabled: false,
	}
	manager := NewManager(cfg, nil)
	ac := NewAccessController(manager)

	realmID := types.RealmID("private-realm")
	targetNode := types.NodeID{1, 2, 3, 4, 5, 6, 7, 8}

	// 设置为私有
	ac.SetAccess(realmID, types.AccessLevelPrivate)

	// 生成邀请
	invite, err := ac.GenerateInvite(realmID, targetNode)
	if err != nil {
		t.Fatalf("GenerateInvite failed: %v", err)
	}

	// 验证正确的邀请
	if !ac.ValidateInvite(realmID, invite, targetNode) {
		t.Error("valid invite should pass validation")
	}

	// 验证错误的节点
	wrongNode := types.NodeID{9, 10, 11}
	if ac.ValidateInvite(realmID, invite, wrongNode) {
		t.Error("invite for different node should fail")
	}
}

func TestAccessController_CanJoin(t *testing.T) {
	cfg := config.RealmConfig{
		RealmAuthEnabled: false,
	}
	manager := NewManager(cfg, nil)
	ac := NewAccessController(manager)

	nodeID := types.NodeID{1, 2, 3}

	// 公开 Realm - 任何人可加入
	publicRealm := types.RealmID("public")
	ac.SetAccess(publicRealm, types.AccessLevelPublic)
	if err := ac.CanJoin(publicRealm, nodeID, nil, nil); err != nil {
		t.Errorf("should be able to join public realm: %v", err)
	}

	// 保护 Realm - 需要 JoinKey
	protectedRealm := types.RealmID("protected")
	ac.SetAccess(protectedRealm, types.AccessLevelProtected)
	joinKey := []byte("secret")
	ac.SetJoinKey(protectedRealm, joinKey)

	if err := ac.CanJoin(protectedRealm, nodeID, nil, nil); err != ErrAccessDenied {
		t.Error("should not be able to join protected realm without key")
	}

	if err := ac.CanJoin(protectedRealm, nodeID, joinKey, nil); err != nil {
		t.Errorf("should be able to join with correct key: %v", err)
	}

	// 私有 Realm - 需要邀请
	privateRealm := types.RealmID("private")
	ac.SetAccess(privateRealm, types.AccessLevelPrivate)

	if err := ac.CanJoin(privateRealm, nodeID, nil, nil); err != ErrAccessDenied {
		t.Error("should not be able to join private realm without invite")
	}

	invite, _ := ac.GenerateInvite(privateRealm, nodeID)
	if err := ac.CanJoin(privateRealm, nodeID, nil, invite); err != nil {
		t.Errorf("should be able to join with valid invite: %v", err)
	}
}

// ============================================================================
//                              RealmFilter 测试
// ============================================================================

func TestRealmFilter_FilterByRealm(t *testing.T) {
	cfg := config.RealmConfig{
		RealmAuthEnabled: false,
	}
	manager := NewManager(cfg, nil)
	// 必须先 Start() 以初始化 ctx
	if err := manager.Start(context.Background()); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	filter := NewRealmFilter(manager)

	ctx := context.Background()
	realmKey := types.GenerateRealmKey()
	realm, err := manager.JoinRealmWithKey(ctx, "test-realm", realmKey)
	if err != nil {
		t.Fatalf("JoinRealmWithKey failed: %v", err)
	}
	realmID := realm.ID()

	// 添加成员
	member1 := types.NodeID{1, 1, 1}
	member2 := types.NodeID{2, 2, 2}
	nonMember := types.NodeID{3, 3, 3}

	manager.addRealmPeer(realmID, member1, nil)
	manager.addRealmPeer(realmID, member2, nil)

	// 过滤节点
	nodes := []types.NodeID{member1, member2, nonMember}
	filtered := filter.FilterByRealm(nodes, realmID)

	if len(filtered) != 2 {
		t.Errorf("expected 2 filtered nodes, got %d", len(filtered))
	}
}

func TestRealmFilter_IsMemberOf(t *testing.T) {
	cfg := config.RealmConfig{
		RealmAuthEnabled: false,
	}
	manager := NewManager(cfg, nil)
	// 必须先 Start() 以初始化 ctx
	if err := manager.Start(context.Background()); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	filter := NewRealmFilter(manager)

	ctx := context.Background()
	realmKey := types.GenerateRealmKey()
	realm, err := manager.JoinRealmWithKey(ctx, "test-realm", realmKey)
	if err != nil {
		t.Fatalf("JoinRealmWithKey failed: %v", err)
	}
	realmID := realm.ID()

	member := types.NodeID{1, 1, 1}
	nonMember := types.NodeID{2, 2, 2}

	manager.addRealmPeer(realmID, member, nil)

	if !filter.IsMemberOf(member, realmID) {
		t.Error("member should be recognized as member")
	}

	if filter.IsMemberOf(nonMember, realmID) {
		t.Error("non-member should not be recognized as member")
	}
}

// ============================================================================
//                              SyncService 测试
// ============================================================================

func TestSyncMessage_EncodeDecode(t *testing.T) {
	cfg := config.RealmConfig{
		RealmAuthEnabled: false,
	}
	manager := NewManager(cfg, nil)
	syncService := NewSyncService(manager, nil)

	msg := &SyncMessage{
		Type:      SyncMsgMemberList,
		RealmID:   types.RealmID("test-realm"),
		From:      types.NodeID{1, 2, 3},
		Timestamp: time.Now().UnixNano(),
		Payload:   []byte("test payload"),
	}

	// 编码
	var buf bytes.Buffer
	err := syncService.writeMessage(&buf, msg)
	if err != nil {
		t.Fatalf("writeMessage failed: %v", err)
	}

	// 解码
	decoded, err := syncService.readMessage(&buf)
	if err != nil {
		t.Fatalf("readMessage failed: %v", err)
	}

	// 验证
	if decoded.Type != msg.Type {
		t.Error("Type mismatch")
	}
	if decoded.RealmID != msg.RealmID {
		t.Error("RealmID mismatch")
	}
	if decoded.From != msg.From {
		t.Error("From mismatch")
	}
	if !bytes.Equal(decoded.Payload, msg.Payload) {
		t.Error("Payload mismatch")
	}
}

func TestMemberList_EncodeDecode(t *testing.T) {
	cfg := config.RealmConfig{
		RealmAuthEnabled: false,
	}
	manager := NewManager(cfg, nil)
	syncService := NewSyncService(manager, nil)

	members := []MemberInfo{
		{
			NodeID:   types.NodeID{1, 1, 1},
			JoinedAt: time.Now().UnixNano(),
			Role:     "admin",
		},
		{
			NodeID:   types.NodeID{2, 2, 2},
			JoinedAt: time.Now().UnixNano(),
			Role:     "member",
		},
	}

	// 编码
	encoded := syncService.encodeMemberList(members)

	// 解码
	decoded, err := syncService.decodeMemberList(encoded)
	if err != nil {
		t.Fatalf("decodeMemberList failed: %v", err)
	}

	// 验证
	if len(decoded) != len(members) {
		t.Errorf("expected %d members, got %d", len(members), len(decoded))
	}

	for i, m := range decoded {
		if m.NodeID != members[i].NodeID {
			t.Errorf("member %d NodeID mismatch", i)
		}
		if m.Role != members[i].Role {
			t.Errorf("member %d Role mismatch", i)
		}
	}
}

// ============================================================================
//                              RealmNamespace 测试
// ============================================================================

func TestRealmNamespace(t *testing.T) {
	realmID := types.RealmID("test-realm")
	ns := RealmNamespace(realmID)

	if ns != "realm/test-realm" {
		t.Errorf("unexpected namespace: %s", ns)
	}

	topicNs := RealmTopicNamespace(realmID, "my-topic")
	if topicNs != "realm/test-realm/topic/my-topic" {
		t.Errorf("unexpected topic namespace: %s", topicNs)
	}
}

func TestParseRealmFromNamespace(t *testing.T) {
	tests := []struct {
		namespace string
		expected  types.RealmID
		ok        bool
	}{
		{"realm/test-realm", types.RealmID("test-realm"), true},
		{"realm/test-realm/topic/foo", types.RealmID("test-realm"), true},
		{"other/namespace", "", false},
		{"realm/", "", false},
		{"", "", false},
	}

	for _, tt := range tests {
		realmID, ok := ParseRealmFromNamespace(tt.namespace)
		if ok != tt.ok {
			t.Errorf("ParseRealmFromNamespace(%q): expected ok=%v, got %v", tt.namespace, tt.ok, ok)
		}
		if realmID != tt.expected {
			t.Errorf("ParseRealmFromNamespace(%q): expected %q, got %q", tt.namespace, tt.expected, realmID)
		}
	}
}

// ============================================================================
//                              RealmDHTKey 测试
// ============================================================================

func TestRealmDHTKey(t *testing.T) {
	realmID := types.RealmID("test-realm")
	nodeID := types.NodeID{1, 2, 3, 4, 5}

	key := types.RealmDHTKey(realmID, nodeID)

	if len(key) != 32 {
		t.Errorf("expected 32 byte key, got %d", len(key))
	}

	// 相同输入应该产生相同的 key
	key2 := types.RealmDHTKey(realmID, nodeID)
	if !bytes.Equal(key, key2) {
		t.Error("same input should produce same key")
	}

	// 不同 Realm 应该产生不同的 key
	key3 := types.RealmDHTKey(types.RealmID("other-realm"), nodeID)
	if bytes.Equal(key, key3) {
		t.Error("different realm should produce different key")
	}
}

// ============================================================================
//                              goroutine 安全测试
// ============================================================================

func TestManager_AnnounceLoopWithoutStart(t *testing.T) {
	cfg := config.RealmConfig{
		RealmAuthEnabled: false,
	}

	manager := NewManager(cfg, nil)

	// 在未启动时调用 announceLoop 应该立即返回而不 panic
	done := make(chan struct{})
	go func() {
		defer close(done)
		manager.announceLoop()
	}()

	select {
	case <-done:
		// 成功
	case <-time.After(time.Second):
		t.Fatal("announceLoop 应该立即返回")
	}
}

func TestManager_SetDiscovery_Concurrent(t *testing.T) {
	cfg := config.RealmConfig{
		RealmAuthEnabled: false,
	}

	manager := NewManager(cfg, nil)

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			manager.SetDiscovery(nil)
		}()
	}
	wg.Wait()
}

func TestAccessController_Close(t *testing.T) {
	ac := NewAccessController(nil)

	// 第一次关闭应该成功
	ac.Close()

	// 第二次关闭应该是幂等的
	ac.Close()
}

func TestAccessController_Close_Concurrent(t *testing.T) {
	ac := NewAccessController(nil)

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ac.Close()
		}()
	}
	wg.Wait()
}

func TestSyncService_SyncLoopWithoutStart(t *testing.T) {
	cfg := config.RealmConfig{
		RealmAuthEnabled: false,
	}
	manager := NewManager(cfg, nil)
	syncService := NewSyncService(manager, nil)

	// 在未启动时调用 syncLoop 应该立即返回而不 panic
	done := make(chan struct{})
	go func() {
		defer close(done)
		syncService.syncLoop()
	}()

	select {
	case <-done:
		// 成功
	case <-time.After(time.Second):
		t.Fatal("syncLoop 应该立即返回")
	}
}

func TestManager_StartStop(t *testing.T) {
	cfg := config.RealmConfig{
		RealmAuthEnabled: false,
	}

	manager := NewManager(cfg, nil)

	ctx := context.Background()
	err := manager.Start(ctx)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// 启动两次应该是幂等的
	err = manager.Start(ctx)
	if err != nil {
		t.Fatalf("Second start failed: %v", err)
	}

	err = manager.Stop()
	if err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	// 停止两次应该是幂等的
	err = manager.Stop()
	if err != nil {
		t.Fatalf("Second stop failed: %v", err)
	}
}

func TestSyncService_StartStop(t *testing.T) {
	cfg := config.RealmConfig{
		RealmAuthEnabled: false,
	}
	manager := NewManager(cfg, nil)
	syncService := NewSyncService(manager, nil)

	ctx := context.Background()
	err := syncService.Start(ctx)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// 启动两次应该是幂等的
	err = syncService.Start(ctx)
	if err != nil {
		t.Fatalf("Second start failed: %v", err)
	}

	err = syncService.Stop()
	if err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	// 停止两次应该是幂等的
	err = syncService.Stop()
	if err != nil {
		t.Fatalf("Second stop failed: %v", err)
	}
}
