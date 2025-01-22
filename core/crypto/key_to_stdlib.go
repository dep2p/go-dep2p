package crypto

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rsa"

	"github.com/decred/dcrd/dcrec/secp256k1/v4"
)

// KeyPairFromStdKey 将标准库(和 secp256k1)的私钥包装为 dep2p/core/crypto 密钥
// 参数:
//   - priv: 标准库的私钥
//
// 返回值:
//   - PrivKey: dep2p 私钥
//   - PubKey: dep2p 公钥
//   - error: 错误信息
func KeyPairFromStdKey(priv crypto.PrivateKey) (PrivKey, PubKey, error) {
	// 检查私钥是否为空
	if priv == nil {
		log.Debugf("私钥为空")
		return nil, nil, ErrNilPrivateKey
	}

	// 根据私钥类型进行转换
	switch p := priv.(type) {
	case *rsa.PrivateKey:
		// RSA 密钥转换
		return &RsaPrivateKey{*p}, &RsaPublicKey{k: p.PublicKey}, nil

	case *ecdsa.PrivateKey:
		// ECDSA 密钥转换
		return &ECDSAPrivateKey{p}, &ECDSAPublicKey{&p.PublicKey}, nil

	case *ed25519.PrivateKey:
		// Ed25519 密钥转换
		pubIfc := p.Public()
		pub, _ := pubIfc.(ed25519.PublicKey)
		return &Ed25519PrivateKey{*p}, &Ed25519PublicKey{pub}, nil

	case *secp256k1.PrivateKey:
		// Secp256k1 密钥转换
		sPriv := Secp256k1PrivateKey(*p)
		sPub := Secp256k1PublicKey(*p.PubKey())
		return &sPriv, &sPub, nil

	default:
		return nil, nil, ErrBadKeyType
	}
}

// PrivKeyToStdKey 将 dep2p/go-dep2p/core/crypto 私钥转换为标准库(和 secp256k1)私钥
// 参数:
//   - priv: dep2p 私钥
//
// 返回值:
//   - crypto.PrivateKey: 标准库私钥
//   - error: 错误信息
func PrivKeyToStdKey(priv PrivKey) (crypto.PrivateKey, error) {
	// 检查私钥是否为空
	if priv == nil {
		log.Debugf("私钥为空")
		return nil, ErrNilPrivateKey
	}

	// 根据私钥类型进行转换
	switch p := priv.(type) {
	case *RsaPrivateKey:
		return &p.sk, nil
	case *ECDSAPrivateKey:
		return p.priv, nil
	case *Ed25519PrivateKey:
		return &p.k, nil
	case *Secp256k1PrivateKey:
		return p, nil
	default:
		return nil, ErrBadKeyType
	}
}

// PubKeyToStdKey 将 dep2p/go-dep2p/core/crypto 公钥转换为标准库(和 secp256k1)公钥
// 参数:
//   - pub: dep2p 公钥
//
// 返回值:
//   - crypto.PublicKey: 标准库公钥
//   - error: 错误信息
func PubKeyToStdKey(pub PubKey) (crypto.PublicKey, error) {
	// 检查公钥是否为空
	if pub == nil {
		log.Debugf("公钥为空")
		return nil, ErrNilPublicKey
	}

	// 根据公钥类型进行转换
	switch p := pub.(type) {
	case *RsaPublicKey:
		return &p.k, nil
	case *ECDSAPublicKey:
		return p.pub, nil
	case *Ed25519PublicKey:
		return p.k, nil
	case *Secp256k1PublicKey:
		return p, nil
	default:
		return nil, ErrBadKeyType
	}
}
