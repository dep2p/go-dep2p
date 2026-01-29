package member

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dep2p/go-dep2p/internal/realm/interfaces"
	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/protocol"
)

// HeartbeatProtocol 心跳协议 ID（使用统一定义）
// 用于 Realm 成员存活检测
var HeartbeatProtocol = string(protocol.Heartbeat)

// HeartbeatMessage 心跳消息
type HeartbeatMessage struct {
	// Timestamp 发送时间戳（纳秒）
	Timestamp int64
	// RealmID Realm 标识
	RealmID string
	// Nonce 随机数（用于防止重放）
	Nonce uint64
}

// ============================================================================
//                              心跳监控
// ============================================================================

// HeartbeatMonitor 心跳监控器
type HeartbeatMonitor struct {
	mu sync.RWMutex

	// 配置
	manager    *Manager
	host       pkgif.Host
	interval   time.Duration
	timeout    time.Duration
	maxRetries int

	// 状态跟踪
	lastHeartbeat map[string]time.Time
	failedCount   map[string]int

	// 控制
	ctx     context.Context
	cancel  context.CancelFunc
	started atomic.Bool
	ticker  *time.Ticker
}

// NewHeartbeatMonitor 创建心跳监控器
func NewHeartbeatMonitor(
	manager *Manager,
	host pkgif.Host,
	interval time.Duration,
	maxRetries int,
) *HeartbeatMonitor {
	if interval <= 0 {
		interval = 15 * time.Second
	}

	if maxRetries <= 0 {
		maxRetries = 3
	}

	return &HeartbeatMonitor{
		manager:       manager,
		host:          host,
		interval:      interval,
		timeout:       interval * time.Duration(maxRetries),
		maxRetries:    maxRetries,
		lastHeartbeat: make(map[string]time.Time),
		failedCount:   make(map[string]int),
	}
}

// Start 启动心跳监控
func (m *HeartbeatMonitor) Start(_ context.Context) error {
	if m.started.Load() {
		return ErrAlreadyStarted
	}

	// 使用 context.Background() 保证后台循环不受上层 ctx 取消的影响
	m.ctx, m.cancel = context.WithCancel(context.Background())
	m.started.Store(true)

	// 注册心跳协议处理器
	m.RegisterHeartbeatHandler()

	// 启动心跳循环
	m.ticker = time.NewTicker(m.interval)
	go m.heartbeatLoop()

	return nil
}

// Stop 停止心跳监控
func (m *HeartbeatMonitor) Stop(_ context.Context) error {
	if !m.started.Load() {
		return ErrNotStarted
	}

	m.started.Store(false)

	if m.cancel != nil {
		m.cancel()
	}

	if m.ticker != nil {
		m.ticker.Stop()
	}

	// 移除心跳协议处理器
	if m.host != nil {
		m.host.RemoveStreamHandler(HeartbeatProtocol)
	}

	return nil
}

// SendHeartbeat 发送心跳
//
// 心跳协议流程：
//  1. 建立流连接
//  2. 发送心跳消息（时间戳 + RealmID + Nonce）
//  3. 等待 ACK 响应
//  4. 更新本地心跳状态
func (m *HeartbeatMonitor) SendHeartbeat(ctx context.Context, peerID string) error {
	if m.host == nil {
		return fmt.Errorf("host is nil, cannot send heartbeat")
	}

	// 1. 建立流连接
	stream, err := m.host.NewStream(ctx, peerID, HeartbeatProtocol)
	if err != nil {
		return fmt.Errorf("failed to open heartbeat stream: %w", err)
	}
	defer stream.Close()

	// 2. 构造心跳消息
	now := time.Now()
	msg := HeartbeatMessage{
		Timestamp: now.UnixNano(),
		RealmID:   m.manager.realmID,
		Nonce:     uint64(now.UnixNano()) ^ 0xDEADBEEF,
	}

	// 3. 发送心跳消息
	if err := m.writeHeartbeat(stream, &msg); err != nil {
		return fmt.Errorf("failed to send heartbeat: %w", err)
	}

	// 4. 等待 ACK（读取回复的时间戳）
	ackBuf := make([]byte, 8)
	if _, err := io.ReadFull(stream, ackBuf); err != nil {
		return fmt.Errorf("failed to receive heartbeat ack: %w", err)
	}

	// 5. 更新本地心跳状态
	m.mu.Lock()
	m.lastHeartbeat[peerID] = now
	m.failedCount[peerID] = 0
	m.mu.Unlock()

	return nil
}

// writeHeartbeat 写入心跳消息
func (m *HeartbeatMonitor) writeHeartbeat(w io.Writer, msg *HeartbeatMessage) error {
	// 写入时间戳（8字节）
	if err := binary.Write(w, binary.BigEndian, msg.Timestamp); err != nil {
		return err
	}

	// 写入 RealmID 长度和内容
	realmIDBytes := []byte(msg.RealmID)
	if err := binary.Write(w, binary.BigEndian, uint16(len(realmIDBytes))); err != nil {
		return err
	}
	if _, err := w.Write(realmIDBytes); err != nil {
		return err
	}

	// 写入 Nonce（8字节）
	if err := binary.Write(w, binary.BigEndian, msg.Nonce); err != nil {
		return err
	}

	return nil
}

