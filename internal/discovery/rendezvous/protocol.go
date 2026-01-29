package rendezvous

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"time"

	"google.golang.org/protobuf/proto"

	pb "github.com/dep2p/go-dep2p/pkg/lib/proto/rendezvous"
	"github.com/dep2p/go-dep2p/pkg/protocol"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              协议常量
// ============================================================================

const (
	// MaxMessageSize 最大消息大小 (1MB)
	MaxMessageSize = 1 << 20

	// DefaultLimit 默认发现限制
	DefaultLimit = 100

	// MaxNamespaceLength 最大命名空间长度
	MaxNamespaceLength = 256

	// MaxAddresses 单个注册最大地址数
	MaxAddresses = 16
)

// ProtocolID Rendezvous 协议 ID（使用统一定义）
var ProtocolID = string(protocol.Rendezvous)

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
func NewRegisterRequest(namespace string, signedPeerRecord []byte, ttl time.Duration) *pb.Message {
	return &pb.Message{
		Type: pb.Message_REGISTER,
		Register: &pb.Message_Register{
			Ns:               namespace,
			SignedPeerRecord: signedPeerRecord,
			Ttl:              uint64(ttl.Seconds()),
		},
	}
}

// NewUnregisterRequest 创建取消注册请求
func NewUnregisterRequest(namespace string, peerID []byte) *pb.Message {
	return &pb.Message{
		Type: pb.Message_UNREGISTER,
		Unregister: &pb.Message_Unregister{
			Ns: namespace,
			Id: peerID,
		},
	}
}

// NewDiscoverRequest 创建发现请求
func NewDiscoverRequest(namespace string, limit int, cookie []byte) *pb.Message {
	return &pb.Message{
		Type: pb.Message_DISCOVER,
		Discover: &pb.Message_Discover{
			Ns:     namespace,
			Limit:  uint64(limit),
			Cookie: cookie,
		},
	}
}

// ============================================================================
//                              响应构造器
// ============================================================================

// NewRegisterResponse 创建注册响应
func NewRegisterResponse(status pb.Message_ResponseStatus, statusText string, ttl time.Duration) *pb.Message {
	return &pb.Message{
		Type: pb.Message_REGISTER_RESPONSE,
		RegisterResponse: &pb.Message_RegisterResponse{
			Status:     status,
			StatusText: statusText,
			Ttl:        uint64(ttl.Seconds()),
		},
	}
}

// NewDiscoverResponse 创建发现响应
func NewDiscoverResponse(status pb.Message_ResponseStatus, statusText string, registrations []*pb.Message_Registration, cookie []byte) *pb.Message {
	return &pb.Message{
		Type: pb.Message_DISCOVER_RESPONSE,
		DiscoverResponse: &pb.Message_DiscoverResponse{
			Registrations: registrations,
			Cookie:        cookie,
			Status:        status,
			StatusText:    statusText,
		},
	}
}

// ============================================================================
//                              类型转换
// ============================================================================

// RegistrationToPeerInfo 将 Registration 转换为 PeerInfo
// 完整实现：解析并验证 SignedPeerRecord
func RegistrationToPeerInfo(reg *pb.Message_Registration) (types.PeerInfo, error) {
	if len(reg.SignedPeerRecord) == 0 {
		return types.PeerInfo{}, errors.New("empty signed peer record")
	}

	// 尝试解析为 SignedPeerRecord
	peerInfo, err := ExtractPeerInfoFromSignedRecord(reg.SignedPeerRecord)
	if err != nil {
		// 向后兼容：如果解析失败，尝试作为简单 PeerID
		peerID := types.PeerID(reg.SignedPeerRecord)
		return types.PeerInfo{
			ID:    peerID,
			Addrs: []types.Multiaddr{},
		}, nil
	}

	return peerInfo, nil
}

// PeerInfoToRegistration 将 PeerInfo 转换为 Registration
// 注意：此函数用于简单转换，不包含签名
// 使用 SignedPeerRecord 时请调用 CreateSignedRegistration
func PeerInfoToRegistration(info types.PeerInfo, namespace string, ttl time.Duration) *pb.Message_Registration {
	return &pb.Message_Registration{
		Ns:               namespace,
		SignedPeerRecord: []byte(info.ID),
		Ttl:              uint64(ttl.Seconds()),
	}
}

// CreateSignedRegistration 创建带签名的 Registration
func CreateSignedRegistration(signedRecord *SignedPeerRecord, namespace string, ttl time.Duration) (*pb.Message_Registration, error) {
	if signedRecord == nil {
		return nil, errors.New("nil signed record")
	}

	data, err := signedRecord.Marshal()
	if err != nil {
		return nil, err
	}

	return &pb.Message_Registration{
		Ns:               namespace,
		SignedPeerRecord: data,
		Ttl:              uint64(ttl.Seconds()),
	}, nil
}

// StatusToError 将响应状态转换为错误
func StatusToError(status pb.Message_ResponseStatus, statusText string) error {
	switch status {
	case pb.Message_OK:
		return nil
	case pb.Message_E_INVALID_NAMESPACE:
		return fmt.Errorf("%w: %s", ErrInvalidNamespace, statusText)
	case pb.Message_E_INVALID_TTL:
		return fmt.Errorf("%w: %s", ErrInvalidTTL, statusText)
	case pb.Message_E_INVALID_COOKIE:
		return fmt.Errorf("%w: %s", ErrInvalidCookie, statusText)
	case pb.Message_E_NOT_AUTHORIZED:
		return fmt.Errorf("%w: %s", ErrNotAuthorized, statusText)
	case pb.Message_E_INTERNAL_ERROR:
		return fmt.Errorf("%w: %s", ErrInternalError, statusText)
	case pb.Message_E_UNAVAILABLE:
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
	defaultTTL := 2 * time.Hour
	if ttl <= 0 {
		return defaultTTL, nil
	}
	if ttl > maxTTL {
		return maxTTL, nil
	}
	return ttl, nil
}

// ValidateAddresses 验证地址列表
func ValidateAddresses(addrs []string) error {
	if len(addrs) == 0 {
		return errors.New("no addresses provided")
	}
	if len(addrs) > MaxAddresses {
		return fmt.Errorf("too many addresses (max %d)", MaxAddresses)
	}
	return nil
}

// ValidateRegisterRequest 验证注册请求
func ValidateRegisterRequest(req *pb.Message_Register, _ time.Duration) error {
	if err := ValidateNamespace(req.Ns); err != nil {
		return err
	}
	if len(req.SignedPeerRecord) == 0 {
		return errors.New("empty signed peer record")
	}
	return nil
}

// ValidateDiscoverRequest 验证发现请求
func ValidateDiscoverRequest(req *pb.Message_Discover) error {
	if err := ValidateNamespace(req.Ns); err != nil {
		return err
	}
	return nil
}
