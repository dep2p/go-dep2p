// Package client 提供中继客户端实现
package client

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dep2p/go-dep2p/internal/core/address"
	endpointif "github.com/dep2p/go-dep2p/pkg/interfaces/endpoint"
	natif "github.com/dep2p/go-dep2p/pkg/interfaces/nat"
	"github.com/dep2p/go-dep2p/pkg/protocolids"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              协议定义
// ============================================================================

// 引用 pkg/protocolids 唯一真源
var (
	// ProtocolAddrExchange 地址交换协议 (v1.1 scope: sys)
	ProtocolAddrExchange = protocolids.SysAddrExchange
)

// 消息类型
const (
	MsgTypeAddrs    uint8 = 1 // 地址列表
	MsgTypeAck      uint8 = 2 // 确认
	MsgTypeUpgraded uint8 = 3 // 升级成功通知
)

// 升级状态
const (
	UpgradeStatePending   = iota // 待处理
	UpgradeStateExchanged        // 已交换地址
	UpgradeStatePunching         // 正在打洞
	UpgradeStateSuccess          // 升级成功
	UpgradeStateFailed           // 升级失败
)

// MaxAddressCount 最大地址数量，防止内存耗尽攻击
const MaxAddressCount = 100

// ============================================================================
//                              错误定义
// ============================================================================

// 连接升级相关错误
var (
	// ErrUpgradeInProgress 升级已在进行中
	ErrUpgradeInProgress = errors.New("upgrade already in progress")
	ErrNoPuncher         = errors.New("hole puncher not available")
	ErrNoAddresses       = errors.New("no addresses to exchange")
	ErrUpgradeFailed     = errors.New("upgrade failed")
	ErrAlreadyDirect     = errors.New("connection is already direct")
)

// ============================================================================
//                              配置
// ============================================================================

// UpgraderConfig 升级器配置
type UpgraderConfig struct {
	// HolePunchTimeout 打洞超时
	HolePunchTimeout time.Duration

	// AddrExchangeTimeout 地址交换超时
	AddrExchangeTimeout time.Duration

	// RetryInterval 重试间隔
	RetryInterval time.Duration

	// MaxRetries 最大重试次数
	MaxRetries int

	// EnableAutoUpgrade 自动升级
	EnableAutoUpgrade bool
}

// DefaultUpgraderConfig 默认配置
func DefaultUpgraderConfig() UpgraderConfig {
	return UpgraderConfig{
		HolePunchTimeout:    10 * time.Second,
		AddrExchangeTimeout: 5 * time.Second,
		RetryInterval:       5 * time.Minute,
		MaxRetries:          3,
		EnableAutoUpgrade:   true,
	}
}

// ============================================================================
//                              ConnectionUpgrader 实现
// ============================================================================

// ConnectionUpgrader 连接升级器
//
// 用于将中继连接升级到直连：
// 1. 通过中继连接交换地址信息
// 2. 并行尝试打洞
// 3. 成功后迁移到直连
type ConnectionUpgrader struct {
	config     UpgraderConfig
	puncher    natif.HolePuncher
	endpoint   endpointif.Endpoint
	localAddrs func() []endpointif.Address

	// 升级会话
	sessions   map[types.NodeID]*upgradeSession
	sessionsMu sync.RWMutex

	// 回调
	onUpgraded func(types.NodeID, endpointif.Address)

	// 状态
	running int32
	ctx     context.Context
	cancel  context.CancelFunc
}

// upgradeSession 升级会话
type upgradeSession struct {
	remoteID    types.NodeID
	relayConn   endpointif.Connection // 中继连接
	localAddrs  []endpointif.Address  // 本地地址
	remoteAddrs []endpointif.Address  // 对方地址
	state       int32           // 升级状态
	retryCount  int             // 重试次数
	lastAttempt time.Time       // 上次尝试时间
	startTime   time.Time       // 开始时间
	done        chan struct{}   // 完成通道
	successAddr endpointif.Address    // 成功地址
	err         error           // 错误
}

// NewConnectionUpgrader 创建连接升级器
func NewConnectionUpgrader(
	config UpgraderConfig,
	puncher natif.HolePuncher,
	endpoint endpointif.Endpoint,
	localAddrs func() []endpointif.Address,
) *ConnectionUpgrader {
	return &ConnectionUpgrader{
		config:     config,
		puncher:    puncher,
		endpoint:   endpoint,
		localAddrs: localAddrs,
		sessions:   make(map[types.NodeID]*upgradeSession),
	}
}

// ============================================================================
//                              生命周期
// ============================================================================

// Start 启动升级器
func (u *ConnectionUpgrader) Start(ctx context.Context) error {
	if !atomic.CompareAndSwapInt32(&u.running, 0, 1) {
		return nil
	}

	u.ctx, u.cancel = context.WithCancel(ctx)

	// 启动定期重试任务
	go u.retryLoop()

	log.Info("ConnectionUpgrader 已启动")
	return nil
}

