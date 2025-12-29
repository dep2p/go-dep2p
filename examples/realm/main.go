// Package main 提供 Realm 入门示例
//
// 这是一个最小化的示例，演示 DeP2P 的 Realm 强制隔离机制。
//
// 使用方法:
//
//	go run main.go
//
// 你将看到：
// 1. 节点创建和启动
// 2. 未 JoinRealm 时业务 API 返回 ErrNotMember
// 3. JoinRealm 后业务 API 可用
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/dep2p/go-dep2p"
	"github.com/dep2p/go-dep2p/pkg/interfaces/endpoint"
	"github.com/dep2p/go-dep2p/pkg/types"
)

func main() {
	fmt.Println("╔══════════════════════════════════════════════╗")
	fmt.Println("║      DeP2P Realm 入门示例                     ║")
	fmt.Println("╚══════════════════════════════════════════════╝")
	fmt.Println()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// ========================================
	// Step 1: 创建 Node（不是 Endpoint）
	// ========================================
	fmt.Println("━━━ Step 1: 创建 Node ━━━")
	fmt.Println("使用 dep2p.StartNode（Node Facade）而非 dep2p.Start（Endpoint）")
	fmt.Println()

	node, err := dep2p.StartNode(ctx,
		dep2p.WithPreset(dep2p.PresetDesktop),
		// v1.1+ 强制内建：Realm 为底层必备能力，用户无需配置启用开关
		// 使用默认随机端口（QUIC 传输）
	)
	if err != nil {
		fmt.Printf("❌ 启动节点失败: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = node.Close() }()

	fmt.Printf("✅ 节点已创建并启动\n")
	fmt.Printf("   节点 ID: %s\n", node.ID())
	fmt.Printf("   监听地址: %v\n", node.ListenAddrs())
	fmt.Println()

	// ========================================
	// Step 2: 验证 Realm 强制隔离
	// ========================================
	fmt.Println("━━━ Step 2: 验证强制隔离 ━━━")
	fmt.Println("未 JoinRealm 时，业务 API 必须返回 ErrNotMember")
	fmt.Println()

	// 创建一个测试目标节点（只用于填充参数）
	targetNode, err := dep2p.StartNode(ctx,
		dep2p.WithPreset(dep2p.PresetDesktop),
		// v1.1+ 强制内建：Realm 为底层必备能力，用户无需配置启用开关
		// 使用默认随机端口（QUIC 传输）
	)
	if err != nil {
		fmt.Printf("❌ 启动目标节点失败: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = targetNode.Close() }()

	targetID := targetNode.ID()

	// IMPL-1227: 测试 Send（未 Join 时）
	// 新 API: node.Send(ctx, nodeID, data) 不再需要 protocol 参数
	fmt.Printf("尝试调用 Send...\n")
	err = node.Send(ctx, targetID, []byte("hello"))
	if err != nil {
		if err == endpoint.ErrNotMember {
			fmt.Printf("  ✅ Send 正确返回: %v\n", err)
		} else {
			fmt.Printf("  ⚠️  Send 返回错误（非预期）: %v\n", err)
		}
	} else {
		fmt.Printf("  ❌ Send 未返回错误（这不应该发生！）\n")
	}

	// 测试 Publish（未 Join 时）
	fmt.Printf("尝试调用 Publish...\n")
	err = node.Publish(ctx, "test-topic", []byte("message"))
	if err != nil {
		if err == endpoint.ErrNotMember {
			fmt.Printf("  ✅ Publish 正确返回: %v\n", err)
		} else {
			fmt.Printf("  ⚠️  Publish 返回错误（非预期）: %v\n", err)
		}
	} else {
		fmt.Printf("  ❌ Publish 未返回错误（这不应该发生！）\n")
	}

	// 测试 Subscribe（未 Join 时）
	fmt.Printf("尝试调用 Subscribe...\n")
	_, err = node.Subscribe(ctx, "test-topic")
	if err != nil {
		if err == endpoint.ErrNotMember {
			fmt.Printf("  ✅ Subscribe 正确返回: %v\n", err)
		} else {
			fmt.Printf("  ⚠️  Subscribe 返回错误（非预期）: %v\n", err)
		}
	} else {
		fmt.Printf("  ❌ Subscribe 未返回错误（这不应该发生！）\n")
	}

	fmt.Println()

	// ========================================
	// Step 3: 加入 Realm
	// ========================================
	fmt.Println("━━━ Step 3: 加入 Realm ━━━")
	fmt.Println()

	// IMPL-1227: 使用新 API 加入 Realm
	// 使用 DeriveRealmKeyFromName 从 realm 名称派生密钥，确保同名 Realm 的节点能互相认证
	realmKey := types.DeriveRealmKeyFromName("my-app-realm")
	fmt.Printf("正在加入 Realm: my-app-realm (使用从名称派生的 realmKey)\n")

	rm := node.Realm()
	if rm == nil {
		fmt.Printf("❌ RealmManager 不可用（Realm 未启用）\n")
		os.Exit(1)
	}

	realm, err := rm.JoinRealmWithKey(ctx, "my-app-realm", realmKey)
	if err != nil {
		fmt.Printf("⚠️  JoinRealm 返回错误: %v\n", err)
		fmt.Println("   （这可能是正常的，取决于实现）")
	} else {
		fmt.Printf("✅ 成功加入 Realm\n")
	}

	// 验证成员状态
	if rm.IsMember() && realm != nil {
		fmt.Printf("   当前 Realm: %s (ID: %s)\n", realm.Name(), realm.ID())
	} else {
		fmt.Printf("   ⚠️  未成为成员（可能需要检查实现）\n")
	}

	fmt.Println()

	// ========================================
	// Step 4: 业务 API 现在可用
	// ========================================
	fmt.Println("━━━ Step 4: 业务 API 可用 ━━━")
	fmt.Println("加入 Realm 后，业务 API 不再返回 ErrNotMember")
	fmt.Println()

	// IMPL-1227: 让目标节点也加入同一个 Realm（必须使用相同的 realmKey）
	targetRM := targetNode.Realm()
	if targetRM != nil {
		_, _ = targetRM.JoinRealmWithKey(ctx, "my-app-realm", realmKey)
	}

	// 测试 Publish（Join 后）
	fmt.Printf("尝试调用 Publish...\n")
	err = node.Publish(ctx, "test-topic", []byte("message"))
	if err != nil {
		if err == endpoint.ErrNotMember {
			fmt.Printf("  ❌ Publish 仍返回 ErrNotMember（不应该！）\n")
		} else {
			fmt.Printf("  ⚠️  Publish 返回其他错误: %v\n", err)
			fmt.Println("     （可能是 GossipSub 未就绪，这是正常的）")
		}
	} else {
		fmt.Printf("  ✅ Publish 成功\n")
	}

	// 测试 Subscribe（Join 后）
	fmt.Printf("尝试调用 Subscribe...\n")
	_, err = node.Subscribe(ctx, "test-topic")
	if err != nil {
		if err == endpoint.ErrNotMember {
			fmt.Printf("  ❌ Subscribe 仍返回 ErrNotMember（不应该！）\n")
		} else {
			fmt.Printf("  ⚠️  Subscribe 返回其他错误: %v\n", err)
			fmt.Println("     （可能是 GossipSub 未就绪，这是正常的）")
		}
	} else {
		fmt.Printf("  ✅ Subscribe 成功\n")
	}

	fmt.Println()

	// ========================================
	// 总结
	// ========================================
	fmt.Println("╔══════════════════════════════════════════════╗")
	fmt.Println("║              Realm 入门示例完成               ║")
	fmt.Println("╚══════════════════════════════════════════════╝")
	fmt.Println()
	fmt.Println("关键要点:")
	fmt.Println("1. 使用 dep2p.StartNode（Node Facade）而非 dep2p.Start（Endpoint）")
	fmt.Println("2. Realm 为底层必备能力：无需用户启用")
	fmt.Println("3. 业务 API 必须先 JoinRealm 才能使用")
	fmt.Println("4. 未 Join 时会返回 endpoint.ErrNotMember")
	fmt.Println()
	fmt.Println("下一步: 查看 examples/echo/ 和 examples/chat/ 了解实际应用")
}

