// Package tls 提供基于 TLS 的安全传输实现
package tls

import (
	"sync"

	securityif "github.com/dep2p/go-dep2p/pkg/interfaces/security"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// AccessMode 访问控制模式
type AccessMode int

const (
	// AccessModeAllow 默认允许模式（只有黑名单生效）
	AccessModeAllow AccessMode = iota
	// AccessModeDeny 默认拒绝模式（只有白名单生效）
	AccessModeDeny
)

// AccessController 访问控制器
// 实现白名单/黑名单访问控制
type AccessController struct {
	mode      AccessMode
	allowList map[types.NodeID]struct{}
	blockList map[types.NodeID]struct{}
	mu        sync.RWMutex
}

// 确保实现 securityif.AccessController 接口
var _ securityif.AccessController = (*AccessController)(nil)

// NewAccessController 创建访问控制器
// 默认为允许模式
func NewAccessController() *AccessController {
	return &AccessController{
		mode:      AccessModeAllow,
		allowList: make(map[types.NodeID]struct{}),
		blockList: make(map[types.NodeID]struct{}),
	}
}

// NewAccessControllerWithMode 使用指定模式创建访问控制器
func NewAccessControllerWithMode(mode AccessMode) *AccessController {
	return &AccessController{
		mode:      mode,
		allowList: make(map[types.NodeID]struct{}),
		blockList: make(map[types.NodeID]struct{}),
	}
}

// SetMode 设置访问控制模式
func (ac *AccessController) SetMode(mode AccessMode) {
	ac.mu.Lock()
	defer ac.mu.Unlock()
	ac.mode = mode
}

// Mode 返回当前访问控制模式
func (ac *AccessController) Mode() AccessMode {
	ac.mu.RLock()
	defer ac.mu.RUnlock()
	return ac.mode
}

// AllowConnect 检查是否允许连接
func (ac *AccessController) AllowConnect(nodeID types.NodeID) bool {
	ac.mu.RLock()
	defer ac.mu.RUnlock()
	return ac.allowConnectLocked(nodeID)
}

// AllowInbound 检查是否允许入站连接
func (ac *AccessController) AllowInbound(nodeID types.NodeID) bool {
	return ac.AllowConnect(nodeID)
}

// AllowOutbound 检查是否允许出站连接
func (ac *AccessController) AllowOutbound(nodeID types.NodeID) bool {
	return ac.AllowConnect(nodeID)
}

// allowConnectLocked 内部检查函数（需持有读锁）
func (ac *AccessController) allowConnectLocked(nodeID types.NodeID) bool {
	// 空 NodeID 总是拒绝
	if nodeID.IsEmpty() {
		return false
	}

	// 检查黑名单
	if _, blocked := ac.blockList[nodeID]; blocked {
		return false
	}

	// 根据模式检查
	switch ac.mode {
	case AccessModeAllow:
		// 允许模式：不在黑名单中即允许
		return true
	case AccessModeDeny:
		// 拒绝模式：必须在白名单中
		_, allowed := ac.allowList[nodeID]
		return allowed
	default:
		return false
	}
}

// AddToAllowList 添加到白名单
func (ac *AccessController) AddToAllowList(nodeID types.NodeID) {
	if nodeID.IsEmpty() {
		return
	}
	ac.mu.Lock()
	defer ac.mu.Unlock()
	ac.allowList[nodeID] = struct{}{}
}

// AddToBlockList 添加到黑名单
func (ac *AccessController) AddToBlockList(nodeID types.NodeID) {
	if nodeID.IsEmpty() {
		return
	}
	ac.mu.Lock()
	defer ac.mu.Unlock()
	ac.blockList[nodeID] = struct{}{}
}

// RemoveFromAllowList 从白名单移除
func (ac *AccessController) RemoveFromAllowList(nodeID types.NodeID) {
	ac.mu.Lock()
	defer ac.mu.Unlock()
	delete(ac.allowList, nodeID)
}

// RemoveFromBlockList 从黑名单移除
func (ac *AccessController) RemoveFromBlockList(nodeID types.NodeID) {
	ac.mu.Lock()
	defer ac.mu.Unlock()
	delete(ac.blockList, nodeID)
}

// IsInAllowList 检查是否在白名单中
func (ac *AccessController) IsInAllowList(nodeID types.NodeID) bool {
	ac.mu.RLock()
	defer ac.mu.RUnlock()
	_, ok := ac.allowList[nodeID]
	return ok
}

// IsInBlockList 检查是否在黑名单中
func (ac *AccessController) IsInBlockList(nodeID types.NodeID) bool {
	ac.mu.RLock()
	defer ac.mu.RUnlock()
	_, ok := ac.blockList[nodeID]
	return ok
}

// AllowListCount 返回白名单数量
func (ac *AccessController) AllowListCount() int {
	ac.mu.RLock()
	defer ac.mu.RUnlock()
	return len(ac.allowList)
}

// BlockListCount 返回黑名单数量
func (ac *AccessController) BlockListCount() int {
	ac.mu.RLock()
	defer ac.mu.RUnlock()
	return len(ac.blockList)
}

// ClearAllowList 清空白名单
func (ac *AccessController) ClearAllowList() {
	ac.mu.Lock()
	defer ac.mu.Unlock()
	ac.allowList = make(map[types.NodeID]struct{})
}

// ClearBlockList 清空黑名单
func (ac *AccessController) ClearBlockList() {
	ac.mu.Lock()
	defer ac.mu.Unlock()
	ac.blockList = make(map[types.NodeID]struct{})
}

// Clear 清空所有列表
func (ac *AccessController) Clear() {
	ac.mu.Lock()
	defer ac.mu.Unlock()
	ac.allowList = make(map[types.NodeID]struct{})
	ac.blockList = make(map[types.NodeID]struct{})
}

// AllowedPeers 返回白名单中的所有节点
func (ac *AccessController) AllowedPeers() []types.NodeID {
	ac.mu.RLock()
	defer ac.mu.RUnlock()
	peers := make([]types.NodeID, 0, len(ac.allowList))
	for id := range ac.allowList {
		peers = append(peers, id)
	}
	return peers
}

// BlockedPeers 返回黑名单中的所有节点
func (ac *AccessController) BlockedPeers() []types.NodeID {
	ac.mu.RLock()
	defer ac.mu.RUnlock()
	peers := make([]types.NodeID, 0, len(ac.blockList))
	for id := range ac.blockList {
		peers = append(peers, id)
	}
	return peers
}

