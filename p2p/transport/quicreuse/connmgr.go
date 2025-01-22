package quicreuse

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"sync"

	ma "github.com/dep2p/go-dep2p/multiformats/multiaddr"
	manet "github.com/dep2p/go-dep2p/multiformats/multiaddr/net"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/quic-go/quic-go"
	quiclogging "github.com/quic-go/quic-go/logging"
	quicmetrics "github.com/quic-go/quic-go/metrics"
)

// ConnManager 管理 QUIC 连接和监听器的复用
// 参数:
//   - reuseUDP4: *reuse UDP4 连接复用器
//   - reuseUDP6: *reuse UDP6 连接复用器
//   - enableReuseport: bool 是否启用端口复用
//   - enableMetrics: bool 是否启用指标收集
//   - registerer: prometheus.Registerer Prometheus 注册器
//   - serverConfig: *quic.Config 服务端配置
//   - clientConfig: *quic.Config 客户端配置
//   - quicListenersMu: sync.Mutex 监听器映射锁
//   - quicListeners: map[string]quicListenerEntry 监听器映射表
//   - srk: quic.StatelessResetKey 无状态重置密钥
//   - tokenKey: quic.TokenGeneratorKey 令牌生成密钥
type ConnManager struct {
	reuseUDP4       *reuse
	reuseUDP6       *reuse
	enableReuseport bool

	enableMetrics bool
	registerer    prometheus.Registerer

	serverConfig *quic.Config
	clientConfig *quic.Config

	quicListenersMu sync.Mutex
	quicListeners   map[string]quicListenerEntry

	srk      quic.StatelessResetKey
	tokenKey quic.TokenGeneratorKey
}

// quicListenerEntry 表示一个 QUIC 监听器条目
// 参数:
//   - refCount: int 引用计数
//   - ln: *quicListener QUIC 监听器
type quicListenerEntry struct {
	refCount int
	ln       *quicListener
}

// NewConnManager 创建一个新的连接管理器
// 参数:
//   - statelessResetKey: quic.StatelessResetKey 无状态重置密钥
//   - tokenKey: quic.TokenGeneratorKey 令牌生成密钥
//   - opts: ...Option 配置选项
//
// 返回值:
//   - *ConnManager 连接管理器
//   - error 错误信息
func NewConnManager(statelessResetKey quic.StatelessResetKey, tokenKey quic.TokenGeneratorKey, opts ...Option) (*ConnManager, error) {
	// 初始化连接管理器
	cm := &ConnManager{
		enableReuseport: true,
		quicListeners:   make(map[string]quicListenerEntry),
		srk:             statelessResetKey,
		tokenKey:        tokenKey,
		registerer:      prometheus.DefaultRegisterer,
	}
	// 应用配置选项
	for _, o := range opts {
		if err := o(cm); err != nil {
			log.Debugf("应用配置选项时出错: %s", err)
			return nil, err
		}
	}

	// 克隆并配置 QUIC 配置
	quicConf := quicConfig.Clone()
	quicConf.Tracer = cm.getTracer()
	serverConfig := quicConf.Clone()

	cm.clientConfig = quicConf
	cm.serverConfig = serverConfig
	// 如果启用端口复用，初始化 UDP4 和 UDP6 复用器
	if cm.enableReuseport {
		cm.reuseUDP4 = newReuse(&statelessResetKey, &tokenKey)
		cm.reuseUDP6 = newReuse(&statelessResetKey, &tokenKey)
	}
	return cm, nil
}

