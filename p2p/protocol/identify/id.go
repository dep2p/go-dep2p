package identify

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"sync"
	"time"

	"github.com/dep2p/go-dep2p/core/crypto"
	"github.com/dep2p/go-dep2p/core/event"
	"github.com/dep2p/go-dep2p/core/host"
	"github.com/dep2p/go-dep2p/core/network"
	"github.com/dep2p/go-dep2p/core/peer"
	"github.com/dep2p/go-dep2p/core/peerstore"
	"github.com/dep2p/go-dep2p/core/protocol"
	"github.com/dep2p/go-dep2p/core/record"
	"github.com/dep2p/go-dep2p/p2p/host/eventbus"
	"github.com/dep2p/go-dep2p/p2p/protocol/identify/pb"

	ma "github.com/dep2p/go-dep2p/multiformats/multiaddr"
	manet "github.com/dep2p/go-dep2p/multiformats/multiaddr/net"
	msmux "github.com/dep2p/go-dep2p/multiformats/multistream"
	"github.com/dep2p/go-dep2p/p2plib/msgio/pbio"
	logging "github.com/dep2p/log"
	"google.golang.org/protobuf/proto"
)

// log 是 identify 服务的日志记录器
var log = logging.Logger("p2p-protocol-identify")

// Timeout 是所有传入的 Identify 交互的超时时间
var Timeout = 30 * time.Second

const (
	// ID 是 identify 服务 1.0.0 版本的 protocol.ID
	ID = "/ipfs/id/1.0.0"

	// IDPush 是 Identify push 协议的 protocol.ID
	// 它发送包含对等节点当前状态的完整 identify 消息
	IDPush = "/ipfs/id/push/1.0.0"

	// ServiceName 是 identify 服务的名称
	ServiceName = "dep2p.identify"

	// legacyIDSize 是旧版 identify 消息的最大大小
	legacyIDSize = 2 * 1024

	// signedIDSize 是签名 identify 消息的最大大小
	signedIDSize = 8 * 1024

	// maxOwnIdentifyMsgSize 是我们自己发送的 identify 消息的最大大小
	// 比我们接受的小。这里是 4k 以兼容 rust-dep2p
	maxOwnIdentifyMsgSize = 4 * 1024

	// maxMessages 是最大消息数量
	maxMessages = 10

	// maxPushConcurrency 是最大并发推送数量
	maxPushConcurrency = 32

	// recentlyConnectedPeerMaxAddrs 是对于已断开连接的对等节点,在 peerstore.RecentlyConnectedTTL 时间内保留的地址数量
	// 这个数字可以很小,因为我们已经根据对等节点是否通过本地回环、私有 IP 或公共 IP 地址与我们连接来过滤对等节点地址
	recentlyConnectedPeerMaxAddrs = 20

	// connectedPeerMaxAddrs 是已连接对等节点的最大地址数量
	connectedPeerMaxAddrs = 500
)

// defaultUserAgent 是默认的用户代理字符串
var defaultUserAgent = "github.com/dep2p/go-dep2p"

// identifySnapshot 保存对等节点的标识信息快照
type identifySnapshot struct {
	// seq 是快照的序列号
	seq uint64
	// protocols 是支持的协议列表
	protocols []protocol.ID
	// addrs 是监听地址列表
	addrs []ma.Multiaddr
	// record 是签名的对等节点记录
	record *record.Envelope
}

// Equal 判断两个快照是否相同
// 参数:
//   - other: *identifySnapshot 要比较的另一个快照
//
// 返回值:
//   - bool 如果两个快照相同则返回 true,否则返回 false
//
// 注意:
//   - 它不比较序列号
func (s identifySnapshot) Equal(other *identifySnapshot) bool {
	hasRecord := s.record != nil
	otherHasRecord := other.record != nil
	if hasRecord != otherHasRecord {
		return false
	}
	if hasRecord && !s.record.Equal(other.record) {
		return false
	}
	if !slices.Equal(s.protocols, other.protocols) {
		return false
	}
	if len(s.addrs) != len(other.addrs) {
		return false
	}
	for i, a := range s.addrs {
		if !a.Equal(other.addrs[i]) {
			return false
		}
	}
	return true
}

// IDService 定义了 identify 服务的接口
type IDService interface {
	// IdentifyConn 同步触发连接上的 identify 请求并等待其完成
	// 如果连接正在被其他调用者识别,此调用将等待
	// 如果连接已经被识别,它将立即返回
	IdentifyConn(network.Conn)

	// IdentifyWait 触发 identify(如果连接尚未被识别)并返回一个通道
	// 该通道在 identify 协议完成时关闭
	IdentifyWait(network.Conn) <-chan struct{}

	// OwnObservedAddrs 返回对等节点报告我们拨号的地址
	OwnObservedAddrs() []ma.Multiaddr

	// ObservedAddrsFor 返回对等节点报告我们从特定本地地址拨号的地址
	ObservedAddrsFor(local ma.Multiaddr) []ma.Multiaddr

	// Start 启动 identify 服务
	Start()

	// Close 关闭 identify 服务
	io.Closer
}

// identifyPushSupport 表示对等节点对 identify push 的支持状态
type identifyPushSupport uint8

const (
	// identifyPushSupportUnknown 表示未知是否支持 identify push
	identifyPushSupportUnknown identifyPushSupport = iota
	// identifyPushSupported 表示支持 identify push
	identifyPushSupported
	// identifyPushUnsupported 表示不支持 identify push
	identifyPushUnsupported
)

// entry 保存连接的 identify 状态
type entry struct {
	// IdentifyWaitChan 在首次调用 IdentifyWait 时创建
	// 当 Identify 请求完成或失败时,IdentifyWait 关闭此通道
	IdentifyWaitChan chan struct{}

	// PushSupport 保存我们对对等节点支持 Identify Push 协议的了解
	// 在 identify 请求返回之前,我们还不知道对等节点是否支持 Identify Push
	PushSupport identifyPushSupport

	// Sequence 是我们发送给此对等节点的最后一个快照的序列号
	Sequence uint64
}

