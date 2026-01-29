// Package main 提供 dep2p 命令行入口
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/dep2p/go-dep2p"
	"github.com/dep2p/go-dep2p/config"
	"github.com/dep2p/go-dep2p/pkg/lib/log"
)

var logger = log.Logger("dep2p/cmd")

// ═══════════════════════════════════════════════════════════════════════════
// 命令行参数
// ═══════════════════════════════════════════════════════════════════════════
//
// 设计原则（参见 design/_discussions/20260116-config-boundary.md）：
//
//   命令行参数：运行时覆盖 / 快速测试（「这次运行」想怎么跑）
//   JSON 配置文件：持久化配置 / 长期运行（「这个节点」的固定配置）
//
// 已移除的参数（应通过配置文件设置）：
//   --relay, --nat, --low-water, --high-water, --bootstrap, --system-relay
//
// ═══════════════════════════════════════════════════════════════════════════
var (
	// ─────────────────────────────────────────────────────────────────────
	// 运行时参数（快速指定）
	// ─────────────────────────────────────────────────────────────────────
	port         = flag.Int("port", 0, "监听端口（0 = 随机端口）")
	configFile   = flag.String("config", "", "配置文件路径")
	preset       = flag.String("preset", "desktop", "预设配置 (mobile/desktop/server/minimal)")
	identityFile = flag.String("identity", "", "身份密钥文件路径")
	publicAddr   = flag.String("public-addr", "", "公网可达地址（基础设施节点必需）")
	dataDir      = flag.String("data-dir", "", "数据目录（默认: ./data）")

	// ─────────────────────────────────────────────────────────────────────
	// 能力开关（ADR-0009 / ADR-0010）
	// ─────────────────────────────────────────────────────────────────────
	enableBootstrap = flag.Bool("enable-bootstrap", false, "启用 Bootstrap 服务能力")
	enableRelay     = flag.Bool("enable-relay", false, "启用 Relay 服务能力")
	enableInfra     = flag.Bool("enable-infra", false, "启用基础设施（Bootstrap + Relay）")

	// ─────────────────────────────────────────────────────────────────────
	// 日志参数
	// ─────────────────────────────────────────────────────────────────────
	logFile = flag.String("log", "", "日志文件路径")
	logDir  = flag.String("log-dir", "logs", "日志目录")
	autoLog = flag.Bool("auto-log", true, "自动生成日志文件")

	// ─────────────────────────────────────────────────────────────────────
	// 信息显示
	// ─────────────────────────────────────────────────────────────────────
	showVersion = flag.Bool("version", false, "显示版本信息")
	showHelp    = flag.Bool("help", false, "显示帮助信息")
)

// actualLogPath 实际使用的日志文件路径（用于输出显示）
var actualLogPath string

// runtimeConfig 运行时配置（不属于 config.Config）
type runtimeConfig struct {
	preset     string
	listenPort int
	logFile    string
	publicAddr string // 公网可达地址（能力开关需要）
	dataDir    string // 数据目录
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "错误: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	flag.Parse()

	// 显示版本
	if *showVersion {
		printVersion()
		return nil
	}

	// 显示帮助
	if *showHelp {
		printHelp()
		return nil
	}

	// 设置日志
	var logFileHandle *os.File
	var err error
	actualLogPath, logFileHandle, err = setupLogging()
	if err != nil {
		fmt.Fprintf(os.Stderr, "警告: %v\n", err)
		fmt.Fprintln(os.Stderr, "将继续使用控制台输出日志")
	}
	if logFileHandle != nil {
		defer func() { _ = logFileHandle.Close() }()
	}

	// 构建选项
	opts, err := buildOptions()
	if err != nil {
		return fmt.Errorf("配置错误: %w", err)
	}

	// 创建上下文
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 打印版本信息（部署验证）
	fmt.Printf("📦 %s\n", dep2p.VersionInfo())
	logger.Info("启动 dep2p 节点", "version", dep2p.Version, "commit", dep2p.GitCommit, "buildDate", dep2p.BuildDate)

	// 启动节点
	fmt.Println("正在启动 dep2p 节点...")
	endpoint, err := dep2p.Start(ctx, opts...)
	if err != nil {
		return fmt.Errorf("启动失败: %w", err)
	}
	defer func() { _ = endpoint.Close() }()

	// 显示节点信息（美化输出）
	printNodeInfo(endpoint)

	// 等待退出信号
	fmt.Println("节点已启动，按 Ctrl+C 退出")
	waitForSignal()

	fmt.Println("\n正在关闭节点...")
	return nil
}

