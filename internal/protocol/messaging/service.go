// Package messaging 实现点对点消息传递协议
package messaging

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/lib/log"
	"github.com/dep2p/go-dep2p/pkg/types"
	"github.com/google/uuid"
)

var logger = log.Logger("protocol/messaging")

// Service 实现 Messaging 接口
type Service struct {
	host     interfaces.Host
	realmMgr interfaces.RealmManager // 可选，用于全局模式
	realm    interfaces.Realm        // 可选，用于 Realm 绑定模式
	realmID  string                  // 绑定的 RealmID（如果有）
	codec    *Codec
	handlers *HandlerRegistry

	mu      sync.RWMutex
	started bool
	ctx     context.Context
	cancel  context.CancelFunc

	// 配置
	config *Config

	// msgrate 集成：ConnManager 用于更新消息速率
	connMgr interfaces.ConnManager
}

// 确保 Service 实现了 interfaces.Messaging 接口
var _ interfaces.Messaging = (*Service)(nil)

// New 创建 Messaging 服务（全局模式，使用 RealmManager）
func New(host interfaces.Host, realmMgr interfaces.RealmManager, opts ...Option) (*Service, error) {
	if host == nil {
		return nil, ErrNilHost
	}
	// RealmManager 现在是可选的，某些功能在没有它时不可用
	// 如果 realmMgr == nil，Realm 相关的消息功能将被禁用

	// 应用配置
	config := DefaultConfig()
	for _, opt := range opts {
		opt(config)
	}

	s := &Service{
		host:     host,
		realmMgr: realmMgr, // 可能为 nil
		codec:    NewCodec(),
		handlers: NewHandlerRegistry(),
		config:   config,
	}

	return s, nil
}

// NewForRealm 创建绑定到特定 Realm 的 Messaging 服务
//
// 与 New 不同，此构造函数将服务绑定到特定的 Realm：
// - 协议 ID 包含 RealmID: /dep2p/app/<realmID>/messaging/1.0.0
// - 只处理该 Realm 的消息
// - 成员验证基于绑定的 Realm
func NewForRealm(host interfaces.Host, realm interfaces.Realm, opts ...Option) (*Service, error) {
	if host == nil {
		return nil, ErrNilHost
	}
	if realm == nil {
		return nil, fmt.Errorf("realm is required for NewForRealm")
	}

	// 应用配置
	config := DefaultConfig()
	for _, opt := range opts {
		opt(config)
	}

	s := &Service{
		host:     host,
		realm:    realm,
		realmID:  realm.ID(),
		codec:    NewCodec(),
		handlers: NewHandlerRegistry(),
		config:   config,
	}

	return s, nil
}

// Start 启动服务
func (s *Service) Start(_ context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.started {
		return ErrAlreadyStarted
	}

	logger.Info("正在启动 Messaging 服务")

	// 使用 context.Background() 而不是传入的 ctx
	// 因为 Fx OnStart 的 ctx 在返回后会被取消，导致后台循环提前退出
	s.ctx, s.cancel = context.WithCancel(context.Background())
	s.started = true

	logger.Info("Messaging 服务启动成功")
	return nil
}

// Stop 停止服务
func (s *Service) Stop(_ context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.started {
		return ErrNotStarted
	}

	logger.Info("正在停止 Messaging 服务")

	if s.cancel != nil {
		s.cancel()
	}

	s.started = false
	logger.Info("Messaging 服务已停止")
	return nil
}

