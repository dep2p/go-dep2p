package mocknet

import (
	"context"
	"crypto/rand"
	"fmt"
	"net"
	"sort"
	"sync"

	ic "github.com/dep2p/go-dep2p/core/crypto"
	"github.com/dep2p/go-dep2p/core/host"
	"github.com/dep2p/go-dep2p/core/network"
	"github.com/dep2p/go-dep2p/core/peer"
	"github.com/dep2p/go-dep2p/core/peerstore"
	bhost "github.com/dep2p/go-dep2p/p2p/host/basic"
	"github.com/dep2p/go-dep2p/p2p/host/eventbus"
	"github.com/dep2p/go-dep2p/p2p/host/peerstore/pstoremem"

	ma "github.com/dep2p/go-dep2p/multiformats/multiaddr"
)

// 被黑洞的IPv6范围（以防我们的流量泄露到互联网上）
var blackholeIP6 = net.ParseIP("100::")

// mocknet 实现了 mocknet.Mocknet 接口
type mocknet struct {
	// nets 存储 peer ID 到对应网络的映射
	nets map[peer.ID]*peernet
	// hosts 存储 peer ID 到对应主机的映射
	hosts map[peer.ID]host.Host

	// links 使得两个 peer 之间可以建立连接
	// 可以将 links 理解为物理介质
	// 通常只有一个连接，但也可能有多个连接 **连接在 peers 之间共享**
	links map[peer.ID]map[peer.ID]map[*link]struct{}

	// 连接的默认配置
	linkDefaults LinkOptions

	// 用于取消上下文的函数
	ctxCancel context.CancelFunc
	// 上下文对象
	ctx context.Context
	// 互斥锁
	sync.Mutex
}

// New 创建并返回一个新的 Mocknet 实例
//
// 返回值:
//   - Mocknet 新创建的 Mocknet 实例
func New() Mocknet {
	// 创建 mocknet 实例并初始化映射
	mn := &mocknet{
		nets:  map[peer.ID]*peernet{},
		hosts: map[peer.ID]host.Host{},
		links: map[peer.ID]map[peer.ID]map[*link]struct{}{},
	}
	// 创建带取消功能的上下文
	mn.ctx, mn.ctxCancel = context.WithCancel(context.Background())
	return mn
}

// Close 关闭 mocknet 及其所有资源
//
// 返回值:
//   - error 关闭过程中的错误
func (mn *mocknet) Close() error {
	// 取消上下文
	mn.ctxCancel()
	// 关闭所有主机
	for _, h := range mn.hosts {
		h.Close()
	}
	// 关闭所有网络
	for _, n := range mn.nets {
		n.Close()
	}
	return nil
}

// GenPeer 生成一个新的对等节点
//
// 返回值:
//   - host.Host 生成的主机
//   - error 生成过程中的错误
func (mn *mocknet) GenPeer() (host.Host, error) {
	return mn.GenPeerWithOptions(PeerOptions{})
}

// GenPeerWithOptions 使用指定选项生成一个新的对等节点
//
// 参数:
//   - opts: PeerOptions 对等节点的配置选项
//
// 返回值:
//   - host.Host 生成的主机
//   - error 生成过程中的错误
func (mn *mocknet) GenPeerWithOptions(opts PeerOptions) (host.Host, error) {
	// 添加默认配置
	if err := mn.addDefaults(&opts); err != nil {
		log.Debugf("添加默认配置失败: %v", err)
		return nil, err
	}
	// 生成 ECDSA 密钥对
	sk, _, err := ic.GenerateECDSAKeyPair(rand.Reader)
	if err != nil {
		log.Debugf("生成ECDSA密钥对失败: %v", err)
		return nil, err
	}
	// 从私钥生成 peer ID
	id, err := peer.IDFromPrivateKey(sk)
	if err != nil {
		log.Debugf("生成peer ID失败: %v", err)
		return nil, err
	}
	// 获取 peer ID 的后缀
	suffix := id
	if len(id) > 8 {
		suffix = id[len(id)-8:]
	}
	// 创建 IP 地址
	ip := append(net.IP{}, blackholeIP6...)
	copy(ip[net.IPv6len-len(suffix):], suffix)
	// 创建多地址
	a, err := ma.NewMultiaddr(fmt.Sprintf("/ip6/%s/tcp/4242", ip))
	if err != nil {
		log.Debugf("创建测试多地址失败: %s", err)
		return nil, err
	}

	// 获取或创建 peerstore
	var ps peerstore.Peerstore
	if opts.ps == nil {
		ps, err = pstoremem.NewPeerstore()
		if err != nil {
			log.Debugf("创建peerstore失败: %v", err)
			return nil, err
		}
	} else {
		ps = opts.ps
	}
	// 更新 peerstore
	p, err := mn.updatePeerstore(sk, a, ps)
	if err != nil {
		log.Debugf("更新peerstore失败: %v", err)
		return nil, err
	}
	// 添加对等节点
	h, err := mn.AddPeerWithOptions(p, opts)
	if err != nil {
		log.Debugf("添加对等节点失败: %v", err)
		return nil, err
	}

	return h, nil
}

