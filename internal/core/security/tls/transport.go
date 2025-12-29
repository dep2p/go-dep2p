// Package tls 提供基于 TLS 的安全传输实现
package tls

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/dep2p/go-dep2p/internal/util/logger"
	identityif "github.com/dep2p/go-dep2p/pkg/interfaces/identity"
	securityif "github.com/dep2p/go-dep2p/pkg/interfaces/security"
	transportif "github.com/dep2p/go-dep2p/pkg/interfaces/transport"
	"github.com/dep2p/go-dep2p/pkg/types"
)

var log = logger.Logger("security.tls")

// Transport TLS 安全传输
// 将普通连接升级为 TLS 加密连接
type Transport struct {
	identity       identityif.Identity
	config         securityif.Config
	configProvider *TLSConfigProvider
	accessCtrl     *AccessController
	accessMu       sync.RWMutex // 保护 accessCtrl 的并发访问

	// 缓存的 TLS 配置
	serverConfig *tls.Config
	clientCert   *tls.Certificate
}

// 确保实现 securityif.SecureTransport 接口
var _ securityif.SecureTransport = (*Transport)(nil)

// NewTransport 创建 TLS 传输
//
// securityif.Config 的以下字段会生效：
//   - MinVersion: TLS 最低版本
//   - RequireClientAuth: 是否要求客户端认证
//   - InsecureSkipVerify: 是否跳过部分验证（保留公钥绑定检查）
//   - Certificate: 使用提供的证书
//   - CipherSuites: 加密套件（仅 TLS 1.2 及以下）
func NewTransport(identity identityif.Identity, config securityif.Config) (*Transport, error) {
	if identity == nil {
		return nil, fmt.Errorf("identity 不能为空")
	}

	// 使用 securityif.Config 创建配置提供者
	configProvider := NewTLSConfigProviderFromConfig(identity, config)

	// 预生成服务端配置
	serverConfig, err := configProvider.ServerConfig()
	if err != nil {
		return nil, fmt.Errorf("生成服务端 TLS 配置失败: %w", err)
	}

	// 获取或生成客户端证书
	var clientCert *tls.Certificate
	if config.Certificate != nil {
		clientCert = config.Certificate
	} else {
		certManager := NewCertificateManager(identity)
		cert, err := certManager.GenerateCertificateFromIdentity()
		if err != nil {
			return nil, fmt.Errorf("生成客户端证书失败: %w", err)
		}
		clientCert = cert
	}

	log.Info("TLS 传输已创建",
		"minVersion", serverConfig.MinVersion,
		"requireClientAuth", config.RequireClientAuth,
		"insecureSkipVerify", config.InsecureSkipVerify)

	return &Transport{
		identity:       identity,
		config:         config,
		configProvider: configProvider,
		accessCtrl:     NewAccessController(),
		serverConfig:   serverConfig,
		clientCert:     clientCert,
	}, nil
}

// SecureInbound 对入站连接进行安全握手
// 服务端角色
func (t *Transport) SecureInbound(ctx context.Context, conn transportif.Conn) (securityif.SecureConn, error) {
	log.Debug("开始入站 TLS 握手",
		"remoteAddr", conn.RemoteAddr().String())

	// 获取底层网络连接
	netConn := t.getNetConn(conn)
	if netConn == nil {
		return nil, fmt.Errorf("无法获取底层网络连接")
	}

	// 创建 TLS 服务端连接
	tlsConn := tls.Server(netConn, t.serverConfig)

	// 设置握手超时
	if deadline, ok := ctx.Deadline(); ok {
		_ = tlsConn.SetDeadline(deadline)
	} else {
		// 默认 10 秒握手超时
		_ = tlsConn.SetDeadline(time.Now().Add(10 * time.Second))
	}

	// 执行 TLS 握手
	if err := tlsConn.HandshakeContext(ctx); err != nil {
		_ = tlsConn.Close()
		return nil, fmt.Errorf("TLS 握手失败: %w", err)
	}

	// 清除超时设置
	_ = tlsConn.SetDeadline(time.Time{})

	// 提取远程 NodeID（从证书公钥派生，不可伪造）
	tlsState := tlsConn.ConnectionState()
	remoteID, err := ExtractNodeIDFromTLSState(tlsState)
	if err != nil {
		_ = tlsConn.Close()
		return nil, fmt.Errorf("提取远程 NodeID 失败: %w", err)
	}

	// 检查访问控制
	ac := t.AccessController()
	if ac != nil && !ac.AllowInbound(remoteID) {
		_ = tlsConn.Close()
		return nil, fmt.Errorf("入站连接被拒绝: NodeID %s", remoteID.String())
	}

	log.Debug("入站 TLS 握手成功",
		"remoteID", remoteID.String(),
		"remoteAddr", conn.RemoteAddr().String())

	// 创建安全连接
	return NewSecureConn(
		tlsConn,
		conn,
		t.identity.ID(),
		t.identity.PublicKey(),
	)
}

