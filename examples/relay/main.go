// Package main 提供 Relay 中继示例
//
// 这个示例演示如何使用 dep2p 的 Relay 功能，让两个 NAT 后的节点
// 通过中继服务器建立连接。
//
// 使用方法:
//
//	# 1. 首先启动 Relay 服务器
//	go run ./cmd/relay-server -port 4001
//
//	# 2. 启动第一个客户端（监听模式）
//	go run main.go -mode listen -relay /ip4/127.0.0.1/udp/4001/quic-v1/p2p/<relay-id>
//
//	# 3. 启动第二个客户端（连接模式）
//	go run main.go -mode dial -relay /ip4/127.0.0.1/udp/4001/quic-v1/p2p/<relay-id> -target <target-id>
//
// 参考: go-libp2p examples/relay
package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/dep2p/go-dep2p"
	"github.com/dep2p/go-dep2p/pkg/protocolids"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// 引用 pkg/protocolids 唯一真源
var relayProtocol = protocolids.AppRelayDemo

func main() {
	// 解析命令行参数
	mode := flag.String("mode", "listen", "运行模式: listen 或 dial")
	relayAddr := flag.String("relay", "", "Relay 服务器地址（multiaddr 格式，如 /ip4/.../p2p/<relay-id>）")
	targetID := flag.String("target", "", "目标节点 ID (dial 模式需要)")
	flag.Parse()

	fmt.Println("╔══════════════════════════════════════════════════════╗")
	fmt.Println("║            DeP2P Relay 示例                          ║")
	fmt.Println("╚══════════════════════════════════════════════════════╝")
	fmt.Println()

	if *relayAddr == "" {
		fmt.Println("❌ 错误: 需要指定 -relay 参数")
		fmt.Println()
		fmt.Println("请先启动 Relay 服务器:")
		fmt.Println("  go run ./cmd/relay-server -port 4001")
		fmt.Println()
		flag.Usage()
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 捕获中断信号
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-signalCh
		fmt.Println("\n\n再见! 👋")
		cancel()
	}()

	switch *mode {
	case "listen":
		runListener(ctx, *relayAddr)
	case "dial":
		if *targetID == "" {
			fmt.Println("❌ 错误: dial 模式需要 -target 参数")
			flag.Usage()
			os.Exit(1)
		}
		runDialer(ctx, *relayAddr, *targetID)
	default:
		fmt.Printf("❌ 未知模式: %s\n", *mode)
		flag.Usage()
		os.Exit(1)
	}
}

