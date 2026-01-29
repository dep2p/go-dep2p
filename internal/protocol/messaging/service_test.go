package messaging

import (
	"context"
	"testing"
	"time"

	"github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	host := newMockHost("peer-1")
	realmMgr := newMockRealmManager()

	svc, err := New(host, realmMgr)
	require.NoError(t, err)
	require.NotNil(t, svc)

	assert.Equal(t, host, svc.host)
	assert.Equal(t, realmMgr, svc.realmMgr)
	assert.NotNil(t, svc.codec)
	assert.NotNil(t, svc.handlers)
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
	svc, err := New(host, nil)
	assert.NoError(t, err)
	assert.NotNil(t, svc)
}

func TestNew_WithOptions(t *testing.T) {
	host := newMockHost("peer-1")
	realmMgr := newMockRealmManager()

	timeout := 5 * time.Second
	maxRetries := 5

	svc, err := New(
		host,
		realmMgr,
		WithTimeout(timeout),
		WithMaxRetries(maxRetries),
	)
	require.NoError(t, err)
	require.NotNil(t, svc)

	assert.Equal(t, timeout, svc.config.Timeout)
	assert.Equal(t, maxRetries, svc.config.MaxRetries)
}

func TestService_StartStop(t *testing.T) {
	host := newMockHost("peer-1")
	realmMgr := newMockRealmManager()

	svc, err := New(host, realmMgr)
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

func TestService_RegisterHandler(t *testing.T) {
	host := newMockHost("peer-1")
	realmMgr := newMockRealmManager()

	// 创建 Realm
	realm := newMockRealm("realm-1", "Test Realm")
	realmMgr.AddRealm(realm)

	svc, err := New(host, realmMgr)
	require.NoError(t, err)

	ctx := context.Background()
	err = svc.Start(ctx)
	require.NoError(t, err)

	// 注册处理器
	handler := func(ctx context.Context, req *interfaces.Request) (*interfaces.Response, error) {
		return &interfaces.Response{
			ID:   req.ID,
			From: host.ID(),
			Data: []byte("pong"),
		}, nil
	}

	err = svc.RegisterHandler("ping", handler)
	require.NoError(t, err)

	// 验证已注册到 handlers
	_, exists := svc.handlers.Get("ping")
	assert.True(t, exists)

	// 验证已注册到 Host
	protocolID := buildProtocolID(realm.ID(), "ping")
	host.mu.RLock()
	_, exists = host.handlers[string(protocolID)]
	host.mu.RUnlock()
	assert.True(t, exists)
}

func TestService_RegisterHandler_InvalidProtocol(t *testing.T) {
	host := newMockHost("peer-1")
	realmMgr := newMockRealmManager()

	svc, err := New(host, realmMgr)
	require.NoError(t, err)

	ctx := context.Background()
	err = svc.Start(ctx)
	require.NoError(t, err)

	handler := func(ctx context.Context, req *interfaces.Request) (*interfaces.Response, error) {
		return nil, nil
	}

	// 空协议
	err = svc.RegisterHandler("", handler)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidProtocol)

	// 包含空格的协议
	err = svc.RegisterHandler("invalid protocol", handler)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidProtocol)
}

func TestService_UnregisterHandler(t *testing.T) {
	host := newMockHost("peer-1")
	realmMgr := newMockRealmManager()

	realm := newMockRealm("realm-1", "Test Realm")
	realmMgr.AddRealm(realm)

	svc, err := New(host, realmMgr)
	require.NoError(t, err)

	ctx := context.Background()
	err = svc.Start(ctx)
	require.NoError(t, err)

	// 注册处理器
	handler := func(ctx context.Context, req *interfaces.Request) (*interfaces.Response, error) {
		return nil, nil
	}
	err = svc.RegisterHandler("test", handler)
	require.NoError(t, err)

	// 注销处理器
	err = svc.UnregisterHandler("test")
	require.NoError(t, err)

	// 验证已注销
	_, exists := svc.handlers.Get("test")
	assert.False(t, exists)
}

func TestService_Send_NotStarted(t *testing.T) {
	host := newMockHost("peer-1")
	realmMgr := newMockRealmManager()

	svc, err := New(host, realmMgr)
	require.NoError(t, err)

	// 未启动时发送应失败
	ctx := context.Background()
	_, err = svc.Send(ctx, "peer-2", "test", []byte("data"))
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrNotStarted)
}

func TestService_Send_InvalidProtocol(t *testing.T) {
	host := newMockHost("peer-1")
	realmMgr := newMockRealmManager()

	svc, err := New(host, realmMgr)
	require.NoError(t, err)

	ctx := context.Background()
	err = svc.Start(ctx)
	require.NoError(t, err)

	// 无效协议
	_, err = svc.Send(ctx, "peer-2", "", []byte("data"))
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidProtocol)
}

