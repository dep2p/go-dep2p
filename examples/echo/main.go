// Package main 提供 Echo 协议示例
//
// 这是一个简单的 Echo 服务示例，演示 dep2p 的基本用法与 DialBy 三语义中的两条（用户路径）：
// - DialByFullAddress（推荐，冷启动/分享/Bootstrap）：ConnectToAddr(fullAddr)
// - DialByNodeID（常规业务）：Connect(nodeID)
//
// 注意：ConnectWithAddrs（DialByNodeIDWithDialAddrs）属于高级/运维/受控环境用法，不在 examples 中展示；
// 相关说明请见 docs/04-usage/examples/advanced.md。
// 支持两种模式：
// - listener: 监听模式，等待连接并回显消息
// - dialer: 拨号模式，连接到远程节点并发送消息
//
// 使用方法:
//
//	# 终端 1: 启动监听节点
//	go run main.go -mode listener
//
//	# 终端 2（推荐，DialByFullAddress）：复制监听端输出的 fullAddr
//	go run main.go -mode dialer -fulladdr <FullAddress> -msg "Hello!"
//
//	# 终端 2（常规业务，DialByNodeID）：在同一局域网/可发现环境下，可直接用 NodeID 连接
//	go run main.go -mode dialer -remote <NodeID> -msg "Hello!"
//
// 参考: iroh examples/echo.rs, go-libp2p examples/echo
package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/dep2p/go-dep2p"
	"github.com/dep2p/go-dep2p/pkg/protocolids"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// ALPN 协议标识符 (v1.1 scope: sys - echo 是系统工具不需要 Realm)
// 引用 pkg/protocolids 唯一真源
var echoProtocol = protocolids.SysEcho

func main() {
	// 解析命令行参数
	mode := flag.String("mode", "listener", "运行模式: listener 或 dialer")
	listenPort := flag.Int("port", 0, "监听端口 (0 表示随机)")
	fullAddr := flag.String("fulladdr", "", "远程节点 Full Address（含 /p2p/<NodeID>，DialByFullAddress 推荐）")
	remoteID := flag.String("remote", "", "远程节点 NodeID（Base58，DialByNodeID 使用）")
	message := flag.String("msg", "Hello, dep2p!", "要发送的消息 (dialer 模式)")
	flag.Parse()

	fmt.Println("=== DeP2P Echo 示例 ===")
	fmt.Println()

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

	switch *mode {
	case "listener":
		runListener(ctx, *listenPort)
	case "dialer":
		// DialByFullAddress（推荐）
		if *fullAddr != "" {
			runDialerByFullAddr(ctx, *fullAddr, *message)
			return
		}
		// DialByNodeID（常规业务）
		if *remoteID == "" {
			fmt.Println("错误: dialer 模式需要 -fulladdr（推荐）或提供 -remote（DialByNodeID）")
			flag.Usage()
			os.Exit(1)
		}
		runDialerByNodeID(ctx, *remoteID, *message)
	default:
		fmt.Printf("未知模式: %s\n", *mode)
		flag.Usage()
		os.Exit(1)
	}
}

