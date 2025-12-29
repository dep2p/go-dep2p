package main

import (
	"encoding/json"
	"os"
	"strconv"
	"strings"

	"github.com/dep2p/go-dep2p"
	"github.com/dep2p/go-dep2p/internal/config"
)

// ============================================================================
//                              配置加载（CLI 专用）
// ============================================================================

// loadConfigFile 从 JSON 文件加载配置
func loadConfigFile(path string) (*dep2p.UserConfig, error) {
	data, err := os.ReadFile(path) //nolint:gosec // G304: 用户指定的配置文件路径是预期行为
	if err != nil {
		return nil, err
	}

	var cfg dep2p.UserConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// applyEnvOverrides 应用环境变量覆盖配置
//
// 环境变量优先级高于配置文件，但低于命令行参数。
// 支持的环境变量（均使用 DEP2P_ 前缀）：
//   - DEP2P_PRESET: 预设名称
//   - DEP2P_LISTEN_PORT: 监听端口
//   - DEP2P_IDENTITY_KEY_FILE: 身份密钥文件
//   - DEP2P_BOOTSTRAP_PEERS: 引导节点（逗号分隔）
//   - DEP2P_ENABLE_RELAY: 启用中继
//   - DEP2P_ENABLE_NAT: 启用 NAT 穿透
//   - DEP2P_LOG_FILE: 日志文件路径
func applyEnvOverrides(cfg *dep2p.UserConfig) {
	// DEP2P_PRESET
	if v := os.Getenv(config.EnvPrefix + config.EnvPreset); v != "" {
		cfg.Preset = v
	}

	// DEP2P_LISTEN_PORT
	if v := os.Getenv(config.EnvPrefix + config.EnvListenPort); v != "" {
		if port, err := strconv.Atoi(v); err == nil {
			cfg.ListenPort = port
		}
	}

	// DEP2P_IDENTITY_KEY_FILE
	if v := os.Getenv(config.EnvPrefix + config.EnvIdentityKeyFile); v != "" {
		if cfg.Identity == nil {
			cfg.Identity = &dep2p.IdentityConfig{}
		}
		cfg.Identity.KeyFile = v
	}

	// DEP2P_BOOTSTRAP_PEERS (逗号分隔)
	if v := os.Getenv(config.EnvPrefix + config.EnvBootstrapPeers); v != "" {
		if cfg.Discovery == nil {
			cfg.Discovery = &dep2p.DiscoveryConfig{}
		}
		cfg.Discovery.BootstrapPeers = splitAndTrim(v, ",")
	}

	// DEP2P_ENABLE_RELAY
	if v := os.Getenv(config.EnvPrefix + config.EnvEnableRelay); v != "" {
		if cfg.Relay == nil {
			cfg.Relay = &dep2p.RelayConfig{}
		}
		cfg.Relay.Enable = parseBool(v)
	}

	// DEP2P_ENABLE_NAT
	if v := os.Getenv(config.EnvPrefix + config.EnvEnableNAT); v != "" {
		if cfg.NAT == nil {
			cfg.NAT = &dep2p.NATConfig{}
		}
		cfg.NAT.Enable = parseBool(v)
	}
}

// getLogFileFromEnv 从环境变量获取日志文件路径
func getLogFileFromEnv() string {
	return os.Getenv(config.EnvPrefix + config.EnvLogFile)
}

// ============================================================================
//                              辅助函数
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

