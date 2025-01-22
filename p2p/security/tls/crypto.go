package dep2ptls

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"errors"
	"fmt"
	"io"
	"math/big"
	"os"
	"runtime/debug"
	"time"

	ic "github.com/dep2p/go-dep2p/core/crypto"
	"github.com/dep2p/go-dep2p/core/peer"
	"github.com/dep2p/go-dep2p/core/sec"
	logging "github.com/dep2p/log"
)

// 证书有效期约100年
const certValidityPeriod = 100 * 365 * 24 * time.Hour

// dep2p TLS握手证书前缀
const certificatePrefix = "dep2p-tls-handshake:"

// ALPN协议标识符
const alpn string = "dep2p"

var log = logging.Logger("p2p-security-tls-crypto")

// 获取带前缀的扩展ID
var extensionID = getPrefixedExtensionID([]int{1, 1})

// 扩展是否为关键扩展,用于测试时标记
var extensionCritical bool

// signedKey 包含公钥及其签名
type signedKey struct {
	// 公钥字节
	PubKey []byte
	// 签名字节
	Signature []byte
}

// Identity 用于安全连接
type Identity struct {
	// TLS配置
	config tls.Config
}

// IdentityConfig 用于配置Identity
type IdentityConfig struct {
	// 证书模板
	CertTemplate *x509.Certificate
	// 密钥日志写入器
	KeyLogWriter io.Writer
}

// IdentityOption 用于转换IdentityConfig以应用可选设置
type IdentityOption func(r *IdentityConfig)

// WithCertTemplate 指定生成新证书时使用的模板
// 参数:
//   - template: *x509.Certificate 证书模板
//
// 返回值:
//   - IdentityOption 身份配置选项函数
func WithCertTemplate(template *x509.Certificate) IdentityOption {
	return func(c *IdentityConfig) {
		c.CertTemplate = template
	}
}

// WithKeyLogWriter 可选地指定用于TLS主密钥的NSS密钥日志格式的目标
// 这可以允许Wireshark等外部程序解密TLS连接
// 参数:
//   - w: io.Writer 日志写入器
//
// 返回值:
//   - IdentityOption 身份配置选项函数
//
// 注意:
//   - 使用KeyLogWriter会降低安全性,仅用于调试
func WithKeyLogWriter(w io.Writer) IdentityOption {
	return func(c *IdentityConfig) {
		c.KeyLogWriter = w
	}
}

// NewIdentity 创建新的身份
// 参数:
//   - privKey: ic.PrivKey 私钥
//   - opts: ...IdentityOption 可选的配置选项
//
// 返回值:
//   - *Identity 身份对象
//   - error 错误信息
func NewIdentity(privKey ic.PrivKey, opts ...IdentityOption) (*Identity, error) {
	config := IdentityConfig{}
	for _, opt := range opts {
		opt(&config)
	}

	var err error
	if config.CertTemplate == nil {
		config.CertTemplate, err = certTemplate()
		if err != nil {
			log.Debugf("生成证书模板时出错: %s", err)
			return nil, err
		}
	}

	cert, err := keyToCertificate(privKey, config.CertTemplate)
	if err != nil {
		log.Debugf("生成证书时出错: %s", err)
		return nil, err
	}
	return &Identity{
		config: tls.Config{
			MinVersion:         tls.VersionTLS13,
			InsecureSkipVerify: true, // 这里不会不安全,我们会自己验证证书链
			ClientAuth:         tls.RequireAnyClientCert,
			Certificates:       []tls.Certificate{*cert},
			VerifyPeerCertificate: func(_ [][]byte, _ [][]*x509.Certificate) error {
				panic("TLS配置未针对对等节点进行特化")
			},
			NextProtos:             []string{alpn},
			SessionTicketsDisabled: true,
			KeyLogWriter:           config.KeyLogWriter,
		},
	}, nil
}

