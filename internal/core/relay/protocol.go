// Package relay 提供中继服务协议实现
package relay

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"time"

	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              协议常量
// ============================================================================

const (
	// ProtocolVersion 协议版本
	ProtocolVersion = 1

	// MaxMessageSize 最大消息大小
	MaxMessageSize = 1024 * 64 // 64KB

	// MaxAddrs 最大地址数
	MaxAddrs = 16

	// MaxFrameSize 最大数据帧大小
	MaxFrameSize = 1024 * 32 // 32KB
)

// ============================================================================
//                              消息类型
// ============================================================================

// MessageType 消息类型
type MessageType uint8

const (
	// MsgHopReserve HOP 预留请求消息
	MsgHopReserve MessageType = 1
	// MsgHopReserveOK HOP 预留成功响应
	MsgHopReserveOK MessageType = 2
	// MsgHopReserveError HOP 预留失败响应
	MsgHopReserveError MessageType = 3
	// MsgHopConnect HOP 连接请求消息
	MsgHopConnect MessageType = 4
	// MsgHopConnectOK HOP 连接成功响应
	MsgHopConnectOK MessageType = 5
	// MsgHopConnectError HOP 连接失败响应
	MsgHopConnectError MessageType = 6

	// MsgStopConnect STOP 连接请求消息
	MsgStopConnect MessageType = 10
	// MsgStopConnectOK STOP 连接成功响应
	MsgStopConnectOK MessageType = 11
	// MsgStopConnectError STOP 连接失败响应
	MsgStopConnectError MessageType = 12

	// MsgStatus 状态消息
	MsgStatus MessageType = 20

	// MsgData 数据帧消息
	MsgData MessageType = 100
)

// ============================================================================
//                              错误码
// ============================================================================

// ErrorCode 错误码
type ErrorCode uint16

const (
	// ErrCodeNone 无错误
	ErrCodeNone ErrorCode = 0
	// ErrCodeMalformedMessage 消息格式错误
	ErrCodeMalformedMessage ErrorCode = 100
	// ErrCodeUnexpectedMessage 意外的消息类型
	ErrCodeUnexpectedMessage ErrorCode = 101
	// ErrCodeResourceLimitHit 资源限制达到
	ErrCodeResourceLimitHit ErrorCode = 200
	// ErrCodeNoReservation 无预留
	ErrCodeNoReservation ErrorCode = 201
	// ErrCodeConnectionFailed 连接失败
	ErrCodeConnectionFailed ErrorCode = 300

	// =========================================================================
	// 兼容保留（旧/扩展错误码，当前 relay server 实现不一定会发出）
	// =========================================================================

	// ErrCodePeerNotFound 节点未找到（兼容保留）
	ErrCodePeerNotFound ErrorCode = 301
	// ErrCodePeerNoReservation 节点无预留（兼容保留）
	ErrCodePeerNoReservation ErrorCode = 302
	// ErrCodeRelayBusy 中继繁忙（兼容保留）
	ErrCodeRelayBusy ErrorCode = 400
	// ErrCodePermissionDenied 权限拒绝（IMPL-1227: PSK 验证失败）
	ErrCodePermissionDenied ErrorCode = 401
	// ErrCodeProtocolNotAllowed 协议不允许（IMPL-1227: 协议白名单）
	ErrCodeProtocolNotAllowed ErrorCode = 402
)

// String 返回错误码描述
func (c ErrorCode) String() string {
	switch c {
	case ErrCodeNone:
		return "no error"
	case ErrCodeMalformedMessage:
		return "malformed message"
	case ErrCodeUnexpectedMessage:
		return "unexpected message"
	case ErrCodeResourceLimitHit:
		return "resource limit reached"
	case ErrCodeNoReservation:
		return "no reservation"
	case ErrCodePermissionDenied:
		return "permission denied"
	case ErrCodeProtocolNotAllowed:
		return "protocol not allowed"
	case ErrCodeConnectionFailed:
		return "connection failed"
	case ErrCodePeerNotFound:
		return "peer not found"
	case ErrCodePeerNoReservation:
		return "peer has no reservation"
	case ErrCodeRelayBusy:
		return "relay busy"
	default:
		return "unknown error"
	}
}

// ToError 转换为 Go error
func (c ErrorCode) ToError() error {
	if c == ErrCodeNone {
		return nil
	}
	return errors.New(c.String())
}

// ============================================================================
//                              消息结构
// ============================================================================

// Message 中继协议消息
type Message struct {
	Type    MessageType
	Payload []byte
}

// ReserveRequest 预留请求
type ReserveRequest struct {
	// TTL 请求的预留时长（秒）
	TTL uint32
}

