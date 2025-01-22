package swarm

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"strconv"
	"sync"
	"time"

	"github.com/dep2p/go-dep2p/core/canonicallog"
	"github.com/dep2p/go-dep2p/core/network"
	"github.com/dep2p/go-dep2p/core/peer"
	"github.com/dep2p/go-dep2p/core/peerstore"
	"github.com/dep2p/go-dep2p/core/transport"

	ma "github.com/dep2p/go-dep2p/multiformats/multiaddr"
	mafmt "github.com/dep2p/go-dep2p/multiformats/multiaddr/fmt"
	manet "github.com/dep2p/go-dep2p/multiformats/multiaddr/net"
)

// 解析地址时返回的最大地址数量
const maximumResolvedAddresses = 100

// DNS地址递归解析的最大深度
const maximumDNSADDRRecursion = 4

// 拨号同步图示:
//
//   多个Dial()调用者   同步 w.  拨号多个地址       结果返回给调用者
//  ----------------------\    dialsync    使用最早的            /--------------
//  -----------------------\              |----------\           /----------------
//  ------------------------>------------<-------     >---------<-----------------
//  -----------------------|              \----x                 \----------------
//  ----------------------|                \-----x                \---------------
//                                         任何一个可能失败          如果最后没有地址
//                                                             重试拨号尝试 x

var (
	// ErrDialBackoff 当对某个peer拨号过于频繁时由退避代码返回
	ErrDialBackoff = errors.New("拨号退避")

	// ErrDialRefusedBlackHole 当我们处于黑洞环境时返回
	ErrDialRefusedBlackHole = errors.New("由于黑洞拒绝拨号")

	// ErrDialToSelf 当尝试拨号到自己时返回
	ErrDialToSelf = errors.New("尝试拨号到自己")

	// ErrNoTransport 当我们不知道给定multiaddr的传输协议时返回
	ErrNoTransport = errors.New("没有传输协议")

	// ErrAllDialsFailed 当连接peer最终失败时返回
	ErrAllDialsFailed = errors.New("所有拨号失败")

	// ErrNoAddresses 当我们找不到要拨号的peer的任何地址时返回
	ErrNoAddresses = errors.New("没有地址")

	// ErrNoGoodAddresses 当我们找到peer的地址但无法使用任何一个时返回
	ErrNoGoodAddresses = errors.New("没有可用地址")

	// ErrGaterDisallowedConnection 当gater阻止我们与peer建立连接时返回
	ErrGaterDisallowedConnection = errors.New("gater禁止与peer建立连接")
)

// ErrQUICDraft29 包装 ErrNoTransport 并提供更有意义的错误消息
var ErrQUICDraft29 errQUICDraft29

// errQUICDraft29 定义QUIC draft-29错误类型
type errQUICDraft29 struct{}

// Error 返回错误信息
//
// 返回值:
//   - string 错误描述
func (errQUICDraft29) Error() string {
	return "QUIC draft-29 已被移除, QUIC (RFC 9000) 可通过 /quic-v1 访问"
}

// Unwrap 返回原始错误
//
// 返回值:
//   - error 原始错误
func (errQUICDraft29) Unwrap() error {
	return ErrNoTransport
}

// DialAttempts 控制一个goroutine尝试拨号给定peer的次数
// 注意:这已经降到1,因为我们现在有太多拨号。要添加回来,在Dial(.)中添加循环
const DialAttempts = 1

// ConcurrentFdDials 是通过消耗文件描述符的传输进行的并发出站拨号数量
const ConcurrentFdDials = 160

// DefaultPerPeerRateLimit 是每个peer的并发出站拨号数量
var DefaultPerPeerRateLimit = 8

// DialBackoff 是用于跟踪peer拨号退避的类型
// Dialbackoff用于避免过度拨号相同的死亡peer
// 每当我们在peer的所有地址上完全超时时,我们将地址添加到DialBackoff
// 然后,每当我们再次尝试拨号该peer时,我们检查每个地址是否退避
// 如果它在退避中,我们不拨号该地址并立即退出
// 如果拨号成功,该peer及其所有地址都会从退避中移除
//
// 注意:
//   - 可以安全使用其零值
//   - 它是线程安全的
//   - 使用后移动此类型是不安全的
type DialBackoff struct {
	entries map[peer.ID]map[string]*backoffAddr
	lock    sync.RWMutex
}

