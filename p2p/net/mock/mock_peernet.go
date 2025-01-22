package mocknet

import (
	"bytes"
	"context"
	"fmt"
	"math/rand"
	"sync"

	"github.com/dep2p/go-dep2p/core/connmgr"
	"github.com/dep2p/go-dep2p/core/event"
	"github.com/dep2p/go-dep2p/core/network"
	"github.com/dep2p/go-dep2p/core/peer"
	"github.com/dep2p/go-dep2p/core/peerstore"
	ma "github.com/dep2p/go-dep2p/multiformats/multiaddr"
)

// peernet 实现了 network.Network 接口
type peernet struct {
	mocknet *mocknet // 父节点

	peer    peer.ID             // 节点ID
	ps      peerstore.Peerstore // 节点存储
	emitter event.Emitter       // 事件发射器

	// conns 是节点间的实际活跃连接
	// 每个链路上可以有多个连接
	// **连接不在节点间共享**
	connsByPeer map[peer.ID]map[*conn]struct{} // 按节点ID索引的连接
	connsByLink map[*link]map[*conn]struct{}   // 按链路索引的连接

	// 连接过滤器,用于检查拨号或接受连接前的状态。可以为 nil 表示允许所有连接
	gater connmgr.ConnectionGater

	// 实现 network.Network 接口
	streamHandler network.StreamHandler // 流处理器

	notifmu sync.Mutex                    // 通知锁
	notifs  map[network.Notifiee]struct{} // 通知处理器映射

	sync.RWMutex // 读写锁
}

// newPeernet 构造一个新的 peernet
// 参数:
//   - m: *mocknet mock网络对象
//   - p: peer.ID 节点ID
//   - opts: PeerOptions 节点配置选项
//   - bus: event.Bus 事件总线
//
// 返回值:
//   - *peernet: 新创建的peernet对象
//   - error: 错误信息
func newPeernet(m *mocknet, p peer.ID, opts PeerOptions, bus event.Bus) (*peernet, error) {
	// 创建事件发射器
	emitter, err := bus.Emitter(&event.EvtPeerConnectednessChanged{})
	if err != nil {
		log.Debugf("创建事件发射器失败: %v", err)
		return nil, err
	}

	// 创建并初始化peernet对象
	n := &peernet{
		mocknet: m,
		peer:    p,
		ps:      opts.ps,
		gater:   opts.gater,
		emitter: emitter,

		connsByPeer: map[peer.ID]map[*conn]struct{}{},
		connsByLink: map[*link]map[*conn]struct{}{},

		notifs: make(map[network.Notifiee]struct{}),
	}

	return n, nil
}

// Close 关闭peernet
// 返回值:
//   - error: 关闭过程中的错误信息
func (pn *peernet) Close() error {
	// 关闭所有连接
	for _, c := range pn.allConns() {
		c.Close()
	}
	pn.emitter.Close()
	return pn.ps.Close()
}

// allConns 返回此节点与其他节点之间的所有连接
// 返回值:
//   - []*conn: 连接数组
func (pn *peernet) allConns() []*conn {
	pn.RLock()
	var cs []*conn
	for _, csl := range pn.connsByPeer {
		for c := range csl {
			cs = append(cs, c)
		}
	}
	pn.RUnlock()
	return cs
}

// Peerstore 返回节点存储
// 返回值:
//   - peerstore.Peerstore: 节点存储对象
func (pn *peernet) Peerstore() peerstore.Peerstore {
	return pn.ps
}

// String 返回peernet的字符串表示
// 返回值:
//   - string: 格式化的字符串
func (pn *peernet) String() string {
	return fmt.Sprintf("<mock.peernet %s - %d conns>", pn.peer, len(pn.allConns()))
}

// handleNewStream 是触发客户端处理器的内部函数
// 参数:
//   - s: network.Stream 网络流对象
func (pn *peernet) handleNewStream(s network.Stream) {
	pn.RLock()
	handler := pn.streamHandler
	pn.RUnlock()
	if handler != nil {
		go handler(s)
	}
}

// DialPeer 尝试建立与给定节点的连接
// 遵循上下文约束
// 参数:
//   - ctx: context.Context 上下文对象
//   - p: peer.ID 目标节点ID
//
// 返回值:
//   - network.Conn: 建立的连接
//   - error: 错误信息
func (pn *peernet) DialPeer(ctx context.Context, p peer.ID) (network.Conn, error) {
	return pn.connect(p)
}

