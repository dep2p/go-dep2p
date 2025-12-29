// Package holepunch 提供 NAT 打洞协议实现
package holepunch

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"

	"github.com/dep2p/go-dep2p/internal/core/address"
	"github.com/dep2p/go-dep2p/pkg/interfaces/endpoint"
	"github.com/dep2p/go-dep2p/pkg/protocolids"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              协议常量
// ============================================================================

// 引用 pkg/protocolids 唯一真源
var (
	// ProtocolID 打洞协议标识 (v1.1 scope: sys)
	ProtocolID = protocolids.SysHolepunch
)

const (
	// MsgTypeRequest 请求消息类型
	MsgTypeRequest uint8 = 0x01
	// MsgTypeConnect 连接消息类型
	MsgTypeConnect uint8 = 0x02
	// MsgTypeSync 同步消息类型
	MsgTypeSync uint8 = 0x03
	// MsgTypeResponse 响应消息类型
	MsgTypeResponse uint8 = 0x04

	// MaxAddresses 最大地址数
	MaxAddresses = 16

	// NonceLen Nonce 长度
	NonceLen = 16
)

// ============================================================================
//                              错误定义
// ============================================================================

// 打洞协议相关错误
var (
	// ErrInvalidMessage 无效的打洞消息
	ErrInvalidMessage   = errors.New("invalid holepunch message")
	ErrTooManyAddresses = errors.New("too many addresses")
	ErrInvalidNonce     = errors.New("invalid nonce")
)

// ============================================================================
//                              消息类型
// ============================================================================

// HolePunchRequest 打洞请求消息
type HolePunchRequest struct {
	// InitiatorID 发起方 NodeID
	InitiatorID types.NodeID

	// InitiatorAddrs 发起方地址列表
	InitiatorAddrs []endpoint.Address

	// ResponderID 响应方 NodeID
	ResponderID types.NodeID
}

// HolePunchConnect 打洞连接消息
type HolePunchConnect struct {
	// InitiatorAddrs 发起方观察到的地址
	InitiatorAddrs []endpoint.Address

	// ResponderAddrs 响应方观察到的地址
	ResponderAddrs []endpoint.Address

	// Nonce 随机数，用于验证
	Nonce []byte
}

// HolePunchSync 打洞同步消息
type HolePunchSync struct {
	// Nonce 匹配的随机数
	Nonce []byte
}

// HolePunchResponse 打洞响应消息
type HolePunchResponse struct {
	// Success 是否成功
	Success bool

	// Nonce 匹配的随机数
	Nonce []byte

	// Error 错误信息
	Error string
}

// ============================================================================
//                              编码方法
// ============================================================================

// Encode 编码请求消息
func (m *HolePunchRequest) Encode() ([]byte, error) {
	var buf bytes.Buffer

	// 消息类型
	buf.WriteByte(MsgTypeRequest)

	// InitiatorID (32 bytes)
	buf.Write(m.InitiatorID[:])

	// InitiatorAddrs
	if len(m.InitiatorAddrs) > MaxAddresses {
		return nil, ErrTooManyAddresses
	}
	buf.WriteByte(byte(len(m.InitiatorAddrs)))
	for _, addr := range m.InitiatorAddrs {
		addrBytes := []byte(addr.String())
		if len(addrBytes) > 255 {
			addrBytes = addrBytes[:255]
		}
		buf.WriteByte(byte(len(addrBytes)))
		buf.Write(addrBytes)
	}

	// ResponderID (32 bytes)
	buf.Write(m.ResponderID[:])

	return buf.Bytes(), nil
}

// Decode 解码请求消息
func (m *HolePunchRequest) Decode(data []byte) error {
	if len(data) < 1+32+1+32 {
		return ErrInvalidMessage
	}

	r := bytes.NewReader(data)

	// 消息类型
	msgType, err := r.ReadByte()
	if err != nil {
		return fmt.Errorf("read message type: %w", err)
	}
	if msgType != MsgTypeRequest {
		return fmt.Errorf("expected request message, got %d", msgType)
	}

	// InitiatorID
	initID := make([]byte, 32)
	if _, err := io.ReadFull(r, initID); err != nil {
		return err
	}
	copy(m.InitiatorID[:], initID)

	// InitiatorAddrs
	addrCount, err := r.ReadByte()
	if err != nil {
		return fmt.Errorf("read address count: %w", err)
	}
	if addrCount > MaxAddresses {
		return ErrTooManyAddresses
	}
	m.InitiatorAddrs = make([]endpoint.Address, 0, addrCount)
	for i := 0; i < int(addrCount); i++ {
		addrLen, err := r.ReadByte()
		if err != nil {
			return fmt.Errorf("read address length: %w", err)
		}
		addrBytes := make([]byte, addrLen)
		if _, err := io.ReadFull(r, addrBytes); err != nil {
			return err
		}
		// 使用 address.Addr 代替 StringAddress（IMPL-ADDRESS-UNIFICATION）
		addr, err := address.Parse(string(addrBytes))
		if err != nil {
			// 地址格式无效，跳过
			continue
		}
		m.InitiatorAddrs = append(m.InitiatorAddrs, addr)
	}

	// ResponderID
	respID := make([]byte, 32)
	if _, err := io.ReadFull(r, respID); err != nil {
		return err
	}
	copy(m.ResponderID[:], respID)

	return nil
}

