package addressbook

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dep2p/go-dep2p/internal/core/storage/engine"
	realmif "github.com/dep2p/go-dep2p/internal/realm/interfaces"
	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/lib/log"
	pb "github.com/dep2p/go-dep2p/pkg/lib/proto/addressbook"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// logger 是地址簿服务的日志记录器
var logger = log.Logger("relay/addressbook")

// AddressBookService 地址簿服务
//
// 封装地址簿的协议处理，提供：
// - 服务端：作为 Relay 接收注册/查询请求
// - 客户端：向 Relay 注册自己/查询他人
type AddressBookService struct {
	book    *MemberAddressBook
	handler *MessageHandler
	host    pkgif.Host
	realmID types.RealmID
	localID types.NodeID

	// 地址收集器（获取本地地址）
	addrProvider func() []types.Multiaddr

	// NAT 类型提供器
	natTypeProvider func() types.NATType

	// Phase 9 修复：能力标签提供器
	capabilitiesProvider func() []string

	// Phase 9 修复：静态能力标签
	capabilities   []string
	capabilitiesMu sync.RWMutex

	// 心跳相关
	heartbeatInterval time.Duration
	heartbeatCancel   context.CancelFunc
	heartbeatWg       sync.WaitGroup

	// 重试配置（v2.0 新增：指数退避重试）
	retryConfig RetryConfig

	// 当前注册的 Relay
	relayAddr string
	relayMu   sync.RWMutex

	// 事件总线（用于发布注册失败事件）
	eventBus pkgif.EventBus

	// 状态
	running atomic.Bool
	mu      sync.Mutex
}

// RetryConfig 重试配置
type RetryConfig struct {
	// MaxRetries 最大连续失败次数（超过后触发 Relay 切换）
	MaxRetries int
	// InitialBackoff 初始退避时间
	InitialBackoff time.Duration
	// MaxBackoff 最大退避时间
	MaxBackoff time.Duration
	// BackoffMultiplier 退避时间乘数
	BackoffMultiplier float64
}

// DefaultRetryConfig 默认重试配置
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries:        5,
		InitialBackoff:    1 * time.Second,
		MaxBackoff:        5 * time.Minute,
		BackoffMultiplier: 2.0,
	}
}

// ServiceConfig 服务配置
type ServiceConfig struct {
	// RealmID Realm 标识
	RealmID types.RealmID

	// LocalID 本地节点 ID
	LocalID types.NodeID

	// Host 网络主机（用于流处理）
	Host pkgif.Host

	// Engine 存储引擎（必需，除非提供 Book）
	// 从 v1.1.0 开始，必须使用持久化存储
	Engine engine.InternalEngine

	// Book 地址簿（可选，优先使用）
	// 如果提供则忽略 Engine
	Book *MemberAddressBook

	// MembershipChecker 成员资格检查器（可选）
	MembershipChecker MembershipChecker

	// AddrProvider 地址提供器（获取本地可公布地址）
	AddrProvider func() []types.Multiaddr

	// NATTypeProvider NAT 类型提供器
	NATTypeProvider func() types.NATType

	// HeartbeatInterval 心跳间隔（默认 5 分钟）
	HeartbeatInterval time.Duration

	// DefaultTTL 默认 TTL（默认 24 小时）
	DefaultTTL time.Duration

	// Capabilities 静态能力标签（可选）
	// 如果不设置，将使用默认能力标签
	Capabilities []string

	// CapabilitiesProvider 动态能力标签提供器（可选，优先于 Capabilities）
	CapabilitiesProvider func() []string

	// RetryConfig 重试配置（v2.0 新增）
	// 如果不设置，将使用默认配置
	RetryConfig *RetryConfig

	// EventBus 事件总线（可选，用于发布注册失败事件）
	EventBus pkgif.EventBus
}

// 默认能力标签
var DefaultCapabilities = []string{
	"relay/v1",      // 基础中继功能
	"addressbook",   // 地址簿支持
	"nat-traversal", // NAT 穿透支持
}

