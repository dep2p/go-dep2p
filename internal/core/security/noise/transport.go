// Package noise 实现 Noise 协议安全传输
package noise

import (
	"context"
	"fmt"
	"net"

	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/types"
	"github.com/dep2p/go-dep2p/pkg/lib/log"
)

var logger = log.Logger("core/security/noise")

// Transport Noise 协议传输
type Transport struct {
	identity        pkgif.Identity
	identityBinding *IdentityBinding // 可选的身份绑定验证器
}

// New 创建 Noise 传输
func New(identity pkgif.Identity) (*Transport, error) {
	if identity == nil {
		return nil, fmt.Errorf("identity is nil")
	}

	return &Transport{
		identity: identity,
	}, nil
}

// SetIdentityBinding 设置身份绑定验证器
//
// 设置后，握手完成时会验证远程节点的身份绑定
func (t *Transport) SetIdentityBinding(ib *IdentityBinding) {
	t.identityBinding = ib
}

// GetIdentityBinding 获取身份绑定验证器
func (t *Transport) GetIdentityBinding() *IdentityBinding {
	return t.identityBinding
}

// ID 返回协议标识
func (t *Transport) ID() types.ProtocolID {
	return types.ProtocolID("/noise/1.0.0")
}

// SecureInbound 保护入站连接
func (t *Transport) SecureInbound(_ context.Context, conn net.Conn, remotePeer types.PeerID) (pkgif.SecureConn, error) {
	// 参数验证
	if conn == nil {
		return nil, fmt.Errorf("conn is nil")
	}
	
	remotePeerLabel := string(remotePeer)
	if len(remotePeerLabel) > 8 {
		remotePeerLabel = remotePeerLabel[:8]
	}
	logger.Debug("Noise 入站握手", "remotePeer", remotePeerLabel)
	
	// 获取私钥
	privKey := t.identity.PrivateKey()
	
	// 执行 Noise XX 握手（服务器端）
	secConn, err := performHandshake(conn, privKey, remotePeer, false)
	if err != nil {
		logger.Warn("Noise 握手失败", "remotePeer", remotePeerLabel, "error", err)
		return nil, fmt.Errorf("handshake failed: %w", err)
	}
	
	// 可选：使用 IdentityBinding 进行额外的身份验证
	if t.identityBinding != nil {
		if err := t.verifyRemoteIdentity(secConn); err != nil {
			logger.Warn("身份绑定验证失败", "remotePeer", remotePeerLabel, "error", err)
			conn.Close()
			return nil, fmt.Errorf("identity binding verification failed: %w", err)
		}
		logger.Debug("身份绑定验证成功", "remotePeer", remotePeerLabel)
	}
	
	logger.Debug("Noise 握手成功", "remotePeer", remotePeerLabel)
	return secConn, nil
}

// SecureOutbound 保护出站连接
func (t *Transport) SecureOutbound(_ context.Context, conn net.Conn, remotePeer types.PeerID) (pkgif.SecureConn, error) {
	// 参数验证
	if conn == nil {
		return nil, fmt.Errorf("conn is nil")
	}
	
	remotePeerLabel := string(remotePeer)
	if len(remotePeerLabel) > 8 {
		remotePeerLabel = remotePeerLabel[:8]
	}
	logger.Debug("Noise 出站握手", "remotePeer", remotePeerLabel)
	
	// 获取私钥
	privKey := t.identity.PrivateKey()
	
	// 执行 Noise XX 握手（客户端）
	secConn, err := performHandshake(conn, privKey, remotePeer, true)
	if err != nil {
		logger.Warn("Noise 握手失败", "remotePeer", remotePeerLabel, "error", err)
		return nil, fmt.Errorf("handshake failed: %w", err)
	}
	
	// 可选：使用 IdentityBinding 进行额外的身份验证
	if t.identityBinding != nil {
		if err := t.verifyRemoteIdentity(secConn); err != nil {
			logger.Warn("身份绑定验证失败", "remotePeer", remotePeerLabel, "error", err)
			conn.Close()
			return nil, fmt.Errorf("identity binding verification failed: %w", err)
		}
		logger.Debug("身份绑定验证成功", "remotePeer", remotePeerLabel)
	}
	
	logger.Debug("Noise 握手成功", "remotePeer", remotePeerLabel)
	return secConn, nil
}

// verifyRemoteIdentity 验证远程节点身份绑定
func (t *Transport) verifyRemoteIdentity(secConn *secureConn) error {
	if t.identityBinding == nil {
		return nil
	}

	// 获取远程公钥和 PeerID
	remotePubKeyBytes := secConn.RemotePublicKey()
	remotePeerID := secConn.RemotePeer()

	// 如果没有远程公钥，跳过验证（握手已验证 PeerID）
	if len(remotePubKeyBytes) == 0 {
		logger.Debug("无远程公钥，跳过 IdentityBinding 验证")
		return nil
	}

	// 使用 IdentityBinding 验证公钥与 PeerID 的绑定关系
	return t.identityBinding.VerifyBindingFromBytes(remotePubKeyBytes, remotePeerID)
}
