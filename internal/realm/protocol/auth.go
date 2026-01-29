// Package protocol 实现 Realm 协议
package protocol

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/dep2p/go-dep2p/internal/realm/auth"
	"github.com/dep2p/go-dep2p/internal/realm/interfaces"
	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/protocol"
)

// 注：logger 变量在 capability.go 中定义（同包共享）

// AuthProtocolID 认证协议 ID 模板（使用统一定义）
//
// 格式：/dep2p/realm/<realmID>/auth/1.0.0
var AuthProtocolID = protocol.RealmAuthFormat

// ============================================================================
//                         成员交换消息结构（即时同步优化）
// ============================================================================

// memberExchangePrefix 成员交换消息前缀
const memberExchangePrefix = "members:"

// memberExchangeTimeout 成员交换超时时间
// 设置为 10 秒，足够传输大多数成员列表
const memberExchangeTimeout = 10 * time.Second

// maxMembersInExchange 成员交换时最大成员数
// 防止成员列表过大导致内存问题
const maxMembersInExchange = 500

// MemberExchangeInfo 成员交换信息（单个成员）
type MemberExchangeInfo struct {
	PeerID   string   `json:"peer_id"`
	Addrs    []string `json:"addrs,omitempty"`
	LastSeen int64    `json:"last_seen,omitempty"` // Unix 时间戳
}

// MemberExchangeMessage 成员交换消息
type MemberExchangeMessage struct {
	Members   []MemberExchangeInfo `json:"members"`
	Timestamp int64                `json:"timestamp"`
}

// ============================================================================
//                              认证协议处理器
// ============================================================================

// AuthHandler 认证协议处理器
//
// 提供 Realm 成员认证的协议层封装，复用 auth 包的核心逻辑。
type AuthHandler struct {
	mu sync.RWMutex

	// 依赖
	host             pkgif.Host
	realmID          string
	authKey          []byte // 认证密钥（从 PSK 派生）
	authenticator    interfaces.Authenticator
	challengeHandler *auth.ChallengeHandler

	// 状态
	started bool
	closed  bool

	// 回调
	onAuthSuccess func(peerID string)
	onAuthFailed  func(peerID string, err error)

	// 成员交换回调（即时同步优化）
	getMemberList func() []MemberExchangeInfo        // 获取本地成员列表
	onMemberMerge func(members []MemberExchangeInfo) // 合并远程成员列表
}

// NewAuthHandler 创建认证处理器
//
// 参数：
//   - host: P2P 主机（用于协议注册和流管理）
//   - realmID: Realm ID（用于协议 ID）
//   - authKey: 认证密钥（从 PSK 派生，用于 HMAC）
//   - authenticator: 认证器（提供证明生成和验证，可选）
//   - challengeHandler: 挑战处理器（提供挑战-响应流程，可选）
func NewAuthHandler(
	host pkgif.Host,
	realmID string,
	authKey []byte,
	authenticator interfaces.Authenticator,
	challengeHandler *auth.ChallengeHandler,
) *AuthHandler {
	if challengeHandler == nil {
		// 使用默认配置
		challengeHandler = auth.NewChallengeHandler(0, 0, 0)
	}

	return &AuthHandler{
		host:             host,
		realmID:          realmID,
		authKey:          authKey,
		authenticator:    authenticator,
		challengeHandler: challengeHandler,
	}
}

// ============================================================================
//                              生命周期管理
// ============================================================================

// Start 启动处理器，注册协议到 Host
func (h *AuthHandler) Start(_ context.Context) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.started {
		return fmt.Errorf("auth handler already started")
	}

	if h.closed {
		return fmt.Errorf("auth handler is closed")
	}

	// 构造协议 ID
	protocolID := fmt.Sprintf(AuthProtocolID, h.realmID)

	// 注册协议处理器
	h.host.SetStreamHandler(protocolID, h.handleIncoming)

	h.started = true

	return nil
}

// Stop 停止处理器，注销协议
func (h *AuthHandler) Stop(_ context.Context) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if !h.started {
		return nil
	}

	if h.closed {
		return nil
	}

	// 注销协议处理器
	protocolID := fmt.Sprintf(AuthProtocolID, h.realmID)
	h.host.RemoveStreamHandler(protocolID)

	h.started = false

	return nil
}

