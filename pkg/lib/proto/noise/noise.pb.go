// Package noise 包含 Noise 协议的 protobuf 定义
//
// 实现 libp2p-noise 规范的 payload 结构
package noise

import (
	"errors"
)

// ErrInvalidPayload 表示无效的 payload 数据
var ErrInvalidPayload = errors.New("invalid noise payload data")

// NoiseExtensions 包含 Noise 握手扩展数据
type NoiseExtensions struct {
	// WebTransport 证书哈希
	WebtransportCerthashes [][]byte
	// 支持的流多路复用器
	StreamMuxers []string
}

// NoiseHandshakePayload 是 Noise 握手的 payload 结构
//
// libp2p-noise 协议要求在握手消息中包含：
//   - IdentityKey: Ed25519 公钥（序列化的 PublicKey）
//   - IdentitySig: 对 "noise-libp2p-static-key:" + Curve25519静态公钥 的签名
type NoiseHandshakePayload struct {
	// Ed25519 身份公钥（序列化格式）
	IdentityKey []byte
	// 签名：Sign("noise-libp2p-static-key:" + curve25519_static_pubkey)
	IdentitySig []byte
	// 可选扩展数据
	Extensions *NoiseExtensions
}

// Marshal 序列化 NoiseHandshakePayload
//
// 使用 protobuf wire format 编码：
//   - Field 1 (identity_key): tag=0x0a, wire type=2 (length-delimited)
//   - Field 2 (identity_sig): tag=0x12, wire type=2 (length-delimited)
func (p *NoiseHandshakePayload) Marshal() ([]byte, error) {
	result := make([]byte, 0, len(p.IdentityKey)+len(p.IdentitySig)+10)

	// Field 1: identity_key (tag = 0x0a = field 1, wire type 2)
	if len(p.IdentityKey) > 0 {
		result = append(result, 0x0a) // tag
		result = appendVarint(result, uint64(len(p.IdentityKey)))
		result = append(result, p.IdentityKey...)
	}

	// Field 2: identity_sig (tag = 0x12 = field 2, wire type 2)
	if len(p.IdentitySig) > 0 {
		result = append(result, 0x12) // tag
		result = appendVarint(result, uint64(len(p.IdentitySig)))
		result = append(result, p.IdentitySig...)
	}

	return result, nil
}

// Unmarshal 反序列化 NoiseHandshakePayload
func (p *NoiseHandshakePayload) Unmarshal(data []byte) error {
	for len(data) > 0 {
		if len(data) < 1 {
			return ErrInvalidPayload
		}
		tag := data[0]
		data = data[1:]

		// 解析 field number 和 wire type
		fieldNum := tag >> 3
		wireType := tag & 0x07

		if wireType != 2 { // 只期望 length-delimited 类型
			return ErrInvalidPayload
		}

		// 读取长度
		length, n := consumeVarint(data)
		if n < 0 {
			return ErrInvalidPayload
		}
		data = data[n:]

		if int(length) > len(data) {
			return ErrInvalidPayload
		}

		// 根据字段号分配
		switch fieldNum {
		case 1: // identity_key
			p.IdentityKey = make([]byte, length)
			copy(p.IdentityKey, data[:length])
		case 2: // identity_sig
			p.IdentitySig = make([]byte, length)
			copy(p.IdentitySig, data[:length])
			// 其他字段静默忽略（向前兼容）
		}

		data = data[length:]
	}

	return nil
}

// appendVarint 追加 varint 编码
func appendVarint(buf []byte, v uint64) []byte {
	for v >= 0x80 {
		buf = append(buf, byte(v)|0x80)
		v >>= 7
	}
	return append(buf, byte(v))
}

// consumeVarint 消费 varint 编码，返回值和消费的字节数
func consumeVarint(data []byte) (uint64, int) {
	var v uint64
	for i := 0; i < len(data) && i < 10; i++ {
		b := data[i]
		v |= uint64(b&0x7f) << (7 * i)
		if b < 0x80 {
			return v, i + 1
		}
	}
	return 0, -1
}
