package network

import (
	ma "github.com/dep2p/go-dep2p/multiformats/multiaddr"
)

// Notifiee 是一个接口，用于接收来自 Network 的通知。
// 它定义了一组回调方法，在网络事件发生时被调用。
type Notifiee interface {
	// Listen 当网络开始监听一个地址时调用
	//
	// 参数：
	//   - n：Network 触发事件的网络实例
	//   - addr：ma.Multiaddr 开始监听的地址
	Listen(Network, ma.Multiaddr)

	// ListenClose 当网络停止监听一个地址时调用
	//
	// 参数：
	//   - n：Network 触发事件的网络实例
	//   - addr：ma.Multiaddr 停止监听的地址
	ListenClose(Network, ma.Multiaddr)

	// Connected 当连接建立时调用
	//
	// 参数：
	//   - n：Network 触发事件的网络实例
	//   - c：Conn 建立的连接
	Connected(Network, Conn)

	// Disconnected 当连接关闭时调用
	//
	// 参数：
	//   - n：Network 触发事件的网络实例
	//   - c：Conn 关闭的连接
	Disconnected(Network, Conn)
}

// NotifyBundle 通过调用设置在其上的任何函数来实现 Notifiee 接口。
// 如果函数未设置则不执行任何操作。这是注册通知的简单方式。
type NotifyBundle struct {
	// ListenF 是监听事件的回调函数
	ListenF func(Network, ma.Multiaddr)
	// ListenCloseF 是停止监听事件的回调函数
	ListenCloseF func(Network, ma.Multiaddr)
	// ConnectedF 是连接建立事件的回调函数
	ConnectedF func(Network, Conn)
	// DisconnectedF 是连接关闭事件的回调函数
	DisconnectedF func(Network, Conn)
}

// 确保 NotifyBundle 实现了 Notifiee 接口
var _ Notifiee = (*NotifyBundle)(nil)

// Listen 如果 ListenF 不为空则调用它
//
// 参数：
//   - n：Network 触发事件的网络实例
//   - a：ma.Multiaddr 监听的地址
func (nb *NotifyBundle) Listen(n Network, a ma.Multiaddr) {
	if nb.ListenF != nil {
		nb.ListenF(n, a)
	}
}

// ListenClose 如果 ListenCloseF 不为空则调用它
//
// 参数：
//   - n：Network 触发事件的网络实例
//   - a：ma.Multiaddr 停止监听的地址
func (nb *NotifyBundle) ListenClose(n Network, a ma.Multiaddr) {
	if nb.ListenCloseF != nil {
		nb.ListenCloseF(n, a)
	}
}

// Connected 如果 ConnectedF 不为空则调用它
//
// 参数：
//   - n：Network 触发事件的网络实例
//   - c：Conn 建立的连接
func (nb *NotifyBundle) Connected(n Network, c Conn) {
	if nb.ConnectedF != nil {
		nb.ConnectedF(n, c)
	}
}

// Disconnected 如果 DisconnectedF 不为空则调用它
//
// 参数：
//   - n：Network 触发事件的网络实例
//   - c：Conn 关闭的连接
func (nb *NotifyBundle) Disconnected(n Network, c Conn) {
	if nb.DisconnectedF != nil {
		nb.DisconnectedF(n, c)
	}
}

// GlobalNoopNotifiee 是一个全局的空操作通知接收者。请勿修改。
var GlobalNoopNotifiee = &NoopNotifiee{}

// NoopNotifiee 实现了一个不执行任何操作的 Notifiee 接口
type NoopNotifiee struct{}

// 确保 NoopNotifiee 实现了 Notifiee 接口
var _ Notifiee = (*NoopNotifiee)(nil)

// Connected 实现了 Notifiee 接口，但不执行任何操作
func (nn *NoopNotifiee) Connected(n Network, c Conn) {}

// Disconnected 实现了 Notifiee 接口，但不执行任何操作
func (nn *NoopNotifiee) Disconnected(n Network, c Conn) {}

// Listen 实现了 Notifiee 接口，但不执行任何操作
func (nn *NoopNotifiee) Listen(n Network, addr ma.Multiaddr) {}

// ListenClose 实现了 Notifiee 接口，但不执行任何操作
func (nn *NoopNotifiee) ListenClose(n Network, addr ma.Multiaddr) {}
