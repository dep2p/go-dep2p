package crypto

import (
	"encoding/binary"
	"fmt"
)

// ============================================================================
//                              序列化格式
// ============================================================================

// 序列化格式：
//
//   ┌─────────────────────────────────────────────────────────────┐
//   │                    公钥/私钥序列化格式                         │
//   ├─────────────────────────────────────────────────────────────┤
//   │  Type:   uint8 (KeyType)                                    │
//   │  Length: uint32 (大端序)                                     │
//   │  Data:   密钥数据                                            │
//   └─────────────────────────────────────────────────────────────┘
//
// 此格式与 pkg/proto/key/key.proto 中的 PublicKey/PrivateKey 消息兼容。

const (
	// 序列化头大小：1 字节类型 + 4 字节长度
	marshalHeaderSize = 5
)

// ============================================================================
//                              公钥序列化
// ============================================================================

// MarshalPublicKey 序列化公钥
//
// 返回格式：[Type(1)] [Length(4)] [Data(n)]
func MarshalPublicKey(key PublicKey) ([]byte, error) {
	if key == nil {
		return nil, ErrNilPublicKey
	}

	raw, err := key.Raw()
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrMarshalFailed, err)
	}

	// 分配缓冲区
	buf := make([]byte, marshalHeaderSize+len(raw))

	// 写入类型
	buf[0] = byte(key.Type())

	// 写入长度（大端序）
	binary.BigEndian.PutUint32(buf[1:5], uint32(len(raw)))

	// 写入数据
	copy(buf[5:], raw)

	return buf, nil
}

// UnmarshalPublicKeyBytes 从字节反序列化公钥
//
// 参数格式：[Type(1)] [Length(4)] [Data(n)]
func UnmarshalPublicKeyBytes(data []byte) (PublicKey, error) {
	if len(data) < marshalHeaderSize {
		return nil, fmt.Errorf("%w: data too short", ErrUnmarshalFailed)
	}

	// 读取类型
	keyType := KeyType(data[0])

	// 读取长度
	length := binary.BigEndian.Uint32(data[1:5])

	// 验证数据长度
	if len(data) < marshalHeaderSize+int(length) {
		return nil, fmt.Errorf("%w: data length mismatch", ErrUnmarshalFailed)
	}

	// 读取密钥数据
	keyData := data[5 : 5+length]

	return UnmarshalPublicKey(keyType, keyData)
}

// ============================================================================
//                              私钥序列化
// ============================================================================

// MarshalPrivateKey 序列化私钥
//
// 返回格式：[Type(1)] [Length(4)] [Data(n)]
func MarshalPrivateKey(key PrivateKey) ([]byte, error) {
	if key == nil {
		return nil, ErrNilPrivateKey
	}

	raw, err := key.Raw()
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrMarshalFailed, err)
	}

	// 分配缓冲区
	buf := make([]byte, marshalHeaderSize+len(raw))

	// 写入类型
	buf[0] = byte(key.Type())

	// 写入长度（大端序）
	binary.BigEndian.PutUint32(buf[1:5], uint32(len(raw)))

	// 写入数据
	copy(buf[5:], raw)

	return buf, nil
}

// UnmarshalPrivateKeyBytes 从字节反序列化私钥
//
// 参数格式：[Type(1)] [Length(4)] [Data(n)]
func UnmarshalPrivateKeyBytes(data []byte) (PrivateKey, error) {
	if len(data) < marshalHeaderSize {
		return nil, fmt.Errorf("%w: data too short", ErrUnmarshalFailed)
	}

	// 读取类型
	keyType := KeyType(data[0])

	// 读取长度
	length := binary.BigEndian.Uint32(data[1:5])

	// 验证数据长度
	if len(data) < marshalHeaderSize+int(length) {
		return nil, fmt.Errorf("%w: data length mismatch", ErrUnmarshalFailed)
	}

	// 读取密钥数据
	keyData := data[5 : 5+length]

	return UnmarshalPrivateKey(keyType, keyData)
}

