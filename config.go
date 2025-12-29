package dep2p

import (
	"encoding/json"
	"time"
)

// UserConfig 用户配置结构
//
// 这是面向用户的简化配置结构，可以从 JSON/YAML 文件加载。
// 内部会转换为详细的组件配置。
//
// 注意：配置文件的读取和环境变量的处理应由应用层（cmd/*）负责，
// 库本身不负责 I/O 操作。示例用法：
//
//	data, _ := os.ReadFile("config.json")
//	var cfg dep2p.UserConfig
//	json.Unmarshal(data, &cfg)
//	node, _ := dep2p.NewNode(cfg.ToOptions()...)
type UserConfig struct {
	// Preset 预设名称
	// 可选值: mobile, desktop, server, minimal, test
	// 预设之后的配置项可以覆盖预设中的值
	Preset string `json:"preset,omitempty"`

	// ListenPort 监听端口
	// 使用 QUIC 协议监听指定端口（同时监听 IPv4 和 IPv6）
	// 0 表示使用随机端口
	ListenPort int `json:"listen_port,omitempty"`

	// Identity 身份配置
	Identity *IdentityConfig `json:"identity,omitempty"`

	// ConnectionLimits 连接限制配置
	ConnectionLimits *ConnectionLimitsConfig `json:"connection_limits,omitempty"`

	// Discovery 发现服务配置
	Discovery *DiscoveryConfig `json:"discovery,omitempty"`

	// Relay 中继配置
	Relay *RelayConfig `json:"relay,omitempty"`

	// NAT NAT 穿透配置
	NAT *NATConfig `json:"nat,omitempty"`

	// Transport 传输配置
	Transport *TransportUserConfig `json:"transport,omitempty"`

	// Introspect 自省服务配置
	Introspect *IntrospectConfig `json:"introspect,omitempty"`
}

// IntrospectConfig 自省服务配置
type IntrospectConfig struct {
	// Enable 启用自省 HTTP 服务
	Enable bool `json:"enable,omitempty"`

	// Addr 监听地址，默认 "127.0.0.1:6060"
	Addr string `json:"addr,omitempty"`
}

// IdentityConfig 身份配置
type IdentityConfig struct {
	// KeyFile 密钥文件路径
	// 如果文件不存在，将自动创建新密钥
	KeyFile string `json:"key_file,omitempty"`
}

// ConnectionLimitsConfig 连接限制配置
type ConnectionLimitsConfig struct {
	// Low 低水位线
	// 连接数低于此值时停止裁剪
	Low int `json:"low,omitempty"`

	// High 高水位线
	// 连接数超过此值时触发裁剪
	High int `json:"high,omitempty"`

	// GracePeriod 新连接保护期
	// 新建立的连接在此期间不会被裁剪
	GracePeriod Duration `json:"grace_period,omitempty"`

	// IdleTimeout 空闲超时
	// 连接空闲超过此时间后可能被裁剪
	IdleTimeout Duration `json:"idle_timeout,omitempty"`
}

// DiscoveryConfig 发现服务配置
type DiscoveryConfig struct {
	// BootstrapPeers 引导节点列表
	BootstrapPeers []string `json:"bootstrap_peers,omitempty"`

	// RefreshInterval 刷新间隔
	RefreshInterval Duration `json:"refresh_interval,omitempty"`
}

// RelayConfig 中继配置
type RelayConfig struct {
	// Enable 启用中继客户端
	Enable bool `json:"enable,omitempty"`

	// EnableServer 启用中继服务器
	EnableServer bool `json:"enable_server,omitempty"`

	// MaxReservations 最大预留数（服务器模式）
	MaxReservations int `json:"max_reservations,omitempty"`

	// MaxCircuits 最大电路数（服务器模式）
	MaxCircuits int `json:"max_circuits,omitempty"`
}