// ReserveResponse 预留响应
type ReserveResponse struct {
	// Status 状态（OK 或 Error）
	Status MessageType

	// TTL 授予的预留时长（秒）
	TTL uint32

	// Slots 授予的槽位数
	Slots uint16

	// Addrs 中继地址列表
	Addrs []string

	// ErrorCode 错误码（如果 Status 是 Error）
	ErrorCode ErrorCode
}

// ConnectRequest 连接请求
type ConnectRequest struct {
	// DestPeer 目标节点 ID
	DestPeer types.NodeID
}

// ConnectResponse 连接响应
type ConnectResponse struct {
	// Status 状态
	Status MessageType

	// ErrorCode 错误码
	ErrorCode ErrorCode
}

// StopConnectRequest STOP 连接请求（发给目标节点）
type StopConnectRequest struct {
	// Relay 中继节点 ID
	Relay types.NodeID

	// Src 源节点 ID
	Src types.NodeID
}

// StopConnectResponse STOP 连接响应
type StopConnectResponse struct {
	// Status 状态
	Status MessageType

	// ErrorCode 错误码
	ErrorCode ErrorCode
}

// StatusMessage 状态消息
type StatusMessage struct {
	// Reservations 当前预留数
	Reservations uint32

	// Circuits 当前活跃电路数
	Circuits uint32

	// DataRate 当前数据速率
	DataRate uint64
}

// DataFrame 数据帧
type DataFrame struct {
	// StreamID 流 ID
	StreamID uint32

	// Flags 标志
	Flags uint8

	// Data 数据
	Data []byte
}

// 数据帧标志
const (
	FlagFin uint8 = 1 << 0 // 流结束
	FlagRst uint8 = 1 << 1 // 重置流
)

// ============================================================================
//                              编解码
// ============================================================================

// EncodeReserveRequest 编码预留请求
func EncodeReserveRequest(req *ReserveRequest) []byte {
	buf := new(bytes.Buffer)

	// 消息类型
	_ = buf.WriteByte(byte(MsgHopReserve))

	// 版本
	_ = buf.WriteByte(ProtocolVersion)

	// TTL
	_ = binary.Write(buf, binary.BigEndian, req.TTL)

	return buf.Bytes()
}

// DecodeReserveRequest 解码预留请求
func DecodeReserveRequest(data []byte) (*ReserveRequest, error) {
	if len(data) < 6 {
		return nil, errors.New("data too short for reserve request")
	}

	buf := bytes.NewReader(data)

	// 消息类型
	msgType, err := buf.ReadByte()
	if err != nil {
		return nil, err
	}
	if MessageType(msgType) != MsgHopReserve {
		return nil, errors.New("invalid message type")
	}

	// 版本
	if _, err := buf.ReadByte(); err != nil {
		return nil, err
	}

	// TTL
	var ttl uint32
	if err := binary.Read(buf, binary.BigEndian, &ttl); err != nil {
		return nil, err
	}

	return &ReserveRequest{TTL: ttl}, nil
}

// EncodeReserveResponse 编码预留响应
func EncodeReserveResponse(resp *ReserveResponse) []byte {
	buf := new(bytes.Buffer)

	// 消息类型
	_ = buf.WriteByte(byte(resp.Status))

	// 版本
	_ = buf.WriteByte(ProtocolVersion)

	if resp.Status == MsgHopReserveOK {
		// TTL
		_ = binary.Write(buf, binary.BigEndian, resp.TTL)

		// Slots
		_ = binary.Write(buf, binary.BigEndian, resp.Slots)

		// 地址数量（安全转换：先限制范围再转换）
		addrCount := len(resp.Addrs)
		if addrCount > MaxAddrs {
			addrCount = MaxAddrs
		}
		_ = buf.WriteByte(uint8(addrCount)) // #nosec G115 -- addrCount is bounded by MaxAddrs (16)

		// 地址列表
		for i := 0; i < addrCount; i++ {
			addr := resp.Addrs[i]
			addrLen := uint16(len(addr)) // #nosec G115 -- address length is bounded by protocol
			_ = binary.Write(buf, binary.BigEndian, addrLen)
			_, _ = buf.WriteString(addr)
		}
	} else {
		// 错误码
		_ = binary.Write(buf, binary.BigEndian, resp.ErrorCode)
	}

	return buf.Bytes()
}

