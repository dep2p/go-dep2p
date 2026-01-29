// Package tls 实现 TLS 安全传输
//
// AccessControl 提供基于 PeerID 的访问控制功能：
//   - 白名单模式：只允许白名单中的节点连接
//   - 黑名单模式：拒绝黑名单中的节点连接
//   - 混合模式：白名单优先，黑名单其次
package tls

import (
	"errors"
	"sync"

	"github.com/dep2p/go-dep2p/pkg/lib/log"
	"github.com/dep2p/go-dep2p/pkg/types"
)

var accessLogger = log.Logger("core/security/tls/access")

// ============================================================================
//                              错误定义
// ============================================================================

var (
	// ErrAccessDenied 访问被拒绝
	ErrAccessDenied = errors.New("access denied")

	// ErrNotInWhitelist 不在白名单中
	ErrNotInWhitelist = errors.New("peer not in whitelist")

	// ErrInBlacklist 在黑名单中
	ErrInBlacklist = errors.New("peer in blacklist")
)

// ============================================================================
//                              访问控制模式
// ============================================================================

// AccessMode 访问控制模式
type AccessMode int

const (
	// AccessModeAllowAll 允许所有（默认）
	// 不进行任何访问控制检查
	AccessModeAllowAll AccessMode = iota

	// AccessModeWhitelist 白名单模式
	// 只允许白名单中的节点连接
	AccessModeWhitelist

	// AccessModeBlacklist 黑名单模式
	// 拒绝黑名单中的节点连接，其他都允许
	AccessModeBlacklist

	// AccessModeMixed 混合模式
	// 白名单优先（允许），黑名单其次（拒绝），其他允许
	AccessModeMixed
)

// String 返回模式的字符串表示
func (m AccessMode) String() string {
	switch m {
	case AccessModeAllowAll:
		return "allow_all"
	case AccessModeWhitelist:
		return "whitelist"
	case AccessModeBlacklist:
		return "blacklist"
	case AccessModeMixed:
		return "mixed"
	default:
		return "unknown"
	}
}

// ============================================================================
//                              AccessControl 结构
// ============================================================================

// AccessControl TLS 访问控制
//
// 提供基于 PeerID 的白名单和黑名单访问控制。
// 线程安全，支持动态添加和移除节点。
type AccessControl struct {
	// 白名单
	whitelist   map[types.PeerID]struct{}
	whitelistMu sync.RWMutex

	// 黑名单
	blacklist   map[types.PeerID]struct{}
	blacklistMu sync.RWMutex

	// 访问控制模式
	mode   AccessMode
	modeMu sync.RWMutex

	// 统计
	allowedCount uint64
	deniedCount  uint64
	statsMu      sync.RWMutex
}

// AccessControlConfig 访问控制配置
type AccessControlConfig struct {
	// Mode 访问控制模式
	Mode AccessMode

	// Whitelist 初始白名单
	Whitelist []types.PeerID

	// Blacklist 初始黑名单
	Blacklist []types.PeerID
}

// DefaultAccessControlConfig 返回默认配置
func DefaultAccessControlConfig() AccessControlConfig {
	return AccessControlConfig{
		Mode:      AccessModeAllowAll,
		Whitelist: nil,
		Blacklist: nil,
	}
}

// ============================================================================
//                              构造函数
// ============================================================================

// NewAccessControl 创建访问控制器
func NewAccessControl(config AccessControlConfig) *AccessControl {
	ac := &AccessControl{
		whitelist: make(map[types.PeerID]struct{}),
		blacklist: make(map[types.PeerID]struct{}),
		mode:      config.Mode,
	}

	// 初始化白名单
	for _, peerID := range config.Whitelist {
		ac.whitelist[peerID] = struct{}{}
	}

	// 初始化黑名单
	for _, peerID := range config.Blacklist {
		ac.blacklist[peerID] = struct{}{}
	}

	accessLogger.Info("访问控制器已创建",
		"mode", config.Mode.String(),
		"whitelist", len(config.Whitelist),
		"blacklist", len(config.Blacklist))

	return ac
}

// ============================================================================
//                              访问检查
// ============================================================================

