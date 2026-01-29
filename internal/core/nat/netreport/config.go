// Package netreport 提供网络诊断功能
package netreport

import "time"

// Config 网络诊断配置
type Config struct {
	// STUNServers STUN 服务器列表
	STUNServers []string

	// RelayServers 中继服务器列表
	RelayServers []string

	// Timeout 诊断超时时间
	Timeout time.Duration

	// ProbeTimeout 单个探测超时时间
	ProbeTimeout time.Duration

	// EnableIPv4 启用 IPv4 探测
	EnableIPv4 bool

	// EnableIPv6 启用 IPv6 探测
	EnableIPv6 bool

	// EnableRelayProbe 启用中继延迟探测
	EnableRelayProbe bool

	// EnablePortMapProbe 启用端口映射协议探测
	EnablePortMapProbe bool

	// EnableCaptivePortalProbe 启用强制门户检测
	EnableCaptivePortalProbe bool

	// MaxConcurrentProbes 最大并发探测数
	MaxConcurrentProbes int

	// FullReportInterval 完整报告间隔
	FullReportInterval time.Duration
}

// DefaultConfig 返回默认配置
func DefaultConfig() Config {
	return Config{
		STUNServers: []string{
			// 国内优先（降低 4G 网络超时概率）
			"stun.qq.com:3478",
			"stun.miwifi.com:3478",
			"stun.syncthing.net:3478",

			// 备选：国际 STUN
			"stun.l.google.com:19302",
			"stun1.l.google.com:19302",
			"stun2.l.google.com:19302",
			"stun3.l.google.com:19302",
			"stun4.l.google.com:19302",
			"stun.cloudflare.com:3478",
			"stun.stunprotocol.org:3478",
		},
		RelayServers:             []string{},
		Timeout:                  30 * time.Second,
		ProbeTimeout:             5 * time.Second,
		EnableIPv4:               true,
		EnableIPv6:               true,
		EnableRelayProbe:         true,
		EnablePortMapProbe:       true,
		EnableCaptivePortalProbe: true,
		MaxConcurrentProbes:      10,
		FullReportInterval:       5 * time.Minute,
	}
}