// backoffAddr 存储单个地址的退避信息
type backoffAddr struct {
	tries int       // 尝试次数
	until time.Time // 退避截止时间
}

// init 初始化退避系统
//
// 参数:
//   - ctx: context.Context 上下文对象
func (db *DialBackoff) init(ctx context.Context) {
	if db.entries == nil {
		db.entries = make(map[peer.ID]map[string]*backoffAddr)
	}
	go db.background(ctx)
}

// background 运行后台清理任务
//
// 参数:
//   - ctx: context.Context 上下文对象
func (db *DialBackoff) background(ctx context.Context) {
	ticker := time.NewTicker(BackoffMax)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			db.cleanup()
		}
	}
}

// Backoff 返回客户端是否应该退避拨号地址addr的peer p
//
// 参数:
//   - p: peer.ID peer标识符
//   - addr: ma.Multiaddr 多地址
//
// 返回值:
//   - bool 是否应该退避
func (db *DialBackoff) Backoff(p peer.ID, addr ma.Multiaddr) (backoff bool) {
	db.lock.RLock()
	defer db.lock.RUnlock()

	ap, found := db.entries[p][string(addr.Bytes())]
	return found && time.Now().Before(ap.until)
}

// BackoffBase 是退避的基本时间量(默认:5秒)
var BackoffBase = time.Second * 5

// BackoffCoef 是退避系数(默认:1秒)
var BackoffCoef = time.Second

// BackoffMax 是最大退避时间(默认:5分钟)
var BackoffMax = time.Minute * 5

// AddBackoff 将peer的地址添加到退避
//
// 退避不是指数级的,而是二次方的,根据以下公式计算:
//
//	BackoffBase + BakoffCoef * PriorBackoffs^2
//
// 其中PriorBackoffs是之前的退避次数
//
// 参数:
//   - p: peer.ID peer标识符
//   - addr: ma.Multiaddr 多地址
func (db *DialBackoff) AddBackoff(p peer.ID, addr ma.Multiaddr) {
	saddr := string(addr.Bytes())
	db.lock.Lock()
	defer db.lock.Unlock()
	bp, ok := db.entries[p]
	if !ok {
		bp = make(map[string]*backoffAddr, 1)
		db.entries[p] = bp
	}
	ba, ok := bp[saddr]
	if !ok {
		bp[saddr] = &backoffAddr{
			tries: 1,
			until: time.Now().Add(BackoffBase),
		}
		return
	}

	backoffTime := BackoffBase + BackoffCoef*time.Duration(ba.tries*ba.tries)
	if backoffTime > BackoffMax {
		backoffTime = BackoffMax
	}
	ba.until = time.Now().Add(backoffTime)
	ba.tries++
}

// Clear 移除退避记录。客户端应在成功拨号后调用此方法
//
// 参数:
//   - p: peer.ID peer标识符
func (db *DialBackoff) Clear(p peer.ID) {
	db.lock.Lock()
	defer db.lock.Unlock()
	delete(db.entries, p)
}

// cleanup 清理过期的退避记录
func (db *DialBackoff) cleanup() {
	db.lock.Lock()
	defer db.lock.Unlock()
	now := time.Now()
	for p, e := range db.entries {
		good := false
		for _, backoff := range e {
			backoffTime := BackoffBase + BackoffCoef*time.Duration(backoff.tries*backoff.tries)
			if backoffTime > BackoffMax {
				backoffTime = BackoffMax
			}
			if now.Before(backoff.until.Add(backoffTime)) {
				good = true
				break
			}
		}
		if !good {
			delete(db.entries, p)
		}
	}
}

// DialPeer 连接到peer。使用network.WithForceDirectDial强制直接连接
// 这个想法是Swarm的客户端不需要知道连接将通过什么网络进行。
// Swarm可以使用它选择的任何网络。
// 这允许我们使用各种传输协议,进行NAT穿透/中继等来实现连接。
//
// 参数:
//   - ctx: context.Context 上下文对象
//   - p: peer.ID 目标peer标识符
//
// 返回值:
//   - network.Conn 建立的连接
//   - error 可能的错误
func (s *Swarm) DialPeer(ctx context.Context, p peer.ID) (network.Conn, error) {
	// 避免类型化nil问题
	c, err := s.dialPeer(ctx, p)
	if err != nil {
		log.Debugf("拨号失败: %v", err)
		return nil, err
	}
	return c, nil
}

