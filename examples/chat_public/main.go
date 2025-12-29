// Package main 提供公网 Chat 示例
//
// chat_public v3: PubSub 群聊 + Stream 私聊 + Relay 透明回退
//
// 核心特性：
//   - 群聊：使用 GossipSub 协议自动广播（node.Publish/Subscribe）
//   - 私聊：使用点对点 Stream（/msg <nick> <message>）
//   - 成员发现：混合模式（Seed Bootstrap + PubSub 自动发现）
//   - NAT 穿透：Relay Transport 透明回退
//
// 运行方式（三节点）：
//
//	# 1. VPS 上运行 Seed（公网可达 + Relay Server）
//	go run main.go -mode seed -port 4001
//
//	# 2. 本地运行 Alice
//	go run main.go -mode peer -seed <seedFullAddr> -name alice
//
//	# 3. 另一台机器运行 Bob
//	go run main.go -mode peer -seed <seedFullAddr> -name bob
//
// 架构：
//
//	┌─────────────────────────────────────────────────────────────────┐
//	│                     chat_public v3 架构                          │
//	├─────────────────────────────────────────────────────────────────┤
//	│                                                                  │
//	│  群聊：GossipSub                    私聊：Stream                 │
//	│  ┌────────────────────────┐        ┌────────────────────────┐   │
//	│  │ node.Publish(topic,msg)│        │ conn.OpenStream(proto) │   │
//	│  │ sub.Messages()         │        │ stream.Write(msg)      │   │
//	│  └────────────────────────┘        └────────────────────────┘   │
//	│           │                                 │                    │
//	│           ▼                                 ▼                    │
//	│  ┌─────────────────────────────────────────────────────────┐    │
//	│  │              Relay Transport（透明回退）                 │    │
//	│  │    直连失败 → 自动通过 Seed Relay 传输                   │    │
//	│  └─────────────────────────────────────────────────────────┘    │
//	│                                                                  │
//	└─────────────────────────────────────────────────────────────────┘
package main

