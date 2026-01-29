package dep2p

import (
	"github.com/dep2p/go-dep2p/config"
)

// ════════════════════════════════════════════════════════════════════════════
//                              预设配置常量
// ════════════════════════════════════════════════════════════════════════════

// 预设名称常量
const (
	// PresetNameMobile 移动端预设名称
	PresetNameMobile = "mobile"

	// PresetNameDesktop 桌面端预设名称
	PresetNameDesktop = "desktop"

	// PresetNameServer 服务器预设名称
	PresetNameServer = "server"

	// PresetNameMinimal 最小预设名称
	PresetNameMinimal = "minimal"
)

// ════════════════════════════════════════════════════════════════════════════
//                              预设配置获取
// ════════════════════════════════════════════════════════════════════════════

// GetMobileConfig 获取移动端配置
//
// 适用场景：移动应用、低配设备
// 特点：
//   - 低资源占用
//   - 较少的并发连接
//   - 启用所有 NAT 穿透技术
//   - 启用中继客户端
//   - 仅 QUIC 传输
//
// 示例：
//
//	cfg := dep2p.GetMobileConfig()
func GetMobileConfig() *config.Config {
	return config.NewMobileConfig()
}

// GetDesktopConfig 获取桌面端配置
//
// 适用场景：桌面应用、个人电脑
// 特点：
//   - 适中的资源占用
//   - 中等并发连接数
//   - 启用所有 NAT 穿透技术
//   - 启用中继客户端
//   - 启用 QUIC 和 TCP
//
// 示例：
//
//	cfg := dep2p.GetDesktopConfig()
func GetDesktopConfig() *config.Config {
	return config.NewConfig()
}

// GetServerConfig 获取服务器配置
//
// 适用场景：服务器部署、公网节点、引导节点
// 特点：
//   - 高资源配置
//   - 大量并发连接
//   - 启用中继服务端
//   - 启用 AutoNAT 服务端
//   - 启用所有传输协议
//
// 示例：
//
//	cfg := dep2p.GetServerConfig()
func GetServerConfig() *config.Config {
	return config.NewServerConfig()
}

// GetMinimalConfig 获取最小配置
//
// 适用场景：测试环境、最小化部署
// 特点：
//   - 最小资源占用
//   - 最少的功能启用
//   - 适合快速测试和开发
//
// 示例：
//
//	cfg := dep2p.GetMinimalConfig()
func GetMinimalConfig() *config.Config {
	return config.NewMinimalConfig()
}

// GetConfigByPreset 根据预设名称获取配置
//
// 支持的预设名称：
//   - "mobile"  - 移动端配置
//   - "desktop" - 桌面端配置（默认）
//   - "server"  - 服务器配置
//   - "minimal" - 最小配置
//
// 如果名称未知，返回桌面端配置（默认）。
//
// 示例：
//
//	cfg := dep2p.GetConfigByPreset("server")
func GetConfigByPreset(name string) *config.Config {
	switch name {
	case PresetNameMobile:
		return GetMobileConfig()
	case PresetNameDesktop:
		return GetDesktopConfig()
	case PresetNameServer:
		return GetServerConfig()
	case PresetNameMinimal:
		return GetMinimalConfig()
	default:
		// 默认返回桌面端配置
		return GetDesktopConfig()
	}
}

// GetDefaultConfig 获取默认配置
//
// 返回桌面端配置作为默认值。
// 等同于 GetDesktopConfig()。
//
// 示例：
//
//	cfg := dep2p.GetDefaultConfig()
func GetDefaultConfig() *config.Config {
	return GetDesktopConfig()
}

// ════════════════════════════════════════════════════════════════════════════
//                              预设应用
// ════════════════════════════════════════════════════════════════════════════

// ApplyPresetToConfig 将预设应用到现有配置
//
// 这会修改传入的配置，而不是创建新配置。
// 用于在已有配置基础上应用预设。
//
// 示例：
//
//	cfg := config.NewConfig()
//	dep2p.ApplyPresetToConfig(cfg, "server")
func ApplyPresetToConfig(cfg *config.Config, presetName string) error {
	return config.ApplyPreset(cfg, presetName)
}

// ════════════════════════════════════════════════════════════════════════════
//                              预设列表
// ════════════════════════════════════════════════════════════════════════════

// PresetInfo 预设信息
type PresetInfo struct {
	// Name 预设名称
	Name string

	// Description 预设描述
	Description string

	// UseCase 适用场景
	UseCase string
}

// AvailablePresets 返回所有可用预设的信息
//
// 示例：
//
//	for _, preset := range dep2p.AvailablePresets() {
//	    fmt.Printf("%s: %s\n", preset.Name, preset.Description)
//	}
func AvailablePresets() []PresetInfo {
	return []PresetInfo{
		{
			Name:        PresetNameMobile,
			Description: "移动端优化配置，低资源占用",
			UseCase:     "移动应用、低配设备、电池供电设备",
		},
		{
			Name:        PresetNameDesktop,
			Description: "桌面端默认配置，均衡的资源和功能",
			UseCase:     "桌面应用、个人电脑",
		},
		{
			Name:        PresetNameServer,
			Description: "服务器优化配置，高性能高并发",
			UseCase:     "服务器部署、公网节点、引导节点",
		},
		{
			Name:        PresetNameMinimal,
			Description: "最小配置，最少的资源和功能",
			UseCase:     "测试环境、开发调试、极度资源受限场景",
		},
	}
}

// IsValidPreset 检查预设名称是否有效
//
// 示例：
//
//	if dep2p.IsValidPreset("server") {
//	    cfg := dep2p.GetConfigByPreset("server")
//	}
func IsValidPreset(name string) bool {
	switch name {
	case PresetNameMobile, PresetNameDesktop, PresetNameServer, PresetNameMinimal:
		return true
	default:
		return false
	}
}
