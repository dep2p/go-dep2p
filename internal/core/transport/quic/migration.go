// Package quic 提供基于 QUIC 的传输层实现
//
// 连接迁移支持：
// - 监听网络接口变化
// - 自动迁移活跃连接
// - 通知地址变更
package quic

import (
	"context"
	"net"
	"sync"
	"time"

	"github.com/dep2p/go-dep2p/internal/util/logger"
)

var log = logger.Logger("transport.quic")

// ============================================================================
//                              ConnectionMigrator 实现
// ============================================================================

// ConnectionMigrator 连接迁移器
type ConnectionMigrator struct {
	transport *Transport

	// 当前网络接口
	currentAddrs   []net.Addr
	currentAddrsMu sync.RWMutex

	// 迁移事件通道
	migrationCh chan MigrationEvent

	// 地址变更回调
	callbacks   []AddressChangeCallback
	callbacksMu sync.RWMutex

	// 状态
	running  bool
	stopCh   chan struct{}
	stopOnce sync.Once
	mu       sync.Mutex
}

// MigrationEvent 迁移事件
type MigrationEvent struct {
	// OldAddrs 旧地址列表
	OldAddrs []net.Addr

	// NewAddrs 新地址列表
	NewAddrs []net.Addr

	// AddedAddrs 新增的地址
	AddedAddrs []net.Addr

	// RemovedAddrs 移除的地址
	RemovedAddrs []net.Addr

	// Timestamp 事件时间
	Timestamp time.Time
}

// AddressChangeCallback 地址变更回调
type AddressChangeCallback func(event MigrationEvent)

// MigratorConfig 迁移器配置
type MigratorConfig struct {
	// PollInterval 轮询间隔
	PollInterval time.Duration

	// EnableAutoMigration 启用自动迁移
	EnableAutoMigration bool
}

// DefaultMigratorConfig 返回默认配置
func DefaultMigratorConfig() MigratorConfig {
	return MigratorConfig{
		PollInterval:        5 * time.Second,
		EnableAutoMigration: true,
	}
}

// NewConnectionMigrator 创建连接迁移器
func NewConnectionMigrator(transport *Transport) *ConnectionMigrator {
	return &ConnectionMigrator{
		transport:   transport,
		migrationCh: make(chan MigrationEvent, 10),
		callbacks:   make([]AddressChangeCallback, 0),
		stopCh:      make(chan struct{}),
	}
}

// ============================================================================
//                              生命周期
// ============================================================================

// Start 启动迁移器
func (m *ConnectionMigrator) Start(ctx context.Context, config MigratorConfig) error {
	m.mu.Lock()
	if m.running {
		m.mu.Unlock()
		return nil
	}
	m.running = true
	m.mu.Unlock()

	// 获取初始地址
	addrs, err := m.getNetworkAddresses()
	if err != nil {
		log.Warn("获取初始网络地址失败", "err", err)
	}
	m.currentAddrsMu.Lock()
	m.currentAddrs = addrs
	m.currentAddrsMu.Unlock()

	// 启动监控循环
	go m.monitorLoop(ctx, config)

	log.Info("连接迁移器已启动",
		"pollInterval", config.PollInterval,
		"initialAddrs", len(addrs))

	return nil
}

// Stop 停止迁移器
// 多次调用是安全的
func (m *ConnectionMigrator) Stop() error {
	m.mu.Lock()
	if !m.running {
		m.mu.Unlock()
		return nil
	}
	m.running = false
	m.mu.Unlock()

	// 使用 sync.Once 确保 channel 只关闭一次
	m.stopOnce.Do(func() {
		close(m.stopCh)
	})

	log.Info("连接迁移器已停止")
	return nil
}

// ============================================================================
//                              回调管理
// ============================================================================

// OnAddressChange 注册地址变更回调
func (m *ConnectionMigrator) OnAddressChange(callback AddressChangeCallback) {
	m.callbacksMu.Lock()
	defer m.callbacksMu.Unlock()
	m.callbacks = append(m.callbacks, callback)
}

// MigrationEvents 返回迁移事件通道
func (m *ConnectionMigrator) MigrationEvents() <-chan MigrationEvent {
	return m.migrationCh
}

// ============================================================================
//                              迁移方法
// ============================================================================

// TriggerMigration 手动触发迁移检查
func (m *ConnectionMigrator) TriggerMigration() error {
	return m.checkForMigration()
}

// GetCurrentAddresses 获取当前网络地址
func (m *ConnectionMigrator) GetCurrentAddresses() []net.Addr {
	m.currentAddrsMu.RLock()
	defer m.currentAddrsMu.RUnlock()
	result := make([]net.Addr, len(m.currentAddrs))
	copy(result, m.currentAddrs)
	return result
}

// ============================================================================
//                              内部方法
// ============================================================================