// idService 是实现 ProtocolIdentify 的结构
// 它是一个简单的服务,向其他对等节点提供一些有用的本地对等节点信息,类似于一个问候
//
// idService 发送:
//   - 我们的 dep2p 协议版本
//   - 我们的 dep2p 代理版本
//   - 我们的公共监听地址
type idService struct {
	// Host 是 dep2p 主机
	Host host.Host
	// UserAgent 是用户代理字符串
	UserAgent string
	// ProtocolVersion 是协议版本
	ProtocolVersion string

	// metricsTracer 用于跟踪指标
	metricsTracer MetricsTracer

	// setupCompleted 在 Start 完成设置时关闭
	setupCompleted chan struct{}
	// ctx 是服务的上下文
	ctx context.Context
	// ctxCancel 用于取消上下文
	ctxCancel context.CancelFunc
	// refCount 跟踪在关闭之前需要关闭的资源
	refCount sync.WaitGroup

	// disableSignedPeerRecord 表示是否禁用签名的对等节点记录
	disableSignedPeerRecord bool

	// connsMu 保护 conns map
	connsMu sync.RWMutex
	// conns 包含我们当前正在处理的所有连接
	// 连接在 swarm 中可用时立即插入
	// 当连接断开时,连接从 map 中删除
	conns map[network.Conn]entry

	// addrMu 保护地址相关字段
	addrMu sync.Mutex

	// observedAddrMgr 管理我们自己观察到的地址
	observedAddrMgr *ObservedAddrManager
	// disableObservedAddrManager 表示是否禁用观察地址管理器
	disableObservedAddrManager bool

	// emitters 包含各种事件发射器
	emitters struct {
		evtPeerProtocolsUpdated        event.Emitter
		evtPeerIdentificationCompleted event.Emitter
		evtPeerIdentificationFailed    event.Emitter
	}

	// currentSnapshot 保存当前的标识快照
	currentSnapshot struct {
		sync.Mutex
		snapshot identifySnapshot
	}

	// natEmitter 用于发出 NAT 相关事件
	natEmitter *natEmitter
}

// normalizer 定义了规范化多地址的接口
type normalizer interface {
	// NormalizeMultiaddr 规范化多地址
	NormalizeMultiaddr(ma.Multiaddr) ma.Multiaddr
}

// NewIDService 构造一个新的 *idService 并通过将其流处理程序附加到给定的 host.Host 来激活它
// 参数:
//   - h: host.Host dep2p 主机
//   - opts: ...Option 配置选项
//
// 返回值:
//   - *idService 新创建的 identify 服务
//   - error 如果创建失败则返回错误
func NewIDService(h host.Host, opts ...Option) (*idService, error) {
	var cfg config
	for _, opt := range opts {
		opt(&cfg)
	}

	userAgent := defaultUserAgent
	if cfg.userAgent != "" {
		userAgent = cfg.userAgent
	}

	ctx, cancel := context.WithCancel(context.Background())
	s := &idService{
		Host:                    h,
		UserAgent:               userAgent,
		ProtocolVersion:         cfg.protocolVersion,
		ctx:                     ctx,
		ctxCancel:               cancel,
		conns:                   make(map[network.Conn]entry),
		disableSignedPeerRecord: cfg.disableSignedPeerRecord,
		setupCompleted:          make(chan struct{}),
		metricsTracer:           cfg.metricsTracer,
	}

	var normalize func(ma.Multiaddr) ma.Multiaddr
	if hn, ok := h.(normalizer); ok {
		normalize = hn.NormalizeMultiaddr
	}

	var err error
	if cfg.disableObservedAddrManager {
		s.disableObservedAddrManager = true
	} else {
		observedAddrs, err := NewObservedAddrManager(h.Network().ListenAddresses,
			h.Addrs, h.Network().InterfaceListenAddresses, normalize)
		if err != nil {
			log.Debugf("创建观察地址管理器失败: %s", err)
			return nil, err
		}
		natEmitter, err := newNATEmitter(h, observedAddrs, time.Minute)
		if err != nil {
			log.Debugf("创建 NAT 发射器失败: %s", err)
			return nil, err
		}
		s.natEmitter = natEmitter
		s.observedAddrMgr = observedAddrs
	}

	s.emitters.evtPeerProtocolsUpdated, err = h.EventBus().Emitter(&event.EvtPeerProtocolsUpdated{})
	if err != nil {
		log.Debugf("identify 服务不发出对等协议更新; err: %s", err)
	}
	s.emitters.evtPeerIdentificationCompleted, err = h.EventBus().Emitter(&event.EvtPeerIdentificationCompleted{})
	if err != nil {
		log.Debugf("identify 服务不发出识别完成事件; err: %s", err)
	}
	s.emitters.evtPeerIdentificationFailed, err = h.EventBus().Emitter(&event.EvtPeerIdentificationFailed{})
	if err != nil {
		log.Debugf("identify 服务不发出识别失败事件; err: %s", err)
	}
	return s, nil
}

// Start 启动 identify 服务
func (ids *idService) Start() {
	ids.Host.Network().Notify((*netNotifiee)(ids))
	ids.Host.SetStreamHandler(ID, ids.handleIdentifyRequest)
	ids.Host.SetStreamHandler(IDPush, ids.handlePush)
	ids.updateSnapshot()
	close(ids.setupCompleted)

	ids.refCount.Add(1)
	go ids.loop(ids.ctx)
}

// loop 是 identify 服务的主循环
// 参数:
//   - ctx: context.Context 服务上下文
func (ids *idService) loop(ctx context.Context) {
	defer ids.refCount.Done()

	sub, err := ids.Host.EventBus().Subscribe(
		[]any{&event.EvtLocalProtocolsUpdated{}, &event.EvtLocalAddressesUpdated{}},
		eventbus.BufSize(256),
		eventbus.Name("identify (loop)"),
	)
	if err != nil {
		log.Errorf("订阅总线事件失败, err=%s", err)
		return
	}
	defer sub.Close()

	// 从单独的 Go 例程发送推送
	// 这样,我们可以得到:
	// * 这个 Go 例程在 sendPushes 中忙于循环所有对等节点
	// * 另一个推送在 triggerPush 通道中排队
	triggerPush := make(chan struct{}, 1)
	ids.refCount.Add(1)
	go func() {
		defer ids.refCount.Done()

		for {
			select {
			case <-ctx.Done():
				return
			case <-triggerPush:
				ids.sendPushes(ctx)
			}
		}
	}()

	for {
		select {
		case e, ok := <-sub.Out():
			if !ok {
				return
			}
			if updated := ids.updateSnapshot(); !updated {
				continue
			}
			if ids.metricsTracer != nil {
				ids.metricsTracer.TriggeredPushes(e)
			}
			select {
			case triggerPush <- struct{}{}:
			default: // 我们已经有一个推送排队,不需要再排队另一个
			}
		case <-ctx.Done():
			return
		}
	}
}

