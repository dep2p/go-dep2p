package dep2pquic

import (
	"context"
	"fmt"
	"net"

	ic "github.com/dep2p/go-dep2p/core/crypto"
	"github.com/dep2p/go-dep2p/core/network"
	"github.com/dep2p/go-dep2p/core/peer"
	tpt "github.com/dep2p/go-dep2p/core/transport"
	p2ptls "github.com/dep2p/go-dep2p/p2p/security/tls"
	"github.com/dep2p/go-dep2p/p2p/transport/quicreuse"

	ma "github.com/dep2p/go-dep2p/multiformats/multiaddr"
	"github.com/quic-go/quic-go"
)

// listener 监听 QUIC 连接
type listener struct {
	reuseListener   quicreuse.Listener            // QUIC 连接复用监听器
	transport       *transport                    // 传输层实例
	rcmgr           network.ResourceManager       // 资源管理器
	privKey         ic.PrivKey                    // 本地私钥
	localPeer       peer.ID                       // 本地节点 ID
	localMultiaddrs map[quic.Version]ma.Multiaddr // 本地多地址映射,按 QUIC 版本索引
}

// newListener 创建一个新的 QUIC 监听器
// 参数:
//   - ln: quicreuse.Listener QUIC 复用监听器
//   - t: *transport 传输层实例
//   - localPeer: peer.ID 本地节点 ID
//   - key: ic.PrivKey 本地私钥
//   - rcmgr: network.ResourceManager 资源管理器
//
// 返回值:
//   - listener 创建的监听器实例
//   - error 错误信息
func newListener(ln quicreuse.Listener, t *transport, localPeer peer.ID, key ic.PrivKey, rcmgr network.ResourceManager) (listener, error) {
	// 初始化本地多地址映射
	localMultiaddrs := make(map[quic.Version]ma.Multiaddr)
	// 遍历所有监听地址
	for _, addr := range ln.Multiaddrs() {
		// 检查是否支持 QUIC v1 协议
		if _, err := addr.ValueForProtocol(ma.P_QUIC_V1); err == nil {
			localMultiaddrs[quic.Version1] = addr
		}
	}

	// 返回监听器实例
	return listener{
		reuseListener:   ln,
		transport:       t,
		rcmgr:           rcmgr,
		privKey:         key,
		localPeer:       localPeer,
		localMultiaddrs: localMultiaddrs,
	}, nil
}

// Accept 接受新的连接
// 返回值:
//   - tpt.CapableConn 新建立的连接
//   - error 错误信息
func (l *listener) Accept() (tpt.CapableConn, error) {
	for {
		// 接受新的 QUIC 连接
		qconn, err := l.reuseListener.Accept(context.Background())
		if err != nil {
			log.Debugf("接受新连接时出错: %s", err)
			return nil, err
		}
		// 包装 QUIC 连接
		c, err := l.wrapConn(qconn)
		if err != nil {
			log.Debugf("设置连接失败: %s", err)
			qconn.CloseWithError(1, "")
			continue
		}
		// 添加连接到传输层
		l.transport.addConn(qconn, c)
		// 检查连接是否被网关拦截
		if l.transport.gater != nil && !(l.transport.gater.InterceptAccept(c) && l.transport.gater.InterceptSecured(network.DirInbound, c.remotePeerID, c)) {
			c.closeWithError(errorCodeConnectionGating, "连接被网关拦截")
			continue
		}

		// 处理打洞连接
		key := holePunchKey{addr: qconn.RemoteAddr().String(), peer: c.remotePeerID}
		var wasHolePunch bool
		l.transport.holePunchingMx.Lock()
		holePunch, ok := l.transport.holePunching[key]
		if ok && !holePunch.fulfilled {
			holePunch.connCh <- c
			wasHolePunch = true
			holePunch.fulfilled = true
		}
		l.transport.holePunchingMx.Unlock()
		if wasHolePunch {
			continue
		}
		return c, nil
	}
}

// wrapConn 将 QUIC 连接包装为 dep2p 的 CapableConn
// 如果包装失败,调用者负责清理连接
// 参数:
//   - qconn: quic.Connection QUIC 连接实例
//
// 返回值:
//   - *conn 包装后的连接
//   - error 错误信息
func (l *listener) wrapConn(qconn quic.Connection) (*conn, error) {
	// 转换远程地址为多地址格式
	remoteMultiaddr, err := quicreuse.ToQuicMultiaddr(qconn.RemoteAddr(), qconn.ConnectionState().Version)
	if err != nil {
		log.Debugf("转换远程地址为多地址格式时出错: %s", err)
		return nil, err
	}

	// 打开入站连接资源
	connScope, err := l.rcmgr.OpenConnection(network.DirInbound, false, remoteMultiaddr)
	if err != nil {
		log.Debugf("资源管理器阻止了入站连接", "addr", qconn.RemoteAddr(), "error", err)
		return nil, err
	}
	// 使用资源范围包装连接
	c, err := l.wrapConnWithScope(qconn, connScope, remoteMultiaddr)
	if err != nil {
		connScope.Done()
		return nil, err
	}

	return c, nil
}

// wrapConnWithScope 使用资源范围包装 QUIC 连接
// 参数:
//   - qconn: quic.Connection QUIC 连接实例
//   - connScope: network.ConnManagementScope 连接资源管理范围
//   - remoteMultiaddr: ma.Multiaddr 远程多地址
//
// 返回值:
//   - *conn 包装后的连接
//   - error 错误信息
func (l *listener) wrapConnWithScope(qconn quic.Connection, connScope network.ConnManagementScope, remoteMultiaddr ma.Multiaddr) (*conn, error) {
	// 从 TLS 证书链中提取远程公钥
	remotePubKey, err := p2ptls.PubKeyFromCertChain(qconn.ConnectionState().TLS.PeerCertificates)
	if err != nil {
		log.Debugf("从TLS证书链中提取远程公钥时出错: %s", err)
		return nil, err
	}
	// 从公钥生成节点 ID
	remotePeerID, err := peer.IDFromPublicKey(remotePubKey)
	if err != nil {
		log.Debugf("从公钥生成节点ID时出错: %s", err)
		return nil, err
	}
	// 设置连接的对等节点
	if err := connScope.SetPeer(remotePeerID); err != nil {
		log.Debugf("资源管理器阻止了与对等节点的入站连接", "peer", remotePeerID, "addr", qconn.RemoteAddr(), "error", err)
		return nil, err
	}

	// 获取对应 QUIC 版本的本地多地址
	localMultiaddr, found := l.localMultiaddrs[qconn.ConnectionState().Version]
	if !found {
		log.Debugf("未知的 QUIC 版本: %s", qconn.ConnectionState().Version.String())
		return nil, fmt.Errorf("未知的 QUIC 版本: %s", qconn.ConnectionState().Version.String())
	}

	// 返回包装后的连接
	return &conn{
		quicConn:        qconn,
		transport:       l.transport,
		scope:           connScope,
		localPeer:       l.localPeer,
		localMultiaddr:  localMultiaddr,
		remoteMultiaddr: remoteMultiaddr,
		remotePeerID:    remotePeerID,
		remotePubKey:    remotePubKey,
	}, nil
}

// Close 关闭监听器
// 返回值:
//   - error 错误信息
func (l *listener) Close() error {
	return l.reuseListener.Close()
}

// Addr 返回监听器的地址
// 返回值:
//   - net.Addr 监听地址
func (l *listener) Addr() net.Addr {
	return l.reuseListener.Addr()
}
