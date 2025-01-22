// Package crypto 实现了dep2p使用的各种加密工具。
// 包括公钥和私钥接口以及支持的密钥算法的实现。
package crypto

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"io"

	"github.com/dep2p/go-dep2p/core/crypto/pb"

	"google.golang.org/protobuf/proto"
)

// 密钥类型常量定义
const (
	// RSA 表示RSA密钥类型
	RSA = iota
	// Ed25519 表示Ed25519密钥类型
	Ed25519
	// Secp256k1 表示Secp256k1密钥类型
	Secp256k1
	// ECDSA 表示ECDSA密钥类型
	ECDSA
)

var (
	// ErrBadKeyType 当密钥类型不支持时返回此错误
	ErrBadKeyType = errors.New("无效或不支持的密钥类型")
	// KeyTypes 支持的密钥类型列表
	KeyTypes = []int{
		RSA,
		Ed25519,
		Secp256k1,
		ECDSA,
	}
)

// PubKeyUnmarshaller 是一个从字节切片创建公钥的函数类型
type PubKeyUnmarshaller func(data []byte) (PubKey, error)

// PrivKeyUnmarshaller 是一个从字节切片创建私钥的函数类型
type PrivKeyUnmarshaller func(data []byte) (PrivKey, error)

// PubKeyUnmarshallers 是按密钥类型映射的公钥反序列化器
var PubKeyUnmarshallers = map[pb.KeyType]PubKeyUnmarshaller{
	pb.KeyType_RSA:       UnmarshalRsaPublicKey,
	pb.KeyType_Ed25519:   UnmarshalEd25519PublicKey,
	pb.KeyType_Secp256k1: UnmarshalSecp256k1PublicKey,
	pb.KeyType_ECDSA:     UnmarshalECDSAPublicKey,
}

// PrivKeyUnmarshallers 是按密钥类型映射的私钥反序列化器
var PrivKeyUnmarshallers = map[pb.KeyType]PrivKeyUnmarshaller{
	pb.KeyType_RSA:       UnmarshalRsaPrivateKey,
	pb.KeyType_Ed25519:   UnmarshalEd25519PrivateKey,
	pb.KeyType_Secp256k1: UnmarshalSecp256k1PrivateKey,
	pb.KeyType_ECDSA:     UnmarshalECDSAPrivateKey,
}

// Key 表示可以与另一个密钥进行比较的加密密钥接口
type Key interface {
	// Equals 检查两个密钥是否相同
	Equals(Key) bool

	// Raw 返回密钥的原始字节(不包含在dep2p-crypto protobuf中)
	// 此函数是{Priv,Pub}KeyUnmarshaler的逆操作
	Raw() ([]byte, error)

	// Type 返回protobuf密钥类型
	Type() pb.KeyType
}

// PrivKey 表示可用于生成公钥和签名数据的私钥接口
type PrivKey interface {
	Key

	// Sign 对给定的字节进行加密签名
	Sign([]byte) ([]byte, error)

	// GetPublic 返回与此私钥配对的公钥
	GetPublic() PubKey
}

// PubKey 是可用于验证使用相应私钥签名的数据的公钥接口
type PubKey interface {
	Key

	// Verify 验证'sig'是否是'data'的签名哈希
	Verify(data []byte, sig []byte) (bool, error)
}

// GenSharedKey 是从给定私钥生成共享密钥的函数类型
type GenSharedKey func([]byte) ([]byte, error)

// GenerateKeyPair 生成私钥和公钥对
// 参数:
//   - typ: 密钥类型
//   - bits: 密钥位数
//
// 返回:
//   - PrivKey: 生成的私钥
//   - PubKey: 生成的公钥
//   - error: 错误信息
func GenerateKeyPair(typ, bits int) (PrivKey, PubKey, error) {
	return GenerateKeyPairWithReader(typ, bits, rand.Reader)
}

// GenerateKeyPairWithReader 使用指定的随机源生成指定类型和位数的密钥对
// 参数:
//   - typ: 密钥类型
//   - bits: 密钥位数
//   - src: 随机源
//
// 返回:
//   - PrivKey: 生成的私钥
//   - PubKey: 生成的公钥
//   - error: 错误信息
func GenerateKeyPairWithReader(typ, bits int, src io.Reader) (PrivKey, PubKey, error) {
	switch typ {
	case RSA:
		return GenerateRSAKeyPair(bits, src)
	case Ed25519:
		return GenerateEd25519Key(src)
	case Secp256k1:
		return GenerateSecp256k1Key(src)
	case ECDSA:
		return GenerateECDSAKeyPair(src)
	default:
		return nil, nil, ErrBadKeyType
	}
}

// UnmarshalPublicKey 将protobuf序列化的公钥转换为其对应的对象
// 参数:
//   - data: 序列化的公钥数据
//
// 返回:
//   - PubKey: 反序列化后的公钥对象
//   - error: 错误信息
func UnmarshalPublicKey(data []byte) (PubKey, error) {
	pmes := new(pb.PublicKey)
	err := proto.Unmarshal(data, pmes)
	if err != nil {
		log.Debugf("反序列化公钥失败: %v", err)
		return nil, err
	}

	return PublicKeyFromProto(pmes)
}