// NATConfig NAT 穿透配置
type NATConfig struct {
	// Enable 启用 NAT 穿透
	Enable bool `json:"enable,omitempty"`

	// EnableUPnP 启用 UPnP 端口映射
	EnableUPnP bool `json:"enable_upnp,omitempty"`

	// EnableAutoNAT 启用自动 NAT 检测
	EnableAutoNAT bool `json:"enable_autonat,omitempty"`

	// STUNServers STUN 服务器列表
	STUNServers []string `json:"stun_servers,omitempty"`
}

// TransportUserConfig 传输配置（用户版）
type TransportUserConfig struct {
	// MaxConnections 最大连接数
	MaxConnections int `json:"max_connections,omitempty"`

	// MaxStreamsPerConn 每连接最大流数
	MaxStreamsPerConn int `json:"max_streams_per_conn,omitempty"`

	// IdleTimeout 空闲超时
	IdleTimeout Duration `json:"idle_timeout,omitempty"`

	// DialTimeout 拨号超时
	DialTimeout Duration `json:"dial_timeout,omitempty"`

	// HandshakeTimeout 握手超时
	HandshakeTimeout Duration `json:"handshake_timeout,omitempty"`
}

// ============================================================================
//                              Duration 类型
// ============================================================================

// Duration 是 time.Duration 的 JSON 友好版本
type Duration time.Duration

// MarshalJSON 实现 json.Marshaler
func (d Duration) MarshalJSON() ([]byte, error) {
	return json.Marshal(time.Duration(d).String())
}

// UnmarshalJSON 实现 json.Unmarshaler
func (d *Duration) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		// 尝试作为数字解析（纳秒）
		var ns int64
		if err := json.Unmarshal(data, &ns); err != nil {
			return err
		}
		*d = Duration(time.Duration(ns))
		return nil
	}

	duration, err := time.ParseDuration(s)
	if err != nil {
		return err
	}
	*d = Duration(duration)
	return nil
}

// Duration 返回 time.Duration
func (d Duration) Duration() time.Duration {
	return time.Duration(d)
}

// ============================================================================
//                              配置转换
// ============================================================================

// ToOptions 将用户配置转换为选项列表
func (c *UserConfig) ToOptions() []Option {
	var opts []Option

	// 预设
	if c.Preset != "" {
		if preset := GetPresetByName(c.Preset); preset != nil {
			opts = append(opts, WithPreset(preset))
		}
	}

	// 监听端口
	if c.ListenPort > 0 {
		opts = append(opts, WithListenPort(c.ListenPort))
	}

	// 身份配置
	if c.Identity != nil && c.Identity.KeyFile != "" {
		opts = append(opts, WithIdentityFromFile(c.Identity.KeyFile))
	}

	// 连接限制
	if c.ConnectionLimits != nil {
		if c.ConnectionLimits.Low > 0 || c.ConnectionLimits.High > 0 {
			opts = append(opts, WithConnectionLimits(
				c.ConnectionLimits.Low,
				c.ConnectionLimits.High,
			))
		}
	}

	// 发现配置
	if c.Discovery != nil {
		if len(c.Discovery.BootstrapPeers) > 0 {
			opts = append(opts, WithBootstrapPeers(c.Discovery.BootstrapPeers...))
		}
	}

	// 中继配置
	if c.Relay != nil {
		if c.Relay.Enable {
			opts = append(opts, WithRelay(true))
		}
		if c.Relay.EnableServer {
			opts = append(opts, WithRelayServer(true))
		}
	}

	// NAT 配置
	if c.NAT != nil {
		if c.NAT.Enable {
			opts = append(opts, WithNAT(true))
		}
	}

	// 自省服务配置
	if c.Introspect != nil {
		if c.Introspect.Enable {
			opts = append(opts, WithIntrospect(true))
		}
		if c.Introspect.Addr != "" {
			opts = append(opts, WithIntrospectAddr(c.Introspect.Addr))
		}
	}

	return opts
}