// Send 发送消息并等待响应
func (s *Service) Send(ctx context.Context, peerID, protocol string, data []byte) ([]byte, error) {
	s.mu.RLock()
	if !s.started {
		s.mu.RUnlock()
		return nil, ErrNotStarted
	}
	s.mu.RUnlock()

	// 验证协议
	if err := validateProtocol(protocol); err != nil {
		return nil, err
	}

	// 验证 Realm 成员资格 (通过检查所有 Realm)
	if !s.isRealmMember(peerID) {
		logger.Warn("发送消息失败：非 Realm 成员", "peerID", log.TruncateID(peerID, 8), "protocol", protocol)
		return nil, fmt.Errorf("%w: peer %s", ErrNotRealmMember, peerID)
	}

	logger.Debug("发送消息", "peerID", log.TruncateID(peerID, 8), "protocol", protocol, "dataSize", len(data))

	// 构造请求
	req := &interfaces.Request{
		ID:        uuid.New().String(),
		From:      s.host.ID(),
		Protocol:  protocol,
		Data:      data,
		Timestamp: time.Now(),
		Metadata:  make(map[string]string),
	}

	// 发送请求并等待响应
	resp, err := s.sendRequest(ctx, peerID, protocol, req)
	if err != nil {
		logger.Warn("发送消息失败", "peerID", log.TruncateID(peerID, 8), "protocol", protocol, "error", err)
		return nil, err
	}

	// 检查响应错误
	if resp.Error != nil {
		logger.Warn("消息响应错误", "peerID", log.TruncateID(peerID, 8), "protocol", protocol, "error", resp.Error)
		return nil, resp.Error
	}

	logger.Debug("消息发送成功", "peerID", log.TruncateID(peerID, 8), "protocol", protocol)
	return resp.Data, nil
}

// SendAsync 异步发送消息
func (s *Service) SendAsync(ctx context.Context, peerID, protocol string, data []byte) (<-chan *interfaces.Response, error) {
	s.mu.RLock()
	if !s.started {
		s.mu.RUnlock()
		return nil, ErrNotStarted
	}
	s.mu.RUnlock()

	// 验证协议
	if err := validateProtocol(protocol); err != nil {
		return nil, err
	}

	// 验证 Realm 成员资格
	if !s.isRealmMember(peerID) {
		return nil, fmt.Errorf("%w: peer %s", ErrNotRealmMember, peerID)
	}

	// 创建响应 channel
	respChan := make(chan *interfaces.Response, 1)

	// 构造请求
	req := &interfaces.Request{
		ID:        uuid.New().String(),
		From:      s.host.ID(),
		Protocol:  protocol,
		Data:      data,
		Timestamp: time.Now(),
		Metadata:  make(map[string]string),
	}

	// 启动 goroutine 执行发送
	go func() {
		defer close(respChan)

		resp, err := s.sendRequest(ctx, peerID, protocol, req)
		if err != nil {
			// 构造错误响应
			respChan <- &interfaces.Response{
				ID:        req.ID,
				From:      peerID,
				Error:     err,
				Timestamp: time.Now(),
			}
			return
		}

		respChan <- resp
	}()

	return respChan, nil
}

// RegisterHandler 注册消息处理器
func (s *Service) RegisterHandler(protocol string, handler interfaces.MessageHandler) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// 验证协议
	if err := validateProtocol(protocol); err != nil {
		return err
	}

	// 注册到本地注册表
	if err := s.handlers.Register(protocol, handler); err != nil {
		return err
	}

	// 根据模式注册到 Host
	if s.realm != nil && s.realmID != "" {
		// Realm-bound 模式：只为绑定的 Realm 注册
		protocolID := buildProtocolID(s.realmID, protocol)
		s.host.SetStreamHandler(string(protocolID), s.createStreamHandler(protocol, handler))
	} else if s.realmMgr != nil {
		// 全局模式：为每个 Realm 注册到 Host
		realms := s.realmMgr.ListRealms()
		for _, realm := range realms {
			protocolID := buildProtocolID(realm.ID(), protocol)
			s.host.SetStreamHandler(string(protocolID), s.createStreamHandler(protocol, handler))
		}
	}

	return nil
}

// UnregisterHandler 注销消息处理器
func (s *Service) UnregisterHandler(protocol string) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// 从本地注册表注销
	if err := s.handlers.Unregister(protocol); err != nil {
		return err
	}

	// 根据模式从 Host 注销
	if s.realm != nil && s.realmID != "" {
		// Realm-bound 模式：只注销绑定的 Realm 的处理器
		protocolID := buildProtocolID(s.realmID, protocol)
		s.host.RemoveStreamHandler(string(protocolID))
	} else if s.realmMgr != nil {
		// 全局模式：从 Host 注销所有 Realm 的处理器
		realms := s.realmMgr.ListRealms()
		for _, realm := range realms {
			protocolID := buildProtocolID(realm.ID(), protocol)
			s.host.RemoveStreamHandler(string(protocolID))
		}
	}

	return nil
}

