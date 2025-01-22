package identify

import (
	"context"
	"fmt"
	"net"
	"slices"
	"sort"
	"sync"

	"github.com/dep2p/go-dep2p/core/network"

	ma "github.com/dep2p/go-dep2p/multiformats/multiaddr"
	manet "github.com/dep2p/go-dep2p/multiformats/multiaddr/net"
)

// ActivationThresh 设置一个地址必须被观察到多少次才能被"激活"，并作为本地节点可被联系的地址广播给其他节点
// 这些"观察"事件默认在40分钟后过期(OwnObservedAddressTTL * ActivationThreshold)
// 它们会在 GCInterval 设置的 GC 轮次中被清理
var ActivationThresh = 4

// observedAddrManagerWorkerChannelSize 定义可以排队添加到 ObservedAddrManager 的地址数量
var observedAddrManagerWorkerChannelSize = 16

// maxExternalThinWaistAddrsPerLocalAddr 定义每个本地地址最多可以有多少个外部瘦腰地址
const maxExternalThinWaistAddrsPerLocalAddr = 3

// thinWaist 是一个存储地址及其瘦腰前缀和剩余 multiaddr 的结构体
type thinWaist struct {
	Addr, TW, Rest ma.Multiaddr
}

// thinWaistWithCount 是一个 thinWaist 结构体，同时包含将其作为本地地址的连接数量
type thinWaistWithCount struct {
	thinWaist
	Count int
}

// thinWaistForm 将一个 multiaddr 转换为瘦腰形式
// 参数:
//   - a: ma.Multiaddr 要转换的 multiaddr 地址
//
// 返回值:
//   - thinWaist: 转换后的瘦腰形式地址
//   - error: 转换过程中的错误
//
// 注意:
//   - 如果地址不是瘦腰形式(不包含 IP 和 TCP/UDP)，将返回错误
func thinWaistForm(a ma.Multiaddr) (thinWaist, error) {
	i := 0
	tw, rest := ma.SplitFunc(a, func(c ma.Component) bool {
		if i > 1 {
			return true
		}
		switch i {
		case 0:
			if c.Protocol().Code == ma.P_IP4 || c.Protocol().Code == ma.P_IP6 {
				i++
				return false
			}
			return true
		case 1:
			if c.Protocol().Code == ma.P_TCP || c.Protocol().Code == ma.P_UDP {
				i++
				return false
			}
			return true
		}
		return false
	})
	if i <= 1 {
		return thinWaist{}, fmt.Errorf("不是瘦腰地址: %s", a)
	}
	return thinWaist{Addr: a, TW: tw, Rest: rest}, nil
}

// getObserver 返回 multiaddr 的观察者
// 对于 IPv4 multiaddr，观察者是 IP 地址
// 对于 IPv6 multiaddr，观察者是 IP 地址的第一个 /56 前缀
//
// 参数:
//   - a: ma.Multiaddr 要获取观察者的 multiaddr 地址
//
// 返回值:
//   - string: 观察者的字符串表示
//   - error: 获取过程中的错误
func getObserver(a ma.Multiaddr) (string, error) {
	ip, err := manet.ToIP(a)
	if err != nil {
		log.Debugf("获取观察者失败: %s", err)
		return "", err
	}
	if ip4 := ip.To4(); ip4 != nil {
		return ip4.String(), nil
	}
	// 将 /56 前缀视为单个观察者
	return ip.Mask(net.CIDRMask(56, 128)).String(), nil
}

// connMultiaddrs 提供 IsClosed 和 network.ConnMultiaddrs 接口
// 比直接使用 network.Conn 更容易模拟
type connMultiaddrs interface {
	network.ConnMultiaddrs
	IsClosed() bool
}

// observerSetCacheSize 是共享相同瘦腰的传输数量(tcp, ws, wss), (quic, webtransport, webrtc-direct)
// 目前实际上是3，但保留3个额外的缓冲元素
const observerSetCacheSize = 5

// observerSet 是观察到 ThinWaistAddr 的观察者集合
type observerSet struct {
	ObservedTWAddr ma.Multiaddr
	ObservedBy     map[string]int

	mu               sync.RWMutex            // 保护以下字段
	cachedMultiaddrs map[string]ma.Multiaddr // 本地 Multiaddr rest(addr - thinwaist) => 输出 multiaddr 的缓存
}

