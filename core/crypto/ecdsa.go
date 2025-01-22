package crypto

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/asn1"
	"errors"
	"io"
	"math/big"

	pb "github.com/dep2p/go-dep2p/core/crypto/pb"
	"github.com/dep2p/go-dep2p/core/internal/catch"
	logging "github.com/dep2p/log"
)

var log = logging.Logger("core-crypto")

// ECDSAPrivateKey 实现了 ECDSA 私钥
type ECDSAPrivateKey struct {
	priv *ecdsa.PrivateKey // 内部 ECDSA 私钥
}

// ECDSAPublicKey 实现了 ECDSA 公钥
type ECDSAPublicKey struct {
	pub *ecdsa.PublicKey // 内部 ECDSA 公钥
}

// ECDSASig 保存 ECDSA 签名的 r 和 s 值
type ECDSASig struct {
	R, S *big.Int // 签名的 r 和 s 分量
}

var (
	// ErrNotECDSAPubKey 当传入的公钥不是 ECDSA 公钥时返回此错误
	ErrNotECDSAPubKey = errors.New("不是 ECDSA 公钥")
	// ErrNilSig 当签名为 nil 时返回此错误
	ErrNilSig = errors.New("签名为空")
	// ErrNilPrivateKey 当提供的私钥为 nil 时返回此错误
	ErrNilPrivateKey = errors.New("私钥为空")
	// ErrNilPublicKey 当提供的公钥为 nil 时返回此错误
	ErrNilPublicKey = errors.New("公钥为空")
	// ECDSACurve 是默认使用的 ECDSA 曲线
	ECDSACurve = elliptic.P256()
)

// GenerateECDSAKeyPair 生成新的 ECDSA 私钥和公钥对
// 参数:
//   - src: 随机数生成器
//
// 返回值:
//   - PrivKey: 生成的私钥
//   - PubKey: 生成的公钥
//   - error: 错误信息
func GenerateECDSAKeyPair(src io.Reader) (PrivKey, PubKey, error) {
	return GenerateECDSAKeyPairWithCurve(ECDSACurve, src)
}

// GenerateECDSAKeyPairWithCurve 使用指定曲线生成新的 ECDSA 私钥和公钥对
// 参数:
//   - curve: 指定的椭圆曲线
//   - src: 随机数生成器
//
// 返回值:
//   - PrivKey: 生成的私钥
//   - PubKey: 生成的公钥
//   - error: 错误信息
func GenerateECDSAKeyPairWithCurve(curve elliptic.Curve, src io.Reader) (PrivKey, PubKey, error) {
	priv, err := ecdsa.GenerateKey(curve, src)
	if err != nil {
		log.Debugf("生成 ECDSA 密钥对失败: %v", err)
		return nil, nil, err
	}

	return &ECDSAPrivateKey{priv}, &ECDSAPublicKey{&priv.PublicKey}, nil
}

// ECDSAKeyPairFromKey 从输入的私钥生成 ECDSA 私钥和公钥对
// 参数:
//   - priv: 输入的 ECDSA 私钥
//
// 返回值:
//   - PrivKey: 生成的私钥
//   - PubKey: 生成的公钥
//   - error: 错误信息
func ECDSAKeyPairFromKey(priv *ecdsa.PrivateKey) (PrivKey, PubKey, error) {
	if priv == nil {
		log.Debugf("私钥为空")
		return nil, nil, ErrNilPrivateKey
	}

	return &ECDSAPrivateKey{priv}, &ECDSAPublicKey{&priv.PublicKey}, nil
}

// ECDSAPublicKeyFromPubKey 从输入的公钥生成 ECDSA 公钥
// 参数:
//   - pub: 输入的 ECDSA 公钥
//
// 返回值:
//   - PubKey: 生成的公钥
//   - error: 错误信息
func ECDSAPublicKeyFromPubKey(pub ecdsa.PublicKey) (PubKey, error) {
	return &ECDSAPublicKey{pub: &pub}, nil
}

// MarshalECDSAPrivateKey 将私钥转换为 x509 字节格式
// 参数:
//   - ePriv: ECDSA 私钥
//
// 返回值:
//   - []byte: x509 编码的私钥
//   - error: 错误信息
func MarshalECDSAPrivateKey(ePriv ECDSAPrivateKey) (res []byte, err error) {
	defer func() { catch.HandlePanic(recover(), &err, "ECDSA private-key marshal") }()
	return x509.MarshalECPrivateKey(ePriv.priv)
}

// MarshalECDSAPublicKey 将公钥转换为 x509 字节格式
// 参数:
//   - ePub: ECDSA 公钥
//
// 返回值:
//   - []byte: x509 编码的公钥
//   - error: 错误信息
func MarshalECDSAPublicKey(ePub ECDSAPublicKey) (res []byte, err error) {
	defer func() { catch.HandlePanic(recover(), &err, "ECDSA public-key marshal") }()
	return x509.MarshalPKIXPublicKey(ePub.pub)
}

