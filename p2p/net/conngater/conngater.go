package conngater

import (
	"context"
	"net"
	"sync"

	"github.com/dep2p/go-dep2p/core/connmgr"
	"github.com/dep2p/go-dep2p/core/control"
	"github.com/dep2p/go-dep2p/core/network"
	"github.com/dep2p/go-dep2p/core/peer"

	ma "github.com/dep2p/go-dep2p/multiformats/multiaddr"
	manet "github.com/dep2p/go-dep2p/multiformats/multiaddr/net"

	"github.com/dep2p/go-dep2p/datastore"
	"github.com/dep2p/go-dep2p/datastore/namespace"
	"github.com/dep2p/go-dep2p/datastore/query"
	logging "github.com/dep2p/log"
)

// BasicConnectionGater 实现了一个连接门控器，允许应用程序对传入和传出连接进行访问控制。
type BasicConnectionGater struct {
	sync.RWMutex

	// 被阻止的对等节点集合
	blockedPeers map[peer.ID]struct{}
	// 被阻止的IP地址集合
	blockedAddrs map[string]struct{}
	// 被阻止的子网集合
	blockedSubnets map[string]*net.IPNet

	// 用于持久化存储的数据存储
	ds datastore.Datastore
}

// 创建日志记录器
var log = logging.Logger("net-conngater")

const (
	// 数据存储命名空间
	ns = "/dep2p/net/conngater"
	// 对等节点键前缀
	keyPeer = "/peer/"
	// 地址键前缀
	keyAddr = "/addr/"
	// 子网键前缀
	keySubnet = "/subnet/"
)

// NewBasicConnectionGater 创建一个新的连接门控器。
// 参数:
//   - ds: 可选的数据存储，用于持久化连接门控器的过滤规则，可以为nil
//
// 返回:
//   - *BasicConnectionGater: 新创建的连接门控器
//   - error: 如果加载规则失败则返回错误
func NewBasicConnectionGater(ds datastore.Datastore) (*BasicConnectionGater, error) {
	// 初始化连接门控器结构体
	cg := &BasicConnectionGater{
		blockedPeers:   make(map[peer.ID]struct{}),
		blockedAddrs:   make(map[string]struct{}),
		blockedSubnets: make(map[string]*net.IPNet),
	}

	// 如果提供了数据存储，则加载持久化的规则
	if ds != nil {
		// 包装数据存储以使用特定的命名空间
		cg.ds = namespace.Wrap(ds, datastore.NewKey(ns))
		// 从数据存储加载规则
		err := cg.loadRules(context.Background())
		if err != nil {
			log.Debugf("加载规则失败: %v", err)
			return nil, err
		}
	}

	return cg, nil
}

// loadRules 从数据存储加载过滤规则。
// 参数:
//   - ctx: 上下文对象
//
// 返回:
//   - error: 如果加载失败则返回错误
func (cg *BasicConnectionGater) loadRules(ctx context.Context) error {
	// 加载被阻止的对等节点
	res, err := cg.ds.Query(ctx, query.Query{Prefix: keyPeer})
	if err != nil {
		log.Errorf("查询数据存储中被阻止的对等节点时出错: %s", err)
		return err
	}

	// 遍历查询结果
	for r := range res.Next() {
		if r.Error != nil {
			log.Errorf("查询结果错误: %s", r.Error)
			return err
		}

		// 将结果添加到被阻止的对等节点集合
		p := peer.ID(r.Entry.Value)
		cg.blockedPeers[p] = struct{}{}
	}

	// 加载被阻止的地址
	res, err = cg.ds.Query(ctx, query.Query{Prefix: keyAddr})
	if err != nil {
		log.Errorf("查询数据存储中被阻止的地址时出错: %s", err)
		return err
	}

	// 遍历查询结果
	for r := range res.Next() {
		if r.Error != nil {
			log.Errorf("查询结果错误: %s", r.Error)
			return err
		}

		// 将结果添加到被阻止的地址集合
		ip := net.IP(r.Entry.Value)
		cg.blockedAddrs[ip.String()] = struct{}{}
	}

	// 加载被阻止的子网
	res, err = cg.ds.Query(ctx, query.Query{Prefix: keySubnet})
	if err != nil {
		log.Errorf("查询数据存储中被阻止的子网时出错: %s", err)
		return err
	}

	// 遍历查询结果
	for r := range res.Next() {
		if r.Error != nil {
			log.Errorf("查询结果错误: %s", r.Error)
			return err
		}

		// 解析CIDR子网并添加到被阻止的子网集合
		ipnetStr := string(r.Entry.Value)
		_, ipnet, err := net.ParseCIDR(ipnetStr)
		if err != nil {
			log.Errorf("解析CIDR子网时出错: %s", err)
			return err
		}
		cg.blockedSubnets[ipnetStr] = ipnet
	}

	return nil
}