// sendPushes 向所有支持 identify push 的连接发送推送
// 参数:
//   - ctx: context.Context 推送上下文
func (ids *idService) sendPushes(ctx context.Context) {
	ids.connsMu.RLock()
	conns := make([]network.Conn, 0, len(ids.conns))
	for c, e := range ids.conns {
		// 即使我们不知道是否支持推送也要推送
		// 这只会在 IdentifyWaitChan 调用正在进行时发生
		if e.PushSupport == identifyPushSupported || e.PushSupport == identifyPushSupportUnknown {
			conns = append(conns, c)
		}
	}
	ids.connsMu.RUnlock()

	sem := make(chan struct{}, maxPushConcurrency)
	var wg sync.WaitGroup
	for _, c := range conns {
		// 检查连接是否仍然活着
		ids.connsMu.RLock()
		e, ok := ids.conns[c]
		ids.connsMu.RUnlock()
		if !ok {
			continue
		}
		// 检查我们是否已经向此对等节点发送了当前快照
		ids.currentSnapshot.Lock()
		snapshot := ids.currentSnapshot.snapshot
		ids.currentSnapshot.Unlock()
		if e.Sequence >= snapshot.seq {
			log.Debugw("已经向对等节点发送此快照", "peer", c.RemotePeer(), "seq", snapshot.seq)
			continue
		}
		// 我们还没有,现在发送
		sem <- struct{}{}
		wg.Add(1)
		go func(c network.Conn) {
			defer wg.Done()
			defer func() { <-sem }()
			ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()

			str, err := newStreamAndNegotiate(ctx, c, IDPush)
			if err != nil { // 连接可能最近已关闭
				return
			}
			// TODO: 如果我们没有任何关于推送支持的信息,找出对等节点是否支持推送
			if err := ids.sendIdentifyResp(str, true); err != nil {
				log.Debugw("发送 identify push 失败", "peer", c.RemotePeer(), "error", err)
				return
			}
		}(c)
	}
	wg.Wait()
}

// Close 关闭 identify 服务
// 返回值:
//   - error 如果关闭失败则返回错误
func (ids *idService) Close() error {
	ids.ctxCancel()
	if !ids.disableObservedAddrManager {
		ids.observedAddrMgr.Close()
		ids.natEmitter.Close()
	}
	ids.refCount.Wait()
	return nil
}

// OwnObservedAddrs 返回对等节点报告我们拨号的地址
// 返回值:
//   - []ma.Multiaddr 观察到的地址列表
func (ids *idService) OwnObservedAddrs() []ma.Multiaddr {
	if ids.disableObservedAddrManager {
		return nil
	}
	return ids.observedAddrMgr.Addrs()
}

// ObservedAddrsFor 返回对等节点报告我们从特定本地地址拨号的地址
// 参数:
//   - local: ma.Multiaddr 本地地址
//
// 返回值:
//   - []ma.Multiaddr 观察到的地址列表
func (ids *idService) ObservedAddrsFor(local ma.Multiaddr) []ma.Multiaddr {
	if ids.disableObservedAddrManager {
		return nil
	}
	return ids.observedAddrMgr.AddrsFor(local)
}

// IdentifyConn 在连接上运行 Identify 协议
// 参数:
//   - c: network.Conn 要运行 Identify 的连接
//
// 注意:
//   - 它在收到对等节点的 Identify 消息(或请求失败)时返回
//   - 如果成功,对等存储将包含对等节点的地址和支持的协议
func (ids *idService) IdentifyConn(c network.Conn) {
	<-ids.IdentifyWait(c)
}

// IdentifyWait 在连接上运行 Identify 协议
// 参数:
//   - c: network.Conn 要运行 Identify 的连接
//
// 返回值:
//   - <-chan struct{} 当收到对等节点的 Identify 消息(或请求失败)时关闭的通道
//
// 注意:
//   - 它不会阻塞并返回一个通道
//   - 如果成功,对等存储将包含对等节点的地址和支持的协议
func (ids *idService) IdentifyWait(c network.Conn) <-chan struct{} {
	// 获取连接锁
	ids.connsMu.Lock()
	defer ids.connsMu.Unlock()

	// 查找连接条目
	e, found := ids.conns[c]
	if !found {
		// 未找到条目,可能收到了无序通知
		// 检查连接是否应该存在(因为仍然连接)
		// 由于持有 ids.connsMu 锁,这是安全的,断开连接事件将在稍后处理
		if c.IsClosed() {
			log.Debugw("在 identify 服务中未找到连接", "peer", c.RemotePeer())
			ch := make(chan struct{})
			close(ch)
			return ch
		} else {
			ids.addConnWithLock(c)
		}
	}

	// 如果已存在等待通道,直接返回
	if e.IdentifyWaitChan != nil {
		return e.IdentifyWaitChan
	}

	// 首次调用此连接的 IdentifyWait,创建通道
	e.IdentifyWaitChan = make(chan struct{})
	ids.conns[c] = e

	// 启动 identify 过程
	go func() {
		defer close(e.IdentifyWaitChan)
		if err := ids.identifyConn(c); err != nil {
			log.Warnf("识别 %s 失败: %s", c.RemotePeer(), err)
			ids.emitters.evtPeerIdentificationFailed.Emit(event.EvtPeerIdentificationFailed{Peer: c.RemotePeer(), Reason: err})
			return
		}
	}()

	return e.IdentifyWaitChan
}

// newStreamAndNegotiate 在给定连接上打开新流并协商协议
// 参数:
//   - ctx: context.Context 上下文
//   - c: network.Conn 要打开流的连接
//   - proto: protocol.ID 要协商的协议
//
// 返回值:
//   - network.Stream 协商后的流
//   - error 错误信息
func newStreamAndNegotiate(ctx context.Context, c network.Conn, proto protocol.ID) (network.Stream, error) {
	// 打开新流
	s, err := c.NewStream(network.WithAllowLimitedConn(ctx, "identify"))
	if err != nil {
		log.Debugf("打开 identify 流错误", "peer", c.RemotePeer(), "error", err)
		return nil, err
	}

	// 设置超时
	_ = s.SetDeadline(time.Now().Add(Timeout))

	// 设置协议
	if err := s.SetProtocol(proto); err != nil {
		log.Debugf("为流设置 identify 协议时出错: %s", err)
		_ = s.Reset()
	}

	// 协商协议
	if err := msmux.SelectProtoOrFail(proto, s); err != nil {
		log.Debugf("与对等节点协商 identify 协议失败", "peer", c.RemotePeer(), "error", err)
		_ = s.Reset()
		return nil, err
	}
	return s, nil
}