// ConfigForPeer 创建新的单次使用TLS配置,用于验证对等节点的证书链
// 并通过channel返回对等节点的公钥
// 参数:
//   - remote: peer.ID 远程节点ID
//
// 返回值:
//   - *tls.Config TLS配置
//   - <-chan ic.PubKey 公钥通道
//
// 注意:
//   - 如果peer ID为空,返回的配置将接受任何对等节点
func (i *Identity) ConfigForPeer(remote peer.ID) (*tls.Config, <-chan ic.PubKey) {
	keyCh := make(chan ic.PubKey, 1)
	// 我们需要在VerifyPeerCertificate回调中检查对等节点ID
	// tls.Config也用于监听,我们可能有并发拨号
	// 克隆它以便我们可以在这里检查特定的对等节点ID
	conf := i.config.Clone()
	// 我们使用InsecureSkipVerify,所以verifiedChains参数将始终为空
	// 我们需要从原始证书自己解析证书
	conf.VerifyPeerCertificate = func(rawCerts [][]byte, _ [][]*x509.Certificate) (err error) {
		defer func() {
			if rerr := recover(); rerr != nil {
				fmt.Fprintf(os.Stderr, "TLS握手中处理对等证书时发生panic: %s\n%s\n", rerr, debug.Stack())
				err = fmt.Errorf("TLS握手中处理对等证书时发生panic: %s", rerr)

			}
		}()

		defer close(keyCh)

		chain := make([]*x509.Certificate, len(rawCerts))
		for i := 0; i < len(rawCerts); i++ {
			cert, err := x509.ParseCertificate(rawCerts[i])
			if err != nil {
				log.Debugf("解析证书时出错: %s", err)
				return err
			}
			chain[i] = cert
		}

		pubKey, err := PubKeyFromCertChain(chain)
		if err != nil {
			log.Debugf("从证书链中提取公钥时出错: %s", err)
			return err
		}
		if remote != "" && !remote.MatchesPublicKey(pubKey) {
			peerID, err := peer.IDFromPublicKey(pubKey)
			if err != nil {
				peerID = peer.ID(fmt.Sprintf("(未确定: %s)", err.Error()))
			}
			return sec.ErrPeerIDMismatch{Expected: remote, Actual: peerID}
		}
		keyCh <- pubKey
		return nil
	}
	return conf, keyCh
}

// PubKeyFromCertChain 验证证书链并提取远程节点的公钥
// 参数:
//   - chain: []*x509.Certificate 证书链
//
// 返回值:
//   - ic.PubKey 公钥
//   - error 错误信息
func PubKeyFromCertChain(chain []*x509.Certificate) (ic.PubKey, error) {
	if len(chain) != 1 {
		log.Debugf("证书链中应该只有一个证书")
		return nil, errors.New("证书链中应该只有一个证书")
	}
	cert := chain[0]
	pool := x509.NewCertPool()
	pool.AddCert(cert)
	var found bool
	var keyExt pkix.Extension
	// 查找dep2p密钥扩展,跳过所有未知扩展
	for _, ext := range cert.Extensions {
		if extensionIDEqual(ext.Id, extensionID) {
			keyExt = ext
			found = true
			for i, oident := range cert.UnhandledCriticalExtensions {
				if oident.Equal(ext.Id) {
					// 从UnhandledCriticalExtensions中删除扩展
					cert.UnhandledCriticalExtensions = append(cert.UnhandledCriticalExtensions[:i], cert.UnhandledCriticalExtensions[i+1:]...)
					break
				}
			}
			break
		}
	}
	if !found {
		log.Debugf("证书应包含密钥扩展")
		return nil, errors.New("证书应包含密钥扩展")
	}
	if _, err := cert.Verify(x509.VerifyOptions{Roots: pool}); err != nil {
		// 如果我们在这里返回x509错误,它将被发送到网络上
		// 包装错误以避免这种情况
		log.Debugf("证书验证失败: %s", err)
		return nil, err
	}

	var sk signedKey
	if _, err := asn1.Unmarshal(keyExt.Value, &sk); err != nil {
		log.Debugf("解析签名证书失败: %s", err)
		return nil, err
	}
	pubKey, err := ic.UnmarshalPublicKey(sk.PubKey)
	if err != nil {
		log.Debugf("解析公钥失败: %s", err)
		return nil, err
	}
	certKeyPub, err := x509.MarshalPKIXPublicKey(cert.PublicKey)
	if err != nil {
		log.Debugf("解析公钥失败: %s", err)
		return nil, err
	}
	valid, err := pubKey.Verify(append([]byte(certificatePrefix), certKeyPub...), sk.Signature)
	if err != nil {
		log.Debugf("签名验证失败: %s", err)
		return nil, err
	}
	if !valid {
		log.Debugf("签名无效")
		return nil, errors.New("签名无效")
	}
	return pubKey, nil
}

