package components

import (
	"time"

	"github.com/dep2p/go-dep2p/internal/config"
)

// RelayOptions 中继选项
type RelayOptions struct {
	// Enable 启用中继客户端
	Enable bool

	// EnableServer 启用中继服务器
	EnableServer bool

	// MaxReservations 最大预留数（服务器）
	MaxReservations int

	// MaxCircuits 最大电路数（服务器）
	MaxCircuits int

	// MaxCircuitsPerPeer 每节点最大电路数（服务器）
	MaxCircuitsPerPeer int

	// ReservationTTL 预留有效期
	ReservationTTL time.Duration
}

// NewRelayOptions 从配置创建中继选项
func NewRelayOptions(cfg *config.RelayConfig) *RelayOptions {
	return &RelayOptions{
		Enable:             cfg.Enable,
		EnableServer:       cfg.EnableServer,
		MaxReservations:    cfg.MaxReservations,
		MaxCircuits:        cfg.MaxCircuits,
		MaxCircuitsPerPeer: cfg.MaxCircuitsPerPeer,
		ReservationTTL:     cfg.ReservationTTL,
	}
}

// DefaultRelayOptions 默认中继选项
func DefaultRelayOptions() *RelayOptions {
	return &RelayOptions{
		Enable:             true,
		EnableServer:       false,
		MaxReservations:    128,
		MaxCircuits:        16,
		MaxCircuitsPerPeer: 4,
		ReservationTTL:     1 * time.Hour,
	}
}

