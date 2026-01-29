// Package main 提供统一的 P2P 聊天示例
//
// 这是一个交互式聊天 Demo，演示 DeP2P 的核心功能：
//   - mDNS 自动发现：同一局域网内的节点自动发现并连接
//   - Bootstrap 发现：通过引导节点发现其他节点
//   - Relay：NAT 后节点通过中继通信（统一 Relay v2.0）
//   - PubSub 群聊：基于 GossipSub 的发布订阅消息
//   - Streams 私聊：基于双向流的点对点消息
//   - 已知节点直连：云服务器场景下直接连接已知节点
//   - STUN 信任模式：云服务器场景下信任 STUN 发现的地址
//
// 使用方法：
//
//	# 场景 1：零配置启动（局域网 mDNS 自动发现）
//	go run ./examples/chat
//
//	# 场景 2：指定引导节点（跨网络发现）
//	go run ./examples/chat --bootstrap "/ip4/1.2.3.4/udp/4001/quic-v1/p2p/12D3KooW..."
//
//	# 场景 3：指定引导节点 + 中继（NAT 穿透）
//	go run ./examples/chat \
//	    --bootstrap "/ip4/1.2.3.4/udp/4001/quic-v1/p2p/12D3KooW..." \
//	    --relay "/ip4/1.2.3.4/udp/4001/quic-v1/p2p/12D3KooW..."
//
//	# 场景 4：服务模式（云服务器部署，提供基础设施）
//	go run ./examples/chat \
//	    --serve \
//	    --port 4001 \
//	    --public-addr "/ip4/YOUR_PUBLIC_IP/udp/4001/quic-v1"
//
//	# 场景 5：云服务器直连模式（不依赖引导节点）
//	go run ./examples/chat \
//	    --trust-stun \
//	    --known-peers "12D3KooW...@/ip4/1.2.3.4/udp/4001/quic-v1"
//
//	# 场景 6：多个已知节点
//	go run ./examples/chat \
//	    --trust-stun \
//	    --known-peers "12D3KooW...@/ip4/1.2.3.4/udp/4001/quic-v1,12D3KooW...@/ip4/5.6.7.8/udp/4001/quic-v1"
//
// 架构说明：
//
//	┌─────────────────────────────────────────────────────────────────────────┐
//	│  统一 Chat 示例                                                          │
//	│                                                                         │
//	│  ┌─────────────────┐          ┌─────────────────┐                       │
//	│  │  Server Node    │          │  Client Nodes   │                       │
//	│  │  (公网可达)      │◄────────►│  (任意网络)      │                       │
//	│  │                 │          │                 │                       │
//	│  │ • Bootstrap     │          │ • mDNS 发现     │                       │
//	│  │ • Relay 中继    │          │ • Bootstrap 发现│                       │
//	│  │ • 可选参与聊天   │          │ • Relay 中继    │                       │
//	│  └─────────────────┘          └─────────────────┘                       │
//	│                                                                         │
//	│                    同一 Realm (PSK)                                      │
//	└─────────────────────────────────────────────────────────────────────────┘
package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/dep2p/go-dep2p"
	"github.com/dep2p/go-dep2p/config"
	"github.com/dep2p/go-dep2p/pkg/lib/log"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// 包级别 logger
var logger = log.Logger("chat")

// ════════════════════════════════════════════════════════════════════════════
//
//	配置常量
//
// ════════════════════════════════════════════════════════════════════════════
const (
	// defaultRealmKey 默认 Realm 密钥
	defaultRealmKey = "demo-chat-secret-key-2024"

	// defaultChatTopic 默认聊天主题
	defaultChatTopic = "chat/general"

	// privateProtocol 私聊协议标识
	privateProtocol = "private-chat"
)

// ════════════════════════════════════════════════════════════════════════════
//
//	全局状态
//
// ════════════════════════════════════════════════════════════════════════════

// ChatApp 聊天应用状态
type ChatApp struct {
	// 节点和 Realm
	node  *dep2p.Node
	realm *dep2p.Realm

	// PubSub 相关
	pubsub        *dep2p.PubSub
	topics        map[string]*dep2p.Topic
	subscriptions map[string]*dep2p.Subscription
	topicsMu      sync.RWMutex

	// 连接状态追踪
	connectedPeers   map[string]bool
	connectedPeersMu sync.RWMutex

	// 配置信息（用于 /status 显示）
	bootstrapAddr string
	relayAddr     string
	isServeMode   bool

	// 用户信息
	nickname string

	// 上下文
	ctx    context.Context
	cancel context.CancelFunc
}

