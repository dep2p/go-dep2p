// Package main 提供 mDNS 自动发现聊天示例
//
// 这是一个交互式聊天示例，演示 dep2p 的 mDNS 发现功能。
// 在同一局域网内的节点会自动发现并连接。
//
// 使用方法:
//
//	# 终端 1
//	go run main.go
//
//	# 终端 2 (同一局域网)
//	go run main.go
//
// 两个节点会自动发现对方并建立连接，然后可以互相发送消息。
//
// 参考: go-libp2p examples/chat-with-mdns
package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/dep2p/go-dep2p"
	"github.com/dep2p/go-dep2p/pkg/protocolids"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// 协议标识符（应用协议，需要 Realm 校验）
// 引用 pkg/protocolids 唯一真源
var chatProtocol = protocolids.AppChat

// 服务名称（用于 mDNS 发现）
const serviceName = "dep2p-chat"

// 存储活跃的对等方
var (
	peers     = make(map[string]dep2p.Stream)
	peersLock sync.RWMutex
)

func main() {
	// 解析命令行参数
	port := flag.Int("port", 0, "监听端口 (0 表示随机)")
	nickname := flag.String("nick", "", "昵称 (默认使用节点ID前8位)")
	realmArg := flag.String("realm", "lan-chat", "Realm ID（聊天室）")
	logFile := flag.String("log-file", "", "日志文件路径（默认自动创建，留空则使用自动生成的文件名）")
	flag.Parse()

	fmt.Println("╔════════════════════════════════════════╗")
	fmt.Println("║     DeP2P Chat - mDNS 自动发现聊天     ║")
	fmt.Println("╚════════════════════════════════════════╝")
	fmt.Println()

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

	// 创建节点配置
	opts := []dep2p.Option{
		dep2p.WithPreset(dep2p.PresetDesktop),
		// v1.1+ 强制内建：mDNS 与 Realm 为底层必备能力，用户无需配置启用开关
	}

	// 自动创建日志文件
	logFilePath := *logFile
	if logFilePath == "" {
		// 自动生成日志文件名：chat-{timestamp}-{pid}.log
		// 使用时间戳和进程ID确保多个节点同时运行时不会冲突
		timestamp := time.Now().Format("20060102-150405")
		pid := os.Getpid()
		logFilePath = fmt.Sprintf("chat-%s-%d.log", timestamp, pid)

		// 创建 logs 目录
		logsDir := "logs"
		if err := os.MkdirAll(logsDir, 0750); err == nil {
			logFilePath = filepath.Join(logsDir, logFilePath)
		}
		// 如果创建目录失败，就在当前目录创建日志文件
	}

	// 打开日志文件，重定向 Go 标准库 log 包的输出（用于第三方库如 mDNS）
	logFileHandle, err := os.OpenFile(logFilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		fmt.Printf("⚠️  无法打开日志文件: %v\n", err)
		fmt.Println("   将继续使用控制台输出")
	} else {
		// 将 Go 标准库的 log 输出重定向到文件
		log.SetOutput(logFileHandle)
		log.SetFlags(log.LstdFlags)

		// 程序退出时关闭日志文件
		defer func() { _ = logFileHandle.Close() }()
	}

	// 配置 dep2p 日志文件
	opts = append(opts, dep2p.WithLogFile(logFilePath))
	fmt.Printf("📝 日志文件: %s\n", logFilePath)
	fmt.Println("   控制台仅显示交互信息")
	fmt.Println()

	if *port > 0 {
		opts = append(opts, dep2p.WithListenPort(*port))
	}

	// 创建并启动节点（Node Facade，使用 QUIC 传输）
	node, err := dep2p.StartNode(ctx, opts...)
	if err != nil {
		fmt.Printf("❌ 启动节点失败: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = node.Close() }()

	// IMPL-1227: 加入 Realm（使用新 API）
	// 使用 DeriveRealmKeyFromName 从 realm 名称派生密钥，确保同名聊天室的节点能互相认证
	realmKey := types.DeriveRealmKeyFromName(*realmArg)
	realm, err := node.JoinRealmWithKey(ctx, *realmArg, realmKey)
	if err != nil {
		fmt.Printf("❌ 加入 Realm 失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("🏠 已加入聊天室（Realm）: %s (ID: %s)\n", realm.Name(), realm.ID())

	// 设置昵称
	nick := *nickname
	if nick == "" {
		nick = node.ID().String()[:8]
	}

	// 打印节点信息
	fmt.Printf("✅ 节点已启动\n")
	fmt.Printf("📍 节点 ID: %s\n", node.ID())
	fmt.Printf("👤 昵称: %s\n", nick)
	fmt.Println()

	// 打印监听地址
	fmt.Println("📡 监听地址:")
	for _, addr := range node.ListenAddrs() {
		fmt.Printf("   • %s\n", addr)
	}
	fmt.Println()

	// 注册聊天协议处理器（处理入站连接，通过 Endpoint）
	node.Endpoint().SetProtocolHandler(chatProtocol, func(stream dep2p.Stream) {
		handleChatStream(stream)
	})

	// 注册 mDNS 发现回调，主动连接发现的节点
	discovery := node.Discovery()
	if discovery != nil {
		discovery.OnPeerDiscovered(func(peer dep2p.PeerInfo) {
			handlePeerDiscovered(ctx, node, peer)
		})
	}

	fmt.Println("🔍 正在通过 mDNS 搜索其他节点...")
	fmt.Println()
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("输入消息并按回车发送，输入 /quit 退出")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println()

	// 启动输入处理
	go handleInput(ctx, node, nick)

	// 等待上下文取消
	<-ctx.Done()
}

// handlePeerDiscovered 处理发现的节点，主动建立连接
func handlePeerDiscovered(ctx context.Context, node *dep2p.Node, peer dep2p.PeerInfo) {
	remoteID := peer.ID.String()[:8]

	// 检查是否已连接
	peersLock.RLock()
	_, exists := peers[remoteID]
	peersLock.RUnlock()
	if exists {
		return
	}

	fmt.Printf("🔍 发现节点: %s @ %v\n", remoteID, peer.Addrs)

	// DialByNodeID（默认、最纯粹）：仅用 NodeID 连接，地址由 AddressBook/Discovery 提供
	// mDNS 发现后的地址会写入地址簿，因此这里不需要暴露 Dial Address 给用户。
	conn, err := node.Connect(ctx, peer.ID)
	if err != nil {
		fmt.Printf("❌ 连接到 %s 失败: %v\n", remoteID, err)
		return
	}

	// 等待 Realm 认证完成（给 RealmAuth 握手一些时间）
	// 这是必需的，因为 Realm 认证是异步的，应用协议需要先完成认证
	if !waitForRealmAuth(conn, 5*time.Second) {
		fmt.Printf("⚠️  连接到 %s 的 Realm 认证超时\n", remoteID)
		return
	}

	// 打开聊天流（此时 Realm 认证已完成）
	stream, err := conn.OpenStream(ctx, chatProtocol)
	if err != nil {
		fmt.Printf("❌ 打开聊天流到 %s 失败: %v\n", remoteID, err)
		return
	}

	fmt.Printf("✅ 已连接到 %s\n", remoteID)

	// 保存流
	peersLock.Lock()
	peers[remoteID] = stream
	peersLock.Unlock()

	// 在后台处理入站消息
	go handleChatStreamRead(stream, remoteID)
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

// handleChatStreamRead 读取聊天流消息
func handleChatStreamRead(stream dep2p.Stream, remoteID string) {
	defer func() {
		peersLock.Lock()
		delete(peers, remoteID)
		peersLock.Unlock()
		_ = stream.Close()
		fmt.Printf("📤 [%s] 离开了聊天\n", remoteID)
	}()

	reader := bufio.NewReader(stream)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err != io.EOF {
				fmt.Printf("❌ 读取消息失败: %v\n", err)
			}
			return
		}

		line = strings.TrimSpace(line)
		if line != "" {
			fmt.Printf("\033[32m[%s]\033[0m %s\n", remoteID, line)
		}
	}
}

// handleChatStream 处理入站聊天流
func handleChatStream(stream dep2p.Stream) {
	// 获取远程节点ID
	conn := stream.Connection()
	if conn == nil {
		_ = stream.Close()
		return
	}
	remoteID := conn.RemoteID().String()[:8]

	fmt.Printf("📥 [%s] 连接到聊天\n", remoteID)

	// 保存流
	peersLock.Lock()
	peers[remoteID] = stream
	peersLock.Unlock()

	defer func() {
		peersLock.Lock()
		delete(peers, remoteID)
		peersLock.Unlock()
		_ = stream.Close()
		fmt.Printf("📤 [%s] 离开了聊天\n", remoteID)
	}()

	// 读取消息
	reader := bufio.NewReader(stream)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err != io.EOF {
				fmt.Printf("❌ 读取消息失败: %v\n", err)
			}
			return
		}

		line = strings.TrimSpace(line)
		if line != "" {
			// 使用不同颜色显示
			fmt.Printf("\033[32m[%s]\033[0m %s\n", remoteID, line)
		}
	}
}

// handleInput 处理用户输入
func handleInput(ctx context.Context, node *dep2p.Node, nick string) {
	reader := bufio.NewReader(os.Stdin)

	for {
		select {
		case <-ctx.Done():
			return
		default:
			fmt.Printf("\033[34m[%s]\033[0m ", nick)
			line, err := reader.ReadString('\n')
			if err != nil {
				return
			}

			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}

			// 处理命令
			if strings.HasPrefix(line, "/") {
				handleCommand(ctx, node, line)
				continue
			}

			// 广播消息
			broadcastMessage(nick, line)
		}
	}
}