// PublicKeyFromProto 将未序列化的protobuf PublicKey消息转换为其对应的对象
// 参数:
//   - pmes: protobuf公钥消息
//
// 返回:
//   - PubKey: 转换后的公钥对象
//   - error: 错误信息
func PublicKeyFromProto(pmes *pb.PublicKey) (PubKey, error) {
	um, ok := PubKeyUnmarshallers[pmes.GetType()]
	if !ok {
		log.Debugf("无效或不支持的密钥类型: %v", pmes.GetType())
		return nil, ErrBadKeyType
	}

	data := pmes.GetData()

	pk, err := um(data)
	if err != nil {
		return nil, err
	}

	switch tpk := pk.(type) {
	case *RsaPublicKey:
		tpk.cached, _ = proto.Marshal(pmes)
	}

	return pk, nil
}

// MarshalPublicKey 将公钥对象转换为protobuf序列化的公钥
// 参数:
//   - k: 要序列化的公钥对象
//
// 返回:
//   - []byte: 序列化后的字节数据
//   - error: 错误信息
func MarshalPublicKey(k PubKey) ([]byte, error) {
	pbmes, err := PublicKeyToProto(k)
	if err != nil {
		return nil, err
	}

	return proto.Marshal(pbmes)
}

// PublicKeyToProto 将公钥对象转换为未序列化的protobuf PublicKey消息
// 参数:
//   - k: 要转换的公钥对象
//
// 返回:
//   - *pb.PublicKey: protobuf公钥消息
//   - error: 错误信息
func PublicKeyToProto(k PubKey) (*pb.PublicKey, error) {
	data, err := k.Raw()
	if err != nil {
		log.Debugf("获取公钥数据失败: %v", err)
		return nil, err
	}
	return &pb.PublicKey{
		Type: k.Type().Enum(),
		Data: data,
	}, nil
}

// UnmarshalPrivateKey 将protobuf序列化的私钥转换为其对应的对象
// 参数:
//   - data: 序列化的私钥数据
//
// 返回:
//   - PrivKey: 反序列化后的私钥对象
//   - error: 错误信息
func UnmarshalPrivateKey(data []byte) (PrivKey, error) {
	pmes := new(pb.PrivateKey)
	err := proto.Unmarshal(data, pmes)
	if err != nil {
		log.Debugf("反序列化私钥失败: %v", err)
		return nil, err
	}

	um, ok := PrivKeyUnmarshallers[pmes.GetType()]
	if !ok {
		log.Debugf("无效或不支持的密钥类型: %v", pmes.GetType())
		return nil, ErrBadKeyType
	}

	return um(pmes.GetData())
}

// MarshalPrivateKey 将私钥对象转换为其protobuf序列化形式
// 参数:
//   - k: 要序列化的私钥对象
//
// 返回:
//   - []byte: 序列化后的字节数据
//   - error: 错误信息
func MarshalPrivateKey(k PrivKey) ([]byte, error) {
	data, err := k.Raw()
	if err != nil {
		log.Debugf("获取密钥数据失败: %v", err)
		return nil, err
	}
	return proto.Marshal(&pb.PrivateKey{
		Type: k.Type().Enum(),
		Data: data,
	})
}

// ConfigDecodeKey 将base64编码的密钥(用于配置文件)解码为可以反序列化的字节数组
// 参数:
//   - b: base64编码的字符串
//
// 返回:
//   - []byte: 解码后的字节数组
//   - error: 错误信息
func ConfigDecodeKey(b string) ([]byte, error) {
	return base64.StdEncoding.DecodeString(b)
}

// ConfigEncodeKey 将序列化的密钥编码为base64(用于配置文件)
// 参数:
//   - b: 要编码的字节数组
//
// 返回:
//   - string: base64编码的字符串
func ConfigEncodeKey(b []byte) string {
	return base64.StdEncoding.EncodeToString(b)
}

// KeyEqual 检查两个密钥是否等价(具有相同的字节表示)
// 参数:
//   - k1: 第一个密钥
//   - k2: 第二个密钥
//
// 返回:
//   - bool: 如果密钥相等返回true，否则返回false
func KeyEqual(k1, k2 Key) bool {
	if k1 == k2 {
		return true
	}

	return k1.Equals(k2)
}

// basicEquals 执行基本的密钥相等性检查
// 参数:
//   - k1: 第一个密钥
//   - k2: 第二个密钥
//
// 返回:
//   - bool: 如果密钥相等返回true，否则返回false
func basicEquals(k1, k2 Key) bool {
	if k1.Type() != k2.Type() {
		log.Debugf("密钥类型不匹配: %v != %v", k1.Type(), k2.Type())
		return false
	}

	a, err := k1.Raw()
	if err != nil {
		log.Debugf("获取密钥数据失败: %v", err)
		return false
	}
	b, err := k2.Raw()
	if err != nil {
		log.Debugf("获取密钥数据失败: %v", err)
		return false
	}
	return subtle.ConstantTimeCompare(a, b) == 1
}