// ════════════════════════════════════════════════════════════════════════════
//
//	主函数
//
// ════════════════════════════════════════════════════════════════════════════
func main() {
	// ────────────────────────────────────────────────────────────────────────
	// 解析命令行参数
	// ────────────────────────────────────────────────────────────────────────

	// Node 层配置（启动前）
	port := flag.Int("port", 0, "监听端口 (0 表示随机)")
	bootstrap := flag.String("bootstrap", "", "引导节点地址 (启动前配置)")
	relay := flag.String("relay", "", "Relay 地址 (启动前配置)")

	// 已知节点直连（云服务器场景）
	knownPeers := flag.String("known-peers", "", "已知节点列表 (格式: peerID1@addr1,peerID2@addr2)")

	// STUN 信任模式（云服务器场景）
	trustSTUN := flag.Bool("trust-stun", false, "信任 STUN 发现的地址，直接标记为已验证")

	// 服务能力开关
	serve := flag.Bool("serve", false, "服务模式：启用 Bootstrap + Relay 能力")
	publicAddr := flag.String("public-addr", "", "公网地址 (--serve 时必需)")

	// 其他
	nickname := flag.String("nick", "", "昵称 (默认使用节点ID前8位)")
	realmKey := flag.String("realm-key", defaultRealmKey, "Realm 密钥")

	flag.Parse()

	// 验证参数
	if *serve && *publicAddr == "" {
		fmt.Println("❌ 错误: --serve 模式必须指定 --public-addr")
		fmt.Println()
		fmt.Println("示例:")
		fmt.Println("  go run ./examples/chat --serve --port 4001 --public-addr \"/ip4/YOUR_PUBLIC_IP/udp/4001/quic-v1\"")
		os.Exit(1)
	}

	// ────────────────────────────────────────────────────────────────────────
	// 打印欢迎信息
	// ────────────────────────────────────────────────────────────────────────
	printBanner(*serve)

	// ────────────────────────────────────────────────────────────────────────
	// 创建上下文和信号处理
	// ────────────────────────────────────────────────────────────────────────
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	//
	// 详见下方 "启动优雅关闭信号处理" 部分
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, syscall.SIGINT, syscall.SIGTERM)

	// ────────────────────────────────────────────────────────────────────────
	// 配置数据目录和日志
	// ────────────────────────────────────────────────────────────────────────
	pid := os.Getpid()
	baseDir := "examples/chat/data"
	dataDir := filepath.Join(baseDir, fmt.Sprintf("node-%d", pid))
	logsDir := filepath.Join(baseDir, "logs")

	if err := os.MkdirAll(logsDir, 0750); err != nil {
		fmt.Printf("⚠️  无法创建数据目录: %v\n", err)
	}

	timestamp := time.Now().Format("20060102-150405")
	logFileName := filepath.Join(logsDir, fmt.Sprintf("chat-%s-%d.log", timestamp, pid))

	logFile, err := os.OpenFile(logFileName, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		fmt.Printf("⚠️  无法创建日志文件: %v\n", err)
	} else {
		log.SetOutputWithLevel(logFile, log.LevelDebug)
		defer logFile.Close()
		fmt.Printf("📝 日志文件: %s\n", logFileName)
	}

	// ────────────────────────────────────────────────────────────────────────
	// 打印版本信息（部署验证）
	// ────────────────────────────────────────────────────────────────────────
	fmt.Println()
	fmt.Printf("📦 %s\n", dep2p.VersionInfo())
	logger.Info("启动 Chat 应用", "version", dep2p.Version, "commit", dep2p.GitCommit, "buildDate", dep2p.BuildDate)

	// ────────────────────────────────────────────────────────────────────────
	// 构建启动选项（Node 层配置 - 启动前）
	// ────────────────────────────────────────────────────────────────────────
	fmt.Println("🚀 正在启动节点...")

	opts := []dep2p.Option{
		dep2p.WithDataDir(dataDir),
		dep2p.WithListenPort(*port),
	}

	// 选择预设
	if *serve {
		opts = append(opts, dep2p.WithPreset("server"))
	} else {
		opts = append(opts, dep2p.WithPreset("desktop"))
	}

	// Bootstrap 配置（启动前）
	if *bootstrap != "" {
		bootstrapPeers := strings.Split(*bootstrap, ",")
		for i, p := range bootstrapPeers {
			bootstrapPeers[i] = strings.TrimSpace(p)
		}
		opts = append(opts, dep2p.WithBootstrapPeers(bootstrapPeers...))
		fmt.Printf("🌐 引导节点: %d 个\n", len(bootstrapPeers))
		for _, p := range bootstrapPeers {
			fmt.Printf("   • %s\n", p)
		}
	}

	// Relay 配置（启动前）
	if *relay != "" {
		opts = append(opts, dep2p.WithRelay(true))
		opts = append(opts, dep2p.WithRelayAddr(*relay))
		fmt.Printf("Relay: %s\n", *relay)
	} else if *bootstrap != "" {
		// 有引导节点但没配中继，仍启用 Relay 客户端（尝试直连）
		opts = append(opts, dep2p.WithRelay(true))
		fmt.Println("🔄 Relay: 启用客户端（未指定 Relay 服务器）")
	}
	// 零配置模式：使用默认 Relay 设置（由预设决定）

	// 服务模式：启用基础设施能力
	if *serve {
		opts = append(opts, dep2p.EnableInfrastructure(true))
		opts = append(opts, dep2p.WithPublicAddr(*publicAddr))
		fmt.Println("服务模式: Bootstrap + Relay")
	}

	// 已知节点直连（云服务器场景）
	if *knownPeers != "" {
		peers := parseKnownPeers(*knownPeers)
		if len(peers) > 0 {
			opts = append(opts, dep2p.WithKnownPeers(peers...))
			fmt.Printf("🔗 已知节点: %d 个\n", len(peers))
			for _, p := range peers {
				fmt.Printf("   • %s → %v\n", p.PeerID[:8], p.Addrs)
			}
		}
	}

	// STUN 信任模式（云服务器场景）
	if *trustSTUN {
		opts = append(opts, dep2p.WithTrustSTUNAddresses(true))
		fmt.Println("🛡️  STUN 信任模式: 已启用")
	}

	if logFile != nil {
		opts = append(opts, dep2p.WithLogFile(logFileName))
	}

	// ────────────────────────────────────────────────────────────────────────
	// 启动节点
	// ────────────────────────────────────────────────────────────────────────
	node, err := dep2p.Start(ctx, opts...)
	if err != nil {
		fmt.Printf("❌ 启动节点失败: %v\n", err)
		os.Exit(1)
	}

	//
	// 在 node 创建后启动，确保能够调用 node.Close() 以发送 MemberLeave 广播
	// 这是正确的优雅关闭流程，而不是之前的 os.Exit(0) 直接退出
	go func() {
		<-signalCh
		fmt.Println("\n\n🔄 正在优雅关闭...")
		fmt.Println("   发送 MemberLeave 广播中...")

		// 主动调用 Close，触发 MemberLeave 广播
		// Close 会调用 Realm.Leave()，进而调用 BroadcastMemberLeave()
		if err := node.Close(); err != nil {
			logger.Error("关闭节点失败", "error", err)
		}

		fmt.Println("   节点已关闭")
		fmt.Println("再见! 👋")
		os.Exit(0)
	}()

	// ────────────────────────────────────────────────────────────────────────
	// 加入 Realm
	// ────────────────────────────────────────────────────────────────────────
	fmt.Println("🏠 正在加入 Realm...")

	realm, err := node.JoinRealm(ctx, []byte(*realmKey))
	if err != nil {
		fmt.Printf("❌ 加入 Realm 失败: %v\n", err)
		os.Exit(1)
	}

	// ────────────────────────────────────────────────────────────────────────
	// 初始化聊天应用
	// ────────────────────────────────────────────────────────────────────────
	app := &ChatApp{
		node:           node,
		realm:          realm,
		pubsub:         realm.PubSub(),
		topics:         make(map[string]*dep2p.Topic),
		subscriptions:  make(map[string]*dep2p.Subscription),
		connectedPeers: make(map[string]bool),
		bootstrapAddr:  *bootstrap,
		relayAddr:      *relay,
		isServeMode:    *serve,
		ctx:            ctx,
		cancel:         cancel,
	}

	if *nickname != "" {
		app.nickname = *nickname
	} else {
		app.nickname = node.ID()[:8]
	}

	// ────────────────────────────────────────────────────────────────────────
	// 注册私聊处理器
	// ────────────────────────────────────────────────────────────────────────
	streams := realm.Streams()
	err = streams.RegisterHandler(privateProtocol, func(stream *dep2p.BiStream) {
		app.handlePrivateMessage(stream)
	})
	if err != nil {
		fmt.Printf("⚠️  注册私聊处理器失败: %v\n", err)
	}

	// ────────────────────────────────────────────────────────────────────────
	// 订阅连接事件
	// ────────────────────────────────────────────────────────────────────────
	go app.subscribeConnectionEvents()

	// ────────────────────────────────────────────────────────────────────────
	// 订阅 Realm 成员事件（使用新的用户层 API）
	// ────────────────────────────────────────────────────────────────────────
	app.subscribeRealmMemberEvents()

	// ────────────────────────────────────────────────────────────────────────
	// 订阅默认聊天主题
	// ────────────────────────────────────────────────────────────────────────
	if err := app.subscribeTopic(defaultChatTopic); err != nil {
		fmt.Printf("⚠️  订阅默认主题失败: %v\n", err)
	}

	// ────────────────────────────────────────────────────────────────────────
	// 等待地址发现完成（STUN/NAT 探测是异步的）
	// ────────────────────────────────────────────────────────────────────────
	fmt.Print("🔍 正在发现外部地址")
	waitForAddressDiscovery(node, 3*time.Second)
	fmt.Println()

	// ────────────────────────────────────────────────────────────────────────
	// 打印节点信息
	// ────────────────────────────────────────────────────────────────────────
	printNodeInfo(node, realm, app.nickname, *serve, *publicAddr)

	// ────────────────────────────────────────────────────────────────────────
	// 启动用户输入处理
	// ────────────────────────────────────────────────────────────────────────
	printCommandHints()
	app.handleInput()
}

