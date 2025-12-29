package components

import (
	"time"

	"github.com/dep2p/go-dep2p/internal/config"
)

// NATOptions NAT 穿透选项
type NATOptions struct {
	// Enable 启用 NAT 穿透
	Enable bool

	// EnableUPnP 启用 UPnP
	EnableUPnP bool

	// EnableAutoNAT 启用自动 NAT 检测
	EnableAutoNAT bool

	// EnableHolePunching 启用打洞
	EnableHolePunching bool

	// STUNServers STUN 服务器列表
	STUNServers []string

	// RefreshInterval 映射刷新间隔
	RefreshInterval time.Duration
}

// NewNATOptions 从配置创建 NAT 选项
func NewNATOptions(cfg *config.NATConfig) *NATOptions {
	return &NATOptions{
		Enable:             cfg.Enable,
		EnableUPnP:         cfg.EnableUPnP,
		EnableAutoNAT:      cfg.EnableAutoNAT,
		EnableHolePunching: cfg.EnableHolePunching,
		STUNServers:        cfg.STUNServers,
		RefreshInterval:    cfg.RefreshInterval,
	}
}

// DefaultNATOptions 默认 NAT 选项
func DefaultNATOptions() *NATOptions {
	return &NATOptions{
		Enable:             true,
		EnableUPnP:         true,
		EnableAutoNAT:      true,
		EnableHolePunching: true,
		STUNServers: []string{
			"stun:stun.l.google.com:19302",
			"stun:stun1.l.google.com:19302",
		},
		RefreshInterval: 30 * time.Second,
	}
}

