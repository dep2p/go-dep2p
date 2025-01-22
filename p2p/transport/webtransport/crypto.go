package dep2pwebtransport

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math/big"
	"time"

	"golang.org/x/crypto/hkdf"

	ic "github.com/dep2p/go-dep2p/core/crypto"

	"github.com/dep2p/go-dep2p/multiformats/multihash"
	"github.com/quic-go/quic-go/http3"
)

// 用于生成确定性证书的信息字符串
const deterministicCertInfo = "determinisitic cert"

// getTLSConf 根据私钥和有效期生成 TLS 配置
// 参数:
//   - key: ic.PrivKey 私钥
//   - start: time.Time 证书生效时间
//   - end: time.Time 证书过期时间
//
// 返回值:
//   - *tls.Config TLS 配置对象
//   - error 错误信息
func getTLSConf(key ic.PrivKey, start, end time.Time) (*tls.Config, error) {
	// 生成证书和私钥
	cert, priv, err := generateCert(key, start, end)
	if err != nil {
		log.Debugf("生成证书和私钥失败: %s", err)
		return nil, err
	}
	// 返回 TLS 配置
	return &tls.Config{
		Certificates: []tls.Certificate{{
			Certificate: [][]byte{cert.Raw},
			PrivateKey:  priv,
			Leaf:        cert,
		}},
		NextProtos: []string{http3.NextProtoH3},
	}, nil
}

// generateCert 基于私钥和开始时间确定性地生成证书
// 使用 golang.org/x/crypto/hkdf 进行密钥派生
// 参数:
//   - key: ic.PrivKey 私钥
//   - start: time.Time 证书生效时间
//   - end: time.Time 证书过期时间
//
// 返回值:
//   - *x509.Certificate 生成的证书
//   - *ecdsa.PrivateKey 生成的 ECDSA 私钥
//   - error 错误信息
func generateCert(key ic.PrivKey, start, end time.Time) (*x509.Certificate, *ecdsa.PrivateKey, error) {
	// 获取私钥原始字节
	keyBytes, err := key.Raw()
	if err != nil {
		log.Debugf("获取私钥原始字节失败: %s", err)
		return nil, nil, err
	}

	// 使用开始时间作为盐值
	startTimeSalt := make([]byte, 8)
	binary.LittleEndian.PutUint64(startTimeSalt, uint64(start.UnixNano()))
	deterministicHKDFReader := newDeterministicReader(keyBytes, startTimeSalt, deterministicCertInfo)

	// 生成序列号
	b := make([]byte, 8)
	if _, err := deterministicHKDFReader.Read(b); err != nil {
		log.Debugf("生成序列号失败: %s", err)
		return nil, nil, err
	}
	serial := int64(binary.BigEndian.Uint64(b))
	if serial < 0 {
		serial = -serial
	}

	// 创建证书模板
	certTempl := &x509.Certificate{
		SerialNumber:          big.NewInt(serial),
		Subject:               pkix.Name{},
		NotBefore:             start,
		NotAfter:              end,
		IsCA:                  true,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}

	// 生成 CA 私钥
	caPrivateKey, err := ecdsa.GenerateKey(elliptic.P256(), deterministicHKDFReader)
	if err != nil {
		log.Debugf("生成 CA 私钥失败: %s", err)
		return nil, nil, err
	}

	// 创建证书
	caBytes, err := x509.CreateCertificate(deterministicHKDFReader, certTempl, certTempl, caPrivateKey.Public(), caPrivateKey)
	if err != nil {
		log.Debugf("创建证书失败: %s", err)
		return nil, nil, err
	}

	// 解析证书
	ca, err := x509.ParseCertificate(caBytes)
	if err != nil {
		log.Debugf("解析证书失败: %s", err)
		return nil, nil, err
	}
	return ca, caPrivateKey, nil
}