// Close 关闭服务
func (s *Service) Close() error {
	return s.Stop(context.Background())
}

// sendRequest 发送请求并等待响应
func (s *Service) sendRequest(ctx context.Context, peerID, protocol string, req *interfaces.Request) (*interfaces.Response, error) {
	// 应用超时
	if s.config.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, s.config.Timeout)
		defer cancel()
	}

	// 尝试发送(带重试)
	var lastErr error
	for attempt := 0; attempt <= s.config.MaxRetries; attempt++ {
		if attempt > 0 {
			// 等待重试延迟
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(s.config.RetryDelay):
			}
		}

		// 尝试发送
		resp, err := s.trySendRequest(ctx, peerID, protocol, req)
		if err == nil {
			return resp, nil
		}

		lastErr = err

		// 检查是否应该重试
		if !shouldRetry(err) {
			break
		}
	}

	return nil, lastErr
}

// MsgKindMessaging 消息类型标识（用于 msgrate）
const MsgKindMessaging uint64 = 1

// trySendRequest 尝试发送请求
func (s *Service) trySendRequest(ctx context.Context, peerID, protocol string, req *interfaces.Request) (*interfaces.Response, error) {
	// 获取 Realm
	realm, err := s.findRealmForPeer(peerID)
	if err != nil {
		return nil, err
	}

	// 构造协议 ID
	protocolID := buildProtocolID(realm.ID(), protocol)

	// 记录开始时间（用于 msgrate）
	startTime := time.Now()

	// P0 修复：确保连接存在，如果没有则尝试自动拨号
	if err := s.ensureConnected(ctx, peerID); err != nil {
		s.updatePeerRate(peerID, startTime, 0)
		return nil, fmt.Errorf("failed to ensure connection: %w", err)
	}

	// 打开流
	stream, err := s.host.NewStream(ctx, peerID, string(protocolID))
	if err != nil {
		// msgrate：更新失败的速率测量
		s.updatePeerRate(peerID, startTime, 0)
		return nil, fmt.Errorf("failed to open stream: %w", err)
	}
	defer stream.Close()

	// 发送请求
	if err := s.codec.WriteRequest(stream, req); err != nil {
		s.updatePeerRate(peerID, startTime, 0)
		return nil, fmt.Errorf("failed to write request: %w", err)
	}

	// 读取响应
	resp, err := s.codec.ReadResponse(stream)
	if err != nil {
		s.updatePeerRate(peerID, startTime, 0)
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// 计算延迟
	resp.Latency = time.Since(req.Timestamp)

	// msgrate：更新成功的速率测量
	s.updatePeerRate(peerID, startTime, 1)

	return resp, nil
}

// SetConnManager 设置连接管理器（用于 msgrate 集成）
func (s *Service) SetConnManager(connMgr interfaces.ConnManager) {
	s.connMgr = connMgr
}

// ensureConnected 确保与目标节点的连接存在
//
// P0 修复：如果没有连接，尝试从 Peerstore 获取地址并自动拨号。
// 这解决了"no connection to peer"错误导致私聊失败的问题。
func (s *Service) ensureConnected(ctx context.Context, peerID string) error {
	// 检查当前连接状态
	network := s.host.Network()
	if network == nil {
		return fmt.Errorf("network not available")
	}

	// 如果已连接，直接返回
	if network.Connectedness(peerID) == interfaces.Connected {
		return nil
	}

	logger.Debug("节点未连接，尝试自动拨号", "peerID", log.TruncateID(peerID, 8))

	// 从 Peerstore 获取地址
	peerstore := s.host.Peerstore()
	if peerstore == nil {
		return fmt.Errorf("peerstore not available")
	}

	addrs := peerstore.Addrs(types.PeerID(peerID))
	if len(addrs) == 0 {
		logger.Debug("Peerstore 中无地址，无法自动拨号", "peerID", log.TruncateID(peerID, 8))
		return fmt.Errorf("no addresses available for peer %s", peerID[:8])
	}

	// 转换地址为字符串
	addrStrs := make([]string, len(addrs))
	for i, addr := range addrs {
		addrStrs[i] = addr.String()
	}

	logger.Debug("尝试自动拨号",
		"peerID", log.TruncateID(peerID, 8),
		"addrs", len(addrStrs))

	// 尝试连接
	if err := s.host.Connect(ctx, peerID, addrStrs); err != nil {
		logger.Debug("自动拨号失败",
			"peerID", log.TruncateID(peerID, 8),
			"error", err)
		return fmt.Errorf("auto-dial failed: %w", err)
	}

	logger.Info("自动拨号成功", "peerID", log.TruncateID(peerID, 8))
	return nil
}

// updatePeerRate 更新节点消息速率（msgrate 集成）
func (s *Service) updatePeerRate(peerID string, startTime time.Time, items int) {
	if s.connMgr == nil {
		return
	}
	elapsed := time.Since(startTime)
	s.connMgr.UpdatePeerRate(peerID, MsgKindMessaging, elapsed, items)
}

// createStreamHandler 创建流处理器
func (s *Service) createStreamHandler(protocol string, handler interfaces.MessageHandler) interfaces.StreamHandler {
	return func(stream interfaces.Stream) {
		defer stream.Close()

		// 读取请求
		req, err := s.codec.ReadRequest(stream)
		if err != nil {
			// 无法读取请求,直接返回
			return
		}

		// 设置协议
		req.Protocol = protocol

		// 创建上下文
		ctx, cancel := context.WithTimeout(s.ctx, s.config.Timeout)
		defer cancel()

		// 调用处理器
		resp, err := handler(ctx, req)
		if err != nil {
			// 构造错误响应
			resp = &interfaces.Response{
				ID:        req.ID,
				From:      s.host.ID(),
				Error:     err,
				Timestamp: time.Now(),
			}
		} else if resp == nil {
			// 构造空响应
			resp = &interfaces.Response{
				ID:        req.ID,
				From:      s.host.ID(),
				Data:      []byte{},
				Timestamp: time.Now(),
			}
		}

		// 确保响应 ID 和 From 正确
		resp.ID = req.ID
		resp.From = s.host.ID()
		resp.Timestamp = time.Now()

		// 发送响应
		if err := s.codec.WriteResponse(stream, resp); err != nil {
			// 忽略写入错误
			return
		}
	}
}

// isRealmMember 检查节点是否是 Realm 的成员
func (s *Service) isRealmMember(peerID string) bool {
	// Realm-bound 模式：检查绑定的 Realm
	if s.realm != nil {
		return s.realm.IsMember(peerID)
	}

	// 全局模式：检查所有 Realm
	if s.realmMgr == nil {
		return false
	}

	realms := s.realmMgr.ListRealms()
	for _, realm := range realms {
		if realm.IsMember(peerID) {
			return true
		}
	}
	return false
}

// findRealmForPeer 查找节点所在的 Realm
func (s *Service) findRealmForPeer(peerID string) (interfaces.Realm, error) {
	// Realm-bound 模式：直接返回绑定的 Realm（如果 peer 是成员）
	if s.realm != nil {
		if s.realm.IsMember(peerID) {
			return s.realm, nil
		}
		return nil, fmt.Errorf("%w: peer %s", ErrNotRealmMember, peerID)
	}

	// 全局模式：从 RealmManager 查找
	if s.realmMgr == nil {
		return nil, fmt.Errorf("%w: peer %s (no realm manager)", ErrNotRealmMember, peerID)
	}

	realms := s.realmMgr.ListRealms()
	for _, realm := range realms {
		if realm.IsMember(peerID) {
			return realm, nil
		}
	}
	return nil, fmt.Errorf("%w: peer %s", ErrNotRealmMember, peerID)
}

// shouldRetry 判断是否应该重试
func shouldRetry(err error) bool {
	// 超时错误不重试
	if err == context.DeadlineExceeded || err == context.Canceled {
		return false
	}

	// 验证错误不重试
	if err == ErrNotRealmMember || err == ErrInvalidProtocol {
		return false
	}

	// 其他错误可以重试
	return true
}