// runListener 运行监听模式
func runListener(ctx context.Context, relayAddr string) {
	fmt.Println("[Listener] 启动中...")

	// 创建并启动节点（Node Facade，使用 QUIC 传输），启用 Relay
	node, err := dep2p.StartNode(ctx,
		dep2p.WithPreset(dep2p.PresetDesktop),
		dep2p.WithRelay(true),
	)
	if err != nil {
		fmt.Printf("❌ 启动节点失败: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = node.Close() }()

	// IMPL-1227: 加入 Realm（使用新 API）
	// 使用 DeriveRealmKeyFromName 从 realm 名称派生密钥，确保 Listener 和 Dialer 能互相认证
	realmKey := types.DeriveRealmKeyFromName("relay-demo")
	realm, err := node.JoinRealmWithKey(ctx, "relay-demo", realmKey)
	if err != nil {
		fmt.Printf("⚠️  加入 Realm 失败: %v\n", err)
	} else {
		fmt.Printf("[Listener] 已加入 Realm: %s (ID: %s)\n", realm.Name(), realm.ID())
	}

	fmt.Printf("✅ 节点 ID: %s\n", node.ID())
	fmt.Println()

	// 验证并解析 Relay NodeID（用于 Reserve/Connect）
	relayIDParsed, err := parseRelayAddress(relayAddr)
	if err != nil {
		fmt.Printf("❌ Relay 地址无效: %v\n", err)
		os.Exit(1)
	}

	// DialByFullAddress：连接到 Relay 服务器（输入必须是 Full Address）
	fmt.Printf("DialByFullAddress: ConnectToAddr(%s)\n", relayAddr)
	relayConn, err := node.ConnectToAddr(ctx, relayAddr)
	if err != nil {
		fmt.Printf("❌ 连接到 Relay 失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("✅ 已连接到 Relay")
	relayID := relayConn.RemoteID()
	// 防御：确保连接到的确实是期望 Relay
	if relayID != relayIDParsed {
		fmt.Printf("❌ Relay 身份不匹配: expected=%s actual=%s\n", relayIDParsed.ShortString(), relayID.ShortString())
		os.Exit(1)
	}

	// 预留中继资源
	relayClient := node.Relay()
	if relayClient == nil {
		fmt.Println("❌ Relay 客户端未启用")
		os.Exit(1)
	}

	reservation, err := relayClient.Reserve(ctx, relayID)
	if err != nil {
		fmt.Printf("❌ 预留 Relay 资源失败: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✅ 预留成功，过期时间: %v\n", reservation.Expiry())
	if addrs := reservation.Addrs(); len(addrs) > 0 {
		fmt.Println("📡 中继地址:")
		for _, addr := range addrs {
			fmt.Printf("   • %s\n", addr)
		}
	}

	fmt.Println()
	fmt.Println("╔══════════════════════════════════════════════════════╗")
	fmt.Println("║ 其他节点可以通过以下命令连接:                          ║")
	fmt.Printf("║   -mode dial -relay %s -target %s\n", relayAddr, node.ID())
	fmt.Println("╚══════════════════════════════════════════════════════╝")
	fmt.Println()

	// 注册协议处理器（通过 Endpoint）
	node.Endpoint().SetProtocolHandler(relayProtocol, func(stream dep2p.Stream) {
		handleStream(stream)
	})

	fmt.Println("等待连接...")

	<-ctx.Done()
}

// runDialer 运行拨号模式
//
// 演示 "host 风格" 的中继连接：
// 1. 构建 relay circuit 地址 (/…/p2p/<relay>/p2p-circuit/p2p/<target>)
// 2. 使用 node.ConnectWithAddrs() 连接到目标（Endpoint 会自动选择 RelayTransport）
// 3. 在 endpoint.Connection 上 OpenStream()，得到应用层流
//
// 这与 libp2p 的 host.Connect() + host.NewStream() 语义一致。
func runDialer(ctx context.Context, relayAddr, targetIDStr string) {
	fmt.Println("[Dialer] 启动中...")

	// 创建并启动节点（Node Facade，使用 QUIC 传输），启用 Relay
	node, err := dep2p.StartNode(ctx,
		dep2p.WithPreset(dep2p.PresetDesktop),
		dep2p.WithRelay(true),
	)
	if err != nil {
		fmt.Printf("❌ 启动节点失败: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = node.Close() }()

	// IMPL-1227: 加入 Realm（使用新 API）
	// 使用 DeriveRealmKeyFromName 从 realm 名称派生密钥，确保 Listener 和 Dialer 能互相认证
	realmKey := types.DeriveRealmKeyFromName("relay-demo")
	realm, err := node.JoinRealmWithKey(ctx, "relay-demo", realmKey)
	if err != nil {
		fmt.Printf("⚠️  加入 Realm 失败: %v\n", err)
	} else {
		fmt.Printf("[Dialer] 已加入 Realm: %s (ID: %s)\n", realm.Name(), realm.ID())
	}
	_ = realm // 可用于后续服务访问

	fmt.Printf("✅ 节点 ID: %s\n", node.ID())
	fmt.Println()

	relayIDParsed, err := parseRelayAddress(relayAddr)
	if err != nil {
		fmt.Printf("❌ Relay 地址无效: %v\n", err)
		os.Exit(1)
	}

	// DialByFullAddress：连接到 Relay 服务器（输入必须是 Full Address）
	// 这一步是为了让 Endpoint 知道 Relay 节点的地址，后续 ConnectWithAddrs 才能找到 relay
	fmt.Printf("DialByFullAddress: ConnectToAddr(%s)\n", relayAddr)
	relayConn, err := node.ConnectToAddr(ctx, relayAddr)
	if err != nil {
		fmt.Printf("❌ 连接到 Relay 失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("✅ 已连接到 Relay")
	relayID := relayConn.RemoteID()
	if relayID != relayIDParsed {
		fmt.Printf("❌ Relay 身份不匹配: expected=%s actual=%s\n", relayIDParsed.ShortString(), relayID.ShortString())
		os.Exit(1)
	}

	// 解析目标 NodeID（Base58）
	targetID, err := types.ParseNodeID(targetIDStr)
	if err != nil {
		fmt.Printf("❌ 解析目标 NodeID 失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("正在通过 Relay 连接到目标: %s\n", targetID.ShortString())

	// ============================================================================
	// Host 风格连接：构建 relay circuit 地址，通过 ConnectWithAddrs 连接
	// ============================================================================
	// 构建 relay circuit 地址: /ip4/.../p2p/<relay>/p2p-circuit/p2p/<target>
	relayCircuitAddr := buildRelayCircuitAddr(relayAddr, targetID)
	fmt.Printf("RelayCircuit 地址: %s\n", relayCircuitAddr)

	// 使用 ConnectWithAddrs 连接：Endpoint 会根据地址选择 RelayTransport
	conn, err := node.ConnectWithAddrs(ctx, targetID, []string{relayCircuitAddr})
	if err != nil {
		fmt.Printf("❌ 通过 Relay 连接失败: %v\n", err)
		fmt.Println()
		fmt.Println("提示：确保目标节点已在 Relay 上预留资源（-mode listen）")
		os.Exit(1)
	}
	fmt.Printf("✅ 已通过 Relay 连接到 %s\n", targetID.ShortString())

	// 在 endpoint.Connection 上打开应用层流
	stream, err := conn.OpenStream(ctx, relayProtocol)
	if err != nil {
		fmt.Printf("❌ 打开流失败: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = stream.Close() }()

	fmt.Println()
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("输入消息并按回车发送，输入 /quit 退出")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println()

	// 启动读取 goroutine
	go func() {
		reader := bufio.NewReader(stream)
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				if err != io.EOF {
					fmt.Printf("❌ 读取错误: %v\n", err)
				}
				return
			}
			fmt.Printf("📨 收到: %s", line)
		}
	}()

	// 读取用户输入并发送
	inputReader := bufio.NewReader(os.Stdin)
	for {
		select {
		case <-ctx.Done():
			return
		default:
			fmt.Print("> ")
			line, err := inputReader.ReadString('\n')
			if err != nil {
				return
			}

			line = strings.TrimSpace(line)
			if line == "/quit" || line == "/exit" {
				return
			}

			if line != "" {
				_, err = stream.Write([]byte(line + "\n"))
				if err != nil {
					fmt.Printf("❌ 发送失败: %v\n", err)
					return
				}
			}
		}
	}
}

// buildRelayCircuitAddr 构建 relay circuit 地址
//
// 输入:
//   - relayAddr: Relay 服务器的 Full Address，如 /ip4/127.0.0.1/udp/4001/quic-v1/p2p/<relay-id>
//   - targetID: 目标节点 ID
//
// 输出:
//   - /ip4/127.0.0.1/udp/4001/quic-v1/p2p/<relay-id>/p2p-circuit/p2p/<target-id>
func buildRelayCircuitAddr(relayAddr string, targetID types.NodeID) string {
	return relayAddr + "/p2p-circuit/p2p/" + targetID.String()
}

// handleStream 处理入站流
func handleStream(stream dep2p.Stream) {
	defer func() { _ = stream.Close() }()

	conn := stream.Connection()
	if conn == nil {
		return
	}

	remoteID := conn.RemoteID().String()[:8]
	fmt.Printf("📥 收到来自 %s 的连接\n", remoteID)

	reader := bufio.NewReader(stream)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err != io.EOF {
				fmt.Printf("❌ 读取错误: %v\n", err)
			}
			return
		}

		fmt.Printf("[%s] %s", remoteID, line)

		// 回复确认
		_, err = stream.Write([]byte("ACK\n"))
		if err != nil {
			return
		}
	}
}

// parseRelayAddress 解析 Relay 地址
//
// 从 multiaddr 格式的地址中提取 NodeID 和基础地址
// 输入格式: /ip4/127.0.0.1/udp/4001/quic-v1/p2p/<relay-id>
// 输出: NodeID, 基础地址（不含 /p2p/...）, error
func parseRelayAddress(addr string) (types.NodeID, error) {
	// Relay 地址必须是 Full Address（含 /p2p/<NodeID>）
	p2pIndex := strings.LastIndex(addr, "/p2p/")
	if p2pIndex == -1 {
		return types.EmptyNodeID, fmt.Errorf("地址格式错误：缺少 /p2p/<node-id>")
	}
	nodeIDStr := addr[p2pIndex+5:]
	if nodeIDStr == "" {
		return types.EmptyNodeID, fmt.Errorf("地址格式错误：NodeID 为空")
	}
	nodeID, err := types.ParseNodeID(nodeIDStr)
	if err != nil {
		return types.EmptyNodeID, fmt.Errorf("解析 NodeID 失败: %w", err)
	}
	return nodeID, nil
}

// waitForRealmAuth 等待 Realm 认证完成
func waitForRealmAuth(conn dep2p.Connection, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		realmCtx := conn.RealmContext()
		if realmCtx.IsValid() {
			return true
		}
		time.Sleep(50 * time.Millisecond)
	}
	return false
}
