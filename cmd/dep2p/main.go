// Package main 提供 dep2p 命令行入口
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/dep2p/go-dep2p"
)

var (
	// 版本信息（通过 ldflags 注入）
	version   = "dev"
	commit    = "unknown"
	buildDate = "unknown"
)

// 命令行参数
var (
	port           = flag.Int("port", 0, "监听端口（0 表示随机端口，默认使用 QUIC）")
	identityFile   = flag.String("identity", "", "身份密钥文件路径")
	configFile     = flag.String("config", "", "配置文件路径")
	preset         = flag.String("preset", "desktop", "预设配置 (mobile/desktop/server/minimal)")
	enableRelay    = flag.Bool("relay", true, "启用中继")
	enableNAT      = flag.Bool("nat", true, "启用 NAT 穿透")
	lowWater       = flag.Int("low-water", 50, "连接低水位线")
	highWater      = flag.Int("high-water", 100, "连接高水位线")
	bootstrapPeers = flag.String("bootstrap", "", "引导节点（逗号分隔的完整地址）")
	logFile        = flag.String("log", "", "日志文件路径")
	showVersion    = flag.Bool("version", false, "显示版本信息")
	showHelp       = flag.Bool("help", false, "显示帮助信息")
)

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

	// 构建选项
	opts, err := buildOptions()
	if err != nil {
		return fmt.Errorf("配置错误: %w", err)
	}

	// 创建上下文
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 启动节点
	fmt.Println("正在启动 dep2p 节点...")
	endpoint, err := dep2p.Start(ctx, opts...)
	if err != nil {
		return fmt.Errorf("启动失败: %w", err)
	}
	defer func() { _ = endpoint.Close() }()

	// 显示节点信息
	fmt.Printf("节点 ID: %s\n", endpoint.ID())
	fmt.Println("监听地址:")
	for _, addr := range endpoint.ListenAddrs() {
		fmt.Printf("  %s\n", addr)
	}

	// 等待退出信号
	fmt.Println("\n节点已启动，按 Ctrl+C 退出")
	waitForSignal()

	fmt.Println("\n正在关闭节点...")
	return nil
}

