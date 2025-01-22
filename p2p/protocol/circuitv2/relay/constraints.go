package relay

import (
	"errors"
	"slices"
	"sync"
	"time"

	"github.com/dep2p/go-dep2p/core/peer"
	asnutil "github.com/dep2p/go-dep2p/p2plib/asn/util"

	ma "github.com/dep2p/go-dep2p/multiformats/multiaddr"
	manet "github.com/dep2p/go-dep2p/multiformats/multiaddr/net"
)

var (
	// 预约数量超过限制
	errTooManyReservations = errors.New("预约数量超过限制")
	// IP地址的预约对等点数量超过限制
	errTooManyReservationsForIP = errors.New("该IP地址的预约对等点数量超过限制")
	// ASN的预约对等点数量超过限制
	errTooManyReservationsForASN = errors.New("该ASN的预约对等点数量超过限制")
)

// peerWithExpiry 带过期时间的对等点信息
type peerWithExpiry struct {
	// 过期时间
	Expiry time.Time
	// 对等点ID
	Peer peer.ID
}

// constraints 实现各种预约约束
type constraints struct {
	// 资源配置
	rc *Resources

	// 互斥锁
	mutex sync.Mutex
	// 所有预约的对等点列表
	total []peerWithExpiry
	// IP地址到预约对等点的映射
	ips map[string][]peerWithExpiry
	// ASN到预约对等点的映射
	asns map[uint32][]peerWithExpiry
}

// newConstraints 创建一个新的约束对象
// 参数:
//   - rc: *Resources 资源配置对象
//
// 返回值:
//   - *constraints 约束对象
//
// 注意:
//   - 方法不是线程安全的,如果需要同步则必须在外部加锁
func newConstraints(rc *Resources) *constraints {
	return &constraints{
		rc:   rc,
		ips:  make(map[string][]peerWithExpiry),
		asns: make(map[uint32][]peerWithExpiry),
	}
}

// Reserve 为指定对等点添加预约
// 参数:
//   - p: peer.ID 对等点ID
//   - a: ma.Multiaddr 多地址
//   - expiry: time.Time 过期时间
//
// 返回值:
//   - error 如果添加预约违反IP、ASN或总预约数限制则返回错误
func (c *constraints) Reserve(p peer.ID, a ma.Multiaddr, expiry time.Time) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	// 获取当前时间
	now := time.Now()
	// 清理过期预约
	c.cleanup(now)
	// 清理该对等点的现有预约以正确处理刷新
	c.cleanupPeer(p)

	// 检查总预约数限制
	if len(c.total) >= c.rc.MaxReservations {
		log.Debugf("总预约数超过限制: %d", len(c.total))
		return errTooManyReservations
	}

	// 获取IP地址
	ip, err := manet.ToIP(a)
	if err != nil {
		log.Debugf("对等点没有关联的IP地址: %v", err)
		return err
	}

	// 检查IP预约数限制
	ipReservations := c.ips[ip.String()]
	if len(ipReservations) >= c.rc.MaxReservationsPerIP {
		log.Debugf("IP地址预约数超过限制: %d", len(ipReservations))
		return errTooManyReservationsForIP
	}

	// 检查ASN预约数限制(仅IPv6)
	var asnReservations []peerWithExpiry
	var asn uint32
	if ip.To4() == nil {
		asn = asnutil.AsnForIPv6(ip)
		if asn != 0 {
			asnReservations = c.asns[asn]
			if len(asnReservations) >= c.rc.MaxReservationsPerASN {
				log.Debugf("ASN预约数超过限制: %d", len(asnReservations))
				return errTooManyReservationsForASN
			}
		}
	}

	// 添加到总预约列表
	c.total = append(c.total, peerWithExpiry{Expiry: expiry, Peer: p})

	// 添加到IP预约映射
	ipReservations = append(ipReservations, peerWithExpiry{Expiry: expiry, Peer: p})
	c.ips[ip.String()] = ipReservations

	// 添加到ASN预约映射
	if asn != 0 {
		asnReservations = append(asnReservations, peerWithExpiry{Expiry: expiry, Peer: p})
		c.asns[asn] = asnReservations
	}
	return nil
}

// cleanup 清理过期的预约
// 参数:
//   - now: time.Time 当前时间
func (c *constraints) cleanup(now time.Time) {
	// 过期判断函数
	expireFunc := func(pe peerWithExpiry) bool {
		return pe.Expiry.Before(now)
	}
	// 清理总预约列表中的过期项
	c.total = slices.DeleteFunc(c.total, expireFunc)
	// 清理IP预约映射中的过期项
	for k, ipReservations := range c.ips {
		c.ips[k] = slices.DeleteFunc(ipReservations, expireFunc)
		if len(c.ips[k]) == 0 {
			delete(c.ips, k)
		}
	}
	// 清理ASN预约映射中的过期项
	for k, asnReservations := range c.asns {
		c.asns[k] = slices.DeleteFunc(asnReservations, expireFunc)
		if len(c.asns[k]) == 0 {
			delete(c.asns, k)
		}
	}
}

// cleanupPeer 清理指定对等点的所有预约
// 参数:
//   - p: peer.ID 要清理的对等点ID
func (c *constraints) cleanupPeer(p peer.ID) {
	// 对等点匹配函数
	removeFunc := func(pe peerWithExpiry) bool {
		return pe.Peer == p
	}
	// 从总预约列表中移除
	c.total = slices.DeleteFunc(c.total, removeFunc)
	// 从IP预约映射中移除
	for k, ipReservations := range c.ips {
		c.ips[k] = slices.DeleteFunc(ipReservations, removeFunc)
		if len(c.ips[k]) == 0 {
			delete(c.ips, k)
		}
	}
	// 从ASN预约映射中移除
	for k, asnReservations := range c.asns {
		c.asns[k] = slices.DeleteFunc(asnReservations, removeFunc)
		if len(c.asns[k]) == 0 {
			delete(c.asns, k)
		}
	}
}