// getTracer 返回一个 QUIC 连接跟踪器生成函数
// 返回值:
//   - func(context.Context, quiclogging.Perspective, quic.ConnectionID) *quiclogging.ConnectionTracer 跟踪器生成函数
func (c *ConnManager) getTracer() func(context.Context, quiclogging.Perspective, quic.ConnectionID) *quiclogging.ConnectionTracer {
	return func(ctx context.Context, p quiclogging.Perspective, ci quic.ConnectionID) *quiclogging.ConnectionTracer {
		var promTracer *quiclogging.ConnectionTracer
		// 如果启用指标收集，创建相应的跟踪器
		if c.enableMetrics {
			switch p {
			case quiclogging.PerspectiveClient:
				promTracer = quicmetrics.NewClientConnectionTracerWithRegisterer(c.registerer)
			case quiclogging.PerspectiveServer:
				promTracer = quicmetrics.NewServerConnectionTracerWithRegisterer(c.registerer)
			default:
				log.Debugf("无效的日志视角: %s", p)
			}
		}
		var tracer *quiclogging.ConnectionTracer
		// 如果设置了 qlog 跟踪目录，创建 qlog 跟踪器
		if qlogTracerDir != "" {
			tracer = qloggerForDir(qlogTracerDir, p, ci)
			if promTracer != nil {
				tracer = quiclogging.NewMultiplexedConnectionTracer(promTracer,
					tracer)
			}
		}
		return tracer
	}
}

// getReuse 根据网络类型返回对应的复用器
// 参数:
//   - network: string 网络类型
//
// 返回值:
//   - *reuse 复用器
//   - error 错误信息
func (c *ConnManager) getReuse(network string) (*reuse, error) {
	switch network {
	case "udp4":
		return c.reuseUDP4, nil
	case "udp6":
		return c.reuseUDP6, nil
	default:
		return nil, errors.New("无效的网络类型：必须是 udp4 或 udp6")
	}
}

// ListenQUIC 在指定地址上监听 QUIC 连接
// 参数:
//   - addr: ma.Multiaddr 监听地址
//   - tlsConf: *tls.Config TLS 配置
//   - allowWindowIncrease: func(conn quic.Connection, delta uint64) bool 窗口增长控制函数
//
// 返回值:
//   - Listener 监听器
//   - error 错误信息
func (c *ConnManager) ListenQUIC(addr ma.Multiaddr, tlsConf *tls.Config, allowWindowIncrease func(conn quic.Connection, delta uint64) bool) (Listener, error) {
	return c.ListenQUICAndAssociate(nil, addr, tlsConf, allowWindowIncrease)
}

// ListenQUICAndAssociate 返回一个 QUIC 监听器并将底层传输与给定的关联关系相关联
// 参数:
//   - association: any 关联对象
//   - addr: ma.Multiaddr 监听地址
//   - tlsConf: *tls.Config TLS 配置
//   - allowWindowIncrease: func(conn quic.Connection, delta uint64) bool 窗口增长控制函数
//
// 返回值:
//   - Listener 监听器
//   - error 错误信息
func (c *ConnManager) ListenQUICAndAssociate(association any, addr ma.Multiaddr, tlsConf *tls.Config, allowWindowIncrease func(conn quic.Connection, delta uint64) bool) (Listener, error) {
	// 解析监听地址
	netw, host, err := manet.DialArgs(addr)
	if err != nil {
		log.Debugf("解析监听地址时出错: %s", err)
		return nil, err
	}
	laddr, err := net.ResolveUDPAddr(netw, host)
	if err != nil {
		log.Debugf("解析监听地址时出错: %s", err)
		return nil, err
	}

	c.quicListenersMu.Lock()
	defer c.quicListenersMu.Unlock()

	key := laddr.String()
	entry, ok := c.quicListeners[key]
	// 如果监听器不存在，创建新的监听器
	if !ok {
		tr, err := c.transportForListen(association, netw, laddr)
		if err != nil {
			log.Debugf("创建传输层时出错: %s", err)
			return nil, err
		}
		ln, err := newQuicListener(tr, c.serverConfig)
		if err != nil {
			log.Debugf("创建QUIC监听器时出错: %s", err)
			return nil, err
		}
		key = tr.LocalAddr().String()
		entry = quicListenerEntry{ln: ln}
	}
	// 添加新的监听配置
	l, err := entry.ln.Add(tlsConf, allowWindowIncrease, func() { c.onListenerClosed(key) })
	if err != nil {
		if entry.refCount <= 0 {
			entry.ln.Close()
		}
		log.Debugf("添加监听器时出错: %s", err)
		return nil, err
	}
	entry.refCount++
	c.quicListeners[key] = entry
	return l, nil
}