// monitorLoop 监控循环
func (m *ConnectionMigrator) monitorLoop(ctx context.Context, config MigratorConfig) {
	ticker := time.NewTicker(config.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-m.stopCh:
			return
		case <-ticker.C:
			if err := m.checkForMigration(); err != nil {
				log.Debug("迁移检查失败", "err", err)
			}
		}
	}
}

// checkForMigration 检查是否需要迁移
func (m *ConnectionMigrator) checkForMigration() error {
	// 获取当前网络地址
	newAddrs, err := m.getNetworkAddresses()
	if err != nil {
		return err
	}

	// 原子地读取、比较和更新地址
	m.currentAddrsMu.Lock()
	// 复制旧地址以避免竞态
	oldAddrs := make([]net.Addr, len(m.currentAddrs))
	copy(oldAddrs, m.currentAddrs)

	added, removed := m.diffAddresses(oldAddrs, newAddrs)

	// 如果有变化，更新当前地址
	var hasChanges bool
	if len(added) > 0 || len(removed) > 0 {
		m.currentAddrs = newAddrs
		hasChanges = true
	}
	m.currentAddrsMu.Unlock()

	// 如果有变化，触发迁移事件（在锁外进行，避免死锁）
	if hasChanges {
		event := MigrationEvent{
			OldAddrs:     oldAddrs,
			NewAddrs:     newAddrs,
			AddedAddrs:   added,
			RemovedAddrs: removed,
			Timestamp:    time.Now(),
		}

		log.Info("检测到网络地址变化",
			"added", len(added),
			"removed", len(removed))

		// 发送事件
		select {
		case m.migrationCh <- event:
		default:
			log.Warn("迁移事件通道已满")
		}

		// 触发回调
		m.triggerCallbacks(event)

		// 执行迁移
		m.migrateConnections(event)
	}

	return nil
}

// getNetworkAddresses 获取所有网络地址
func (m *ConnectionMigrator) getNetworkAddresses() ([]net.Addr, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	var addrs []net.Addr
	for _, iface := range interfaces {
		// 跳过回环接口和未激活的接口
		if iface.Flags&net.FlagLoopback != 0 || iface.Flags&net.FlagUp == 0 {
			continue
		}

		ifAddrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		addrs = append(addrs, ifAddrs...)
	}

	return addrs, nil
}

// diffAddresses 比较地址差异
func (m *ConnectionMigrator) diffAddresses(old, current []net.Addr) (added, removed []net.Addr) {
	oldMap := make(map[string]net.Addr)
	for _, addr := range old {
		oldMap[addr.String()] = addr
	}

	newMap := make(map[string]net.Addr)
	for _, addr := range current {
		newMap[addr.String()] = addr
	}

	// 找新增的
	for key, addr := range newMap {
		if _, ok := oldMap[key]; !ok {
			added = append(added, addr)
		}
	}

	// 找移除的
	for key, addr := range oldMap {
		if _, ok := newMap[key]; !ok {
			removed = append(removed, addr)
		}
	}

	return
}

// triggerCallbacks 触发所有回调
func (m *ConnectionMigrator) triggerCallbacks(event MigrationEvent) {
	m.callbacksMu.RLock()
	callbacks := make([]AddressChangeCallback, len(m.callbacks))
	copy(callbacks, m.callbacks)
	m.callbacksMu.RUnlock()

	for _, callback := range callbacks {
		go callback(event)
	}
}

// migrateConnections 迁移连接
func (m *ConnectionMigrator) migrateConnections(event MigrationEvent) {
	if m.transport == nil {
		return
	}

	// QUIC 原生支持连接迁移，quic-go 会自动处理
	// 这里主要做日志记录和状态更新

	m.transport.connsMu.RLock()
	connCount := len(m.transport.conns)
	m.transport.connsMu.RUnlock()

	log.Info("连接迁移触发",
		"activeConns", connCount,
		"addedAddrs", len(event.AddedAddrs),
		"removedAddrs", len(event.RemovedAddrs))

	// 注意：quic-go 的连接迁移是自动的
	// 当底层 UDP socket 的本地地址改变时，QUIC 连接会自动迁移
}

// ============================================================================
//                              工具方法
// ============================================================================

// IsPublicAddr 检查是否是公网地址
func IsPublicAddr(addr net.Addr) bool {
	ipNet, ok := addr.(*net.IPNet)
	if !ok {
		return false
	}

	ip := ipNet.IP
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() {
		return false
	}

	return true
}

// FilterPublicAddrs 过滤出公网地址
func FilterPublicAddrs(addrs []net.Addr) []net.Addr {
	var public []net.Addr
	for _, addr := range addrs {
		if IsPublicAddr(addr) {
			public = append(public, addr)
		}
	}
	return public
}