// GenerateSignedExtension 使用提供的私钥签名公钥,并在pkix.Extension中返回签名
// 参数:
//   - sk: ic.PrivKey 私钥
//   - pubKey: crypto.PublicKey 公钥
//
// 返回值:
//   - pkix.Extension 扩展对象
//   - error 错误信息
//
// 注意:
//   - 此扩展包含在证书中以加密方式将其绑定到dep2p私钥
func GenerateSignedExtension(sk ic.PrivKey, pubKey crypto.PublicKey) (pkix.Extension, error) {
	keyBytes, err := ic.MarshalPublicKey(sk.GetPublic())
	if err != nil {
		log.Debugf("序列化公钥失败: %s", err)
		return pkix.Extension{}, err
	}
	certKeyPub, err := x509.MarshalPKIXPublicKey(pubKey)
	if err != nil {
		log.Debugf("序列化公钥失败: %s", err)
		return pkix.Extension{}, err
	}
	signature, err := sk.Sign(append([]byte(certificatePrefix), certKeyPub...))
	if err != nil {
		log.Debugf("签名失败: %s", err)
		return pkix.Extension{}, err
	}
	value, err := asn1.Marshal(signedKey{
		PubKey:    keyBytes,
		Signature: signature,
	})
	if err != nil {
		log.Debugf("序列化签名证书失败: %s", err)
		return pkix.Extension{}, err
	}

	return pkix.Extension{Id: extensionID, Critical: extensionCritical, Value: value}, nil
}

// keyToCertificate 生成新的ECDSA私钥和对应的x509证书
// 参数:
//   - sk: ic.PrivKey 私钥
//   - certTmpl: *x509.Certificate 证书模板
//
// 返回值:
//   - *tls.Certificate TLS证书
//   - error 错误信息
//
// 注意:
//   - 证书包含一个扩展,将其加密绑定到提供的dep2p私钥以验证TLS连接
func keyToCertificate(sk ic.PrivKey, certTmpl *x509.Certificate) (*tls.Certificate, error) {
	certKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		log.Debugf("生成密钥对时出错: %s", err)
		return nil, err
	}

	// 调用CreateCertificate后,这些将最终出现在Certificate.Extensions中
	extension, err := GenerateSignedExtension(sk, certKey.Public())
	if err != nil {
		log.Debugf("生成签名证书时出错: %s", err)
		return nil, err
	}
	certTmpl.ExtraExtensions = append(certTmpl.ExtraExtensions, extension)

	certDER, err := x509.CreateCertificate(rand.Reader, certTmpl, certTmpl, certKey.Public(), certKey)
	if err != nil {
		log.Debugf("生成证书时出错: %s", err)
		return nil, err
	}
	return &tls.Certificate{
		Certificate: [][]byte{certDER},
		PrivateKey:  certKey,
	}, nil
}

// certTemplate 返回用于生成身份TLS证书的模板
// 返回值:
//   - *x509.Certificate 证书模板
//   - error 错误信息
func certTemplate() (*x509.Certificate, error) {
	bigNum := big.NewInt(1 << 62)
	sn, err := rand.Int(rand.Reader, bigNum)
	if err != nil {
		log.Debugf("生成证书时出错: %s", err)
		return nil, err
	}

	subjectSN, err := rand.Int(rand.Reader, bigNum)
	if err != nil {
		log.Debugf("生成证书时出错: %s", err)
		return nil, err
	}

	return &x509.Certificate{
		SerialNumber: sn,
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(certValidityPeriod),
		// 根据RFC 3280,必须设置颁发者字段
		// 参见 https://datatracker.ietf.org/doc/html/rfc3280#section-4.1.2.4
		Subject: pkix.Name{SerialNumber: subjectSN.String()},
	}, nil
}