// dialPeer 内部拨号方法,返回未包装的conn
// 它受swarm的拨号同步系统控制:dialsync和dialbackoff。
//
// 参数:
//   - ctx: context.Context 上下文对象
//   - p: peer.ID 目标peer标识符
//
// 返回值:
//   - *Conn 建立的连接
//   - error 可能的错误
//
// 注意:
//   - 它受swarm的拨号同步系统控制:dialsync和dialbackoff
func (s *Swarm) dialPeer(ctx context.Context, p peer.ID) (*Conn, error) {
	log.Debugw("正在拨号对等点", "从", s.local, "到", p)
	err := p.Validate()
	if err != nil {
		log.Debugf("验证peer失败: %v", err)
		return nil, err
	}

	if p == s.local {
		log.Debugf("拨号到自己")
		return nil, ErrDialToSelf
	}

	// 检查我们是否已经有一个打开的(可用的)连接
	conn := s.bestAcceptableConnToPeer(ctx, p)
	if conn != nil {
		return conn, nil
	}

	if s.gater != nil && !s.gater.InterceptPeerDial(p) {
		log.Debugf("gater禁止拨号到peer %s", p)
		return nil, &DialError{Peer: p, Cause: ErrGaterDisallowedConnection}
	}

	// 应用DialPeer超时
	ctx, cancel := context.WithTimeout(ctx, network.GetDialPeerTimeout(ctx))
	defer cancel()

	conn, err = s.dsync.Dial(ctx, p)
	if err == nil {
		// 确保我们连接到正确的peer
		// 这很可能已经由安全协议检查过,但在这里再次检查也无妨
		if conn.RemotePeer() != p {
			conn.Close()
			log.Debugf("握手未能正确认证peer", "已认证", conn.RemotePeer(), "预期", p)
			return nil, fmt.Errorf("意外的peer")
		}
		return conn, nil
	}

	log.Debugf("网络对 %s 完成拨号 %s", s.local, p)

	if ctx.Err() != nil {
		// 上下文错误优先于任何拨号错误,因为它可能是最终原因
		return nil, ctx.Err()
	}

	if s.ctx.Err() != nil {
		// 好的,swarm正在关闭
		return nil, ErrSwarmClosed
	}

	return nil, err
}

// dialWorkerLoop 同步并执行到单个peer的并发拨号
//
// 参数:
//   - p: peer.ID peer标识符
//   - reqch: <-chan dialRequest 拨号请求通道
func (s *Swarm) dialWorkerLoop(p peer.ID, reqch <-chan dialRequest) {
	w := newDialWorker(s, p, reqch, nil)
	w.loop()
}

// addrsForDial 获取用于拨号的地址列表
//
// 参数:
//   - ctx: context.Context 上下文对象
//   - p: peer.ID peer标识符
//
// 返回值:
//   - []ma.Multiaddr 可用的地址列表
//   - []TransportError 地址错误列表
//   - error 可能的错误
func (s *Swarm) addrsForDial(ctx context.Context, p peer.ID) (goodAddrs []ma.Multiaddr, addrErrs []TransportError, err error) {
	peerAddrs := s.peers.Addrs(p)
	if len(peerAddrs) == 0 {
		log.Debugf("没有可用的地址")
		return nil, nil, ErrNoAddresses
	}

	// 解析dns或dnsaddrs
	resolved := s.resolveAddrs(ctx, peer.AddrInfo{ID: p, Addrs: peerAddrs})

	goodAddrs = ma.Unique(resolved)
	goodAddrs, addrErrs = s.filterKnownUndialables(p, goodAddrs)
	if forceDirect, _ := network.GetForceDirectDial(ctx); forceDirect {
		goodAddrs = ma.FilterAddrs(goodAddrs, s.nonProxyAddr)
	}

	if len(goodAddrs) == 0 {
		log.Debugf("没有可用的地址")
		return nil, addrErrs, ErrNoGoodAddresses
	}

	s.peers.AddAddrs(p, goodAddrs, peerstore.TempAddrTTL)

	return goodAddrs, addrErrs, nil
}