// buildOptions 构建选项
//
// 配置优先级（从高到低）：
//  1. 命令行参数（运行时覆盖）
//  2. 环境变量（DEP2P_* 前缀）
//  3. 配置文件（持久化配置）
//  4. 预设默认值
//
// 配置边界：
//   - 命令行参数：运行时覆盖 / 快速测试
//   - 配置文件：持久化配置（中继、NAT、连接限制、引导节点等）
func buildOptions() ([]dep2p.Option, error) {
	var opts []dep2p.Option
	var cfg *config.Config
	runtime := &runtimeConfig{}

	// ═══════════════════════════════════════════════════════════════════
	// 1. 加载配置文件（持久化配置）
	// ═══════════════════════════════════════════════════════════════════
	if *configFile != "" {
		var err error
		cfg, err = loadConfigFile(*configFile)
		if err != nil {
			return nil, fmt.Errorf("加载配置文件失败: %w", err)
		}
	} else {
		cfg = config.NewConfig()
	}

	// ═══════════════════════════════════════════════════════════════════
	// 2. 应用环境变量覆盖
	// ═══════════════════════════════════════════════════════════════════
	applyEnvOverrides(cfg, runtime)

	// ═══════════════════════════════════════════════════════════════════
	// 3. 应用命令行参数覆盖（运行时参数，最高优先级）
	// ═══════════════════════════════════════════════════════════════════

	// 预设（命令行 > 环境变量 > 配置文件）
	presetName := *preset
	if runtime.preset != "" && !isFlagSet("preset") {
		presetName = runtime.preset
	}
	if dep2p.IsValidPreset(presetName) {
		opts = append(opts, dep2p.WithPreset(presetName))
	}

	// 监听端口（运行时快速指定）
	if isFlagSet("port") {
		opts = append(opts, dep2p.WithListenPort(*port))
	} else if runtime.listenPort > 0 {
		opts = append(opts, dep2p.WithListenPort(runtime.listenPort))
	}

	// 身份密钥文件（运行时指定）
	if isFlagSet("identity") && *identityFile != "" {
		opts = append(opts, dep2p.WithIdentityFromFile(*identityFile))
	} else if cfg.Identity.KeyFile != "" {
		opts = append(opts, dep2p.WithIdentityFromFile(cfg.Identity.KeyFile))
	}

	// 数据目录（命令行 > 环境变量 > 配置文件 > 默认值）
	if isFlagSet("data-dir") && *dataDir != "" {
		opts = append(opts, dep2p.WithDataDir(*dataDir))
	} else if runtime.dataDir != "" {
		opts = append(opts, dep2p.WithDataDir(runtime.dataDir))
	} else if cfg.Storage.DataDir != "" {
		opts = append(opts, dep2p.WithDataDir(cfg.Storage.DataDir))
	}

	// ═══════════════════════════════════════════════════════════════════
	// 以下配置从配置文件读取（不再支持命令行参数直接设置）
	// ═══════════════════════════════════════════════════════════════════

	// 引导节点（来自配置文件）
	if len(cfg.Discovery.Bootstrap.Peers) > 0 {
		opts = append(opts, dep2p.WithBootstrapPeers(cfg.Discovery.Bootstrap.Peers...))
	}

	// 中继配置（来自配置文件）
	opts = append(opts, dep2p.WithRelay(cfg.Relay.EnableClient))

	// Relay 地址（来自配置文件）
	if cfg.Relay.RelayAddr != "" {
		opts = append(opts, dep2p.WithRelayAddr(cfg.Relay.RelayAddr))
	}

	// NAT 配置（来自配置文件）
	opts = append(opts, dep2p.WithNAT(cfg.NAT.EnableAutoNAT))

	// 连接限制（来自配置文件）
	if cfg.ConnMgr.LowWater > 0 || cfg.ConnMgr.HighWater > 0 {
		opts = append(opts, dep2p.WithConnectionLimits(cfg.ConnMgr.LowWater, cfg.ConnMgr.HighWater))
	}

	// ═══════════════════════════════════════════════════════════════════
	// 日志文件（命令行 > 环境变量）
	// ═══════════════════════════════════════════════════════════════════
	logPath := *logFile
	if logPath == "" {
		logPath = getLogFileFromEnv()
	}
	if logPath != "" {
		opts = append(opts, dep2p.WithLogFile(logPath))
	}

	// ═══════════════════════════════════════════════════════════════════
	// 应用完整配置（必须在能力开关之前，这样能力开关可以覆盖配置文件的值）
	// ═══════════════════════════════════════════════════════════════════
	opts = append(opts, dep2p.WithConfig(cfg))

	// ═══════════════════════════════════════════════════════════════════
	// 能力开关（ADR-0009 / ADR-0010）
	// 说明：能力开关是运行时参数，表示「这次运行」是否提供服务
	// 注意：能力开关必须在 WithConfig 之后，以便覆盖配置文件中的默认值
	// ═══════════════════════════════════════════════════════════════════

	// 基础设施快捷方式（同时启用 Bootstrap + Relay）
	if isFlagSet("enable-infra") && *enableInfra {
		opts = append(opts, dep2p.EnableInfrastructure(true))
	} else {
		// 单独启用 Bootstrap 能力
		if isFlagSet("enable-bootstrap") && *enableBootstrap {
			opts = append(opts, dep2p.EnableBootstrap(true))
		} else if cfg.Discovery.Bootstrap.EnableService {
			opts = append(opts, dep2p.EnableBootstrap(true))
		}

		// 单独启用 Relay 能力
		if isFlagSet("enable-relay") && *enableRelay {
			opts = append(opts, dep2p.EnableRelayServer(true))
		} else if cfg.Relay.EnableServer {
			opts = append(opts, dep2p.EnableRelayServer(true))
		}
	}

	// 公网地址（基础设施节点必需，运行时参数）
	pubAddr := *publicAddr
	if pubAddr == "" {
		pubAddr = runtime.publicAddr
	}
	if pubAddr != "" {
		opts = append(opts, dep2p.WithPublicAddr(pubAddr))
	}

	return opts, nil
}

