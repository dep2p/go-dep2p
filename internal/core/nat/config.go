package nat

import (
	"errors"
	"time"
)

// Config NAT 服务配置
type Config struct {
	// EnableAutoNAT 是否启用 AutoNAT 检测
	EnableAutoNAT bool

	// EnableUPnP 是否启用 UPnP 端口映射
	EnableUPnP bool

	// EnableNATPMP 是否启用 NAT-PMP 端口映射
	EnableNATPMP bool

	// EnableHolePunch 是否启用 Hole Punching
	EnableHolePunch bool

	// STUNServers STUN 服务器列表
	STUNServers []string

	// ProbeInterval AutoNAT 探测间隔
	ProbeInterval time.Duration

	// ProbeTimeout 探测超时时间
	ProbeTimeout time.Duration

	// MappingDuration 端口映射租期
	MappingDuration time.Duration

	// ConfidenceThreshold 置信度阈值（达到该值才改变状态）
	ConfidenceThreshold int

	// ProbeSuccessThreshold 成功探测次数阈值
	ProbeSuccessThreshold int

	// ProbeFailureThreshold 失败探测次数阈值
	ProbeFailureThreshold int

	// STUNCacheDuration STUN 地址缓存时间
	STUNCacheDuration time.Duration

	// MappingRenewalInterval 端口映射续期检查间隔
	MappingRenewalInterval time.Duration

	// NATTypeDetectionEnabled 是否启用 NAT 类型检测
	NATTypeDetectionEnabled bool

	// AlternateSTUNServer 备用 STUN 服务器（不同 IP，用于 NAT 类型检测）
	AlternateSTUNServer string

	// NATTypeDetectionTimeout NAT 类型检测超时时间
	NATTypeDetectionTimeout time.Duration

	// v2.0 新增：自动 Relay 配置
	// AutoEnableRelay 当检测到 Private 时是否自动启用 Relay 客户端
	// 默认: true
	AutoEnableRelay bool

	// 
	// LockReachabilityPublic 是否锁定可达性为 Public
	// 当节点作为引导节点运行且配置了公网地址时设为 true
	// 锁定后不会被 AutoNAT 或 NAT 类型检测降级为 Private
	LockReachabilityPublic bool

	// NATPMPTimeout NAT-PMP 操作超时时间（发现、测试连接、端口映射）
	// 21: 添加可配置的 NAT-PMP 超时
	// 默认: 5 秒
	NATPMPTimeout time.Duration

	// UPnPTimeout UPnP 操作超时时间
	// 默认: 5 秒
	UPnPTimeout time.Duration
}

// DefaultConfig 返回默认配置
func DefaultConfig() *Config {
	return &Config{
		EnableAutoNAT:   true,
		EnableUPnP:      true,
		EnableNATPMP:    true,
		EnableHolePunch: true,
		STUNServers: []string{
			"stun.l.google.com:19302",
			"stun1.l.google.com:19302",
			"stun2.l.google.com:19302",
			"stun.stunprotocol.org:3478",
		},
		ProbeInterval:          15 * time.Second,
		ProbeTimeout:           10 * time.Second,
		MappingDuration:        3600 * time.Second, // 1 小时
		ConfidenceThreshold:    3,
		ProbeSuccessThreshold:  3,
		ProbeFailureThreshold:  3,
		STUNCacheDuration:       5 * time.Minute,
		MappingRenewalInterval:  30 * time.Minute,
		NATTypeDetectionEnabled: true,
		AlternateSTUNServer:     "stun.stunprotocol.org:3478",
		NATTypeDetectionTimeout: 3 * time.Second,
		AutoEnableRelay:         true, // v2.0 默认启用自动 Relay
		NATPMPTimeout:           5 * time.Second, // 21: 默认 5 秒超时
		UPnPTimeout:             5 * time.Second, // 默认 5 秒超时
	}
}

// Validate 验证配置
func (c *Config) Validate() error {
	if c == nil {
		return errors.New("config is nil")
	}

	if c.ProbeInterval <= 0 {
		return errors.New("probe interval must be positive")
	}

	if c.ProbeTimeout <= 0 {
		return errors.New("probe timeout must be positive")
	}

	if c.MappingDuration <= 0 {
		return errors.New("mapping duration must be positive")
	}

	if c.ConfidenceThreshold <= 0 {
		return errors.New("confidence threshold must be positive")
	}

	if c.ProbeSuccessThreshold <= 0 {
		return errors.New("probe success threshold must be positive")
	}

	if c.ProbeFailureThreshold <= 0 {
		return errors.New("probe failure threshold must be positive")
	}

	if c.STUNCacheDuration <= 0 {
		return errors.New("STUN cache duration must be positive")
	}

	if c.MappingRenewalInterval <= 0 {
		return errors.New("mapping renewal interval must be positive")
	}

	// 如果启用 AutoNAT，探测间隔应该合理
	if c.EnableAutoNAT && c.ProbeInterval < time.Second {
		return errors.New("probe interval too short (min 1s)")
	}

	// 超时应该小于间隔
	if c.ProbeTimeout >= c.ProbeInterval {
		return errors.New("probe timeout must be less than probe interval")
	}

	return nil
}