// startsWithDNSComponent 检查地址是否以DNS组件开头
//
// 参数:
//   - m: ma.Multiaddr 多地址
//
// 返回值:
//   - bool 是否以DNS组件开头
func startsWithDNSComponent(m ma.Multiaddr) bool {
	if m == nil {
		return false
	}
	startsWithDNS := false
	// 使用ForEach避免分配
	ma.ForEach(m, func(c ma.Component) bool {
		switch c.Protocol().Code {
		case ma.P_DNS, ma.P_DNS4, ma.P_DNS6:
			startsWithDNS = true
		}

		return false
	})
	return startsWithDNS
}

// stripP2PComponent 移除地址中的P2P组件
//
// 参数:
//   - addrs: []ma.Multiaddr 地址列表
//
// 返回值:
//   - []ma.Multiaddr 处理后的地址列表
func stripP2PComponent(addrs []ma.Multiaddr) []ma.Multiaddr {
	for i, addr := range addrs {
		if id, _ := peer.IDFromP2PAddr(addr); id != "" {
			addrs[i], _ = ma.SplitLast(addr)
		}
	}
	return addrs
}

// resolver 定义地址解析器接口
type resolver struct {
	canResolve func(ma.Multiaddr) bool                                                                // 检查是否可以解析地址
	resolve    func(ctx context.Context, maddr ma.Multiaddr, outputLimit int) ([]ma.Multiaddr, error) // 解析地址
}

// resolveErr 定义解析错误
type resolveErr struct {
	addr ma.Multiaddr // 出错的地址
	err  error        // 错误信息
}

// chainResolvers 链式调用多个解析器
//
// 参数:
//   - ctx: context.Context 上下文对象
//   - addrs: []ma.Multiaddr 待解析的地址列表
//   - outputLimit: int 输出限制
//   - resolvers: []resolver 解析器列表
//
// 返回值:
//   - []ma.Multiaddr 解析后的地址列表
//   - []resolveErr 解析错误列表
func chainResolvers(ctx context.Context, addrs []ma.Multiaddr, outputLimit int, resolvers []resolver) ([]ma.Multiaddr, []resolveErr) {
	nextAddrs := make([]ma.Multiaddr, 0, len(addrs))
	errs := make([]resolveErr, 0)
	for _, r := range resolvers {
		for _, a := range addrs {
			if !r.canResolve(a) {
				nextAddrs = append(nextAddrs, a)
				continue
			}
			if len(nextAddrs) >= outputLimit {
				nextAddrs = nextAddrs[:outputLimit]
				break
			}
			next, err := r.resolve(ctx, a, outputLimit-len(nextAddrs))
			if err != nil {
				errs = append(errs, resolveErr{addr: a, err: err})
				continue
			}
			nextAddrs = append(nextAddrs, next...)
		}
		addrs, nextAddrs = nextAddrs, addrs
		nextAddrs = nextAddrs[:0]
	}
	return addrs, errs
}