// verifyRawCerts 验证原始证书
// 参数:
//   - rawCerts: [][]byte 原始证书列表
//   - certHashes: []multihash.DecodedMultihash 证书哈希列表
//
// 返回值:
//   - error 错误信息
func verifyRawCerts(rawCerts [][]byte, certHashes []multihash.DecodedMultihash) error {
	// 检查是否有证书
	if len(rawCerts) < 1 {
		log.Debugf("没有证书")
		return errors.New("没有证书")
	}

	// 获取叶子证书
	leaf := rawCerts[len(rawCerts)-1]
	// W3C WebTransport 规范目前仅允许 SHA-256 证书哈希
	hash := sha256.Sum256(leaf)

	// 验证证书哈希
	var verified bool
	for _, h := range certHashes {
		if h.Code == multihash.SHA2_256 && bytes.Equal(h.Digest, hash[:]) {
			verified = true
			break
		}
	}
	if !verified {
		digests := make([][]byte, 0, len(certHashes))
		for _, h := range certHashes {
			digests = append(digests, h.Digest)
		}
		return fmt.Errorf("未找到证书哈希: %#x (期望: %#x)", hash, digests)
	}

	// 解析证书
	cert, err := x509.ParseCertificate(leaf)
	if err != nil {
		log.Debugf("解析证书失败: %s", err)
		return err
	}

	// 检查是否使用 RSA
	switch cert.SignatureAlgorithm {
	case x509.SHA1WithRSA, x509.SHA256WithRSA, x509.SHA384WithRSA, x509.SHA512WithRSA, x509.MD2WithRSA, x509.MD5WithRSA:
		log.Debugf("证书使用了 RSA")
		return errors.New("证书使用了 RSA")
	}

	// 检查证书有效期
	if l := cert.NotAfter.Sub(cert.NotBefore); l > 14*24*time.Hour {
		log.Debugf("证书有效期不能超过14天 (生效时间: %s, 过期时间: %s, 有效期: %s)", cert.NotBefore, cert.NotAfter, l)
		return fmt.Errorf("证书有效期不能超过14天 (生效时间: %s, 过期时间: %s, 有效期: %s)", cert.NotBefore, cert.NotAfter, l)
	}

	// 检查当前时间是否在有效期内
	now := time.Now()
	if now.Before(cert.NotBefore) || now.After(cert.NotAfter) {
		log.Debugf("证书无效 (生效时间: %s, 过期时间: %s)", cert.NotBefore, cert.NotAfter)
		return fmt.Errorf("证书无效 (生效时间: %s, 过期时间: %s)", cert.NotBefore, cert.NotAfter)
	}
	return nil
}

// deterministicReader 是一个特殊的读取器
// 它用于抵消 Go 库试图使 ECDSA 签名非确定性的行为
// Go 通过随机丢弃读取流中的单个字节来添加非确定性
// 这里通过检测是否读取单个字节并使用不同的读取器来抵消这种行为
type deterministicReader struct {
	reader           io.Reader // 普通读取器
	singleByteReader io.Reader // 单字节读取器
}

// newDeterministicReader 创建新的确定性读取器
// 参数:
//   - seed: []byte 种子
//   - salt: []byte 盐值
//   - info: string 信息字符串
//
// 返回值:
//   - io.Reader 确定性读取器
func newDeterministicReader(seed []byte, salt []byte, info string) io.Reader {
	reader := hkdf.New(sha256.New, seed, salt, []byte(info))
	singleByteReader := hkdf.New(sha256.New, seed, salt, []byte(info+" single byte"))

	return &deterministicReader{
		reader:           reader,
		singleByteReader: singleByteReader,
	}
}

// Read 实现 io.Reader 接口
// 参数:
//   - p: []byte 读取缓冲区
//
// 返回值:
//   - n: int 读取的字节数
//   - err: error 错误信息
func (r *deterministicReader) Read(p []byte) (n int, err error) {
	if len(p) == 1 {
		return r.singleByteReader.Read(p)
	}
	return r.reader.Read(p)
}
