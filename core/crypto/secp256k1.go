package crypto

import (
	"crypto/sha256"
	"fmt"
	"io"

	pb "github.com/dep2p/go-dep2p/core/crypto/pb"
	"github.com/dep2p/go-dep2p/core/internal/catch"

	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/decred/dcrd/dcrec/secp256k1/v4/ecdsa"
)

// Secp256k1PrivateKey 表示一个 Secp256k1 私钥
type Secp256k1PrivateKey secp256k1.PrivateKey

// Secp256k1PublicKey 表示一个 Secp256k1 公钥
type Secp256k1PublicKey secp256k1.PublicKey

// GenerateSecp256k1Key 生成一个新的 Secp256k1 私钥和公钥对
// 参数:
//   - src: 随机数生成器
//
// 返回值:
//   - PrivKey: 生成的私钥
//   - PubKey: 生成的公钥
//   - error: 错误信息
func GenerateSecp256k1Key(src io.Reader) (PrivKey, PubKey, error) {
	// 生成私钥
	privk, err := secp256k1.GeneratePrivateKey()
	if err != nil {
		log.Debugf("生成Secp256k1私钥失败: %v", err)
		return nil, nil, err
	}

	// 转换为 Secp256k1PrivateKey 类型并返回私钥和对应的公钥
	k := (*Secp256k1PrivateKey)(privk)
	return k, k.GetPublic(), nil
}

// UnmarshalSecp256k1PrivateKey 从字节数组中解析私钥
// 参数:
//   - data: 待解析的字节数组
//
// 返回值:
//   - PrivKey: 解析得到的私钥
//   - error: 错误信息
func UnmarshalSecp256k1PrivateKey(data []byte) (k PrivKey, err error) {
	// 检查输入数据长度是否符合要求
	if len(data) != secp256k1.PrivKeyBytesLen {
		log.Debugf("预期secp256k1数据长度为%d, 实际长度为%d", secp256k1.PrivKeyBytesLen, len(data))
		return nil, fmt.Errorf("预期secp256k1数据长度为%d, 实际长度为%d", secp256k1.PrivKeyBytesLen, len(data))
	}
	defer func() { catch.HandlePanic(recover(), &err, "secp256k1 private-key unmarshal") }()

	// 从字节数组创建私钥
	privk := secp256k1.PrivKeyFromBytes(data)
	return (*Secp256k1PrivateKey)(privk), nil
}

// UnmarshalSecp256k1PublicKey 从字节数组中解析公钥
// 参数:
//   - data: 待解析的字节数组
//
// 返回值:
//   - PubKey: 解析得到的公钥
//   - error: 错误信息
func UnmarshalSecp256k1PublicKey(data []byte) (_k PubKey, err error) {
	defer func() { catch.HandlePanic(recover(), &err, "secp256k1 public-key unmarshal") }()
	// 解析公钥
	k, err := secp256k1.ParsePubKey(data)
	if err != nil {
		log.Debugf("解析Secp256k1公钥失败: %v", err)
		return nil, err
	}

	return (*Secp256k1PublicKey)(k), nil
}

// Type 返回私钥类型
// 返回值:
//   - pb.KeyType: 私钥类型
func (k *Secp256k1PrivateKey) Type() pb.KeyType {
	return pb.KeyType_Secp256k1
}

// Raw 返回私钥的字节表示
// 返回值:
//   - []byte: 私钥的字节数组
//   - error: 错误信息
func (k *Secp256k1PrivateKey) Raw() ([]byte, error) {
	return (*secp256k1.PrivateKey)(k).Serialize(), nil
}

// Equals 比较两个私钥是否相等
// 参数:
//   - o: 待比较的密钥
//
// 返回值:
//   - bool: 是否相等
func (k *Secp256k1PrivateKey) Equals(o Key) bool {
	sk, ok := o.(*Secp256k1PrivateKey)
	if !ok {
		return basicEquals(k, o)
	}

	return k.GetPublic().Equals(sk.GetPublic())
}

// Sign 使用私钥对数据进行签名
// 参数:
//   - data: 待签名的数据
//
// 返回值:
//   - []byte: 签名结果
//   - error: 错误信息
func (k *Secp256k1PrivateKey) Sign(data []byte) (_sig []byte, err error) {
	defer func() { catch.HandlePanic(recover(), &err, "secp256k1 signing") }()
	// 转换为原始私钥类型
	key := (*secp256k1.PrivateKey)(k)
	// 计算数据的 SHA256 哈希
	hash := sha256.Sum256(data)
	// 使用私钥对哈希进行签名
	sig := ecdsa.Sign(key, hash[:])

	return sig.Serialize(), nil
}

// GetPublic 获取对应的公钥
// 返回值:
//   - PubKey: 对应的公钥
func (k *Secp256k1PrivateKey) GetPublic() PubKey {
	return (*Secp256k1PublicKey)((*secp256k1.PrivateKey)(k).PubKey())
}

// Type 返回公钥类型
// 返回值:
//   - pb.KeyType: 公钥类型
func (k *Secp256k1PublicKey) Type() pb.KeyType {
	return pb.KeyType_Secp256k1
}

// Raw 返回公钥的字节表示
// 返回值:
//   - []byte: 公钥的字节数组
//   - error: 错误信息
func (k *Secp256k1PublicKey) Raw() (res []byte, err error) {
	defer func() { catch.HandlePanic(recover(), &err, "secp256k1 public key marshaling") }()
	return (*secp256k1.PublicKey)(k).SerializeCompressed(), nil
}

// Equals 比较两个公钥是否相等
// 参数:
//   - o: 待比较的密钥
//
// 返回值:
//   - bool: 是否相等
func (k *Secp256k1PublicKey) Equals(o Key) bool {
	sk, ok := o.(*Secp256k1PublicKey)
	if !ok {
		return basicEquals(k, o)
	}

	return (*secp256k1.PublicKey)(k).IsEqual((*secp256k1.PublicKey)(sk))
}

// Verify 验证签名是否有效
// 参数:
//   - data: 原始数据
//   - sigStr: 签名数据
//
// 返回值:
//   - bool: 签名是否有效
//   - error: 错误信息
func (k *Secp256k1PublicKey) Verify(data []byte, sigStr []byte) (success bool, err error) {
	defer func() {
		catch.HandlePanic(recover(), &err, "secp256k1 signature verification")

		// 确保发生错误时返回 false
		if err != nil {
			success = false
		}
	}()
	// 解析签名
	sig, err := ecdsa.ParseDERSignature(sigStr)
	if err != nil {
		return false, err
	}

	// 计算数据的 SHA256 哈希并验证签名
	hash := sha256.Sum256(data)
	return sig.Verify(hash[:], (*secp256k1.PublicKey)(k)), nil
}
