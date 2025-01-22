package routedhost

import (
	"context"
	"fmt"
	"time"

	"github.com/dep2p/go-dep2p/core/connmgr"
	"github.com/dep2p/go-dep2p/core/event"
	"github.com/dep2p/go-dep2p/core/host"
	"github.com/dep2p/go-dep2p/core/network"
	"github.com/dep2p/go-dep2p/core/peer"
	"github.com/dep2p/go-dep2p/core/peerstore"
	"github.com/dep2p/go-dep2p/core/protocol"

	logging "github.com/dep2p/log"

	ma "github.com/dep2p/go-dep2p/multiformats/multiaddr"
)

// 路由主机的日志记录器
var log = logging.Logger("host-routed")

// AddressTTL 是我们地址的过期时间
// 我们快速使其过期
const AddressTTL = time.Second * 10

// RoutedHost 是一个包含路由系统的 p2p 主机
// 这允许主机在没有对等节点地址时能够找到它们
type RoutedHost struct {
	host  host.Host // 嵌入的其他主机
	route Routing   // 路由接口
}

// Routing 定义了查找对等节点的接口
type Routing interface {
	// FindPeer 通过对等节点 ID 查找对等节点的地址信息
	FindPeer(context.Context, peer.ID) (peer.AddrInfo, error)
}

// Wrap 将普通主机和路由包装成路由主机
// 参数:
//   - h: 要包装的主机
//   - r: 路由接口实现
//
// 返回:
//   - *RoutedHost: 包装后的路由主机
func Wrap(h host.Host, r Routing) *RoutedHost {
	return &RoutedHost{h, r}
}

// Connect 确保此主机与给定对等节点 ID 的对等节点之间存在连接
// 如果主机没有对等节点的地址，它将使用路由系统尝试查找
// 参数:
//   - ctx: 上下文
//   - pi: 对等节点地址信息
//
// 返回:
//   - error: 连接错误
func (rh *RoutedHost) Connect(ctx context.Context, pi peer.AddrInfo) error {
	// 首先检查是否已经连接，除非强制直接拨号
	forceDirect, _ := network.GetForceDirectDial(ctx)
	canUseLimitedConn, _ := network.GetAllowLimitedConn(ctx)
	if !forceDirect {
		connectedness := rh.Network().Connectedness(pi.ID)
		if connectedness == network.Connected || (canUseLimitedConn && connectedness == network.Limited) {
			return nil
		}
	}

	// 如果提供了地址，保存并使用它们
	if len(pi.Addrs) > 0 {
		rh.Peerstore().AddAddrs(pi.ID, pi.Addrs, peerstore.TempAddrTTL)
	}

	// 检查最近内存中是否有一些地址
	addrs := rh.Peerstore().Addrs(pi.ID)
	if len(addrs) < 1 {
		// 没有地址？使用路由系统查找
		var err error
		addrs, err = rh.findPeerAddrs(ctx, pi.ID)
		if err != nil {
			log.Debugf("查找对等节点地址失败: %v", err)
			return err
		}
	}

	// Issue 448: 如果我们的地址集包含路由特定的中继地址，
	// 我们需要确保中继节点的地址本身在对等存储中，否则
	// 我们将无法拨号连接它
	for _, addr := range addrs {
		if _, err := addr.ValueForProtocol(ma.P_CIRCUIT); err != nil {
			// 不是中继地址
			continue
		}

		if addr.Protocols()[0].Code != ma.P_P2P {
			// 不是路由中继特定地址
			continue
		}

		relay, _ := addr.ValueForProtocol(ma.P_P2P)
		relayID, err := peer.Decode(relay)
		if err != nil {
			log.Debugf("解析地址 %s 中的中继 ID 失败: %s", relay, err)
			continue
		}

		if len(rh.Peerstore().Addrs(relayID)) > 0 {
			// 我们已经有这个中继的地址
			continue
		}

		relayAddrs, err := rh.findPeerAddrs(ctx, relayID)
		if err != nil {
			log.Debugf("查找中继 %s 失败: %s", relay, err)
			continue
		}

		rh.Peerstore().AddAddrs(relayID, relayAddrs, peerstore.TempAddrTTL)
	}

	// 如果到达这里，说明我们获得了一些地址，使用包装的主机进行连接
	pi.Addrs = addrs
	if cerr := rh.host.Connect(ctx, pi); cerr != nil {
		// 我们无法连接。让我们检查是否有该对等节点的最新地址
		// 如果有我们之前不知道的地址，我们再次尝试连接
		newAddrs, err := rh.findPeerAddrs(ctx, pi.ID)
		if err != nil {
			log.Debugf("查找更多对等节点地址 %s 失败: %s", pi.ID, err)
			return cerr
		}

		// 构建查找映射
		lookup := make(map[string]struct{}, len(addrs))
		for _, addr := range addrs {
			lookup[string(addr.Bytes())] = struct{}{}
		}

		// 如果有任何地址不在之前的地址集中
		// 再次尝试连接。如果所有地址之前都已知
		// 则返回原始错误
		for _, newAddr := range newAddrs {
			if _, found := lookup[string(newAddr.Bytes())]; found {
				continue
			}

			pi.Addrs = newAddrs
			return rh.host.Connect(ctx, pi)
		}
		// 没有找到合适的新地址
		// 返回原始拨号错误
		return cerr
	}
	return nil
}