// BlockPeer 将对等节点添加到被阻止的对等节点集合中。
// 注意：不会自动关闭与该对等节点的活动连接。
// 参数:
//   - p: 要阻止的对等节点ID
//
// 返回:
//   - error: 如果操作失败则返回错误
func (cg *BasicConnectionGater) BlockPeer(p peer.ID) error {
	// 如果启用了数据存储，则保存到数据存储
	if cg.ds != nil {
		err := cg.ds.Put(context.Background(), datastore.NewKey(keyPeer+p.String()), []byte(p))
		if err != nil {
			log.Errorf("将被阻止的对等节点写入数据存储时出错: %s", err)
			return err
		}
	}

	// 添加到内存中的集合
	cg.Lock()
	defer cg.Unlock()
	cg.blockedPeers[p] = struct{}{}

	return nil
}

// UnblockPeer 从被阻止的对等节点集合中移除一个对等节点。
// 参数:
//   - p: 要解除阻止的对等节点ID
//
// 返回:
//   - error: 如果操作失败则返回错误
func (cg *BasicConnectionGater) UnblockPeer(p peer.ID) error {
	// 如果启用了数据存储，则从数据存储中删除
	if cg.ds != nil {
		err := cg.ds.Delete(context.Background(), datastore.NewKey(keyPeer+p.String()))
		if err != nil {
			log.Errorf("从数据存储删除被阻止的对等节点时出错: %s", err)
			return err
		}
	}

	// 从内存中的集合移除
	cg.Lock()
	defer cg.Unlock()
	delete(cg.blockedPeers, p)

	return nil
}

// ListBlockedPeers 返回被阻止的对等节点列表。
// 返回:
//   - []peer.ID: 被阻止的对等节点ID列表
func (cg *BasicConnectionGater) ListBlockedPeers() []peer.ID {
	cg.RLock()
	defer cg.RUnlock()

	// 创建结果切片
	result := make([]peer.ID, 0, len(cg.blockedPeers))
	// 将所有被阻止的对等节点添加到结果中
	for p := range cg.blockedPeers {
		result = append(result, p)
	}

	return result
}

// BlockAddr 将IP地址添加到被阻止的地址集合中。
// 注意：不会自动关闭与该IP地址的活动连接。
// 参数:
//   - ip: 要阻止的IP地址
//
// 返回:
//   - error: 如果操作失败则返回错误
func (cg *BasicConnectionGater) BlockAddr(ip net.IP) error {
	// 如果启用了数据存储，则保存到数据存储
	if cg.ds != nil {
		err := cg.ds.Put(context.Background(), datastore.NewKey(keyAddr+ip.String()), []byte(ip))
		if err != nil {
			log.Errorf("将被阻止的地址写入数据存储时出错: %s", err)
			return err
		}
	}

	// 添加到内存中的集合
	cg.Lock()
	defer cg.Unlock()
	cg.blockedAddrs[ip.String()] = struct{}{}

	return nil
}

// UnblockAddr 从被阻止的地址集合中移除一个IP地址。
// 参数:
//   - ip: 要解除阻止的IP地址
//
// 返回:
//   - error: 如果操作失败则返回错误
func (cg *BasicConnectionGater) UnblockAddr(ip net.IP) error {
	// 如果启用了数据存储，则从数据存储中删除
	if cg.ds != nil {
		err := cg.ds.Delete(context.Background(), datastore.NewKey(keyAddr+ip.String()))
		if err != nil {
			log.Errorf("从数据存储删除被阻止的地址时出错: %s", err)
			return err
		}
	}

	// 从内存中的集合移除
	cg.Lock()
	defer cg.Unlock()
	delete(cg.blockedAddrs, ip.String())

	return nil
}

// ListBlockedAddrs 返回被阻止的IP地址列表。
// 返回:
//   - []net.IP: 被阻止的IP地址列表
func (cg *BasicConnectionGater) ListBlockedAddrs() []net.IP {
	cg.RLock()
	defer cg.RUnlock()

	// 创建结果切片
	result := make([]net.IP, 0, len(cg.blockedAddrs))
	// 将所有被阻止的IP地址添加到结果中
	for ipStr := range cg.blockedAddrs {
		ip := net.ParseIP(ipStr)
		result = append(result, ip)
	}

	return result
}

// BlockSubnet 将IP子网添加到被阻止的子网集合中。
// 注意：不会自动关闭与该子网的活动连接。
// 参数:
//   - ipnet: 要阻止的IP子网
//
// 返回:
//   - error: 如果操作失败则返回错误
func (cg *BasicConnectionGater) BlockSubnet(ipnet *net.IPNet) error {
	// 如果启用了数据存储，则保存到数据存储
	if cg.ds != nil {
		err := cg.ds.Put(context.Background(), datastore.NewKey(keySubnet+ipnet.String()), []byte(ipnet.String()))
		if err != nil {
			log.Errorf("将被阻止的子网写入数据存储时出错: %s", err)
			return err
		}
	}

	// 添加到内存中的集合
	cg.Lock()
	defer cg.Unlock()
	cg.blockedSubnets[ipnet.String()] = ipnet

	return nil
}

