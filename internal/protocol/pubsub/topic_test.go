package pubsub

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTopic_String(t *testing.T) {
	host := newMockHost("peer-1")
	realmMgr := newMockRealmManager()

	realm := newMockRealm("realm-1", "Test Realm")
	realm.AddMember("peer-1")
	realmMgr.AddRealm(realm)

	svc, err := New(host, realmMgr, WithDisableHeartbeat(true))
	require.NoError(t, err)

	ctx := context.Background()
	err = svc.Start(ctx)
	require.NoError(t, err)
	defer svc.Close()

	topic, err := svc.Join("my-topic")
	require.NoError(t, err)

	assert.Equal(t, "my-topic", topic.String())
}

func TestTopic_Subscribe(t *testing.T) {
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

	topic, err := svc.Join("test-topic")
	require.NoError(t, err)

	// 订阅
	sub, err := topic.Subscribe()
	require.NoError(t, err)
	require.NotNil(t, sub)
}

func TestTopic_EventHandler(t *testing.T) {
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

	topic, err := svc.Join("test-topic")
	require.NoError(t, err)

	// 创建事件处理器
	handler, err := topic.EventHandler()
	require.NoError(t, err)
	require.NotNil(t, handler)
}

func TestTopic_ListPeers(t *testing.T) {
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

	topic, err := svc.Join("test-topic")
	require.NoError(t, err)

	// 初始应为空
	peers := topic.ListPeers()
	assert.Empty(t, peers)
}

func TestTopic_Close(t *testing.T) {
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

	topic, err := svc.Join("test-topic")
	require.NoError(t, err)

	// 关闭主题
	err = topic.Close()
	require.NoError(t, err)

	// 重复关闭应成功(幂等)
	err = topic.Close()
	require.NoError(t, err)
}

func TestTopic_Publish_WhenClosed(t *testing.T) {
	host := newMockHost("peer-1")
	realmMgr := newMockRealmManager()

	realm := newMockRealm("realm-1", "Test Realm")
	realm.AddMember("peer-1")
	realmMgr.AddRealm(realm)

	svc, err := New(host, realmMgr, WithDisableHeartbeat(true))
	require.NoError(t, err)

	ctx := context.Background()
	err = svc.Start(ctx)
	require.NoError(t, err)
	defer svc.Close()

	topic, err := svc.Join("test-topic")
	require.NoError(t, err)

	// 关闭主题
	err = topic.Close()
	require.NoError(t, err)

	// 发布应失败
	err = topic.Publish(ctx, []byte("data"))
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrTopicClosed)
}

func TestTopic_Subscribe_WhenClosed(t *testing.T) {
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

	topic, err := svc.Join("test-topic")
	require.NoError(t, err)

	// 关闭主题
	err = topic.Close()
	require.NoError(t, err)

	// 订阅应失败
	_, err = topic.Subscribe()
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrTopicClosed)
}