// NewAddressBookService 创建地址簿服务
//
// 从 v1.1.0 开始，必须提供 Engine 或 Book。
func NewAddressBookService(config ServiceConfig) (*AddressBookService, error) {
	// 默认值
	heartbeatInterval := config.HeartbeatInterval
	if heartbeatInterval <= 0 {
		heartbeatInterval = 5 * time.Minute
	}

	defaultTTL := config.DefaultTTL
	if defaultTTL <= 0 {
		defaultTTL = 24 * time.Hour
	}

	// 创建地址簿（如果未提供）
	book := config.Book
	if book == nil {
		if config.Engine == nil {
			return nil, fmt.Errorf("addressbook service: %w", ErrEngineRequired)
		}
		store, err := NewStore(StoreConfig{
			Engine:     config.Engine,
			DefaultTTL: defaultTTL,
		})
		if err != nil {
			return nil, fmt.Errorf("addressbook service: create store: %w", err)
		}
		book, err = NewWithStore(config.RealmID, store)
		if err != nil {
			return nil, fmt.Errorf("addressbook service: create book: %w", err)
		}
	}

	// 创建消息处理器
	handler := NewMessageHandler(HandlerConfig{
		Book:              book,
		RealmID:           config.RealmID,
		LocalID:           config.LocalID,
		MembershipChecker: config.MembershipChecker,
		DefaultTTL:        defaultTTL,
	})

	// 初始化能力标签
	var capabilities []string
	if len(config.Capabilities) > 0 {
		capabilities = make([]string, len(config.Capabilities))
		copy(capabilities, config.Capabilities)
	} else {
		// 使用默认能力标签
		capabilities = make([]string, len(DefaultCapabilities))
		copy(capabilities, DefaultCapabilities)
	}

	// 初始化重试配置
	retryConfig := DefaultRetryConfig()
	if config.RetryConfig != nil {
		retryConfig = *config.RetryConfig
	}

	return &AddressBookService{
		book:                 book,
		handler:              handler,
		host:                 config.Host,
		realmID:              config.RealmID,
		localID:              config.LocalID,
		addrProvider:         config.AddrProvider,
		natTypeProvider:      config.NATTypeProvider,
		capabilitiesProvider: config.CapabilitiesProvider,
		capabilities:         capabilities,
		heartbeatInterval:    heartbeatInterval,
		retryConfig:          retryConfig,
		eventBus:             config.EventBus,
	}, nil
}

// ============================================================================
//                              服务端功能（Relay）
// ============================================================================

// Start 启动服务（注册协议处理器）
//
// 在 Relay 节点上调用，开始接受地址注册/查询请求。
func (s *AddressBookService) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running.Load() {
		return nil // 已启动
	}

	if s.host == nil {
		return fmt.Errorf("host not set")
	}

	// 注册协议处理器
	protocolID := FormatProtocolID(string(s.realmID))
	s.host.SetStreamHandler(protocolID, s.handler.HandleStream)

	s.running.Store(true)
	return nil
}

// Stop 停止服务（注销协议处理器）
func (s *AddressBookService) Stop() error {
	if !s.running.Load() {
		return nil // 未启动
	}

	// 停止心跳（在获取主锁之前，因为心跳循环可能需要主锁）
	s.StopHeartbeat()

	s.mu.Lock()
	defer s.mu.Unlock()

	// 注销协议处理器
	if s.host != nil {
		protocolID := FormatProtocolID(string(s.realmID))
		s.host.RemoveStreamHandler(protocolID)
	}

	// 关闭地址簿
	s.book.Close()

	s.running.Store(false)
	return nil
}

// Book 返回内部地址簿（用于直接操作）
func (s *AddressBookService) Book() *MemberAddressBook {
	return s.book
}

// ============================================================================
//                              客户端功能（Member）
// ============================================================================