// readHeartbeat 读取心跳消息
func (m *HeartbeatMonitor) readHeartbeat(r io.Reader) (*HeartbeatMessage, error) {
	msg := &HeartbeatMessage{}

	// 读取时间戳
	if err := binary.Read(r, binary.BigEndian, &msg.Timestamp); err != nil {
		return nil, err
	}

	// 读取 RealmID 长度和内容
	var realmIDLen uint16
	if err := binary.Read(r, binary.BigEndian, &realmIDLen); err != nil {
		return nil, err
	}
	realmIDBytes := make([]byte, realmIDLen)
	if _, err := io.ReadFull(r, realmIDBytes); err != nil {
		return nil, err
	}
	msg.RealmID = string(realmIDBytes)

	// 读取 Nonce
	if err := binary.Read(r, binary.BigEndian, &msg.Nonce); err != nil {
		return nil, err
	}

	return msg, nil
}

// HandleHeartbeatStream 处理心跳请求（作为服务端）
func (m *HeartbeatMonitor) HandleHeartbeatStream(stream pkgif.Stream) {
	defer stream.Close()

	// 1. 读取心跳消息
	msg, err := m.readHeartbeat(stream)
	if err != nil {
		return
	}

	// 2. 验证 RealmID
	if msg.RealmID != m.manager.realmID {
		return // Realm 不匹配，忽略
	}

	// 3. 更新成员最后活跃时间
	remotePeerID := stream.Conn().RemotePeer()
	if m.manager != nil {
		ctx := context.Background()
		m.manager.UpdateLastSeen(ctx, string(remotePeerID))
	}

	// 4. 发送 ACK（当前时间戳）
	ackBuf := make([]byte, 8)
	binary.BigEndian.PutUint64(ackBuf, uint64(time.Now().UnixNano()))
	stream.Write(ackBuf)
}

// RegisterHeartbeatHandler 注册心跳协议处理器
func (m *HeartbeatMonitor) RegisterHeartbeatHandler() {
	if m.host != nil {
		m.host.SetStreamHandler(HeartbeatProtocol, m.HandleHeartbeatStream)
	}
}

// ReceiveHeartbeat 接收心跳
func (m *HeartbeatMonitor) ReceiveHeartbeat(ctx context.Context, peerID string) error {
	// 更新成员的最后活跃时间
	if m.manager != nil {
		return m.manager.UpdateLastSeen(ctx, peerID)
	}

	return nil
}

// GetStatus 获取成员状态
func (m *HeartbeatMonitor) GetStatus(peerID string) (bool, error) {
	m.mu.RLock()
	lastTime, ok := m.lastHeartbeat[peerID]
	m.mu.RUnlock()

	if !ok {
		// 没有心跳记录，检查管理器中的状态
		if m.manager != nil {
			ctx := context.Background()
			member, err := m.manager.Get(ctx, peerID)
			if err != nil {
				return false, err
			}
			return member.Online, nil
		}
		return false, ErrMemberNotFound
	}

	// 检查是否超时
	if time.Since(lastTime) > m.timeout {
		return false, nil
	}

	return true, nil
}

// heartbeatLoop 心跳循环
func (m *HeartbeatMonitor) heartbeatLoop() {
	for {
		select {
		case <-m.ticker.C:
			m.checkHeartbeats()

		case <-m.ctx.Done():
			return
		}
	}
}

// checkHeartbeats 检查所有成员的心跳状态
func (m *HeartbeatMonitor) checkHeartbeats() {
	if m.manager == nil {
		return
	}

	ctx := context.Background()

	// 获取所有在线成员
	members, err := m.manager.List(ctx, &interfaces.ListOptions{
		OnlineOnly: true,
	})
	if err != nil {
		return
	}

	now := time.Now()

	for _, memberInfo := range members {
		m.mu.RLock()
		lastTime, ok := m.lastHeartbeat[memberInfo.PeerID]
		failCount := m.failedCount[memberInfo.PeerID]
		m.mu.RUnlock()

		if !ok {
			// 首次检查，记录当前时间
			m.mu.Lock()
			m.lastHeartbeat[memberInfo.PeerID] = now
			m.mu.Unlock()
			continue
		}

		// 检查是否超时
		if now.Sub(lastTime) > m.interval {
			// 尝试发送心跳
			if err := m.SendHeartbeat(ctx, memberInfo.PeerID); err != nil {
				// 心跳失败，增加失败计数
				m.mu.Lock()
				m.failedCount[memberInfo.PeerID] = failCount + 1
				m.mu.Unlock()

				// 超过最大重试次数，标记为离线
				if failCount+1 >= m.maxRetries {
					m.manager.UpdateStatus(ctx, memberInfo.PeerID, false)
				}
			}
		}
	}
}

// 确保实现接口
var _ interfaces.HeartbeatMonitor = (*HeartbeatMonitor)(nil)
