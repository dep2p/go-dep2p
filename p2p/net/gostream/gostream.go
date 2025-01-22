// Package gostream 允许使用 [DeP2P](https://github.com/dep2p) 流替换 Go 中的标准网络栈。
//
// 给定一个 dep2p.Host，gostream 提供 Dial() 和 Listen() 方法，这些方法返回 net.Conn 和 net.Listener 的实现。
//
// 与常规的 "host:port" 寻址不同，`gostream` 使用对等点 ID，并且不使用原始 TCP 连接，而是使用 dep2p 的 net.Stream。
// 这意味着您的连接将利用 DeP2P 的多路由、NAT 穿透和流多路复用功能。
//
// 注意，DeP2P 主机不能拨号到自身，因此无法将同一个 Host 同时用作服务器和客户端。
package gostream

import logging "github.com/dep2p/log"

// Network 是 gostream 连接使用的地址返回的 "net.Addr.Network()" 名称。
// 相应地，"net.Addr.String()" 将是一个对等点 ID。
var Network = "dep2p"

var log = logging.Logger("net-gostream")
