// Package rendezvous 提供基于主题的节点发现功能
//
// Rendezvous 协议允许节点基于命名空间（主题）相互发现，
// 是一种轻量级的发现机制，不需要完整的 DHT 支持。
package rendezvous

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/dep2p/go-dep2p/pkg/interfaces/endpoint"
	discoveryif "github.com/dep2p/go-dep2p/pkg/interfaces/discovery"
	"github.com/dep2p/go-dep2p/pkg/protocolids"
	pb "github.com/dep2p/go-dep2p/pkg/proto/rendezvous"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              协议常量
// ============================================================================

const (
	// MaxMessageSize 最大消息大小 (1MB)
	MaxMessageSize = 1 << 20
)

// 引用 pkg/protocolids 唯一真源
var (
	// ProtocolIDString Rendezvous 协议标识符字符串 (v1.1 scope: sys)
	ProtocolIDString = string(protocolids.SysRendezvous)

	// ProtocolID Rendezvous 协议标识符
	ProtocolID = endpoint.ProtocolID(protocolids.SysRendezvous)

	// DefaultLimit 默认发现限制
	DefaultLimit = 100

	// DefaultTTL 默认 TTL
	DefaultTTL = 2 * time.Hour

	// MaxNamespaceLength 最大命名空间长度
	MaxNamespaceLength = 256

	// MaxAddresses 单个注册最大地址数
	MaxAddresses = 16
)

// ============================================================================
//                              错误定义
// ============================================================================

var (
	// ErrInvalidNamespace 无效的命名空间
	ErrInvalidNamespace = errors.New("invalid namespace")

	// ErrInvalidTTL 无效的 TTL
	ErrInvalidTTL = errors.New("invalid TTL")

	// ErrInvalidCookie 无效的分页 cookie
	ErrInvalidCookie = errors.New("invalid cookie")

	// ErrNotAuthorized 未授权
	ErrNotAuthorized = errors.New("not authorized")

	// ErrInternalError 内部错误
	ErrInternalError = errors.New("internal error")

	// ErrUnavailable 服务不可用
	ErrUnavailable = errors.New("service unavailable")

	// ErrMessageTooLarge 消息过大
	ErrMessageTooLarge = errors.New("message too large")

	// ErrInvalidMessage 无效消息
	ErrInvalidMessage = errors.New("invalid message")
)

// ============================================================================
//                              消息编解码
// ============================================================================

// WriteMessage 写入消息到 writer
func WriteMessage(w io.Writer, msg *pb.Message) error {
	data, err := proto.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	if len(data) > MaxMessageSize {
		return ErrMessageTooLarge
	}

	// 写入长度前缀 (4 字节大端)
	lenBuf := make([]byte, 4)
	binary.BigEndian.PutUint32(lenBuf, uint32(len(data)))
	if _, err := w.Write(lenBuf); err != nil {
		return fmt.Errorf("failed to write length: %w", err)
	}

	// 写入消息体
	if _, err := w.Write(data); err != nil {
		return fmt.Errorf("failed to write message: %w", err)
	}

	return nil
}

// ReadMessage 从 reader 读取消息
func ReadMessage(r io.Reader) (*pb.Message, error) {
	// 读取长度前缀
	lenBuf := make([]byte, 4)
	if _, err := io.ReadFull(r, lenBuf); err != nil {
		return nil, fmt.Errorf("failed to read length: %w", err)
	}

	length := binary.BigEndian.Uint32(lenBuf)
	if length > MaxMessageSize {
		return nil, ErrMessageTooLarge
	}

	// 读取消息体
	data := make([]byte, length)
	if _, err := io.ReadFull(r, data); err != nil {
		return nil, fmt.Errorf("failed to read message: %w", err)
	}

	// 反序列化
	msg := &pb.Message{}
	if err := proto.Unmarshal(data, msg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal message: %w", err)
	}

	return msg, nil
}

// ============================================================================
//                              请求构造器
// ============================================================================

// NewRegisterRequest 创建注册请求
func NewRegisterRequest(namespace string, peerID types.NodeID, addrs []string, ttl time.Duration) *pb.Message {
	return &pb.Message{
		Type: pb.MessageType_MESSAGE_TYPE_REGISTER,
		Register: &pb.Register{
			Namespace: namespace,
			PeerId:    peerID[:],
			Addrs:     addrs,
			Ttl:       uint64(ttl.Seconds()),
		},
	}
}

// NewUnregisterRequest 创建取消注册请求
func NewUnregisterRequest(namespace string, peerID types.NodeID) *pb.Message {
	return &pb.Message{
		Type: pb.MessageType_MESSAGE_TYPE_UNREGISTER,
		Unregister: &pb.Unregister{
			Namespace: namespace,
			PeerId:    peerID[:],
		},
	}
}

// NewDiscoverRequest 创建发现请求
func NewDiscoverRequest(namespace string, limit int, cookie []byte) *pb.Message {
	return &pb.Message{
		Type: pb.MessageType_MESSAGE_TYPE_DISCOVER,
		Discover: &pb.Discover{
			Namespace: namespace,
			Limit:     uint64(limit),
			Cookie:    cookie,
		},
	}
}

// ============================================================================
//                              响应构造器
// ============================================================================