// Close 关闭处理器
func (h *AuthHandler) Close() error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.closed {
		return nil
	}

	h.closed = true

	// 如果已启动，先停止
	if h.started {
		protocolID := fmt.Sprintf(AuthProtocolID, h.realmID)
		h.host.RemoveStreamHandler(protocolID)
		h.started = false
	}

	return nil
}

// ============================================================================
//                              入站处理（服务端侧）
// ============================================================================

// handleIncoming 处理入站认证请求
//
// 当远程节点发起认证时被调用。
//
// 认证成功后自动进行成员交换，实现即时成员同步。
func (h *AuthHandler) handleIncoming(stream pkgif.Stream) {
	defer stream.Close()

	h.mu.RLock()
	if h.closed {
		h.mu.RUnlock()
		return
	}
	authKey := h.authKey
	challengeHandler := h.challengeHandler
	onSuccess := h.onAuthSuccess
	onFailed := h.onAuthFailed
	getMemberList := h.getMemberList
	onMemberMerge := h.onMemberMerge
	h.mu.RUnlock()

	// 不使用 stream.Conn().RemotePeer()
	// 中继连接时 RemotePeer() 返回 Relay 节点 ID，不是真正的对端
	// 改为从 HandleChallenge 返回值获取实际的 PeerID

	// 使用 ChallengeHandler 处理认证
	// 复用已实现的挑战-响应逻辑
	remotePeer, err := challengeHandler.HandleChallenge(
		context.Background(),
		authKey,
		func() ([]byte, error) { return readMessage(stream) },         // receiveRequest
		func(data []byte) error { return writeMessage(stream, data) }, // sendChallenge
		func() ([]byte, error) { return readMessage(stream) },         // receiveResponse
		func(data []byte) error { return writeMessage(stream, data) }, // sendResult
	)

	if err != nil {
		// 认证失败
		if onFailed != nil {
			onFailed(remotePeer, err)
		}
		return
	}

	// 认证成功
	if onSuccess != nil {
		onSuccess(remotePeer)
	}

	// 认证成功后进行成员交换（即时同步优化）
	// 接收方先接收对方的成员列表，再发送本地成员列表
	if getMemberList != nil && onMemberMerge != nil {
		if err := h.exchangeMembersAsResponder(stream, getMemberList, onMemberMerge, remotePeer); err != nil {
			// 成员交换失败不影响认证结果
			logger.Debug("成员交换失败（接收方）", "err", err, "peerID", truncatePeerID(remotePeer))
		}
	}
}

// ============================================================================
//                              出站请求（客户端侧）
// ============================================================================

// Authenticate 向指定节点发起认证请求
//
// 参数：
//   - ctx: 上下文
//   - peerID: 目标节点 ID
//
// 返回：
//   - error: 认证错误（nil 表示认证成功）
//
// 认证成功后自动进行成员交换，实现即时成员同步。
func (h *AuthHandler) Authenticate(ctx context.Context, peerID string) error {
	h.mu.RLock()
	if h.closed {
		h.mu.RUnlock()
		return fmt.Errorf("auth handler is closed")
	}
	authKey := h.authKey
	challengeHandler := h.challengeHandler
	realmID := h.realmID
	localPeerID := string(h.host.ID())
	getMemberList := h.getMemberList
	onMemberMerge := h.onMemberMerge
	h.mu.RUnlock()

	// 构造协议 ID
	protocolID := fmt.Sprintf(AuthProtocolID, realmID)

	// 打开流到目标节点
	stream, err := h.host.NewStream(ctx, peerID, protocolID)
	if err != nil {
		return fmt.Errorf("failed to open auth stream: %w", err)
	}
	defer stream.Close()

	// 使用 ChallengeHandler 执行认证
	// 复用已实现的挑战-响应逻辑
	err = challengeHandler.PerformChallenge(
		ctx,
		localPeerID,
		realmID,
		authKey,
		func(data []byte) error { return writeMessage(stream, data) }, // sendRequest
		func() ([]byte, error) { return readMessage(stream) },         // receiveChallenge
		func(data []byte) error { return writeMessage(stream, data) }, // sendResponse
		func() ([]byte, error) { return readMessage(stream) },         // receiveResult
	)

	if err != nil {
		return fmt.Errorf("authentication failed: %w", err)
	}

	// 认证成功后进行成员交换（即时同步优化）
	// 发起方先发送本地成员列表，再接收对方的成员列表
	if getMemberList != nil && onMemberMerge != nil {
		if err := h.exchangeMembersAsInitiator(stream, getMemberList, onMemberMerge, peerID); err != nil {
			// 成员交换失败不影响认证结果
			logger.Debug("成员交换失败（发起方）", "err", err, "peerID", truncatePeerID(peerID))
		}
	}

	return nil
}

