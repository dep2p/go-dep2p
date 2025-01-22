package crypto

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"errors"
	"io"

	pb "github.com/dep2p/go-dep2p/core/crypto/pb"
	"github.com/dep2p/go-dep2p/core/internal/catch"
)

// RsaPrivateKey RSA私钥结构体
type RsaPrivateKey struct {
	sk rsa.PrivateKey // 标准库RSA私钥
}

// RsaPublicKey RSA公钥结构体
type RsaPublicKey struct {
	k rsa.PublicKey // 标准库RSA公钥

	cached []byte // 缓存的序列化公钥数据
}

// GenerateRSAKeyPair 生成新的RSA密钥对
// 参数:
//   - bits: RSA密钥的位数
//   - src: 随机数生成器
//
// 返回值:
//   - PrivKey: 生成的私钥
//   - PubKey: 生成的公钥
//   - error: 错误信息
func GenerateRSAKeyPair(bits int, src io.Reader) (PrivKey, PubKey, error) {
	// 检查密钥位数是否小于最小要求
	if bits < MinRsaKeyBits {
		log.Debugf("密钥位数小于最小要求: %d < %d", bits, MinRsaKeyBits)
		return nil, nil, ErrRsaKeyTooSmall
	}
	// 检查密钥位数是否超过最大限制
	if bits > maxRsaKeyBits {
		log.Debugf("密钥位数超过最大限制: %d > %d", bits, maxRsaKeyBits)
		return nil, nil, ErrRsaKeyTooBig
	}
	// 生成RSA密钥对
	priv, err := rsa.GenerateKey(src, bits)
	if err != nil {
		log.Debugf("生成RSA密钥对失败: %v", err)
		return nil, nil, err
	}
	pk := priv.PublicKey
	return &RsaPrivateKey{sk: *priv}, &RsaPublicKey{k: pk}, nil
}

// Verify 验证数据签名是否有效
// 参数:
//   - data: 原始数据
//   - sig: 签名数据
//
// 返回值:
//   - bool: 签名是否有效
//   - error: 错误信息
func (pk *RsaPublicKey) Verify(data, sig []byte) (success bool, err error) {
	defer func() {
		catch.HandlePanic(recover(), &err, "RSA signature verification")

		// 确保发生错误时返回false
		if err != nil {
			success = false
		}
	}()
	// 计算数据的SHA256哈希
	hashed := sha256.Sum256(data)
	// 验证PKCS1v15签名
	err = rsa.VerifyPKCS1v15(&pk.k, crypto.SHA256, hashed[:], sig)
	if err != nil {
		log.Debugf("RSA签名验证失败: %v", err)
		return false, err
	}
	return true, nil
}

// Type 返回密钥类型
// 返回值:
//   - pb.KeyType: RSA密钥类型
func (pk *RsaPublicKey) Type() pb.KeyType {
	return pb.KeyType_RSA
}

// Raw 返回公钥的PKIX编码
// 返回值:
//   - []byte: 编码后的公钥数据
//   - error: 错误信息
func (pk *RsaPublicKey) Raw() (res []byte, err error) {
	defer func() { catch.HandlePanic(recover(), &err, "RSA public-key marshaling") }()
	return x509.MarshalPKIXPublicKey(&pk.k)
}

// Equals 检查两个密钥是否相等
// 参数:
//   - k: 要比较的密钥
//
// 返回值:
//   - bool: 密钥是否相等
func (pk *RsaPublicKey) Equals(k Key) bool {
	// 检查是否为RSA公钥
	other, ok := (k).(*RsaPublicKey)
	if !ok {
		log.Debugf("密钥类型不匹配: %v != %v", pk.Type(), k.Type())
		return basicEquals(pk, k)
	}

	// 比较模数N和指数E
	return pk.k.N.Cmp(other.k.N) == 0 && pk.k.E == other.k.E
}

// Sign 对输入数据进行签名
// 参数:
//   - message: 要签名的数据
//
// 返回值:
//   - []byte: 签名数据
//   - error: 错误信息
func (sk *RsaPrivateKey) Sign(message []byte) (sig []byte, err error) {
	defer func() { catch.HandlePanic(recover(), &err, "RSA signing") }()
	// log.Infof("RSA签名")
	// 计算消息的SHA256哈希
	hashed := sha256.Sum256(message)
	// 使用PKCS1v15进行签名
	return rsa.SignPKCS1v15(rand.Reader, &sk.sk, crypto.SHA256, hashed[:])
}