// Check 检查节点是否允许连接
//
// 参数：
//   - peerID: 待检查的节点 ID
//
// 返回：
//   - error: 如果拒绝访问则返回错误，允许则返回 nil
func (ac *AccessControl) Check(peerID types.PeerID) error {
	ac.modeMu.RLock()
	mode := ac.mode
	ac.modeMu.RUnlock()

	var err error

	switch mode {
	case AccessModeAllowAll:
		err = nil

	case AccessModeWhitelist:
		if !ac.IsInWhitelist(peerID) {
			err = ErrNotInWhitelist
		}

	case AccessModeBlacklist:
		if ac.IsInBlacklist(peerID) {
			err = ErrInBlacklist
		}

	case AccessModeMixed:
		// 白名单优先
		if ac.IsInWhitelist(peerID) {
			err = nil
		} else if ac.IsInBlacklist(peerID) {
			err = ErrInBlacklist
		}
		// 既不在白名单也不在黑名单，允许

	default:
		err = nil
	}

	// 更新统计
	ac.statsMu.Lock()
	if err == nil {
		ac.allowedCount++
	} else {
		ac.deniedCount++
	}
	ac.statsMu.Unlock()

	if err != nil {
		peerIDStr := string(peerID)
		if len(peerIDStr) > 8 {
			peerIDStr = peerIDStr[:8]
		}
		accessLogger.Debug("访问被拒绝",
			"peerID", peerIDStr,
			"mode", mode.String(),
			"reason", err.Error())
	}

	return err
}

// ============================================================================
//                              白名单管理
// ============================================================================

// AddToWhitelist 添加节点到白名单
func (ac *AccessControl) AddToWhitelist(peerIDs ...types.PeerID) {
	ac.whitelistMu.Lock()
	defer ac.whitelistMu.Unlock()

	for _, peerID := range peerIDs {
		ac.whitelist[peerID] = struct{}{}
	}

	accessLogger.Debug("已添加到白名单", "count", len(peerIDs))
}

// RemoveFromWhitelist 从白名单移除节点
func (ac *AccessControl) RemoveFromWhitelist(peerIDs ...types.PeerID) {
	ac.whitelistMu.Lock()
	defer ac.whitelistMu.Unlock()

	for _, peerID := range peerIDs {
		delete(ac.whitelist, peerID)
	}

	accessLogger.Debug("已从白名单移除", "count", len(peerIDs))
}

// IsInWhitelist 检查节点是否在白名单中
func (ac *AccessControl) IsInWhitelist(peerID types.PeerID) bool {
	ac.whitelistMu.RLock()
	defer ac.whitelistMu.RUnlock()

	_, exists := ac.whitelist[peerID]
	return exists
}

// WhitelistSize 返回白名单大小
func (ac *AccessControl) WhitelistSize() int {
	ac.whitelistMu.RLock()
	defer ac.whitelistMu.RUnlock()

	return len(ac.whitelist)
}

// ClearWhitelist 清空白名单
func (ac *AccessControl) ClearWhitelist() {
	ac.whitelistMu.Lock()
	defer ac.whitelistMu.Unlock()

	ac.whitelist = make(map[types.PeerID]struct{})
	accessLogger.Debug("白名单已清空")
}

// GetWhitelist 返回白名单副本
func (ac *AccessControl) GetWhitelist() []types.PeerID {
	ac.whitelistMu.RLock()
	defer ac.whitelistMu.RUnlock()

	list := make([]types.PeerID, 0, len(ac.whitelist))
	for peerID := range ac.whitelist {
		list = append(list, peerID)
	}
	return list
}

// ============================================================================
//                              黑名单管理
// ============================================================================

// AddToBlacklist 添加节点到黑名单
func (ac *AccessControl) AddToBlacklist(peerIDs ...types.PeerID) {
	ac.blacklistMu.Lock()
	defer ac.blacklistMu.Unlock()

	for _, peerID := range peerIDs {
		ac.blacklist[peerID] = struct{}{}
	}

	accessLogger.Debug("已添加到黑名单", "count", len(peerIDs))
}

// RemoveFromBlacklist 从黑名单移除节点
func (ac *AccessControl) RemoveFromBlacklist(peerIDs ...types.PeerID) {
	ac.blacklistMu.Lock()
	defer ac.blacklistMu.Unlock()

	for _, peerID := range peerIDs {
		delete(ac.blacklist, peerID)
	}

	accessLogger.Debug("已从黑名单移除", "count", len(peerIDs))
}