// RegisterSelf 向 Relay 注册自己的地址
//
// 成员节点调用，将自己的地址信息注册到 Relay。
func (s *AddressBookService) RegisterSelf(ctx context.Context, relayPeerID string) error {
	// FIX #B45: 加锁保护所有 provider 字段的读取，避免与 Setter 的写入竞争
	s.mu.Lock()
	host := s.host
	addrProvider := s.addrProvider
	natTypeProvider := s.natTypeProvider
	s.mu.Unlock()
	
	if host == nil {
		return fmt.Errorf("host not set")
	}
	
	var addrs []types.Multiaddr
	if addrProvider != nil {
		addrs = addrProvider()
	}

	// 获取 NAT 类型
	var natType types.NATType
	if natTypeProvider != nil {
		natType = natTypeProvider()
	}

	// 构建注册消息
	addrBytes := make([][]byte, len(addrs))
	for i, addr := range addrs {
		if addr != nil {
			addrBytes[i] = addr.Bytes()
		}
	}

	// Phase 9 修复：获取能力标签
	capabilities := s.getCapabilities()

	reg := &pb.AddressRegister{
		NodeId:       []byte(s.localID),
		Addrs:        addrBytes,
		NatType:      natTypeToProto(natType),
		Capabilities: capabilities,
		Timestamp:    time.Now().Unix(),
		// Signature: 暂时跳过签名
	}

	// 创建流并发送
	protocolID := FormatProtocolID(string(s.realmID))
	stream, err := s.host.NewStream(ctx, relayPeerID, protocolID)
	if err != nil {
		if strings.Contains(err.Error(), "protocols not supported") {
			return ErrProtocolNotSupported
		}
		return fmt.Errorf("create stream: %w", err)
	}
	defer stream.Close()

	// 发送注册消息
	msg := NewRegisterMessage(reg)
	if err := WriteMessage(stream, msg); err != nil {
		return fmt.Errorf("send register: %w", err)
	}

	// 读取响应
	resp, err := ReadMessage(stream)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	if resp.Type != pb.AddressBookMessage_REGISTER_RESPONSE {
		return fmt.Errorf("unexpected response type: %v", resp.Type)
	}

	regResp := resp.GetRegisterResponse()
	if regResp == nil || !regResp.Success {
		errMsg := "unknown error"
		if regResp != nil && regResp.Error != "" {
			errMsg = regResp.Error
		}
		return fmt.Errorf("register failed: %s", errMsg)
	}

	// 保存当前 Relay
	s.relayMu.Lock()
	s.relayAddr = relayPeerID
	s.relayMu.Unlock()

	return nil
}

// Query 从 Relay 查询目标成员地址
//
// 成员节点调用，查询其他成员的地址信息。
func (s *AddressBookService) Query(ctx context.Context, relayPeerID, targetID string) (*realmif.MemberEntry, error) {
	// FIX #B45: 加锁保护 host 读取
	s.mu.Lock()
	host := s.host
	s.mu.Unlock()
	
	if host == nil {
		return nil, fmt.Errorf("host not set")
	}

	// 构建查询消息
	query := &pb.AddressQuery{
		TargetNodeId: []byte(targetID),
		RequestorId:  []byte(s.localID),
	}

	// 创建流并发送
	protocolID := FormatProtocolID(string(s.realmID))
	stream, err := host.NewStream(ctx, relayPeerID, protocolID)
	if err != nil {
		return nil, fmt.Errorf("create stream: %w", err)
	}
	defer stream.Close()

	// 发送查询消息
	msg := NewQueryMessage(query)
	if err := WriteMessage(stream, msg); err != nil {
		return nil, fmt.Errorf("send query: %w", err)
	}

	// 读取响应
	resp, err := ReadMessage(stream)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.Type != pb.AddressBookMessage_RESPONSE {
		return nil, fmt.Errorf("unexpected response type: %v", resp.Type)
	}

	addrResp := resp.GetResponse()
	if addrResp == nil {
		return nil, fmt.Errorf("empty response")
	}

	if !addrResp.Found {
		return nil, ErrMemberNotFound
	}

	if addrResp.Entry == nil {
		return nil, fmt.Errorf("found but entry is nil")
	}

	// 转换为 MemberEntry
	entry, err := EntryFromProto(addrResp.Entry)
	if err != nil {
		return nil, fmt.Errorf("parse entry: %w", err)
	}

	return &entry, nil
}

