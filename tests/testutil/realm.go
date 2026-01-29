// Package testutil 提供测试辅助工具
package testutil

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/dep2p/go-dep2p"
)

// TestRealmBuilder Realm 构建器
//
// 使用 Builder 模式简化 Realm 的创建和配置。
//
// 示例:
//
//	realm := testutil.NewTestRealm(t, node).
//		WithPSK("test-secret-key").
//		Join()
//
type TestRealmBuilder struct {
	t    *testing.T
	node *dep2p.Node
	psk  []byte
}

// NewTestRealm 创建测试 Realm 构建器
func NewTestRealm(t *testing.T, node *dep2p.Node) *TestRealmBuilder {
	t.Helper()
	return &TestRealmBuilder{
		t:    t,
		node: node,
		psk:  []byte(DefaultTestPSK),
	}
}

// WithPSK 设置 PSK (Pre-Shared Key)
//
// 相同 PSK 的节点会加入同一个 Realm。
func (b *TestRealmBuilder) WithPSK(psk string) *TestRealmBuilder {
	b.t.Helper()
	b.psk = []byte(psk)
	return b
}

// WithPSKBytes 设置 PSK (字节形式)
func (b *TestRealmBuilder) WithPSKBytes(psk []byte) *TestRealmBuilder {
	b.t.Helper()
	b.psk = psk
	return b
}

// Join 加入 Realm 并注册清理函数
//
// Realm 会在测试结束时自动离开。
func (b *TestRealmBuilder) Join() *dep2p.Realm {
	b.t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	realm, err := b.node.JoinRealm(ctx, b.psk)
	require.NoError(b.t, err, "加入 Realm 失败")
	require.NotNil(b.t, realm, "Realm 不应为 nil")

	// 注册清理函数
	b.t.Cleanup(func() {
		// Realm 会在 Node.Close() 时自动清理，这里不需要额外操作
		// 但如果需要显式离开，可以调用 realm.Leave()
	})

	return realm
}