// DecodeReserveResponse 解码预留响应
func DecodeReserveResponse(data []byte) (*ReserveResponse, error) {
	if len(data) < 2 {
		return nil, errors.New("data too short for reserve response")
	}

	buf := bytes.NewReader(data)

	// 消息类型
	msgType, err := buf.ReadByte()
	if err != nil {
		return nil, err
	}

	// 版本
	if _, err := buf.ReadByte(); err != nil {
		return nil, err
	}

	resp := &ReserveResponse{Status: MessageType(msgType)}

	if resp.Status == MsgHopReserveOK {
		// TTL
		if err := binary.Read(buf, binary.BigEndian, &resp.TTL); err != nil {
			return nil, err
		}

		// Slots
		if err := binary.Read(buf, binary.BigEndian, &resp.Slots); err != nil {
			return nil, err
		}

		// 地址数量
		addrCount, err := buf.ReadByte()
		if err != nil {
			return nil, err
		}

		// 地址列表
		resp.Addrs = make([]string, addrCount)
		for i := uint8(0); i < addrCount; i++ {
			var addrLen uint16
			if err := binary.Read(buf, binary.BigEndian, &addrLen); err != nil {
				return nil, err
			}
			addrBytes := make([]byte, addrLen)
			if _, err := buf.Read(addrBytes); err != nil {
				return nil, err
			}
			resp.Addrs[i] = string(addrBytes)
		}
	} else {
		// 错误码
		if err := binary.Read(buf, binary.BigEndian, &resp.ErrorCode); err != nil {
			return nil, err
		}
	}

	return resp, nil
}

// EncodeConnectRequest 编码连接请求
func EncodeConnectRequest(req *ConnectRequest) []byte {
	buf := new(bytes.Buffer)

	// 消息类型
	buf.WriteByte(byte(MsgHopConnect))

	// 版本
	buf.WriteByte(ProtocolVersion)

	// 目标节点 ID
	buf.Write(req.DestPeer[:])

	return buf.Bytes()
}

// DecodeConnectRequest 解码连接请求
func DecodeConnectRequest(data []byte) (*ConnectRequest, error) {
	if len(data) < 2+32 { // NodeID is 32 bytes
		return nil, errors.New("data too short for connect request")
	}

	buf := bytes.NewReader(data)

	// 消息类型
	msgType, err := buf.ReadByte()
	if err != nil {
		return nil, err
	}
	if MessageType(msgType) != MsgHopConnect {
		return nil, errors.New("invalid message type")
	}

	// 版本
	if _, err := buf.ReadByte(); err != nil {
		return nil, err
	}

	// 目标节点 ID
	req := &ConnectRequest{}
	if _, err := buf.Read(req.DestPeer[:]); err != nil {
		return nil, err
	}

	return req, nil
}

// EncodeConnectResponse 编码连接响应
func EncodeConnectResponse(resp *ConnectResponse) []byte {
	buf := new(bytes.Buffer)

	// 消息类型
	_ = buf.WriteByte(byte(resp.Status))

	// 版本
	_ = buf.WriteByte(ProtocolVersion)

	if resp.Status != MsgHopConnectOK {
		// 错误码
		_ = binary.Write(buf, binary.BigEndian, resp.ErrorCode)
	}

	return buf.Bytes()
}

// DecodeConnectResponse 解码连接响应
func DecodeConnectResponse(data []byte) (*ConnectResponse, error) {
	if len(data) < 2 {
		return nil, errors.New("data too short for connect response")
	}

	buf := bytes.NewReader(data)

	// 消息类型
	msgType, err := buf.ReadByte()
	if err != nil {
		return nil, err
	}

	// 版本
	if _, err := buf.ReadByte(); err != nil {
		return nil, err
	}

	resp := &ConnectResponse{Status: MessageType(msgType)}

	if resp.Status != MsgHopConnectOK {
		if err := binary.Read(buf, binary.BigEndian, &resp.ErrorCode); err != nil {
			return nil, err
		}
	}

	return resp, nil
}

// EncodeStopConnectRequest 编码 STOP 连接请求
func EncodeStopConnectRequest(req *StopConnectRequest) []byte {
	buf := new(bytes.Buffer)

	// 消息类型
	buf.WriteByte(byte(MsgStopConnect))

	// 版本
	buf.WriteByte(ProtocolVersion)

	// 中继节点 ID
	buf.Write(req.Relay[:])

	// 源节点 ID
	buf.Write(req.Src[:])

	return buf.Bytes()
}