// resolveAddrs 解析给定peer地址中的DNS/DNSADDR组件
//
// 参数:
//   - ctx: context.Context 上下文对象
//   - pi: peer.AddrInfo peer信息
//
// 返回值:
//   - []ma.Multiaddr 解析后的地址列表
//
// 注意:
//   - 我们想要将DNS组件解析为IP地址,因为我们希望swarm管理多个连接的排名和拨号
//   - 单个DNS地址可以解析为多个IP地址
func (s *Swarm) resolveAddrs(ctx context.Context, pi peer.AddrInfo) []ma.Multiaddr {
	// 定义DNSADDR解析器
	dnsAddrResolver := resolver{
		canResolve: startsWithDNSADDR,
		resolve: func(ctx context.Context, maddr ma.Multiaddr, outputLimit int) ([]ma.Multiaddr, error) {
			return s.multiaddrResolver.ResolveDNSAddr(ctx, pi.ID, maddr, maximumDNSADDRRecursion, outputLimit)
		},
	}

	// 定义跳过解析器
	var skipped []ma.Multiaddr
	skipResolver := resolver{
		canResolve: func(addr ma.Multiaddr) bool {
			tpt := s.TransportForDialing(addr)
			if tpt == nil {
				return false
			}
			_, ok := tpt.(transport.SkipResolver)
			return ok

		},
		resolve: func(ctx context.Context, addr ma.Multiaddr, outputLimit int) ([]ma.Multiaddr, error) {
			tpt := s.TransportForDialing(addr)
			resolver, ok := tpt.(transport.SkipResolver)
			if !ok {
				return []ma.Multiaddr{addr}, nil
			}
			if resolver.SkipResolve(ctx, addr) {
				skipped = append(skipped, addr)
				return nil, nil
			}
			return []ma.Multiaddr{addr}, nil
		},
	}

	// 定义传输解析器
	tptResolver := resolver{
		canResolve: func(addr ma.Multiaddr) bool {
			tpt := s.TransportForDialing(addr)
			if tpt == nil {
				return false
			}
			_, ok := tpt.(transport.Resolver)
			return ok
		},
		resolve: func(ctx context.Context, addr ma.Multiaddr, outputLimit int) ([]ma.Multiaddr, error) {
			tpt := s.TransportForDialing(addr)
			resolver, ok := tpt.(transport.Resolver)
			if !ok {
				return []ma.Multiaddr{addr}, nil
			}
			addrs, err := resolver.Resolve(ctx, addr)
			if err != nil {
				log.Debugf("解析地址失败: %v", err)
				return nil, err
			}
			if len(addrs) > outputLimit {
				addrs = addrs[:outputLimit]
			}
			return addrs, nil
		},
	}

	// 定义DNS解析器
	dnsResolver := resolver{
		canResolve: startsWithDNSComponent,
		resolve:    s.multiaddrResolver.ResolveDNSComponent,
	}

	// 链式调用解析器
	addrs, errs := chainResolvers(ctx, pi.Addrs, maximumResolvedAddresses, []resolver{dnsAddrResolver, skipResolver, tptResolver, dnsResolver})

	// 记录解析错误
	for _, err := range errs {
		log.Warnf("解析地址 %s 失败: %v", err.addr, err.err)
	}

	// 将跳过的地址添加回已解析的地址
	addrs = append(addrs, skipped...)
	return stripP2PComponent(addrs)
}

// dialNextAddr 尝试拨号下一个地址
//
// 参数:
//   - ctx: context.Context 上下文对象
//   - p: peer.ID peer标识符
//   - addr: ma.Multiaddr 要拨号的地址
//   - resch: chan transport.DialUpdate 拨号更新通道
//
// 返回值:
//   - error 拨号过程中的错误
func (s *Swarm) dialNextAddr(ctx context.Context, p peer.ID, addr ma.Multiaddr, resch chan transport.DialUpdate) error {
	// 检查拨号退避
	if forceDirect, _ := network.GetForceDirectDial(ctx); !forceDirect {
		if s.backf.Backoff(p, addr) {
			log.Debugf("拨号退避: %v", ErrDialBackoff)
			return ErrDialBackoff
		}
	}

	// 开始拨号
	s.limitedDial(ctx, p, addr, resch)

	return nil
}

// CanDial 检查是否可以拨号到指定peer和地址
//
// 参数:
//   - p: peer.ID peer标识符
//   - addr: ma.Multiaddr 要检查的地址
//
// 返回值:
//   - bool 是否可以拨号
func (s *Swarm) CanDial(p peer.ID, addr ma.Multiaddr) bool {
	dialable, _ := s.filterKnownUndialables(p, []ma.Multiaddr{addr})
	return len(dialable) > 0
}

// nonProxyAddr 检查地址是否为非代理地址
//
// 参数:
//   - addr: ma.Multiaddr 要检查的地址
//
// 返回值:
//   - bool 是否为非代理地址
func (s *Swarm) nonProxyAddr(addr ma.Multiaddr) bool {
	t := s.TransportForDialing(addr)
	return !t.Proxy()
}

// QUIC draft-29 拨号匹配器
var quicDraft29DialMatcher = mafmt.And(mafmt.IP, mafmt.Base(ma.P_UDP), mafmt.Base(ma.P_QUIC))