// identifyConn 对给定连接执行 identify 过程
// 参数:
//   - c: network.Conn 要执行 identify 的连接
//
// 返回值:
//   - error 错误信息
func (ids *idService) identifyConn(c network.Conn) error {
	// 创建带超时的上下文
	ctx, cancel := context.WithTimeout(context.Background(), Timeout)
	defer cancel()

	// 打开并协商流
	s, err := newStreamAndNegotiate(network.WithAllowLimitedConn(ctx, "identify"), c, ID)
	if err != nil {
		log.Debugf("打开 identify 流错误", "peer", c.RemotePeer(), "error", err)
		return err
	}

	return ids.handleIdentifyResponse(s, false)
}

// handlePush 处理传入的 identify push 流
// 参数:
//   - s: network.Stream 要处理的流
func (ids *idService) handlePush(s network.Stream) {
	s.SetDeadline(time.Now().Add(Timeout))
	ids.handleIdentifyResponse(s, true)
}

// handleIdentifyRequest 处理传入的 identify 请求
// 参数:
//   - s: network.Stream 要处理的流
func (ids *idService) handleIdentifyRequest(s network.Stream) {
	_ = ids.sendIdentifyResp(s, false)
}

// sendIdentifyResp 发送 identify 响应
// 参数:
//   - s: network.Stream 要发送响应的流
//   - isPush: bool 是否为 push 类型
//
// 返回值:
//   - error 错误信息
func (ids *idService) sendIdentifyResp(s network.Stream, isPush bool) error {
	// 设置流的服务
	if err := s.Scope().SetService(ServiceName); err != nil {
		log.Debugf("将流附加到 identify 服务失败: %w", err)
		s.Reset()
		return err
	}
	defer s.Close()

	// 获取当前快照
	ids.currentSnapshot.Lock()
	snapshot := ids.currentSnapshot.snapshot
	ids.currentSnapshot.Unlock()

	log.Debugw("发送快照", "seq", snapshot.seq, "protocols", snapshot.protocols, "addrs", snapshot.addrs)

	// 创建响应消息
	mes := ids.createBaseIdentifyResponse(s.Conn(), &snapshot)
	mes.SignedPeerRecord = ids.getSignedRecord(&snapshot)

	log.Debugf("%s 向 %s %s 发送消息", ID, s.Conn().RemotePeer(), s.Conn().RemoteMultiaddr())
	if err := ids.writeChunkedIdentifyMsg(s, mes); err != nil {
		log.Debugf("写入 identify 消息失败: %v", err)
		return err
	}

	// 更新指标
	if ids.metricsTracer != nil {
		ids.metricsTracer.IdentifySent(isPush, len(mes.Protocols), len(mes.ListenAddrs))
	}

	// 更新连接状态
	ids.connsMu.Lock()
	defer ids.connsMu.Unlock()
	e, ok := ids.conns[s.Conn()]
	// 连接可能已经关闭。
	// 我们*应该*在能够接受对等节点的 Identify 流之前从 swarm 收到 Connected 通知,但如果由于某种原因不起作用,我们也不会在这里有一个 map 条目。
	// 唯一的后果是我们稍后会向该对等节点发送一个虚假的 Push。
	if !ok {
		return nil
	}
	e.Sequence = snapshot.seq
	ids.conns[s.Conn()] = e
	return nil
}

// handleIdentifyResponse 处理 identify 响应
// 参数:
//   - s: network.Stream 要处理的流
//   - isPush: bool 是否为 push 类型
//
// 返回值:
//   - error 错误信息
func (ids *idService) handleIdentifyResponse(s network.Stream, isPush bool) error {
	// 设置流的服务和内存限制
	if err := s.Scope().SetService(ServiceName); err != nil {
		log.Debugf("将流附加到 identify 服务时出错: %s", err)
		s.Reset()
		return err
	}

	if err := s.Scope().ReserveMemory(signedIDSize, network.ReservationPriorityAlways); err != nil {
		log.Debugf("为 identify 流预留内存时出错: %s", err)
		s.Reset()
		return err
	}
	defer s.Scope().ReleaseMemory(signedIDSize)

	c := s.Conn()

	// 读取响应消息
	r := pbio.NewDelimitedReader(s, signedIDSize)
	mes := &pb.Identify{}

	if err := readAllIDMessages(r, mes); err != nil {
		log.Debugf("读取 identify 消息时出错: %v", err)
		s.Reset()
		return err
	}

	defer s.Close()

	log.Debugf("%s 从 %s %s 收到消息", s.Protocol(), c.RemotePeer(), c.RemoteMultiaddr())

	// 处理消息
	ids.consumeMessage(mes, c, isPush)

	// 更新指标
	if ids.metricsTracer != nil {
		ids.metricsTracer.IdentifyReceived(isPush, len(mes.Protocols), len(mes.ListenAddrs))
	}

	// 更新连接状态
	ids.connsMu.Lock()
	defer ids.connsMu.Unlock()
	e, ok := ids.conns[c]
	if !ok { // 可能已断开连接
		return nil
	}

	// 检查是否支持 push
	sup, err := ids.Host.Peerstore().SupportsProtocols(c.RemotePeer(), IDPush)
	if supportsIdentifyPush := err == nil && len(sup) > 0; supportsIdentifyPush {
		e.PushSupport = identifyPushSupported
	} else {
		e.PushSupport = identifyPushUnsupported
	}

	// 更新指标
	if ids.metricsTracer != nil {
		ids.metricsTracer.ConnPushSupport(e.PushSupport)
	}

	ids.conns[c] = e
	return nil
}

// readAllIDMessages 读取所有 identify 消息并合并到最终消息中
// 参数:
//   - r: pbio.Reader 消息读取器
//   - finalMsg: proto.Message 最终消息
//
// 返回值:
//   - error 错误信息
func readAllIDMessages(r pbio.Reader, finalMsg proto.Message) error {
	mes := &pb.Identify{}
	for i := 0; i < maxMessages; i++ {
		switch err := r.ReadMsg(mes); err {
		case io.EOF:
			return nil
		case nil:
			proto.Merge(finalMsg, mes)
		default:
			log.Debugf("读取 identify 消息时出错: %v", err)
			return err
		}
	}

	return fmt.Errorf("太多部分")
}