// UnblockSubnet 从被阻止的子网集合中移除一个IP子网。
// 参数:
//   - ipnet: 要解除阻止的IP子网
//
// 返回:
//   - error: 如果操作失败则返回错误
func (cg *BasicConnectionGater) UnblockSubnet(ipnet *net.IPNet) error {
	// 如果启用了数据存储，则从数据存储中删除
	if cg.ds != nil {
		err := cg.ds.Delete(context.Background(), datastore.NewKey(keySubnet+ipnet.String()))
		if err != nil {
			log.Errorf("从数据存储删除被阻止的子网时出错: %s", err)
			return err
		}
	}

	// 从内存中的集合移除
	cg.Lock()
	defer cg.Unlock()
	delete(cg.blockedSubnets, ipnet.String())

	return nil
}

// ListBlockedSubnets 返回被阻止的IP子网列表。
// 返回:
//   - []*net.IPNet: 被阻止的IP子网列表
func (cg *BasicConnectionGater) ListBlockedSubnets() []*net.IPNet {
	cg.RLock()
	defer cg.RUnlock()

	// 创建结果切片
	result := make([]*net.IPNet, 0, len(cg.blockedSubnets))
	// 将所有被阻止的子网添加到结果中
	for _, ipnet := range cg.blockedSubnets {
		result = append(result, ipnet)
	}

	return result
}

// 确保 BasicConnectionGater 实现了 ConnectionGater 接口
var _ connmgr.ConnectionGater = (*BasicConnectionGater)(nil)

// InterceptPeerDial 拦截对等节点拨号请求。
// 参数:
//   - p: 目标对等节点ID
//
// 返回:
//   - bool: 如果允许连接则返回true，否则返回false
func (cg *BasicConnectionGater) InterceptPeerDial(p peer.ID) (allow bool) {
	cg.RLock()
	defer cg.RUnlock()

	// 检查对等节点是否被阻止
	_, block := cg.blockedPeers[p]
	return !block
}

// InterceptAddrDial 拦截地址拨号请求。
// 参数:
//   - p: 目标对等节点ID
//   - a: 目标多地址
//
// 返回:
//   - bool: 如果允许连接则返回true，否则返回false
func (cg *BasicConnectionGater) InterceptAddrDial(p peer.ID, a ma.Multiaddr) (allow bool) {
	// 已在 InterceptPeerDial 中过滤了被阻止的对等节点，这里只检查IP
	cg.RLock()
	defer cg.RUnlock()

	// 将多地址转换为IP地址
	ip, err := manet.ToIP(a)
	if err != nil {
		log.Warnf("将多地址转换为IP地址时出错: %s", err)
		return true
	}

	// 检查IP地址是否被阻止
	_, block := cg.blockedAddrs[ip.String()]
	if block {
		return false
	}

	// 检查IP地址是否在被阻止的子网中
	for _, ipnet := range cg.blockedSubnets {
		if ipnet.Contains(ip) {
			return false
		}
	}

	return true
}

// InterceptAccept 拦截接受连接请求。
// 参数:
//   - cma: 连接的多地址信息
//
// 返回:
//   - bool: 如果允许连接则返回true，否则返回false
func (cg *BasicConnectionGater) InterceptAccept(cma network.ConnMultiaddrs) (allow bool) {
	cg.RLock()
	defer cg.RUnlock()

	// 获取远程多地址
	a := cma.RemoteMultiaddr()

	// 将多地址转换为IP地址
	ip, err := manet.ToIP(a)
	if err != nil {
		log.Warnf("将多地址转换为IP地址时出错: %s", err)
		return true
	}

	// 检查IP地址是否被阻止
	_, block := cg.blockedAddrs[ip.String()]
	if block {
		return false
	}

	// 检查IP地址是否在被阻止的子网中
	for _, ipnet := range cg.blockedSubnets {
		if ipnet.Contains(ip) {
			return false
		}
	}

	return true
}

// InterceptSecured 拦截已建立安全连接的请求。
// 参数:
//   - dir: 连接方向
//   - p: 对等节点ID
//   - cma: 连接的多地址信息
//
// 返回:
//   - bool: 如果允许连接则返回true，否则返回false
func (cg *BasicConnectionGater) InterceptSecured(dir network.Direction, p peer.ID, cma network.ConnMultiaddrs) (allow bool) {
	// 对于出站连接，已在 InterceptPeerDial/InterceptAddrDial 中过滤
	if dir == network.DirOutbound {
		return true
	}

	// 对于入站连接，已在 InterceptAccept 中过滤了地址，这里只检查对等节点ID
	cg.RLock()
	defer cg.RUnlock()

	_, block := cg.blockedPeers[p]
	return !block
}

// InterceptUpgraded 拦截已升级的连接。
// 参数:
//   - network.Conn: 网络连接
//
// 返回:
//   - bool: 如果允许连接则返回true，否则返回false
//   - control.DisconnectReason: 断开连接的原因
func (cg *BasicConnectionGater) InterceptUpgraded(network.Conn) (allow bool, reason control.DisconnectReason) {
	return true, 0
}