// cacheMultiaddr 缓存并返回完整的 multiaddr
// 参数:
//   - addr: ma.Multiaddr 要缓存的地址
//
// 返回值:
//   - ma.Multiaddr: 缓存的完整 multiaddr
func (s *observerSet) cacheMultiaddr(addr ma.Multiaddr) ma.Multiaddr {
	if addr == nil {
		return s.ObservedTWAddr
	}
	addrStr := string(addr.Bytes())
	s.mu.RLock()
	res, ok := s.cachedMultiaddrs[addrStr]
	s.mu.RUnlock()
	if ok {
		return res
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	// 检查其他 goroutine 是否在等待时添加了此地址
	res, ok = s.cachedMultiaddrs[addrStr]
	if ok {
		return res
	}
	if s.cachedMultiaddrs == nil {
		s.cachedMultiaddrs = make(map[string]ma.Multiaddr, observerSetCacheSize)
	}
	if len(s.cachedMultiaddrs) == observerSetCacheSize {
		// 如果超过限制，删除一个条目
		for k := range s.cachedMultiaddrs {
			delete(s.cachedMultiaddrs, k)
			break
		}
	}
	s.cachedMultiaddrs[addrStr] = ma.Join(s.ObservedTWAddr, addr)
	return s.cachedMultiaddrs[addrStr]
}

// observation 表示一次地址观察
type observation struct {
	conn     connMultiaddrs
	observed ma.Multiaddr
}

// ObservedAddrManager 将连接的本地 multiaddr 映射到它们的外部可观察 multiaddr
type ObservedAddrManager struct {
	// 我们的监听地址
	listenAddrs func() []ma.Multiaddr
	// 我们的监听地址，包含未指定地址的接口地址
	interfaceListenAddrs func() ([]ma.Multiaddr, error)
	// 所有主机地址
	hostAddrs func() []ma.Multiaddr
	// 比较前需要的任何规范化。用于移除 certhash
	normalize func(ma.Multiaddr) ma.Multiaddr
	// 新观察的工作通道
	wch chan observation
	// 记录观察时通知
	addrRecordedNotif chan struct{}

	// 用于关闭
	wg        sync.WaitGroup
	ctx       context.Context
	ctxCancel context.CancelFunc

	mu sync.RWMutex
	// 本地瘦腰 => 外部瘦腰 => observerSet
	externalAddrs map[string]map[string]*observerSet
	// connObservedTWAddrs 将连接映射到该连接上最后观察到的瘦腰 multiaddr
	connObservedTWAddrs map[connMultiaddrs]ma.Multiaddr
	// localMultiaddr => 带有连接计数的瘦腰形式，用于跟踪我们的本地监听地址
	localAddrs map[string]*thinWaistWithCount
}

// NewObservedAddrManager 返回一个新的地址管理器，使用 peerstore.OwnObservedAddressTTL 作为 TTL
// 参数:
//   - listenAddrs: func() []ma.Multiaddr 返回监听地址的函数
//   - hostAddrs: func() []ma.Multiaddr 返回主机地址的函数
//   - interfaceListenAddrs: func() ([]ma.Multiaddr, error) 返回接口监听地址的函数
//   - normalize: func(ma.Multiaddr) ma.Multiaddr 地址规范化函数
//
// 返回值:
//   - *ObservedAddrManager: 新创建的地址管理器
//   - error: 创建过程中的错误
func NewObservedAddrManager(listenAddrs, hostAddrs func() []ma.Multiaddr,
	interfaceListenAddrs func() ([]ma.Multiaddr, error), normalize func(ma.Multiaddr) ma.Multiaddr) (*ObservedAddrManager, error) {
	if normalize == nil {
		normalize = func(addr ma.Multiaddr) ma.Multiaddr { return addr }
	}
	o := &ObservedAddrManager{
		externalAddrs:        make(map[string]map[string]*observerSet),
		connObservedTWAddrs:  make(map[connMultiaddrs]ma.Multiaddr),
		localAddrs:           make(map[string]*thinWaistWithCount),
		wch:                  make(chan observation, observedAddrManagerWorkerChannelSize),
		addrRecordedNotif:    make(chan struct{}, 1),
		listenAddrs:          listenAddrs,
		interfaceListenAddrs: interfaceListenAddrs,
		hostAddrs:            hostAddrs,
		normalize:            normalize,
	}
	o.ctx, o.ctxCancel = context.WithCancel(context.Background())

	o.wg.Add(1)
	go o.worker()
	return o, nil
}

// AddrsFor 返回与给定(已解析的)监听地址关联的所有已激活的观察地址
// 参数:
//   - addr: ma.Multiaddr 监听地址
//
// 返回值:
//   - []ma.Multiaddr: 已激活的观察地址列表
func (o *ObservedAddrManager) AddrsFor(addr ma.Multiaddr) (addrs []ma.Multiaddr) {
	if addr == nil {
		return nil
	}
	o.mu.RLock()
	defer o.mu.RUnlock()
	tw, err := thinWaistForm(o.normalize(addr))
	if err != nil {
		return nil
	}

	observerSets := o.getTopExternalAddrs(string(tw.TW.Bytes()))
	res := make([]ma.Multiaddr, 0, len(observerSets))
	for _, s := range observerSets {
		res = append(res, s.cacheMultiaddr(tw.Rest))
	}
	return res
}

// appendInferredAddrs 推断与已有观察传输共享本地瘦腰的其他传输的外部地址
//
// 例如：如果我们对端口 9000 的 QUIC 地址有观察，并且我们在相同接口和端口 9000 上监听 WebTransport，
// 我们可以推断出外部 WebTransport 地址
//
// 参数:
//   - twToObserverSets: map[string][]*observerSet 瘦腰到观察者集合的映射
//   - addrs: []ma.Multiaddr 要追加到的地址列表
//
// 返回值:
//   - []ma.Multiaddr: 追加了推断地址后的地址列表
func (o *ObservedAddrManager) appendInferredAddrs(twToObserverSets map[string][]*observerSet, addrs []ma.Multiaddr) []ma.Multiaddr {
	if twToObserverSets == nil {
		twToObserverSets = make(map[string][]*observerSet)
		for localTWStr := range o.externalAddrs {
			twToObserverSets[localTWStr] = append(twToObserverSets[localTWStr], o.getTopExternalAddrs(localTWStr)...)
		}
	}
	lAddrs, err := o.interfaceListenAddrs()
	if err != nil {
		log.Warnw("获取接口解析的监听地址失败。仅使用监听地址", "error", err)
		lAddrs = nil
	}
	lAddrs = append(lAddrs, o.listenAddrs()...)
	seenTWs := make(map[string]struct{})
	for _, a := range lAddrs {
		if _, ok := o.localAddrs[string(a.Bytes())]; ok {
			// 我们已经在列表中有这个地址
			continue
		}
		if _, ok := seenTWs[string(a.Bytes())]; ok {
			// 我们已经添加过这个
			continue
		}
		seenTWs[string(a.Bytes())] = struct{}{}
		a = o.normalize(a)
		t, err := thinWaistForm(a)
		if err != nil {
			continue
		}
		for _, s := range twToObserverSets[string(t.TW.Bytes())] {
			addrs = append(addrs, s.cacheMultiaddr(t.Rest))
		}
	}
	return addrs
}

// Addrs 返回所有已激活的观察地址
// 返回值:
//   - []ma.Multiaddr: 所有已激活的观察地址列表
func (o *ObservedAddrManager) Addrs() []ma.Multiaddr {
	o.mu.RLock()
	defer o.mu.RUnlock()

	m := make(map[string][]*observerSet)
	for localTWStr := range o.externalAddrs {
		m[localTWStr] = append(m[localTWStr], o.getTopExternalAddrs(localTWStr)...)
	}
	addrs := make([]ma.Multiaddr, 0, maxExternalThinWaistAddrsPerLocalAddr*5) // 假设5个传输
	for _, t := range o.localAddrs {
		for _, s := range m[string(t.TW.Bytes())] {
			addrs = append(addrs, s.cacheMultiaddr(t.Rest))
		}
	}

	addrs = o.appendInferredAddrs(m, addrs)
	return addrs
}

// getTopExternalAddrs 获取给定本地瘦腰地址的顶部外部地址
// 参数:
//   - localTWStr: string 本地瘦腰地址的字符串表示
//
// 返回值:
//   - []*observerSet: 顶部外部地址的观察者集合列表
func (o *ObservedAddrManager) getTopExternalAddrs(localTWStr string) []*observerSet {
	observerSets := make([]*observerSet, 0, len(o.externalAddrs[localTWStr]))
	for _, v := range o.externalAddrs[localTWStr] {
		if len(v.ObservedBy) >= ActivationThresh {
			observerSets = append(observerSets, v)
		}
	}
	slices.SortFunc(observerSets, func(a, b *observerSet) int {
		diff := len(b.ObservedBy) - len(a.ObservedBy)
		if diff != 0 {
			return diff
		}
		// 如果有相同计数的元素，通过使用字典序较小的地址来保持地址列表稳定
		as := a.ObservedTWAddr.String()
		bs := b.ObservedTWAddr.String()
		if as < bs {
			return -1
		} else if as > bs {
			return 1
		} else {
			return 0
		}

	})
	n := len(observerSets)
	if n > maxExternalThinWaistAddrsPerLocalAddr {
		n = maxExternalThinWaistAddrsPerLocalAddr
	}
	return observerSets[:n]
}

// Record 将观察排队以进行记录
// 参数:
//   - conn: connMultiaddrs 连接对象
//   - observed: ma.Multiaddr 观察到的地址
func (o *ObservedAddrManager) Record(conn connMultiaddrs, observed ma.Multiaddr) {
	select {
	case o.wch <- observation{
		conn:     conn,
		observed: observed,
	}:
	default:
		log.Debugw("由于缓冲区已满而丢弃地址观察",
			"from", conn.RemoteMultiaddr(),
			"observed", observed,
		)
	}
}

// worker 是处理观察记录的工作协程
func (o *ObservedAddrManager) worker() {
	defer o.wg.Done()

	for {
		select {
		case obs := <-o.wch:
			o.maybeRecordObservation(obs.conn, obs.observed)
		case <-o.ctx.Done():
			return
		}
	}
}

// isRelayedAddress 检查地址是否是中继地址
// 参数:
//   - a: ma.Multiaddr 要检查的地址
//
// 返回值:
//   - bool: 如果是中继地址返回 true，否则返回 false
func isRelayedAddress(a ma.Multiaddr) bool {
	_, err := a.ValueForProtocol(ma.P_CIRCUIT)
	return err == nil
}

// shouldRecordObservation 检查是否应该记录观察
// 参数:
//   - conn: connMultiaddrs 连接对象
//   - observed: ma.Multiaddr 观察到的地址
//
// 返回值:
//   - bool: 是否应该记录
//   - thinWaist: 本地瘦腰地址
//   - thinWaist: 观察到的瘦腰地址
func (o *ObservedAddrManager) shouldRecordObservation(conn connMultiaddrs, observed ma.Multiaddr) (shouldRecord bool, localTW thinWaist, observedTW thinWaist) {
	if conn == nil || observed == nil {
		return false, thinWaist{}, thinWaist{}
	}
	// 忽略来自回环节点的观察。我们已经知道我们的回环地址。
	if manet.IsIPLoopback(observed) {
		return false, thinWaist{}, thinWaist{}
	}

	// 由 NAT64 对等点提供，这些地址特定于对等点且不可公开路由
	if manet.IsNAT64IPv4ConvertedIPv6Addr(observed) {
		return false, thinWaist{}, thinWaist{}
	}

	// 忽略 p2p-circuit 地址。这些是中继的观察地址。
	// 对我们没有用。
	if isRelayedAddress(observed) {
		return false, thinWaist{}, thinWaist{}
	}

	// 我们只应在连接的 LocalAddr 是我们的 ListenAddrs 之一时使用 ObservedAddr。
	// 如果我们使用临时地址拨出，知道该地址的外部映射并不是很有用，因为端口与监听地址不同。
	ifaceaddrs, err := o.interfaceListenAddrs()
	if err != nil {
		log.Debugf("获取接口监听地址失败: %s", err)
		return false, thinWaist{}, thinWaist{}
	}

	for i, a := range ifaceaddrs {
		ifaceaddrs[i] = o.normalize(a)
	}

	local := o.normalize(conn.LocalMultiaddr())

	listenAddrs := o.listenAddrs()
	for i, a := range listenAddrs {
		listenAddrs[i] = o.normalize(a)
	}

	if !ma.Contains(ifaceaddrs, local) && !ma.Contains(listenAddrs, local) {
		// 不在我们的列表中
		return false, thinWaist{}, thinWaist{}
	}

	localTW, err = thinWaistForm(local)
	if err != nil {
		return false, thinWaist{}, thinWaist{}
	}
	observedTW, err = thinWaistForm(o.normalize(observed))
	if err != nil {
		return false, thinWaist{}, thinWaist{}
	}

	hostAddrs := o.hostAddrs()
	for i, a := range hostAddrs {
		hostAddrs[i] = o.normalize(a)
	}

	// 如果观察不匹配我们公布的地址的传输，我们应该拒绝连接
	if !HasConsistentTransport(observed, hostAddrs) &&
		!HasConsistentTransport(observed, listenAddrs) {
		log.Debugw(
			"观察到的 multiaddr 与任何已宣布地址的传输不匹配",
			"from", conn.RemoteMultiaddr(),
			"observed", observed,
		)
		return false, thinWaist{}, thinWaist{}
	}

	return true, localTW, observedTW
}

// maybeRecordObservation 可能记录一个观察
// 参数:
//   - conn: connMultiaddrs 连接对象
//   - observed: ma.Multiaddr 观察到的地址
func (o *ObservedAddrManager) maybeRecordObservation(conn connMultiaddrs, observed ma.Multiaddr) {
	shouldRecord, localTW, observedTW := o.shouldRecordObservation(conn, observed)
	if !shouldRecord {
		return
	}
	log.Debugw("添加了自己观察到的监听地址", "conn", conn, "observed", observed)

	o.mu.Lock()
	defer o.mu.Unlock()
	o.recordObservationUnlocked(conn, localTW, observedTW)
	select {
	case o.addrRecordedNotif <- struct{}{}:
	default:
	}
}

// recordObservationUnlocked 在不加锁的情况下记录观察
// 参数:
//   - conn: connMultiaddrs 连接对象
//   - localTW: thinWaist 本地瘦腰地址
//   - observedTW: thinWaist 观察到的瘦腰地址
func (o *ObservedAddrManager) recordObservationUnlocked(conn connMultiaddrs, localTW, observedTW thinWaist) {
	if conn.IsClosed() {
		// 如果连接已经关闭，不要记录。任何先前的观察都将在断开连接的回调中被删除
		return
	}
	localTWStr := string(localTW.TW.Bytes())
	observedTWStr := string(observedTW.TW.Bytes())
	observer, err := getObserver(conn.RemoteMultiaddr())
	if err != nil {
		return
	}

	prevObservedTWAddr, ok := o.connObservedTWAddrs[conn]
	if !ok {
		t, ok := o.localAddrs[string(localTW.Addr.Bytes())]
		if !ok {
			t = &thinWaistWithCount{
				thinWaist: localTW,
			}
			o.localAddrs[string(localTW.Addr.Bytes())] = t
		}
		t.Count++
	} else {
		if prevObservedTWAddr.Equal(observedTW.TW) {
			// 我们再次收到相同的观察，无需操作
			return
		}
		// 如果我们有先前的条目，从 externalAddrs 中删除它
		o.removeExternalAddrsUnlocked(observer, localTWStr, string(prevObservedTWAddr.Bytes()))
		// 这里不需要更改 localAddrs 映射
	}
	o.connObservedTWAddrs[conn] = observedTW.TW
	o.addExternalAddrsUnlocked(observedTW.TW, observer, localTWStr, observedTWStr)
}

// removeExternalAddrsUnlocked 在不加锁的情况下删除外部地址
// 参数:
//   - observer: string 观察者
//   - localTWStr: string 本地瘦腰地址字符串
//   - observedTWStr: string 观察到的瘦腰地址字符串
func (o *ObservedAddrManager) removeExternalAddrsUnlocked(observer, localTWStr, observedTWStr string) {
	s, ok := o.externalAddrs[localTWStr][observedTWStr]
	if !ok {
		return
	}
	s.ObservedBy[observer]--
	if s.ObservedBy[observer] <= 0 {
		delete(s.ObservedBy, observer)
	}
	if len(s.ObservedBy) == 0 {
		delete(o.externalAddrs[localTWStr], observedTWStr)
	}
	if len(o.externalAddrs[localTWStr]) == 0 {
		delete(o.externalAddrs, localTWStr)
	}
}

// addExternalAddrsUnlocked 在不加锁的情况下添加外部地址
// 参数:
//   - observedTWAddr: ma.Multiaddr 观察到的瘦腰地址
//   - observer: string 观察者
//   - localTWStr: string 本地瘦腰地址字符串
//   - observedTWStr: string 观察到的瘦腰地址字符串
func (o *ObservedAddrManager) addExternalAddrsUnlocked(observedTWAddr ma.Multiaddr, observer, localTWStr, observedTWStr string) {
	s, ok := o.externalAddrs[localTWStr][observedTWStr]
	if !ok {
		s = &observerSet{
			ObservedTWAddr: observedTWAddr,
			ObservedBy:     make(map[string]int),
		}
		if _, ok := o.externalAddrs[localTWStr]; !ok {
			o.externalAddrs[localTWStr] = make(map[string]*observerSet)
		}
		o.externalAddrs[localTWStr][observedTWStr] = s
	}
	s.ObservedBy[observer]++
}

// removeConn 移除连接
// 参数:
//   - conn: connMultiaddrs 要移除的连接
func (o *ObservedAddrManager) removeConn(conn connMultiaddrs) {
	if conn == nil {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()

	observedTWAddr, ok := o.connObservedTWAddrs[conn]
	if !ok {
		return
	}
	delete(o.connObservedTWAddrs, conn)

	// 在获取瘦腰之前进行规范化，这样我们始终处理地址的规范化形式
	localTW, err := thinWaistForm(o.normalize(conn.LocalMultiaddr()))
	if err != nil {
		return
	}
	t, ok := o.localAddrs[string(localTW.Addr.Bytes())]
	if !ok {
		return
	}
	t.Count--
	if t.Count <= 0 {
		delete(o.localAddrs, string(localTW.Addr.Bytes()))
	}

	observer, err := getObserver(conn.RemoteMultiaddr())
	if err != nil {
		return
	}

	o.removeExternalAddrsUnlocked(observer, string(localTW.TW.Bytes()), string(observedTWAddr.Bytes()))
	select {
	case o.addrRecordedNotif <- struct{}{}:
	default:
	}
}

// getNATType 获取 NAT 类型
// 返回值:
//   - network.NATDeviceType: TCP NAT 类型
//   - network.NATDeviceType: UDP NAT 类型
func (o *ObservedAddrManager) getNATType() (tcpNATType, udpNATType network.NATDeviceType) {
	o.mu.RLock()
	defer o.mu.RUnlock()

	var tcpCounts, udpCounts []int
	var tcpTotal, udpTotal int
	for _, m := range o.externalAddrs {
		isTCP := false
		for _, v := range m {
			if _, err := v.ObservedTWAddr.ValueForProtocol(ma.P_TCP); err == nil {
				isTCP = true
			}
			break
		}
		for _, v := range m {
			if isTCP {
				tcpCounts = append(tcpCounts, len(v.ObservedBy))
				tcpTotal += len(v.ObservedBy)
			} else {
				udpCounts = append(udpCounts, len(v.ObservedBy))
				udpTotal += len(v.ObservedBy)
			}
		}
	}

	sort.Sort(sort.Reverse(sort.IntSlice(tcpCounts)))
	sort.Sort(sort.Reverse(sort.IntSlice(udpCounts)))

	tcpTopCounts, udpTopCounts := 0, 0
	for i := 0; i < maxExternalThinWaistAddrsPerLocalAddr && i < len(tcpCounts); i++ {
		tcpTopCounts += tcpCounts[i]
	}
	for i := 0; i < maxExternalThinWaistAddrsPerLocalAddr && i < len(udpCounts); i++ {
		udpTopCounts += udpCounts[i]
	}

	// If the top elements cover more than 1/2 of all the observations, there's a > 50% chance that hole punching based on outputs of observed address manager will succeed
	if tcpTotal >= 3*maxExternalThinWaistAddrsPerLocalAddr {
		if tcpTopCounts >= tcpTotal/2 {
			tcpNATType = network.NATDeviceTypeCone
		} else {
			tcpNATType = network.NATDeviceTypeSymmetric
		}
	}
	if udpTotal >= 3*maxExternalThinWaistAddrsPerLocalAddr {
		if udpTopCounts >= udpTotal/2 {
			udpNATType = network.NATDeviceTypeCone
		} else {
			udpNATType = network.NATDeviceTypeSymmetric
		}
	}
	return
}

func (o *ObservedAddrManager) Close() error {
	o.ctxCancel()
	o.wg.Wait()
	return nil
}