// isFlagSet 检查命令行参数是否被显式设置
func isFlagSet(name string) bool {
	found := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == name {
			found = true
		}
	})
	return found
}

// waitForSignal 等待退出信号
func waitForSignal() {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	<-signals
}

// setupLogging 设置日志输出
//
// 根据配置自动创建日志文件，返回实际使用的日志路径。
// 如果禁用自动日志且未指定日志文件，返回空字符串（日志输出到 stderr）。
//
// 日志文件命名规则：
//   - Bootstrap 节点: bootstrap-{timestamp}-{pid}.log
//   - Relay 节点: relay-{timestamp}-{pid}.log
//   - Bootstrap+Relay: infra-{timestamp}-{pid}.log
//   - 普通节点: dep2p-{timestamp}-{pid}.log
func setupLogging() (string, *os.File, error) {
	// 如果禁用自动日志且未指定日志文件，则不使用文件日志
	if !*autoLog && *logFile == "" {
		return "", nil, nil
	}

	logPath := *logFile
	if logPath == "" {
		// 根据节点功能自动生成日志文件名
		prefix := determineLogPrefix()
		timestamp := time.Now().Format("20060102-150405")
		logPath = filepath.Join(*logDir, fmt.Sprintf("%s-%s-%d.log", prefix, timestamp, os.Getpid()))
	}

	// 确保日志目录存在
	if err := os.MkdirAll(filepath.Dir(logPath), 0750); err != nil {
		return "", nil, fmt.Errorf("创建日志目录失败: %w", err)
	}

	// 打开日志文件
	file, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return "", nil, fmt.Errorf("打开日志文件失败: %w", err)
	}

	// 设置全局日志输出
	log.SetOutput(file)

	return logPath, file, nil
}

// determineLogPrefix 根据节点能力确定日志文件前缀
//
// 优先级：
//   1. 同时启用 Bootstrap 和 Relay → "infra"（基础设施）
//   2. 仅启用 Bootstrap → "bootstrap"（引导节点）
//   3. 仅启用 Relay → "relay"（中继节点）
//   4. 普通节点 → "dep2p"
func determineLogPrefix() string {
	isBootstrap := false
	isRelay := false

	// 检查能力开关
	if isFlagSet("enable-infra") && *enableInfra {
		return "infra"
	}

	if isFlagSet("enable-bootstrap") && *enableBootstrap {
		isBootstrap = true
	}

	if isFlagSet("enable-relay") && *enableRelay {
		isRelay = true
	}

	// 检查配置文件中的能力设置（如果有）
	if !isBootstrap || !isRelay {
		if cfg, err := loadConfigIfNeeded(); err == nil && cfg != nil {
			if !isBootstrap && cfg.Discovery.Bootstrap.EnableService {
				isBootstrap = true
			}
			if !isRelay && cfg.Relay.EnableServer {
				isRelay = true
			}
		}
	}

	// 根据能力组合确定前缀
	if isBootstrap && isRelay {
		return "infra"
	}
	if isBootstrap {
		return "bootstrap"
	}
	if isRelay {
		return "relay"
	}

	return "dep2p"
}