// Stop 停止升级器
func (u *ConnectionUpgrader) Stop() error {
	if !atomic.CompareAndSwapInt32(&u.running, 1, 0) {
		return nil
	}

	if u.cancel != nil {
		u.cancel()
	}

	// 安全取消所有进行中的会话
	u.sessionsMu.Lock()
	for _, session := range u.sessions {
		// 使用 select 安全关闭通道，避免重复关闭导致 panic
		select {
		case <-session.done:
			// 通道已关闭，跳过
		default:
			close(session.done)
		}
	}
	u.sessions = make(map[types.NodeID]*upgradeSession)
	u.sessionsMu.Unlock()

	log.Info("ConnectionUpgrader 已停止")
	return nil
}

// ============================================================================
//                              公开接口
// ============================================================================

// TryUpgrade 尝试升级连接
//
// 流程：
// 1. 通过中继连接交换地址
// 2. 并行尝试打洞
// 3. 成功后返回直连地址
func (u *ConnectionUpgrader) TryUpgrade(ctx context.Context, remoteID types.NodeID, relayConn endpointif.Connection) (endpointif.Address, error) {
	// 检查是否已有进行中的升级
	u.sessionsMu.RLock()
	if session, exists := u.sessions[remoteID]; exists {
		u.sessionsMu.RUnlock()
		// 等待现有会话完成
		select {
		case <-session.done:
			if session.successAddr != nil {
				return session.successAddr, nil
			}
			return nil, session.err
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	u.sessionsMu.RUnlock()

	// 创建新会话
	session := &upgradeSession{
		remoteID:  remoteID,
		relayConn: relayConn,
		startTime: time.Now(),
		done:      make(chan struct{}),
	}

	u.sessionsMu.Lock()
	u.sessions[remoteID] = session
	u.sessionsMu.Unlock()

	defer func() {
		u.sessionsMu.Lock()
		delete(u.sessions, remoteID)
		u.sessionsMu.Unlock()
	}()

	log.Info("开始连接升级",
		"remoteID", remoteID.ShortString())

	// 步骤 1: 交换地址
	if err := u.exchangeAddresses(ctx, session); err != nil {
		session.err = fmt.Errorf("address exchange failed: %w", err)
		close(session.done)
		return nil, session.err
	}

	// 步骤 2: 打洞
	addr, err := u.holePunch(ctx, session)
	if err != nil {
		session.err = fmt.Errorf("hole punch failed: %w", err)
		close(session.done)
		return nil, session.err
	}

	// 步骤 3: 通知成功
	session.successAddr = addr
	close(session.done)

	// 触发回调
	if u.onUpgraded != nil {
		u.onUpgraded(remoteID, addr)
	}

	log.Info("连接升级成功",
		"remoteID", remoteID.ShortString(),
		"directAddr", addr.String(),
		"duration", time.Since(session.startTime))

	return addr, nil
}

// OnUpgraded 设置升级成功回调
func (u *ConnectionUpgrader) OnUpgraded(callback func(types.NodeID, endpointif.Address)) {
	u.onUpgraded = callback
}

// ============================================================================
//                              地址交换
// ============================================================================

// exchangeAddresses 交换地址
func (u *ConnectionUpgrader) exchangeAddresses(ctx context.Context, session *upgradeSession) error {
	atomic.StoreInt32(&session.state, UpgradeStatePending)

	// 获取本地地址
	if u.localAddrs != nil {
		session.localAddrs = u.localAddrs()
	}

	if len(session.localAddrs) == 0 {
		return ErrNoAddresses
	}

	// 打开地址交换流
	exchCtx, cancel := context.WithTimeout(ctx, u.config.AddrExchangeTimeout)
	defer cancel()

	stream, err := session.relayConn.OpenStream(exchCtx, ProtocolAddrExchange)
	if err != nil {
		return fmt.Errorf("open stream failed: %w", err)
	}
	defer func() { _ = stream.Close() }()

	// 发送本地地址
	if err := u.sendAddresses(stream, session.localAddrs); err != nil {
		return fmt.Errorf("send addresses failed: %w", err)
	}

	// 接收对方地址
	remoteAddrs, err := u.receiveAddresses(stream)
	if err != nil {
		return fmt.Errorf("receive addresses failed: %w", err)
	}

	session.remoteAddrs = remoteAddrs
	atomic.StoreInt32(&session.state, UpgradeStateExchanged)

	log.Debug("地址交换完成",
		"remoteID", session.remoteID.ShortString(),
		"localAddrs", len(session.localAddrs),
		"remoteAddrs", len(remoteAddrs))

	return nil
}

// sendAddresses 发送地址列表
func (u *ConnectionUpgrader) sendAddresses(stream endpointif.Stream, addrs []endpointif.Address) error {
	// 消息格式: [type:1][count:2][addr1_len:2][addr1]...[addrN_len:2][addrN]
	buf := make([]byte, 1+2)
	buf[0] = MsgTypeAddrs
	binary.BigEndian.PutUint16(buf[1:3], uint16(len(addrs))) //nolint:gosec // G115: 地址数量由协议限制

	if _, err := stream.Write(buf); err != nil {
		return err
	}

	for _, addr := range addrs {
		addrBytes := []byte(addr.String())
		lenBuf := make([]byte, 2)
		binary.BigEndian.PutUint16(lenBuf, uint16(len(addrBytes))) //nolint:gosec // G115: 地址长度由协议限制

		if _, err := stream.Write(lenBuf); err != nil {
			return err
		}
		if _, err := stream.Write(addrBytes); err != nil {
			return err
		}
	}

	return nil
}

// receiveAddresses 接收地址列表
func (u *ConnectionUpgrader) receiveAddresses(stream endpointif.Stream) ([]endpointif.Address, error) {
	// 读取消息头
	header := make([]byte, 3)
	if _, err := io.ReadFull(stream, header); err != nil {
		return nil, err
	}

	if header[0] != MsgTypeAddrs {
		return nil, fmt.Errorf("unexpected message type: %d", header[0])
	}

	count := binary.BigEndian.Uint16(header[1:3])

	// 防止内存耗尽攻击：限制最大地址数量
	if count > MaxAddressCount {
		return nil, fmt.Errorf("too many addresses: %d > %d", count, MaxAddressCount)
	}

	addrs := make([]endpointif.Address, 0, count)

	for i := uint16(0); i < count; i++ {
		lenBuf := make([]byte, 2)
		if _, err := io.ReadFull(stream, lenBuf); err != nil {
			return nil, err
		}

		addrLen := binary.BigEndian.Uint16(lenBuf)
		addrBytes := make([]byte, addrLen)
		if _, err := io.ReadFull(stream, addrBytes); err != nil {
			return nil, err
		}

		addrs = append(addrs, address.NewAddr(types.Multiaddr(string(addrBytes))))
	}

	return addrs, nil
}

// ============================================================================
//                              打洞
// ============================================================================

// holePunch 尝试打洞
func (u *ConnectionUpgrader) holePunch(ctx context.Context, session *upgradeSession) (endpointif.Address, error) {
	if u.puncher == nil {
		return nil, ErrNoPuncher
	}

	if len(session.remoteAddrs) == 0 {
		return nil, ErrNoAddresses
	}

	atomic.StoreInt32(&session.state, UpgradeStatePunching)

	punchCtx, cancel := context.WithTimeout(ctx, u.config.HolePunchTimeout)
	defer cancel()

	addr, err := u.puncher.Punch(punchCtx, session.remoteID, session.remoteAddrs)
	if err != nil {
		atomic.StoreInt32(&session.state, UpgradeStateFailed)
		return nil, err
	}

	atomic.StoreInt32(&session.state, UpgradeStateSuccess)
	return addr, nil
}

// ============================================================================
//                              定期重试
// ============================================================================

// retryLoop 定期重试循环
func (u *ConnectionUpgrader) retryLoop() {
	// 检查上下文是否有效
	if u.ctx == nil {
		return
	}

	ticker := time.NewTicker(u.config.RetryInterval)
	defer ticker.Stop()

	for {
		select {
		case <-u.ctx.Done():
			return
		case <-ticker.C:
			if atomic.LoadInt32(&u.running) == 0 {
				return
			}
			u.retryFailedSessions()
		}
	}
}

// retryFailedSessions 重试失败的会话
func (u *ConnectionUpgrader) retryFailedSessions() {
	// 检查运行状态和上下文
	if atomic.LoadInt32(&u.running) == 0 || u.ctx == nil {
		return
	}

	u.sessionsMu.RLock()
	var toRetry []*upgradeSession
	for _, session := range u.sessions {
		state := atomic.LoadInt32(&session.state)
		if state == UpgradeStateFailed && session.retryCount < u.config.MaxRetries {
			if time.Since(session.lastAttempt) >= u.config.RetryInterval {
				toRetry = append(toRetry, session)
			}
		}
	}
	u.sessionsMu.RUnlock()

	for _, session := range toRetry {
		go func(s *upgradeSession) {
			// 再次检查上下文有效性
			if u.ctx == nil {
				return
			}
			s.retryCount++
			s.lastAttempt = time.Now()

			log.Debug("重试连接升级",
				"remoteID", s.remoteID.ShortString(),
				"retry", s.retryCount)

			addr, err := u.holePunch(u.ctx, s)
			if err == nil {
				s.successAddr = addr
				atomic.StoreInt32(&s.state, UpgradeStateSuccess)

				if u.onUpgraded != nil {
					u.onUpgraded(s.remoteID, addr)
				}
			}
		}(session)
	}
}

// stringAddress 已删除，统一使用 address.Addr