// UnmarshalECDSAPrivateKey 从 x509 字节格式解析私钥
// 参数:
//   - data: x509 编码的私钥数据
//
// 返回值:
//   - PrivKey: 解析出的私钥
//   - error: 错误信息
func UnmarshalECDSAPrivateKey(data []byte) (res PrivKey, err error) {
	defer func() { catch.HandlePanic(recover(), &err, "ECDSA private-key unmarshal") }()

	priv, err := x509.ParseECPrivateKey(data)
	if err != nil {
		log.Debugf("解析 ECDSA 私钥失败: %v", err)
		return nil, err
	}

	return &ECDSAPrivateKey{priv}, nil
}

// UnmarshalECDSAPublicKey 从 x509 字节格式解析公钥
// 参数:
//   - data: x509 编码的公钥数据
//
// 返回值:
//   - PubKey: 解析出的公钥
//   - error: 错误信息
func UnmarshalECDSAPublicKey(data []byte) (key PubKey, err error) {
	defer func() { catch.HandlePanic(recover(), &err, "ECDSA public-key unmarshal") }()

	pubIfc, err := x509.ParsePKIXPublicKey(data)
	if err != nil {
		log.Debugf("解析 ECDSA 公钥失败: %v", err)
		return nil, err
	}

	pub, ok := pubIfc.(*ecdsa.PublicKey)
	if !ok {
		log.Debugf("解析 ECDSA 公钥失败: %v", ErrNotECDSAPubKey)
		return nil, ErrNotECDSAPubKey
	}

	return &ECDSAPublicKey{pub}, nil
}

// Type 返回私钥类型
// 返回值:
//   - pb.KeyType: 密钥类型
func (ePriv *ECDSAPrivateKey) Type() pb.KeyType {
	return pb.KeyType_ECDSA
}

// Raw 将私钥转换为 x509 字节格式
// 返回值:
//   - []byte: x509 编码的私钥
//   - error: 错误信息
func (ePriv *ECDSAPrivateKey) Raw() (res []byte, err error) {
	defer func() { catch.HandlePanic(recover(), &err, "ECDSA private-key marshal") }()
	return x509.MarshalECPrivateKey(ePriv.priv)
}

// Equals 比较两个私钥是否相等
// 参数:
//   - o: 要比较的另一个密钥
//
// 返回值:
//   - bool: 是否相等
func (ePriv *ECDSAPrivateKey) Equals(o Key) bool {
	return basicEquals(ePriv, o)
}

// Sign 对输入数据进行签名
// 参数:
//   - data: 要签名的数据
//
// 返回值:
//   - []byte: 签名结果
//   - error: 错误信息
func (ePriv *ECDSAPrivateKey) Sign(data []byte) (sig []byte, err error) {
	defer func() { catch.HandlePanic(recover(), &err, "ECDSA signing") }()
	hash := sha256.Sum256(data)
	r, s, err := ecdsa.Sign(rand.Reader, ePriv.priv, hash[:])
	if err != nil {
		log.Debugf("ECDSA 签名失败: %v", err)
		return nil, err
	}

	return asn1.Marshal(ECDSASig{
		R: r,
		S: s,
	})
}

// GetPublic 获取对应的公钥
// 返回值:
//   - PubKey: 对应的公钥
func (ePriv *ECDSAPrivateKey) GetPublic() PubKey {
	return &ECDSAPublicKey{&ePriv.priv.PublicKey}
}

// Type 返回公钥类型
// 返回值:
//   - pb.KeyType: 密钥类型
func (ePub *ECDSAPublicKey) Type() pb.KeyType {
	return pb.KeyType_ECDSA
}

// Raw 将公钥转换为 x509 字节格式
// 返回值:
//   - []byte: x509 编码的公钥
//   - error: 错误信息
func (ePub *ECDSAPublicKey) Raw() ([]byte, error) {
	return x509.MarshalPKIXPublicKey(ePub.pub)
}

// Equals 比较两个公钥是否相等
// 参数:
//   - o: 要比较的另一个密钥
//
// 返回值:
//   - bool: 是否相等
func (ePub *ECDSAPublicKey) Equals(o Key) bool {
	return basicEquals(ePub, o)
}

// Verify 验证数据和签名是否匹配
// 参数:
//   - data: 原始数据
//   - sigBytes: 签名数据
//
// 返回值:
//   - bool: 验证是否通过
//   - error: 错误信息
func (ePub *ECDSAPublicKey) Verify(data, sigBytes []byte) (success bool, err error) {
	defer func() {
		catch.HandlePanic(recover(), &err, "ECDSA signature verification")

		// 额外的安全检查
		if err != nil {
			log.Debugf("ECDSA 签名验证失败: %v", err)
			success = false
		}
	}()

	sig := new(ECDSASig)
	if _, err := asn1.Unmarshal(sigBytes, sig); err != nil {
		log.Debugf("ECDSA 签名验证失败: %v", err)
		return false, err
	}

	hash := sha256.Sum256(data)

	return ecdsa.Verify(ePub.pub, hash[:], sig.R, sig.S), nil
}