// findPeerAddrs 查找对等节点的地址
// 参数:
//   - ctx: 上下文
//   - id: 对等节点 ID
//
// 返回:
//   - []ma.Multiaddr: 对等节点的多地址列表
//   - error: 查找错误
func (rh *RoutedHost) findPeerAddrs(ctx context.Context, id peer.ID) ([]ma.Multiaddr, error) {
	pi, err := rh.route.FindPeer(ctx, id)
	if err != nil {
		log.Debugf("查找对等节点地址失败: %v", err)
		return nil, err // 找不到任何地址 :(
	}

	if pi.ID != id {
		err = fmt.Errorf("路由失败：提供了不同对等节点的地址")
		log.Debugf("获得了错误的对等节点: %v", err)
		return nil, err
	}

	return pi.Addrs, nil
}

// ID 返回主机的对等节点 ID
// 返回:
//   - peer.ID: 主机的对等节点 ID
func (rh *RoutedHost) ID() peer.ID {
	// 返回内部主机的对等节点 ID
	return rh.host.ID()
}

// Peerstore 返回对等存储
// 返回:
//   - peerstore.Peerstore: 对等存储接口
func (rh *RoutedHost) Peerstore() peerstore.Peerstore {
	// 返回内部主机的对等存储
	return rh.host.Peerstore()
}

// Addrs 返回主机的监听地址列表
// 返回:
//   - []ma.Multiaddr: 主机监听的多地址列表
func (rh *RoutedHost) Addrs() []ma.Multiaddr {
	// 返回内部主机的监听地址列表
	return rh.host.Addrs()
}

// Network 返回主机的网络接口
// 返回:
//   - network.Network: 网络接口
func (rh *RoutedHost) Network() network.Network {
	// 返回内部主机的网络接口
	return rh.host.Network()
}

// Mux 返回协议多路复用器
// 返回:
//   - protocol.Switch: 协议多路复用器接口
func (rh *RoutedHost) Mux() protocol.Switch {
	// 返回内部主机的协议多路复用器
	return rh.host.Mux()
}

// EventBus 返回事件总线
// 返回:
//   - event.Bus: 事件总线接口
func (rh *RoutedHost) EventBus() event.Bus {
	// 返回内部主机的事件总线
	return rh.host.EventBus()
}

// SetStreamHandler 设置指定协议的流处理器
// 参数:
//   - pid: 协议 ID
//   - handler: 流处理器函数
func (rh *RoutedHost) SetStreamHandler(pid protocol.ID, handler network.StreamHandler) {
	// 在内部主机上设置流处理器
	rh.host.SetStreamHandler(pid, handler)
}

// SetStreamHandlerMatch 设置带匹配函数的流处理器
// 参数:
//   - pid: 协议 ID
//   - m: 协议匹配函数
//   - handler: 流处理器函数
func (rh *RoutedHost) SetStreamHandlerMatch(pid protocol.ID, m func(protocol.ID) bool, handler network.StreamHandler) {
	// 在内部主机上设置带匹配函数的流处理器
	rh.host.SetStreamHandlerMatch(pid, m, handler)
}

// RemoveStreamHandler 移除指定协议的流处理器
// 参数:
//   - pid: 要移除处理器的协议 ID
func (rh *RoutedHost) RemoveStreamHandler(pid protocol.ID) {
	// 从内部主机移除流处理器
	rh.host.RemoveStreamHandler(pid)
}

// NewStream 创建到指定对等节点的新流
// 参数:
//   - ctx: 上下文
//   - p: 目标对等节点 ID
//   - pids: 协议 ID 列表
//
// 返回:
//   - network.Stream: 创建的流
//   - error: 创建错误
func (rh *RoutedHost) NewStream(ctx context.Context, p peer.ID, pids ...protocol.ID) (network.Stream, error) {
	// 检查是否禁用拨号
	if nodial, _ := network.GetNoDial(ctx); !nodial {
		// 确保与目标对等节点建立连接
		err := rh.Connect(ctx, peer.AddrInfo{ID: p})
		if err != nil {
			return nil, err
		}
	}

	// 通过内部主机创建新流
	return rh.host.NewStream(ctx, p, pids...)
}

// Close 关闭主机
// 返回:
//   - error: 关闭错误
func (rh *RoutedHost) Close() error {
	// 仅关闭内部主机,不关闭路由系统
	return rh.host.Close()
}

// ConnManager 返回连接管理器
// 返回:
//   - connmgr.ConnManager: 连接管理器接口
func (rh *RoutedHost) ConnManager() connmgr.ConnManager {
	// 返回内部主机的连接管理器
	return rh.host.ConnManager()
}

// 确保 RoutedHost 实现了 host.Host 接口
var _ (host.Host) = (*RoutedHost)(nil)
