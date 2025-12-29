// Package gossipsub 实现 GossipSub v1.1 协议
package gossipsub

import (
	"encoding/binary"
	"errors"
	"io"
	"time"

	"github.com/dep2p/go-dep2p/pkg/proto/gossipsub"
	"github.com/dep2p/go-dep2p/pkg/protocolids"
	"github.com/dep2p/go-dep2p/pkg/types"
	"google.golang.org/protobuf/proto"
)

// ============================================================================
//                              协议常量
// ============================================================================

// 引用 pkg/protocolids 唯一真源
var (
	// ProtocolGossipSub GossipSub 协议 ID (v1.1 scope: sys)
	ProtocolGossipSub = protocolids.SysGossipSub
)

const (

	// MaxRPCSize 最大 RPC 消息大小
	MaxRPCSize = 10 * 1024 * 1024 // 10 MB
)

// 错误定义
var (
	ErrMessageTooLarge  = errors.New("message too large")
	ErrInvalidMessage   = errors.New("invalid message")
	ErrReadFailed       = errors.New("read failed")
	ErrWriteFailed      = errors.New("write failed")
	ErrInvalidSignature = errors.New("invalid signature")
)

// ============================================================================
//                              RPC 编码器（使用 Protobuf）
// ============================================================================

// RPCCodec RPC 编解码器（基于 Protobuf）
type RPCCodec struct{}

// NewRPCCodec 创建新的 RPC 编解码器
func NewRPCCodec() *RPCCodec {
	return &RPCCodec{}
}

// EncodeRPC 编码 RPC 消息为 protobuf 格式
func (c *RPCCodec) EncodeRPC(rpc *RPC) ([]byte, error) {
	pbRPC := c.toProtoRPC(rpc)
	return proto.Marshal(pbRPC)
}

// DecodeRPC 从 protobuf 格式解码 RPC 消息
func (c *RPCCodec) DecodeRPC(data []byte) (*RPC, error) {
	if len(data) > MaxRPCSize {
		return nil, ErrMessageTooLarge
	}

	pbRPC := &gossipsub.RPC{}
	if err := proto.Unmarshal(data, pbRPC); err != nil {
		return nil, err
	}

	return c.fromProtoRPC(pbRPC), nil
}

// ============================================================================
//                              类型转换：Go -> Protobuf
// ============================================================================

// toProtoRPC 将 RPC 转换为 protobuf 消息
func (c *RPCCodec) toProtoRPC(rpc *RPC) *gossipsub.RPC {
	pbRPC := &gossipsub.RPC{}

	// 转换订阅列表
	for _, sub := range rpc.Subscriptions {
		pbRPC.Subscriptions = append(pbRPC.Subscriptions, &gossipsub.SubOpts{
			Subscribe: sub.Subscribe,
			Topic:     sub.Topic,
		})
	}

	// 转换消息列表
	for _, msg := range rpc.Messages {
		pbRPC.Publish = append(pbRPC.Publish, c.toProtoMessage(msg))
	}

	// 转换控制消息
	if rpc.Control != nil {
		pbRPC.Control = c.toProtoControl(rpc.Control)
	}

	return pbRPC
}

// toProtoMessage 将 Message 转换为 protobuf 消息
func (c *RPCCodec) toProtoMessage(msg *Message) *gossipsub.Message {
	seqno := make([]byte, 8)
	binary.BigEndian.PutUint64(seqno, msg.Sequence)

	return &gossipsub.Message{
		From:      msg.From[:],
		Data:      msg.Data,
		Seqno:     seqno,
		Topic:     msg.Topic,
		Signature: msg.Signature,
		Key:       msg.Key,
	}
}

