// Package logger 提供统一的日志接口
//
// 支持通过环境变量配置日志级别：
//   - DEP2P_LOG_LEVEL: 设置日志级别，支持按子系统配置
//     格式: 子系统=级别,子系统=级别,默认级别
//     示例: discovery=debug,transport=warn,info
//   - DEP2P_LOG_FORMAT: 日志格式 (text 或 json)
package logger

import (
	"log/slog"
	"os"
	"strings"
	"sync"
)

// LogFormat 日志输出格式
type LogFormat int

const (
	// FormatText 文本格式（默认）
	FormatText LogFormat = iota
	// FormatJSON JSON 格式
	FormatJSON
)

// Config 日志配置
type Config struct {
	// DefaultLevel 默认日志级别
	DefaultLevel slog.Level

	// SubsystemLevels 各子系统的日志级别
	SubsystemLevels map[string]slog.Level

	// Format 输出格式
	Format LogFormat

	// AddSource 是否添加源码位置
	AddSource bool
}

// LevelForSubsystem 获取指定子系统的日志级别
func (c *Config) LevelForSubsystem(subsystem string) slog.Level {
	if level, ok := c.SubsystemLevels[subsystem]; ok {
		return level
	}
	return c.DefaultLevel
}

// configCache 缓存配置，避免重复解析
var (
	configCache *Config
	configOnce  sync.Once
)

// ConfigFromEnv 从环境变量解析配置
//
// 环境变量:
//   - DEP2P_LOG_LEVEL: 日志级别配置
//     格式: 子系统=级别,子系统=级别,默认级别
//     示例: discovery=debug,transport=warn,info
//   - DEP2P_LOG_FORMAT: text 或 json
//   - DEP2P_LOG_ADD_SOURCE: true 或 false
func ConfigFromEnv() *Config {
	configOnce.Do(func() {
		configCache = parseConfig()
	})
	return configCache
}

// parseConfig 解析环境变量配置
func parseConfig() *Config {
	cfg := &Config{
		DefaultLevel:    slog.LevelDebug, // 默认 Debug 级别
		SubsystemLevels: make(map[string]slog.Level),
		Format:          FormatText,
		AddSource:       true,
	}

	// 解析 DEP2P_LOG_LEVEL
	if levelStr := os.Getenv("DEP2P_LOG_LEVEL"); levelStr != "" {
		parseLevelConfig(cfg, levelStr)
	}

	// 解析 DEP2P_LOG_FORMAT
	if formatStr := os.Getenv("DEP2P_LOG_FORMAT"); formatStr != "" {
		switch strings.ToLower(formatStr) {
		case "json":
			cfg.Format = FormatJSON
		default:
			cfg.Format = FormatText
		}
	}

	// 解析 DEP2P_LOG_ADD_SOURCE
	if addSourceStr := os.Getenv("DEP2P_LOG_ADD_SOURCE"); addSourceStr != "" {
		cfg.AddSource = addSourceStr != "false" && addSourceStr != "0"
	}

	return cfg
}

// parseLevelConfig 解析日志级别配置字符串
// 格式: subsystem=level,subsystem=level,defaultLevel
// 示例: discovery=debug,transport=warn,info
func parseLevelConfig(cfg *Config, levelStr string) {
	parts := strings.Split(levelStr, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		if strings.Contains(part, "=") {
			// 子系统级别: subsystem=level
			kv := strings.SplitN(part, "=", 2)
			if len(kv) == 2 {
				subsystem := strings.TrimSpace(kv[0])
				levelName := strings.TrimSpace(kv[1])
				if level, ok := parseLevel(levelName); ok {
					cfg.SubsystemLevels[subsystem] = level
				}
			}
		} else {
			// 默认级别
			if level, ok := parseLevel(part); ok {
				cfg.DefaultLevel = level
			}
		}
	}
}

// parseLevel 解析日志级别名称
func parseLevel(name string) (slog.Level, bool) {
	switch strings.ToLower(name) {
	case "debug":
		return slog.LevelDebug, true
	case "info":
		return slog.LevelInfo, true
	case "warn", "warning":
		return slog.LevelWarn, true
	case "error":
		return slog.LevelError, true
	default:
		return slog.LevelInfo, false
	}
}

// ResetConfig 重置配置缓存（仅用于测试）
func ResetConfig() {
	configOnce = sync.Once{}
	configCache = nil
}