// runListener 运行监听模式
func runListener(ctx context.Context, port int) {
	fmt.Println("[Listener] 启动中...")

	// 配置（使用 QUIC 传输）
	opts := []dep2p.Option{
		dep2p.WithPreset(dep2p.PresetTest),
	}
	if port > 0 {
		opts = append(opts, dep2p.WithListenPort(port))
	}
	// port == 0 时使用预设的默认随机端口

	// 创建并启动节点（Node Facade，使用 QUIC 传输）
	node, err := dep2p.StartNode(ctx, opts...)
	if err != nil {
		fmt.Printf("启动节点失败: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = node.Close() }()

	// 打印节点信息
	printNodeInfo(node.Endpoint())
	// 输出可分享的 Full Address（DialByFullAddress）
	if addrs := node.ShareableAddrs(); len(addrs) > 0 {
		fmt.Printf("[Listener] 分享给拨号端的 Full Address:\n  %s\n\n", addrs[0])
	}

	// 注册 Echo 处理器（通过 Endpoint）
	node.Endpoint().SetProtocolHandler(echoProtocol, func(stream dep2p.Stream) {
		handleEchoStream(stream)
	})

	fmt.Println("[Listener] 等待连接...")
	fmt.Println()
	fmt.Println("使用以下命令从另一个终端连接:")
	if addrs := node.ShareableAddrs(); len(addrs) > 0 {
		fmt.Printf("  go run main.go -mode dialer -fulladdr %q -msg %q\n", addrs[0], "Hello, dep2p!")
	} else {
		fmt.Printf("  go run main.go -mode dialer -remote %s -msg %q\n", node.ID(), "Hello, dep2p!")
		fmt.Println("  # 提示：若 DialByNodeID 在你的网络环境不可发现，请改用 -fulladdr（推荐）")
	}
	fmt.Println()

	// 等待上下文取消
	<-ctx.Done()
	fmt.Println("[Listener] 已关闭")
}

// runDialerByFullAddr 运行拨号模式（DialByFullAddress）
func runDialerByFullAddr(ctx context.Context, fullAddr string, message string) {
	fmt.Println("[Dialer] 启动中...")

	// 创建并启动节点（Node Facade，使用 QUIC 传输）
	node, err := dep2p.StartNode(ctx,
		dep2p.WithPreset(dep2p.PresetTest),
		// 使用默认随机端口，不指定监听地址
	)
	if err != nil {
		fmt.Printf("启动节点失败: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = node.Close() }()

	fmt.Printf("[Dialer] 本地节点 ID: %s\n", node.ID())
	fmt.Printf("[Dialer] DialByFullAddress: ConnectToAddr(%s)\n", fullAddr)

	// 连接到远程节点
	fmt.Println("[Dialer] 正在连接...")
	connectCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	conn, err := node.ConnectToAddr(connectCtx, fullAddr)
	if err != nil {
		fmt.Printf("连接失败: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("[Dialer] 连接成功!")

	// 打开流
	stream, err := conn.OpenStream(ctx, echoProtocol)
	if err != nil {
		fmt.Printf("打开流失败: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = stream.Close() }()

	// 发送消息
	fmt.Printf("[Dialer] 发送: %s\n", message)
	_, err = stream.Write([]byte(message))
	if err != nil {
		fmt.Printf("发送失败: %v\n", err)
		os.Exit(1)
	}

	// 读取响应
	response := make([]byte, len(message)+100)
	n, err := stream.Read(response)
	if err != nil && err != io.EOF {
		fmt.Printf("读取响应失败: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("[Dialer] 收到响应: %s\n", string(response[:n]))

	// 验证
	if string(response[:n]) == message {
		fmt.Println("[Dialer] ✓ Echo 验证成功!")
	} else {
		fmt.Println("[Dialer] ✗ Echo 验证失败!")
	}

	fmt.Println("[Dialer] 完成")
}

// runDialerByNodeID 运行拨号模式（DialByNodeID）
//
// 注意：该模式依赖 AddressBook/Discovery（如 mDNS）获取对端地址；在不可发现网络环境下建议使用 DialByFullAddress。
func runDialerByNodeID(ctx context.Context, remoteID, message string) {
	fmt.Println("[Dialer] 启动中...")

	node, err := dep2p.StartNode(ctx,
		dep2p.WithPreset(dep2p.PresetTest),
	)
	if err != nil {
		fmt.Printf("启动节点失败: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = node.Close() }()

	fmt.Printf("[Dialer] 本地节点 ID: %s\n", node.ID())
	fmt.Printf("[Dialer] DialByNodeID: Connect(%s)\n", remoteID)

	targetID, err := types.ParseNodeID(remoteID)
	if err != nil {
		fmt.Printf("解析 NodeID 失败: %v\n", err)
		os.Exit(1)
	}

	connectCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	conn, err := node.Connect(connectCtx, targetID)
	if err != nil {
		fmt.Printf("连接失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("[Dialer] 连接成功!")

	stream, err := conn.OpenStream(ctx, echoProtocol)
	if err != nil {
		fmt.Printf("打开流失败: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = stream.Close() }()

	fmt.Printf("[Dialer] 发送: %s\n", message)
	_, err = stream.Write([]byte(message))
	if err != nil {
		fmt.Printf("发送失败: %v\n", err)
		os.Exit(1)
	}

	response := make([]byte, len(message)+100)
	n, err := stream.Read(response)
	if err != nil && err != io.EOF {
		fmt.Printf("读取响应失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("[Dialer] 收到响应: %s\n", string(response[:n]))
	if string(response[:n]) == message {
		fmt.Println("[Dialer] ✓ Echo 验证成功!")
	} else {
		fmt.Println("[Dialer] ✗ Echo 验证失败!")
	}
	fmt.Println("[Dialer] 完成")
}

// handleEchoStream 处理 Echo 流
func handleEchoStream(stream dep2p.Stream) {
	defer func() { _ = stream.Close() }()

	fmt.Printf("[Handler] 收到来自连接的流\n")

	// 读取并回显
	reader := bufio.NewReader(stream)
	for {
		// 设置读取超时
		_ = stream.SetReadDeadline(time.Now().Add(30 * time.Second))

		buf := make([]byte, 1024)
		n, err := reader.Read(buf)
		if err != nil {
			if err != io.EOF {
				fmt.Printf("[Handler] 读取错误: %v\n", err)
			}
			return
		}

		data := buf[:n]
		fmt.Printf("[Handler] 收到: %s\n", string(data))

		// 回显
		_, err = stream.Write(data)
		if err != nil {
			fmt.Printf("[Handler] 写入错误: %v\n", err)
			return
		}

		fmt.Printf("[Handler] 已回显 %d 字节\n", n)
	}
}

// printNodeInfo 打印节点信息
func printNodeInfo(node dep2p.Endpoint) {
	fmt.Println()
	fmt.Println("╔════════════════════════════════════════════════════════╗")
	fmt.Println("║                    节点信息                             ║")
	fmt.Println("╠════════════════════════════════════════════════════════╣")
	fmt.Printf("║ 节点 ID: %s\n", node.ID())
	fmt.Println("║ 监听地址:")
	for i, addr := range node.ListenAddrs() {
		fmt.Printf("║   [%d] %s\n", i+1, addr)
	}
	fmt.Println("╚════════════════════════════════════════════════════════╝")
	fmt.Println()
}

// 注意：不再提供 “DialByNodeIDWithDialAddrs” 示例，因此不需要从 ListenAddrs 导出 DialAddr。