import (
	"bufio"
	"context"
	"encoding/json"
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
	"github.com/dep2p/go-dep2p/pkg/interfaces/messaging"
	"github.com/dep2p/go-dep2p/pkg/interfaces/reachability"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              协议和常量
// ============================================================================

// 协议标识符
var (
	// privateProtocol 私聊协议
	privateProtocol = dep2p.ProtocolID("/dep2p/chat/private/1.0.0")
)

// chatTopicPrefix 群聊 topic 前缀
const chatTopicPrefix = "chat-room:"

// ChatMessage 聊天消息结构
type ChatMessage struct {
	Type    string `json:"type"`              // "broadcast" | "private" | "join" | "leave"
	From    string `json:"from"`              // 发送者昵称
	To      string `json:"to,omitempty"`      // 私聊目标昵称（私聊时使用）
	NodeID  string `json:"nodeID"`            // 发送者 NodeID
	Message string `json:"message,omitempty"` // 消息内容
}

// ============================================================================
//                              全局状态
// ============================================================================

var (
	// 当前节点信息
	currentNode *dep2p.Node
	currentNick string
	chatTopic   string // 当前聊天室 topic
	isSeedMode  bool   // 是否是 seed 模式（用于诊断输出）

	// 群聊订阅
	groupSub     messaging.Subscription
	groupSubLock sync.RWMutex

	// 私聊流管理
	privateStreams     = make(map[string]dep2p.Stream) // shortID -> stream
	privateStreamsLock sync.RWMutex

	// 昵称映射
	nickToNodeID    = make(map[string]string) // nick -> fullNodeID
	nodeIDToNick    = make(map[string]string) // shortID -> nick
	nickMappingLock sync.RWMutex

	// whois 查询等待机制
	whoisWaiters   = make(map[string]chan string) // requestID -> channel (等待 NodeID 响应)
	whoisWaitersMu sync.RWMutex
)

// ============================================================================
//                              主函数
// ============================================================================

func main() {
	// 解析命令行参数
	mode := flag.String("mode", "", "运行模式: seed（公网种子）或 peer（客户端）")
	port := flag.Int("port", 0, "监听端口（seed 模式建议固定端口）")
	seedAddr := flag.String("seed", "", "Seed 的 Full Address（peer 模式必填）")
	name := flag.String("name", "", "昵称（默认使用节点 ID 前 8 位）")
	realmArg := flag.String("realm", "public-chat", "Realm ID（聊天室）")
	logFile := flag.String("log-file", "", "日志文件路径")
	flag.Parse()

	// 验证参数
	if *mode == "" {
		fmt.Println("❌ 请指定运行模式: -mode seed 或 -mode peer")
		fmt.Println()
		printUsage()
		os.Exit(1)
	}

	if *mode == "peer" && *seedAddr == "" {
		fmt.Println("❌ peer 模式必须指定 -seed <fulladdr>")
		fmt.Println()
		printUsage()
		os.Exit(1)
	}

	if *mode != "seed" && *mode != "peer" {
		fmt.Printf("❌ 未知模式: %s\n", *mode)
		fmt.Println()
		printUsage()
		os.Exit(1)
	}

	fmt.Println("╔══════════════════════════════════════════════════════╗")
	fmt.Println("║   DeP2P Chat Public v3                               ║")
	fmt.Println("║   PubSub 群聊 + Stream 私聊 + Relay 透明回退          ║")
	fmt.Println("╚══════════════════════════════════════════════════════╝")
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

	// 配置日志
	logFilePath := *logFile
	if logFilePath == "" {
		timestamp := time.Now().Format("20060102-150405")
		pid := os.Getpid()
		logFilePath = fmt.Sprintf("chat-public-%s-%s-%d.log", *mode, timestamp, pid)
		logsDir := "logs"
		if err := os.MkdirAll(logsDir, 0750); err == nil {
			logFilePath = filepath.Join(logsDir, logFilePath)
		}
	}

	fmt.Printf("📝 日志文件: %s\n", logFilePath)
	fmt.Println("   控制台仅显示交互信息")
	fmt.Println()

	// 设置聊天室 topic
	chatTopic = chatTopicPrefix + *realmArg

	// 创建节点配置
	opts := []dep2p.Option{
		dep2p.WithPreset(dep2p.PresetDesktop),
		dep2p.WithRelay(true),          // 启用 Relay Client
		dep2p.WithLogFile(logFilePath), // DeP2P 内部日志输出到文件
	}

	// 同时配置标准库 log 包输出到同一文件（用于应用层日志）
	logFileHandle, err := os.OpenFile(logFilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		fmt.Printf("⚠️  无法打开日志文件: %v\n", err)
	} else {
		log.SetOutput(logFileHandle)
		log.SetFlags(log.LstdFlags)
		defer func() { _ = logFileHandle.Close() }()
	}

	if *port != 0 {
		opts = append(opts, dep2p.WithListenPort(*port))
	}

	// 关键：Peer 模式下，为 RelayTransport 追加一个 /p2p-circuit 监听地址。
	// 否则虽然 Relay Server 能对目标 peer 发送 STOP 并建立电路，
	// 但本地 Endpoint 没有 acceptLoop 去 Accept() 这些入站 relay conn，
	// 会导致电路长期悬挂占用 relay reservation 的 Slots，最终触发 error code 200（槽位已满）。
	if *mode == "peer" && *seedAddr != "" {
		if !strings.Contains(*seedAddr, "/p2p-circuit") {
			opts = append(opts, dep2p.WithExtraListenAddrs(*seedAddr+"/p2p-circuit"))
		}
	}

	if *mode == "seed" {
		opts = append(opts, dep2p.WithPreset(dep2p.PresetServer))
		opts = append(opts, dep2p.WithRelayServer(true)) // Seed 启用 Relay Server
		isSeedMode = true
	}

	// 创建节点
	node, err := dep2p.NewNode(opts...)
	if err != nil {
		fmt.Printf("❌ 创建节点失败: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = node.Close() }()

	// 启动监听（必须！否则 ListenAddrs/ShareableAddrs 为空）
	if err := node.Endpoint().Listen(ctx); err != nil {
		fmt.Printf("❌ 启动监听失败: %v\n", err)
		os.Exit(1)
	}

	currentNode = node

	// IMPL-1227: 加入 Realm（使用新 API）
	// 使用 DeriveRealmKeyFromName 从 realm 名称派生密钥，确保同名聊天室的节点能互相认证
	realmKey := types.DeriveRealmKeyFromName(*realmArg)
	realm, err := node.JoinRealmWithKey(ctx, *realmArg, realmKey)
	if err != nil {
		fmt.Printf("⚠️  加入 Realm 失败: %v\n", err)
	} else {
		fmt.Printf("🏠 已加入聊天室: %s (ID: %s)\n", realm.Name(), realm.ID())
	}
	_ = realm // 可用于后续服务访问

	fmt.Println("✅ 节点已启动")
	fmt.Printf("📍 节点 ID: %s\n", node.ID())

	// 设置昵称
	nick := *name
	if nick == "" {
		nick = node.ID().String()[:8]
	}
	currentNick = nick
	registerNick(nick, node.ID().String())

	fmt.Printf("👤 昵称: %s\n", nick)
	fmt.Printf("🎭 模式: %s\n", *mode)
	fmt.Printf("💬 群聊 Topic: %s\n", chatTopic)
	fmt.Println()

	// 打印监听地址
	fmt.Println("📡 监听地址:")
	for _, addr := range node.ListenAddrs() {
		fmt.Printf("   • %s\n", addr)
	}
	fmt.Println()

	// 根据模式运行
	switch *mode {
	case "seed":
		runSeed(ctx, node, nick)
	case "peer":
		runPeer(ctx, node, *seedAddr, nick)
	}

	// 等待上下文取消
	<-ctx.Done()
}

// ============================================================================
//                              Seed 模式
// ============================================================================

// runSeed 运行 Seed 节点
func runSeed(ctx context.Context, node *dep2p.Node, nick string) {
	fmt.Println("════════════════════════════════════════════════════════")
	fmt.Println("        🌐 SEED 模式（Bootstrap + Relay Server）")
	fmt.Println("════════════════════════════════════════════════════════")
	fmt.Println()
	fmt.Println("v3 架构说明：")
	fmt.Println("  • Seed 作为 Bootstrap 节点和 Relay Server")
	fmt.Println("  • 群聊使用 GossipSub 协议（自动广播）")
	fmt.Println("  • 私聊使用点对点 Stream")
	fmt.Println("  • NAT 节点间通过 Relay 透明通信")
	fmt.Println()

	// 立即打印 BootstrapCandidates（旁路候选，立即可用）
	candidates := node.BootstrapCandidates()
	if len(candidates) > 0 {
		fmt.Println("📋 候选地址（BootstrapCandidates，立即可用于 peer 冷启动）：")
		for _, c := range candidates {
			fmt.Printf("   %s\n", c.FullAddr)
		}
		fmt.Println()
	} else {
		// 如果立即获取不到，等待几秒再试
		waitCandidates := waitBootstrapCandidates(ctx, node, 3*time.Second)
		if len(waitCandidates) > 0 {
			fmt.Println("📋 候选地址（BootstrapCandidates，立即可用于 peer 冷启动）：")
			for _, c := range waitCandidates {
				fmt.Printf("   %s\n", c.FullAddr)
			}
			fmt.Println()
		} else {
			fmt.Println("⚠️  暂无可用候选地址（请检查网络配置）")
			fmt.Println()
		}
	}

	// 异步等待 ShareableAddrs（严格验证的公网直连地址）
	go func() {
		waitCtx, waitCancel := context.WithTimeout(ctx, 30*time.Second)
		defer waitCancel()

		addrs, err := node.WaitShareableAddrs(waitCtx)
		if err == nil && len(addrs) > 0 {
			fmt.Println()
			fmt.Println("✅ 已验证的可分享地址（ShareableAddrs，VerifiedDirect）：")
			for _, addr := range addrs {
				fmt.Printf("   %s\n", addr)
			}
			fmt.Println()
		}
	}()

	// 注册私聊协议处理器
	node.Endpoint().SetProtocolHandler(privateProtocol, handlePrivateStream)

	// 订阅群聊（Seed 也参与群聊）
	if err := subscribeGroupChat(ctx, node); err != nil {
		fmt.Printf("⚠️  订阅群聊失败: %v\n", err)
	} else {
		fmt.Println("✅ 已订阅群聊 Topic")
	}

	// 广播加入消息
	announceJoin(ctx, node, nick)

	fmt.Println()
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("Seed 已就绪，等待 Peer 连接...")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println()
	fmt.Println("输入消息并按回车发送群聊，输入 /help 查看命令")
	fmt.Println()

	// 处理用户输入
	go handleInput(ctx, node, nick)
}

// ============================================================================
//                              Peer 模式
// ============================================================================

// runPeer 运行 Peer 节点
func runPeer(ctx context.Context, node *dep2p.Node, seedFullAddr, nick string) {
	fmt.Println("════════════════════════════════════════════════════════")
	fmt.Println("        📱 PEER 模式（PubSub 群聊 + 私聊）")
	fmt.Println("════════════════════════════════════════════════════════")
	fmt.Println()
	fmt.Printf("🔗 正在连接到 Seed: %s\n", seedFullAddr)
	fmt.Println()

	// 连接到 Seed（Bootstrap）
	conn, err := node.ConnectToAddr(ctx, seedFullAddr)
	if err != nil {
		fmt.Printf("❌ 连接 Seed 失败: %v\n", err)
		os.Exit(1)
	}

	seedNodeID := conn.RemoteID()
	seedShort := seedNodeID.String()[:8]
	fmt.Printf("✅ 已连接到 Seed: %s\n", seedShort)

	// 等待 Realm 认证
	if !waitForRealmAuth(conn, 5*time.Second) {
		fmt.Println("⚠️  Realm 认证超时")
	}
	fmt.Println()

	// 注册私聊协议处理器
	node.Endpoint().SetProtocolHandler(privateProtocol, handlePrivateStream)

	// 订阅群聊
	if err := subscribeGroupChat(ctx, node); err != nil {
		fmt.Printf("❌ 订阅群聊失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("✅ 已订阅群聊 Topic")

	// 等待 GossipSub mesh 建立
	fmt.Println("⏳ 等待 GossipSub mesh 建立...")
	time.Sleep(2 * time.Second)

	// 广播加入消息
	announceJoin(ctx, node, nick)

	fmt.Println()
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("聊天已就绪！")
	fmt.Println("  • 直接输入消息 → 群聊（所有人可见）")
	fmt.Println("  • /msg <昵称> <消息> → 私聊（仅对方可见）")
	fmt.Println("  • /peers → 查看在线成员")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println()
	fmt.Println("输入 /help 查看所有命令")
	fmt.Println()

	// 处理用户输入
	go handleInput(ctx, node, nick)
}

// ============================================================================
//                              群聊功能（PubSub）
// ============================================================================

// subscribeGroupChat 订阅群聊 topic
func subscribeGroupChat(ctx context.Context, node *dep2p.Node) error {
	sub, err := node.Subscribe(ctx, chatTopic)
	if err != nil {
		return err
	}

	groupSubLock.Lock()
	groupSub = sub
	groupSubLock.Unlock()

	// 启动消息接收循环
	go receiveGroupMessages(ctx, sub)

	return nil
}

// receiveGroupMessages 接收群聊消息
func receiveGroupMessages(ctx context.Context, sub messaging.Subscription) {
	msgChan := sub.Messages()

	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-msgChan:
			if !ok {
				return
			}

			// 跳过自己的消息
			if msg.From.String() == currentNode.ID().String() {
				continue
			}

			// 解析消息
			var cm ChatMessage
			if err := json.Unmarshal(msg.Data, &cm); err != nil {
				log.Printf("解析群聊消息失败: %v", err)
				continue
			}

			// seed 模式诊断输出：便于核对云端是否收到消息
			if isSeedMode {
				shortNodeID := cm.NodeID
				if len(shortNodeID) > 8 {
					shortNodeID = shortNodeID[:8]
				}
				log.Printf("[诊断] 收到群聊消息: type=%s, from=%s, nodeID=%s, msgLen=%d",
					cm.Type, cm.From, shortNodeID, len(cm.Message))
			}

			// 注册昵称映射
			if cm.NodeID != "" && cm.From != "" {
				registerNick(cm.From, cm.NodeID)
			}

			// 根据消息类型显示
			switch cm.Type {
			case "broadcast":
				fmt.Printf("\033[32m[群聊] %s: %s\033[0m\n", cm.From, cm.Message)
			case "join":
				fmt.Printf("\033[33m📥 %s 加入了聊天室\033[0m\n", cm.From)
				// 回复 welcome 消息，让新加入者知道我们的存在
				// 这解决了后来者无法知道先到者昵称的问题
				go func() {
					welcomeMsg := ChatMessage{
						Type:   "welcome",
						From:   currentNick,
						NodeID: currentNode.ID().String(),
					}
					data, _ := json.Marshal(welcomeMsg)
					_ = currentNode.Publish(ctx, chatTopic, data)
				}()
			case "welcome":
				// 静默处理 welcome 消息，只用于注册昵称（已在上面完成）
			case "leave":
				fmt.Printf("\033[33m📤 %s 离开了聊天室\033[0m\n", cm.From)
			case "whois_req":
				// 处理 whois 查询请求：如果查询的是自己，回复 NodeID
				if cm.To == currentNick {
					go func() {
						respMsg := ChatMessage{
							Type:    "whois_resp",
							From:    currentNick,
							To:      cm.From, // 回复给请求者
							NodeID:  currentNode.ID().String(),
							Message: cm.Message, // 携带原 requestID
						}
						data, _ := json.Marshal(respMsg)
						_ = currentNode.Publish(ctx, chatTopic, data)
					}()
				}
			case "whois_resp":
				// 处理 whois 响应：如果是发给自己的，唤醒等待者
				if cm.To == currentNick {
					whoisWaitersMu.RLock()
					if ch, ok := whoisWaiters[cm.Message]; ok {
						select {
						case ch <- cm.NodeID:
						default:
						}
					}
					whoisWaitersMu.RUnlock()
				}
			}
		}
	}
}

// broadcastMessage 广播群聊消息
func broadcastMessage(ctx context.Context, nick, message string) error {
	cm := ChatMessage{
		Type:    "broadcast",
		From:    nick,
		NodeID:  currentNode.ID().String(),
		Message: message,
	}

	data, err := json.Marshal(cm)
	if err != nil {
		return err
	}

	return currentNode.Publish(ctx, chatTopic, data)
}

// announceJoin 广播加入消息
func announceJoin(ctx context.Context, node *dep2p.Node, nick string) {
	cm := ChatMessage{
		Type:   "join",
		From:   nick,
		NodeID: node.ID().String(),
	}

	data, _ := json.Marshal(cm)
	_ = node.Publish(ctx, chatTopic, data)
}

// announceLeave 广播离开消息
func announceLeave(ctx context.Context, node *dep2p.Node, nick string) {
	cm := ChatMessage{
		Type:   "leave",
		From:   nick,
		NodeID: node.ID().String(),
	}

	data, _ := json.Marshal(cm)
	_ = node.Publish(ctx, chatTopic, data)
}

// ============================================================================
//                              私聊功能（Stream）
// ============================================================================

// sendPrivateMessage 发送私聊消息
func sendPrivateMessage(ctx context.Context, targetNick, message string) error {
	// 使用独立的 timeout context，避免主 ctx 取消导致 context canceled
	sendCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 查找目标 NodeID
	nickMappingLock.RLock()
	targetNodeIDStr, ok := nickToNodeID[targetNick]
	nickMappingLock.RUnlock()

	// 如果找不到映射，自动发起 whois 查询
	if !ok {
		fmt.Printf("🔍 正在查询用户 '%s'...\n", targetNick)
		resolvedNodeID, err := lookupNickname(sendCtx, targetNick)
		if err != nil {
			return err
		}
		targetNodeIDStr = resolvedNodeID
		// 注册到映射（已在 lookupNickname 中完成）
	}

	targetNodeID, err := types.ParseNodeID(targetNodeIDStr)
	if err != nil {
		return fmt.Errorf("无效的 NodeID: %v", err)
	}

	shortID := targetNodeIDStr[:8]

	// 检查是否已有私聊流
	privateStreamsLock.RLock()
	stream, exists := privateStreams[shortID]
	privateStreamsLock.RUnlock()

	if !exists || stream == nil {
		// 建立新连接和流
		conn, err := currentNode.Connect(sendCtx, targetNodeID)
		if err != nil {
			return fmt.Errorf("连接失败: %v", err)
		}

		stream, err = conn.OpenStream(sendCtx, privateProtocol)
		if err != nil {
			return fmt.Errorf("打开私聊流失败: %v", err)
		}

		privateStreamsLock.Lock()
		privateStreams[shortID] = stream
		privateStreamsLock.Unlock()

		// 启动读取 goroutine
		go readPrivateStream(stream, shortID)
	}

	// 发送消息
	cm := ChatMessage{
		Type:    "private",
		From:    currentNick,
		To:      targetNick,
		NodeID:  currentNode.ID().String(),
		Message: message,
	}

	data, _ := json.Marshal(cm)
	_, err = stream.Write(append(data, '\n'))
	if err != nil {
		// 流已关闭，移除并返回错误
		privateStreamsLock.Lock()
		delete(privateStreams, shortID)
		privateStreamsLock.Unlock()
		return fmt.Errorf("发送失败: %v", err)
	}

	fmt.Printf("\033[35m[私聊 → %s] %s\033[0m\n", targetNick, message)
	return nil
}

// handlePrivateStream 处理入站私聊流
func handlePrivateStream(stream dep2p.Stream) {
	conn := stream.Connection()
	if conn == nil {
		_ = stream.Close()
		return
	}

	remoteID := conn.RemoteID().String()
	shortID := remoteID[:8]

	// 检查是否已有该 peer 的流
	privateStreamsLock.Lock()
	if existing, ok := privateStreams[shortID]; ok && existing != nil {
		privateStreamsLock.Unlock()
		// 已有流，关闭新的入站流
		_ = stream.Close()
		return
	}
	privateStreams[shortID] = stream
	privateStreamsLock.Unlock()

	// 读取消息
	readPrivateStream(stream, shortID)
}

// readPrivateStream 读取私聊流
func readPrivateStream(stream dep2p.Stream, shortID string) {
	defer func() {
		privateStreamsLock.Lock()
		delete(privateStreams, shortID)
		privateStreamsLock.Unlock()
		_ = stream.Close()
	}()

	reader := bufio.NewReader(stream)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err != io.EOF {
				log.Printf("读取私聊流失败: %v", err)
			}
			return
		}

		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		var cm ChatMessage
		if err := json.Unmarshal([]byte(line), &cm); err != nil {
			log.Printf("解析私聊消息失败: %v", err)
			continue
		}

		// 注册昵称映射
		if cm.NodeID != "" && cm.From != "" {
			registerNick(cm.From, cm.NodeID)
		}

		fmt.Printf("\033[35m[私聊 ← %s] %s\033[0m\n", cm.From, cm.Message)
	}
}

// ============================================================================
//                              昵称管理
// ============================================================================

// registerNick 注册昵称映射
func registerNick(nick, nodeID string) {
	if nick == "" || nodeID == "" {
		return
	}

	nickMappingLock.Lock()
	defer nickMappingLock.Unlock()

	nickToNodeID[nick] = nodeID
	if len(nodeID) >= 8 {
		nodeIDToNick[nodeID[:8]] = nick
	}
}

// getNickByShortID 通过 shortID 获取昵称
func getNickByShortID(shortID string) string {
	nickMappingLock.RLock()
	defer nickMappingLock.RUnlock()

	if nick, ok := nodeIDToNick[shortID]; ok {
		return nick
	}
	return shortID
}

// lookupNickname 通过 whois 机制查询昵称对应的 NodeID
//
// 发送 whois_req 并等待 whois_resp，超时返回错误。
// 成功后自动注册昵称映射。
func lookupNickname(ctx context.Context, targetNick string) (string, error) {
	// 生成唯一的 requestID
	requestID := fmt.Sprintf("%d-%s", time.Now().UnixNano(), currentNick)

	// 创建响应等待通道
	respChan := make(chan string, 1)
	whoisWaitersMu.Lock()
	whoisWaiters[requestID] = respChan
	whoisWaitersMu.Unlock()

	// 清理函数
	defer func() {
		whoisWaitersMu.Lock()
		delete(whoisWaiters, requestID)
		whoisWaitersMu.Unlock()
	}()

	// 发送 whois_req
	reqMsg := ChatMessage{
		Type:    "whois_req",
		From:    currentNick,
		To:      targetNick,
		NodeID:  currentNode.ID().String(),
		Message: requestID,
	}
	data, _ := json.Marshal(reqMsg)
	if err := currentNode.Publish(ctx, chatTopic, data); err != nil {
		return "", fmt.Errorf("发送 whois 请求失败: %v", err)
	}

	// 等待响应（带超时）
	lookupCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	select {
	case nodeID := <-respChan:
		// 注册映射
		registerNick(targetNick, nodeID)
		fmt.Printf("✅ 已找到用户 '%s'\n", targetNick)
		return nodeID, nil
	case <-lookupCtx.Done():
		return "", fmt.Errorf("lookup 超时：用户 '%s' 未在线或未响应", targetNick)
	}
}

// ============================================================================
//                              用户输入处理
// ============================================================================

// handleInput 处理用户输入
func handleInput(ctx context.Context, node *dep2p.Node, nick string) {
	reader := bufio.NewReader(os.Stdin)

	for {
		select {
		case <-ctx.Done():
			// 发送离开消息
			announceLeave(context.Background(), node, nick)
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

			if strings.HasPrefix(line, "/") {
				handleCommand(ctx, node, line)
				continue
			}

			// 发送群聊消息
			if err := broadcastMessage(ctx, nick, line); err != nil {
				fmt.Printf("❌ 发送失败: %v\n", err)
			} else {
				// 本地回显（与接收端格式一致），避免"自己发的看不见"
				fmt.Printf("\033[32m[群聊] %s: %s\033[0m\n", nick, line)
			}
		}
	}
}

// handleCommand 处理命令
func handleCommand(ctx context.Context, node *dep2p.Node, cmd string) {
	parts := strings.Fields(cmd)
	if len(parts) == 0 {
		return
	}

	switch parts[0] {
	case "/quit", "/exit", "/q":
		fmt.Println("正在退出...")
		announceLeave(context.Background(), node, currentNick)
		os.Exit(0)

	case "/msg", "/pm", "/whisper":
		if len(parts) < 3 {
			fmt.Println("用法: /msg <昵称> <消息>")
			fmt.Println("示例: /msg bob 你好，这是私聊消息")
			return
		}
		targetNick := parts[1]
		message := strings.Join(parts[2:], " ")

		if err := sendPrivateMessage(ctx, targetNick, message); err != nil {
			fmt.Printf("❌ 私聊失败: %v\n", err)
		}

	case "/peers", "/list", "/who":
		listPeers()

	case "/info":
		fmt.Printf("📍 节点 ID: %s\n", node.ID())
		fmt.Printf("👤 昵称: %s\n", currentNick)
		fmt.Printf("💬 群聊 Topic: %s\n", chatTopic)
		fmt.Println("📡 监听地址:")
		for _, addr := range node.ListenAddrs() {
			fmt.Printf("   • %s\n", addr)
		}
		fmt.Println("📢 通告地址:")
		for _, addr := range node.AdvertisedAddrs() {
			fmt.Printf("   • %s\n", addr)
		}

	case "/help", "/?":
		printHelp()

	default:
		fmt.Printf("未知命令: %s\n", parts[0])
		fmt.Println("输入 /help 查看帮助")
	}
}

// listPeers 列出在线成员
func listPeers() {
	fmt.Println("📋 在线成员:")

	// 从昵称映射获取
	nickMappingLock.RLock()
	defer nickMappingLock.RUnlock()

	if len(nickToNodeID) == 0 {
		fmt.Println("   (暂无已知成员)")
		fmt.Println("   提示: 等待其他成员发送消息后将自动发现")
		return
	}

	for nick, nodeID := range nickToNodeID {
		shortID := nodeID[:8]
		if nick == currentNick {
			fmt.Printf("   • %s (%s) [自己]\n", nick, shortID)
		} else {
			fmt.Printf("   • %s (%s)\n", nick, shortID)
		}
	}
}

// ============================================================================
//                              辅助函数
// ============================================================================

// waitBootstrapCandidates 等待候选地址
func waitBootstrapCandidates(ctx context.Context, node *dep2p.Node, timeout time.Duration) []reachability.BootstrapCandidate {
	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-waitCtx.Done():
			return node.BootstrapCandidates()
		case <-ticker.C:
			candidates := node.BootstrapCandidates()
			if len(candidates) > 0 {
				return candidates
			}
		}
	}
}

