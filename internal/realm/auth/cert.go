package auth

import (
	"context"
	"crypto/ecdsa"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/dep2p/go-dep2p/internal/realm/interfaces"
)

// ============================================================================
//                              证书认证器
// ============================================================================

// CertAuthenticator 证书认证器
type CertAuthenticator struct {
	// 证书配置
	certPath string
	keyPath  string
	peerID   string
	realmID  string

	// 证书和密钥
	mu          sync.RWMutex
	certificate *tls.Certificate
	certPool    *x509.CertPool

	// 状态
	closed bool
}

// NewCertAuthenticator 创建证书认证器
func NewCertAuthenticator(certPath, keyPath, peerID string) (*CertAuthenticator, error) {
	if certPath == "" || keyPath == "" {
		return nil, fmt.Errorf("%w: cert and key paths are required", ErrInvalidConfig)
	}

	if peerID == "" {
		return nil, fmt.Errorf("%w: peerID cannot be empty", ErrInvalidConfig)
	}

	// 检查文件是否存在
	if _, err := os.Stat(certPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("%w: certificate file not found: %s", ErrInvalidCert, certPath)
	}

	if _, err := os.Stat(keyPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("%w: key file not found: %s", ErrInvalidCert, keyPath)
	}

	auth := &CertAuthenticator{
		certPath: certPath,
		keyPath:  keyPath,
		peerID:   peerID,
		certPool: x509.NewCertPool(),
	}

	// 加载证书
	if err := auth.loadCertificate(); err != nil {
		return nil, err
	}

	// 从证书派生 RealmID
	auth.realmID = auth.derivRealmIDFromCert()

	return auth, nil
}

// loadCertificate 加载证书
func (a *CertAuthenticator) loadCertificate() error {
	cert, err := tls.LoadX509KeyPair(a.certPath, a.keyPath)
	if err != nil {
		return fmt.Errorf("%w: failed to load certificate: %v", ErrInvalidCert, err)
	}

	a.mu.Lock()
	a.certificate = &cert
	a.mu.Unlock()

	// 解析证书以验证有效性
	if len(cert.Certificate) > 0 {
		x509Cert, err := x509.ParseCertificate(cert.Certificate[0])
		if err != nil {
			return fmt.Errorf("%w: failed to parse certificate: %v", ErrInvalidCert, err)
		}

		// 添加到证书池
		a.certPool.AddCert(x509Cert)
	}

	return nil
}

// derivRealmIDFromCert 从证书派生 RealmID
func (a *CertAuthenticator) derivRealmIDFromCert() string {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if a.certificate == nil || len(a.certificate.Certificate) == 0 {
		return ""
	}

	cert, err := x509.ParseCertificate(a.certificate.Certificate[0])
	if err != nil {
		return ""
	}

	// 使用证书指纹作为 RealmID
	return fmt.Sprintf("cert-%x", cert.SerialNumber)
}

// Mode 返回认证模式
func (a *CertAuthenticator) Mode() interfaces.AuthMode {
	return interfaces.AuthModeCert
}

// RealmID 返回 Realm ID
func (a *CertAuthenticator) RealmID() string {
	return a.realmID
}

// GenerateProof 生成认证证明（证书签名）
func (a *CertAuthenticator) GenerateProof(_ context.Context) ([]byte, error) {
	a.mu.RLock()
	closed := a.closed
	cert := a.certificate
	a.mu.RUnlock()

	if closed {
		return nil, ErrAuthenticatorClosed
	}

	if cert == nil || len(cert.Certificate) == 0 {
		return nil, ErrInvalidCert
	}

	// 返回证书（DER 编码）
	return cert.Certificate[0], nil
}