// ════════════════════════════════════════════════════════════════════════════
//
//	PubSub 群聊
//
// ════════════════════════════════════════════════════════════════════════════
func (app *ChatApp) subscribeTopic(topicName string) error {
	app.topicsMu.Lock()
	defer app.topicsMu.Unlock()

	if _, exists := app.topics[topicName]; exists {
		return fmt.Errorf("已经订阅了主题 %s", topicName)
	}

	topic, err := app.pubsub.Join(topicName)
	if err != nil {
		return fmt.Errorf("加入主题失败: %w", err)
	}

	sub, err := topic.Subscribe()
	if err != nil {
		topic.Close()
		return fmt.Errorf("订阅主题失败: %w", err)
	}

	app.topics[topicName] = topic
	app.subscriptions[topicName] = sub

	go app.receiveMessages(topicName, sub)

	fmt.Printf("✅ 已订阅主题: %s\n", topicName)
	return nil
}

func (app *ChatApp) unsubscribeTopic(topicName string) error {
	app.topicsMu.Lock()
	defer app.topicsMu.Unlock()

	sub, exists := app.subscriptions[topicName]
	if !exists {
		return fmt.Errorf("未订阅主题 %s", topicName)
	}

	sub.Cancel()
	delete(app.subscriptions, topicName)

	if topic, ok := app.topics[topicName]; ok {
		topic.Close()
		delete(app.topics, topicName)
	}

	fmt.Printf("✅ 已取消订阅主题: %s\n", topicName)
	return nil
}

