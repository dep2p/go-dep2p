package quicreuse

import (
	"context"
	"crypto/tls"
	"net"
	"sync"
	"time"

	"github.com/dep2p/go-dep2p/p2plib/netroute"
	"github.com/google/gopacket/routing"
	"github.com/quic-go/quic-go"
)

// refCountedQuicTransport 定义了一个带引用计数的 QUIC 传输接口
// 该接口提供了基本的 QUIC 传输功能和引用计数管理
type refCountedQuicTransport interface {
	// LocalAddr 返回本地地址
	// 返回值:
	//   - net.Addr 本地网络地址
	LocalAddr() net.Addr

	// WriteTo 直接发送数据包,用于打洞
	// 参数:
	//   - []byte: 要发送的数据
	//   - net.Addr: 目标地址
	// 返回值:
	//   - int: 发送的字节数
	//   - error: 发送过程中的错误
	WriteTo([]byte, net.Addr) (int, error)

	// Close 关闭传输
	// 返回值:
	//   - error: 关闭过程中的错误
	Close() error

	// DecreaseCount 减少引用计数
	DecreaseCount()

	// IncreaseCount 增加引用计数
	IncreaseCount()

	// Dial 建立 QUIC 连接
	// 参数:
	//   - ctx: 上下文
	//   - addr: 目标地址
	//   - tlsConf: TLS 配置
	//   - conf: QUIC 配置
	// 返回值:
	//   - quic.Connection: QUIC 连接
	//   - error: 连接过程中的错误
	Dial(ctx context.Context, addr net.Addr, tlsConf *tls.Config, conf *quic.Config) (quic.Connection, error)

	// Listen 监听 QUIC 连接
	// 参数:
	//   - tlsConf: TLS 配置
	//   - conf: QUIC 配置
	// 返回值:
	//   - *quic.Listener: QUIC 监听器
	//   - error: 监听过程中的错误
	Listen(tlsConf *tls.Config, conf *quic.Config) (*quic.Listener, error)
}

// singleOwnerTransport 实现了单一所有者的 QUIC 传输
type singleOwnerTransport struct {
	// Transport 是底层的 QUIC 传输
	quic.Transport

	// packetConn 用于直接发送数据包
	packetConn net.PacketConn
}

// IncreaseCount 增加引用计数(空实现)
func (c *singleOwnerTransport) IncreaseCount() {}

// DecreaseCount 减少引用计数并关闭传输
func (c *singleOwnerTransport) DecreaseCount() {
	c.Transport.Close()
}

// LocalAddr 返回本地地址
func (c *singleOwnerTransport) LocalAddr() net.Addr {
	return c.Transport.Conn.LocalAddr()
}

// Close 关闭传输和数据包连接
func (c *singleOwnerTransport) Close() error {
	// TODO(当我们放弃支持 go 1.19 时使用 errors.Join)
	c.Transport.Close()
	return c.packetConn.Close()
}

// WriteTo 直接发送数据包
func (c *singleOwnerTransport) WriteTo(b []byte, addr net.Addr) (int, error) {
	return c.Transport.WriteTo(b, addr)
}

// 常量定义为变量以简化测试
var (
	// garbageCollectInterval 垃圾回收间隔时间
	garbageCollectInterval = 30 * time.Second
	// maxUnusedDuration 最大未使用时间
	maxUnusedDuration = 10 * time.Second
)

// refcountedTransport 实现了带引用计数的 QUIC 传输
type refcountedTransport struct {
	// Transport 是底层的 QUIC 传输
	quic.Transport

	// packetConn 用于直接发送数据包
	packetConn net.PacketConn

	// mutex 用于保护并发访问
	mutex sync.Mutex
	// refCount 引用计数
	refCount int
	// unusedSince 记录最后一次使用时间
	unusedSince time.Time

	// assocations 存储与该传输关联的值
	assocations map[any]struct{}
}