// filterKnownUndialables 过滤掉已知无法拨号的地址
//
// 参数:
//   - p: peer.ID peer标识符
//   - addrs: []ma.Multiaddr 待过滤的地址列表
//
// 返回值:
//   - []ma.Multiaddr 可拨号的地址列表
//   - []TransportError 过滤过程中的错误列表
//
// 注意:
//   - 移除那些我们明确不想拨号的地址:
//   - 配置为被阻止的地址
//   - IPv6链路本地地址
//   - 没有可拨号传输的地址
//   - 我们知道是我们自己的地址
//   - 有更好传输可用的地址
//   - 这是一个优化,以避免在我们知道会失败或有更好替代方案的拨号上浪费时间
func (s *Swarm) filterKnownUndialables(p peer.ID, addrs []ma.Multiaddr) (goodAddrs []ma.Multiaddr, addrErrs []TransportError) {
	// 获取本地监听地址
	lisAddrs, _ := s.InterfaceListenAddresses()
	var ourAddrs []ma.Multiaddr
	for _, addr := range lisAddrs {
		// 我们目前只确定过滤/ip4和/ip6地址
		ma.ForEach(addr, func(c ma.Component) bool {
			if c.Protocol().Code == ma.P_IP4 || c.Protocol().Code == ma.P_IP6 {
				ourAddrs = append(ourAddrs, addr)
			}
			return false
		})
	}

	addrErrs = make([]TransportError, 0, len(addrs))

	// 检查传输和过滤低优先级地址的顺序很重要
	// 如果我们只能拨号/webtransport,我们不想因为peer有/quic-v1地址就过滤掉/webtransport地址

	// 过滤没有传输的地址
	addrs = ma.FilterAddrs(addrs, func(a ma.Multiaddr) bool {
		if s.TransportForDialing(a) == nil {
			e := ErrNoTransport
			// 我们长期支持QUIC draft-29
			// 在尝试拨号QUIC draft-29地址时提供更有用的错误
			if quicDraft29DialMatcher.Matches(a) {
				e = ErrQUICDraft29
			}
			addrErrs = append(addrErrs, TransportError{Address: a, Cause: e})
			return false
		}
		return true
	})

	// 在我们可以拨号的地址中过滤低优先级地址 我们不为这些地址返回错误
	addrs = filterLowPriorityAddresses(addrs)

	// 移除黑洞地址
	addrs, blackHoledAddrs := s.bhd.FilterAddrs(addrs)
	for _, a := range blackHoledAddrs {
		addrErrs = append(addrErrs, TransportError{Address: a, Cause: ErrDialRefusedBlackHole})
	}

	return ma.FilterAddrs(addrs,
		// Linux和BSD在拨号时将未指定的地址视为localhost地址
		// Windows不支持这一点。我们过滤掉所有这些地址,因为在未指定地址上监听的peer将广播更具体的地址
		// https://unix.stackexchange.com/a/419881
		// https://superuser.com/a/1755455
		func(addr ma.Multiaddr) bool {
			return !manet.IsIPUnspecified(addr)
		},
		func(addr ma.Multiaddr) bool {
			if ma.Contains(ourAddrs, addr) {
				addrErrs = append(addrErrs, TransportError{Address: addr, Cause: ErrDialToSelf})
				return false
			}
			return true
		},
		// TODO: 考虑允许链路本地地址
		func(addr ma.Multiaddr) bool { return !manet.IsIP6LinkLocal(addr) },
		func(addr ma.Multiaddr) bool {
			if s.gater != nil && !s.gater.InterceptAddrDial(p, addr) {
				addrErrs = append(addrErrs, TransportError{Address: addr, Cause: ErrGaterDisallowedConnection})
				return false
			}
			return true
		},
	), addrErrs
}

// limitedDial 在可能时开始拨号给定的peer
//
// 参数:
//   - ctx: context.Context 上下文对象
//   - p: peer.ID peer标识符
//   - a: ma.Multiaddr 要拨号的地址
//   - resp: chan transport.DialUpdate 拨号更新通道
//
// 注意:
//   - 尊重各种不同类型的速率限制
//   - 不使用每个addr的额外goroutine
func (s *Swarm) limitedDial(ctx context.Context, p peer.ID, a ma.Multiaddr, resp chan transport.DialUpdate) {
	timeout := s.dialTimeout
	if manet.IsPrivateAddr(a) && s.dialTimeoutLocal < s.dialTimeout {
		timeout = s.dialTimeoutLocal
	}
	s.limiter.AddDialJob(&dialJob{
		addr:    a,
		peer:    p,
		resp:    resp,
		ctx:     ctx,
		timeout: timeout,
	})
}

