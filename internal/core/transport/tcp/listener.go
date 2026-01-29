// Package tcp 实现 TCP 传输
package tcp

import (
	"context"
	"fmt"
	"net"

	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// 确保实现了接口
var _ pkgif.Listener = (*Listener)(nil)

// Listener TCP 监听器
type Listener struct {
	tcpListener net.Listener
	localAddr   types.Multiaddr
	localPeer   types.PeerID
	transport   *Transport
}

// Accept 接受新连接
func (l *Listener) Accept() (pkgif.Connection, error) {
	rawConn, err := l.tcpListener.Accept()
	if err != nil {
		return nil, err
	}

	// 提取远程地址
	remoteTCPAddr := rawConn.RemoteAddr().(*net.TCPAddr)
	remoteAddrStr := fmt.Sprintf("/ip4/%s/tcp/%d", remoteTCPAddr.IP.String(), remoteTCPAddr.Port)
	remoteAddr, err := types.NewMultiaddr(remoteAddrStr)
	if err != nil {
		rawConn.Close()
		return nil, err
	}

	// 如果有 Upgrader，进行连接升级（Security + Muxer）
	// Upgrader 会通过安全握手获取远程 PeerID
	if l.transport.upgrader != nil {
		ctx := context.Background() // 可以考虑加入超时
		
		// 入站升级：remotePeer 为空，由握手后确定
		upgradedConn, err := l.transport.upgrader.Upgrade(ctx, rawConn, pkgif.DirInbound, "")
		if err != nil {
			rawConn.Close()
			return nil, fmt.Errorf("upgrade connection: %w", err)
		}
		
		// 包装为 Connection
		return wrapUpgradedConn(upgradedConn, l.localPeer, remoteAddr), nil
	}

	// 如果没有 Upgrader，返回原始 TCP 连接（不推荐，仅用于测试）
	// 使用临时 PeerID（因为没有握手）
	remotePeer := types.PeerID("temp_" + remoteTCPAddr.String())
	return newConnection(rawConn, l.localPeer, remotePeer, remoteAddr, pkgif.DirInbound), nil
}

// Close 关闭监听器
func (l *Listener) Close() error {
	return l.tcpListener.Close()
}

// Addr 返回监听地址
func (l *Listener) Addr() types.Multiaddr {
	// 获取实际监听的地址
	actualAddr := l.tcpListener.Addr().(*net.TCPAddr)
	actualAddrStr := fmt.Sprintf("/ip4/%s/tcp/%d", actualAddr.IP.String(), actualAddr.Port)
	addr, _ := types.NewMultiaddr(actualAddrStr)
	return addr
}

// Multiaddr 返回多地址格式
func (l *Listener) Multiaddr() types.Multiaddr {
	return l.Addr()
}