// AddPeer 添加一个带有指定私钥和地址的对等节点
//
// 参数:
//   - k: ic.PrivKey 私钥
//   - a: ma.Multiaddr 多地址
//
// 返回值:
//   - host.Host 创建的主机
//   - error 创建过程中的错误
func (mn *mocknet) AddPeer(k ic.PrivKey, a ma.Multiaddr) (host.Host, error) {
	// 创建新的 peerstore
	ps, err := pstoremem.NewPeerstore()
	if err != nil {
		log.Debugf("创建peerstore失败: %v", err)
		return nil, err
	}
	// 更新 peerstore
	p, err := mn.updatePeerstore(k, a, ps)
	if err != nil {
		log.Debugf("更新peerstore失败: %v", err)
		return nil, err
	}

	return mn.AddPeerWithPeerstore(p, ps)
}

// AddPeerWithPeerstore 使用指定的 peerstore 添加对等节点
//
// 参数:
//   - p: peer.ID 对等节点 ID
//   - ps: peerstore.Peerstore peerstore 实例
//
// 返回值:
//   - host.Host 创建的主机
//   - error 创建过程中的错误
func (mn *mocknet) AddPeerWithPeerstore(p peer.ID, ps peerstore.Peerstore) (host.Host, error) {
	return mn.AddPeerWithOptions(p, PeerOptions{ps: ps})
}

// AddPeerWithOptions 使用指定选项添加对等节点
//
// 参数:
//   - p: peer.ID 对等节点 ID
//   - opts: PeerOptions 配置选项
//
// 返回值:
//   - host.Host 创建的主机
//   - error 创建过程中的错误
func (mn *mocknet) AddPeerWithOptions(p peer.ID, opts PeerOptions) (host.Host, error) {
	// 创建事件总线
	bus := eventbus.NewBus()
	// 添加默认配置
	if err := mn.addDefaults(&opts); err != nil {
		log.Debugf("添加默认配置失败: %v", err)
		return nil, err
	}
	// 创建新的对等网络
	n, err := newPeernet(mn, p, opts, bus)
	if err != nil {
		log.Debugf("创建新的对等网络失败: %v", err)
		return nil, err
	}

	// 设置主机选项
	hostOpts := &bhost.HostOpts{
		NegotiationTimeout:      -1,
		DisableSignedPeerRecord: true,
		EventBus:                bus,
	}

	// 创建新主机
	h, err := bhost.NewHost(n, hostOpts)
	if err != nil {
		log.Debugf("创建新主机失败: %v", err)
		return nil, err
	}
	// 启动主机
	h.Start()

	// 将网络和主机添加到 mocknet
	mn.Lock()
	mn.nets[n.peer] = n
	mn.hosts[n.peer] = h
	mn.Unlock()
	return h, nil
}

// addDefaults 为选项添加默认值
//
// 参数:
//   - opts: *PeerOptions 要添加默认值的选项
//
// 返回值:
//   - error 添加默认值过程中的错误
func (mn *mocknet) addDefaults(opts *PeerOptions) error {
	if opts.ps == nil {
		ps, err := pstoremem.NewPeerstore()
		if err != nil {
			log.Debugf("创建peerstore失败: %v", err)
			return err
		}
		opts.ps = ps
	}
	return nil
}

