// Package sec 提供了 dep2p 的安全连接和传输接口
package sec

import (
	"context"
	"fmt"
	"net"

	"github.com/dep2p/go-dep2p/core/network"
	"github.com/dep2p/go-dep2p/core/peer"
	"github.com/dep2p/go-dep2p/core/protocol"
)

// SecureConn 定义了一个经过认证和加密的连接接口
// 继承了 net.Conn 和 network.ConnSecurity 接口
type SecureConn interface {
	net.Conn
	network.ConnSecurity
}

// SecureTransport 定义了一个安全传输接口
// 用于将未认证的、明文的原生连接转换为经过认证和加密的连接
type SecureTransport interface {
	// SecureInbound 用于安全化入站连接
	// 参数：
	//   - ctx: context.Context 上下文对象，用于控制操作的生命周期
	//   - insecure: net.Conn 未加密的原始连接
	//   - p: peer.ID 对等节点ID，如果为空则接受任何对等节点的连接
	//
	// 返回值：
	//   - SecureConn: 安全的加密连接
	//   - error: 如果发生错误，返回错误信息
	SecureInbound(ctx context.Context, insecure net.Conn, p peer.ID) (SecureConn, error)

	// SecureOutbound 用于安全化出站连接
	// 参数：
	//   - ctx: context.Context 上下文对象，用于控制操作的生命周期
	//   - insecure: net.Conn 未加密的原始连接
	//   - p: peer.ID 目标对等节点ID
	//
	// 返回值：
	//   - SecureConn: 安全的加密连接
	//   - error: 如果发生错误，返回错误信息
	SecureOutbound(ctx context.Context, insecure net.Conn, p peer.ID) (SecureConn, error)

	// ID 返回安全协议的协议ID
	// 返回值：
	//   - protocol.ID: 协议标识符
	ID() protocol.ID
}

// ErrPeerIDMismatch 定义了对等节点ID不匹配的错误结构
type ErrPeerIDMismatch struct {
	// Expected 期望的对等节点ID
	Expected peer.ID
	// Actual 实际的对等节点ID
	Actual peer.ID
}

// Error 实现了 error 接口，返回格式化的错误信息
// 返回值：
//   - string: 格式化的错误信息，包含期望和实际的对等节点ID
func (e ErrPeerIDMismatch) Error() string {
	return fmt.Sprintf("对等节点ID不匹配: 期望 %s, 但远程密钥匹配 %s", e.Expected, e.Actual)
}

// 确保 ErrPeerIDMismatch 类型实现了 error 接口
var _ error = (*ErrPeerIDMismatch)(nil)