// BatchQuery 批量查询成员地址
func (s *AddressBookService) BatchQuery(ctx context.Context, relayPeerID string, targetIDs []string) (map[string]*realmif.MemberEntry, error) {
	// FIX #B45: 加锁保护 host 读取
	s.mu.Lock()
	host := s.host
	s.mu.Unlock()
	
	if host == nil {
		return nil, fmt.Errorf("host not set")
	}

	// 限制批量数量
	if len(targetIDs) > MaxBatchSize {
		targetIDs = targetIDs[:MaxBatchSize]
	}

	// 构建查询消息
	targetIDBytes := make([][]byte, len(targetIDs))
	for i, id := range targetIDs {
		targetIDBytes[i] = []byte(id)
	}

	query := &pb.BatchAddressQuery{
		TargetNodeIds: targetIDBytes,
		RequestorId:   []byte(s.localID),
	}

	// 创建流并发送
	protocolID := FormatProtocolID(string(s.realmID))
	stream, err := host.NewStream(ctx, relayPeerID, protocolID)
	if err != nil {
		return nil, fmt.Errorf("create stream: %w", err)
	}
	defer stream.Close()

	// 发送查询消息
	msg := NewBatchQueryMessage(query)
	if err := WriteMessage(stream, msg); err != nil {
		return nil, fmt.Errorf("send batch query: %w", err)
	}

	// 读取响应
	resp, err := ReadMessage(stream)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.Type != pb.AddressBookMessage_BATCH_RESPONSE {
		return nil, fmt.Errorf("unexpected response type: %v", resp.Type)
	}

	batchResp := resp.GetBatchResponse()
	if batchResp == nil {
		return nil, fmt.Errorf("empty batch response")
	}

	// 解析结果
	result := make(map[string]*realmif.MemberEntry)
	for i, addrResp := range batchResp.Entries {
		if i >= len(targetIDs) {
			break
		}
		targetID := targetIDs[i]

		if addrResp != nil && addrResp.Found && addrResp.Entry != nil {
			entry, err := EntryFromProto(addrResp.Entry)
			if err == nil {
				result[targetID] = &entry
			}
		}
	}

	return result, nil
}

// ============================================================================
//                              心跳机制
// ============================================================================

// StartHeartbeat 启动心跳（定期向 Relay 重新注册）
//
// 在成功注册后调用，保持地址信息的有效性。
func (s *AddressBookService) StartHeartbeat(relayPeerID string) {
	// 停止现有心跳
	s.StopHeartbeat()

	s.mu.Lock()
	defer s.mu.Unlock()

	// 创建取消上下文
	ctx, cancel := context.WithCancel(context.Background())
	s.heartbeatCancel = cancel

	// 启动心跳协程
	s.heartbeatWg.Add(1)
	go s.heartbeatLoop(ctx, relayPeerID)
}

// StopHeartbeat 停止心跳
func (s *AddressBookService) StopHeartbeat() {
	// 获取锁并取消心跳
	s.mu.Lock()
	cancel := s.heartbeatCancel
	s.heartbeatCancel = nil
	s.mu.Unlock()

	// 取消心跳上下文
	if cancel != nil {
		cancel()
	}

	// 等待心跳协程退出（不持有锁，避免死锁）
	s.heartbeatWg.Wait()
}

