// Package auth 实现 Realm 认证
package auth

import "context"

// Authenticator 认证器接口
type Authenticator interface {
	Authenticate(ctx context.Context, peerID string, proof []byte) (bool, error)
	GenerateProof(ctx context.Context) ([]byte, error)
}

// Mode 认证模式
type Mode int

const (
	ModePSK Mode = iota
	ModeCert
	ModeCustom
)