// toProtoControl 将 ControlMessage 转换为 protobuf 消息
func (c *RPCCodec) toProtoControl(ctrl *ControlMessage) *gossipsub.ControlMessage {
	pbCtrl := &gossipsub.ControlMessage{}

	// IHAVE
	for _, ihave := range ctrl.IHave {
		pbCtrl.Ihave = append(pbCtrl.Ihave, &gossipsub.ControlIHave{
			Topic:      ihave.Topic,
			MessageIds: ihave.MessageIDs,
		})
	}

	// IWANT
	for _, iwant := range ctrl.IWant {
		pbCtrl.Iwant = append(pbCtrl.Iwant, &gossipsub.ControlIWant{
			MessageIds: iwant.MessageIDs,
		})
	}

	// GRAFT
	for _, graft := range ctrl.Graft {
		pbCtrl.Graft = append(pbCtrl.Graft, &gossipsub.ControlGraft{
			Topic: graft.Topic,
		})
	}

	// PRUNE
	for _, prune := range ctrl.Prune {
		pbPrune := &gossipsub.ControlPrune{
			Topic:   prune.Topic,
			Backoff: prune.Backoff,
		}
		for _, peer := range prune.Peers {
			pbPrune.Peers = append(pbPrune.Peers, &gossipsub.PeerInfo{
				PeerId:           peer.ID[:],
				SignedPeerRecord: peer.SignedPeerRecord,
			})
		}
		pbCtrl.Prune = append(pbCtrl.Prune, pbPrune)
	}

	return pbCtrl
}

// ============================================================================
//                              类型转换：Protobuf -> Go
// ============================================================================

// fromProtoRPC 将 protobuf 消息转换为 RPC
func (c *RPCCodec) fromProtoRPC(pbRPC *gossipsub.RPC) *RPC {
	rpc := &RPC{
		Subscriptions: make([]SubOpt, 0, len(pbRPC.Subscriptions)),
		Messages:      make([]*Message, 0, len(pbRPC.Publish)),
	}

	// 转换订阅列表
	for _, sub := range pbRPC.Subscriptions {
		rpc.Subscriptions = append(rpc.Subscriptions, SubOpt{
			Subscribe: sub.Subscribe,
			Topic:     sub.Topic,
		})
	}

	// 转换消息列表
	for _, msg := range pbRPC.Publish {
		rpc.Messages = append(rpc.Messages, c.fromProtoMessage(msg))
	}

	// 转换控制消息
	if pbRPC.Control != nil {
		rpc.Control = c.fromProtoControl(pbRPC.Control)
	}

	return rpc
}

// fromProtoMessage 将 protobuf 消息转换为 Message
//
// 注意：此函数不会对无效的 From 字段返回错误，但会标记 FromValid 标志。
// 调用方应该在 validateMessage 中检查 From 的有效性。
func (c *RPCCodec) fromProtoMessage(pbMsg *gossipsub.Message) *Message {
	msg := &Message{
		Topic:     pbMsg.Topic,
		Data:      pbMsg.Data,
		Signature: pbMsg.Signature,
		Key:       pbMsg.Key,
		Timestamp: time.Now(), // 接收时间
	}

	// 解析发送者 NodeID - 必须是 32 字节
	if len(pbMsg.From) == 32 {
		copy(msg.From[:], pbMsg.From)
	}
	// 如果不是 32 字节，msg.From 保持零值，validateMessage 会拒绝

	// 解析序列号
	if len(pbMsg.Seqno) >= 8 {
		msg.Sequence = binary.BigEndian.Uint64(pbMsg.Seqno)
	}

	// 生成消息 ID
	msg.ID = ComputeMessageID(msg)

	return msg
}