// associate 将任意值与此传输关联
// 这允许我们在监听时"标记"refcountedTransport，以便后续拨号时使用
// 对于打洞和了解我们自己的观察到的监听地址是必需的
func (c *refcountedTransport) associate(a any) {
	if a == nil {
		return
	}
	c.mutex.Lock()
	defer c.mutex.Unlock()
	if c.assocations == nil {
		c.assocations = make(map[any]struct{})
	}
	c.assocations[a] = struct{}{}
}

// hasAssociation 检查传输是否具有给定关联
// 如果是空关联，总是返回 true
func (c *refcountedTransport) hasAssociation(a any) bool {
	if a == nil {
		return true
	}
	c.mutex.Lock()
	defer c.mutex.Unlock()
	_, ok := c.assocations[a]
	return ok
}

// IncreaseCount 增加引用计数
func (c *refcountedTransport) IncreaseCount() {
	c.mutex.Lock()
	c.refCount++
	c.unusedSince = time.Time{}
	c.mutex.Unlock()
}

// Close 关闭传输和数据包连接
func (c *refcountedTransport) Close() error {
	// TODO(当我们放弃支持 go 1.19 时使用 errors.Join)
	c.Transport.Close()
	return c.packetConn.Close()
}

// WriteTo 直接发送数据包
func (c *refcountedTransport) WriteTo(b []byte, addr net.Addr) (int, error) {
	return c.Transport.WriteTo(b, addr)
}

// LocalAddr 返回本地地址
func (c *refcountedTransport) LocalAddr() net.Addr {
	return c.Transport.Conn.LocalAddr()
}

// DecreaseCount 减少引用计数
func (c *refcountedTransport) DecreaseCount() {
	c.mutex.Lock()
	c.refCount--
	if c.refCount == 0 {
		c.unusedSince = time.Now()
	}
	c.mutex.Unlock()
}

// ShouldGarbageCollect 检查是否应该进行垃圾回收
func (c *refcountedTransport) ShouldGarbageCollect(now time.Time) bool {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	return !c.unusedSince.IsZero() && c.unusedSince.Add(maxUnusedDuration).Before(now)
}

// reuse 管理 QUIC 传输的复用
type reuse struct {
	mutex sync.Mutex

	// closeChan 用于关闭信号
	closeChan chan struct{}
	// gcStopChan 用于垃圾回收停止信号
	gcStopChan chan struct{}

	// routes 路由器
	routes routing.Router
	// unicast 存储单播地址的传输映射
	unicast map[string] /* IP.String() */ map[int] /* port */ *refcountedTransport
	// globalListeners 包含监听在 0.0.0.0 / :: 上的传输
	globalListeners map[int]*refcountedTransport
	// globalDialers 包含我们已拨出的传输，这些传输监听在 0.0.0.0 / ::
	// 在拨号时，如果 globalListeners 中没有可用传输，则从此映射中复用传输
	// 在监听时，如果请求的端口为 0，则从此映射中复用传输，然后移至 globalListeners
	globalDialers map[int]*refcountedTransport

	// statelessResetKey 用于无状态重置
	statelessResetKey *quic.StatelessResetKey
	// tokenGeneratorKey 用于令牌生成
	tokenGeneratorKey *quic.TokenGeneratorKey
}

// newReuse 创建一个新的 QUIC 传输复用器
// 参数:
//   - srk: *quic.StatelessResetKey 无状态重置密钥
//   - tokenKey: *quic.TokenGeneratorKey 令牌生成器密钥
//
// 返回值:
//   - *reuse QUIC 传输复用器实例
func newReuse(srk *quic.StatelessResetKey, tokenKey *quic.TokenGeneratorKey) *reuse {
	// 初始化复用器结构体
	r := &reuse{
		unicast:           make(map[string]map[int]*refcountedTransport), // 初始化单播传输映射
		globalListeners:   make(map[int]*refcountedTransport),            // 初始化全局监听器映射
		globalDialers:     make(map[int]*refcountedTransport),            // 初始化全局拨号器映射
		closeChan:         make(chan struct{}),                           // 初始化关闭通道
		gcStopChan:        make(chan struct{}),                           // 初始化垃圾回收停止通道
		statelessResetKey: srk,                                           // 设置无状态重置密钥
		tokenGeneratorKey: tokenKey,                                      // 设置令牌生成器密钥
	}
	// 启动垃圾回收协程
	go r.gc()
	return r
}