// onListenerClosed 处理监听器关闭事件
// 参数:
//   - key: string 监听器键值
func (c *ConnManager) onListenerClosed(key string) {
	c.quicListenersMu.Lock()
	defer c.quicListenersMu.Unlock()

	entry := c.quicListeners[key]
	entry.refCount = entry.refCount - 1
	// 如果引用计数为 0，删除并关闭监听器
	if entry.refCount <= 0 {
		delete(c.quicListeners, key)
		entry.ln.Close()
	} else {
		c.quicListeners[key] = entry
	}
}

// SharedNonQUICPacketConn 返回一个共享的非 QUIC 数据包连接
// 参数:
//   - network: string 网络类型
//   - laddr: *net.UDPAddr 本地地址
//
// 返回值:
//   - net.PacketConn 数据包连接
//   - error 错误信息
func (c *ConnManager) SharedNonQUICPacketConn(network string, laddr *net.UDPAddr) (net.PacketConn, error) {
	c.quicListenersMu.Lock()
	defer c.quicListenersMu.Unlock()
	key := laddr.String()
	entry, ok := c.quicListeners[key]
	if !ok {
		log.Debugf("期望能够与 QUIC 监听器共享，但未找到 QUIC 监听器。QUIC 监听器应该先启动")
		return nil, fmt.Errorf("期望能够与 QUIC 监听器共享，但未找到 QUIC 监听器。QUIC 监听器应该先启动")
	}
	t := entry.ln.transport
	if t, ok := t.(*refcountedTransport); ok {
		t.IncreaseCount()
		ctx, cancel := context.WithCancel(context.Background())
		return &nonQUICPacketConn{
			ctx:             ctx,
			ctxCancel:       cancel,
			owningTransport: t,
			tr:              &t.Transport,
		}, nil
	}
	log.Debugf("期望能够与 QUIC 监听器共享，但 QUIC 监听器未使用 refcountedTransport。不应设置 `DisableReuseport`")
	return nil, fmt.Errorf("期望能够与 QUIC 监听器共享，但 QUIC 监听器未使用 refcountedTransport。不应设置 `DisableReuseport`")
}

// transportForListen 为监听创建传输层
// 参数:
//   - association: any 关联对象
//   - network: string 网络类型
//   - laddr: *net.UDPAddr 本地地址
//
// 返回值:
//   - refCountedQuicTransport 传输层
//   - error 错误信息
func (c *ConnManager) transportForListen(association any, network string, laddr *net.UDPAddr) (refCountedQuicTransport, error) {
	if c.enableReuseport {
		reuse, err := c.getReuse(network)
		if err != nil {
			log.Debugf("获取复用器时出错: %s", err)
			return nil, err
		}
		tr, err := reuse.TransportForListen(network, laddr)
		if err != nil {
			log.Debugf("获取传输层时出错: %s", err)
			return nil, err
		}
		tr.associate(association)
		return tr, nil
	}

	conn, err := net.ListenUDP(network, laddr)
	if err != nil {
		log.Debugf("创建UDP连接时出错: %s", err)
		return nil, err
	}
	return &singleOwnerTransport{
		packetConn: conn,
		Transport: quic.Transport{
			Conn:              conn,
			StatelessResetKey: &c.srk,
			TokenGeneratorKey: &c.tokenKey,
		},
	}, nil
}

type associationKey struct{}

// WithAssociation 返回一个带有给定关联的新上下文
// 参数:
//   - ctx: context.Context 上下文
//   - association: any 关联对象
//
// 返回值:
//   - context.Context 新的上下文
func WithAssociation(ctx context.Context, association any) context.Context {
	return context.WithValue(ctx, associationKey{}, association)
}

