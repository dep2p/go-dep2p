// Package upgrader 实现连接升级器
package upgrader

import (
	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// 确保实现了接口
var _ pkgif.UpgradedConn = (*upgradedConn)(nil)

// upgradedConn 升级后的连接
type upgradedConn struct {
	pkgif.MuxedConn // 嵌入多路复用连接

	secConn pkgif.SecureConn // 安全连接（用于访问 PeerID）

	securityProto types.ProtocolID // 使用的安全协议
	muxerID       string            // 使用的多路复用器
	
	// 资源管理
	connScope pkgif.ConnManagementScope // 连接资源范围（可选）
}

// newUpgradedConnWithScope 创建升级后的连接（带资源管理）
func newUpgradedConnWithScope(
	muxedConn pkgif.MuxedConn,
	secConn pkgif.SecureConn,
	securityProto types.ProtocolID,
	muxerID string,
	connScope pkgif.ConnManagementScope,
) *upgradedConn {
	return &upgradedConn{
		MuxedConn:     muxedConn,
		secConn:       secConn,
		securityProto: securityProto,
		muxerID:       muxerID,
		connScope:     connScope,
	}
}

// LocalPeer 返回本地节点 ID
func (c *upgradedConn) LocalPeer() types.PeerID {
	return c.secConn.LocalPeer()
}

// RemotePeer 返回远端节点 ID
func (c *upgradedConn) RemotePeer() types.PeerID {
	return c.secConn.RemotePeer()
}

// Security 返回协商的安全协议
func (c *upgradedConn) Security() types.ProtocolID {
	return c.securityProto
}

// Muxer 返回协商的多路复用器
func (c *upgradedConn) Muxer() string {
	return c.muxerID
}

// Close 关闭连接并释放资源
//
// 关闭顺序：
//  1. 关闭多路复用连接
//  2. 释放资源范围（如果有）
func (c *upgradedConn) Close() error {
	// 先关闭多路复用连接
	err := c.MuxedConn.Close()
	
	// 释放资源范围
	if c.connScope != nil {
		c.connScope.Done()
		c.connScope = nil
	}
	
	return err
}

// Scope 返回连接资源范围
func (c *upgradedConn) Scope() pkgif.ConnManagementScope {
	return c.connScope
}