// IsInBlacklist 检查节点是否在黑名单中
func (ac *AccessControl) IsInBlacklist(peerID types.PeerID) bool {
	ac.blacklistMu.RLock()
	defer ac.blacklistMu.RUnlock()

	_, exists := ac.blacklist[peerID]
	return exists
}

// BlacklistSize 返回黑名单大小
func (ac *AccessControl) BlacklistSize() int {
	ac.blacklistMu.RLock()
	defer ac.blacklistMu.RUnlock()

	return len(ac.blacklist)
}

// ClearBlacklist 清空黑名单
func (ac *AccessControl) ClearBlacklist() {
	ac.blacklistMu.Lock()
	defer ac.blacklistMu.Unlock()

	ac.blacklist = make(map[types.PeerID]struct{})
	accessLogger.Debug("黑名单已清空")
}

// GetBlacklist 返回黑名单副本
func (ac *AccessControl) GetBlacklist() []types.PeerID {
	ac.blacklistMu.RLock()
	defer ac.blacklistMu.RUnlock()

	list := make([]types.PeerID, 0, len(ac.blacklist))
	for peerID := range ac.blacklist {
		list = append(list, peerID)
	}
	return list
}

// ============================================================================
//                              模式管理
// ============================================================================

// SetMode 设置访问控制模式
func (ac *AccessControl) SetMode(mode AccessMode) {
	ac.modeMu.Lock()
	defer ac.modeMu.Unlock()

	oldMode := ac.mode
	ac.mode = mode

	accessLogger.Info("访问控制模式已更改",
		"old", oldMode.String(),
		"new", mode.String())
}

// GetMode 获取当前访问控制模式
func (ac *AccessControl) GetMode() AccessMode {
	ac.modeMu.RLock()
	defer ac.modeMu.RUnlock()

	return ac.mode
}

// ============================================================================
//                              统计信息
// ============================================================================

// AccessStats 访问统计
type AccessStats struct {
	// AllowedCount 允许的连接数
	AllowedCount uint64

	// DeniedCount 拒绝的连接数
	DeniedCount uint64

	// WhitelistSize 白名单大小
	WhitelistSize int

	// BlacklistSize 黑名单大小
	BlacklistSize int

	// Mode 当前模式
	Mode AccessMode
}

// Stats 返回访问统计
func (ac *AccessControl) Stats() AccessStats {
	ac.statsMu.RLock()
	allowed := ac.allowedCount
	denied := ac.deniedCount
	ac.statsMu.RUnlock()

	return AccessStats{
		AllowedCount:  allowed,
		DeniedCount:   denied,
		WhitelistSize: ac.WhitelistSize(),
		BlacklistSize: ac.BlacklistSize(),
		Mode:          ac.GetMode(),
	}
}

// ResetStats 重置统计计数
func (ac *AccessControl) ResetStats() {
	ac.statsMu.Lock()
	defer ac.statsMu.Unlock()

	ac.allowedCount = 0
	ac.deniedCount = 0
}

// ============================================================================
//                              接口断言
// ============================================================================

// ConnectionGater 连接门控接口
// 与 libp2p 的 ConnectionGater 兼容
type ConnectionGater interface {
	// InterceptPeerDial 拦截对等节点拨号
	InterceptPeerDial(peerID types.PeerID) bool

	// InterceptAccept 拦截入站连接接受
	InterceptAccept(peerID types.PeerID) bool

	// InterceptSecured 拦截安全连接建立
	InterceptSecured(peerID types.PeerID) bool
}

// InterceptPeerDial 实现 ConnectionGater 接口
func (ac *AccessControl) InterceptPeerDial(peerID types.PeerID) bool {
	return ac.Check(peerID) == nil
}

// InterceptAccept 实现 ConnectionGater 接口
func (ac *AccessControl) InterceptAccept(peerID types.PeerID) bool {
	return ac.Check(peerID) == nil
}

// InterceptSecured 实现 ConnectionGater 接口
func (ac *AccessControl) InterceptSecured(peerID types.PeerID) bool {
	return ac.Check(peerID) == nil
}

// 确保实现了 ConnectionGater 接口
var _ ConnectionGater = (*AccessControl)(nil)