// DecodeStopConnectRequest 解码 STOP 连接请求
func DecodeStopConnectRequest(data []byte) (*StopConnectRequest, error) {
	if len(data) < 2+32*2 { // NodeID is 32 bytes
		return nil, errors.New("data too short for stop connect request")
	}

	buf := bytes.NewReader(data)

	// 消息类型
	msgType, err := buf.ReadByte()
	if err != nil {
		return nil, err
	}
	if MessageType(msgType) != MsgStopConnect {
		return nil, errors.New("invalid message type")
	}

	// 版本
	if _, err := buf.ReadByte(); err != nil {
		return nil, err
	}

	req := &StopConnectRequest{}
	if _, err := buf.Read(req.Relay[:]); err != nil {
		return nil, err
	}
	if _, err := buf.Read(req.Src[:]); err != nil {
		return nil, err
	}

	return req, nil
}

// EncodeStopConnectResponse 编码 STOP 连接响应
func EncodeStopConnectResponse(resp *StopConnectResponse) []byte {
	buf := new(bytes.Buffer)

	// 消息类型
	_ = buf.WriteByte(byte(resp.Status))

	// 版本
	_ = buf.WriteByte(ProtocolVersion)

	if resp.Status != MsgStopConnectOK {
		_ = binary.Write(buf, binary.BigEndian, resp.ErrorCode)
	}

	return buf.Bytes()
}

// DecodeStopConnectResponse 解码 STOP 连接响应
func DecodeStopConnectResponse(data []byte) (*StopConnectResponse, error) {
	if len(data) < 2 {
		return nil, errors.New("data too short for stop connect response")
	}

	buf := bytes.NewReader(data)

	msgType, err := buf.ReadByte()
	if err != nil {
		return nil, err
	}
	if _, err := buf.ReadByte(); err != nil { // version
		return nil, err
	}

	resp := &StopConnectResponse{Status: MessageType(msgType)}

	if resp.Status != MsgStopConnectOK {
		if err := binary.Read(buf, binary.BigEndian, &resp.ErrorCode); err != nil {
			return nil, err
		}
	}

	return resp, nil
}

// EncodeStatusMessage 编码状态消息
func EncodeStatusMessage(msg *StatusMessage) []byte {
	buf := new(bytes.Buffer)

	_ = buf.WriteByte(byte(MsgStatus))
	_ = buf.WriteByte(ProtocolVersion)

	_ = binary.Write(buf, binary.BigEndian, msg.Reservations)
	_ = binary.Write(buf, binary.BigEndian, msg.Circuits)
	_ = binary.Write(buf, binary.BigEndian, msg.DataRate)

	return buf.Bytes()
}

// DecodeStatusMessage 解码状态消息
func DecodeStatusMessage(data []byte) (*StatusMessage, error) {
	if len(data) < 18 {
		return nil, errors.New("data too short for status message")
	}

	buf := bytes.NewReader(data)

	if _, err := buf.ReadByte(); err != nil { // type
		return nil, err
	}
	if _, err := buf.ReadByte(); err != nil { // version
		return nil, err
	}

	msg := &StatusMessage{}
	if err := binary.Read(buf, binary.BigEndian, &msg.Reservations); err != nil {
		return nil, err
	}
	if err := binary.Read(buf, binary.BigEndian, &msg.Circuits); err != nil {
		return nil, err
	}
	if err := binary.Read(buf, binary.BigEndian, &msg.DataRate); err != nil {
		return nil, err
	}

	return msg, nil
}

// EncodeDataFrame 编码数据帧
func EncodeDataFrame(frame *DataFrame) []byte {
	buf := new(bytes.Buffer)

	_ = buf.WriteByte(byte(MsgData))

	// StreamID
	_ = binary.Write(buf, binary.BigEndian, frame.StreamID)

	// Flags
	_ = buf.WriteByte(frame.Flags)

	// Data length (安全转换：frame.Data 长度受 MaxFrameSize 限制)
	dataLen := len(frame.Data)
	if dataLen > MaxFrameSize {
		dataLen = MaxFrameSize
	}
	_ = binary.Write(buf, binary.BigEndian, uint32(dataLen)) // #nosec G115 -- bounded by MaxFrameSize

	// Data
	_, _ = buf.Write(frame.Data[:dataLen])

	return buf.Bytes()
}

// DecodeDataFrame 解码数据帧
func DecodeDataFrame(data []byte) (*DataFrame, error) {
	if len(data) < 10 {
		return nil, errors.New("data too short for data frame")
	}

	buf := bytes.NewReader(data)

	msgType, err := buf.ReadByte()
	if err != nil {
		return nil, err
	}
	if MessageType(msgType) != MsgData {
		return nil, errors.New("invalid message type")
	}

	frame := &DataFrame{}
	if err := binary.Read(buf, binary.BigEndian, &frame.StreamID); err != nil {
		return nil, err
	}
	frame.Flags, err = buf.ReadByte()
	if err != nil {
		return nil, err
	}

	var dataLen uint32
	if err := binary.Read(buf, binary.BigEndian, &dataLen); err != nil {
		return nil, err
	}

	if dataLen > MaxFrameSize {
		return nil, errors.New("frame too large")
	}

	frame.Data = make([]byte, dataLen)
	if _, err := buf.Read(frame.Data); err != nil {
		return nil, err
	}

	return frame, nil
}

