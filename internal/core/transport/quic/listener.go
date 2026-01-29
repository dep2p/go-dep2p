// Package quic 实现 QUIC 传输
package quic

import (
	"context"
	"fmt"
	"net"

	tlssec "github.com/dep2p/go-dep2p/internal/core/security/tls"
	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/types"
	"github.com/quic-go/quic-go"
)

// 确保实现了接口
var _ pkgif.Listener = (*Listener)(nil)

// Listener QUIC 监听器
type Listener struct {
	quicListener *quic.Listener
	localAddr    types.Multiaddr
	localPeer    types.PeerID
	transport    *Transport
}

// Accept 接受新连接
func (l *Listener) Accept() (pkgif.Connection, error) {
	quicConn, err := l.quicListener.Accept(context.Background())
	if err != nil {
		return nil, err
	}

	// 提取远程地址
	remoteUDPAddr := quicConn.RemoteAddr().(*net.UDPAddr)
	remoteAddrStr := fmt.Sprintf("/ip4/%s/udp/%d/quic-v1", remoteUDPAddr.IP.String(), remoteUDPAddr.Port)
	remoteAddr, err := types.NewMultiaddr(remoteAddrStr)
	if err != nil {
		quicConn.CloseWithError(1, "invalid remote address")
		return nil, err
	}

	// 从 TLS 连接状态提取远程 PeerID
	remotePeer, err := extractPeerIDFromConn(quicConn)
	if err != nil {
		quicConn.CloseWithError(1, "failed to extract peer ID")
		return nil, err
	}

	return newConnection(quicConn, l.localPeer, remotePeer, remoteAddr, pkgif.DirInbound), nil
}

// Close 关闭监听器
func (l *Listener) Close() error {
	return l.quicListener.Close()
}

// Addr 返回监听地址
func (l *Listener) Addr() types.Multiaddr {
	// 获取实际监听的地址（可能与请求的不同，如端口为 0）
	if l.quicListener == nil {
		return nil
	}
	rawAddr := l.quicListener.Addr()
	if rawAddr == nil {
		return nil
	}
	actualAddr, ok := rawAddr.(*net.UDPAddr)
	if !ok {
		return nil
	}
	
	// 根据 IP 类型选择协议
	var actualAddrStr string
	ip := actualAddr.IP
	if ip4 := ip.To4(); ip4 != nil {
		// IPv4 地址
		actualAddrStr = fmt.Sprintf("/ip4/%s/udp/%d/quic-v1", ip4.String(), actualAddr.Port)
	} else if ip.IsUnspecified() {
		// 未指定地址 (0.0.0.0 或 ::)，使用 0.0.0.0
		actualAddrStr = fmt.Sprintf("/ip4/0.0.0.0/udp/%d/quic-v1", actualAddr.Port)
	} else {
		// IPv6 地址
		actualAddrStr = fmt.Sprintf("/ip6/%s/udp/%d/quic-v1", ip.String(), actualAddr.Port)
	}
	
	addr, err := types.NewMultiaddr(actualAddrStr)
	if err != nil {
		return nil
	}
	return addr
}

// Multiaddr 返回多地址格式
func (l *Listener) Multiaddr() types.Multiaddr {
	return l.Addr()
}

// extractPeerIDFromConn 从 QUIC 连接提取 PeerID
func extractPeerIDFromConn(conn *quic.Conn) (types.PeerID, error) {
	// 获取 TLS 连接状态
	connState := conn.ConnectionState().TLS
	
	// 检查是否有对端证书
	if len(connState.PeerCertificates) == 0 {
		return "", fmt.Errorf("no peer certificates")
	}
	
	// 获取第一个证书
	cert := connState.PeerCertificates[0]
	
	// 从证书中提取 PeerID
	peerID, err := tlssec.ExtractPeerIDFromCert(cert)
	if err != nil {
		return "", fmt.Errorf("extract peer id from cert: %w", err)
	}
	
	return types.PeerID(peerID), nil
}