// connect 连接到指定节点
// 参数:
//   - p: peer.ID 目标节点ID
//
// 返回值:
//   - *conn: 建立的连接
//   - error: 错误信息
func (pn *peernet) connect(p peer.ID) (*conn, error) {
	if p == pn.peer {
		log.Debugf("尝试连接到自身 %s", p)
		return nil, fmt.Errorf("尝试连接到自身 %s", p)
	}

	// 首先检查是否已有活跃连接
	pn.RLock()
	cs, found := pn.connsByPeer[p]
	if found && len(cs) > 0 {
		var chosen *conn
		for c := range cs { // 因为cs是一个map
			chosen = c // 选择第一个
			break
		}
		pn.RUnlock()
		return chosen, nil
	}
	pn.RUnlock()

	if pn.gater != nil && !pn.gater.InterceptPeerDial(p) {
		log.Debugf("连接过滤器禁止了到节点 %s 的出站连接", p)
		return nil, fmt.Errorf("%v 连接过滤器禁止了到 %v 的连接", pn.peer, p)
	}
	log.Debugf("%s (新建) 正在拨号 %s", pn.peer, p)

	// 需要创建新连接,我们需要一个链路
	links := pn.mocknet.LinksBetweenPeers(pn.peer, p)
	if len(links) < 1 {
		log.Debugf("%s 无法连接到 %s", pn.peer, p)
		return nil, fmt.Errorf("%s 无法连接到 %s", pn.peer, p)
	}

	// 如果找到多个链路,如何选择?目前是随机选择...
	// 这是一个测试链路度量(网络接口)并正确选择的好地方
	l := links[rand.Intn(len(links))]

	log.Debugf("%s 正在拨号 %s openingConn", pn.peer, p)
	// 使用链路创建新连接
	return pn.openConn(p, l.(*link))
}

// openConn 打开一个新连接
// 参数:
//   - r: peer.ID 远程节点ID
//   - l: *link 链路对象
//
// 返回值:
//   - *conn: 建立的连接
//   - error: 错误信息
func (pn *peernet) openConn(r peer.ID, l *link) (*conn, error) {
	lc, rc := l.newConnPair(pn)
	addConnPair(pn, rc.net, lc, rc)
	log.Debugf("%s 正在打开到 %s 的连接", pn.LocalPeer(), lc.RemotePeer())
	abort := func() {
		_ = lc.Close()
		_ = rc.Close()
	}
	if pn.gater != nil && !pn.gater.InterceptAddrDial(lc.remote, lc.remoteAddr) {
		log.Debugf("%s 拒绝了到 %s 在地址 %s 的拨号", lc.local, lc.remote, lc.remoteAddr)
		abort()
		return nil, fmt.Errorf("%v 拒绝了到 %v 在地址 %v 的拨号", lc.local, lc.remote, lc.remoteAddr)
	}
	if rc.net.gater != nil && !rc.net.gater.InterceptAccept(rc) {
		log.Debugf("%s 拒绝了来自 %s 的连接", rc.local, rc.remote)
		abort()
		return nil, fmt.Errorf("%v 拒绝了来自 %v 的连接", rc.local, rc.remote)
	}
	if err := checkSecureAndUpgrade(network.DirOutbound, pn.gater, lc); err != nil {
		log.Debugf("安全检查失败: %v", err)
		abort()
		return nil, err
	}
	if err := checkSecureAndUpgrade(network.DirInbound, rc.net.gater, rc); err != nil {
		log.Debugf("安全检查失败: %v", err)
		abort()
		return nil, err
	}

	go rc.net.remoteOpenedConn(rc)
	pn.addConn(lc)
	return lc, nil
}

// checkSecureAndUpgrade 检查安全性并升级连接
// 参数:
//   - dir: network.Direction 连接方向
//   - gater: connmgr.ConnectionGater 连接过滤器
//   - c: *conn 连接对象
//
// 返回值:
//   - error: 错误信息
func checkSecureAndUpgrade(dir network.Direction, gater connmgr.ConnectionGater, c *conn) error {
	if gater == nil {
		return nil
	}
	if !gater.InterceptSecured(dir, c.remote, c) {
		log.Debugf("%v 拒绝了与 %v 的安全握手", c.local, c.remote)
		return fmt.Errorf("%v 拒绝了与 %v 的安全握手", c.local, c.remote)
	}
	allow, _ := gater.InterceptUpgraded(c)
	if !allow {
		log.Debugf("%v 拒绝了与 %v 的升级", c.local, c.remote)
		return fmt.Errorf("%v 拒绝了与 %v 的升级", c.local, c.remote)
	}
	return nil
}

