package dep2pwebrtc

import (
	"crypto"
	"crypto/x509"
	"errors"

	ma "github.com/dep2p/go-dep2p/multiformats/multiaddr"
	"github.com/dep2p/go-dep2p/multiformats/multibase"
	mh "github.com/dep2p/go-dep2p/multiformats/multihash"
	"github.com/pion/webrtc/v4"
)

// parseFingerprint 从 pion 分叉而来,避免了字节到字符串的分配,并且避免了不必要的十六进制交错
// 注意: 这是一个内部方法,用于生成证书指纹

var errHashUnavailable = errors.New("指纹: 哈希算法未链接到二进制文件中")

// parseFingerprint 使用指定的哈希算法为证书创建指纹
// 参数:
//   - cert: *x509.Certificate 证书对象
//   - algo: crypto.Hash 哈希算法
//
// 返回值:
//   - []byte 生成的指纹字节数组
//   - error 可能的错误
func parseFingerprint(cert *x509.Certificate, algo crypto.Hash) ([]byte, error) {
	// 检查哈希算法是否可用
	if !algo.Available() {
		log.Debugf("哈希算法未链接到二进制文件中")
		return nil, errHashUnavailable
	}
	// 创建新的哈希对象
	h := algo.New()
	// Hash.Writer 被指定为永远不会返回错误
	// 参考: https://golang.org/pkg/hash/#Hash
	h.Write(cert.Raw)
	// 计算并返回哈希值
	return h.Sum(nil), nil
}

// decodeRemoteFingerprint 解码远程节点的指纹
// 参数:
//   - maddr: ma.Multiaddr 多地址对象
//
// 返回值:
//   - *mh.DecodedMultihash 解码后的多重哈希
//   - error 可能的错误
func decodeRemoteFingerprint(maddr ma.Multiaddr) (*mh.DecodedMultihash, error) {
	// 从多地址中获取证书哈希值
	remoteFingerprintMultibase, err := maddr.ValueForProtocol(ma.P_CERTHASH)
	if err != nil {
		log.Debugf("获取证书哈希值时出错: %s", err)
		return nil, err
	}
	// 解码多重基编码的数据
	_, data, err := multibase.Decode(remoteFingerprintMultibase)
	if err != nil {
		log.Debugf("解码多重基编码的数据时出错: %s", err)
		return nil, err
	}
	// 解码多重哈希数据
	return mh.Decode(data)
}

// encodeDTLSFingerprint 编码 DTLS 指纹
// 参数:
//   - fp: webrtc.DTLSFingerprint DTLS 指纹对象
//
// 返回值:
//   - string 编码后的指纹字符串
//   - error 可能的错误
func encodeDTLSFingerprint(fp webrtc.DTLSFingerprint) (string, error) {
	// 从 ASCII 字符串解码交错的十六进制数据
	digest, err := decodeInterspersedHexFromASCIIString(fp.Value)
	if err != nil {
		log.Debugf("解码交错的十六进制数据时出错: %s", err)
		return "", err
	}
	// 使用 SHA-256 算法编码摘要
	encoded, err := mh.Encode(digest, mh.SHA2_256)
	if err != nil {
		log.Debugf("编码摘要时出错: %s", err)
		return "", err
	}
	// 使用 Base64URL 编码结果
	return multibase.Encode(multibase.Base64url, encoded)
}