// SecureOutbound 对出站连接进行安全握手
// 客户端角色
func (t *Transport) SecureOutbound(ctx context.Context, conn transportif.Conn, remotePeer types.NodeID) (securityif.SecureConn, error) {
	log.Debug("开始出站 TLS 握手",
		"remoteAddr", conn.RemoteAddr().String(),
		"expectedPeerID", remotePeer.String())

	// 检查访问控制
	ac := t.AccessController()
	if ac != nil && !ac.AllowOutbound(remotePeer) {
		return nil, fmt.Errorf("出站连接被拒绝: NodeID %s", remotePeer.String())
	}

	// 获取底层网络连接
	netConn := t.getNetConn(conn)
	if netConn == nil {
		return nil, fmt.Errorf("无法获取底层网络连接")
	}

	// 创建客户端 TLS 配置（包含 expectedPeerID 校验）
	clientConfig, err := t.configProvider.ClientConfig(remotePeer)
	if err != nil {
		return nil, fmt.Errorf("创建客户端 TLS 配置失败: %w", err)
	}

	// 创建 TLS 客户端连接
	tlsConn := tls.Client(netConn, clientConfig)

	// 设置握手超时
	if deadline, ok := ctx.Deadline(); ok {
		_ = tlsConn.SetDeadline(deadline)
	} else {
		// 默认 10 秒握手超时
		_ = tlsConn.SetDeadline(time.Now().Add(10 * time.Second))
	}

	// 执行 TLS 握手
	if err := tlsConn.HandshakeContext(ctx); err != nil {
		_ = tlsConn.Close()
		return nil, fmt.Errorf("TLS 握手失败: %w", err)
	}

	// 清除超时设置
	_ = tlsConn.SetDeadline(time.Time{})

	// 提取远程 NodeID（从证书公钥派生，不可伪造）
	tlsState := tlsConn.ConnectionState()
	actualRemoteID, err := ExtractNodeIDFromTLSState(tlsState)
	if err != nil {
		_ = tlsConn.Close()
		return nil, fmt.Errorf("提取远程 NodeID 失败: %w", err)
	}

	// 如果提供了期望的 NodeID，再次验证是否匹配（双重检查）
	// 注意：VerifyPeerCertificate 回调已经校验过一次
	if !remotePeer.IsEmpty() && !actualRemoteID.Equal(remotePeer) {
		_ = tlsConn.Close()
		return nil, fmt.Errorf("NodeID 不匹配: 期望 %s, 实际 %s",
			remotePeer.String(), actualRemoteID.String())
	}

	log.Debug("出站 TLS 握手成功",
		"remoteID", actualRemoteID.String(),
		"remoteAddr", conn.RemoteAddr().String())

	// 创建安全连接
	return NewSecureConn(
		tlsConn,
		conn,
		t.identity.ID(),
		t.identity.PublicKey(),
	)
}

// Protocol 返回安全协议名称
func (t *Transport) Protocol() string {
	return "tls"
}

// getNetConn 获取底层网络连接
func (t *Transport) getNetConn(conn transportif.Conn) net.Conn {
	// 如果连接实现了 NetConn 接口，直接获取
	if nc, ok := conn.(interface{ NetConn() net.Conn }); ok {
		return nc.NetConn()
	}

	// 使用适配器包装
	return &connAdapter{conn: conn}
}

// SetAccessController 设置访问控制器（线程安全）
func (t *Transport) SetAccessController(ac *AccessController) {
	t.accessMu.Lock()
	defer t.accessMu.Unlock()
	t.accessCtrl = ac
}

// AccessController 返回访问控制器（线程安全）
func (t *Transport) AccessController() *AccessController {
	t.accessMu.RLock()
	defer t.accessMu.RUnlock()
	return t.accessCtrl
}

// ============================================================================
//                              connAdapter 适配器
// ============================================================================

// connAdapter 将 transportif.Conn 适配为 net.Conn
type connAdapter struct {
	conn transportif.Conn
}

// 确保实现 net.Conn 接口
var _ net.Conn = (*connAdapter)(nil)

// Read 读取数据
func (a *connAdapter) Read(b []byte) (int, error) {
	return a.conn.Read(b)
}

// Write 写入数据
func (a *connAdapter) Write(b []byte) (int, error) {
	return a.conn.Write(b)
}

// Close 关闭连接
func (a *connAdapter) Close() error {
	return a.conn.Close()
}

// LocalAddr 返回本地地址
func (a *connAdapter) LocalAddr() net.Addr {
	return a.conn.LocalNetAddr()
}

// RemoteAddr 返回远程地址
func (a *connAdapter) RemoteAddr() net.Addr {
	return a.conn.RemoteNetAddr()
}

// SetDeadline 设置超时
func (a *connAdapter) SetDeadline(t time.Time) error {
	return a.conn.SetDeadline(t)
}

// SetReadDeadline 设置读超时
func (a *connAdapter) SetReadDeadline(t time.Time) error {
	return a.conn.SetReadDeadline(t)
}

// SetWriteDeadline 设置写超时
func (a *connAdapter) SetWriteDeadline(t time.Time) error {
	return a.conn.SetWriteDeadline(t)
}