// waitForRealmAuth 等待 Realm 认证完成
func waitForRealmAuth(conn dep2p.Connection, timeout time.Duration) bool {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return false
		case <-ticker.C:
			rc := conn.RealmContext()
			if rc != nil && rc.Verified {
				return true
			}
		}
	}
}

// printHelp 打印帮助信息
func printHelp() {
	fmt.Println()
	fmt.Println("═══════════════════════════════════════════════════════════════")
	fmt.Println("                        📖 命令帮助")
	fmt.Println("═══════════════════════════════════════════════════════════════")
	fmt.Println()
	fmt.Println("消息命令:")
	fmt.Println("  <直接输入>            - 发送群聊消息（所有人可见）")
	fmt.Println("  /msg <昵称> <消息>    - 发送私聊消息（仅对方可见）")
	fmt.Println()
	fmt.Println("查看命令:")
	fmt.Println("  /peers, /list, /who   - 列出在线成员")
	fmt.Println("  /info                 - 显示本节点信息")
	fmt.Println()
	fmt.Println("其他命令:")
	fmt.Println("  /quit, /exit          - 退出程序")
	fmt.Println("  /help                 - 显示帮助")
	fmt.Println()
	fmt.Println("═══════════════════════════════════════════════════════════════")
	fmt.Println("                      v3 架构说明")
	fmt.Println("═══════════════════════════════════════════════════════════════")
	fmt.Println()
	fmt.Println("群聊机制（GossipSub）：")
	fmt.Println("  • 使用 PubSub 协议自动广播消息")
	fmt.Println("  • 消息通过 GossipSub mesh 网络传播")
	fmt.Println("  • 支持 Relay Transport 透明回退")
	fmt.Println()
	fmt.Println("私聊机制（Stream）：")
	fmt.Println("  • 使用点对点 Stream 直接通信")
	fmt.Println("  • 自动通过 Relay 建立连接（若直连失败）")
	fmt.Println("  • 消息仅对方可见")
	fmt.Println()
	fmt.Println("成员发现（混合模式）：")
	fmt.Println("  • Seed 作为 Bootstrap 节点")
	fmt.Println("  • 成员加入时广播 join 消息")
	fmt.Println("  • 昵称自动学习（通过消息发现）")
	fmt.Println()
}