// buildOptions 构建选项
//
// 配置优先级（从高到低）：
//  1. 命令行参数
//  2. 环境变量（DEP2P_* 前缀）
//  3. 配置文件
//  4. 预设默认值
func buildOptions() ([]dep2p.Option, error) {
	var opts []dep2p.Option
	var cfg *dep2p.UserConfig

	// 1. 加载配置文件（如果指定）
	if *configFile != "" {
		var err error
		cfg, err = loadConfigFile(*configFile)
		if err != nil {
			return nil, fmt.Errorf("加载配置文件失败: %w", err)
		}
	} else {
		cfg = &dep2p.UserConfig{}
	}

	// 2. 应用环境变量覆盖
	applyEnvOverrides(cfg)

	// 3. 应用命令行参数覆盖（最高优先级）

	// 预设（命令行 > 环境变量 > 配置文件）
	presetName := *preset
	if cfg.Preset != "" && !isFlagSet("preset") {
		presetName = cfg.Preset
	}
	if p := dep2p.GetPresetByName(presetName); p != nil {
		opts = append(opts, dep2p.WithPreset(p))
	}

	// 监听端口
	if isFlagSet("port") {
		opts = append(opts, dep2p.WithListenPort(*port))
	} else if cfg.ListenPort > 0 {
		opts = append(opts, dep2p.WithListenPort(cfg.ListenPort))
	}

	// 身份密钥文件
	if isFlagSet("identity") && *identityFile != "" {
		opts = append(opts, dep2p.WithIdentityFromFile(*identityFile))
	} else if cfg.Identity != nil && cfg.Identity.KeyFile != "" {
		opts = append(opts, dep2p.WithIdentityFromFile(cfg.Identity.KeyFile))
	}

	// 引导节点（正确处理逗号分隔）
	if isFlagSet("bootstrap") && *bootstrapPeers != "" {
		peers := splitAndTrim(*bootstrapPeers, ",")
		if len(peers) > 0 {
			opts = append(opts, dep2p.WithBootstrapPeers(peers...))
		}
	} else if cfg.Discovery != nil && len(cfg.Discovery.BootstrapPeers) > 0 {
		opts = append(opts, dep2p.WithBootstrapPeers(cfg.Discovery.BootstrapPeers...))
	}

	// 中继
	if isFlagSet("relay") {
		opts = append(opts, dep2p.WithRelay(*enableRelay))
	} else if cfg.Relay != nil {
		opts = append(opts, dep2p.WithRelay(cfg.Relay.Enable))
	} else {
		opts = append(opts, dep2p.WithRelay(true)) // 默认启用
	}

	// NAT
	if isFlagSet("nat") {
		opts = append(opts, dep2p.WithNAT(*enableNAT))
	} else if cfg.NAT != nil {
		opts = append(opts, dep2p.WithNAT(cfg.NAT.Enable))
	} else {
		opts = append(opts, dep2p.WithNAT(true)) // 默认启用
	}

	// 连接限制
	low, high := *lowWater, *highWater
	if cfg.ConnectionLimits != nil {
		if !isFlagSet("low-water") && cfg.ConnectionLimits.Low > 0 {
			low = cfg.ConnectionLimits.Low
		}
		if !isFlagSet("high-water") && cfg.ConnectionLimits.High > 0 {
			high = cfg.ConnectionLimits.High
		}
	}
	if low > 0 || high > 0 {
		opts = append(opts, dep2p.WithConnectionLimits(low, high))
	}

	// 日志文件（命令行 > 环境变量）
	logPath := *logFile
	if logPath == "" {
		logPath = getLogFileFromEnv()
	}
	if logPath != "" {
		opts = append(opts, dep2p.WithLogFile(logPath))
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

// printVersion 打印版本信息
func printVersion() {
	fmt.Printf("dep2p %s\n", version)
	fmt.Printf("  commit: %s\n", commit)
	fmt.Printf("  built:  %s\n", buildDate)
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
	fmt.Println("环境变量:")
	fmt.Println("  DEP2P_PRESET           预设名称")
	fmt.Println("  DEP2P_LISTEN_PORT      监听端口")
	fmt.Println("  DEP2P_IDENTITY_KEY_FILE 身份密钥文件")
	fmt.Println("  DEP2P_BOOTSTRAP_PEERS  引导节点（逗号分隔）")
	fmt.Println("  DEP2P_ENABLE_RELAY     启用中继 (true/false)")
	fmt.Println("  DEP2P_ENABLE_NAT       启用 NAT 穿透 (true/false)")
	fmt.Println("  DEP2P_LOG_FILE         日志文件路径")
	fmt.Println()
	fmt.Println("预设配置:")
	fmt.Println("  mobile    - 移动端优化，低资源占用")
	fmt.Println("  desktop   - 桌面端默认配置")
	fmt.Println("  server    - 服务器优化，高性能")
	fmt.Println("  minimal   - 最小配置，仅用于测试")
	fmt.Println()
	fmt.Println("示例:")
	fmt.Println("  # 使用默认配置启动")
	fmt.Println("  dep2p")
	fmt.Println()
	fmt.Println("  # 指定端口")
	fmt.Println("  dep2p -port 4002")
	fmt.Println()
	fmt.Println("  # 使用配置文件")
	fmt.Println("  dep2p -config config.json")
	fmt.Println()
	fmt.Println("  # 服务器模式")
	fmt.Println("  dep2p -preset server -port 4001")
	fmt.Println()
	fmt.Println("  # 使用环境变量")
	fmt.Println("  DEP2P_PRESET=server DEP2P_LISTEN_PORT=4001 dep2p")
}