// NewRegisterResponse 创建注册响应
func NewRegisterResponse(status pb.ResponseStatus, statusText string, ttl time.Duration) *pb.Message {
	return &pb.Message{
		Type: pb.MessageType_MESSAGE_TYPE_REGISTER_RESPONSE,
		RegisterResponse: &pb.RegisterResponse{
			Status:     status,
			StatusText: statusText,
			Ttl:        uint64(ttl.Seconds()),
		},
	}
}

// NewDiscoverResponse 创建发现响应
func NewDiscoverResponse(status pb.ResponseStatus, statusText string, registrations []*pb.Registration, cookie []byte) *pb.Message {
	return &pb.Message{
		Type: pb.MessageType_MESSAGE_TYPE_DISCOVER_RESPONSE,
		DiscoverResponse: &pb.DiscoverResponse{
			Status:        status,
			StatusText:    statusText,
			Registrations: registrations,
			Cookie:        cookie,
		},
	}
}

// ============================================================================
//                              类型转换
// ============================================================================

// NodeIDSize NodeID 字节大小
const NodeIDSize = 32

// RegistrationToPeerInfo 将 Registration 转换为 PeerInfo
func RegistrationToPeerInfo(reg *pb.Registration) (discoveryif.PeerInfo, error) {
	if len(reg.PeerId) != NodeIDSize {
		return discoveryif.PeerInfo{}, fmt.Errorf("invalid peer ID size: %d", len(reg.PeerId))
	}

	var nodeID types.NodeID
	copy(nodeID[:], reg.PeerId)

	return discoveryif.PeerInfo{
		ID:    nodeID,
		Addrs: types.StringsToMultiaddrs(reg.Addrs),
	}, nil
}

// PeerInfoToRegistration 将 PeerInfo 转换为 Registration
func PeerInfoToRegistration(info discoveryif.PeerInfo, namespace string, ttl time.Duration, registeredAt time.Time) *pb.Registration {
	return &pb.Registration{
		Namespace:    namespace,
		PeerId:       info.ID[:],
		Addrs:        types.MultiaddrsToStrings(info.Addrs),
		Ttl:          uint64(ttl.Seconds()),
		RegisteredAt: timestamppb.New(registeredAt),
	}
}

// StatusToError 将响应状态转换为错误
func StatusToError(status pb.ResponseStatus, statusText string) error {
	switch status {
	case pb.ResponseStatus_RESPONSE_STATUS_OK:
		return nil
	case pb.ResponseStatus_RESPONSE_STATUS_E_INVALID_NAMESPACE:
		return fmt.Errorf("%w: %s", ErrInvalidNamespace, statusText)
	case pb.ResponseStatus_RESPONSE_STATUS_E_INVALID_TTL:
		return fmt.Errorf("%w: %s", ErrInvalidTTL, statusText)
	case pb.ResponseStatus_RESPONSE_STATUS_E_INVALID_COOKIE:
		return fmt.Errorf("%w: %s", ErrInvalidCookie, statusText)
	case pb.ResponseStatus_RESPONSE_STATUS_E_NOT_AUTHORIZED:
		return fmt.Errorf("%w: %s", ErrNotAuthorized, statusText)
	case pb.ResponseStatus_RESPONSE_STATUS_E_INTERNAL_ERROR:
		return fmt.Errorf("%w: %s", ErrInternalError, statusText)
	case pb.ResponseStatus_RESPONSE_STATUS_E_UNAVAILABLE:
		return fmt.Errorf("%w: %s", ErrUnavailable, statusText)
	default:
		return fmt.Errorf("unknown status: %v", status)
	}
}

// ============================================================================
//                              验证函数
// ============================================================================

// ValidateNamespace 验证命名空间
func ValidateNamespace(namespace string) error {
	if namespace == "" {
		return fmt.Errorf("%w: empty namespace", ErrInvalidNamespace)
	}
	if len(namespace) > MaxNamespaceLength {
		return fmt.Errorf("%w: namespace too long (max %d)", ErrInvalidNamespace, MaxNamespaceLength)
	}
	return nil
}

// ValidateTTL 验证 TTL
func ValidateTTL(ttl time.Duration, maxTTL time.Duration) (time.Duration, error) {
	if ttl <= 0 {
		return DefaultTTL, nil
	}
	if ttl > maxTTL {
		return maxTTL, nil
	}
	return ttl, nil
}

// ValidateAddresses 验证地址列表
func ValidateAddresses(addrs []string) error {
	if len(addrs) == 0 {
		return fmt.Errorf("no addresses provided")
	}
	if len(addrs) > MaxAddresses {
		return fmt.Errorf("too many addresses (max %d)", MaxAddresses)
	}
	return nil
}

// ValidateRegisterRequest 验证注册请求
func ValidateRegisterRequest(req *pb.Register, _ time.Duration) error {
	if err := ValidateNamespace(req.Namespace); err != nil {
		return err
	}
	if len(req.PeerId) != NodeIDSize {
		return fmt.Errorf("invalid peer ID size: %d", len(req.PeerId))
	}
	if err := ValidateAddresses(req.Addrs); err != nil {
		return err
	}
	return nil
}

// ValidateDiscoverRequest 验证发现请求
func ValidateDiscoverRequest(req *pb.Discover) error {
	if err := ValidateNamespace(req.Namespace); err != nil {
		return err
	}
	return nil
}