// loadConfigIfNeeded 仅在需要时加载配置文件（用于确定日志前缀）
func loadConfigIfNeeded() (*config.Config, error) {
	if *configFile == "" {
		return nil, nil
	}
	return loadConfigFile(*configFile)
}

// printNodeInfo 打印节点信息（美化输出）
//
// 输出包含可复制的完整地址，便于分享给其他设备连接。
func printNodeInfo(endpoint dep2p.Endpoint) {
	id := endpoint.ID()
	addrs := selectDisplayAddrs(endpoint)

	fmt.Println()
	fmt.Println("╔════════════════════════════════════════════════════════════════════════╗")
	fmt.Printf("║                    DeP2P Node Started (%s)                        ║\n", dep2p.Version)
	fmt.Println("╠════════════════════════════════════════════════════════════════════════╣")
	fmt.Printf("║  Node ID: %-60s  ║\n", id)
	fmt.Println("║                                                                        ║")

	// 显示能力状态
	capabilities := buildCapabilityString(endpoint)
	if capabilities != "" {
		fmt.Printf("║  Capabilities: %-56s  ║\n", capabilities)
		fmt.Println("║                                                                        ║")
	}

	fmt.Println("║  Addresses (copy to share):                                            ║")

	// 输出完整地址（含 /p2p/NodeID），不截断，便于复制
	for _, addr := range addrs {
		fullAddr := addr
		if !strings.Contains(fullAddr, "/p2p/") {
			fullAddr = fmt.Sprintf("%s/p2p/%s", addr, id)
		}
		printWrappedLine(fullAddr, 68)
	}

	fmt.Println("║                                                                        ║")

	// 显示日志文件路径
	if actualLogPath != "" {
		printWrappedLabel("Log file:", actualLogPath, 60)
		fmt.Println("║                                                                        ║")
	}

	fmt.Println("╚════════════════════════════════════════════════════════════════════════╝")
	fmt.Println()
}

// selectDisplayAddrs 选择可展示的连接地址
func selectDisplayAddrs(endpoint dep2p.Endpoint) []string {
	// 1. 优先展示可分享地址（已过滤 0.0.0.0 等不可连接地址）
	if addrs := endpoint.ShareableAddrs(); len(addrs) > 0 {
		return addrs
	}

	// 2. 使用对外公告地址（过滤不可连接地址）
	if addrs := filterConnectableAddrs(endpoint.AdvertisedAddrs()); len(addrs) > 0 {
		return addrs
	}

	// 3. 兜底使用监听地址（过滤不可连接地址）
	return filterConnectableAddrs(endpoint.ListenAddrs())
}

func filterConnectableAddrs(addrs []string) []string {
	if len(addrs) == 0 {
		return nil
	}
	result := make([]string, 0, len(addrs))
	for _, addr := range addrs {
		if isConnectableAddr(addr) {
			result = append(result, addr)
		}
	}
	return result
}

func isConnectableAddr(addr string) bool {
	if addr == "" {
		return false
	}

	unconnectablePatterns := []string{
		"/ip4/0.0.0.0/",
		"/ip6/::/",
		"/ip4/127.0.0.1/",
		"/ip4/127.",
		"/ip6/::1/",
	}

	for _, pattern := range unconnectablePatterns {
		if strings.Contains(addr, pattern) {
			return false
		}
	}

	return true
}

// buildCapabilityString 构建能力状态字符串
func buildCapabilityString(endpoint dep2p.Endpoint) string {
	var caps []string

	// 检查 Bootstrap 能力
	if endpoint.IsBootstrapEnabled() {
		caps = append(caps, "Bootstrap")
	}

	// 检查 Relay 能力
	if endpoint.IsRelayEnabled() {
		caps = append(caps, "Relay")
	}

	if len(caps) == 0 {
		return ""
	}

	return strings.Join(caps, ", ")
}

// printWrappedLine 打印可复制的长行内容（不截断）
func printWrappedLine(text string, width int) {
	if width <= 0 {
		fmt.Printf("║    %s  ║\n", text)
		return
	}
	for len(text) > width {
		fmt.Printf("║    %-*s  ║\n", width, text[:width])
		text = text[width:]
	}
	fmt.Printf("║    %-*s  ║\n", width, text)
}