// fromProtoControl 将 protobuf 控制消息转换为 ControlMessage
func (c *RPCCodec) fromProtoControl(pbCtrl *gossipsub.ControlMessage) *ControlMessage {
	ctrl := &ControlMessage{
		IHave: make([]ControlIHaveMessage, 0, len(pbCtrl.Ihave)),
		IWant: make([]ControlIWantMessage, 0, len(pbCtrl.Iwant)),
		Graft: make([]ControlGraftMessage, 0, len(pbCtrl.Graft)),
		Prune: make([]ControlPruneMessage, 0, len(pbCtrl.Prune)),
	}

	// IHAVE
	for _, ihave := range pbCtrl.Ihave {
		ctrl.IHave = append(ctrl.IHave, ControlIHaveMessage{
			Topic:      ihave.Topic,
			MessageIDs: ihave.MessageIds,
		})
	}

	// IWANT
	for _, iwant := range pbCtrl.Iwant {
		ctrl.IWant = append(ctrl.IWant, ControlIWantMessage{
			MessageIDs: iwant.MessageIds,
		})
	}

	// GRAFT
	for _, graft := range pbCtrl.Graft {
		ctrl.Graft = append(ctrl.Graft, ControlGraftMessage{
			Topic: graft.Topic,
		})
	}

	// PRUNE
	for _, prune := range pbCtrl.Prune {
		pruneMsg := ControlPruneMessage{
			Topic:   prune.Topic,
			Backoff: prune.Backoff,
			Peers:   make([]PeerInfo, 0, len(prune.Peers)),
		}
		for _, peer := range prune.Peers {
			var nodeID types.NodeID
			if len(peer.PeerId) == 32 {
				copy(nodeID[:], peer.PeerId)
			}
			pruneMsg.Peers = append(pruneMsg.Peers, PeerInfo{
				ID:               nodeID,
				SignedPeerRecord: peer.SignedPeerRecord,
			})
		}
		ctrl.Prune = append(ctrl.Prune, pruneMsg)
	}

	return ctrl
}

// ============================================================================
//                              流式编解码
// ============================================================================

// WriteRPC 写入 RPC 到流
func WriteRPC(w io.Writer, rpc *RPC) error {
	codec := NewRPCCodec()
	data, err := codec.EncodeRPC(rpc)
	if err != nil {
		return err
	}

	// 写入长度前缀（4字节大端序）
	lenBuf := make([]byte, 4)
	binary.BigEndian.PutUint32(lenBuf, uint32(len(data)))
	if _, err := w.Write(lenBuf); err != nil {
		return ErrWriteFailed
	}

	// 写入数据
	if _, err := w.Write(data); err != nil {
		return ErrWriteFailed
	}

	return nil
}

// ReadRPC 从流读取 RPC
func ReadRPC(r io.Reader) (*RPC, error) {
	// 读取长度前缀
	lenBuf := make([]byte, 4)
	if _, err := io.ReadFull(r, lenBuf); err != nil {
		return nil, ErrReadFailed
	}

	length := binary.BigEndian.Uint32(lenBuf)
	if length > MaxRPCSize {
		return nil, ErrMessageTooLarge
	}

	// 读取数据
	data := make([]byte, length)
	if _, err := io.ReadFull(r, data); err != nil {
		return nil, ErrReadFailed
	}

	// 解码 RPC
	codec := NewRPCCodec()
	return codec.DecodeRPC(data)
}

// ============================================================================
//                              消息 ID 计算
// ============================================================================

// ComputeMessageID 计算消息 ID
//
// 消息 ID 基于 (from, seqno) 计算，用于去重
func ComputeMessageID(msg *Message) []byte {
	// 使用 from + seqno 作为消息 ID
	id := make([]byte, 40) // 32 (NodeID) + 8 (seqno)
	copy(id[:32], msg.From[:])
	binary.BigEndian.PutUint64(id[32:], msg.Sequence)
	return id
}

// ============================================================================
//                              向后兼容的辅助方法
// ============================================================================

// encodeLength 使用变长编码（用于自定义格式的向后兼容）
func encodeLength(length int) []byte {
	buf := make([]byte, 0, 4)
	for length >= 0x80 {
		buf = append(buf, byte(length)|0x80)
		length >>= 7
	}
	buf = append(buf, byte(length))
	return buf
}

// decodeLength 解码变长编码
func decodeLength(data []byte) (int, int) {
	length := 0
	shift := 0
	for i, b := range data {
		length |= int(b&0x7F) << shift
		if b < 0x80 {
			return length, i + 1
		}
		shift += 7
		if shift >= 32 {
			return 0, 0
		}
	}
	return 0, 0
}

// encodeString 编码字符串
func encodeString(s string) []byte {
	buf := encodeLength(len(s))
	return append(buf, []byte(s)...)
}

// encodeBytes 编码字节切片
func encodeBytes(b []byte) []byte {
	buf := encodeLength(len(b))
	return append(buf, b...)
}