// gc 执行垃圾回收，清理未使用的传输
func (r *reuse) gc() {
	// 延迟执行清理操作
	defer func() {
		r.mutex.Lock()
		// 关闭所有全局监听器
		for _, tr := range r.globalListeners {
			tr.Close()
		}
		// 关闭所有全局拨号器
		for _, tr := range r.globalDialers {
			tr.Close()
		}
		// 关闭所有单播传输
		for _, trs := range r.unicast {
			for _, tr := range trs {
				tr.Close()
			}
		}
		r.mutex.Unlock()
		// 关闭垃圾回收停止通道
		close(r.gcStopChan)
	}()

	// 创建定时器
	ticker := time.NewTicker(garbageCollectInterval)
	defer ticker.Stop()

	// 循环执行垃圾回收
	for {
		select {
		case <-r.closeChan:
			// 收到关闭信号时退出
			return
		case <-ticker.C:
			// 定时执行垃圾回收
			now := time.Now()
			r.mutex.Lock()
			// 清理全局监听器
			for key, tr := range r.globalListeners {
				if tr.ShouldGarbageCollect(now) {
					tr.Close()
					delete(r.globalListeners, key)
				}
			}
			// 清理全局拨号器
			for key, tr := range r.globalDialers {
				if tr.ShouldGarbageCollect(now) {
					tr.Close()
					delete(r.globalDialers, key)
				}
			}
			// 清理单播传输
			for ukey, trs := range r.unicast {
				for key, tr := range trs {
					if tr.ShouldGarbageCollect(now) {
						tr.Close()
						delete(trs, key)
					}
				}
				// 如果某个 IP 下没有传输了，删除该 IP 的映射
				if len(trs) == 0 {
					delete(r.unicast, ukey)
					// 如果所有单播传输都被删除，重置路由
					if len(r.unicast) == 0 {
						r.routes = nil
					} else {
						// 忽略错误，因为我们无法处理
						r.routes, _ = netroute.New()
					}
				}
			}
			r.mutex.Unlock()
		}
	}
}

// transportWithAssociationForDial 获取用于拨号的关联传输
// 参数:
//   - association: any 关联对象
//   - network: string 网络类型
//   - raddr: *net.UDPAddr 远程地址
//
// 返回值:
//   - *refcountedTransport 引用计数传输
//   - error 错误信息
func (r *reuse) transportWithAssociationForDial(association any, network string, raddr *net.UDPAddr) (*refcountedTransport, error) {
	var ip *net.IP

	// 只有在有非 0.0.0.0 监听器时才查找源地址
	r.mutex.Lock()
	router := r.routes
	r.mutex.Unlock()

	if router != nil {
		_, _, src, err := router.Route(raddr.IP)
		if err == nil && !src.IsUnspecified() {
			ip = &src
		}
	}

	r.mutex.Lock()
	defer r.mutex.Unlock()

	// 获取用于拨号的传输
	tr, err := r.transportForDialLocked(association, network, ip)
	if err != nil {
		log.Debugf("获取用于拨号的传输时出错: %s", err)
		return nil, err
	}
	// 增加引用计数
	tr.IncreaseCount()
	return tr, nil
}