// heartbeatLoop 心跳循环（带指数退避重试）
//
// v2.0 新增：指数退避重试机制
// - 首次失败后立即重试，使用初始退避时间
// - 连续失败时，退避时间指数增长（最大不超过 MaxBackoff）
// - 连续失败达到 MaxRetries 后，发布 RelayRegistrationFailed 事件
// - 成功注册后，重置退避状态
func (s *AddressBookService) heartbeatLoop(ctx context.Context, relayPeerID string) {
	defer s.heartbeatWg.Done()

	ticker := time.NewTicker(s.heartbeatInterval)
	defer ticker.Stop()

	// 重试状态
	consecutiveFailures := 0
	currentBackoff := s.retryConfig.InitialBackoff

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// 重新注册
			if err := s.RegisterSelf(ctx, relayPeerID); err != nil {
				consecutiveFailures++
				logger.Warn("地址簿自动重注册失败",
					"relay", relayPeerID,
					"error", err,
					"consecutiveFailures", consecutiveFailures,
					"nextBackoff", currentBackoff)

				// 检查是否达到最大重试次数
				if consecutiveFailures >= s.retryConfig.MaxRetries {
					logger.Error("Relay 注册连续失败达到阈值，需要切换 Relay",
						"relay", relayPeerID,
						"failures", consecutiveFailures,
						"maxRetries", s.retryConfig.MaxRetries)

					// 发布注册失败事件
					s.publishRegistrationFailedEvent(relayPeerID, consecutiveFailures, err)

					// 重置失败计数，但保持较长的退避时间
					consecutiveFailures = 0
				}

				// 执行退避重试
				s.performBackoffRetry(ctx, relayPeerID, &consecutiveFailures, &currentBackoff)
			} else {
				// 成功：重置退避状态
				if consecutiveFailures > 0 {
					logger.Info("Relay 注册恢复成功",
						"relay", relayPeerID,
						"previousFailures", consecutiveFailures)
				}
				consecutiveFailures = 0
				currentBackoff = s.retryConfig.InitialBackoff
			}
		}
	}
}

// performBackoffRetry 执行退避重试
func (s *AddressBookService) performBackoffRetry(
	ctx context.Context,
	relayPeerID string,
	consecutiveFailures *int,
	currentBackoff *time.Duration,
) {
	// 等待退避时间
	select {
	case <-ctx.Done():
		return
	case <-time.After(*currentBackoff):
	}

	// 尝试重试
	if err := s.RegisterSelf(ctx, relayPeerID); err != nil {
		// 重试也失败，增加退避时间
		newBackoff := time.Duration(float64(*currentBackoff) * s.retryConfig.BackoffMultiplier)
		if newBackoff > s.retryConfig.MaxBackoff {
			newBackoff = s.retryConfig.MaxBackoff
		}
		*currentBackoff = newBackoff

		logger.Debug("退避重试失败",
			"relay", relayPeerID,
			"error", err,
			"nextBackoff", newBackoff)
	} else {
		// 重试成功
		logger.Debug("退避重试成功", "relay", relayPeerID)
		*consecutiveFailures = 0
		*currentBackoff = s.retryConfig.InitialBackoff
	}
}

// publishRegistrationFailedEvent 发布注册失败事件
func (s *AddressBookService) publishRegistrationFailedEvent(relayPeerID string, failures int, lastErr error) {
	if s.eventBus == nil {
		return
	}

	event := RelayRegistrationFailedEvent{
		RelayPeerID:         relayPeerID,
		ConsecutiveFailures: failures,
		LastError:           lastErr,
		Timestamp:           time.Now(),
	}

	// 获取发射器
	emitter, emitErr := s.eventBus.Emitter(new(RelayRegistrationFailedEvent))
	if emitErr != nil {
		logger.Debug("获取事件发射器失败", "error", emitErr)
		return
	}
	defer emitter.Close()

	if pubErr := emitter.Emit(event); pubErr != nil {
		logger.Debug("发布注册失败事件失败", "error", pubErr)
	}
}

// RelayRegistrationFailedEvent Relay 注册失败事件
//
// 当连续注册失败达到阈值时发布此事件，
// 上层组件可以订阅此事件并触发 Relay 切换。
type RelayRegistrationFailedEvent struct {
	// RelayPeerID 失败的 Relay 节点 ID
	RelayPeerID string
	// ConsecutiveFailures 连续失败次数
	ConsecutiveFailures int
	// LastError 最后一次错误
	LastError error
	// Timestamp 事件时间
	Timestamp time.Time
}

// ============================================================================
//                              辅助方法
// ============================================================================

// GetRelayPeerID 获取当前注册的 Relay 节点 ID
//
// v2.0 新增：用于网络变化时重新注册。
func (s *AddressBookService) GetRelayPeerID() string {
	s.relayMu.RLock()
	defer s.relayMu.RUnlock()
	return s.relayAddr
}

