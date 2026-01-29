package pubsub

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	host := newMockHost("peer-1")
	realmMgr := newMockRealmManager()

	svc, err := New(host, realmMgr, WithDisableHeartbeat(true))
	require.NoError(t, err)
	require.NotNil(t, svc)

	assert.Equal(t, host, svc.host)
	assert.Equal(t, realmMgr, svc.realmMgr)
	assert.NotNil(t, svc.gossip)
	assert.NotNil(t, svc.validator)
	assert.NotNil(t, svc.config)
}

func TestNew_NilHost(t *testing.T) {
	realmMgr := newMockRealmManager()

	svc, err := New(nil, realmMgr)
	assert.Error(t, err)
	assert.Nil(t, svc)
	assert.ErrorIs(t, err, ErrNilHost)
}

func TestNew_NilRealmManager(t *testing.T) {
	host := newMockHost("peer-1")

	// RealmManager 现在是可选的
	svc, err := New(host, nil, WithDisableHeartbeat(true))
	assert.NoError(t, err)
	assert.NotNil(t, svc)
}

func TestService_StartStop(t *testing.T) {
	host := newMockHost("peer-1")
	realmMgr := newMockRealmManager()

	realm := newMockRealm("realm-1", "Test Realm")
	realmMgr.AddRealm(realm)

	svc, err := New(host, realmMgr, WithDisableHeartbeat(true))
	require.NoError(t, err)

	ctx := context.Background()

	// 启动
	err = svc.Start(ctx)
	require.NoError(t, err)
	assert.True(t, svc.started)

	// 重复启动应失败
	err = svc.Start(ctx)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrAlreadyStarted)

	// 停止
	err = svc.Stop(ctx)
	require.NoError(t, err)
	assert.False(t, svc.started)

	// 重复停止应失败
	err = svc.Stop(ctx)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrNotStarted)
}

func TestService_Join(t *testing.T) {
	host := newMockHost("peer-1")
	realmMgr := newMockRealmManager()

	realm := newMockRealm("realm-1", "Test Realm")
	realmMgr.AddRealm(realm)

	svc, err := New(host, realmMgr, WithDisableHeartbeat(true))
	require.NoError(t, err)

	ctx := context.Background()
	err = svc.Start(ctx)
	require.NoError(t, err)
	defer svc.Close()

	// 加入主题
	topic, err := svc.Join("test-topic")
	require.NoError(t, err)
	require.NotNil(t, topic)
	assert.Equal(t, "test-topic", topic.String())

	// 重复加入应返回错误
	_, err = svc.Join("test-topic")
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrTopicAlreadyJoined)
}

func TestService_Join_NotStarted(t *testing.T) {
	host := newMockHost("peer-1")
	realmMgr := newMockRealmManager()

	svc, err := New(host, realmMgr, WithDisableHeartbeat(true))
	require.NoError(t, err)

	// 未启动时加入应失败
	_, err = svc.Join("test-topic")
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrNotStarted)
}

func TestService_GetTopics(t *testing.T) {
	host := newMockHost("peer-1")
	realmMgr := newMockRealmManager()

	realm := newMockRealm("realm-1", "Test Realm")
	realmMgr.AddRealm(realm)

	svc, err := New(host, realmMgr, WithDisableHeartbeat(true))
	require.NoError(t, err)

	ctx := context.Background()
	err = svc.Start(ctx)
	require.NoError(t, err)
	defer svc.Close()

	// 初始应为空
	topics := svc.GetTopics()
	assert.Empty(t, topics)

	// 加入主题
	_, err = svc.Join("topic1")
	require.NoError(t, err)
	_, err = svc.Join("topic2")
	require.NoError(t, err)

	// 获取主题列表
	topics = svc.GetTopics()
	assert.Len(t, topics, 2)
	assert.Contains(t, topics, "topic1")
	assert.Contains(t, topics, "topic2")
}

func TestService_ListPeers(t *testing.T) {
	host := newMockHost("peer-1")
	realmMgr := newMockRealmManager()

	realm := newMockRealm("realm-1", "Test Realm")
	realmMgr.AddRealm(realm)

	svc, err := New(host, realmMgr, WithDisableHeartbeat(true))
	require.NoError(t, err)

	ctx := context.Background()
	err = svc.Start(ctx)
	require.NoError(t, err)
	defer svc.Close()

	// 初始应为空
	peers := svc.ListPeers("test-topic")
	assert.Empty(t, peers)
}

func TestService_Close(t *testing.T) {
	host := newMockHost("peer-1")
	realmMgr := newMockRealmManager()

	svc, err := New(host, realmMgr, WithDisableHeartbeat(true))
	require.NoError(t, err)

	ctx := context.Background()
	err = svc.Start(ctx)
	require.NoError(t, err)

	// 关闭
	err = svc.Close()
	require.NoError(t, err)
	assert.False(t, svc.started)
}
