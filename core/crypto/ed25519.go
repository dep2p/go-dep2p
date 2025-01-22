package crypto

import (
	"bytes"
	"crypto/ed25519"
	"crypto/subtle"
	"errors"
	"fmt"
	"io"

	pb "github.com/dep2p/go-dep2p/core/crypto/pb"
	"github.com/dep2p/go-dep2p/core/internal/catch"
)

// Ed25519PrivateKey 表示一个 ed25519 私钥
type Ed25519PrivateKey struct {
	k ed25519.PrivateKey // 底层的 ed25519 私钥
}

// Ed25519PublicKey 表示一个 ed25519 公钥
type Ed25519PublicKey struct {
	k ed25519.PublicKey // 底层的 ed25519 公钥
}

// GenerateEd25519Key 生成一个新的 ed25519 密钥对
// 参数:
//   - src: 随机数生成器
//
// 返回值:
//   - PrivKey: 生成的私钥
//   - PubKey: 生成的公钥
//   - error: 生成过程中的错误,如果成功则为 nil
func GenerateEd25519Key(src io.Reader) (PrivKey, PubKey, error) {
	// 使用提供的随机数生成器生成密钥对
	pub, priv, err := ed25519.GenerateKey(src)
	if err != nil {
		log.Debugf("生成 ed25519 密钥对失败: %v", err)
		return nil, nil, err
	}

	// 返回包装后的私钥和公钥
	return &Ed25519PrivateKey{
			k: priv,
		},
		&Ed25519PublicKey{
			k: pub,
		},
		nil
}

// Type 返回私钥的类型
// 返回值:
//   - pb.KeyType: 返回 Ed25519 类型标识
func (k *Ed25519PrivateKey) Type() pb.KeyType {
	return pb.KeyType_Ed25519
}

// Raw 返回私钥的原始字节
// 返回值:
//   - []byte: 私钥的字节表示
//   - error: 获取过程中的错误,如果成功则为 nil
func (k *Ed25519PrivateKey) Raw() ([]byte, error) {
	// Ed25519 私钥包含两个 32 字节的曲线点:私钥和公钥
	// 这使得获取公钥更高效,无需重新计算椭圆曲线乘法
	buf := make([]byte, len(k.k))
	copy(buf, k.k)

	return buf, nil
}

// pubKeyBytes 从私钥中提取公钥部分
// 返回值:
//   - []byte: 公钥的字节表示
func (k *Ed25519PrivateKey) pubKeyBytes() []byte {
	return k.k[ed25519.PrivateKeySize-ed25519.PublicKeySize:]
}

// Equals 比较两个 ed25519 私钥是否相等
// 参数:
//   - o: 要比较的另一个密钥
//
// 返回值:
//   - bool: 如果密钥相等返回 true,否则返回 false
func (k *Ed25519PrivateKey) Equals(o Key) bool {
	edk, ok := o.(*Ed25519PrivateKey)
	if !ok {
		return basicEquals(k, o)
	}

	return subtle.ConstantTimeCompare(k.k, edk.k) == 1
}

// GetPublic 从私钥获取对应的公钥
// 返回值:
//   - PubKey: 对应的公钥
func (k *Ed25519PrivateKey) GetPublic() PubKey {
	return &Ed25519PublicKey{k: k.pubKeyBytes()}
}

// Sign 使用私钥对消息进行签名
// 参数:
//   - msg: 要签名的消息
//
// 返回值:
//   - []byte: 生成的签名
//   - error: 签名过程中的错误,如果成功则为 nil
func (k *Ed25519PrivateKey) Sign(msg []byte) (res []byte, err error) {
	defer func() { catch.HandlePanic(recover(), &err, "ed15519 signing") }()

	return ed25519.Sign(k.k, msg), nil
}

// Type 返回公钥的类型
// 返回值:
//   - pb.KeyType: 返回 Ed25519 类型标识
func (k *Ed25519PublicKey) Type() pb.KeyType {
	return pb.KeyType_Ed25519
}

