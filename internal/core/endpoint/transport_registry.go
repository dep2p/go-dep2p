// Package endpoint 提供 Endpoint 聚合模块的实现
package endpoint

import (
	"sync"

	"github.com/dep2p/go-dep2p/internal/util/logger"
	endpointif "github.com/dep2p/go-dep2p/pkg/interfaces/endpoint"
	transportif "github.com/dep2p/go-dep2p/pkg/interfaces/transport"
	"github.com/dep2p/go-dep2p/pkg/types"
)

var transportRegistryLog = logger.Logger("transport-registry")

// ============================================================================
//                              TransportRegistry 实现
// ============================================================================

// TransportRegistry 传输注册表实现
//
// 管理多个传输实现，提供统一的传输选择能力。
// 支持的传输类型：
//   - QUIC Transport: 直连传输，支持 quic-v1 协议
//   - TCP Transport: 直连传输，支持 tcp 协议
//   - Relay Transport: 代理传输，支持 p2p-circuit 协议
type TransportRegistry struct {
	mu         sync.RWMutex
	transports map[string]transportif.Transport // protocol -> transport
}

// NewTransportRegistry 创建新的传输注册表
func NewTransportRegistry() *TransportRegistry {
	return &TransportRegistry{
		transports: make(map[string]transportif.Transport),
	}
}

// AddTransport 添加传输到注册表
//
// 如果已存在同协议的传输，返回错误。
func (r *TransportRegistry) AddTransport(t transportif.Transport) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	protocols := t.Protocols()
	if len(protocols) == 0 {
		return transportif.ErrNoSuitableTransport
	}

	// 检查是否存在冲突
	for _, proto := range protocols {
		if _, exists := r.transports[proto]; exists {
			return transportif.ErrTransportExists
		}
	}

	// 添加所有协议映射
	for _, proto := range protocols {
		r.transports[proto] = t
		transportRegistryLog.Info("注册传输",
			"protocol", proto,
			"proxy", t.Proxy(),
		)
	}

	return nil
}

// RemoveTransport 移除指定协议的传输
func (r *TransportRegistry) RemoveTransport(protocol string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	t, exists := r.transports[protocol]
	if !exists {
		return transportif.ErrTransportNotFound
	}

	// 移除该传输的所有协议映射
	for _, proto := range t.Protocols() {
		delete(r.transports, proto)
	}

	transportRegistryLog.Info("移除传输", "protocol", protocol)
	return nil
}

// TransportForDialing 获取可拨号到指定地址的传输
//
// 根据地址协议选择合适的传输：
//   - 中继地址（包含 /p2p-circuit）→ Relay Transport
//   - 其他地址 → 通过 CanDial 检查选择
func (r *TransportRegistry) TransportForDialing(addr endpointif.Address) transportif.Transport {
	r.mu.RLock()
	defer r.mu.RUnlock()

	addrStr := addr.String()

	// 优先检查是否为中继地址
	if types.IsRelayAddr(addrStr) {
		if t, ok := r.transports[types.RelayAddrProtocol]; ok {
			return t
		}
		transportRegistryLog.Debug("中继地址但无 Relay Transport",
			"addr", addrStr,
		)
		return nil
	}

	// 提取地址协议
	protocol := types.GetProtocolFromAddr(addrStr)
	if protocol != "" {
		if t, ok := r.transports[protocol]; ok {
			if t.CanDial(addr) {
				return t
			}
		}
	}

	// 遍历所有传输，找到可拨号的
	for _, t := range r.transports {
		if t.CanDial(addr) {
			return t
		}
	}

	transportRegistryLog.Debug("无合适传输",
		"addr", addrStr,
		"protocol", protocol,
	)
	return nil
}

// TransportForListening 获取可监听指定地址的传输
//
// 与 TransportForDialing 类似，但用于 Listen() 场景。
// 根据地址协议选择合适的传输：
//   - 中继地址（包含 /p2p-circuit）→ Relay Transport
//   - 其他地址 → 通过协议匹配或 CanDial 检查选择
func (r *TransportRegistry) TransportForListening(addr endpointif.Address) transportif.Transport {
	r.mu.RLock()
	defer r.mu.RUnlock()

	addrStr := addr.String()

	// 优先检查是否为中继地址
	if types.IsRelayAddr(addrStr) {
		if t, ok := r.transports[types.RelayAddrProtocol]; ok {
			return t
		}
		transportRegistryLog.Debug("中继监听地址但无 Relay Transport",
			"addr", addrStr,
		)
		return nil
	}

	// 提取地址协议
	protocol := types.GetProtocolFromAddr(addrStr)
	if protocol != "" {
		if t, ok := r.transports[protocol]; ok {
			return t
		}
	}

	// 遍历所有传输，找到可处理该地址的
	for _, t := range r.transports {
		if t.CanDial(addr) {
			return t
		}
	}

	return nil
}

// TransportForProtocol 获取指定协议的传输
func (r *TransportRegistry) TransportForProtocol(protocol string) transportif.Transport {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.transports[protocol]
}

// Transports 返回所有注册的传输（去重）
func (r *TransportRegistry) Transports() []transportif.Transport {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// 使用 map 去重（同一个 transport 可能注册了多个 protocol）
	seen := make(map[transportif.Transport]bool)
	result := make([]transportif.Transport, 0, len(r.transports))

	for _, t := range r.transports {
		if !seen[t] {
			seen[t] = true
			result = append(result, t)
		}
	}

	return result
}

// Protocols 返回所有支持的协议
func (r *TransportRegistry) Protocols() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	protocols := make([]string, 0, len(r.transports))
	for proto := range r.transports {
		protocols = append(protocols, proto)
	}
	return protocols
}

// Close 关闭所有传输
func (r *TransportRegistry) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// 使用 map 去重，避免重复关闭
	seen := make(map[transportif.Transport]bool)
	var lastErr error

	for _, t := range r.transports {
		if !seen[t] {
			seen[t] = true
			if err := t.Close(); err != nil {
				transportRegistryLog.Error("关闭传输失败",
					"protocols", t.Protocols(),
					"err", err,
				)
				lastErr = err
			}
		}
	}

	r.transports = make(map[string]transportif.Transport)
	return lastErr
}

// ============================================================================
//                              地址排序
// ============================================================================

// DefaultAddressRanker 默认地址排序器
type DefaultAddressRanker struct{}

// RankAddresses 对地址进行排序
//
// 排序策略：
//  1. 直连地址优先于中继地址
//  2. 同类型地址保持原始顺序
func (r *DefaultAddressRanker) RankAddresses(addrs []endpointif.Address) []endpointif.Address {
	if len(addrs) <= 1 {
		return addrs
	}

	direct := make([]endpointif.Address, 0, len(addrs))
	relay := make([]endpointif.Address, 0)

	for _, addr := range addrs {
		if types.IsRelayAddr(addr.String()) {
			relay = append(relay, addr)
		} else {
			direct = append(direct, addr)
		}
	}

	// 直连优先
	result := make([]endpointif.Address, 0, len(addrs))
	result = append(result, direct...)
	result = append(result, relay...)

	return result
}

// ============================================================================
//                              接口断言
// ============================================================================

var _ transportif.TransportRegistry = (*TransportRegistry)(nil)
var _ transportif.AddressRanker = (*DefaultAddressRanker)(nil)