func (app *ChatApp) receiveMessages(topicName string, sub *dep2p.Subscription) {
	for {
		msg, err := sub.Next(app.ctx)
		if err != nil {
			return
		}

		if msg.From == app.node.ID() {
			continue
		}

		senderID := msg.From
		if len(senderID) > 8 {
			senderID = senderID[:8]
		}
		// 清除当前行并打印消息，然后重新显示提示符
		fmt.Printf("\r\033[K\033[32m[%s][%s]\033[0m %s\n", topicName, senderID, string(msg.Data))
		fmt.Printf("\033[34m[%s]\033[0m ", app.nickname)
	}
}

func (app *ChatApp) publishMessage(topicName string, message string) error {
	app.topicsMu.RLock()
	topic, exists := app.topics[topicName]
	app.topicsMu.RUnlock()

	if !exists {
		return fmt.Errorf("未订阅主题 %s", topicName)
	}

	fullMessage := fmt.Sprintf("%s: %s", app.nickname, message)
	return topic.Publish(app.ctx, []byte(fullMessage))
}

// ════════════════════════════════════════════════════════════════════════════
//
//	Streams 私聊
//
// ════════════════════════════════════════════════════════════════════════════
func (app *ChatApp) sendPrivateMessage(targetID string, message string) error {
	streams := app.realm.Streams()

	logger.Debug("准备发送私聊", "target", targetID[:8])

	stream, err := streams.Open(app.ctx, targetID, privateProtocol)
	if err != nil {
		return fmt.Errorf("打开私聊流失败: %w", err)
	}
	defer stream.Close()

	fullMessage := fmt.Sprintf("%s: %s\n", app.nickname, message)

	n, err := stream.Write([]byte(fullMessage))
	if err != nil {
		return fmt.Errorf("发送私聊消息失败: %w", err)
	}

	logger.Debug("私聊消息已写入", "target", targetID[:8], "bytes", n)

	if err := stream.CloseWrite(); err != nil {
		logger.Warn("关闭写端失败", "error", err)
	}

	fmt.Printf("\033[34m[私聊 → %s]\033[0m %s\n", targetID[:8], message)
	return nil
}

func (app *ChatApp) handlePrivateMessage(stream *dep2p.BiStream) {
	defer stream.Close()

	senderID := stream.RemotePeer()
	senderLabel := senderID
	if len(senderLabel) > 8 {
		senderLabel = senderLabel[:8]
	}

	logger.Debug("开始处理私聊流", "sender", senderLabel)

	data, err := io.ReadAll(stream)
	if err != nil {
		logger.Error("读取私聊消息失败", "sender", senderLabel, "error", err)
		return
	}

	if len(data) == 0 {
		logger.Debug("私聊消息为空", "sender", senderLabel)
		return
	}

	logger.Debug("读取私聊消息成功", "sender", senderLabel, "bytes", len(data))

	// 清除当前行并打印消息，然后重新显示提示符
	// 消息末尾可能有换行符，需要处理
	msgStr := strings.TrimSuffix(string(data), "\n")
	fmt.Printf("\r\033[K\033[35m[私聊 ← %s]\033[0m %s\n", senderLabel, msgStr)
	fmt.Printf("\033[34m[%s]\033[0m ", app.nickname)
}

// ════════════════════════════════════════════════════════════════════════════
//
//	EventBus 事件监听
//
// ════════════════════════════════════════════════════════════════════════════
func (app *ChatApp) subscribeConnectionEvents() {
	eventBus := app.node.Host().EventBus()
	if eventBus == nil {
		logger.Warn("EventBus 不可用")
		return
	}

	connectedSub, err := eventBus.Subscribe(new(types.EvtPeerConnected))
	if err != nil {
		logger.Error("订阅连接事件失败", "error", err)
		return
	}
	defer connectedSub.Close()

	disconnectedSub, err := eventBus.Subscribe(new(types.EvtPeerDisconnected))
	if err != nil {
		logger.Error("订阅断开事件失败", "error", err)
		return
	}
	defer disconnectedSub.Close()

	for {
		select {
		case <-app.ctx.Done():
			return

		case evt := <-connectedSub.Out():
			if e, ok := evt.(*types.EvtPeerConnected); ok {
				fullPeerID := string(e.PeerID)
				peerLabel := fullPeerID
				if len(peerLabel) > 8 {
					peerLabel = peerLabel[:8]
				}

				app.connectedPeersMu.Lock()
				if app.connectedPeers[fullPeerID] {
					app.connectedPeersMu.Unlock()
					continue
				}
				app.connectedPeers[fullPeerID] = true
				app.connectedPeersMu.Unlock()

				logger.Info("节点已连接", "peer", peerLabel)

				// 打印到终端（绿色）
				fmt.Printf("\r\033[K\033[32m[系统] 🔗 节点已连接: %s\033[0m\n", peerLabel)
				fmt.Printf("\033[34m[%s]\033[0m ", app.nickname)
			}

		case evt := <-disconnectedSub.Out():
			if e, ok := evt.(*types.EvtPeerDisconnected); ok {
				fullPeerID := string(e.PeerID)
				peerLabel := fullPeerID
				if len(peerLabel) > 8 {
					peerLabel = peerLabel[:8]
				}

				app.connectedPeersMu.Lock()
				if !app.connectedPeers[fullPeerID] {
					app.connectedPeersMu.Unlock()
					continue
				}
				delete(app.connectedPeers, fullPeerID)
				app.connectedPeersMu.Unlock()

				logger.Info("节点已断开", "peer", peerLabel)

				// 打印到终端（黄色）
				fmt.Printf("\r\033[K\033[33m[系统] ⚡ 节点已断开: %s\033[0m\n", peerLabel)
				fmt.Printf("\033[34m[%s]\033[0m ", app.nickname)
			}
		}
	}
}