// Option 配置选项函数
type Option func(*Config) error

// WithAutoNAT 设置是否启用 AutoNAT
func WithAutoNAT(enabled bool) Option {
	return func(c *Config) error {
		c.EnableAutoNAT = enabled
		return nil
	}
}

// WithUPnP 设置是否启用 UPnP
func WithUPnP(enabled bool) Option {
	return func(c *Config) error {
		c.EnableUPnP = enabled
		return nil
	}
}

// WithNATPMP 设置是否启用 NAT-PMP
func WithNATPMP(enabled bool) Option {
	return func(c *Config) error {
		c.EnableNATPMP = enabled
		return nil
	}
}

// WithHolePunch 设置是否启用 Hole Punching
func WithHolePunch(enabled bool) Option {
	return func(c *Config) error {
		c.EnableHolePunch = enabled
		return nil
	}
}

// WithSTUNServers 设置 STUN 服务器列表
func WithSTUNServers(servers []string) Option {
	return func(c *Config) error {
		if len(servers) == 0 {
			return errors.New("STUN servers list is empty")
		}
		c.STUNServers = servers
		return nil
	}
}

// WithProbeInterval 设置探测间隔
func WithProbeInterval(interval time.Duration) Option {
	return func(c *Config) error {
		if interval <= 0 {
			return errors.New("probe interval must be positive")
		}
		c.ProbeInterval = interval
		return nil
	}
}

// WithProbeTimeout 设置探测超时
func WithProbeTimeout(timeout time.Duration) Option {
	return func(c *Config) error {
		if timeout <= 0 {
			return errors.New("probe timeout must be positive")
		}
		c.ProbeTimeout = timeout
		return nil
	}
}

// WithMappingDuration 设置端口映射租期
func WithMappingDuration(duration time.Duration) Option {
	return func(c *Config) error {
		if duration <= 0 {
			return errors.New("mapping duration must be positive")
		}
		c.MappingDuration = duration
		return nil
	}
}

// WithConfidenceThreshold 设置置信度阈值
func WithConfidenceThreshold(threshold int) Option {
	return func(c *Config) error {
		if threshold <= 0 {
			return errors.New("confidence threshold must be positive")
		}
		c.ConfidenceThreshold = threshold
		return nil
	}
}

// WithNATTypeDetection 设置是否启用 NAT 类型检测
func WithNATTypeDetection(enabled bool) Option {
	return func(c *Config) error {
		c.NATTypeDetectionEnabled = enabled
		return nil
	}
}

// WithAlternateSTUNServer 设置备用 STUN 服务器
func WithAlternateSTUNServer(server string) Option {
	return func(c *Config) error {
		c.AlternateSTUNServer = server
		return nil
	}
}

// WithNATTypeDetectionTimeout 设置 NAT 类型检测超时时间
func WithNATTypeDetectionTimeout(timeout time.Duration) Option {
	return func(c *Config) error {
		if timeout <= 0 {
			return errors.New("NAT type detection timeout must be positive")
		}
		c.NATTypeDetectionTimeout = timeout
		return nil
	}
}

// WithNATPMPTimeout 设置 NAT-PMP 操作超时时间
//
// 21: 添加可配置的 NAT-PMP 超时
func WithNATPMPTimeout(timeout time.Duration) Option {
	return func(c *Config) error {
		if timeout <= 0 {
			return errors.New("NAT-PMP timeout must be positive")
		}
		c.NATPMPTimeout = timeout
		return nil
	}
}

// WithUPnPTimeout 设置 UPnP 操作超时时间
func WithUPnPTimeout(timeout time.Duration) Option {
	return func(c *Config) error {
		if timeout <= 0 {
			return errors.New("UPnP timeout must be positive")
		}
		c.UPnPTimeout = timeout
		return nil
	}
}

// ApplyOptions 应用配置选项
func (c *Config) ApplyOptions(opts ...Option) error {
	for _, opt := range opts {
		if err := opt(c); err != nil {
			return err
		}
	}
	return c.Validate()
}
