// Package main 提供 dep2p 简单示例
//
// 这是一个展示 dep2p API 基本用法的示例
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/dep2p/go-dep2p"
	"github.com/dep2p/go-dep2p/pkg/protocolids"
	"github.com/dep2p/go-dep2p/pkg/types"
)

func main() {
	fmt.Println("=== DeP2P 简单示例 ===")
	fmt.Println()

	// 创建上下文
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 捕获中断信号
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-signalCh
		fmt.Println("\n收到中断信号，准备关闭...")
		cancel()
	}()

	// 使用 StartNode 创建并启动节点（Node Facade，推荐入口，使用 QUIC 传输）
	fmt.Println("正在创建 dep2p 节点...")
	node, err := dep2p.StartNode(ctx,
		dep2p.WithPreset(dep2p.PresetDesktop),
		// v1.1+ 强制内建：Realm 为底层必备能力，用户无需配置启用开关
		// 使用默认随机端口（QUIC）
	)
	if err != nil {
		fmt.Printf("启动节点失败: %v\n", err)
		fmt.Println()
		fmt.Println("注意: dep2p 核心框架正在开发中，部分功能可能尚未实现。")
		os.Exit(1)
	}
	defer func() { _ = node.Close() }()

	// 打印节点信息
	fmt.Printf("节点 ID: %s\n", node.ID())
	fmt.Println()

	// 打印监听地址
	addrs := node.ListenAddrs()
	if len(addrs) > 0 {
		fmt.Println("监听地址:")
		for i, addr := range addrs {
			fmt.Printf("  [%d] %s\n", i+1, addr)
		}
	} else {
		fmt.Println("(暂无监听地址)")
	}
	fmt.Println()

	// 加入 Realm（必须！业务 API 需要）
	// IMPL-1227: 使用新 API，必须提供 realmKey
	fmt.Println("加入 Realm...")
	// 使用 DeriveRealmKeyFromName 从 realm 名称派生密钥，确保同名 Realm 的节点能互相认证
	realmKey := types.DeriveRealmKeyFromName("basic-demo")
	realm, err := node.JoinRealmWithKey(ctx, "basic-demo", realmKey)
	if err != nil {
		fmt.Printf("⚠️  加入 Realm 失败: %v\n", err)
	} else {
		fmt.Printf("✅ 已加入 Realm: %s (ID: %s)\n", realm.Name(), realm.ID())
	}
	fmt.Println()

	// 设置协议处理器（通过 Endpoint）
	// 注意：这是一个简单示例，仅演示协议注册。完整的 echo 实现请参考 examples/echo/
	// 引用 pkg/protocolids 唯一真源
	node.Endpoint().SetProtocolHandler(protocolids.SysEcho, func(stream dep2p.Stream) {
		defer func() { _ = stream.Close() }()
		fmt.Println("收到新的流连接")
		// 简单 echo：读取数据并回写
		buf := make([]byte, 1024)
		for {
			n, err := stream.Read(buf)
			if err != nil {
				return
			}
			_, _ = stream.Write(buf[:n])
		}
	})
	fmt.Printf("已注册 %s 协议处理器\n", protocolids.SysEcho)
	fmt.Println()

	// 打印使用说明
	fmt.Println("=== 节点已启动 ===")
	fmt.Println("按 Ctrl+C 退出")
	fmt.Println()

	// 等待上下文取消
	<-ctx.Done()
	fmt.Println("节点已关闭")
}