// ════════════════════════════════════════════════════════════════════════════
//
//	Realm 成员事件订阅
//
// ════════════════════════════════════════════════════════════════════════════
func (app *ChatApp) subscribeRealmMemberEvents() {
	// 订阅成员加入事件
	if err := app.realm.OnMemberJoin(func(peerID string) {
		// 跳过自己
		if peerID == app.node.ID() {
			return
		}

		peerLabel := peerID
		if len(peerLabel) > 8 {
			peerLabel = peerLabel[:8]
		}

		logger.Info("Realm 成员加入", "peer", peerLabel)

		// 打印到终端（青色）
		fmt.Printf("\r\033[K\033[36m[Realm] 👋 成员加入: %s\033[0m\n", peerLabel)
		fmt.Printf("\033[34m[%s]\033[0m ", app.nickname)
	}); err != nil {
		logger.Warn("订阅成员加入事件失败", "error", err)
	}

	// 订阅成员离开事件
	if err := app.realm.OnMemberLeave(func(peerID string) {
		peerLabel := peerID
		if len(peerLabel) > 8 {
			peerLabel = peerLabel[:8]
		}

		logger.Info("Realm 成员离开", "peer", peerLabel)

		// 打印到终端（红色）
		fmt.Printf("\r\033[K\033[31m[Realm] 👋 成员离开: %s\033[0m\n", peerLabel)
		fmt.Printf("\033[34m[%s]\033[0m ", app.nickname)
	}); err != nil {
		logger.Warn("订阅成员离开事件失败", "error", err)
	}
}

// ════════════════════════════════════════════════════════════════════════════
//
//	用户输入处理
//
// ════════════════════════════════════════════════════════════════════════════
func (app *ChatApp) handleInput() {
	reader := bufio.NewReader(os.Stdin)

	for {
		select {
		case <-app.ctx.Done():
			return
		default:
			fmt.Printf("\033[34m[%s]\033[0m ", app.nickname)

			line, err := reader.ReadString('\n')
			if err != nil {
				return
			}

			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}

			if strings.HasPrefix(line, "/") {
				app.handleCommand(line)
			} else {
				if err := app.publishMessage(defaultChatTopic, line); err != nil {
					fmt.Printf("❌ 发送失败: %v\n", err)
				}
			}
		}
	}
}

func (app *ChatApp) handleCommand(cmd string) {
	parts := strings.Fields(cmd)
	if len(parts) == 0 {
		return
	}

	switch parts[0] {
	case "/quit", "/exit", "/q":
		fmt.Println("正在退出...")
		app.cancel()
		time.Sleep(500 * time.Millisecond)
		os.Exit(0)

	case "/peers", "/list", "/members":
		members := app.realm.Members()
		fmt.Printf("📋 在线成员 (%d):\n", len(members))
		if len(members) == 0 {
			fmt.Println("   (暂无其他成员)")
		} else {
			for _, m := range members {
				displayID := m
				if len(displayID) > 8 {
					displayID = displayID[:8]
				}
				if m == app.node.ID() {
					fmt.Printf("   • %s (我)\n", displayID)
				} else {
					fmt.Printf("   • %s\n", displayID)
				}
			}
		}

	case "/msg", "/pm", "/private":
		if len(parts) < 3 {
			fmt.Println("用法: /msg <节点ID或前缀> <消息>")
			fmt.Println("示例: /msg GUjWXgqA hello")
			return
		}
		targetInput := parts[1]
		message := strings.Join(parts[2:], " ")

		// 尝试从完整地址中提取节点 ID
		targetID := app.resolveTargetID(targetInput)
		if targetID == "" {
			fmt.Printf("❌ 未找到匹配的节点: %s\n", targetInput)
			fmt.Println("   提示: 使用节点 ID 或 ID 前缀，而不是完整地址")
			fmt.Println("   示例: /msg GUjWXgqA hello")
			return
		}

		if err := app.sendPrivateMessage(targetID, message); err != nil {
			fmt.Printf("❌ 私聊失败: %v\n", err)
		}

	case "/connect":
		// Node 层操作：直接连接节点
		if len(parts) < 2 {
			fmt.Println("用法: /connect <完整地址>")
			fmt.Println("示例: /connect /ip4/192.168.1.100/udp/9000/quic-v1/p2p/12D3KooW...")
			return
		}
		addr := parts[1]
		app.connectPeer(addr)

	case "/relay":
		// Realm 层操作：Gateway 配置
		if len(parts) < 2 {
			fmt.Println("用法:")
			fmt.Println("  /gateway set <地址>  - 设置 Gateway")
			fmt.Println("  /gateway remove      - 移除 Gateway")
			fmt.Println("  /gateway enable      - 启用 Gateway 服务（需公网可达）")
			fmt.Println("  /gateway disable     - 禁用 Gateway 服务")
			fmt.Println("  /relay status        - 查看 Relay 状态")
			return
		}
		app.handleRelayCommand(parts[1:])

	case "/status":
		app.printStatus()

	case "/sub", "/subscribe":
		if len(parts) < 2 {
			fmt.Println("用法: /sub <主题名>")
			return
		}
		topicName := parts[1]
		if err := app.subscribeTopic(topicName); err != nil {
			fmt.Printf("❌ %v\n", err)
		}

	case "/unsub", "/unsubscribe":
		if len(parts) < 2 {
			fmt.Println("用法: /unsub <主题名>")
			return
		}
		topicName := parts[1]
		if err := app.unsubscribeTopic(topicName); err != nil {
			fmt.Printf("❌ %v\n", err)
		}

	case "/topics":
		app.topicsMu.RLock()
		topics := make([]string, 0, len(app.topics))
		for t := range app.topics {
			topics = append(topics, t)
		}
		app.topicsMu.RUnlock()

		fmt.Printf("📝 已订阅主题 (%d):\n", len(topics))
		for _, t := range topics {
			fmt.Printf("   • %s\n", t)
		}

	case "/info":
		printNodeInfo(app.node, app.realm, app.nickname, app.isServeMode, "")

	case "/help", "/?":
		printHelp()

	default:
		fmt.Printf("未知命令: %s\n", parts[0])
		fmt.Println("输入 /help 查看帮助")
	}
}