// updateSnapshot 更新当前快照
// 返回值:
//   - bool 是否有更新
func (ids *idService) updateSnapshot() (updated bool) {
	// 获取协议列表并排序
	protos := ids.Host.Mux().Protocols()
	slices.Sort(protos)

	// 获取地址列表并排序
	addrs := ids.Host.Addrs()
	slices.SortFunc(addrs, func(a, b ma.Multiaddr) int { return bytes.Compare(a.Bytes(), b.Bytes()) })

	// 计算已使用空间
	usedSpace := len(ids.ProtocolVersion) + len(ids.UserAgent)
	for i := 0; i < len(protos); i++ {
		usedSpace += len(protos[i])
	}
	// 裁剪地址列表以适应消息大小限制
	addrs = trimHostAddrList(addrs, maxOwnIdentifyMsgSize-usedSpace-256) // 256 字节的缓冲区

	// 创建新快照
	snapshot := identifySnapshot{
		addrs:     addrs,
		protocols: protos,
	}

	// 获取签名记录
	if !ids.disableSignedPeerRecord {
		if cab, ok := peerstore.GetCertifiedAddrBook(ids.Host.Peerstore()); ok {
			snapshot.record = cab.GetPeerRecord(ids.Host.ID())
		}
	}

	ids.currentSnapshot.Lock()
	defer ids.currentSnapshot.Unlock()

	// 检查是否有变化
	if ids.currentSnapshot.snapshot.Equal(&snapshot) {
		return false
	}

	// 更新序列号和快照
	snapshot.seq = ids.currentSnapshot.snapshot.seq + 1
	ids.currentSnapshot.snapshot = snapshot

	log.Debugw("更新快照", "seq", snapshot.seq, "addrs", snapshot.addrs)
	return true
}

// writeChunkedIdentifyMsg 分块写入 identify 消息
// 参数:
//   - s: network.Stream 要写入的流
//   - mes: *pb.Identify 要写入的消息
//
// 返回值:
//   - error 错误信息
func (ids *idService) writeChunkedIdentifyMsg(s network.Stream, mes *pb.Identify) error {
	writer := pbio.NewDelimitedWriter(s)

	// 如果消息较小或没有签名记录,直接写入
	if mes.SignedPeerRecord == nil || proto.Size(mes) <= legacyIDSize {
		return writer.WriteMsg(mes)
	}

	// 分两部分写入:先写入基本消息,再写入签名记录
	sr := mes.SignedPeerRecord
	mes.SignedPeerRecord = nil
	if err := writer.WriteMsg(mes); err != nil {
		log.Debugf("写入 identify 消息失败: %v", err)
		return err
	}
	// 然后只写签名记录
	return writer.WriteMsg(&pb.Identify{SignedPeerRecord: sr})
}

// createBaseIdentifyResponse 创建基本的 identify 响应消息
// 参数:
//   - conn: network.Conn 网络连接对象
//   - snapshot: *identifySnapshot 标识快照对象
//
// 返回值:
//   - *pb.Identify identify 响应消息
func (ids *idService) createBaseIdentifyResponse(conn network.Conn, snapshot *identifySnapshot) *pb.Identify {
	// 创建新的 identify 消息
	mes := &pb.Identify{}

	// 获取远程和本地地址
	remoteAddr := conn.RemoteMultiaddr()
	localAddr := conn.LocalMultiaddr()

	// 设置此节点当前处理的协议
	mes.Protocols = protocol.ConvertToStrings(snapshot.protocols)

	// 设置观察到的地址,以便另一方知道他们的"公共"地址
	mes.ObservedAddr = remoteAddr.Bytes()

	// 填充未签名的地址
	// 尚不支持签名地址的对等节点将需要这个
	// 注意:LocalMultiaddr 有时是 0.0.0.0
	viaLoopback := manet.IsIPLoopback(localAddr) || manet.IsIPLoopback(remoteAddr)
	mes.ListenAddrs = make([][]byte, 0, len(snapshot.addrs))
	for _, addr := range snapshot.addrs {
		// 如果不是通过本地回环连接且地址是本地回环地址,则跳过
		if !viaLoopback && manet.IsIPLoopback(addr) {
			continue
		}
		mes.ListenAddrs = append(mes.ListenAddrs, addr.Bytes())
	}

	// 获取本节点的公钥
	ownKey := ids.Host.Peerstore().PubKey(ids.Host.ID())

	// 检查是否有公钥
	if ownKey == nil {
		// 公钥为空,检查是否在"安全模式"下运行
		if ids.Host.Peerstore().PrivKey(ids.Host.ID()) != nil {
			// 私钥存在但公钥不存在,记录错误
			log.Errorf("在 Peerstore 中没有自己的公钥")
		}
		// 如果两个密钥都不存在,说明使用的是不安全的传输
	} else {
		// 公钥存在,将其转换为字节并设置到消息中
		if kb, err := crypto.MarshalPublicKey(ownKey); err != nil {
			log.Errorf("将密钥转换为字节失败")
		} else {
			mes.PublicKey = kb
		}
	}

	// 设置协议版本和用户代理
	mes.ProtocolVersion = &ids.ProtocolVersion
	mes.AgentVersion = &ids.UserAgent

	return mes
}

// getSignedRecord 获取签名的对等节点记录
// 参数:
//   - snapshot: *identifySnapshot 标识快照对象
//
// 返回值:
//   - []byte 序列化后的签名记录,如果禁用或失败则返回 nil
func (ids *idService) getSignedRecord(snapshot *identifySnapshot) []byte {
	// 如果禁用签名记录或记录为空则返回 nil
	if ids.disableSignedPeerRecord || snapshot.record == nil {
		log.Debugf("禁用签名记录或记录为空")
		return nil
	}

	// 序列化记录
	recBytes, err := snapshot.record.Marshal()
	if err != nil {
		log.Debugf("序列化签名记录失败", "err", err)
		return nil
	}

	return recBytes
}

