package config

import (
	"errors"
	"time"
)

// NATConfig NAT 穿透配置
//
// 配置节点的 NAT 穿透策略：
//   - AutoNAT: 自动检测 NAT 类型
//   - UPnP: 通用即插即用端口映射
//   - NAT-PMP: NAT 端口映射协议
//   - Hole Punching: 打洞技术（DCUtR）
//   - STUN: STUN 服务器配置
type NATConfig struct {
	// EnableAutoNAT 是否启用 AutoNAT 检测
	EnableAutoNAT bool `json:"enable_autonat"`

	// EnableUPnP 是否启用 UPnP 端口映射
	EnableUPnP bool `json:"enable_upnp"`

	// 
	// LockReachabilityPublic 是否锁定可达性为 Public
	// 当节点作为基础设施节点（Bootstrap/Relay）运行且配置了公网地址时自动设为 true
	// 锁定后不会被 AutoNAT 或 NAT 类型检测降级为 Private
	LockReachabilityPublic bool `json:"lock_reachability_public,omitempty"`

	// EnableNATPMP 是否启用 NAT-PMP 端口映射
	EnableNATPMP bool `json:"enable_natpmp"`

	// EnableHolePunch 是否启用 Hole Punching
	EnableHolePunch bool `json:"enable_holepunch"`

	// AutoNAT 配置
	AutoNAT AutoNATConfig `json:"autonat,omitempty"`

	// UPnP 配置
	UPnP UPnPConfig `json:"upnp,omitempty"`

	// NAT-PMP 配置
	// 21: 新增 NAT-PMP 专用配置
	NATPMP NATPMPConfig `json:"natpmp,omitempty"`

	// Hole Punching 配置
	HolePunch HolePunchConfig `json:"holepunch,omitempty"`

	// STUN 服务器列表
	STUNServers []string `json:"stun_servers,omitempty"`
}

// AutoNATConfig AutoNAT 配置
type AutoNATConfig struct {
	// ProbeInterval 探测间隔
	ProbeInterval time.Duration

	// ProbeTimeout 探测超时
	ProbeTimeout time.Duration

	// ConfidenceThreshold 置信度阈值
	// 达到该值才改变可达性状态
	ConfidenceThreshold int

	// SuccessThreshold 成功探测次数阈值
	SuccessThreshold int

	// FailureThreshold 失败探测次数阈值
	FailureThreshold int

	// EnableServer 是否启用 AutoNAT 服务端
	// 为其他节点提供 NAT 检测服务
	EnableServer bool

	// ServerMaxProbes 服务端最大探测数
	ServerMaxProbes int

	// ServerProbeTimeout 服务端探测超时
	ServerProbeTimeout time.Duration
}

// UPnPConfig UPnP 配置
type UPnPConfig struct {
	// MappingDuration 端口映射租期
	MappingDuration time.Duration

	// RenewalInterval 续期检查间隔
	RenewalInterval time.Duration

	// CacheDuration 地址缓存时间
	CacheDuration time.Duration

	// Timeout UPnP 操作超时时间
	// 21: 添加可配置超时，避免长时间阻塞
	Timeout time.Duration
}

// NATPMPConfig NAT-PMP 配置
//
// 21: 新增 NAT-PMP 专用配置，支持超时设置
type NATPMPConfig struct {
	// Timeout NAT-PMP 操作超时时间（网关发现、外部地址获取、端口映射）
	// 默认: 5 秒
	Timeout time.Duration
}

// HolePunchConfig Hole Punching 配置
type HolePunchConfig struct {
	// MaxRetries 最大重试次数
	MaxRetries int

	// RetryDelay 重试延迟
	RetryDelay time.Duration

	// ConnectTimeout 连接超时
	ConnectTimeout time.Duration

	// SyncTimeout SYNC 消息超时
	SyncTimeout time.Duration
}