// connectPeer 连接 Realm 成员
//
// 使用 realm.Connect() 进行 Realm 级别连接，包含 PSK 认证。
// 连接成功 = 可通信（传输层 + Realm 认证完成）。
//
// 支持格式（底层自动处理）：
//   - ConnectionTicket: dep2p://...（便于分享）
//   - Full Address: /ip4/x.x.x.x/udp/port/quic-v1/p2p/12D3KooW...
//   - NodeID: 12D3KooW...（通过 DHT 发现）
//
// 底层自动处理：
//   - 解析 target 提取 NodeID 和地址
//   - 如果目标不是成员，自动建立连接并等待 PSK 认证
//   - 认证完成后返回已认证的连接
func (app *ChatApp) connectPeer(target string) {
	// 截断显示（避免票据过长）
	displayTarget := target
	if len(displayTarget) > 60 {
		displayTarget = displayTarget[:60] + "..."
	}
	fmt.Printf("🔗 正在连接 %s\n", displayTarget)

	ctx, cancel := context.WithTimeout(app.ctx, 30*time.Second)
	defer cancel()

	// 直接调用 Realm.Connect，底层自动处理所有格式和认证流程
	_, err := app.realm.Connect(ctx, target)
	if err != nil {
		fmt.Printf("❌ 连接失败: %v\n", err)
		fmt.Println("   支持的格式:")
		fmt.Println("   • 连接票据: dep2p://...（推荐）")
		fmt.Println("   • 完整地址: /ip4/x.x.x.x/udp/port/quic-v1/p2p/12D3KooW...")
		fmt.Println("   • 节点 ID:  12D3KooW...（需要 DHT 发现）")
		return
	}

	fmt.Println("✅ 连接成功")
}

// handleRelayCommand 处理 Relay 命令
//
// v2.0 统一 Relay 架构：Relay 功能已移至节点级别
// Realm 不再直接管理 Relay 连接
func (app *ChatApp) handleRelayCommand(args []string) {
	if len(args) == 0 {
		app.printRelayStatus()
		return
	}

	switch args[0] {
	case "status":
		app.printRelayStatus()

	default:
		fmt.Printf("v2.0 统一 Relay 架构：Relay 功能已移至节点级别\n")
		fmt.Println("请使用节点级 Relay 配置 (dep2p.WithRelayAddr)")
	}
}

// printRelayStatus 打印 Relay 状态
//
// v2.0 统一 Relay 架构：显示节点级 Relay 配置状态
func (app *ChatApp) printRelayStatus() {
	fmt.Println()
	fmt.Println("╭────────────────────────────────────────╮")
	fmt.Println("│ 🔄 Relay 状态                          │")
	fmt.Println("├────────────────────────────────────────┤")

	if app.relayAddr != "" {
		fmt.Println("│ Relay:        ✅ 已配置（节点级）")
		displayAddr := app.relayAddr
		if len(displayAddr) > 35 {
			displayAddr = displayAddr[:35] + "..."
		}
		fmt.Printf("│               %s\n", displayAddr)
	} else {
		fmt.Println("│ Relay:        ❌ 未配置")
		fmt.Println("│               提示: 使用 --relay 参数配置")
	}

	fmt.Println("│")
	fmt.Println("│ v2.0 统一 Relay 架构：")
	fmt.Println("│ Relay 功能已移至节点级别统一管理")
	fmt.Println("╰────────────────────────────────────────╯")
	fmt.Println()
}

// printStatus 打印网络状态
func (app *ChatApp) printStatus() {
	fmt.Println()
	fmt.Println("╭────────────────────────────────────────────────────────────────╮")
	fmt.Println("│ 📊 网络状态                                                     │")
	fmt.Println("├────────────────────────────────────────────────────────────────┤")

	// Node 层
	fmt.Println("│ ─── Node 层 ───")
	fmt.Println("│ mDNS:           ✅ 已启用（局域网发现）")

	if app.bootstrapAddr != "" {
		fmt.Println("│ Bootstrap:      ✅ 已配置")
		displayAddr := app.bootstrapAddr
		if len(displayAddr) > 45 {
			displayAddr = displayAddr[:45] + "..."
		}
		fmt.Printf("│                    %s\n", displayAddr)
	} else {
		fmt.Println("│ Bootstrap:      ❌ 未配置")
	}

	if app.relayAddr != "" {
		fmt.Println("│ Relay:          配置完成")
	} else {
		fmt.Println("│ Relay:          未配置")
	}

	if app.isServeMode {
		fmt.Println("│ 服务能力:       Bootstrap + Relay")
	}

	// Realm 层
	fmt.Println("│")
	fmt.Println("│ ─── Realm 层 ───")
	fmt.Printf("│ Realm ID:       %s\n", app.realm.ID())

	// 连接统计
	fmt.Println("│")
	fmt.Println("│ ─── 连接统计 ───")
	app.connectedPeersMu.RLock()
	peerCount := len(app.connectedPeers)
	app.connectedPeersMu.RUnlock()
	fmt.Printf("│ 已连接节点:     %d\n", peerCount)

	fmt.Println("╰────────────────────────────────────────────────────────────────╯")
	fmt.Println()
}