func TestService_Send_NotRealmMember(t *testing.T) {
	host := newMockHost("peer-1")
	realmMgr := newMockRealmManager()

	realm := newMockRealm("realm-1", "Test Realm")
	realmMgr.AddRealm(realm)

	svc, err := New(host, realmMgr)
	require.NoError(t, err)

	ctx := context.Background()
	err = svc.Start(ctx)
	require.NoError(t, err)

	// 节点不是 Realm 成员
	_, err = svc.Send(ctx, "peer-2", "test", []byte("data"))
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrNotRealmMember)
}

func TestService_SendAsync_NotStarted(t *testing.T) {
	host := newMockHost("peer-1")
	realmMgr := newMockRealmManager()

	svc, err := New(host, realmMgr)
	require.NoError(t, err)

	// 未启动时发送应失败
	ctx := context.Background()
	_, err = svc.SendAsync(ctx, "peer-2", "test", []byte("data"))
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrNotStarted)
}

func TestService_SendAsync_InvalidProtocol(t *testing.T) {
	host := newMockHost("peer-1")
	realmMgr := newMockRealmManager()

	svc, err := New(host, realmMgr)
	require.NoError(t, err)

	ctx := context.Background()
	err = svc.Start(ctx)
	require.NoError(t, err)

	// 无效协议
	_, err = svc.SendAsync(ctx, "peer-2", "invalid protocol", []byte("data"))
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidProtocol)
}

func TestService_SendAsync_NotRealmMember(t *testing.T) {
	host := newMockHost("peer-1")
	realmMgr := newMockRealmManager()

	realm := newMockRealm("realm-1", "Test Realm")
	realmMgr.AddRealm(realm)

	svc, err := New(host, realmMgr)
	require.NoError(t, err)

	ctx := context.Background()
	err = svc.Start(ctx)
	require.NoError(t, err)

	// 节点不是 Realm 成员
	_, err = svc.SendAsync(ctx, "peer-2", "test", []byte("data"))
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrNotRealmMember)
}

func TestService_Close(t *testing.T) {
	host := newMockHost("peer-1")
	realmMgr := newMockRealmManager()

	svc, err := New(host, realmMgr)
	require.NoError(t, err)

	ctx := context.Background()
	err = svc.Start(ctx)
	require.NoError(t, err)

	// 关闭
	err = svc.Close()
	require.NoError(t, err)
	assert.False(t, svc.started)
}

func TestService_IsRealmMember(t *testing.T) {
	host := newMockHost("peer-1")
	realmMgr := newMockRealmManager()

	realm := newMockRealm("realm-1", "Test Realm")
	realm.AddMember("peer-2")
	realmMgr.AddRealm(realm)

	svc, err := New(host, realmMgr)
	require.NoError(t, err)

	// peer-2 是成员
	assert.True(t, svc.isRealmMember("peer-2"))

	// peer-3 不是成员
	assert.False(t, svc.isRealmMember("peer-3"))
}

func TestService_FindRealmForPeer(t *testing.T) {
	host := newMockHost("peer-1")
	realmMgr := newMockRealmManager()

	realm1 := newMockRealm("realm-1", "Realm 1")
	realm1.AddMember("peer-2")
	realmMgr.AddRealm(realm1)

	realm2 := newMockRealm("realm-2", "Realm 2")
	realm2.AddMember("peer-3")
	realmMgr.AddRealm(realm2)

	svc, err := New(host, realmMgr)
	require.NoError(t, err)

	// 查找 peer-2
	realm, err := svc.findRealmForPeer("peer-2")
	require.NoError(t, err)
	assert.Equal(t, "realm-1", realm.ID())

	// 查找 peer-3
	realm, err = svc.findRealmForPeer("peer-3")
	require.NoError(t, err)
	assert.Equal(t, "realm-2", realm.ID())

	// 查找不存在的 peer
	_, err = svc.findRealmForPeer("peer-unknown")
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrNotRealmMember)
}

func TestShouldRetry(t *testing.T) {
	tests := []struct {
		name         string
		err          error
		expectRetry  bool
	}{
		{
			name:        "context deadline",
			err:         context.DeadlineExceeded,
			expectRetry: false,
		},
		{
			name:        "context canceled",
			err:         context.Canceled,
			expectRetry: false,
		},
		{
			name:        "not realm member",
			err:         ErrNotRealmMember,
			expectRetry: false,
		},
		{
			name:        "invalid protocol",
			err:         ErrInvalidProtocol,
			expectRetry: false,
		},
		{
			name:        "stream closed",
			err:         ErrStreamClosed,
			expectRetry: true,
		},
		{
			name:        "timeout error",
			err:         ErrTimeout,
			expectRetry: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			retry := shouldRetry(tt.err)
			assert.Equal(t, tt.expectRetry, retry)
		})
	}
}