// printWrappedLabel 打印带标签的长行内容（不截断）
func printWrappedLabel(label, text string, width int) {
	prefix := fmt.Sprintf("║  %s ", label)
	if width <= 0 {
		fmt.Printf("%s%s  ║\n", prefix, text)
		return
	}
	remaining := width
	linePrefix := prefix
	for len(text) > remaining {
		fmt.Printf("%s%-*s  ║\n", linePrefix, remaining, text[:remaining])
		text = text[remaining:]
		// 续行对齐
		linePrefix = "║" + strings.Repeat(" ", len(label)+2) + " "
		remaining = width
	}
	fmt.Printf("%s%-*s  ║\n", linePrefix, remaining, text)
}

// printVersion 打印版本信息
func printVersion() {
	fmt.Printf("dep2p %s\n", dep2p.Version)
	if dep2p.GitCommit != "" {
		fmt.Printf("  commit: %s\n", dep2p.GitCommit)
	}
	if dep2p.BuildDate != "" {
		fmt.Printf("  built:  %s\n", dep2p.BuildDate)
	}
}

// printHelp 打印帮助信息
func printHelp() {
	fmt.Println("dep2p - 简洁可靠的 P2P 网络库")
	fmt.Println()
	fmt.Println("用法:")
	fmt.Println("  dep2p [选项]")
	fmt.Println()
	fmt.Println("选项:")
	flag.PrintDefaults()
	fmt.Println()
	fmt.Println("═══════════════════════════════════════════════════════════════════════════")
	fmt.Println("配置边界说明")
	fmt.Println("═══════════════════════════════════════════════════════════════════════════")
	fmt.Println()
	fmt.Println("命令行参数（运行时覆盖）：")
	fmt.Println("  -port, -preset, -config, -identity, -public-addr   # 运行时参数")
	fmt.Println("  -data-dir                                          # 数据目录")
	fmt.Println("  -enable-bootstrap, -enable-relay, -enable-infra  # 能力开关")
	fmt.Println("  -log, -log-dir, -auto-log                          # 日志参数")
	fmt.Println()
	fmt.Println("配置文件（持久化配置）：")
	fmt.Println("  relay.enable_client      # 是否启用中继客户端")
	fmt.Println("  relay.relay_addr         # Relay 地址")
	fmt.Println("  nat.enable_auto_nat      # 是否启用 NAT 穿透")
	fmt.Println("  conn_mgr.low_water       # 连接低水位")
	fmt.Println("  conn_mgr.high_water      # 连接高水位")
	fmt.Println("  discovery.bootstrap.peers # 引导节点列表")
	fmt.Println()
	fmt.Println("环境变量:")
	fmt.Println("  DEP2P_PRESET              预设名称")
	fmt.Println("  DEP2P_LISTEN_PORT         监听端口")
	fmt.Println("  DEP2P_IDENTITY_KEY_FILE   身份密钥文件")
	fmt.Println("  DEP2P_DATA_DIR            数据目录（隔离多节点数据库）")
	fmt.Println("  DEP2P_ENABLE_BOOTSTRAP    启用 Bootstrap 服务能力 (true/false)")
	fmt.Println("  DEP2P_ENABLE_RELAY        启用 Relay 服务能力 (true/false)")
	fmt.Println("  DEP2P_PUBLIC_ADDR         公网可达地址（服务端必需）")
	fmt.Println("  DEP2P_LOG_FILE            日志文件路径")
	fmt.Println()
	fmt.Println("═══════════════════════════════════════════════════════════════════════════")
	fmt.Println("预设配置")
	fmt.Println("═══════════════════════════════════════════════════════════════════════════")
	fmt.Println()
	fmt.Println("  mobile    - 移动端优化，低资源占用")
	fmt.Println("  desktop   - 桌面端默认配置")
	fmt.Println("  server    - 服务器优化，高性能")
	fmt.Println("  minimal   - 最小配置，仅用于测试")
	fmt.Println()
	fmt.Println("═══════════════════════════════════════════════════════════════════════════")
	fmt.Println("使用示例")
	fmt.Println("═══════════════════════════════════════════════════════════════════════════")
	fmt.Println()
	fmt.Println("普通节点（用户使用）:")
	fmt.Println()
	fmt.Println("  # 使用默认配置启动")
	fmt.Println("  dep2p")
	fmt.Println()
	fmt.Println("  # 指定端口（开发测试）")
	fmt.Println("  dep2p -port 9000")
	fmt.Println()
	fmt.Println("  # 使用配置文件（推荐用于生产环境）")
	fmt.Println("  dep2p -config config.json")
	fmt.Println()
	fmt.Println("  # 服务器模式 + 指定端口")
	fmt.Println("  dep2p -preset server -port 4001")
	fmt.Println()
	fmt.Println("  # 禁用自动日志文件（输出到控制台）")
	fmt.Println("  dep2p -auto-log=false")
	fmt.Println()
	fmt.Println("基础设施节点（项目方部署）:")
	fmt.Println()
	fmt.Println("  # 启用全部基础设施能力（Bootstrap + Relay）")
	fmt.Println("  dep2p -enable-infra -port 4001 -public-addr /ip4/YOUR_PUBLIC_IP/udp/4001/quic-v1")
	fmt.Println()
	fmt.Println("  # 仅启用 Bootstrap 服务")
	fmt.Println("  dep2p -enable-bootstrap -port 4001 -public-addr /ip4/YOUR_PUBLIC_IP/udp/4001/quic-v1")
	fmt.Println()
	fmt.Println("  # 仅启用 Relay 服务")
	fmt.Println("  dep2p -enable-relay -port 4001 -public-addr /ip4/YOUR_PUBLIC_IP/udp/4001/quic-v1")
	fmt.Println()
	fmt.Println("  # 使用配置文件 + 公网地址")
	fmt.Println("  dep2p -config infra.json -public-addr /ip4/YOUR_PUBLIC_IP/udp/4001/quic-v1")
	fmt.Println()
	fmt.Println("  # 使用环境变量")
	fmt.Println("  DEP2P_ENABLE_BOOTSTRAP=true DEP2P_PUBLIC_ADDR=/ip4/... dep2p")
	fmt.Println()
	fmt.Println("同机多节点部署（需使用不同数据目录）:")
	fmt.Println()
	fmt.Println("  # Bootstrap 节点")
	fmt.Println("  dep2p -config deploy/bootstrap/bootstrap.config.json -port 4001 ...")
	fmt.Println()
	fmt.Println("  # Relay 节点（使用不同端口和数据目录）")
	fmt.Println("  dep2p -config deploy/relay/relay.config.json -port 4002 ...")
	fmt.Println()
	fmt.Println("  # 或使用命令行参数指定数据目录")
	fmt.Println("  dep2p -data-dir ./data/node1 -port 4001 -enable-bootstrap ...")
	fmt.Println("  dep2p -data-dir ./data/node2 -port 4002 -enable-relay ...")
	fmt.Println()
	fmt.Println("═══════════════════════════════════════════════════════════════════════════")
	fmt.Println("配置文件示例 (config.json)")
	fmt.Println("═══════════════════════════════════════════════════════════════════════════")
	fmt.Println()
	fmt.Println(`  {`)
	fmt.Println(`    "identity": {`)
	fmt.Println(`      "key_file": "~/.dep2p/identity.key"`)
	fmt.Println(`    },`)
	fmt.Println(`    "relay": {`)
	fmt.Println(`      "enable_client": true,`)
	fmt.Println(`      "relay_addr": "/ip4/.../p2p/12D3KooW..."`)
	fmt.Println(`    },`)
	fmt.Println(`    "nat": {`)
	fmt.Println(`      "enable_auto_nat": true`)
	fmt.Println(`    },`)
	fmt.Println(`    "conn_mgr": {`)
	fmt.Println(`      "low_water": 50,`)
	fmt.Println(`      "high_water": 100`)
	fmt.Println(`    },`)
	fmt.Println(`    "discovery": {`)
	fmt.Println(`      "bootstrap": {`)
	fmt.Println(`        "peers": ["/ip4/.../p2p/12D3KooW..."]`)
	fmt.Println(`      }`)
	fmt.Println(`    }`)
	fmt.Println(`  }`)
	fmt.Println()
	fmt.Println("═══════════════════════════════════════════════════════════════════════════")
	fmt.Println("地址格式 (multiaddr)")
	fmt.Println("═══════════════════════════════════════════════════════════════════════════")
	fmt.Println()
	fmt.Println("  /ip4/<IP>/udp/<PORT>/quic-v1/p2p/<NodeID>   # QUIC (推荐)")
	fmt.Println("  /ip4/<IP>/tcp/<PORT>/p2p/<NodeID>           # TCP")
	fmt.Println("  /dnsaddr/<DOMAIN>/p2p/<NodeID>              # DNS")
}