// diff 计算两个协议 ID 切片的差异
// 接受两个字符串切片(a 和 b)并计算在 b 中添加和删除了哪些元素
// 参数:
//   - a: []protocol.ID 原始协议 ID 切片
//   - b: []protocol.ID 新的协议 ID 切片
//
// 返回值:
//   - added: []protocol.ID 新增的协议 ID
//   - removed: []protocol.ID 移除的协议 ID
func diff(a, b []protocol.ID) (added, removed []protocol.ID) {
	// 计算新增的协议
	// 这是 O(n^2),但没关系,因为切片很小。
	for _, x := range b {
		var found bool
		for _, y := range a {
			if x == y {
				found = true
				break
			}
		}
		if !found {
			added = append(added, x)
		}
	}

	// 计算移除的协议
	for _, x := range a {
		var found bool
		for _, y := range b {
			if x == y {
				found = true
				break
			}
		}
		if !found {
			removed = append(removed, x)
		}
	}
	return
}

// consumeMessage 处理 identify 消息
// 参数:
//   - mes: *pb.Identify identify 消息
//   - c: network.Conn 网络连接对象
//   - isPush: bool 是否为 push 类型消息
func (ids *idService) consumeMessage(mes *pb.Identify, c network.Conn, isPush bool) {
	// 获取远程对等节点 ID
	p := c.RemotePeer()

	// 获取对等节点支持的协议
	supported, _ := ids.Host.Peerstore().GetProtocols(p)
	// 将消息中的协议字符串转换为协议 ID
	mesProtocols := protocol.ConvertFromStrings(mes.Protocols)
	// 计算协议变更
	added, removed := diff(supported, mesProtocols)
	// 更新对等节点支持的协议
	ids.Host.Peerstore().SetProtocols(p, mesProtocols...)
	// 如果是 push 消息,发出协议更新事件
	if isPush {
		ids.emitters.evtPeerProtocolsUpdated.Emit(event.EvtPeerProtocolsUpdated{
			Peer:    p,
			Added:   added,
			Removed: removed,
		})
	}

	// 解析观察到的地址
	obsAddr, err := ma.NewMultiaddrBytes(mes.GetObservedAddr())
	if err != nil {
		log.Debugf("解析收到的观察地址失败 %s: %s", c, err)
		obsAddr = nil
	}

	// 记录观察到的地址
	if obsAddr != nil && !ids.disableObservedAddrManager {
		// TODO: 重构此处,使用事件而不是显式函数调用
		ids.observedAddrMgr.Record(c, obsAddr)
	}

	// 解析监听地址
	laddrs := mes.GetListenAddrs()
	lmaddrs := make([]ma.Multiaddr, 0, len(laddrs))
	for _, addr := range laddrs {
		maddr, err := ma.NewMultiaddrBytes(addr)
		if err != nil {
			log.Debugf("%s 从 %s %s 解析多地址失败", ID,
				p, c.RemoteMultiaddr())
			continue
		}
		lmaddrs = append(lmaddrs, maddr)
	}

	// 注意: 如果远程对等节点没有告诉我们这样做,不要将 c.RemoteMultiaddr() 添加到 peerstore。否则,我们会广播它。
	//
	// 这可能导致"地址爆炸"问题,网络会慢慢收集和传播观察到但未公布的地址。
	// 对于选择随机源端口的 NAT,这可能导致 DHT 节点收集大量其他对等节点的不可拨号地址。

	// 如果对等节点发送了签名的对等节点记录,则添加认证的地址,否则使用未签名的地址
	signedPeerRecord, err := signedPeerRecordFromMessage(mes)
	if err != nil {
		log.Errorf("从 Identify 消息获取对等节点记录失败: %v", err)
	}

	// 延长已知(可能)良好地址的 TTL。
	// 加锁确保不会并发处理断开连接。
	ids.addrMu.Lock()
	ttl := peerstore.RecentlyConnectedAddrTTL
	switch ids.Host.Network().Connectedness(p) {
	case network.Limited, network.Connected:
		ttl = peerstore.ConnectedAddrTTL
	}

	// 将已连接和最近连接的地址降级为临时 TTL
	for _, ttl := range []time.Duration{
		peerstore.RecentlyConnectedAddrTTL,
		peerstore.ConnectedAddrTTL,
	} {
		ids.Host.Peerstore().UpdateAddrs(p, ttl, peerstore.TempAddrTTL)
	}

	// 处理地址
	var addrs []ma.Multiaddr
	if signedPeerRecord != nil {
		signedAddrs, err := ids.consumeSignedPeerRecord(c.RemotePeer(), signedPeerRecord)
		if err != nil {
			log.Debugf("处理签名对等节点记录失败: %s", err)
			signedPeerRecord = nil
		} else {
			addrs = signedAddrs
		}
	} else {
		addrs = lmaddrs
	}
	// 过滤地址
	addrs = filterAddrs(addrs, c.RemoteMultiaddr())
	if len(addrs) > connectedPeerMaxAddrs {
		addrs = addrs[:connectedPeerMaxAddrs]
	}

	// 添加地址到 peerstore
	ids.Host.Peerstore().AddAddrs(p, addrs, ttl)

	// 最后,使所有临时地址过期
	ids.Host.Peerstore().UpdateAddrs(p, peerstore.TempAddrTTL, 0)
	ids.addrMu.Unlock()

	log.Debugf("%s 收到 %s 的监听地址: %s", c.LocalPeer(), c.RemotePeer(), addrs)

	// 获取协议版本
	pv := mes.GetProtocolVersion()
	av := mes.GetAgentVersion()

	// 存储版本信息
	ids.Host.Peerstore().Put(p, "ProtocolVersion", pv)
	ids.Host.Peerstore().Put(p, "AgentVersion", av)

	// 获取对方的公钥(可能没有,如无认证传输)
	ids.consumeReceivedPubKey(c, mes.PublicKey)

	// 发出对等节点识别完成事件
	ids.emitters.evtPeerIdentificationCompleted.Emit(event.EvtPeerIdentificationCompleted{
		Peer:             c.RemotePeer(),
		Conn:             c,
		ListenAddrs:      lmaddrs,
		Protocols:        mesProtocols,
		SignedPeerRecord: signedPeerRecord,
		ObservedAddr:     obsAddr,
		ProtocolVersion:  pv,
		AgentVersion:     av,
	})
}

