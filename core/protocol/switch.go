// Package protocol 提供了 dep2p 中用于协议路由和协商的核心接口。
package protocol

import (
	"io"

	"github.com/dep2p/go-dep2p/multiformats/multistream"
)

// HandlerFunc 是一个用户提供的函数，由 Router 用来处理协议/流
//
// 参数：
//   - protocol: ID 协议标识符，可能与注册时使用的ID不同(如果处理程序是使用匹配函数注册的)
//   - stream: io.ReadWriteCloser 需要处理的数据流
//
// 返回值：
//   - error: 如果处理过程中发生错误，返回错误信息
type HandlerFunc = multistream.HandlerFunc[ID]

// Router 是一个接口，允许用户添加和删除协议处理程序
// 当接受到已注册协议的传入流请求时，这些处理程序将被调用
//
// 在接收到传入流请求时，Router 将检查所有已注册的协议处理程序，以确定哪个(如果有)能够处理该流
// 处理程序按注册顺序检查；如果多个处理程序符合条件，则只调用第一个注册的处理程序
type Router interface {
	// AddHandler 为给定的协议ID字符串注册处理程序，用于精确的字面量匹配
	//
	// 参数：
	//   - protocol: ID 要注册的协议标识符
	//   - handler: HandlerFunc 处理该协议的函数
	AddHandler(protocol ID, handler HandlerFunc)

	// AddHandlerWithFunc 注册一个处理程序，当提供的匹配函数返回 true 时将被调用
	//
	// 参数：
	//   - protocol: ID 协议标识符(注意：此参数不用于匹配)
	//   - match: func(ID) bool 用于判断是否支持协议的匹配函数
	//   - handler: HandlerFunc 处理该协议的函数
	AddHandlerWithFunc(protocol ID, match func(ID) bool, handler HandlerFunc)

	// RemoveHandler 移除指定协议ID的已注册处理程序(如果存在)
	//
	// 参数：
	//   - protocol: ID 要移除的协议标识符
	RemoveHandler(protocol ID)

	// Protocols 返回所有已注册的协议ID字符串列表
	// 注意：如果使用 AddHandlerWithFunc 添加了带匹配函数的处理程序，Router 可能能够处理此列表中未包含的协议ID
	//
	// 返回值：
	//   - []ID: 已注册的协议ID列表
	Protocols() []ID
}

// Negotiator 是一个能够就入站通信流使用什么协议达成一致的组件
type Negotiator interface {
	// Negotiate 返回用于给定入站流的已注册协议处理程序
	// 在确定协议并且 Negotiator 完成使用流进行协商后返回
	//
	// 参数：
	//   - rwc: io.ReadWriteCloser 需要协商的数据流
	//
	// 返回值：
	//   - ID: 协商确定的协议标识符
	//   - HandlerFunc: 处理该协议的函数
	//   - error: 如果协商失败，返回错误信息
	Negotiate(rwc io.ReadWriteCloser) (ID, HandlerFunc, error)

	// Handle 调用 Negotiate 确定用于入站流的协议处理程序
	// 然后调用协议处理函数，将协议ID和流传递给它
	//
	// 参数：
	//   - rwc: io.ReadWriteCloser 需要处理的数据流
	//
	// 返回值：
	//   - error: 如果协商失败，返回错误信息
	Handle(rwc io.ReadWriteCloser) error
}

// Switch 是负责将传入流请求"分发"到其对应流处理程序的组件
// 它同时实现了 Negotiator 和 Router 接口
type Switch interface {
	Router
	Negotiator
}
