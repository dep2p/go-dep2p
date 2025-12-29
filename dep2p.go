// Package dep2p 提供 DeP2P 的用户 API 入口
//
// dep2p 是一个简洁、可靠的 P2P 网络库，专为业务应用设计。
// 本包提供面向用户的简洁 API，隐藏底层复杂性。
//
// 快速开始:
//
//	// 最简启动（使用默认配置）
//	endpoint, err := dep2p.QuickStart(ctx)
//
//	// 使用预设配置
//	endpoint, err := dep2p.New(
//	    dep2p.WithPreset(dep2p.PresetDesktop),
//	)
//
//	// 自定义配置
//	endpoint, err := dep2p.New(
//	    dep2p.WithListenPort(4001),
//	    dep2p.WithConnectionLimits(50, 100),
//	)
package dep2p

import (
	"context"
	"fmt"

	"github.com/dep2p/go-dep2p/internal/app"
	"github.com/dep2p/go-dep2p/pkg/interfaces/endpoint"
)

// Version 当前版本
const Version = "0.1.0"

// ============================================================================
//                              类型别名
// ============================================================================

// Endpoint 是用户使用的主接口
// 通过类型别名暴露，避免用户直接依赖内部包
type Endpoint = endpoint.Endpoint

// Connection 连接接口
type Connection = endpoint.Connection

// Stream 流接口
type Stream = endpoint.Stream

// ProtocolHandler 协议处理器
type ProtocolHandler = endpoint.ProtocolHandler

// NodeID 节点唯一标识符
type NodeID = endpoint.NodeID

// ProtocolID 协议标识符
type ProtocolID = endpoint.ProtocolID

// Address 网络地址接口
type Address = endpoint.Address

// PeerInfo 节点信息（用于发现回调）
type PeerInfo = endpoint.PeerInfo

// ============================================================================
//                              网络模式
// ============================================================================

// NetworkMode 网络模式
type NetworkMode int

const (
	// NetworkModeProduction 生产模式（默认）
	NetworkModeProduction NetworkMode = iota
	// NetworkModeLocalTest 本地测试模式
	NetworkModeLocalTest
)

// ============================================================================
//                              创建函数
// ============================================================================

// New 创建一个新的 dep2p 节点
//
// 使用 Option 模式配置节点：
//
//	endpoint, err := dep2p.New(
//	    dep2p.WithPreset(dep2p.PresetDesktop),
//	    dep2p.WithListenPort(4001),
//	)
//
// 如果不传入任何选项，将使用 PresetDesktop 作为默认配置。
func New(opts ...Option) (Endpoint, error) {
	// 解析用户选项
	options := newOptions()
	for _, opt := range opts {
		if err := opt(options); err != nil {
			return nil, fmt.Errorf("应用选项失败: %w", err)
		}
	}

	// 如果没有设置预设，使用默认预设
	if options.preset == nil {
		options.preset = PresetDesktop
	}

	// 转换为内部配置
	internalConfig := options.toInternalConfig()

	// 创建 bootstrap
	bootstrap := app.NewBootstrap(internalConfig)

	// 构建并返回 Endpoint
	endpoint, err := bootstrap.Build()
	if err != nil {
		return nil, fmt.Errorf("构建节点失败: %w", err)
	}

	return endpoint, nil
}

// Start 创建并启动一个 dep2p 节点
//
// 这是 New() + Listen() 的便捷方法：
//
//	endpoint, err := dep2p.Start(ctx,
//	    dep2p.WithPreset(dep2p.PresetDesktop),
//	)
func Start(ctx context.Context, opts ...Option) (Endpoint, error) {
	endpoint, err := New(opts...)
	if err != nil {
		return nil, err
	}

	if err := endpoint.Listen(ctx); err != nil {
		_ = endpoint.Close()
		return nil, fmt.Errorf("启动监听失败: %w", err)
	}

	return endpoint, nil
}

// QuickStart 使用默认配置快速启动节点
//
// 这是最简单的启动方式，适用于快速原型开发：
//
//	endpoint, err := dep2p.QuickStart(ctx)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer endpoint.Close()
//
// 等价于:
//
//	dep2p.Start(ctx, dep2p.WithPreset(dep2p.PresetDesktop))
func QuickStart(ctx context.Context) (Endpoint, error) {
	return Start(ctx, WithPreset(PresetDesktop))
}

// ============================================================================
//                              Facade: Node（推荐）
// ============================================================================

// NewNode 创建一个新的 dep2p Node（Facade）。
//
// Node 在 Endpoint 的最小稳定接口之上，提供 Send/Request/Publish/Subscribe 等高层 API，
// 并且持有 fx Runtime 的 Stop 句柄，Close 时会正确释放资源。
//
// 优雅下线: 调用 Close() 时会先发送 Goodbye 消息，等待传播后再断开连接。
// 可通过 WithGoodbyeWait() 配置等待时间。
func NewNode(opts ...Option) (*Node, error) {
	// 解析用户选项（与 New 保持一致）
	options := newOptions()
	for _, opt := range opts {
		if err := opt(options); err != nil {
			return nil, fmt.Errorf("应用选项失败: %w", err)
		}
	}

	if options.preset == nil {
		options.preset = PresetDesktop
	}

	internalConfig := options.toInternalConfig()
	bootstrap := app.NewBootstrap(internalConfig)
	rt, err := bootstrap.BuildRuntime()
	if err != nil {
		return nil, fmt.Errorf("构建节点失败: %w", err)
	}

	return &Node{
		rt:          rt,
		goodbyeWait: options.shutdown.goodbyeWait,
	}, nil
}

// StartNode 创建并启动 dep2p Node（Facade）。
//
// 等价于 NewNode() + node.Endpoint().Listen()
func StartNode(ctx context.Context, opts ...Option) (*Node, error) {
	node, err := NewNode(opts...)
	if err != nil {
		return nil, err
	}

	if err := node.Endpoint().Listen(ctx); err != nil {
		_ = node.Close()
		return nil, fmt.Errorf("启动监听失败: %w", err)
	}

	return node, nil
}

// QuickStartNode 使用默认配置快速启动 Node（Facade）。
func QuickStartNode(ctx context.Context) (*Node, error) {
	return StartNode(ctx, WithPreset(PresetDesktop))
}

// ============================================================================
//                              辅助函数
// ============================================================================

// MustNew 创建节点，失败时 panic
// 仅用于测试或确定不会失败的场景
func MustNew(opts ...Option) Endpoint {
	ep, err := New(opts...)
	if err != nil {
		panic(fmt.Sprintf("dep2p.MustNew failed: %v", err))
	}
	return ep
}

// MustStart 创建并启动节点，失败时 panic
// 仅用于测试或确定不会失败的场景
func MustStart(ctx context.Context, opts ...Option) Endpoint {
	ep, err := Start(ctx, opts...)
	if err != nil {
		panic(fmt.Sprintf("dep2p.MustStart failed: %v", err))
	}
	return ep
}

// ============================================================================
//                              地址解析
// ============================================================================

// 注意：ParseAddress、ParseAddresses、MustParseAddress 函数已移至 Node 方法
//
// 正确用法（通过 Node 获取）：
//
//	node, err := dep2p.NewNode(ctx, opts...)
//	addr, err := node.ParseAddress("192.168.1.1:8000")
//	addrs, err := node.ParseAddresses([]string{"192.168.1.1:8000"})
//
// 这样确保 AddressParser 通过 Fx 依赖注入获取，符合架构规范。