// consumeSignedPeerRecord 处理签名的对等节点记录
// 参数:
//   - p: peer.ID 对等节点 ID
//   - signedPeerRecord: *record.Envelope 签名的对等节点记录信封
//
// 返回值:
//   - []ma.Multiaddr 记录中的地址列表
//   - error 处理过程中的错误
func (ids *idService) consumeSignedPeerRecord(p peer.ID, signedPeerRecord *record.Envelope) ([]ma.Multiaddr, error) {
	if signedPeerRecord.PublicKey == nil {
		log.Debugf("缺少公钥")
		return nil, errors.New("缺少公钥")
	}
	id, err := peer.IDFromPublicKey(signedPeerRecord.PublicKey)
	if err != nil {
		log.Debugf("从公钥派生对等节点 ID 失败: %s", err)
		return nil, err
	}
	if id != p {
		log.Debugf("收到的签名对等节点记录信封的对等节点 ID 不符。期望 %s, 得到 %s", p, id)
		return nil, fmt.Errorf("收到的签名对等节点记录信封的对等节点 ID 不符。期望 %s, 得到 %s", p, id)
	}
	r, err := signedPeerRecord.Record()
	if err != nil {
		log.Debugf("获取记录失败: %w", err)
		return nil, fmt.Errorf("获取记录失败: %w", err)
	}
	rec, ok := r.(*peer.PeerRecord)
	if !ok {
		log.Debugf("不是对等节点记录")
		return nil, errors.New("不是对等节点记录")
	}
	if rec.PeerID != p {
		log.Debugf("收到的签名对等节点记录的对等节点 ID 不符。期望 %s, 得到 %s", p, rec.PeerID)
		return nil, fmt.Errorf("收到的签名对等节点记录的对等节点 ID 不符。期望 %s, 得到 %s", p, rec.PeerID)
	}
	// 不要将签名的对等节点记录放入 peerstore
	// 它们没有在任何地方使用
	// 我们只关心地址
	return rec.Addrs, nil
}

// consumeReceivedPubKey 处理收到的公钥
// 参数:
//   - c: network.Conn 网络连接对象
//   - kb: []byte 公钥字节
func (ids *idService) consumeReceivedPubKey(c network.Conn, kb []byte) {
	lp := c.LocalPeer()
	rp := c.RemotePeer()

	if kb == nil {
		log.Debugf("%s 没有收到远程对等节点的公钥: %s", lp, rp)
		return
	}

	// 解析公钥
	newKey, err := crypto.UnmarshalPublicKey(kb)
	if err != nil {
		log.Debugf("%s 无法解析远程对等节点的密钥: %s, %s", lp, rp, err)
		return
	}

	// 验证密钥是否匹配对等节点 ID
	np, err := peer.IDFromPublicKey(newKey)
	if err != nil {
		log.Debugf("%s 无法从远程对等节点的密钥获取对等节点 ID: %s, %s", lp, rp, err)
		return
	}

	if np != rp {
		// 如果新密钥的对等节点 ID 与已知的对等节点 ID 不匹配...

		if rp == "" && np != "" {
			// 如果本地对等节点 ID 为空,则使用新发送的密钥
			err := ids.Host.Peerstore().AddPubKey(rp, newKey)
			if err != nil {
				log.Errorf("%s 无法将 %s 的密钥添加到 peerstore: %s", lp, rp, err)
			}

		} else {
			// 我们有本地对等节点 ID 且它与发送的密钥不匹配...错误
			log.Errorf("%s 收到的远程对等节点 %s 的密钥不匹配: %s", lp, rp, np)
		}
		return
	}

	// 获取当前密钥
	currKey := ids.Host.Peerstore().PubKey(rp)
	if currKey == nil {
		// 没有密钥?没有认证传输。设置这个密钥
		err := ids.Host.Peerstore().AddPubKey(rp, newKey)
		if err != nil {
			log.Debugf("%s 无法将 %s 的密钥添加到 peerstore: %s", lp, rp, err)
		}
		return
	}

	// 好的,我们有本地密钥,我们应该验证它们是否匹配
	if currKey.Equals(newKey) {
		return // 很好。我们完成了
	}

	// 奇怪,得到了一个不同的密钥...但不同的密钥匹配对等节点 ID
	// 这很奇怪。让我们记录错误并调查
	// 这基本上不应该发生,这意味着我们有一些奇怪的情况,可能是一个 bug
	log.Errorf("%s identify 为 %s 获得了一个不同的密钥", lp, rp)

	// 好吧...我们的密钥是否与远程对等节点 ID 不匹配?
	cp, err := peer.IDFromPublicKey(currKey)
	if err != nil {
		log.Errorf("%s 无法从远程对等节点的本地密钥获取对等节点 ID: %s, %s", lp, rp, err)
		return
	}
	if cp != rp {
		log.Errorf("%s 远程对等节点 %s 的本地密钥产生了不同的对等节点 ID: %s", lp, rp, cp)
		return
	}

	// 好吧...当前密钥不匹配新密钥。两者都匹配对等节点 ID。怎么回事?
	log.Errorf("%s %s 的本地密钥和收到的密钥不匹配,但匹配对等节点 ID", lp, rp)
}

// HasConsistentTransport 如果地址 'a' 与绿色集合中的任何地址共享协议集,则返回 true
// 这用于检查给定地址是否可能是对等节点正在监听的地址之一
// 参数:
//   - a: ma.Multiaddr 要检查的地址
//   - green: []ma.Multiaddr 绿色地址集合
//
// 返回值:
//   - bool 是否共享协议集
func HasConsistentTransport(a ma.Multiaddr, green []ma.Multiaddr) bool {
	// 检查两个协议列表是否匹配
	protosMatch := func(a, b []ma.Protocol) bool {
		if len(a) != len(b) {
			return false
		}

		for i, p := range a {
			if b[i].Code != p.Code {
				return false
			}
		}
		return true
	}

	// 获取地址的协议
	protos := a.Protocols()

	// 与每个绿色地址比较
	for _, ga := range green {
		if protosMatch(protos, ga.Protocols()) {
			return true
		}
	}

	return false
}

// addConnWithLock 添加连接,假定调用者持有 connsMu 锁
// 参数:
//   - c: network.Conn 要添加的连接
func (ids *idService) addConnWithLock(c network.Conn) {
	_, found := ids.conns[c]
	if !found {
		<-ids.setupCompleted
		ids.conns[c] = entry{}
	}
}