// transportForDialLocked 获取用于拨号的传输（已加锁）
// 参数:
//   - association: any 关联对象
//   - network: string 网络类型
//   - source: *net.IP 源 IP 地址
//
// 返回值:
//   - *refcountedTransport 引用计数传输
//   - error 错误信息
func (r *reuse) transportForDialLocked(association any, network string, source *net.IP) (*refcountedTransport, error) {
	if source != nil {
		// 如果已有合适的传输
		if trs, ok := r.unicast[source.String()]; ok {
			// 优先使用具有给定关联的传输
			for _, tr := range trs {
				if tr.hasAssociation(association) {
					return tr, nil
				}
			}
		}
	}

	// 使用监听在 0.0.0.0（或 ::）的传输
	// 同样优先使用具有给定关联的传输
	for _, tr := range r.globalListeners {
		if tr.hasAssociation(association) {
			return tr, nil
		}
	}

	// 使用之前用于拨号的传输
	for _, tr := range r.globalDialers {
		return tr, nil
	}

	// 如果没有可用的传输，从随机端口创建新连接
	var addr *net.UDPAddr
	switch network {
	case "udp4":
		addr = &net.UDPAddr{IP: net.IPv4zero, Port: 0}
	case "udp6":
		addr = &net.UDPAddr{IP: net.IPv6zero, Port: 0}
	}
	// 创建新的 UDP 监听器
	conn, err := net.ListenUDP(network, addr)
	if err != nil {
		log.Debugf("创建UDP监听器时出错: %s", err)
		return nil, err
	}
	// 创建新的传输
	tr := &refcountedTransport{Transport: quic.Transport{
		Conn:              conn,
		StatelessResetKey: r.statelessResetKey,
		TokenGeneratorKey: r.tokenGeneratorKey,
	}, packetConn: conn}
	// 将新传输添加到全局拨号器映射
	r.globalDialers[conn.LocalAddr().(*net.UDPAddr).Port] = tr
	return tr, nil
}

// TransportForListen 获取用于监听的传输
// 参数:
//   - network: string 网络类型
//   - laddr: *net.UDPAddr 本地地址
//
// 返回值:
//   - *refcountedTransport 引用计数传输
//   - error 错误信息
func (r *reuse) TransportForListen(network string, laddr *net.UDPAddr) (*refcountedTransport, error) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	// 检查是否可以复用已有的拨号传输
	// 当请求的端口为 0 或已在 globalDialers 中时，复用 globalDialers 中的传输
	// 复用时，将传输从 globalDialers 移动到 globalListeners
	if laddr.IP.IsUnspecified() {
		var rTr *refcountedTransport
		var localAddr *net.UDPAddr

		if laddr.Port == 0 {
			// 请求端口为 0，可以复用任何传输
			for _, tr := range r.globalDialers {
				rTr = tr
				localAddr = rTr.LocalAddr().(*net.UDPAddr)
				delete(r.globalDialers, localAddr.Port)
				break
			}
		} else if _, ok := r.globalDialers[laddr.Port]; ok {
			rTr = r.globalDialers[laddr.Port]
			localAddr = rTr.LocalAddr().(*net.UDPAddr)
			delete(r.globalDialers, localAddr.Port)
		}
		// 找到匹配的传输
		if rTr != nil {
			rTr.IncreaseCount()
			r.globalListeners[localAddr.Port] = rTr
			return rTr, nil
		}
	}

	// 创建新的 UDP 监听器
	conn, err := net.ListenUDP(network, laddr)
	if err != nil {
		log.Debugf("创建UDP监听器时出错: %s", err)
		return nil, err
	}
	localAddr := conn.LocalAddr().(*net.UDPAddr)
	// 创建新的传输
	tr := &refcountedTransport{
		Transport: quic.Transport{
			Conn:              conn,
			StatelessResetKey: r.statelessResetKey,
		},
		packetConn: conn,
	}
	tr.IncreaseCount()

	// 处理全局地址的监听
	if localAddr.IP.IsUnspecified() {
		// 内核已检查 laddr 未被监听，此处无需再次检查
		r.globalListeners[localAddr.Port] = tr
		return tr, nil
	}

	// 处理单播地址的监听
	if _, ok := r.unicast[localAddr.IP.String()]; !ok {
		r.unicast[localAddr.IP.String()] = make(map[int]*refcountedTransport)
		// 添加新监听器时，假设系统路由可能已更改
		// 忽略错误，因为我们无法处理
		r.routes, _ = netroute.New()
	}

	// 内核已检查 laddr 未被监听，此处无需再次检查
	r.unicast[localAddr.IP.String()][localAddr.Port] = tr
	return tr, nil
}

// Close 关闭复用器
// 返回值:
//   - error 错误信息
func (r *reuse) Close() error {
	// 关闭主通道
	close(r.closeChan)
	// 等待垃圾回收完成
	<-r.gcStopChan
	return nil
}