// ============================================================================
//                              回调函数
// ============================================================================

// SetOnAuthSuccess 设置认证成功回调
func (h *AuthHandler) SetOnAuthSuccess(fn func(peerID string)) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.onAuthSuccess = fn
}

// SetOnAuthFailed 设置认证失败回调
func (h *AuthHandler) SetOnAuthFailed(fn func(peerID string, err error)) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.onAuthFailed = fn
}

// SetMemberExchangeCallbacks 设置成员交换回调（即时同步优化）
//
// 参数：
//   - getMemberList: 获取本地成员列表的回调
//   - onMemberMerge: 合并远程成员列表的回调
func (h *AuthHandler) SetMemberExchangeCallbacks(
	getMemberList func() []MemberExchangeInfo,
	onMemberMerge func(members []MemberExchangeInfo),
) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.getMemberList = getMemberList
	h.onMemberMerge = onMemberMerge
}

// ============================================================================
//                         成员交换实现（即时同步优化）
// ============================================================================

// exchangeMembersAsInitiator 作为发起方进行成员交换
//
// 发起方流程：
// 1. 发送本地成员列表
// 2. 接收对方成员列表
// 3. 合并对方成员
func (h *AuthHandler) exchangeMembersAsInitiator(
	stream pkgif.Stream,
	getMemberList func() []MemberExchangeInfo,
	onMemberMerge func(members []MemberExchangeInfo),
	remotePeerID string,
) error {
	// 1. 发送本地成员列表
	localMembers := getMemberList()
	if err := h.sendMemberExchange(stream, localMembers); err != nil {
		return fmt.Errorf("发送成员列表失败: %w", err)
	}

	logger.Debug("发送成员列表（发起方）",
		"memberCount", len(localMembers),
		"remotePeer", truncatePeerID(remotePeerID))

	// 2. 接收对方成员列表
	remoteMembers, err := h.receiveMemberExchange(stream)
	if err != nil {
		return fmt.Errorf("接收成员列表失败: %w", err)
	}

	logger.Debug("收到成员列表（发起方）",
		"memberCount", len(remoteMembers),
		"remotePeer", truncatePeerID(remotePeerID))

	// 3. 合并对方成员
	if len(remoteMembers) > 0 {
		onMemberMerge(remoteMembers)
	}

	// 添加成功日志便于调试
	logger.Info("成员交换完成（发起方）",
		"localMembers", len(localMembers),
		"remoteMembers", len(remoteMembers),
		"remotePeer", truncatePeerID(remotePeerID))

	return nil
}

// exchangeMembersAsResponder 作为接收方进行成员交换
//
// 接收方流程：
// 1. 接收对方成员列表
// 2. 发送本地成员列表
// 3. 合并对方成员
func (h *AuthHandler) exchangeMembersAsResponder(
	stream pkgif.Stream,
	getMemberList func() []MemberExchangeInfo,
	onMemberMerge func(members []MemberExchangeInfo),
	remotePeerID string,
) error {
	// 1. 接收对方成员列表
	remoteMembers, err := h.receiveMemberExchange(stream)
	if err != nil {
		return fmt.Errorf("接收成员列表失败: %w", err)
	}

	logger.Debug("收到成员列表（接收方）",
		"memberCount", len(remoteMembers),
		"remotePeer", truncatePeerID(remotePeerID))

	// 2. 发送本地成员列表
	localMembers := getMemberList()
	if err := h.sendMemberExchange(stream, localMembers); err != nil {
		return fmt.Errorf("发送成员列表失败: %w", err)
	}

	logger.Debug("发送成员列表（接收方）",
		"memberCount", len(localMembers),
		"remotePeer", truncatePeerID(remotePeerID))

	// 3. 合并对方成员
	if len(remoteMembers) > 0 {
		onMemberMerge(remoteMembers)
	}

	// 添加成功日志便于调试
	logger.Info("成员交换完成（接收方）",
		"localMembers", len(localMembers),
		"remoteMembers", len(remoteMembers),
		"remotePeer", truncatePeerID(remotePeerID))

	return nil
}