// SetRelayPeerID 设置 Relay 节点 ID（用于首次注册）
func (s *AddressBookService) SetRelayPeerID(relayPeerID string) {
	s.relayMu.Lock()
	defer s.relayMu.Unlock()
	if s.relayAddr == "" && relayPeerID != "" {
		s.relayAddr = relayPeerID
		logger.Debug("预设 Relay PeerID", "relayPeerID", relayPeerID[:min(8, len(relayPeerID))])
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// SetHost 设置 Host
func (s *AddressBookService) SetHost(host pkgif.Host) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.host = host
}

// SetAddrProvider 设置地址提供器
func (s *AddressBookService) SetAddrProvider(provider func() []types.Multiaddr) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.addrProvider = provider
}

// SetNATTypeProvider 设置 NAT 类型提供器
func (s *AddressBookService) SetNATTypeProvider(provider func() types.NATType) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.natTypeProvider = provider
}

// SetCapabilitiesProvider 设置能力标签提供器
//
// Phase 9 修复：支持动态能力标签
func (s *AddressBookService) SetCapabilitiesProvider(provider func() []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.capabilitiesProvider = provider
}

// SetCapabilities 设置静态能力标签
//
// Phase 9 修复：支持静态能力标签配置
func (s *AddressBookService) SetCapabilities(caps []string) {
	s.capabilitiesMu.Lock()
	defer s.capabilitiesMu.Unlock()
	s.capabilities = make([]string, len(caps))
	copy(s.capabilities, caps)
}

// AddCapability 添加能力标签
func (s *AddressBookService) AddCapability(cap string) {
	s.capabilitiesMu.Lock()
	defer s.capabilitiesMu.Unlock()
	// 避免重复
	for _, existing := range s.capabilities {
		if existing == cap {
			return
		}
	}
	s.capabilities = append(s.capabilities, cap)
}

// RemoveCapability 移除能力标签
func (s *AddressBookService) RemoveCapability(cap string) {
	s.capabilitiesMu.Lock()
	defer s.capabilitiesMu.Unlock()
	for i, existing := range s.capabilities {
		if existing == cap {
			s.capabilities = append(s.capabilities[:i], s.capabilities[i+1:]...)
			return
		}
	}
}

// getCapabilities 获取当前能力标签
func (s *AddressBookService) getCapabilities() []string {
	// 优先使用动态提供器
	if s.capabilitiesProvider != nil {
		return s.capabilitiesProvider()
	}

	// 使用静态配置
	s.capabilitiesMu.RLock()
	defer s.capabilitiesMu.RUnlock()
	result := make([]string, len(s.capabilities))
	copy(result, s.capabilities)
	return result
}

// IsRunning 检查服务是否运行中
func (s *AddressBookService) IsRunning() bool {
	return s.running.Load()
}

// CurrentRelay 返回当前注册的 Relay
func (s *AddressBookService) CurrentRelay() string {
	s.relayMu.RLock()
	defer s.relayMu.RUnlock()
	return s.relayAddr
}

// AddressBookStats 地址簿统计信息
//
// Phase 9 修复：添加统计信息结构
type AddressBookStats struct {
	// TotalMembers 总成员数
	TotalMembers int
	// OnlineMembers 在线成员数
	OnlineMembers int
	// HasCurrentRelay 是否有当前 Relay
	HasCurrentRelay bool
}

// Stats 获取地址簿统计信息
//
// Phase 9 修复：实现真实统计数据获取
func (s *AddressBookService) Stats() AddressBookStats {
	stats := AddressBookStats{}

	// 检查当前 Relay
	s.relayMu.RLock()
	stats.HasCurrentRelay = s.relayAddr != ""
	s.relayMu.RUnlock()

	// 获取成员统计
	if s.book != nil {
		ctx := context.Background()

		// 总成员数
		if members, err := s.book.Members(ctx); err == nil {
			stats.TotalMembers = len(members)
		}

		// 在线成员数
		if onlineMembers, err := s.book.OnlineMembers(ctx); err == nil {
			stats.OnlineMembers = len(onlineMembers)
		}
	}

	return stats
}