// addConnPair 同时向两个peernet添加连接
// 必须跟随 pn1.addConn(c1) 和 pn2.addConn(c2) 调用
// 参数:
//   - pn1: *peernet 第一个peernet
//   - pn2: *peernet 第二个peernet
//   - c1: *conn 第一个连接
//   - c2: *conn 第二个连接
func addConnPair(pn1, pn2 *peernet, c1, c2 *conn) {
	var l1, l2 = pn1, pn2 // 按锁定顺序排列peernet
	// 字节比较等同于字符串的字典序比较
	if bytes.Compare([]byte(l1.LocalPeer()), []byte(l2.LocalPeer())) > 0 {
		l1, l2 = l2, l1
	}

	l1.Lock()
	l2.Lock()

	add := func(pn *peernet, c *conn) {
		_, found := pn.connsByPeer[c.RemotePeer()]
		if !found {
			pn.connsByPeer[c.RemotePeer()] = map[*conn]struct{}{}
		}
		pn.connsByPeer[c.RemotePeer()][c] = struct{}{}

		_, found = pn.connsByLink[c.link]
		if !found {
			pn.connsByLink[c.link] = map[*conn]struct{}{}
		}
		pn.connsByLink[c.link][c] = struct{}{}
	}
	add(pn1, c1)
	add(pn2, c2)

	c1.notifLk.Lock()
	c2.notifLk.Lock()
	l2.Unlock()
	l1.Unlock()
}

// remoteOpenedConn 处理远程打开的连接
// 参数:
//   - c: *conn 连接对象
func (pn *peernet) remoteOpenedConn(c *conn) {
	log.Debugf("%s 接受来自 %s 的连接", pn.LocalPeer(), c.RemotePeer())
	pn.addConn(c)
}

// addConn 构造并添加一个连接
// 到给定远程节点通过给定链路
// 参数:
//   - c: *conn 连接对象
func (pn *peernet) addConn(c *conn) {
	defer c.notifLk.Unlock()

	pn.notifyAll(func(n network.Notifiee) {
		n.Connected(pn, c)
	})

	pn.emitter.Emit(event.EvtPeerConnectednessChanged{
		Peer:          c.remote,
		Connectedness: network.Connected,
	})
}

// removeConn 移除给定连接
// 参数:
//   - c: *conn 要移除的连接
func (pn *peernet) removeConn(c *conn) {
	pn.Lock()
	cs, found := pn.connsByLink[c.link]
	if !found || len(cs) < 1 {
		panic(fmt.Sprintf("尝试移除不存在的连接 %p", c.link))
	}
	delete(cs, c)

	cs, found = pn.connsByPeer[c.remote]
	if !found {
		panic(fmt.Sprintf("尝试移除不存在的连接 %v", c.remote))
	}
	delete(cs, c)
	pn.Unlock()

	// 异步通知以模拟 Swarm
	// FIXME: 如果没记错的话,我们想要使 Close 的通知同步
	go func() {
		c.notifLk.Lock()
		defer c.notifLk.Unlock()
		pn.notifyAll(func(n network.Notifiee) {
			n.Disconnected(c.net, c)
		})
	}()

	c.net.emitter.Emit(event.EvtPeerConnectednessChanged{
		Peer:          c.remote,
		Connectedness: network.NotConnected,
	})
}

// LocalPeer 返回网络的本地节点ID
// 返回值:
//   - peer.ID: 本地节点ID
func (pn *peernet) LocalPeer() peer.ID {
	return pn.peer
}

// Peers 返回已连接的节点
// 返回值:
//   - []peer.ID: 已连接节点ID列表
func (pn *peernet) Peers() []peer.ID {
	pn.RLock()
	defer pn.RUnlock()

	peers := make([]peer.ID, 0, len(pn.connsByPeer))
	for _, cs := range pn.connsByPeer {
		for c := range cs {
			peers = append(peers, c.remote)
			break
		}
	}
	return peers
}

// Conns 返回此节点的所有连接
// 返回值:
//   - []network.Conn: 连接列表
func (pn *peernet) Conns() []network.Conn {
	pn.RLock()
	defer pn.RUnlock()

	out := make([]network.Conn, 0, len(pn.connsByPeer))
	for _, cs := range pn.connsByPeer {
		for c := range cs {
			out = append(out, c)
		}
	}
	return out
}

// ConnsToPeer 返回到指定节点的所有连接
// 参数:
//   - p: peer.ID 目标节点ID
//
// 返回值:
//   - []network.Conn: 连接列表
func (pn *peernet) ConnsToPeer(p peer.ID) []network.Conn {
	pn.RLock()
	defer pn.RUnlock()

	cs, found := pn.connsByPeer[p]
	if !found || len(cs) == 0 {
		return nil
	}

	cs2 := make([]network.Conn, 0, len(cs))
	for c := range cs {
		cs2 = append(cs2, c)
	}
	return cs2
}