// ============================================================================
//                              流式读写
// ============================================================================

// WriteMessage 写入消息到流
func WriteMessage(w io.Writer, msgType MessageType, payload []byte) error {
	// 写入长度前缀（安全转换：payload 长度受 MaxMessageSize 限制）
	payloadLen := len(payload)
	if payloadLen > MaxMessageSize-1 {
		return errors.New("payload too large")
	}
	totalLen := uint32(1 + payloadLen) // #nosec G115 -- bounded by MaxMessageSize
	if err := binary.Write(w, binary.BigEndian, totalLen); err != nil {
		return err
	}

	// 写入消息类型
	if _, err := w.Write([]byte{byte(msgType)}); err != nil {
		return err
	}

	// 写入载荷
	if len(payload) > 0 {
		if _, err := w.Write(payload); err != nil {
			return err
		}
	}

	return nil
}

// ReadMessage 从流读取消息
func ReadMessage(r io.Reader) (MessageType, []byte, error) {
	// 读取长度
	var totalLen uint32
	if err := binary.Read(r, binary.BigEndian, &totalLen); err != nil {
		return 0, nil, err
	}

	if totalLen > MaxMessageSize {
		return 0, nil, errors.New("message too large")
	}

	if totalLen < 1 {
		return 0, nil, errors.New("message too short")
	}

	// 读取消息类型
	typeBuf := make([]byte, 1)
	if _, err := io.ReadFull(r, typeBuf); err != nil {
		return 0, nil, err
	}

	// 读取载荷
	payloadLen := totalLen - 1
	var payload []byte
	if payloadLen > 0 {
		payload = make([]byte, payloadLen)
		if _, err := io.ReadFull(r, payload); err != nil {
			return 0, nil, err
		}
	}

	return MessageType(typeBuf[0]), payload, nil
}

// ============================================================================
//                              限制信息
// ============================================================================

// Limit 限制信息
type Limit struct {
	// Duration 最大连接时长
	Duration time.Duration

	// Data 最大数据量
	Data int64
}

// DefaultLimit 默认限制
func DefaultLimit() Limit {
	return Limit{
		Duration: 2 * time.Minute,
		Data:     1024 * 1024 * 128, // 128 MB
	}
}

// Voucher 凭证（用于资源预留）
type Voucher struct {
	// Relay 中继节点
	Relay types.NodeID

	// Peer 预留节点
	Peer types.NodeID

	// Expiration 过期时间
	Expiration time.Time

	// Limit 限制
	Limit Limit
}

// IsExpired 检查是否过期
func (v *Voucher) IsExpired() bool {
	return time.Now().After(v.Expiration)
}

// EncodeVoucher 编码凭证
func EncodeVoucher(v *Voucher) []byte {
	buf := new(bytes.Buffer)

	_, _ = buf.Write(v.Relay[:])
	_, _ = buf.Write(v.Peer[:])
	_ = binary.Write(buf, binary.BigEndian, v.Expiration.Unix())
	_ = binary.Write(buf, binary.BigEndian, int64(v.Limit.Duration))
	_ = binary.Write(buf, binary.BigEndian, v.Limit.Data)

	return buf.Bytes()
}

// DecodeVoucher 解码凭证
func DecodeVoucher(data []byte) (*Voucher, error) {
	if len(data) < 32*2+24 { // NodeID is 32 bytes
		return nil, errors.New("data too short for voucher")
	}

	buf := bytes.NewReader(data)

	v := &Voucher{}
	if _, err := buf.Read(v.Relay[:]); err != nil {
		return nil, err
	}
	if _, err := buf.Read(v.Peer[:]); err != nil {
		return nil, err
	}

	var expUnix int64
	if err := binary.Read(buf, binary.BigEndian, &expUnix); err != nil {
		return nil, err
	}
	v.Expiration = time.Unix(expUnix, 0)

	var durNano int64
	if err := binary.Read(buf, binary.BigEndian, &durNano); err != nil {
		return nil, err
	}
	v.Limit.Duration = time.Duration(durNano)

	if err := binary.Read(buf, binary.BigEndian, &v.Limit.Data); err != nil {
		return nil, err
	}

	return v, nil
}
