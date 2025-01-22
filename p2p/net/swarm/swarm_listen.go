package swarm

import (
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/dep2p/go-dep2p/core/canonicallog"
	"github.com/dep2p/go-dep2p/core/network"
	"github.com/dep2p/go-dep2p/core/transport"

	ma "github.com/dep2p/go-dep2p/multiformats/multiaddr"
)

// OrderedListener 接口定义了监听器的设置顺序
type OrderedListener interface {
	// ListenOrder 返回监听器的设置顺序
	// 一些传输层可能会选择性地使用其他已设置的监听器
	// 例如 WebRTC 可能会重用 QUIC 的 UDP 端口,但前提是 QUIC 先设置
	// 返回值越小,设置顺序越靠前
	ListenOrder() int
}

// Listen 为所有给定地址设置监听器
// 只要成功监听至少一个地址就返回
//
// 参数:
//   - addrs: 要监听的多地址列表
//
// 返回值:
//   - error: 如果没有成功监听任何地址则返回错误
func (s *Swarm) Listen(addrs ...ma.Multiaddr) error {
	// 记录每个地址的错误信息
	errs := make([]error, len(addrs))
	// 记录成功监听的地址数量
	var succeeded int

	// 定义地址和监听器的组合结构
	type addrAndListener struct {
		addr ma.Multiaddr
		lTpt transport.Transport
	}
	// 为所有地址创建监听器
	sortedAddrsAndTpts := make([]addrAndListener, 0, len(addrs))
	for _, a := range addrs {
		t := s.TransportForListening(a)
		sortedAddrsAndTpts = append(sortedAddrsAndTpts, addrAndListener{addr: a, lTpt: t})
	}
	// 按照监听器顺序排序
	slices.SortFunc(sortedAddrsAndTpts, func(a, b addrAndListener) int {
		aOrder := 0
		bOrder := 0
		if l, ok := a.lTpt.(OrderedListener); ok {
			aOrder = l.ListenOrder()
		}
		if l, ok := b.lTpt.(OrderedListener); ok {
			bOrder = l.ListenOrder()
		}
		return aOrder - bOrder
	})

	// 为每个地址添加监听器
	for i, a := range sortedAddrsAndTpts {
		if err := s.AddListenAddr(a.addr); err != nil {
			errs[i] = err
		} else {
			succeeded++
		}
	}

	// 记录失败的监听地址
	for i, e := range errs {
		if e != nil {
			log.Warnw("监听失败", "地址", sortedAddrsAndTpts[i].addr, "错误", errs[i])
		}
	}

	// 如果没有成功监听任何地址则返回错误
	if succeeded == 0 && len(sortedAddrsAndTpts) > 0 {
		log.Debugf("未能监听任何地址: %s", errs)
		return fmt.Errorf("未能监听任何地址: %s", errs)
	}

	return nil
}

// ListenClose 停止并删除给定地址的监听器
// 如果一个地址属于某个监听器提供的地址之一,那么该监听器将关闭其提供的所有地址
// 例如,如果关闭一个 `/quic` 地址,那么 QUIC 监听器将关闭并同时关闭任何 `/quic-v1` 地址
//
// 参数:
//   - addrs: 要关闭的多地址列表
func (s *Swarm) ListenClose(addrs ...ma.Multiaddr) {
	// 记录需要关闭的监听器
	listenersToClose := make(map[transport.Listener]struct{}, len(addrs))

	s.listeners.Lock()
	for l := range s.listeners.m {
		if !containsMultiaddr(addrs, l.Multiaddr()) {
			continue
		}

		delete(s.listeners.m, l)
		listenersToClose[l] = struct{}{}
	}
	s.listeners.cacheEOL = time.Time{}
	s.listeners.Unlock()

	// 关闭所有标记的监听器
	for l := range listenersToClose {
		l.Close()
	}
}

// AddListenAddr 告诉 swarm 监听单个地址
// 与 Listen 不同,此方法不会尝试过滤掉错误的地址
//
// 参数:
//   - a: 要监听的多地址
//
// 返回值:
//   - error: 如果监听失败则返回错误
func (s *Swarm) AddListenAddr(a ma.Multiaddr) error {
	tpt := s.TransportForListening(a)
	if tpt == nil {
		// TransportForListening 在以下情况下返回 nil:
		// 1. 没有注册传输层
		// 2. 我们已关闭(因此传输层映射被置空)
		//
		// 区分这两种情况以避免混淆用户
		select {
		case <-s.ctx.Done():
			return ErrSwarmClosed
		default:
			return ErrNoTransport
		}
	}

	// 创建监听器
	list, err := tpt.Listen(a)
	if err != nil {
		log.Debugf("创建监听器失败: %v", err)
		return err
	}

	// 将监听器添加到 swarm 中
	s.listeners.Lock()
	if s.listeners.m == nil {
		s.listeners.Unlock()
		list.Close()
		return ErrSwarmClosed
	}
	s.refs.Add(1)
	s.listeners.m[list] = struct{}{}
	s.listeners.cacheEOL = time.Time{}
	s.listeners.Unlock()

	maddr := list.Multiaddr()

	// 通知所有观察者监听开始
	s.notifyAll(func(n network.Notifiee) {
		n.Listen(s, maddr)
	})

	// 启动监听协程
	go func() {
		defer func() {
			s.listeners.Lock()
			_, ok := s.listeners.m[list]
			if ok {
				delete(s.listeners.m, list)
				s.listeners.cacheEOL = time.Time{}
			}
			s.listeners.Unlock()

			if ok {
				list.Close()
				log.Errorf("swarm 监听器意外关闭")
			}

			// 通知所有观察者监听关闭
			s.notifyAll(func(n network.Notifiee) {
				n.ListenClose(s, maddr)
			})
			s.refs.Done()
		}()
		for {
			// 接受新连接
			c, err := list.Accept()
			if err != nil {
				if !errors.Is(err, transport.ErrListenerClosed) {
					log.Errorf("swarm 监听器 %s 接受连接错误: %s", a, err)
				}
				return
			}
			canonicallog.LogPeerStatus(100, c.RemotePeer(), c.RemoteMultiaddr(), "connection_status", "established", "dir", "inbound")
			if s.metricsTracer != nil {
				c = wrapWithMetrics(c, s.metricsTracer, time.Now(), network.DirInbound)
			}

			log.Debugf("swarm 监听器接受连接: %s <-> %s", c.LocalMultiaddr(), c.RemoteMultiaddr())
			s.refs.Add(1)
			go func() {
				defer s.refs.Done()
				_, err := s.addConn(c, network.DirInbound)
				switch err {
				case nil:
				case ErrSwarmClosed:
					// 忽略
					return
				default:
					log.Warnw("添加连接失败", "目标", a, "错误", err)
					return
				}
			}()
		}
	}()
	return nil
}

// containsMultiaddr 检查地址列表中是否包含指定地址
//
// 参数:
//   - addrs: 地址列表
//   - addr: 要检查的地址
//
// 返回值:
//   - bool: 如果地址列表包含指定地址则返回 true
func containsMultiaddr(addrs []ma.Multiaddr, addr ma.Multiaddr) bool {
	for _, a := range addrs {
		if addr.Equal(a) {
			return true
		}
	}
	return false
}