// dialAddr 执行实际的地址拨号
//
// 参数:
//   - ctx: context.Context 上下文对象
//   - p: peer.ID peer标识符
//   - addr: ma.Multiaddr 要拨号的地址
//   - updCh: chan<- transport.DialUpdate 拨号更新通道
//
// 返回值:
//   - transport.CapableConn 建立的连接
//   - error 拨号过程中的错误
//
// 注意:
//   - 通过限制器间接调用
func (s *Swarm) dialAddr(ctx context.Context, p peer.ID, addr ma.Multiaddr, updCh chan<- transport.DialUpdate) (transport.CapableConn, error) {
	// 仅作双重检查。不花费任何代价。
	if s.local == p {
		log.Debugf("拨号到自己")
		return nil, ErrDialToSelf
	}
	// 在我们开始工作之前检查
	if err := ctx.Err(); err != nil {
		log.Debugf("%s swarm不拨号。上下文已取消: %v。%s %s", s.local, err, p, addr)
		return nil, err
	}
	log.Debugf("%s swarm正在拨号 %s %s", s.local, p, addr)

	tpt := s.TransportForDialing(addr)
	if tpt == nil {
		log.Debugf("没有可用的传输")
		return nil, ErrNoTransport
	}

	start := time.Now()
	var connC transport.CapableConn
	var err error
	if du, ok := tpt.(transport.DialUpdater); ok {
		connC, err = du.DialWithUpdates(ctx, addr, p, updCh)
	} else {
		connC, err = tpt.Dial(ctx, addr, p)
	}

	// 我们在这里将任何错误记录为失败
	// 值得注意的是,这也适用于取消(即如果另一个拨号尝试更快)
	// 这没问题,因为黑洞检测器使用非常低的阈值(5%)
	s.bhd.RecordResult(addr, err == nil)

	if err != nil {
		if s.metricsTracer != nil {
			s.metricsTracer.FailedDialing(addr, err, context.Cause(ctx))
		}
		log.Debugf("拨号失败: %v", err)
		return nil, err
	}
	canonicallog.LogPeerStatus(100, connC.RemotePeer(), connC.RemoteMultiaddr(), "connection_status", "established", "dir", "outbound")
	if s.metricsTracer != nil {
		connWithMetrics := wrapWithMetrics(connC, s.metricsTracer, start, network.DirOutbound)
		connWithMetrics.completedHandshake()
		connC = connWithMetrics
	}

	// 相信传输?是的...对的。
	if connC.RemotePeer() != p {
		connC.Close()
		err = fmt.Errorf("传输%T中的BUG:尝试拨号%s,拨号到%s", tpt, p, connC.RemotePeer())
		log.Debugf(err.Error())
		return nil, err
	}

	// 成功!我们得到一个!
	return connC, nil
}

// TODO 我们应该在dep2p/core/transport.Transport接口上有一个`IsFdConsuming() bool`方法。
// isFdConsumingAddr 检查地址是否消耗文件描述符
//
// 参数:
//   - addr: ma.Multiaddr 要检查的地址
//
// 返回值:
//   - bool 是否消耗文件描述符
//
// 注意:
//   - 具有TCP/UNIX协议的非电路地址被认为是消耗FD的
//   - 对于电路中继地址,我们查看中继服务器/代理的地址,并使用上述相同的逻辑来决定
func isFdConsumingAddr(addr ma.Multiaddr) bool {
	first, _ := ma.SplitFunc(addr, func(c ma.Component) bool {
		return c.Protocol().Code == ma.P_CIRCUIT
	})

	// 为了安全
	if first == nil {
		return true
	}

	_, err1 := first.ValueForProtocol(ma.P_TCP)
	_, err2 := first.ValueForProtocol(ma.P_UNIX)
	return err1 == nil || err2 == nil
}

