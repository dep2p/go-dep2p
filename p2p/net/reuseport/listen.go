package reuseport

import (
	"net"

	ma "github.com/dep2p/go-dep2p/multiformats/multiaddr"
	manet "github.com/dep2p/go-dep2p/multiformats/multiaddr/net"
	"github.com/dep2p/go-dep2p/p2plib/reuseport"
)

// listener 结构体封装了一个 manet.Listener 和对应的网络对象
type listener struct {
	manet.Listener          // 底层的 multiaddr 网络监听器
	network        *network // 关联的网络对象
}

// Close 关闭监听器并清理相关资源
// 返回值:
//   - error 关闭过程中的错误,如果成功则返回 nil
func (l *listener) Close() error {
	// 加锁保护共享资源
	l.network.mu.Lock()
	// 从网络的监听器映射中删除自己
	delete(l.network.listeners, l)
	// 清空网络的拨号器
	l.network.dialer = nil
	// 解锁
	l.network.mu.Unlock()
	// 关闭底层监听器
	return l.Listener.Close()
}

// Listen 在给定的多地址上创建监听器
// 参数:
//   - laddr: ma.Multiaddr 要监听的多地址
//
// 返回值:
//   - manet.Listener 创建的监听器
//   - error 创建过程中的错误,如果成功则返回 nil
//
// 注意:
//   - 如果支持端口重用,将为此监听器启用它,此传输的未来拨号可能会重用该端口
//   - 你可以在同一个多地址上多次监听(但只有一个监听器最终会处理入站连接)
func (t *Transport) Listen(laddr ma.Multiaddr) (manet.Listener, error) {
	// 解析多地址获取网络类型和地址字符串
	nw, naddr, err := manet.DialArgs(laddr)
	if err != nil {
		log.Debugf("解析多地址失败: %v", err)
		return nil, err
	}

	// 根据网络类型选择对应的网络对象
	var n *network
	switch nw {
	case "tcp4":
		n = &t.v4
	case "tcp6":
		n = &t.v6
	default:
		log.Debugf("不支持的网络类型: %s", nw)
		return nil, ErrWrongProto // 返回协议错误
	}

	// 如果不支持端口重用,使用普通监听
	if !reuseport.Available() {
		return manet.Listen(laddr)
	}

	// 创建支持端口重用的监听器
	nl, err := reuseport.Listen(nw, naddr)
	if err != nil {
		log.Debugf("创建支持端口重用的监听器失败: %v", err)
		return manet.Listen(laddr)
	}

	// 验证地址类型是否为 TCP
	if _, ok := nl.Addr().(*net.TCPAddr); !ok {
		nl.Close()
		log.Debugf("地址类型不是TCP: %v", nl.Addr())
		return nil, ErrWrongProto
	}

	// 将网络监听器包装为 multiaddr 监听器
	malist, err := manet.WrapNetListener(nl)
	if err != nil {
		nl.Close()
		log.Debugf("包装网络监听器失败: %v", err)
		return nil, err
	}

	// 创建监听器对象
	list := &listener{
		Listener: malist,
		network:  n,
	}

	// 加锁保护共享资源
	n.mu.Lock()
	defer n.mu.Unlock()

	// 初始化监听器映射
	if n.listeners == nil {
		n.listeners = make(map[*listener]struct{})
	}
	// 将监听器添加到映射中
	n.listeners[list] = struct{}{}
	// 清空拨号器
	n.dialer = nil

	return list, nil
}