// updatePeerstore 更新 peerstore 中的信息
//
// 参数:
//   - k: ic.PrivKey 私钥
//   - a: ma.Multiaddr 多地址
//   - ps: peerstore.Peerstore 要更新的 peerstore
//
// 返回值:
//   - peer.ID 对等节点 ID
//   - error 更新过程中的错误
func (mn *mocknet) updatePeerstore(k ic.PrivKey, a ma.Multiaddr, ps peerstore.Peerstore) (peer.ID, error) {
	// 从公钥获取 peer ID
	p, err := peer.IDFromPublicKey(k.GetPublic())
	if err != nil {
		log.Debugf("从公钥获取peer ID失败: %v", err)
		return "", err
	}

	// 添加地址
	ps.AddAddr(p, a, peerstore.PermanentAddrTTL)
	// 添加私钥
	err = ps.AddPrivKey(p, k)
	if err != nil {
		log.Debugf("添加私钥失败: %v", err)
		return "", err
	}
	// 添加公钥
	err = ps.AddPubKey(p, k.GetPublic())
	if err != nil {
		log.Debugf("添加公钥失败: %v", err)
		return "", err
	}
	return p, nil
}

// Peers 返回所有对等节点的 ID 列表
//
// 返回值:
//   - []peer.ID 对等节点 ID 列表
func (mn *mocknet) Peers() []peer.ID {
	mn.Lock()
	defer mn.Unlock()

	// 创建 ID 列表副本
	cp := make([]peer.ID, 0, len(mn.nets))
	for _, n := range mn.nets {
		cp = append(cp, n.peer)
	}
	// 对 ID 列表排序
	sort.Sort(peer.IDSlice(cp))
	return cp
}

// Host 返回指定 ID 的主机
//
// 参数:
//   - pid: peer.ID 对等节点 ID
//
// 返回值:
//   - host.Host 对应的主机
func (mn *mocknet) Host(pid peer.ID) host.Host {
	mn.Lock()
	host := mn.hosts[pid]
	mn.Unlock()
	return host
}

// Net 返回指定 ID 的网络
//
// 参数:
//   - pid: peer.ID 对等节点 ID
//
// 返回值:
//   - network.Network 对应的网络
func (mn *mocknet) Net(pid peer.ID) network.Network {
	mn.Lock()
	n := mn.nets[pid]
	mn.Unlock()
	return n
}

// Hosts 返回所有主机的列表
//
// 返回值:
//   - []host.Host 主机列表
func (mn *mocknet) Hosts() []host.Host {
	mn.Lock()
	defer mn.Unlock()

	// 创建主机列表副本
	cp := make([]host.Host, 0, len(mn.hosts))
	for _, h := range mn.hosts {
		cp = append(cp, h)
	}

	// 对主机列表排序
	sort.Sort(hostSlice(cp))
	return cp
}

// Nets 返回所有网络的列表
//
// 返回值:
//   - []network.Network 网络列表
func (mn *mocknet) Nets() []network.Network {
	mn.Lock()
	defer mn.Unlock()

	// 创建网络列表副本
	cp := make([]network.Network, 0, len(mn.nets))
	for _, n := range mn.nets {
		cp = append(cp, n)
	}
	// 对网络列表排序
	sort.Sort(netSlice(cp))
	return cp
}

// Links 返回内部连接状态映射的副本
//
// 返回值:
//   - LinkMap 连接状态映射
func (mn *mocknet) Links() LinkMap {
	mn.Lock()
	defer mn.Unlock()

	// 创建连接映射副本
	links := map[string]map[string]map[Link]struct{}{}
	for p1, lm := range mn.links {
		sp1 := string(p1)
		links[sp1] = map[string]map[Link]struct{}{}
		for p2, ls := range lm {
			sp2 := string(p2)
			links[sp1][sp2] = map[Link]struct{}{}
			for l := range ls {
				links[sp1][sp2][l] = struct{}{}
			}
		}
	}
	return links
}

// LinkAll 连接所有对等节点
//
// 返回值:
//   - error 连接过程中的错误
func (mn *mocknet) LinkAll() error {
	nets := mn.Nets()
	for _, n1 := range nets {
		for _, n2 := range nets {
			if _, err := mn.LinkNets(n1, n2); err != nil {
				log.Debugf("连接网络失败: %v", err)
				return err
			}
		}
	}
	return nil
}

// LinkPeers 连接两个对等节点
//
// 参数:
//   - p1, p2: peer.ID 要连接的两个对等节点 ID
//
// 返回值:
//   - Link 创建的连接
//   - error 连接过程中的错误
func (mn *mocknet) LinkPeers(p1, p2 peer.ID) (Link, error) {
	mn.Lock()
	n1 := mn.nets[p1]
	n2 := mn.nets[p2]
	mn.Unlock()

	if n1 == nil {
		log.Debugf("p1 的网络不在 mocknet 中")
		return nil, fmt.Errorf("p1 的网络不在 mocknet 中")
	}

	if n2 == nil {
		log.Debugf("p2 的网络不在 mocknet 中")
		return nil, fmt.Errorf("p2 的网络不在 mocknet 中")
	}

	return mn.LinkNets(n1, n2)
}