// Encode 编码连接消息
func (m *HolePunchConnect) Encode() ([]byte, error) {
	var buf bytes.Buffer

	// 消息类型
	buf.WriteByte(MsgTypeConnect)

	// InitiatorAddrs
	if len(m.InitiatorAddrs) > MaxAddresses {
		return nil, ErrTooManyAddresses
	}
	buf.WriteByte(byte(len(m.InitiatorAddrs)))
	for _, addr := range m.InitiatorAddrs {
		addrBytes := []byte(addr.String())
		if len(addrBytes) > 255 {
			addrBytes = addrBytes[:255]
		}
		buf.WriteByte(byte(len(addrBytes)))
		buf.Write(addrBytes)
	}

	// ResponderAddrs
	if len(m.ResponderAddrs) > MaxAddresses {
		return nil, ErrTooManyAddresses
	}
	buf.WriteByte(byte(len(m.ResponderAddrs)))
	for _, addr := range m.ResponderAddrs {
		addrBytes := []byte(addr.String())
		if len(addrBytes) > 255 {
			addrBytes = addrBytes[:255]
		}
		buf.WriteByte(byte(len(addrBytes)))
		buf.Write(addrBytes)
	}

	// Nonce
	if len(m.Nonce) != NonceLen {
		return nil, ErrInvalidNonce
	}
	buf.Write(m.Nonce)

	return buf.Bytes(), nil
}

// Decode 解码连接消息
func (m *HolePunchConnect) Decode(data []byte) error {
	if len(data) < 1+1+1+NonceLen {
		return ErrInvalidMessage
	}

	r := bytes.NewReader(data)

	// 消息类型
	msgType, err := r.ReadByte()
	if err != nil {
		return fmt.Errorf("read message type: %w", err)
	}
	if msgType != MsgTypeConnect {
		return fmt.Errorf("expected connect message, got %d", msgType)
	}

	// InitiatorAddrs
	initCount, err := r.ReadByte()
	if err != nil {
		return fmt.Errorf("read initiator address count: %w", err)
	}
	if initCount > MaxAddresses {
		return ErrTooManyAddresses
	}
	m.InitiatorAddrs = make([]endpoint.Address, 0, initCount)
	for i := 0; i < int(initCount); i++ {
		addrLen, err := r.ReadByte()
		if err != nil {
			return fmt.Errorf("read address length: %w", err)
		}
		addrBytes := make([]byte, addrLen)
		if _, err := io.ReadFull(r, addrBytes); err != nil {
			return err
		}
		// 使用 address.Addr 代替 StringAddress（IMPL-ADDRESS-UNIFICATION）
		addr, err := address.Parse(string(addrBytes))
		if err != nil {
			continue
		}
		m.InitiatorAddrs = append(m.InitiatorAddrs, addr)
	}

	// ResponderAddrs
	respCount, err := r.ReadByte()
	if err != nil {
		return fmt.Errorf("read responder address count: %w", err)
	}
	if respCount > MaxAddresses {
		return ErrTooManyAddresses
	}
	m.ResponderAddrs = make([]endpoint.Address, 0, respCount)
	for i := 0; i < int(respCount); i++ {
		addrLen, err := r.ReadByte()
		if err != nil {
			return fmt.Errorf("read address length: %w", err)
		}
		addrBytes := make([]byte, addrLen)
		if _, err := io.ReadFull(r, addrBytes); err != nil {
			return err
		}
		// 使用 address.Addr 代替 StringAddress（IMPL-ADDRESS-UNIFICATION）
		addr, err := address.Parse(string(addrBytes))
		if err != nil {
			continue
		}
		m.ResponderAddrs = append(m.ResponderAddrs, addr)
	}

	// Nonce
	m.Nonce = make([]byte, NonceLen)
	if _, err := io.ReadFull(r, m.Nonce); err != nil {
		return err
	}

	return nil
}

// Encode 编码同步消息
func (m *HolePunchSync) Encode() ([]byte, error) {
	if len(m.Nonce) != NonceLen {
		return nil, ErrInvalidNonce
	}

	buf := make([]byte, 1+NonceLen)
	buf[0] = MsgTypeSync
	copy(buf[1:], m.Nonce)

	return buf, nil
}

// Decode 解码同步消息
func (m *HolePunchSync) Decode(data []byte) error {
	if len(data) < 1+NonceLen {
		return ErrInvalidMessage
	}

	if data[0] != MsgTypeSync {
		return fmt.Errorf("expected sync message, got %d", data[0])
	}

	m.Nonce = make([]byte, NonceLen)
	copy(m.Nonce, data[1:1+NonceLen])

	return nil
}

