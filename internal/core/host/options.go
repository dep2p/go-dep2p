package host

import (
	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/internal/core/nat"
	"github.com/dep2p/go-dep2p/internal/core/protocol"
	"github.com/dep2p/go-dep2p/internal/core/relay"
)

// Option Host 构造选项类型
type Option func(*Host) error

// WithSwarm 设置 Swarm
func WithSwarm(swarm pkgif.Swarm) Option {
	return func(h *Host) error {
		h.swarm = swarm
		return nil
	}
}

// WithPeerstore 设置 Peerstore
func WithPeerstore(ps pkgif.Peerstore) Option {
	return func(h *Host) error {
		h.peerstore = ps
		return nil
	}
}

// WithEventBus 设置 EventBus
func WithEventBus(eb pkgif.EventBus) Option {
	return func(h *Host) error {
		h.eventbus = eb
		return nil
	}
}

// WithConnManager 设置 ConnManager
func WithConnManager(cm pkgif.ConnManager) Option {
	return func(h *Host) error {
		h.connmgr = cm
		return nil
	}
}

// WithResourceManager 设置 ResourceManager
func WithResourceManager(rm pkgif.ResourceManager) Option {
	return func(h *Host) error {
		h.resourcemgr = rm
		return nil
	}
}

// WithProtocol 设置 Protocol Router
func WithProtocol(pr *protocol.Router) Option {
	return func(h *Host) error {
		h.protocol = pr
		return nil
	}
}

// WithNAT 设置 NAT Service
func WithNAT(natService *nat.Service) Option {
	return func(h *Host) error {
		h.nat = natService
		return nil
	}
}

// WithRelay 设置 Relay Manager
func WithRelay(relayMgr *relay.Manager) Option {
	return func(h *Host) error {
		h.relay = relayMgr
		return nil
	}
}

// WithConfig 设置配置
func WithConfig(cfg *Config) Option {
	return func(h *Host) error {
		if cfg == nil {
			return nil
		}
		if err := cfg.Validate(); err != nil {
			return err
		}
		h.config = cfg
		return nil
	}
}
