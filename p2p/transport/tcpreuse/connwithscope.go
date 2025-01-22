package tcpreuse

import (
	"fmt"

	"github.com/dep2p/go-dep2p/core/network"
	manet "github.com/dep2p/go-dep2p/multiformats/multiaddr/net"
	"github.com/dep2p/go-dep2p/p2p/transport/tcpreuse/internal/sampledconn"
)

// connWithScope 包装了TCP连接并添加了作用域管理功能的结构体
type connWithScope struct {
	sampledconn.ManetTCPConnInterface                             // TCP连接接口
	scope                             network.ConnManagementScope // 连接作用域管理器
}

// Scope 获取连接的作用域管理器
// 返回值:
//   - network.ConnManagementScope 连接作用域管理器
func (c connWithScope) Scope() network.ConnManagementScope {
	return c.scope
}

// manetConnWithScope 为网络连接添加作用域管理功能
// 参数:
//   - c: manet.Conn 原始网络连接
//   - scope: network.ConnManagementScope 连接作用域管理器
//
// 返回值:
//   - manet.Conn 添加作用域管理后的连接
//   - error 可能的错误
func manetConnWithScope(c manet.Conn, scope network.ConnManagementScope) (manet.Conn, error) {
	// 尝试将连接转换为TCP连接
	if tcpconn, ok := c.(sampledconn.ManetTCPConnInterface); ok {
		// 创建并返回带作用域的连接
		return &connWithScope{tcpconn, scope}, nil
	}

	log.Debugf("传入的连接不是TCP连接")
	return nil, fmt.Errorf("传入的连接不是TCP连接")
}