// ClosePeer 关闭到指定节点的连接
// 参数:
//   - p: peer.ID 要关闭连接的节点ID
//
// 返回值:
//   - error: 错误信息
func (pn *peernet) ClosePeer(p peer.ID) error {
	pn.RLock()
	cs, found := pn.connsByPeer[p]
	if !found {
		pn.RUnlock()
		return nil
	}

	conns := make([]*conn, 0, len(cs))
	for c := range cs {
		conns = append(conns, c)
	}
	pn.RUnlock()
	for _, c := range conns {
		c.Close()
	}
	return nil
}

// BandwidthTotals 返回传输的总带宽
// 返回值:
//   - uint64: 入站总流量
//   - uint64: 出站总流量
func (pn *peernet) BandwidthTotals() (in uint64, out uint64) {
	// 需要实现这个。这次可能最好在swarm中实现。
	// 需要一个"metrics"对象
	return 0, 0
}

// Listen 告诉网络开始在给定的多地址上监听
// 参数:
//   - addrs: ...ma.Multiaddr 要监听的地址列表
//
// 返回值:
//   - error: 错误信息
func (pn *peernet) Listen(addrs ...ma.Multiaddr) error {
	pn.Peerstore().AddAddrs(pn.LocalPeer(), addrs, peerstore.PermanentAddrTTL)
	return nil
}

// ListenAddresses 返回此网络监听的地址列表
// 返回值:
//   - []ma.Multiaddr: 监听地址列表
func (pn *peernet) ListenAddresses() []ma.Multiaddr {
	return pn.Peerstore().Addrs(pn.LocalPeer())
}

// InterfaceListenAddresses 返回此网络监听的地址列表
// 它会展开"任意接口"地址(/ip4/0.0.0.0, /ip6/::)以使用已知的本地接口
// 返回值:
//   - []ma.Multiaddr: 监听地址列表
//   - error: 错误信息
func (pn *peernet) InterfaceListenAddresses() ([]ma.Multiaddr, error) {
	return pn.ListenAddresses(), nil
}

// Connectedness 返回表示连接能力的状态
// 目前只返回 Connected || NotConnected。以后扩展更多状态。
// 参数:
//   - p: peer.ID 要检查的节点ID
//
// 返回值:
//   - network.Connectedness: 连接状态
func (pn *peernet) Connectedness(p peer.ID) network.Connectedness {
	pn.Lock()
	defer pn.Unlock()

	cs, found := pn.connsByPeer[p]
	if found && len(cs) > 0 {
		return network.Connected
	}
	return network.NotConnected
}

// NewStream 返回到给定节点p的新流
// 如果没有到p的连接,尝试创建一个
// 参数:
//   - ctx: context.Context 上下文对象
//   - p: peer.ID 目标节点ID
//
// 返回值:
//   - network.Stream: 新建的流
//   - error: 错误信息
func (pn *peernet) NewStream(ctx context.Context, p peer.ID) (network.Stream, error) {
	c, err := pn.DialPeer(ctx, p)
	if err != nil {
		return nil, err
	}
	return c.NewStream(ctx)
}

// SetStreamHandler 在网络上设置新的流处理器
// 此操作是线程安全的
// 参数:
//   - h: network.StreamHandler 流处理器
func (pn *peernet) SetStreamHandler(h network.StreamHandler) {
	pn.Lock()
	pn.streamHandler = h
	pn.Unlock()
}

// Notify 注册Notifiee以接收事件发生时的信号
// 参数:
//   - f: network.Notifiee 通知处理器
func (pn *peernet) Notify(f network.Notifiee) {
	pn.notifmu.Lock()
	pn.notifs[f] = struct{}{}
	pn.notifmu.Unlock()
}

// StopNotify 取消注册Notifiee接收信号
// 参数:
//   - f: network.Notifiee 要取消的通知处理器
func (pn *peernet) StopNotify(f network.Notifiee) {
	pn.notifmu.Lock()
	delete(pn.notifs, f)
	pn.notifmu.Unlock()
}

// notifyAll 在所有Notifiee上运行通知函数
// 参数:
//   - notification: func(f network.Notifiee) 要执行的通知函数
func (pn *peernet) notifyAll(notification func(f network.Notifiee)) {
	pn.notifmu.Lock()
	// 同步通知以模拟Swarm
	for n := range pn.notifs {
		notification(n)
	}
	pn.notifmu.Unlock()
}

// ResourceManager 返回资源管理器
// 返回值:
//   - network.ResourceManager: 资源管理器接口
func (pn *peernet) ResourceManager() network.ResourceManager {
	return &network.NullResourceManager{}
}

// CanDial 检查是否可以拨号到指定节点和地址
// 参数:
//   - p: peer.ID 目标节点ID
//   - addr: ma.Multiaddr 目标地址
//
// 返回值:
//   - bool: 是否可以拨号
func (pn *peernet) CanDial(p peer.ID, addr ma.Multiaddr) bool {
	return true
}
