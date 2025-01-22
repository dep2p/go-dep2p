package config

import (
	"crypto/sha256"
	"io"

	"golang.org/x/crypto/hkdf"

	"github.com/dep2p/go-dep2p/core/crypto"

	"github.com/quic-go/quic-go"
)

// QUIC 相关的密钥信息常量
const (
	// statelessResetKeyInfo 用于生成无状态重置密钥的信息字符串
	statelessResetKeyInfo = "dep2p quic stateless reset key"
	// tokenGeneratorKeyInfo 用于生成令牌生成器密钥的信息字符串
	tokenGeneratorKeyInfo = "dep2p quic token generator key"
)

// PrivKeyToStatelessResetKey 将私钥转换为 QUIC 无状态重置密钥
// 参数:
//   - key: dep2p 私钥
//
// 返回:
//   - quic.StatelessResetKey: 生成的无状态重置密钥
//   - error: 转换过程中的错误,如果成功则返回 nil
func PrivKeyToStatelessResetKey(key crypto.PrivKey) (quic.StatelessResetKey, error) {
	// 初始化无状态重置密钥
	var statelessResetKey quic.StatelessResetKey
	// 获取私钥的原始字节
	keyBytes, err := key.Raw()
	if err != nil {
		log.Debugf("获取私钥的原始字节失败: %v", err)
		return statelessResetKey, err
	}
	// 使用 HKDF 从私钥派生无状态重置密钥
	keyReader := hkdf.New(sha256.New, keyBytes, nil, []byte(statelessResetKeyInfo))
	// 读取派生的密钥数据
	if _, err := io.ReadFull(keyReader, statelessResetKey[:]); err != nil {
		log.Debugf("读取派生的密钥数据失败: %v", err)
		return statelessResetKey, err
	}
	return statelessResetKey, nil
}

// PrivKeyToTokenGeneratorKey 将私钥转换为 QUIC 令牌生成器密钥
// 参数:
//   - key: dep2p 私钥
//
// 返回:
//   - quic.TokenGeneratorKey: 生成的令牌生成器密钥
//   - error: 转换过程中的错误,如果成功则返回 nil
func PrivKeyToTokenGeneratorKey(key crypto.PrivKey) (quic.TokenGeneratorKey, error) {
	// 初始化令牌生成器密钥
	var tokenKey quic.TokenGeneratorKey
	// 获取私钥的原始字节
	keyBytes, err := key.Raw()
	if err != nil {
		log.Debugf("获取私钥的原始字节失败: %v", err)
		return tokenKey, err
	}
	// 使用 HKDF 从私钥派生令牌生成器密钥
	keyReader := hkdf.New(sha256.New, keyBytes, nil, []byte(tokenGeneratorKeyInfo))
	// 读取派生的密钥数据
	if _, err := io.ReadFull(keyReader, tokenKey[:]); err != nil {
		log.Debugf("读取派生的密钥数据失败: %v", err)
		return tokenKey, err
	}
	return tokenKey, nil
}