// Authenticate 验证证书
func (a *CertAuthenticator) Authenticate(_ context.Context, peerID string, proof []byte) (bool, error) {
	a.mu.RLock()
	closed := a.closed
	a.mu.RUnlock()

	if closed {
		return false, ErrAuthenticatorClosed
	}

	if len(proof) == 0 {
		return false, ErrInvalidProof
	}

	// 解析证书
	cert, err := x509.ParseCertificate(proof)
	if err != nil {
		return false, fmt.Errorf("%w: failed to parse certificate: %v", ErrInvalidCert, err)
	}

	// 验证证书有效期
	now := context.Background()
	if err := a.verifyCertificate(cert); err != nil {
		return false, err
	}

	_ = now // 避免未使用警告
	return true, nil
}

// verifyCertificate 验证证书
func (a *CertAuthenticator) verifyCertificate(cert *x509.Certificate) error {
	// 创建验证选项
	opts := x509.VerifyOptions{
		Roots:     a.certPool,
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageAny},
	}

	// 验证证书链
	if _, err := cert.Verify(opts); err != nil {
		// 如果证书池为空，只验证基本信息
		// nolint:staticcheck // SA1019: Subjects() 仍适用于检查本地添加的证书
		if len(a.certPool.Subjects()) == 0 {
			// 自签名证书，只检查有效期
			return a.verifyBasicCertInfo(cert)
		}
		return fmt.Errorf("%w: certificate verification failed: %v", ErrInvalidCert, err)
	}

	return nil
}

// verifyBasicCertInfo 验证基本证书信息
//
// Phase 11 修复：实现真实的证书验证
func (a *CertAuthenticator) verifyBasicCertInfo(cert *x509.Certificate) error {
	now := time.Now()

	// 1. 检查证书有效期
	if now.Before(cert.NotBefore) {
		return fmt.Errorf("%w: certificate not yet valid (valid from %v)", ErrInvalidCert, cert.NotBefore)
	}
	if now.After(cert.NotAfter) {
		return fmt.Errorf("%w: certificate has expired (expired at %v)", ErrInvalidCert, cert.NotAfter)
	}

	// 2. 检查证书是否自签名
	if err := cert.CheckSignatureFrom(cert); err != nil {
		return fmt.Errorf("%w: invalid self-signed certificate: %v", ErrInvalidCert, err)
	}

	// 3. 检查密钥用途（如果指定）
	if cert.KeyUsage != 0 {
		// 必须包含数字签名用途
		if cert.KeyUsage&x509.KeyUsageDigitalSignature == 0 {
			return fmt.Errorf("%w: certificate missing digital signature key usage", ErrInvalidCert)
		}
	}

	// 4. 检查扩展密钥用途（如果指定）
	if len(cert.ExtKeyUsage) > 0 {
		hasClientAuth := false
		hasServerAuth := false
		for _, usage := range cert.ExtKeyUsage {
			if usage == x509.ExtKeyUsageClientAuth {
				hasClientAuth = true
			}
			if usage == x509.ExtKeyUsageServerAuth {
				hasServerAuth = true
			}
		}
		// P2P 节点通常同时作为客户端和服务端
		if !hasClientAuth && !hasServerAuth {
			return fmt.Errorf("%w: certificate missing required extended key usage", ErrInvalidCert)
		}
	}

	// 5. 检查公钥强度（RSA >= 2048 位，ECDSA >= 256 位）
	switch pub := cert.PublicKey.(type) {
	case *rsa.PublicKey:
		if pub.Size()*8 < 2048 {
			return fmt.Errorf("%w: RSA key too weak (%d bits, minimum 2048)", ErrInvalidCert, pub.Size()*8)
		}
	case *ecdsa.PublicKey:
		if pub.Curve.Params().BitSize < 256 {
			return fmt.Errorf("%w: ECDSA key too weak (%d bits, minimum 256)", ErrInvalidCert, pub.Curve.Params().BitSize)
		}
	}

	return nil
}

// Close 关闭认证器
func (a *CertAuthenticator) Close() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.closed {
		return nil
	}

	a.closed = true
	a.certificate = nil
	a.certPool = nil

	return nil
}

// 确保实现接口
var _ interfaces.Authenticator = (*CertAuthenticator)(nil)