// sendMemberExchange 发送成员交换消息
func (h *AuthHandler) sendMemberExchange(stream pkgif.Stream, members []MemberExchangeInfo) error {
	// 限制成员数量，防止消息过大
	if len(members) > maxMembersInExchange {
		members = members[:maxMembersInExchange]
		logger.Debug("成员列表过大，截断发送", "limit", maxMembersInExchange)
	}

	msg := MemberExchangeMessage{
		Members:   members,
		Timestamp: time.Now().Unix(),
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("编码成员交换消息失败: %w", err)
	}

	// 添加前缀
	payload := append([]byte(memberExchangePrefix), data...)

	// 设置写超时
	if deadline, ok := stream.(interface{ SetWriteDeadline(time.Time) error }); ok {
		_ = deadline.SetWriteDeadline(time.Now().Add(memberExchangeTimeout))
		defer func() { _ = deadline.SetWriteDeadline(time.Time{}) }()
	}

	return writeMessage(stream, payload)
}

// receiveMemberExchange 接收成员交换消息
func (h *AuthHandler) receiveMemberExchange(stream pkgif.Stream) ([]MemberExchangeInfo, error) {
	// 设置读超时
	if deadline, ok := stream.(interface{ SetReadDeadline(time.Time) error }); ok {
		_ = deadline.SetReadDeadline(time.Now().Add(memberExchangeTimeout))
		defer func() { _ = deadline.SetReadDeadline(time.Time{}) }()
	}

	data, err := readMessage(stream)
	if err != nil {
		return nil, fmt.Errorf("读取成员交换消息失败: %w", err)
	}

	// 检查前缀
	if len(data) < len(memberExchangePrefix) {
		return nil, fmt.Errorf("成员交换消息格式错误")
	}

	prefix := string(data[:len(memberExchangePrefix)])
	if prefix != memberExchangePrefix {
		return nil, fmt.Errorf("成员交换消息前缀错误: %s", prefix)
	}

	// 解码 JSON
	var msg MemberExchangeMessage
	if err := json.Unmarshal(data[len(memberExchangePrefix):], &msg); err != nil {
		return nil, fmt.Errorf("解码成员交换消息失败: %w", err)
	}

	// 限制接收的成员数量
	if len(msg.Members) > maxMembersInExchange {
		msg.Members = msg.Members[:maxMembersInExchange]
		logger.Debug("收到的成员列表过大，截断处理", "limit", maxMembersInExchange)
	}

	return msg.Members, nil
}

// truncatePeerID 截断 PeerID 用于日志
func truncatePeerID(peerID string) string {
	if len(peerID) > 8 {
		return peerID[:8]
	}
	return peerID
}

// ============================================================================
//                              辅助函数
// ============================================================================

// readMessage 从流中读取一条消息（长度前缀格式）
//
// 消息格式：[4字节长度][消息体]
func readMessage(stream pkgif.Stream) ([]byte, error) {
	// 读取长度前缀（4字节，大端序）
	lenBuf := make([]byte, 4)
	if _, err := io.ReadFull(stream, lenBuf); err != nil {
		return nil, fmt.Errorf("failed to read message length: %w", err)
	}

	// 解析长度
	length := int(lenBuf[0])<<24 | int(lenBuf[1])<<16 | int(lenBuf[2])<<8 | int(lenBuf[3])

	// 验证长度（防止过大的消息）
	const maxMessageSize = 1024 * 1024 // 1MB
	if length < 0 || length > maxMessageSize {
		return nil, fmt.Errorf("invalid message length: %d", length)
	}

	// 读取消息体
	data := make([]byte, length)
	if _, err := io.ReadFull(stream, data); err != nil {
		return nil, fmt.Errorf("failed to read message body: %w", err)
	}

	return data, nil
}

// writeMessage 向流中写入一条消息（长度前缀格式）
//
// 消息格式：[4字节长度][消息体]
func writeMessage(stream pkgif.Stream, data []byte) error {
	// 写入长度前缀（4字节，大端序）
	length := len(data)
	lenBuf := []byte{
		byte(length >> 24),
		byte(length >> 16),
		byte(length >> 8),
		byte(length),
	}

	if _, err := stream.Write(lenBuf); err != nil {
		return fmt.Errorf("failed to write message length: %w", err)
	}

	// 写入消息体
	if _, err := stream.Write(data); err != nil {
		return fmt.Errorf("failed to write message body: %w", err)
	}

	return nil
}