// Encode 编码响应消息
func (m *HolePunchResponse) Encode() ([]byte, error) {
	var buf bytes.Buffer

	// 消息类型
	buf.WriteByte(MsgTypeResponse)

	// Success
	if m.Success {
		buf.WriteByte(1)
	} else {
		buf.WriteByte(0)
	}

	// Nonce
	if len(m.Nonce) != NonceLen {
		return nil, ErrInvalidNonce
	}
	buf.Write(m.Nonce)

	// Error
	errBytes := []byte(m.Error)
	if len(errBytes) > 255 {
		errBytes = errBytes[:255]
	}
	buf.WriteByte(byte(len(errBytes)))
	buf.Write(errBytes)

	return buf.Bytes(), nil
}

// Decode 解码响应消息
func (m *HolePunchResponse) Decode(data []byte) error {
	if len(data) < 1+1+NonceLen+1 {
		return ErrInvalidMessage
	}

	r := bytes.NewReader(data)

	// 消息类型
	msgType, err := r.ReadByte()
	if err != nil {
		return fmt.Errorf("read message type: %w", err)
	}
	if msgType != MsgTypeResponse {
		return fmt.Errorf("expected response message, got %d", msgType)
	}

	// Success
	success, err := r.ReadByte()
	if err != nil {
		return fmt.Errorf("read success flag: %w", err)
	}
	m.Success = success == 1

	// Nonce
	m.Nonce = make([]byte, NonceLen)
	if _, err := io.ReadFull(r, m.Nonce); err != nil {
		return err
	}

	// Error
	errLen, err := r.ReadByte()
	if err != nil {
		return fmt.Errorf("read error length: %w", err)
	}
	if errLen > 0 {
		errBytes := make([]byte, errLen)
		if _, err := io.ReadFull(r, errBytes); err != nil {
			return err
		}
		m.Error = string(errBytes)
	}

	return nil
}

// ============================================================================
//                              辅助类型（已废弃）
// ============================================================================

// StringAddress 已废弃
//
// Deprecated: 根据 IMPL-ADDRESS-UNIFICATION.md 规范，此类型已被 address.Addr 替代。
// 所有地址应使用 internal/core/address.Addr 或 types.Multiaddr。

// ============================================================================
//                              解析辅助
// ============================================================================

// ParseMessage 解析消息
func ParseMessage(data []byte) (interface{}, error) {
	if len(data) < 1 {
		return nil, ErrInvalidMessage
	}

	msgType := data[0]
	switch msgType {
	case MsgTypeRequest:
		msg := &HolePunchRequest{}
		if err := msg.Decode(data); err != nil {
			return nil, err
		}
		return msg, nil

	case MsgTypeConnect:
		msg := &HolePunchConnect{}
		if err := msg.Decode(data); err != nil {
			return nil, err
		}
		return msg, nil

	case MsgTypeSync:
		msg := &HolePunchSync{}
		if err := msg.Decode(data); err != nil {
			return nil, err
		}
		return msg, nil

	case MsgTypeResponse:
		msg := &HolePunchResponse{}
		if err := msg.Decode(data); err != nil {
			return nil, err
		}
		return msg, nil

	default:
		return nil, fmt.Errorf("unknown message type: %d", msgType)
	}
}

// EncodeAddresses 编码地址列表
func EncodeAddresses(addrs []endpoint.Address) []byte {
	var buf bytes.Buffer

	count := len(addrs)
	if count > MaxAddresses {
		count = MaxAddresses
	}

	buf.WriteByte(byte(count))
	for i := 0; i < count; i++ {
		addrBytes := []byte(addrs[i].String())
		if len(addrBytes) > 255 {
			addrBytes = addrBytes[:255]
		}
		_ = binary.Write(&buf, binary.BigEndian, uint8(len(addrBytes)))
		buf.Write(addrBytes)
	}

	return buf.Bytes()
}

// DecodeAddresses 解码地址列表
func DecodeAddresses(data []byte) ([]endpoint.Address, error) {
	if len(data) < 1 {
		return nil, ErrInvalidMessage
	}

	r := bytes.NewReader(data)

	count, _ := r.ReadByte()
	addrs := make([]endpoint.Address, 0, count)

	for i := 0; i < int(count); i++ {
		addrLen, err := r.ReadByte()
		if err != nil {
			return addrs, err
		}

		addrBytes := make([]byte, addrLen)
		if _, err := io.ReadFull(r, addrBytes); err != nil {
			return addrs, err
		}

		// 使用 address.Addr 代替 StringAddress（IMPL-ADDRESS-UNIFICATION）
		addr, err := address.Parse(string(addrBytes))
		if err != nil {
			continue
		}
		addrs = append(addrs, addr)
	}

	return addrs, nil
}
