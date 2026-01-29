package main

import (
	"encoding/json"
	"os"
	"strconv"
	"strings"

	"github.com/dep2p/go-dep2p/config"
)

// ═══════════════════════════════════════════════════════════════════════════
//
//	环境变量常量
//
// ═══════════════════════════════════════════════════════════════════════════
//
// 配置边界原则（参见 design/_discussions/20260116-config-boundary.md）：
//
//   运行时环境变量：运行时覆盖（命令行参数的等效方式）
//     DEP2P_PRESET, DEP2P_LISTEN_PORT, DEP2P_IDENTITY_KEY_FILE
//     DEP2P_ENABLE_BOOTSTRAP, DEP2P_ENABLE_SYSTEM_RELAY, DEP2P_PUBLIC_ADDR
//     DEP2P_LOG_FILE
//
//   配置覆盖环境变量：覆盖配置文件中的值（无对应命令行参数）
//     DEP2P_BOOTSTRAP_PEERS, DEP2P_SYSTEM_RELAY_ADDR
//     DEP2P_ENABLE_RELAY, DEP2P_ENABLE_NAT
//
// ═══════════════════════════════════════════════════════════════════════════

const (
	// envPrefix 环境变量前缀
	envPrefix = "DEP2P_"

	// ─────────────────────────────────────────────────────────────────────
	// 运行时环境变量（有对应命令行参数）
	// ─────────────────────────────────────────────────────────────────────
	envPreset          = "PRESET"
	envListenPort      = "LISTEN_PORT"
	envIdentityKeyFile = "IDENTITY_KEY_FILE"
	envLogFile         = "LOG_FILE"
	envDataDir         = "DATA_DIR" // 数据目录

	// 能力开关（ADR-0009 / v2.0 统一 Relay）
	envEnableBootstrap = "ENABLE_BOOTSTRAP"
	envEnableRelaySvc  = "ENABLE_RELAY_SERVER" // 启用 Relay 服务能力
	envPublicAddr      = "PUBLIC_ADDR"

	// ─────────────────────────────────────────────────────────────────────
	// 配置覆盖环境变量（无对应命令行参数，仅用于覆盖配置文件）
	// ─────────────────────────────────────────────────────────────────────
	envBootstrapPeers = "BOOTSTRAP_PEERS" // 覆盖 discovery.bootstrap.peers
	envRelayAddr      = "RELAY_ADDR"      // 覆盖 relay.relay_addr
	envEnableRelay    = "ENABLE_RELAY"    // 覆盖 relay.enable_client
	envEnableNAT      = "ENABLE_NAT"      // 覆盖 nat.enable_*
)

// ============================================================================
//
//	配置加载（CLI 专用）
//
// ============================================================================

// loadConfigFile 从 JSON 文件加载配置
func loadConfigFile(path string) (*config.Config, error) {
	data, err := os.ReadFile(path) //nolint:gosec // G304: 用户指定的配置文件路径是预期行为
	if err != nil {
		return nil, err
	}

	cfg := config.NewConfig()
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

// applyEnvOverrides 应用环境变量覆盖配置
//
// 配置优先级：命令行参数 > 环境变量 > 配置文件 > 预设默认值
//
// 环境变量分为两类：
//
//  1. 运行时环境变量（有对应命令行参数）：
//     - DEP2P_PRESET: 预设名称
//     - DEP2P_LISTEN_PORT: 监听端口
//     - DEP2P_IDENTITY_KEY_FILE: 身份密钥文件
//     - DEP2P_ENABLE_BOOTSTRAP: 启用 Bootstrap 服务能力
//     - DEP2P_ENABLE_RELAY_SERVER: 启用 Relay 服务能力（v2.0 统一）
//     - DEP2P_PUBLIC_ADDR: 公网可达地址
//     - DEP2P_LOG_FILE: 日志文件路径
//
//  2. 配置覆盖环境变量（无对应命令行参数，仅用于覆盖配置文件）：
//     - DEP2P_BOOTSTRAP_PEERS: 引导节点列表（逗号分隔）
//     - DEP2P_RELAY_ADDR: Relay 地址（v2.0 统一）
//     - DEP2P_ENABLE_RELAY: 启用中继客户端
//     - DEP2P_ENABLE_NAT: 启用 NAT 穿透
func applyEnvOverrides(cfg *config.Config, runtime *runtimeConfig) {
	// DEP2P_PRESET
	if v := os.Getenv(envPrefix + envPreset); v != "" {
		runtime.preset = v
		_ = config.ApplyPreset(cfg, v)
	}

	// DEP2P_LISTEN_PORT
	if v := os.Getenv(envPrefix + envListenPort); v != "" {
		if port, err := strconv.Atoi(v); err == nil {
			runtime.listenPort = port
		}
	}

	// DEP2P_IDENTITY_KEY_FILE
	if v := os.Getenv(envPrefix + envIdentityKeyFile); v != "" {
		cfg.Identity.KeyFile = v
	}

	// DEP2P_DATA_DIR
	if v := os.Getenv(envPrefix + envDataDir); v != "" {
		cfg.Storage.DataDir = v
		runtime.dataDir = v
	}

	// DEP2P_BOOTSTRAP_PEERS (逗号分隔)
	if v := os.Getenv(envPrefix + envBootstrapPeers); v != "" {
		cfg.Discovery.Bootstrap.Peers = splitAndTrim(v, ",")
	}

	// DEP2P_RELAY_ADDR（v2.0 统一 Relay）
	if v := os.Getenv(envPrefix + envRelayAddr); v != "" {
		cfg.Relay.RelayAddr = v
	}

	// DEP2P_ENABLE_RELAY
	if v := os.Getenv(envPrefix + envEnableRelay); v != "" {
		cfg.Relay.EnableClient = parseBool(v)
	}

	// DEP2P_ENABLE_NAT
	if v := os.Getenv(envPrefix + envEnableNAT); v != "" {
		enable := parseBool(v)
		cfg.NAT.EnableAutoNAT = enable
		cfg.NAT.EnableUPnP = enable
		cfg.NAT.EnableNATPMP = enable
		cfg.NAT.EnableHolePunch = enable
	}

	// ═══════════════════════════════════════════════════════════════════════
	// 能力开关环境变量（ADR-0009 / ADR-0010）
	// ═══════════════════════════════════════════════════════════════════════

	// DEP2P_ENABLE_BOOTSTRAP
	if v := os.Getenv(envPrefix + envEnableBootstrap); v != "" {
		cfg.Discovery.Bootstrap.EnableService = parseBool(v)
	}

	// DEP2P_ENABLE_RELAY_SERVER（v2.0 统一 Relay）
	if v := os.Getenv(envPrefix + envEnableRelaySvc); v != "" {
		cfg.Relay.EnableServer = parseBool(v)
	}

	// DEP2P_PUBLIC_ADDR（存储到 runtime，由 main.go 处理）
	if v := os.Getenv(envPrefix + envPublicAddr); v != "" {
		runtime.publicAddr = v
	}
}

// getLogFileFromEnv 从环境变量获取日志文件路径
func getLogFileFromEnv() string {
	return os.Getenv(envPrefix + envLogFile)
}

// ============================================================================
//
//	辅助函数
//
// ============================================================================

// parseBool 解析布尔值字符串
func parseBool(s string) bool {
	s = strings.ToLower(strings.TrimSpace(s))
	return s == "true" || s == "1" || s == "yes" || s == "on"
}

// splitAndTrim 分割字符串并去除空白
func splitAndTrim(s, sep string) []string {
	parts := strings.Split(s, sep)
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}
