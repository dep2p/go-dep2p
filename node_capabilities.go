package dep2p

import (
	"context"
	"fmt"

	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// ════════════════════════════════════════════════════════════════════════════
//                              Bootstrap 能力（ADR-0009）
// ════════════════════════════════════════════════════════════════════════════

// EnableBootstrap 启用 Bootstrap 能力
//
// 将当前节点设置为引导节点，为网络中的新节点提供初始对等方发现服务。
func (n *Node) EnableBootstrap(ctx context.Context) error {
	if n.bootstrapService == nil {
		return fmt.Errorf("bootstrap service not available")
	}
	return n.bootstrapService.Enable(ctx)
}

// DisableBootstrap 禁用 Bootstrap 能力
//
// 停止作为引导节点服务，但保留已存储的节点信息。
func (n *Node) DisableBootstrap(ctx context.Context) error {
	if n.bootstrapService == nil {
		return nil
	}
	return n.bootstrapService.Disable(ctx)
}

// IsBootstrapEnabled 检查 Bootstrap 能力是否已启用
func (n *Node) IsBootstrapEnabled() bool {
	if n.bootstrapService == nil {
		return false
	}
	return n.bootstrapService.IsEnabled()
}

// BootstrapStats 获取 Bootstrap 统计信息
func (n *Node) BootstrapStats() pkgif.BootstrapStats {
	if n.bootstrapService == nil {
		return pkgif.BootstrapStats{}
	}
	return n.bootstrapService.Stats()
}

// ════════════════════════════════════════════════════════════════════════════
//                              Relay 能力（v2.0 统一接口）
// ════════════════════════════════════════════════════════════════════════════

// EnableRelay 启用 Relay 能力
//
// 将当前节点设置为中继服务器，为 NAT 后的节点提供中继服务。
func (n *Node) EnableRelay(ctx context.Context) error {
	if n.relayManager == nil {
		return fmt.Errorf("relay manager not available")
	}
	return n.relayManager.EnableRelay(ctx)
}

// DisableRelay 禁用 Relay 能力
func (n *Node) DisableRelay(ctx context.Context) error {
	if n.relayManager == nil {
		return nil
	}
	return n.relayManager.DisableRelay(ctx)
}

// IsRelayEnabled 检查 Relay 能力是否已启用
func (n *Node) IsRelayEnabled() bool {
	if n.relayManager == nil {
		return false
	}
	return n.relayManager.IsRelayEnabled()
}

// SetRelayAddr 设置要使用的 Relay 地址（客户端使用）
func (n *Node) SetRelayAddr(addr types.Multiaddr) error {
	if n.relayManager == nil {
		return fmt.Errorf("relay manager not available")
	}
	return n.relayManager.SetRelayAddr(addr)
}

// RemoveRelayAddr 移除 Relay 地址配置
func (n *Node) RemoveRelayAddr() error {
	if n.relayManager == nil {
		return nil
	}
	return n.relayManager.RemoveRelayAddr()
}

// RelayAddr 获取当前配置的 Relay 地址
func (n *Node) RelayAddr() (types.Multiaddr, bool) {
	if n.relayManager == nil {
		return nil, false
	}
	return n.relayManager.RelayAddr()
}

// RelayStats 获取 Relay 统计信息
func (n *Node) RelayStats() pkgif.RelayStats {
	if n.relayManager == nil {
		return pkgif.RelayStats{}
	}
	return n.relayManager.RelayStats()
}