// validate 验证网络是否有效
//
// 参数:
//   - n: network.Network 要验证的网络
//
// 返回值:
//   - *peernet 验证后的网络
//   - error 验证过程中的错误
func (mn *mocknet) validate(n network.Network) (*peernet, error) {
	// 警告：假设已获取锁

	nr, ok := n.(*peernet)
	if !ok {
		log.Debugf("不支持的网络类型（仅支持 mock 包中的网络）")
		return nil, fmt.Errorf("不支持的网络类型（仅支持 mock 包中的网络）")
	}

	if _, found := mn.nets[nr.peer]; !found {
		log.Debugf("网络不在 mocknet 中，是否来自其他 mocknet？")
		return nil, fmt.Errorf("网络不在 mocknet 中，是否来自其他 mocknet？")
	}

	return nr, nil
}

// LinkNets 连接两个网络
//
// 参数:
//   - n1, n2: network.Network 要连接的两个网络
//
// 返回值:
//   - Link 创建的连接
//   - error 连接过程中的错误
func (mn *mocknet) LinkNets(n1, n2 network.Network) (Link, error) {
	mn.Lock()
	n1r, err1 := mn.validate(n1)
	n2r, err2 := mn.validate(n2)
	ld := mn.linkDefaults
	mn.Unlock()

	if err1 != nil {
		log.Debugf("验证网络失败: %v", err1)
		return nil, err1
	}
	if err2 != nil {
		log.Debugf("验证网络失败: %v", err2)
		return nil, err2
	}

	l := newLink(mn, ld)
	l.nets = append(l.nets, n1r, n2r)
	mn.addLink(l)
	return l, nil
}

// Unlink 断开连接
//
// 参数:
//   - l2: Link 要断开的连接
//
// 返回值:
//   - error 断开过程中的错误
func (mn *mocknet) Unlink(l2 Link) error {
	l, ok := l2.(*link)
	if !ok {
		log.Debugf("仅支持来自 mocknet 的连接")
		return fmt.Errorf("仅支持来自 mocknet 的连接")
	}

	mn.removeLink(l)
	return nil
}

// UnlinkPeers 断开两个对等节点之间的连接
//
// 参数:
//   - p1, p2: peer.ID 要断开连接的两个对等节点 ID
//
// 返回值:
//   - error 断开过程中的错误
func (mn *mocknet) UnlinkPeers(p1, p2 peer.ID) error {
	ls := mn.LinksBetweenPeers(p1, p2)
	if ls == nil {
		log.Debugf("p1 和 p2 之间没有连接")
		return fmt.Errorf("p1 和 p2 之间没有连接")
	}

	for _, l := range ls {
		if err := mn.Unlink(l); err != nil {
			log.Debugf("断开连接失败: %v", err)
			return err
		}
	}
	return nil
}

// UnlinkNets 断开两个网络之间的连接
//
// 参数:
//   - n1, n2: network.Network 要断开连接的两个网络
//
// 返回值:
//   - error 断开过程中的错误
func (mn *mocknet) UnlinkNets(n1, n2 network.Network) error {
	return mn.UnlinkPeers(n1.LocalPeer(), n2.LocalPeer())
}

// linksMapGet 从连接映射中获取或创建连接集合
//
// 参数:
//   - p1, p2: peer.ID 两个对等节点 ID
//
// 返回值:
//   - map[*link]struct{} 连接集合
func (mn *mocknet) linksMapGet(p1, p2 peer.ID) map[*link]struct{} {
	l1, found := mn.links[p1]
	if !found {
		mn.links[p1] = map[peer.ID]map[*link]struct{}{}
		l1 = mn.links[p1]
	}

	l2, found := l1[p2]
	if !found {
		m := map[*link]struct{}{}
		l1[p2] = m
		l2 = l1[p2]
	}

	return l2
}

// addLink 添加连接
//
// 参数:
//   - l: *link 要添加的连接
func (mn *mocknet) addLink(l *link) {
	mn.Lock()
	defer mn.Unlock()

	n1, n2 := l.nets[0], l.nets[1]
	mn.linksMapGet(n1.peer, n2.peer)[l] = struct{}{}
	mn.linksMapGet(n2.peer, n1.peer)[l] = struct{}{}
}

