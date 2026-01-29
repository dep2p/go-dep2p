package addressbook

import (
	"encoding/binary"
	"fmt"
	"io"

	"github.com/dep2p/go-dep2p/pkg/interfaces"
	pb "github.com/dep2p/go-dep2p/pkg/lib/proto/addressbook"
	"github.com/dep2p/go-dep2p/pkg/protocol"
	"google.golang.org/protobuf/proto"
)

// 协议 ID 模板（使用统一定义）
// %s 替换为 RealmID
var (
	// ProtocolIDAddressBook 地址簿协议 ID
	ProtocolIDAddressBook = protocol.RealmAddressbookFormat
)

// 消息大小限制
const (
	// MaxMessageSize 最大消息大小 (64KB)
	MaxMessageSize = 64 * 1024

	// MaxBatchSize 最大批量查询数量
	MaxBatchSize = 100
)

// FormatProtocolID 格式化协议 ID
func FormatProtocolID(realmID string) string {
	return fmt.Sprintf(ProtocolIDAddressBook, realmID)
}

// ============================================================================
//                              消息读写
// ============================================================================

// WriteMessage 写入 protobuf 消息到流
//
// 格式: [4字节长度][消息体]
func WriteMessage(stream interfaces.Stream, msg *pb.AddressBookMessage) error {
	data, err := proto.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal message: %w", err)
	}

	if len(data) > MaxMessageSize {
		return fmt.Errorf("message too large: %d > %d", len(data), MaxMessageSize)
	}

	// 写入长度前缀 (4字节，大端)
	lenBuf := make([]byte, 4)
	binary.BigEndian.PutUint32(lenBuf, uint32(len(data)))

	if _, err := stream.Write(lenBuf); err != nil {
		return fmt.Errorf("write length: %w", err)
	}

	// 写入消息体
	if _, err := stream.Write(data); err != nil {
		return fmt.Errorf("write data: %w", err)
	}

	return nil
}

// ReadMessage 从流读取 protobuf 消息
//
// 格式: [4字节长度][消息体]
func ReadMessage(stream interfaces.Stream) (*pb.AddressBookMessage, error) {
	// 读取长度前缀
	lenBuf := make([]byte, 4)
	if _, err := io.ReadFull(stream, lenBuf); err != nil {
		return nil, fmt.Errorf("read length: %w", err)
	}

	length := binary.BigEndian.Uint32(lenBuf)
	if length > MaxMessageSize {
		return nil, fmt.Errorf("message too large: %d > %d", length, MaxMessageSize)
	}

	// 读取消息体
	data := make([]byte, length)
	if _, err := io.ReadFull(stream, data); err != nil {
		return nil, fmt.Errorf("read data: %w", err)
	}

	// 解析消息
	msg := &pb.AddressBookMessage{}
	if err := proto.Unmarshal(data, msg); err != nil {
		return nil, fmt.Errorf("unmarshal message: %w", err)
	}

	return msg, nil
}

// ============================================================================
//                              消息构建辅助函数
// ============================================================================

// NewRegisterMessage 创建注册消息
func NewRegisterMessage(reg *pb.AddressRegister) *pb.AddressBookMessage {
	return &pb.AddressBookMessage{
		Type:    pb.AddressBookMessage_REGISTER,
		Payload: &pb.AddressBookMessage_Register{Register: reg},
	}
}

// NewRegisterResponseMessage 创建注册响应消息
func NewRegisterResponseMessage(resp *pb.AddressRegisterResponse) *pb.AddressBookMessage {
	return &pb.AddressBookMessage{
		Type:    pb.AddressBookMessage_REGISTER_RESPONSE,
		Payload: &pb.AddressBookMessage_RegisterResponse{RegisterResponse: resp},
	}
}

// NewQueryMessage 创建查询消息
func NewQueryMessage(query *pb.AddressQuery) *pb.AddressBookMessage {
	return &pb.AddressBookMessage{
		Type:    pb.AddressBookMessage_QUERY,
		Payload: &pb.AddressBookMessage_Query{Query: query},
	}
}

// NewResponseMessage 创建查询响应消息
func NewResponseMessage(resp *pb.AddressResponse) *pb.AddressBookMessage {
	return &pb.AddressBookMessage{
		Type:    pb.AddressBookMessage_RESPONSE,
		Payload: &pb.AddressBookMessage_Response{Response: resp},
	}
}

// NewBatchQueryMessage 创建批量查询消息
func NewBatchQueryMessage(query *pb.BatchAddressQuery) *pb.AddressBookMessage {
	return &pb.AddressBookMessage{
		Type:    pb.AddressBookMessage_BATCH_QUERY,
		Payload: &pb.AddressBookMessage_BatchQuery{BatchQuery: query},
	}
}

// NewBatchResponseMessage 创建批量查询响应消息
func NewBatchResponseMessage(resp *pb.BatchAddressResponse) *pb.AddressBookMessage {
	return &pb.AddressBookMessage{
		Type:    pb.AddressBookMessage_BATCH_RESPONSE,
		Payload: &pb.AddressBookMessage_BatchResponse{BatchResponse: resp},
	}
}

// NewUpdateMessage 创建更新通知消息
func NewUpdateMessage(update *pb.AddressUpdate) *pb.AddressBookMessage {
	return &pb.AddressBookMessage{
		Type:    pb.AddressBookMessage_UPDATE,
		Payload: &pb.AddressBookMessage_Update{Update: update},
	}
}