// ============================================================================
//                              签名序列化
// ============================================================================

// MarshalSignature 序列化签名
//
// 返回格式：[Type(1)] [Length(4)] [Data(n)]
func MarshalSignature(keyType KeyType, sig []byte) ([]byte, error) {
	if sig == nil {
		return nil, ErrNilSignature
	}

	// 分配缓冲区
	buf := make([]byte, marshalHeaderSize+len(sig))

	// 写入类型
	buf[0] = byte(keyType)

	// 写入长度（大端序）
	binary.BigEndian.PutUint32(buf[1:5], uint32(len(sig)))

	// 写入数据
	copy(buf[5:], sig)

	return buf, nil
}

// UnmarshalSignature 反序列化签名
//
// 参数格式：[Type(1)] [Length(4)] [Data(n)]
// 返回：密钥类型和签名数据
func UnmarshalSignature(data []byte) (KeyType, []byte, error) {
	if len(data) < marshalHeaderSize {
		return KeyTypeUnspecified, nil, fmt.Errorf("%w: data too short", ErrUnmarshalFailed)
	}

	// 读取类型
	keyType := KeyType(data[0])

	// 读取长度
	length := binary.BigEndian.Uint32(data[1:5])

	// 验证数据长度
	if len(data) < marshalHeaderSize+int(length) {
		return KeyTypeUnspecified, nil, fmt.Errorf("%w: data length mismatch", ErrUnmarshalFailed)
	}

	// 复制签名数据
	sig := make([]byte, length)
	copy(sig, data[5:5+length])

	return keyType, sig, nil
}

// ============================================================================
//                              密钥对序列化
// ============================================================================

// MarshalKeyPair 序列化密钥对
//
// 返回格式：[PubLen(4)] [PubKey] [PrivLen(4)] [PrivKey]
func MarshalKeyPair(priv PrivateKey, pub PublicKey) ([]byte, error) {
	pubBytes, err := MarshalPublicKey(pub)
	if err != nil {
		return nil, err
	}

	privBytes, err := MarshalPrivateKey(priv)
	if err != nil {
		return nil, err
	}

	// 分配缓冲区
	buf := make([]byte, 4+len(pubBytes)+4+len(privBytes))

	// 写入公钥长度和数据
	binary.BigEndian.PutUint32(buf[0:4], uint32(len(pubBytes)))
	copy(buf[4:4+len(pubBytes)], pubBytes)

	// 写入私钥长度和数据
	offset := 4 + len(pubBytes)
	binary.BigEndian.PutUint32(buf[offset:offset+4], uint32(len(privBytes)))
	copy(buf[offset+4:], privBytes)

	return buf, nil
}

// UnmarshalKeyPair 反序列化密钥对
func UnmarshalKeyPair(data []byte) (PrivateKey, PublicKey, error) {
	if len(data) < 8 { // 至少需要两个长度字段
		return nil, nil, fmt.Errorf("%w: data too short", ErrUnmarshalFailed)
	}

	// 读取公钥长度
	pubLen := binary.BigEndian.Uint32(data[0:4])
	if len(data) < 4+int(pubLen)+4 {
		return nil, nil, fmt.Errorf("%w: public key data truncated", ErrUnmarshalFailed)
	}

	// 反序列化公钥
	pub, err := UnmarshalPublicKeyBytes(data[4 : 4+pubLen])
	if err != nil {
		return nil, nil, err
	}

	// 读取私钥长度
	offset := 4 + int(pubLen)
	privLen := binary.BigEndian.Uint32(data[offset : offset+4])
	if len(data) < offset+4+int(privLen) {
		return nil, nil, fmt.Errorf("%w: private key data truncated", ErrUnmarshalFailed)
	}

	// 反序列化私钥
	priv, err := UnmarshalPrivateKeyBytes(data[offset+4 : offset+4+int(privLen)])
	if err != nil {
		return nil, nil, err
	}

	return priv, pub, nil
}