// DefaultNATConfig 返回默认 NAT 配置
func DefaultNATConfig() NATConfig {
	return NATConfig{
		// ════════════════════════════════════════════════════════════════════
		// NAT 穿透技术启用配置
		// ════════════════════════════════════════════════════════════════════
		EnableAutoNAT:   true, // 启用 AutoNAT：自动检测 NAT 类型和公网可达性
		EnableUPnP:      true, // 启用 UPnP：自动向路由器请求端口映射
		EnableNATPMP:    true, // 启用 NAT-PMP：Apple 路由器端口映射协议
		EnableHolePunch: true, // 启用 Hole Punching：通过协调服务器打洞直连

		// ════════════════════════════════════════════════════════════════════
		// AutoNAT 客户端配置
		// ════════════════════════════════════════════════════════════════════
		AutoNAT: AutoNATConfig{
			ProbeInterval:       15 * time.Second, // 探测间隔：15 秒，定期检查可达性
			ProbeTimeout:        10 * time.Second, // 探测超时：10 秒，必须小于间隔
			ConfidenceThreshold: 3,                // 置信度阈值：3 次一致结果才改变状态
			SuccessThreshold:    3,                // 成功阈值：3 次成功探测确认公网可达
			FailureThreshold:    3,                // 失败阈值：3 次失败探测确认 NAT 后
			EnableServer:        false,            // 服务端：默认禁用，服务器预设会启用
			ServerMaxProbes:     10,               // 服务端每分钟最大探测数：10 次
			ServerProbeTimeout:  10 * time.Second, // 服务端探测超时：10 秒
		},

		// ════════════════════════════════════════════════════════════════════
		// UPnP 配置
		// ════════════════════════════════════════════════════════════════════
		UPnP: UPnPConfig{
			MappingDuration: 1 * time.Hour,    // 端口映射租期：1 小时
			RenewalInterval: 30 * time.Minute, // 续期间隔：30 分钟，在过期前续期
			CacheDuration:   5 * time.Minute,  // 外部地址缓存：5 分钟
			Timeout:         5 * time.Second,  // 21: 操作超时 5 秒
		},

		// ════════════════════════════════════════════════════════════════════
		// NAT-PMP 配置（21）
		// ════════════════════════════════════════════════════════════════════
		NATPMP: NATPMPConfig{
			Timeout: 5 * time.Second, // NAT-PMP 操作超时：5 秒
		},

		// ════════════════════════════════════════════════════════════════════
		// Hole Punching 配置（DCUtR 协议）
		// ════════════════════════════════════════════════════════════════════
		HolePunch: HolePunchConfig{
			MaxRetries:     3,                 // 最大重试次数：3 次
			RetryDelay:     5 * time.Second,   // 重试延迟：5 秒
			ConnectTimeout: 30 * time.Second,  // 打洞连接超时：30 秒
			SyncTimeout:    10 * time.Second,  // SYNC 消息超时：10 秒
		},

		// ════════════════════════════════════════════════════════════════════
		// STUN 服务器（用于发现公网 IP 和端口）
		// ════════════════════════════════════════════════════════════════════
		STUNServers: []string{
			// 国内优先（降低 4G 网络超时概率）
			"stun.qq.com:3478",
			"stun.miwifi.com:3478",
			"stun.syncthing.net:3478",

			// 备选：国际 STUN
			"stun.l.google.com:19302",      // Google STUN 服务器 1
			"stun1.l.google.com:19302",     // Google STUN 服务器 2
			"stun2.l.google.com:19302",     // Google STUN 服务器 3
			"stun3.l.google.com:19302",     // Google STUN 服务器 4
			"stun.cloudflare.com:3478",     // Cloudflare STUN
			"stun.stunprotocol.org:3478",   // 开源 STUN 服务器
		},
	}
}

// Validate 验证 NAT 配置
func (c NATConfig) Validate() error {
	// 验证 AutoNAT 配置
	if c.EnableAutoNAT {
		if c.AutoNAT.ProbeInterval <= 0 {
			return errors.New("AutoNAT probe interval must be positive")
		}
		if c.AutoNAT.ProbeTimeout <= 0 {
			return errors.New("AutoNAT probe timeout must be positive")
		}
		if c.AutoNAT.ProbeTimeout >= c.AutoNAT.ProbeInterval {
			return errors.New("AutoNAT probe timeout must be less than probe interval")
		}
		if c.AutoNAT.ConfidenceThreshold <= 0 {
			return errors.New("AutoNAT confidence threshold must be positive")
		}
		if c.AutoNAT.SuccessThreshold <= 0 {
			return errors.New("AutoNAT success threshold must be positive")
		}
		if c.AutoNAT.FailureThreshold <= 0 {
			return errors.New("AutoNAT failure threshold must be positive")
		}
		if c.AutoNAT.EnableServer {
			if c.AutoNAT.ServerMaxProbes <= 0 {
				return errors.New("AutoNAT server max probes must be positive")
			}
			if c.AutoNAT.ServerProbeTimeout <= 0 {
				return errors.New("AutoNAT server probe timeout must be positive")
			}
		}
	}

	// 验证 UPnP 配置
	if c.EnableUPnP {
		if c.UPnP.MappingDuration <= 0 {
			return errors.New("UPnP mapping duration must be positive")
		}
		if c.UPnP.RenewalInterval <= 0 {
			return errors.New("UPnP renewal interval must be positive")
		}
		if c.UPnP.CacheDuration <= 0 {
			return errors.New("UPnP cache duration must be positive")
		}
	}

	// 验证 Hole Punching 配置
	if c.EnableHolePunch {
		if c.HolePunch.MaxRetries <= 0 {
			return errors.New("hole punch max retries must be positive")
		}
		if c.HolePunch.RetryDelay <= 0 {
			return errors.New("hole punch retry delay must be positive")
		}
		if c.HolePunch.ConnectTimeout <= 0 {
			return errors.New("hole punch connect timeout must be positive")
		}
		if c.HolePunch.SyncTimeout <= 0 {
			return errors.New("hole punch sync timeout must be positive")
		}
	}

	// 验证 STUN 服务器
	if c.EnableAutoNAT && len(c.STUNServers) == 0 {
		return errors.New("STUN servers list is empty but AutoNAT is enabled")
	}

	return nil
}

// WithAutoNAT 设置是否启用 AutoNAT
func (c NATConfig) WithAutoNAT(enabled bool) NATConfig {
	c.EnableAutoNAT = enabled
	return c
}

// WithUPnP 设置是否启用 UPnP
func (c NATConfig) WithUPnP(enabled bool) NATConfig {
	c.EnableUPnP = enabled
	return c
}

// WithNATPMP 设置是否启用 NAT-PMP
func (c NATConfig) WithNATPMP(enabled bool) NATConfig {
	c.EnableNATPMP = enabled
	return c
}

// WithHolePunch 设置是否启用 Hole Punching
func (c NATConfig) WithHolePunch(enabled bool) NATConfig {
	c.EnableHolePunch = enabled
	return c
}

// WithSTUNServers 设置 STUN 服务器列表
func (c NATConfig) WithSTUNServers(servers []string) NATConfig {
	c.STUNServers = servers
	return c
}