// handleCommand 处理命令
func handleCommand(_ context.Context, node *dep2p.Node, cmd string) {
	parts := strings.Fields(cmd)
	if len(parts) == 0 {
		return
	}

	switch parts[0] {
	case "/quit", "/exit", "/q":
		fmt.Println("正在退出...")
		os.Exit(0)

	case "/peers", "/list":
		fmt.Println("📋 当前连接的节点:")
		peersLock.RLock()
		if len(peers) == 0 {
			fmt.Println("   (暂无连接)")
		} else {
			for id := range peers {
				fmt.Printf("   • %s\n", id)
			}
		}
		peersLock.RUnlock()

	case "/info":
		fmt.Printf("📍 节点 ID: %s\n", node.ID())
		fmt.Println("📡 监听地址:")
		for _, addr := range node.ListenAddrs() {
			fmt.Printf("   • %s\n", addr)
		}

	case "/connect":
		if len(parts) < 3 {
			fmt.Println("用法: /connect <节点ID> <地址>")
			return
		}
		fmt.Printf("尝试连接到 %s @ %s...\n", parts[1], parts[2])
		// P3: 可选功能 - mDNS 自动发现已满足基本需求
		// 手动连接属于高级用法，对应 DialByNodeIDWithDialAddrs：
		//   node.ConnectWithAddrs(ctx, nodeID, []string{addr})

	case "/help", "/?":
		printHelp()

	default:
		fmt.Printf("未知命令: %s\n", parts[0])
		fmt.Println("输入 /help 查看帮助")
	}
}

// broadcastMessage 广播消息给所有对等方
func broadcastMessage(nick, message string) {
	fullMessage := fmt.Sprintf("%s: %s\n", nick, message)

	peersLock.RLock()
	defer peersLock.RUnlock()

	if len(peers) == 0 {
		fmt.Println("⚠️  没有连接的节点，消息未发送")
		return
	}

	for id, stream := range peers {
		_, err := stream.Write([]byte(fullMessage))
		if err != nil {
			fmt.Printf("❌ 发送到 %s 失败: %v\n", id, err)
		}
	}
}

// printHelp 打印帮助信息
func printHelp() {
	fmt.Println()
	fmt.Println("可用命令:")
	fmt.Println("  /peers, /list   - 列出连接的节点")
	fmt.Println("  /info           - 显示本节点信息")
	fmt.Println("  /connect ID ADDR - 手动连接节点")
	fmt.Println("  /quit, /exit    - 退出程序")
	fmt.Println("  /help           - 显示帮助")
	fmt.Println()
}
