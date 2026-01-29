package addressbook

import (
	"context"
	"fmt"
	"time"

	"github.com/dep2p/go-dep2p/pkg/interfaces"
	pb "github.com/dep2p/go-dep2p/pkg/lib/proto/addressbook"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// MembershipChecker 成员资格检查函数
type MembershipChecker func(nodeID types.NodeID) bool

// SignatureVerifier 签名验证函数
//
// Phase 9 修复：验证注册消息的签名
// 参数：nodeID - 声称的节点 ID，signature - 签名，data - 被签名的数据
// 返回：true 表示签名有效，false 表示签名无效或无法验证
type SignatureVerifier func(nodeID types.NodeID, signature []byte, data []byte) bool

// MessageHandler 消息处理器
//
// 处理地址簿协议的各类消息（注册、查询等）。
// 在 Relay 端运行，接收并处理来自成员的请求。
type MessageHandler struct {
	book    *MemberAddressBook
	realmID types.RealmID
	localID types.NodeID

	// 成员资格检查器（可选，nil 表示信任 PSK 认证）
	membershipChecker MembershipChecker

	// Phase 9 修复：签名验证器（可选，nil 表示跳过签名验证）
	signatureVerifier SignatureVerifier

	// 签名验证配置
	requireSignature bool          // 是否强制要求签名
	maxTimestampAge  time.Duration // 时间戳最大偏差

	// 默认 TTL
	defaultTTL time.Duration
}

// HandlerConfig 处理器配置
type HandlerConfig struct {
	Book              *MemberAddressBook
	RealmID           types.RealmID
	LocalID           types.NodeID
	MembershipChecker MembershipChecker
	SignatureVerifier SignatureVerifier // Phase 9 修复：可选的签名验证器
	RequireSignature  bool              // Phase 9 修复：是否强制要求签名
	MaxTimestampAge   time.Duration     // Phase 9 修复：时间戳最大偏差（默认 5 分钟）
	DefaultTTL        time.Duration
}

// NewMessageHandler 创建消息处理器
func NewMessageHandler(config HandlerConfig) *MessageHandler {
	ttl := config.DefaultTTL
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}

	// Phase 9 修复：时间戳最大偏差默认 5 分钟
	maxTimestampAge := config.MaxTimestampAge
	if maxTimestampAge <= 0 {
		maxTimestampAge = 5 * time.Minute
	}

	return &MessageHandler{
		book:              config.Book,
		realmID:           config.RealmID,
		localID:           config.LocalID,
		membershipChecker: config.MembershipChecker,
		signatureVerifier: config.SignatureVerifier,
		requireSignature:  config.RequireSignature,
		maxTimestampAge:   maxTimestampAge,
		defaultTTL:        ttl,
	}
}

// HandleStream 处理流（协议处理器入口）
func (h *MessageHandler) HandleStream(stream interfaces.Stream) {
	defer stream.Close()

	// 读取消息
	msg, err := ReadMessage(stream)
	if err != nil {
		// 读取失败，静默关闭
		return
	}

	// 根据消息类型分发
	var resp *pb.AddressBookMessage
	switch msg.Type {
	case pb.AddressBookMessage_REGISTER:
		resp = h.handleRegister(stream, msg.GetRegister())
	case pb.AddressBookMessage_QUERY:
		resp = h.handleQuery(stream, msg.GetQuery())
	case pb.AddressBookMessage_BATCH_QUERY:
		resp = h.handleBatchQuery(stream, msg.GetBatchQuery())
	default:
		// 未知消息类型，忽略
		return
	}

	// 发送响应
	if resp != nil {
		WriteMessage(stream, resp)
	}
}

// handleRegister 处理注册请求
func (h *MessageHandler) handleRegister(_ interfaces.Stream, reg *pb.AddressRegister) *pb.AddressBookMessage {
	if reg == nil {
		return NewRegisterResponseMessage(&pb.AddressRegisterResponse{
			Success: false,
			Error:   "empty register request",
		})
	}

	// 获取请求者 ID
	nodeID := types.PeerID(reg.NodeId)
	if nodeID.IsEmpty() {
		return NewRegisterResponseMessage(&pb.AddressRegisterResponse{
			Success: false,
			Error:   "invalid node ID",
		})
	}

	// 验证成员资格（如果配置了检查器）
	if h.membershipChecker != nil && !h.membershipChecker(nodeID) {
		return NewRegisterResponseMessage(&pb.AddressRegisterResponse{
			Success: false,
			Error:   "not a realm member",
		})
	}

	// Phase 9 修复：签名验证
	if err := h.verifySignature(reg); err != nil {
		return NewRegisterResponseMessage(&pb.AddressRegisterResponse{
			Success: false,
			Error:   fmt.Sprintf("signature verification failed: %v", err),
		})
	}

	// 转换为 MemberEntry
	entry, err := RegisterToEntry(reg)
	if err != nil {
		return NewRegisterResponseMessage(&pb.AddressRegisterResponse{
			Success: false,
			Error:   fmt.Sprintf("invalid entry: %v", err),
		})
	}

	// 注册到地址簿
	ctx := context.Background()
	if err := h.book.Register(ctx, entry); err != nil {
		return NewRegisterResponseMessage(&pb.AddressRegisterResponse{
			Success: false,
			Error:   fmt.Sprintf("register failed: %v", err),
		})
	}

	// 返回成功响应
	return NewRegisterResponseMessage(&pb.AddressRegisterResponse{
		Success: true,
		Ttl:     int64(h.defaultTTL.Seconds()),
	})
}

