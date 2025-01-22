package autonatv2

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/dep2p/go-dep2p/core/event"
	"github.com/dep2p/go-dep2p/core/host"
	"github.com/dep2p/go-dep2p/core/network"
	"github.com/dep2p/go-dep2p/core/peer"
	ma "github.com/dep2p/go-dep2p/multiformats/multiaddr"
	manet "github.com/dep2p/go-dep2p/multiformats/multiaddr/net"
	"github.com/dep2p/go-dep2p/p2p/protocol/autonatv2/pb"
	logging "github.com/dep2p/log"
	"golang.org/x/exp/rand"
)

// ServiceName 是 AutoNAT v2 服务的名称
const (
	ServiceName = "dep2p.autonatv2"
	// DialBackProtocol 是用于回拨的协议标识符
	DialBackProtocol = "/dep2p/autonat/2/dial-back"
	// DialProtocol 是用于拨号请求的协议标识符
	DialProtocol = "/dep2p/autonat/2/dial-request"

	// maxMsgSize 是消息的最大字节数
	maxMsgSize = 8192
	// streamTimeout 是流的超时时间
	streamTimeout = 15 * time.Second
	// dialBackStreamTimeout 是回拨流的超时时间
	dialBackStreamTimeout = 5 * time.Second
	// dialBackDialTimeout 是回拨拨号的超时时间
	dialBackDialTimeout = 10 * time.Second
	// dialBackMaxMsgSize 是回拨消息的最大字节数
	dialBackMaxMsgSize = 1024
	// minHandshakeSizeBytes 是为防止放大攻击的最小握手字节数
	minHandshakeSizeBytes = 30_000
	// maxHandshakeSizeBytes 是最大握手字节数
	maxHandshakeSizeBytes = 100_000
	// maxPeerAddresses 是服务器将检查的拨号请求中的地址数量,超出部分将被忽略
	maxPeerAddresses = 50
)

var (
	// ErrNoValidPeers 表示没有有效的 AutoNAT v2 对等节点
	ErrNoValidPeers = errors.New("没有有效的 AutoNAT v2 对等节点")
	// ErrDialRefused 表示拨号被拒绝
	ErrDialRefused = errors.New("拨号被拒绝")

	// log 是 AutoNAT v2 的日志记录器
	log = logging.Logger("p2p-protocol-autonatv2")
)

// Request 是验证单个地址可达性的请求
// 参数:
//   - Addr: 要验证的多地址
//   - SendDialData: 如果服务器请求 Addr 的拨号数据,是否发送
type Request struct {
	// Addr 是要验证的多地址
	Addr ma.Multiaddr
	// SendDialData 表示如果服务器请求 Addr 的拨号数据,是否发送
	SendDialData bool
}

// Result 是 CheckReachability 调用的结果
// 参数:
//   - Addr: 已拨号的地址
//   - Reachability: 已拨号地址的可达性
//   - Status: 回拨的结果状态
type Result struct {
	// Addr 是已拨号的地址
	Addr ma.Multiaddr
	// Reachability 是已拨号地址的可达性
	Reachability network.Reachability
	// Status 是回拨的结果状态
	Status pb.DialStatus
}

// AutoNAT 实现了 AutoNAT v2 客户端和服务器
// 用户可以使用 CheckReachability 方法检查其地址的可达性
// 服务器提供放大攻击防护和速率限制
type AutoNAT struct {
	// host 是 dep2p 主机
	host host.Host

	// 用于清理关闭
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	srv *server
	cli *client

	mx    sync.Mutex
	peers *peersMap
	// allowPrivateAddrs 启用使用私有和本地主机地址进行可达性检查
	// 这仅用于测试
	allowPrivateAddrs bool
}