// isRelayAddr 检查地址是否为中继地址
//
// 参数:
//   - addr: ma.Multiaddr 要检查的地址
//
// 返回值:
//   - bool 是否为中继地址
func isRelayAddr(addr ma.Multiaddr) bool {
	_, err := addr.ValueForProtocol(ma.P_CIRCUIT)
	return err == nil
}

// filterLowPriorityAddresses 移除我们有更好替代方案的地址
//
// 参数:
//   - addrs: []ma.Multiaddr 要过滤的地址列表
//
// 返回值:
//   - []ma.Multiaddr 过滤后的地址列表
//
// 注意:
//  1. 如果存在/quic-v1地址,过滤掉相同2元组上的/quic和/webtransport地址:
//     QUIC v1优先于已弃用的QUIC draft-29,在有选择的情况下,我们更喜欢使用原始QUIC而不是使用WebTransport
//  2. 如果存在/tcp地址,过滤掉相同2元组上的/ws或/wss地址:
//     我们更喜欢使用原始TCP而不是使用WebSocket
func filterLowPriorityAddresses(addrs []ma.Multiaddr) []ma.Multiaddr {
	// 制作QUIC v1和TCP AddrPorts的映射
	quicV1Addr := make(map[netip.AddrPort]struct{})
	tcpAddr := make(map[netip.AddrPort]struct{})
	for _, a := range addrs {
		switch {
		case isProtocolAddr(a, ma.P_WEBTRANSPORT):
		case isProtocolAddr(a, ma.P_QUIC_V1):
			ap, err := addrPort(a, ma.P_UDP)
			if err != nil {
				log.Debugf("解析地址失败: %v", err)
				continue
			}
			quicV1Addr[ap] = struct{}{}
		case isProtocolAddr(a, ma.P_WS) || isProtocolAddr(a, ma.P_WSS):
		case isProtocolAddr(a, ma.P_TCP):
			ap, err := addrPort(a, ma.P_TCP)
			if err != nil {
				log.Debugf("解析地址失败: %v", err)
				continue
			}
			tcpAddr[ap] = struct{}{}
		}
	}

	i := 0
	for _, a := range addrs {
		switch {
		case isProtocolAddr(a, ma.P_WEBTRANSPORT) || isProtocolAddr(a, ma.P_QUIC):
			ap, err := addrPort(a, ma.P_UDP)
			if err != nil {
				log.Debugf("解析地址失败: %v", err)
				break
			}
			if _, ok := quicV1Addr[ap]; ok {
				continue
			}
		case isProtocolAddr(a, ma.P_WS) || isProtocolAddr(a, ma.P_WSS):
			ap, err := addrPort(a, ma.P_TCP)
			if err != nil {
				log.Debugf("解析地址失败: %v", err)
				break
			}
			if _, ok := tcpAddr[ap]; ok {
				continue
			}
		}
		addrs[i] = a
		i++
	}
	return addrs[:i]
}

// addrPort 返回地址的IP和端口
//
// 参数:
//   - a: ma.Multiaddr 多地址
//   - p: int 协议(应为ma.P_TCP或ma.P_UDP)
//
// 返回值:
//   - netip.AddrPort IP地址和端口
//   - error 可能的错误
//
// 注意:
//   - a必须是(ip,TCP)或(ip,UDP)地址
func addrPort(a ma.Multiaddr, p int) (netip.AddrPort, error) {
	ip, err := manet.ToIP(a)
	if err != nil {
		log.Debugf("解析地址失败: %v", err)
		return netip.AddrPort{}, err
	}
	port, err := a.ValueForProtocol(p)
	if err != nil {
		log.Debugf("解析地址失败: %v", err)
		return netip.AddrPort{}, err
	}
	pi, err := strconv.Atoi(port)
	if err != nil {
		log.Debugf("解析地址失败: %v", err)
		return netip.AddrPort{}, err
	}
	addr, ok := netip.AddrFromSlice(ip)
	if !ok {
		log.Debugf("解析IP %s 失败", ip)
		return netip.AddrPort{}, fmt.Errorf("解析IP %s 失败", ip)
	}
	return netip.AddrPortFrom(addr, uint16(pi)), nil
}
