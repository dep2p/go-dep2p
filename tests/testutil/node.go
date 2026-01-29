// Package testutil 提供测试辅助工具
package testutil

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/dep2p/go-dep2p"
)

// TestNodeBuilder 测试节点构建器
//
// 使用 Builder 模式简化测试节点的创建和配置。
//
// 示例:
//
//	node := testutil.NewTestNode(t).
//		WithListenAddr("/ip4/127.0.0.1/udp/0/quic-v1").
//		WithPreset("minimal").
//		Start()
//
type TestNodeBuilder struct {
	t           *testing.T
	listenAddrs []string
	dataDir     string
	preset      string
	enableRelay bool
}

// NewTestNode 创建测试节点构建器
//
// 默认配置:
//   - preset: "minimal"
//   - listenAddr: "/ip4/127.0.0.1/udp/0/quic-v1"
//   - dataDir: t.TempDir()
//   - enableRelay: false
func NewTestNode(t *testing.T) *TestNodeBuilder {
	t.Helper()
	return &TestNodeBuilder{
		t:           t,
		preset:      "minimal",
		listenAddrs: []string{"/ip4/127.0.0.1/udp/0/quic-v1"},
		enableRelay: false,
	}
}

// WithListenAddr 设置监听地址
//
// 可以多次调用以设置多个地址。
func (b *TestNodeBuilder) WithListenAddr(addr string) *TestNodeBuilder {
	b.t.Helper()
	b.listenAddrs = []string{addr} // 替换现有地址
	return b
}

// WithListenAddrs 设置多个监听地址
func (b *TestNodeBuilder) WithListenAddrs(addrs ...string) *TestNodeBuilder {
	b.t.Helper()
	b.listenAddrs = addrs
	return b
}

// WithDataDir 设置数据目录
//
// 默认使用 t.TempDir()，测试结束后自动清理。
func (b *TestNodeBuilder) WithDataDir(dir string) *TestNodeBuilder {
	b.t.Helper()
	b.dataDir = dir
	return b
}

// WithPreset 设置预设配置
//
// 可选值: "minimal", "desktop", "server", "mobile"
func (b *TestNodeBuilder) WithPreset(preset string) *TestNodeBuilder {
	b.t.Helper()
	b.preset = preset
	return b
}

// WithRelay 设置是否启用 Relay
func (b *TestNodeBuilder) WithRelay(enable bool) *TestNodeBuilder {
	b.t.Helper()
	b.enableRelay = enable
	return b
}

// Start 启动节点并注册清理函数
//
// 节点会在测试结束时自动关闭。
func (b *TestNodeBuilder) Start() *dep2p.Node {
	b.t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 构建选项
	opts := []dep2p.Option{
		dep2p.WithPreset(b.preset),
		dep2p.WithRelay(b.enableRelay),
	}

	// 数据目录
	if b.dataDir == "" {
		b.dataDir = b.t.TempDir()
	}
	opts = append(opts, dep2p.WithDataDir(b.dataDir))

	// 监听地址
	if len(b.listenAddrs) > 0 {
		opts = append(opts, dep2p.WithListenAddrs(b.listenAddrs...))
	}

	// 启动节点
	node, err := dep2p.Start(ctx, opts...)
	require.NoError(b.t, err, "启动测试节点失败")
	require.NotNil(b.t, node, "节点不应为 nil")

	// 注册清理函数
	b.t.Cleanup(func() {
		if err := node.Close(); err != nil {
			b.t.Logf("关闭节点失败: %v", err)
		}
	})

	return node
}

// CreateTestNode 创建测试节点（向后兼容，推荐使用 NewTestNode）
//
// Deprecated: 使用 NewTestNode().Start() 替代
func CreateTestNode(t *testing.T, opts ...dep2p.Option) *dep2p.Node {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	node, err := dep2p.New(ctx, opts...)
	require.NoError(t, err, "创建测试节点失败")
	require.NotNil(t, node, "节点不应为 nil")

	t.Cleanup(func() {
		if err := node.Close(); err != nil {
			t.Logf("关闭节点失败: %v", err)
		}
	})

	return node
}

// CreateTestNodes 创建多个测试节点（向后兼容）
//
// Deprecated: 使用循环调用 NewTestNode().Start() 替代
func CreateTestNodes(t *testing.T, count int, opts ...dep2p.Option) []*dep2p.Node {
	t.Helper()

	nodes := make([]*dep2p.Node, count)
	for i := 0; i < count; i++ {
		nodes[i] = CreateTestNode(t, opts...)
	}

	return nodes
}

// StartTestNode 创建并启动测试节点（向后兼容）
//
// Deprecated: 使用 NewTestNode().Start() 替代
func StartTestNode(t *testing.T, opts ...dep2p.Option) *dep2p.Node {
	t.Helper()

	node := CreateTestNode(t, opts...)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := node.Start(ctx)
	require.NoError(t, err, "启动节点失败")

	return node
}