// signedPeerRecordFromMessage 从消息中获取签名的对等节点记录
// 参数:
//   - msg: *pb.Identify identify 消息
//
// 返回值:
//   - *record.Envelope 签名的对等节点记录信封
//   - error 处理过程中的错误
func signedPeerRecordFromMessage(msg *pb.Identify) (*record.Envelope, error) {
	if len(msg.SignedPeerRecord) == 0 {
		return nil, nil
	}
	env, _, err := record.ConsumeEnvelope(msg.SignedPeerRecord, peer.PeerRecordEnvelopeDomain)
	return env, err
}

// netNotifiee 定义用于 swarm 的方法
type netNotifiee idService

// IDService 返回 idService 实例
// 返回值:
//   - *idService idService 实例
func (nn *netNotifiee) IDService() *idService {
	return (*idService)(nn)
}

// Connected 处理连接建立事件
// 参数:
//   - _: network.Network 网络对象(未使用)
//   - c: network.Conn 建立的连接
func (nn *netNotifiee) Connected(_ network.Network, c network.Conn) {
	ids := nn.IDService()

	ids.connsMu.Lock()
	ids.addConnWithLock(c)
	ids.connsMu.Unlock()

	nn.IDService().IdentifyWait(c)
}

// Disconnected 处理连接断开事件
// 参数:
//   - _: network.Network 网络对象(未使用)
//   - c: network.Conn 断开的连接
func (nn *netNotifiee) Disconnected(_ network.Network, c network.Conn) {
	ids := nn.IDService()

	// 停止跟踪连接
	ids.connsMu.Lock()
	delete(ids.conns, c)
	ids.connsMu.Unlock()

	if !ids.disableObservedAddrManager {
		ids.observedAddrMgr.removeConn(c)
	}

	// 最后一次断开连接
	// 撤销我们之前对地址设置的 peer.ConnectedAddrTTL
	ids.addrMu.Lock()
	defer ids.addrMu.Unlock()

	// 此检查必须在获取锁之后进行,因为在不同连接上的 identify 可能正在尝试添加地址
	switch ids.Host.Network().Connectedness(c.RemotePeer()) {
	case network.Connected, network.Limited:
		return
	}
	// peerstore 使用 map 存储地址,因此返回的元素顺序是随机的
	addrs := ids.Host.Peerstore().Addrs(c.RemotePeer())
	n := len(addrs)
	if n > recentlyConnectedPeerMaxAddrs {
		// 我们总是要保存我们连接到的地址
		for i, a := range addrs {
			if a.Equal(c.RemoteMultiaddr()) {
				addrs[i], addrs[0] = addrs[0], addrs[i]
			}
		}
		n = recentlyConnectedPeerMaxAddrs
	}
	ids.Host.Peerstore().UpdateAddrs(c.RemotePeer(), peerstore.ConnectedAddrTTL, peerstore.TempAddrTTL)
	ids.Host.Peerstore().AddAddrs(c.RemotePeer(), addrs[:n], peerstore.RecentlyConnectedAddrTTL)
	ids.Host.Peerstore().UpdateAddrs(c.RemotePeer(), peerstore.TempAddrTTL, 0)
}

// Listen 处理监听事件
// 参数:
//   - n: network.Network 网络对象
//   - a: ma.Multiaddr 监听地址
func (nn *netNotifiee) Listen(n network.Network, a ma.Multiaddr) {}

// ListenClose 处理监听关闭事件
// 参数:
//   - n: network.Network 网络对象
//   - a: ma.Multiaddr 关闭的监听地址
func (nn *netNotifiee) ListenClose(n network.Network, a ma.Multiaddr) {}

// filterAddrs 基于远程多地址过滤地址切片:
//   - 如果是本地主机地址,不进行过滤
//   - 如果是私有网络地址,过滤掉所有本地主机地址
//   - 如果是公共地址,过滤掉所有非公共地址
//   - 如果都不是(例如丢弃前缀),不进行过滤
//     我们在这里无法做任何有意义的事情,所以什么都不做
//
// 参数:
//   - addrs: []ma.Multiaddr 要过滤的地址切片
//   - remote: ma.Multiaddr 远程多地址
//
// 返回值:
//   - []ma.Multiaddr 过滤后的地址切片
func filterAddrs(addrs []ma.Multiaddr, remote ma.Multiaddr) []ma.Multiaddr {
	switch {
	case manet.IsIPLoopback(remote):
		return addrs
	case manet.IsPrivateAddr(remote):
		return ma.FilterAddrs(addrs, func(a ma.Multiaddr) bool { return !manet.IsIPLoopback(a) })
	case manet.IsPublicAddr(remote):
		return ma.FilterAddrs(addrs, manet.IsPublicAddr)
	default:
		return addrs
	}
}

// trimHostAddrList 裁剪主机地址列表以适应最大大小
// 参数:
//   - addrs: []ma.Multiaddr 要裁剪的地址切片
//   - maxSize: int 最大大小(字节)
//
// 返回值:
//   - []ma.Multiaddr 裁剪后的地址切片
func trimHostAddrList(addrs []ma.Multiaddr, maxSize int) []ma.Multiaddr {
	totalSize := 0
	for _, a := range addrs {
		totalSize += len(a.Bytes())
	}
	if totalSize <= maxSize {
		return addrs
	}

	// 计算地址得分
	score := func(addr ma.Multiaddr) int {
		var res int
		if manet.IsPublicAddr(addr) {
			res |= 1 << 12
		} else if !manet.IsIPLoopback(addr) {
			res |= 1 << 11
		}
		var protocolWeight int
		ma.ForEach(addr, func(c ma.Component) bool {
			switch c.Protocol().Code {
			case ma.P_QUIC_V1:
				protocolWeight = 5
			case ma.P_TCP:
				protocolWeight = 4
			case ma.P_WSS:
				protocolWeight = 3
			case ma.P_WEBTRANSPORT:
				protocolWeight = 2
			case ma.P_WEBRTC_DIRECT:
				protocolWeight = 1
			case ma.P_P2P:
				return false
			}
			return true
		})
		res |= 1 << protocolWeight
		return res
	}

	// 按得分排序
	slices.SortStableFunc(addrs, func(a, b ma.Multiaddr) int {
		return score(b) - score(a) // b-a 用于反向排序
	})
	totalSize = 0
	for i, a := range addrs {
		totalSize += len(a.Bytes())
		if totalSize > maxSize {
			addrs = addrs[:i]
			break
		}
	}
	return addrs
}
