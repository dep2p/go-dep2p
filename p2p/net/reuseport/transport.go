// Package reuseport 提供了一个基础传输层,用于自动(且智能地)复用TCP端口
//
// 使用方法:构造一个新的Transport并配置监听器 tr.Listen(...)
// 当进行拨号(tr.Dial(...))时,传输层会尝试复用当前正在监听的端口,根据目标地址选择最佳端口
//
// 建议将所有连接的SO_LINGER设置为0,否则在重新拨号最近关闭的连接时可能会复用端口失败
// 详见 https://hea-www.harvard.edu/~fine/Tech/addrinuse.html
package reuseport

import (
	"errors"
	"sync"

	logging "github.com/dep2p/log"
)

// log 用于记录reuseport-transport相关的日志
var log = logging.Logger("net-reuseport")

// ErrWrongProto 当拨号使用非TCP协议时返回此错误
var ErrWrongProto = errors.New("只能通过IPv4或IPv6拨号TCP连接")

// Transport 是一个TCP复用传输层,可以复用监听器端口
// 零值可以安全使用
type Transport struct {
	v4 network // IPv4网络
	v6 network // IPv6网络
}

// network 表示一个网络实例
type network struct {
	mu        sync.RWMutex           // 用于保护listeners和dialer的互斥锁
	listeners map[*listener]struct{} // 当前活跃的监听器集合
	dialer    *dialer                // 用于创建出站连接的拨号器
}