func (app *ChatApp) findPeerByPrefix(prefix string) string {
	members := app.realm.Members()
	for _, m := range members {
		if strings.HasPrefix(m, prefix) && m != app.node.ID() {
			return m
		}
	}
	return ""
}

// resolveTargetID 解析目标节点 ID
//
// 支持多种输入格式：
//   - 节点 ID 前缀: GUjWXgqA
//   - 完整节点 ID: GUjWXgqA8ag9pD2Q5tBenVKQ1zkKG5NDM6L4HqoCuvv9
//   - 完整地址: /ip4/x.x.x.x/udp/4003/quic-v1/p2p/GUjWXgqA...
func (app *ChatApp) resolveTargetID(input string) string {
	// 1. 检查是否是完整 multiaddr 地址（包含 /p2p/）
	if strings.Contains(input, "/p2p/") {
		// 提取 /p2p/ 后面的节点 ID
		parts := strings.Split(input, "/p2p/")
		if len(parts) >= 2 {
			nodeID := parts[len(parts)-1]
			// 移除可能的尾部路径
			if idx := strings.Index(nodeID, "/"); idx > 0 {
				nodeID = nodeID[:idx]
			}
			// 验证提取的 ID 是否在成员列表中
			if app.findPeerByPrefix(nodeID) != "" {
				return nodeID
			}
			// 即使不在成员列表中，也返回提取的 ID（允许直接尝试连接）
			members := app.realm.Members()
			for _, m := range members {
				if m == nodeID && m != app.node.ID() {
					return m
				}
			}
		}
	}

	// 2. 作为节点 ID 或前缀进行匹配
	return app.findPeerByPrefix(input)
}

// ════════════════════════════════════════════════════════════════════════════
//
//	辅助函数
//
// ════════════════════════════════════════════════════════════════════════════

// parseKnownPeers 解析已知节点参数
//
// 格式: peerID1@addr1,peerID2@addr2
// 或者: peerID1@addr1;addr2,peerID2@addr3
//
// 示例:
//
//	"12D3KooW...@/ip4/1.2.3.4/udp/4001/quic-v1"
//	"12D3KooW...@/ip4/1.2.3.4/udp/4001/quic-v1;/ip4/5.6.7.8/udp/4001/quic-v1"
func parseKnownPeers(input string) []config.KnownPeer {
	if input == "" {
		return nil
	}

	var peers []config.KnownPeer

	// 按逗号分割不同的节点
	peerStrs := strings.Split(input, ",")
	for _, peerStr := range peerStrs {
		peerStr = strings.TrimSpace(peerStr)
		if peerStr == "" {
			continue
		}

		// 按 @ 分割 PeerID 和地址
		parts := strings.SplitN(peerStr, "@", 2)
		if len(parts) != 2 {
			fmt.Printf("⚠️  无法解析已知节点: %s (格式: peerID@addr)\n", peerStr)
			continue
		}

		peerID := strings.TrimSpace(parts[0])
		addrsStr := strings.TrimSpace(parts[1])

		if peerID == "" || addrsStr == "" {
			fmt.Printf("⚠️  无效的已知节点配置: %s\n", peerStr)
			continue
		}

		// 按分号分割多个地址
		addrs := strings.Split(addrsStr, ";")
		var validAddrs []string
		for _, addr := range addrs {
			addr = strings.TrimSpace(addr)
			if addr != "" {
				validAddrs = append(validAddrs, addr)
			}
		}

		if len(validAddrs) == 0 {
			fmt.Printf("⚠️  已知节点无有效地址: %s\n", peerID)
			continue
		}

		peers = append(peers, config.KnownPeer{
			PeerID: peerID,
			Addrs:  validAddrs,
		})
	}

	return peers
}

func printBanner(serveMode bool) {
	fmt.Println()
	fmt.Println("╔════════════════════════════════════════════════════════════╗")
	if serveMode {
		fmt.Println("║           DeP2P Chat - Server Mode                         ║")
	} else {
		fmt.Println("║           DeP2P Chat Demo                                  ║")
	}
	fmt.Println("║           P2P 聊天示例                                       ║")
	fmt.Println("╚════════════════════════════════════════════════════════════╝")
	fmt.Println()
	fmt.Println("功能:")
	fmt.Println("  • mDNS 自动发现   - 同一局域网节点自动连接")
	fmt.Println("  • Bootstrap 发现  - 通过引导节点发现其他节点")
	fmt.Println("  • Relay           - NAT 后节点通过中继通信")
	fmt.Println("  • Gateway         - Realm 内部网关（运行时配置）")
	fmt.Println("  • PubSub 群聊     - 基于 GossipSub 的发布订阅")
	fmt.Println("  • Streams 私聊    - 基于双向流的点对点消息")
}