// printUsage 打印使用说明
func printUsage() {
	fmt.Println("用法:")
	fmt.Println()
	fmt.Println("  # Seed 模式（公网可达节点 + Relay Server）")
	fmt.Println("  go run main.go -mode seed -port 4001")
	fmt.Println()
	fmt.Println("  # Peer 模式（客户端）")
	fmt.Println("  go run main.go -mode peer -seed <fulladdr> -name <昵称>")
	fmt.Println()
	fmt.Println("参数:")
	fmt.Println("  -mode      运行模式: seed 或 peer")
	fmt.Println("  -port      监听端口（seed 建议固定端口，peer 可用 0）")
	fmt.Println("  -seed      Seed 的 Full Address（peer 模式必填）")
	fmt.Println("  -name      昵称")
	fmt.Println("  -realm     聊天室名称（默认 public-chat）")
	fmt.Println("  -log-file  日志文件路径")
	fmt.Println()
	fmt.Println("示例:")
	fmt.Println()
	fmt.Println("  # 启动 Seed（在公网服务器上）")
	fmt.Println("  go run main.go -mode seed -port 4001")
	fmt.Println()
	fmt.Println("  # 启动 Peer（复制 Seed 输出的地址）")
	fmt.Println("  go run main.go -mode peer \\")
	fmt.Println("    -seed '/ip4/1.2.3.4/udp/4001/quic-v1/p2p/5Q2STWvB...' \\")
	fmt.Println("    -name alice")
	fmt.Println()
}