// New 返回一个新的 AutoNAT 实例
// 参数:
//   - host: dep2p 主机
//   - dialerHost: 用于拨号的主机
//   - opts: AutoNAT 选项
//
// 返回值:
//   - *AutoNAT: 新创建的 AutoNAT 实例
//   - error: 如果创建失败则返回错误
//
// 注意:
//   - host 和 dialerHost 应具有相同的拨号能力
//   - 如果主机不支持某个传输,则该传输的地址的回拨请求将被忽略
func New(host host.Host, dialerHost host.Host, opts ...AutoNATOption) (*AutoNAT, error) {
	s := defaultSettings()
	for _, o := range opts {
		if err := o(s); err != nil {
			log.Debugf("应用选项失败: %v", err)
			return nil, err
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	an := &AutoNAT{
		host:              host,
		ctx:               ctx,
		cancel:            cancel,
		srv:               newServer(host, dialerHost, s),
		cli:               newClient(host),
		allowPrivateAddrs: s.allowPrivateAddrs,
		peers:             newPeersMap(),
	}
	return an, nil
}

// background 处理订阅的事件
// 参数:
//   - sub: 事件订阅
func (an *AutoNAT) background(sub event.Subscription) {
	for {
		select {
		case <-an.ctx.Done():
			sub.Close()
			an.wg.Done()
			return
		case e := <-sub.Out():
			switch evt := e.(type) {
			case event.EvtPeerProtocolsUpdated:
				an.updatePeer(evt.Peer)
			case event.EvtPeerConnectednessChanged:
				an.updatePeer(evt.Peer)
			case event.EvtPeerIdentificationCompleted:
				an.updatePeer(evt.Peer)
			}
		}
	}
}

// Start 启动 AutoNAT 服务
// 返回值:
//   - error: 如果启动失败则返回错误
func (an *AutoNAT) Start() error {
	// 监听 event.EvtPeerProtocolsUpdated, event.EvtPeerConnectednessChanged event.EvtPeerIdentificationCompleted 事件以维护支持 autonat 的对等节点集
	sub, err := an.host.EventBus().Subscribe([]interface{}{
		new(event.EvtPeerProtocolsUpdated),
		new(event.EvtPeerConnectednessChanged),
		new(event.EvtPeerIdentificationCompleted),
	})
	if err != nil {
		log.Debugf("事件订阅失败: %v", err)
		return err
	}
	an.cli.Start()
	an.srv.Start()

	an.wg.Add(1)
	go an.background(sub)
	return nil
}

// Close 关闭 AutoNAT 服务
func (an *AutoNAT) Close() {
	an.cancel()
	an.wg.Wait()
	an.srv.Close()
	an.cli.Close()
	an.peers = nil
}

// GetReachability 发起单个拨号请求以检查请求地址的可达性
// 参数:
//   - ctx: 上下文
//   - reqs: 要检查的地址请求列表
//
// 返回值:
//   - Result: 可达性检查结果
//   - error: 如果检查失败则返回错误
func (an *AutoNAT) GetReachability(ctx context.Context, reqs []Request) (Result, error) {
	if !an.allowPrivateAddrs {
		for _, r := range reqs {
			if !manet.IsPublicAddr(r.Addr) {
				log.Debugf("私有地址无法通过 autonatv2 验证: %s", r.Addr)
				return Result{}, fmt.Errorf("私有地址无法通过 autonatv2 验证: %s", r.Addr)
			}
		}
	}
	an.mx.Lock()
	p := an.peers.GetRand()
	an.mx.Unlock()
	if p == "" {
		return Result{}, ErrNoValidPeers
	}

	res, err := an.cli.GetReachability(ctx, p, reqs)
	if err != nil {
		log.Debugf("与 %s 的可达性检查失败, 错误: %s", p, err)
		return Result{}, fmt.Errorf("与 %s 的可达性检查失败: %w", p, err)
	}
	log.Debugf("与 %s 的可达性检查成功", p)
	return res, nil
}

// updatePeer 更新对等节点状态
// 参数:
//   - p: 对等节点 ID
func (an *AutoNAT) updatePeer(p peer.ID) {
	an.mx.Lock()
	defer an.mx.Unlock()

	// identify 和 swarm 事件之间没有顺序保证
	// 检查 peerstore 和 swarm 的当前状态
	protos, err := an.host.Peerstore().SupportsProtocols(p, DialProtocol)
	connectedness := an.host.Network().Connectedness(p)
	if err == nil && slices.Contains(protos, DialProtocol) && connectedness == network.Connected {
		an.peers.Put(p)
	} else {
		an.peers.Delete(p)
	}
}

// peersMap 提供对对等节点集的随机访问
// 当映射迭代顺序不够随机时很有用
type peersMap struct {
	peerIdx map[peer.ID]int
	peers   []peer.ID
}

// newPeersMap 创建一个新的 peersMap
// 返回值:
//   - *peersMap: 新创建的 peersMap
func newPeersMap() *peersMap {
	return &peersMap{
		peerIdx: make(map[peer.ID]int),
		peers:   make([]peer.ID, 0),
	}
}

// GetRand 返回一个随机的对等节点 ID
// 返回值:
//   - peer.ID: 随机选择的对等节点 ID,如果没有对等节点则返回空
func (p *peersMap) GetRand() peer.ID {
	if len(p.peers) == 0 {
		return ""
	}
	return p.peers[rand.Intn(len(p.peers))]
}

// Put 添加一个对等节点
// 参数:
//   - pid: 要添加的对等节点 ID
func (p *peersMap) Put(pid peer.ID) {
	if _, ok := p.peerIdx[pid]; ok {
		return
	}
	p.peers = append(p.peers, pid)
	p.peerIdx[pid] = len(p.peers) - 1
}

// Delete 删除一个对等节点
// 参数:
//   - pid: 要删除的对等节点 ID
func (p *peersMap) Delete(pid peer.ID) {
	idx, ok := p.peerIdx[pid]
	if !ok {
		return
	}
	p.peers[idx] = p.peers[len(p.peers)-1]
	p.peerIdx[p.peers[idx]] = idx
	p.peers = p.peers[:len(p.peers)-1]
	delete(p.peerIdx, pid)
}