// verifySignature 验证注册消息的签名
//
// Phase 9 修复：实现签名验证逻辑
func (h *MessageHandler) verifySignature(reg *pb.AddressRegister) error {
	// 如果没有配置签名验证器
	if h.signatureVerifier == nil {
		if h.requireSignature {
			return fmt.Errorf("signature verifier not configured but signature is required")
		}
		// 没有验证器且不强制要求签名，跳过验证
		return nil
	}

	// 检查是否提供了签名
	if len(reg.Signature) == 0 {
		if h.requireSignature {
			return fmt.Errorf("signature is required but not provided")
		}
		// 不强制要求签名，允许空签名
		return nil
	}

	// 验证时间戳（防止重放攻击）
	if reg.Timestamp > 0 {
		msgTime := time.Unix(reg.Timestamp, 0)
		now := time.Now()
		if now.Sub(msgTime).Abs() > h.maxTimestampAge {
			return fmt.Errorf("timestamp too old or in the future: %v", msgTime)
		}
	} else if h.requireSignature {
		return fmt.Errorf("timestamp is required for signed messages")
	}

	// 构造被签名的数据（node_id + addrs + nat_type + capabilities + timestamp）
	signedData := h.buildSignedData(reg)

	// 验证签名
	nodeID := types.PeerID(reg.NodeId)
	if !h.signatureVerifier(nodeID, reg.Signature, signedData) {
		return fmt.Errorf("invalid signature")
	}

	return nil
}

// buildSignedData 构造被签名的数据
func (h *MessageHandler) buildSignedData(reg *pb.AddressRegister) []byte {
	// 简单拼接：node_id || 地址数量 || 各地址 || nat_type || timestamp
	// 这是一个简化版本，生产环境可使用更规范的序列化
	var data []byte

	// Node ID
	data = append(data, reg.NodeId...)

	// 地址列表
	for _, addr := range reg.Addrs {
		data = append(data, addr...)
	}

	// NAT 类型（转为单字节）
	data = append(data, byte(reg.NatType))

	// 能力标签
	for _, cap := range reg.Capabilities {
		data = append(data, []byte(cap)...)
	}

	// 时间戳（8 字节大端序）
	ts := reg.Timestamp
	data = append(data,
		byte(ts>>56), byte(ts>>48), byte(ts>>40), byte(ts>>32),
		byte(ts>>24), byte(ts>>16), byte(ts>>8), byte(ts),
	)

	return data
}

// handleQuery 处理查询请求
func (h *MessageHandler) handleQuery(_ interfaces.Stream, query *pb.AddressQuery) *pb.AddressBookMessage {
	if query == nil {
		return NewResponseMessage(&pb.AddressResponse{
			Found: false,
			Error: "empty query request",
		})
	}

	// 获取请求者 ID（用于验证成员资格）
	requestorID := types.PeerID(query.RequestorId)

	// 验证成员资格（如果配置了检查器）
	if h.membershipChecker != nil && !requestorID.IsEmpty() && !h.membershipChecker(requestorID) {
		return NewResponseMessage(&pb.AddressResponse{
			Found: false,
			Error: "requestor not a realm member",
		})
	}

	// 获取目标 ID
	targetID := types.PeerID(query.TargetNodeId)
	if targetID.IsEmpty() {
		return NewResponseMessage(&pb.AddressResponse{
			Found: false,
			Error: "invalid target node ID",
		})
	}

	// 查询地址簿
	ctx := context.Background()
	entry, err := h.book.Query(ctx, targetID)
	if err != nil {
		if err == ErrMemberNotFound {
			return NewResponseMessage(&pb.AddressResponse{
				Found: false,
			})
		}
		return NewResponseMessage(&pb.AddressResponse{
			Found: false,
			Error: fmt.Sprintf("query failed: %v", err),
		})
	}

	// 返回结果
	return NewResponseMessage(&pb.AddressResponse{
		Found: true,
		Entry: EntryToProto(entry),
	})
}

// handleBatchQuery 处理批量查询请求
func (h *MessageHandler) handleBatchQuery(_ interfaces.Stream, query *pb.BatchAddressQuery) *pb.AddressBookMessage {
	if query == nil {
		return NewBatchResponseMessage(&pb.BatchAddressResponse{})
	}

	// 验证请求者成员资格
	requestorID := types.PeerID(query.RequestorId)
	if h.membershipChecker != nil && !requestorID.IsEmpty() && !h.membershipChecker(requestorID) {
		return NewBatchResponseMessage(&pb.BatchAddressResponse{})
	}

	// 限制批量查询数量
	targetIDs := query.TargetNodeIds
	if len(targetIDs) > MaxBatchSize {
		targetIDs = targetIDs[:MaxBatchSize]
	}

	// 批量查询
	ctx := context.Background()
	entries := make([]*pb.AddressResponse, 0, len(targetIDs))

	for _, targetIDBytes := range targetIDs {
		targetID := types.PeerID(targetIDBytes)
		if targetID.IsEmpty() {
			entries = append(entries, &pb.AddressResponse{Found: false})
			continue
		}

		entry, err := h.book.Query(ctx, targetID)
		if err != nil {
			entries = append(entries, &pb.AddressResponse{Found: false})
			continue
		}

		entries = append(entries, &pb.AddressResponse{
			Found: true,
			Entry: EntryToProto(entry),
		})
	}

	return NewBatchResponseMessage(&pb.BatchAddressResponse{
		Entries: entries,
	})
}