// DialQUIC 建立 QUIC 连接
// 参数:
//   - ctx: context.Context 上下文
//   - raddr: ma.Multiaddr 远程地址
//   - tlsConf: *tls.Config TLS 配置
//   - allowWindowIncrease: func(conn quic.Connection, delta uint64) bool 窗口增长控制函数
//
// 返回值:
//   - quic.Connection QUIC 连接
//   - error 错误信息
func (c *ConnManager) DialQUIC(ctx context.Context, raddr ma.Multiaddr, tlsConf *tls.Config, allowWindowIncrease func(conn quic.Connection, delta uint64) bool) (quic.Connection, error) {
	naddr, v, err := FromQuicMultiaddr(raddr)
	if err != nil {
		log.Debugf("从QUIC多地址转换为版本时出错: %s", err)
		return nil, err
	}
	netw, _, err := manet.DialArgs(raddr)
	if err != nil {
		log.Debugf("解析远程地址时出错: %s", err)
		return nil, err
	}

	quicConf := c.clientConfig.Clone()
	quicConf.AllowConnectionWindowIncrease = allowWindowIncrease

	if v == quic.Version1 {
		// 端点明确支持 QUIC v1，因此我们只使用该版本
		quicConf.Versions = []quic.Version{quic.Version1}
	} else {
		return nil, errors.New("未知的 QUIC 版本")
	}

	var tr refCountedQuicTransport
	if association := ctx.Value(associationKey{}); association != nil {
		tr, err = c.TransportWithAssociationForDial(association, netw, naddr)
	} else {
		tr, err = c.TransportForDial(netw, naddr)
	}
	if err != nil {
		log.Debugf("获取传输层时出错: %s", err)
		return nil, err
	}
	conn, err := tr.Dial(ctx, naddr, tlsConf, quicConf)
	if err != nil {
		tr.DecreaseCount()
		log.Debugf("拨号时出错: %s", err)
		return nil, err
	}
	return conn, nil
}

// TransportForDial 为拨号创建传输层
// 参数:
//   - network: string 网络类型
//   - raddr: *net.UDPAddr 远程地址
//
// 返回值:
//   - refCountedQuicTransport 传输层
//   - error 错误信息
func (c *ConnManager) TransportForDial(network string, raddr *net.UDPAddr) (refCountedQuicTransport, error) {
	return c.TransportWithAssociationForDial(nil, network, raddr)
}

// TransportWithAssociationForDial 返回用于拨号的 QUIC 传输层，优先使用具有给定关联的传输层
// 参数:
//   - association: any 关联对象
//   - network: string 网络类型
//   - raddr: *net.UDPAddr 远程地址
//
// 返回值:
//   - refCountedQuicTransport 传输层
//   - error 错误信息
func (c *ConnManager) TransportWithAssociationForDial(association any, network string, raddr *net.UDPAddr) (refCountedQuicTransport, error) {
	if c.enableReuseport {
		reuse, err := c.getReuse(network)
		if err != nil {
			log.Debugf("获取复用器时出错: %s", err)
			return nil, err
		}
		return reuse.transportWithAssociationForDial(association, network, raddr)
	}

	var laddr *net.UDPAddr
	switch network {
	case "udp4":
		laddr = &net.UDPAddr{IP: net.IPv4zero, Port: 0}
	case "udp6":
		laddr = &net.UDPAddr{IP: net.IPv6zero, Port: 0}
	}
	conn, err := net.ListenUDP(network, laddr)
	if err != nil {
		log.Debugf("创建UDP连接时出错: %s", err)
		return nil, err
	}
	return &singleOwnerTransport{Transport: quic.Transport{Conn: conn, StatelessResetKey: &c.srk}, packetConn: conn}, nil
}

// Protocols 返回支持的协议列表
// 返回值:
//   - []int 协议 ID 列表
func (c *ConnManager) Protocols() []int {
	return []int{ma.P_QUIC_V1}
}

// Close 关闭连接管理器
// 返回值:
//   - error 错误信息
func (c *ConnManager) Close() error {
	if !c.enableReuseport {
		return nil
	}
	if err := c.reuseUDP6.Close(); err != nil {
		log.Debugf("关闭UDP6连接时出错: %s", err)
		return err
	}
	return c.reuseUDP4.Close()
}

// ClientConfig 返回客户端配置
// 返回值:
//   - *quic.Config 客户端配置
func (c *ConnManager) ClientConfig() *quic.Config {
	return c.clientConfig
}