// Raw 返回公钥的原始字节
// 返回值:
//   - []byte: 公钥的字节表示
//   - error: 获取过程中的错误,如果成功则为 nil
func (k *Ed25519PublicKey) Raw() ([]byte, error) {
	return k.k, nil
}

// Equals 比较两个 ed25519 公钥是否相等
// 参数:
//   - o: 要比较的另一个密钥
//
// 返回值:
//   - bool: 如果密钥相等返回 true,否则返回 false
func (k *Ed25519PublicKey) Equals(o Key) bool {
	edk, ok := o.(*Ed25519PublicKey)
	if !ok {
		return basicEquals(k, o)
	}

	return bytes.Equal(k.k, edk.k)
}

// Verify 验证签名是否有效
// 参数:
//   - data: 原始消息数据
//   - sig: 要验证的签名
//
// 返回值:
//   - bool: 如果签名有效返回 true,否则返回 false
//   - error: 验证过程中的错误,如果成功则为 nil
func (k *Ed25519PublicKey) Verify(data []byte, sig []byte) (success bool, err error) {
	defer func() {
		catch.HandlePanic(recover(), &err, "ed15519 signature verification")

		// 为了安全起见
		if err != nil {
			success = false
		}
	}()
	return ed25519.Verify(k.k, data, sig), nil
}

// UnmarshalEd25519PublicKey 从字节数据解析出公钥
// 参数:
//   - data: 包含公钥的字节数据
//
// 返回值:
//   - PubKey: 解析出的公钥
//   - error: 解析过程中的错误,如果成功则为 nil
func UnmarshalEd25519PublicKey(data []byte) (PubKey, error) {
	if len(data) != 32 {
		log.Debugf("解析 ed25519 公钥失败: 预期数据长度为 32, 实际长度为 %d", len(data))
		return nil, errors.New("预期 ed25519 公钥数据长度为 32")
	}

	return &Ed25519PublicKey{
		k: ed25519.PublicKey(data),
	}, nil
}

// UnmarshalEd25519PrivateKey 从字节数据解析出私钥
// 参数:
//   - data: 包含私钥的字节数据
//
// 返回值:
//   - PrivKey: 解析出的私钥
//   - error: 解析过程中的错误,如果成功则为 nil
func UnmarshalEd25519PrivateKey(data []byte) (PrivKey, error) {
	switch len(data) {
	case ed25519.PrivateKeySize + ed25519.PublicKeySize:
		// 移除冗余的公钥。参见 issue #36
		redundantPk := data[ed25519.PrivateKeySize:]
		pk := data[ed25519.PrivateKeySize-ed25519.PublicKeySize : ed25519.PrivateKeySize]
		if subtle.ConstantTimeCompare(pk, redundantPk) == 0 {
			log.Debugf("预期冗余的 ed25519 公钥应该是冗余的")
			return nil, errors.New("预期冗余的 ed25519 公钥应该是冗余的")
		}

		// 无需存储额外数据
		newKey := make([]byte, ed25519.PrivateKeySize)
		copy(newKey, data[:ed25519.PrivateKeySize])
		data = newKey
	case ed25519.PrivateKeySize:
	default:
		log.Debugf(
			"预期 ed25519 数据长度为 %d 或 %d, 实际长度为 %d",
			ed25519.PrivateKeySize,
			ed25519.PrivateKeySize+ed25519.PublicKeySize,
			len(data),
		)
		return nil, fmt.Errorf(
			"预期 ed25519 数据长度为 %d 或 %d, 实际长度为 %d",
			ed25519.PrivateKeySize,
			ed25519.PrivateKeySize+ed25519.PublicKeySize,
			len(data),
		)
	}

	return &Ed25519PrivateKey{
		k: ed25519.PrivateKey(data),
	}, nil
}