// GetPublic 获取对应的公钥
// 返回值:
//   - PubKey: 对应的公钥
func (sk *RsaPrivateKey) GetPublic() PubKey {
	return &RsaPublicKey{k: sk.sk.PublicKey}
}

// Type 返回密钥类型
// 返回值:
//   - pb.KeyType: RSA密钥类型
func (sk *RsaPrivateKey) Type() pb.KeyType {
	return pb.KeyType_RSA
}

// Raw 返回私钥的PKCS1编码
// 返回值:
//   - []byte: 编码后的私钥数据
//   - error: 错误信息
func (sk *RsaPrivateKey) Raw() (res []byte, err error) {
	defer func() { catch.HandlePanic(recover(), &err, "RSA private-key marshaling") }()
	// log.Infof("RSA私钥编码")
	b := x509.MarshalPKCS1PrivateKey(&sk.sk)
	return b, nil
}

// Equals 检查两个密钥是否相等
// 参数:
//   - k: 要比较的密钥
//
// 返回值:
//   - bool: 密钥是否相等
func (sk *RsaPrivateKey) Equals(k Key) bool {
	// 检查是否为RSA私钥
	other, ok := (k).(*RsaPrivateKey)
	if !ok {
		log.Debugf("密钥类型不匹配: %v != %v", sk.Type(), k.Type())
		return basicEquals(sk, k)
	}

	a := sk.sk
	b := other.sk

	// 只比较公钥部分,不需要常量时间比较
	return a.PublicKey.N.Cmp(b.PublicKey.N) == 0 && a.PublicKey.E == b.PublicKey.E
}

// UnmarshalRsaPrivateKey 从PKCS1编码的数据中解析RSA私钥
// 参数:
//   - b: PKCS1编码的私钥数据
//
// 返回值:
//   - PrivKey: 解析出的私钥
//   - error: 错误信息
func UnmarshalRsaPrivateKey(b []byte) (key PrivKey, err error) {
	defer func() { catch.HandlePanic(recover(), &err, "RSA private-key unmarshaling") }()
	// log.Infof("RSA私钥反序列化")
	// 解析PKCS1格式的私钥
	sk, err := x509.ParsePKCS1PrivateKey(b)
	if err != nil {
		log.Debugf("RSA私钥反序列化失败: %v", err)
		return nil, err
	}
	// 检查密钥长度
	if sk.N.BitLen() < MinRsaKeyBits {
		log.Debugf("密钥长度小于最小要求: %d < %d", sk.N.BitLen(), MinRsaKeyBits)
		return nil, ErrRsaKeyTooSmall
	}
	if sk.N.BitLen() > maxRsaKeyBits {
		log.Debugf("密钥长度超过最大限制: %d > %d", sk.N.BitLen(), maxRsaKeyBits)
		return nil, ErrRsaKeyTooBig
	}
	return &RsaPrivateKey{sk: *sk}, nil
}

// UnmarshalRsaPublicKey 从PKIX编码的数据中解析RSA公钥
// 参数:
//   - b: PKIX编码的公钥数据
//
// 返回值:
//   - PubKey: 解析出的公钥
//   - error: 错误信息
func UnmarshalRsaPublicKey(b []byte) (key PubKey, err error) {
	defer func() { catch.HandlePanic(recover(), &err, "RSA public-key unmarshaling") }()
	// 解析PKIX格式的公钥
	pub, err := x509.ParsePKIXPublicKey(b)
	if err != nil {
		log.Debugf("RSA公钥反序列化失败: %v", err)
		return nil, err
	}
	// 类型断言为RSA公钥
	pk, ok := pub.(*rsa.PublicKey)
	if !ok {
		log.Debugf("公钥不是RSA公钥")
		return nil, errors.New("不是有效的RSA公钥")
	}
	// 检查密钥长度
	if pk.N.BitLen() < MinRsaKeyBits {
		log.Debugf("密钥长度小于最小要求: %d < %d", pk.N.BitLen(), MinRsaKeyBits)
		return nil, ErrRsaKeyTooSmall
	}
	if pk.N.BitLen() > maxRsaKeyBits {
		log.Debugf("密钥长度超过最大限制: %d > %d", pk.N.BitLen(), maxRsaKeyBits)
		return nil, ErrRsaKeyTooBig
	}

	return &RsaPublicKey{k: *pk}, nil
}