// removeLink 移除连接
//
// 参数:
//   - l: *link 要移除的连接
func (mn *mocknet) removeLink(l *link) {
	mn.Lock()
	defer mn.Unlock()

	n1, n2 := l.nets[0], l.nets[1]
	delete(mn.linksMapGet(n1.peer, n2.peer), l)
	delete(mn.linksMapGet(n2.peer, n1.peer), l)
}

// ConnectAllButSelf 连接所有对等节点（除了自身）
//
// 返回值:
//   - error 连接过程中的错误
func (mn *mocknet) ConnectAllButSelf() error {
	nets := mn.Nets()
	for _, n1 := range nets {
		for _, n2 := range nets {
			if n1 == n2 {
				continue
			}

			if _, err := mn.ConnectNets(n1, n2); err != nil {
				log.Debugf("连接网络失败: %v", err)
				return err
			}
		}
	}
	return nil
}

// ConnectPeers 建立两个对等节点之间的连接
//
// 参数:
//   - a, b: peer.ID 要连接的两个对等节点 ID
//
// 返回值:
//   - network.Conn 建立的连接
//   - error 连接过程中的错误
func (mn *mocknet) ConnectPeers(a, b peer.ID) (network.Conn, error) {
	return mn.Net(a).DialPeer(mn.ctx, b)
}

// ConnectNets 建立两个网络之间的连接
//
// 参数:
//   - a, b: network.Network 要连接的两个网络
//
// 返回值:
//   - network.Conn 建立的连接
//   - error 连接过程中的错误
func (mn *mocknet) ConnectNets(a, b network.Network) (network.Conn, error) {
	return a.DialPeer(mn.ctx, b.LocalPeer())
}

// DisconnectPeers 断开两个对等节点之间的连接
//
// 参数:
//   - p1, p2: peer.ID 要断开连接的两个对等节点 ID
//
// 返回值:
//   - error 断开过程中的错误
func (mn *mocknet) DisconnectPeers(p1, p2 peer.ID) error {
	return mn.Net(p1).ClosePeer(p2)
}

// DisconnectNets 断开两个网络之间的连接
//
// 参数:
//   - n1, n2: network.Network 要断开连接的两个网络
//
// 返回值:
//   - error 断开过程中的错误
func (mn *mocknet) DisconnectNets(n1, n2 network.Network) error {
	return n1.ClosePeer(n2.LocalPeer())
}

// LinksBetweenPeers 返回两个对等节点之间的所有连接
//
// 参数:
//   - p1, p2: peer.ID 两个对等节点 ID
//
// 返回值:
//   - []Link 连接列表
func (mn *mocknet) LinksBetweenPeers(p1, p2 peer.ID) []Link {
	mn.Lock()
	defer mn.Unlock()

	ls2 := mn.linksMapGet(p1, p2)
	cp := make([]Link, 0, len(ls2))
	for l := range ls2 {
		cp = append(cp, l)
	}
	return cp
}

// LinksBetweenNets 返回两个网络之间的所有连接
//
// 参数:
//   - n1, n2: network.Network 两个网络
//
// 返回值:
//   - []Link - 连接列表
func (mn *mocknet) LinksBetweenNets(n1, n2 network.Network) []Link {
	return mn.LinksBetweenPeers(n1.LocalPeer(), n2.LocalPeer())
}

// SetLinkDefaults 设置连接的默认选项
//
// 参数:
//   - o: LinkOption 默认连接选项
func (mn *mocknet) SetLinkDefaults(o LinkOptions) {
	mn.Lock()
	mn.linkDefaults = o
	mn.Unlock()
}

// LinkDefaults 获取连接的默认选项
//
// 返回值:
//   - LinkOptions 默认连接选项
func (mn *mocknet) LinkDefaults() LinkOptions {
	mn.Lock()
	defer mn.Unlock()
	return mn.linkDefaults
}

// netSlice 用于按对等节点排序的网络切片
type netSlice []network.Network

func (es netSlice) Len() int           { return len(es) }
func (es netSlice) Swap(i, j int)      { es[i], es[j] = es[j], es[i] }
func (es netSlice) Less(i, j int) bool { return string(es[i].LocalPeer()) < string(es[j].LocalPeer()) }

// hostSlice 用于按对等节点排序的主机切片
type hostSlice []host.Host

func (es hostSlice) Len() int           { return len(es) }
func (es hostSlice) Swap(i, j int)      { es[i], es[j] = es[j], es[i] }
func (es hostSlice) Less(i, j int) bool { return string(es[i].ID()) < string(es[j].ID()) }