func printNodeInfo(node *dep2p.Node, realm *dep2p.Realm, nickname string, serveMode bool, _ string) {
	fmt.Println()
	fmt.Println("╭─────────────────────────────────────────────────────────────────╮")
	fmt.Printf("│ 👤 昵称:    %s\n", nickname)
	fmt.Printf("│ 🆔 节点 ID: %s\n", node.ID())
	fmt.Printf("│ 🏠 Realm:   %s\n", realm.ID())

	if serveMode {
		fmt.Println("│")
		fmt.Println("│ 服务能力: Bootstrap, Relay")
	}

	// 获取可分享的地址（过滤 0.0.0.0 等不可连接地址）
	shareableAddrs := node.ShareableAddrs()

	// 如果 ShareableAddrs 为空，尝试从监听地址中过滤
	if len(shareableAddrs) == 0 {
		for _, addr := range node.ListenAddrs() {
			// 过滤不可连接的地址
			if strings.Contains(addr, "/0.0.0.0/") ||
				strings.Contains(addr, "/::/") ||
				strings.Contains(addr, "/127.0.0.1/") {
				continue
			}
			fullAddr := fmt.Sprintf("%s/p2p/%s", addr, node.ID())
			shareableAddrs = append(shareableAddrs, fullAddr)
		}
	}

	// 只有在有可分享地址时才显示连接信息
	if len(shareableAddrs) > 0 {
		fmt.Println("│")
		fmt.Println("│ 🔗 连接地址（分享给其他人）:")
		for _, addr := range shareableAddrs {
			fmt.Printf("│    %s\n", addr)
		}

		// 只有在有可分享地址时才显示票据
		if ticket := node.ConnectionTicket(); ticket != "" {
			fmt.Println("│")
			fmt.Println("│ 📋 连接票据（便于分享）:")
			fmt.Printf("│    %s\n", ticket)
		}
	} else {
		// 没有可分享地址时，只显示提示信息
		fmt.Println("│")
		fmt.Println("│ ℹ️  暂无可分享的公网地址")
		fmt.Println("│    使用 /info 命令随时查看最新状态")
	}

	fmt.Println("╰─────────────────────────────────────────────────────────────────╯")
	fmt.Println()
}

func printCommandHints() {
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("命令提示:")
	fmt.Println("  直接输入消息      → 发送群聊")
	fmt.Println("  /msg <ID> <消息>  → 发送私聊 (ID 可只输入前几位)")
	fmt.Println("  /connect <目标>   → 连接节点 (支持地址/票据/NodeID)")
	fmt.Println("  /peers            → 查看在线成员")
	fmt.Println("  /status           → 查看网络状态")
	fmt.Println("  /gateway          → Gateway 配置")
	fmt.Println("  /help             → 查看全部命令")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println()
}

func printHelp() {
	fmt.Println()
	fmt.Println("═══════════════════════════════════════════════════════════════════")
	fmt.Println("                          命令帮助")
	fmt.Println("═══════════════════════════════════════════════════════════════════")
	fmt.Println()
	fmt.Println("群聊:")
	fmt.Println("  直接输入消息       - 发送到默认主题 (chat/general)")
	fmt.Println("  /sub <主题>        - 订阅新主题")
	fmt.Println("  /unsub <主题>      - 取消订阅主题")
	fmt.Println("  /topics            - 列出已订阅主题")
	fmt.Println()
	fmt.Println("私聊:")
	fmt.Println("  /msg <ID> <消息>   - 发送私聊消息 (ID 可以只输入前几位)")
	fmt.Println()
	fmt.Println("连接:")
	fmt.Println("  /connect <目标>    - 连接节点（支持三种格式）")
	fmt.Println("                       • 完整地址: /ip4/x.x.x.x/udp/port/quic-v1/p2p/12D3KooW...")
	fmt.Println("                       • 连接票据: dep2p://...")
	fmt.Println("                       • 节点 ID:  12D3KooW...（需要 DHT 发现）")
	fmt.Println()
	fmt.Println("Relay（Realm 层，运行时配置）:")
	fmt.Println("  /gateway set <地址>  - 设置 Gateway")
	fmt.Println("  /gateway remove      - 移除 Gateway")
	fmt.Println("  /gateway enable      - 启用 Gateway 服务（需公网可达）")
	fmt.Println("  /gateway disable     - 禁用 Gateway 服务")
	fmt.Println("  /relay status      - 查看 Relay 状态")
	fmt.Println()
	fmt.Println("信息:")
	fmt.Println("  /peers             - 列出在线成员")
	fmt.Println("  /status            - 查看网络状态")
	fmt.Println("  /info              - 显示节点信息")
	fmt.Println("  /help              - 显示此帮助")
	fmt.Println()
	fmt.Println("其他:")
	fmt.Println("  /quit              - 退出程序")
	fmt.Println()
	fmt.Println("═══════════════════════════════════════════════════════════════════")
	fmt.Println()
}

// waitForAddressDiscovery 等待地址发现完成
//
// STUN/NAT 探测是异步的，需要等待一段时间让外部地址发现完成。
// 该函数会轮询检查 ShareableAddrs()，直到有地址或超时。
func waitForAddressDiscovery(node *dep2p.Node, timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	checkInterval := 200 * time.Millisecond

	for time.Now().Before(deadline) {
		// 检查是否已发现可分享地址
		if addrs := node.ShareableAddrs(); len(addrs) > 0 {
			fmt.Print(" ✓")
			return
		}

		// 显示进度
		fmt.Print(".")
		time.Sleep(checkInterval)
	}

	// 超时，继续但可能没有外部地址
	fmt.Print(" (超时)")
}
