package blankhost

import (
	"context"
	"errors"
	"io"

	"github.com/dep2p/go-dep2p/core/connmgr"
	"github.com/dep2p/go-dep2p/core/event"
	"github.com/dep2p/go-dep2p/core/host"
	"github.com/dep2p/go-dep2p/core/network"
	"github.com/dep2p/go-dep2p/core/peer"
	"github.com/dep2p/go-dep2p/core/peerstore"
	"github.com/dep2p/go-dep2p/core/protocol"
	"github.com/dep2p/go-dep2p/core/record"
	"github.com/dep2p/go-dep2p/p2p/host/eventbus"

	logging "github.com/dep2p/log"

	ma "github.com/dep2p/go-dep2p/multiformats/multiaddr"
	mstream "github.com/dep2p/go-dep2p/multiformats/multistream"
)

// 定义日志记录器
var log = logging.Logger("host-blank")

// BlankHost 是 host.Host 接口的最简实现
type BlankHost struct {
	n        network.Network                        // 网络接口
	mux      *mstream.MultistreamMuxer[protocol.ID] // 多流复用器
	cmgr     connmgr.ConnManager                    // 连接管理器
	eventbus event.Bus                              // 事件总线
	emitters struct {
		evtLocalProtocolsUpdated event.Emitter // 本地协议更新事件发射器
	}
}

// config 定义配置选项
type config struct {
	cmgr     connmgr.ConnManager // 连接管理器
	eventBus event.Bus           // 事件总线
}

// Option 定义配置选项函数类型
type Option = func(cfg *config)

// WithConnectionManager 设置连接管理器选项
// 参数:
//   - cmgr: connmgr.ConnManager 连接管理器实例
//
// 返回:
//   - Option 配置选项函数
func WithConnectionManager(cmgr connmgr.ConnManager) Option {
	return func(cfg *config) {
		cfg.cmgr = cmgr
	}
}

// WithEventBus 设置事件总线选项
// 参数:
//   - eventBus: event.Bus 事件总线实例
//
// 返回:
//   - Option 配置选项函数
func WithEventBus(eventBus event.Bus) Option {
	return func(cfg *config) {
		cfg.eventBus = eventBus
	}
}

// NewBlankHost 创建新的空白主机实例
// 参数:
//   - n: network.Network 网络实例
//   - options: ...Option 配置选项
//
// 返回:
//   - *BlankHost 空白主机实例
func NewBlankHost(n network.Network, options ...Option) *BlankHost {
	// 初始化默认配置
	cfg := config{
		cmgr: &connmgr.NullConnMgr{},
	}
	// 应用配置选项
	for _, opt := range options {
		opt(&cfg)
	}

	// 创建空白主机实例
	bh := &BlankHost{
		n:        n,
		cmgr:     cfg.cmgr,
		mux:      mstream.NewMultistreamMuxer[protocol.ID](),
		eventbus: cfg.eventBus,
	}
	// 如果未设置事件总线,则创建默认事件总线
	if bh.eventbus == nil {
		bh.eventbus = eventbus.NewBus(eventbus.WithMetricsTracer(eventbus.NewMetricsTracer()))
	}

	// 订阅网络通知
	n.Notify(bh.cmgr.Notifee())

	var err error
	// 创建本地协议更新事件发射器
	if bh.emitters.evtLocalProtocolsUpdated, err = bh.eventbus.Emitter(&event.EvtLocalProtocolsUpdated{}); err != nil {
		return nil
	}

	// 设置流处理器
	n.SetStreamHandler(bh.newStreamHandler)

	// 在对等存储中持久化签名的对等记录
	if err := bh.initSignedRecord(); err != nil {
		log.Errorf("创建空白主机错误: %s", err)
		return nil
	}

	return bh
}

// initSignedRecord 初始化并持久化签名的对等记录
// 返回:
//   - error 错误信息
func (bh *BlankHost) initSignedRecord() error {
	// 获取认证地址簿
	cab, ok := peerstore.GetCertifiedAddrBook(bh.n.Peerstore())
	if !ok {
		log.Errorf("对等存储不支持签名记录")
		return errors.New("对等存储不支持签名记录")
	}
	// 创建对等记录
	rec := peer.PeerRecordFromAddrInfo(peer.AddrInfo{ID: bh.ID(), Addrs: bh.Addrs()})
	// 签名记录
	ev, err := record.Seal(rec, bh.Peerstore().PrivKey(bh.ID()))
	if err != nil {
		log.Errorf("为自身创建签名记录失败: %s", err)
		return err
	}
	// 保存记录到对等存储
	_, err = cab.ConsumePeerRecord(ev, peerstore.PermanentAddrTTL)
	if err != nil {
		log.Errorf("将签名记录持久化到对等存储失败: %s", err)
		return err
	}
	return nil
}

// 确保 BlankHost 实现了 host.Host 接口
var _ host.Host = (*BlankHost)(nil)

// Addrs 返回主机的监听地址列表
// 返回:
//   - []ma.Multiaddr 监听地址列表
func (bh *BlankHost) Addrs() []ma.Multiaddr {
	// 获取网络接口监听地址
	addrs, err := bh.n.InterfaceListenAddresses()
	if err != nil {
		log.Debugf("获取网络接口地址错误: %v", err)
		return nil
	}

	return addrs
}

// Close 关闭主机
// 返回:
//   - error 错误信息
func (bh *BlankHost) Close() error {
	return bh.n.Close()
}

// Connect 连接到指定的对等节点
// 参数:
//   - ctx: context.Context 上下文
//   - ai: peer.AddrInfo 对等节点信息
//
// 返回:
//   - error 错误信息
func (bh *BlankHost) Connect(ctx context.Context, ai peer.AddrInfo) error {
	// 将地址添加到对等存储
	bh.Peerstore().AddAddrs(ai.ID, ai.Addrs, peerstore.TempAddrTTL)

	// 检查是否已连接
	cs := bh.n.ConnsToPeer(ai.ID)
	if len(cs) > 0 {
		return nil
	}

	// 拨号连接对等节点
	_, err := bh.Network().DialPeer(ctx, ai.ID)
	if err != nil {
		log.Errorf("拨号失败: %v", err)
		return err
	}
	return nil
}

// Peerstore 返回对等存储
// 返回:
//   - peerstore.Peerstore 对等存储实例
func (bh *BlankHost) Peerstore() peerstore.Peerstore {
	return bh.n.Peerstore()
}

// ID 返回本地对等节点ID
// 返回:
//   - peer.ID 对等节点ID
func (bh *BlankHost) ID() peer.ID {
	return bh.n.LocalPeer()
}

// NewStream 创建新的流
// 参数:
//   - ctx: context.Context 上下文
//   - p: peer.ID 对等节点ID
//   - protos: ...protocol.ID 协议ID列表
//
// 返回:
//   - network.Stream 网络流
//   - error 错误信息
func (bh *BlankHost) NewStream(ctx context.Context, p peer.ID, protos ...protocol.ID) (network.Stream, error) {
	// 创建新流
	s, err := bh.n.NewStream(ctx, p)
	if err != nil {
		log.Debugf("打开流失败: %v", err)
		return nil, err
	}

	// 协商协议
	selected, err := mstream.SelectOneOf(protos, s)
	if err != nil {
		s.Reset()
		log.Debugf("协议协商失败: %v", err)
		return nil, err
	}

	// 设置协议
	s.SetProtocol(selected)
	bh.Peerstore().AddProtocols(p, selected)

	return s, nil
}

// RemoveStreamHandler 移除流处理器
// 参数:
//   - pid: protocol.ID 协议ID
func (bh *BlankHost) RemoveStreamHandler(pid protocol.ID) {
	bh.Mux().RemoveHandler(pid)
	bh.emitters.evtLocalProtocolsUpdated.Emit(event.EvtLocalProtocolsUpdated{
		Removed: []protocol.ID{pid},
	})
}

// SetStreamHandler 设置流处理器
// 参数:
//   - pid: protocol.ID 协议ID
//   - handler: network.StreamHandler 流处理器
func (bh *BlankHost) SetStreamHandler(pid protocol.ID, handler network.StreamHandler) {
	bh.Mux().AddHandler(pid, func(p protocol.ID, rwc io.ReadWriteCloser) error {
		is := rwc.(network.Stream)
		is.SetProtocol(p)
		handler(is)
		return nil
	})
	bh.emitters.evtLocalProtocolsUpdated.Emit(event.EvtLocalProtocolsUpdated{
		Added: []protocol.ID{pid},
	})
}

// SetStreamHandlerMatch 设置带匹配函数的流处理器
// 参数:
//   - pid: protocol.ID 协议ID
//   - m: func(protocol.ID) bool 匹配函数
//   - handler: network.StreamHandler 流处理器
func (bh *BlankHost) SetStreamHandlerMatch(pid protocol.ID, m func(protocol.ID) bool, handler network.StreamHandler) {
	bh.Mux().AddHandlerWithFunc(pid, m, func(p protocol.ID, rwc io.ReadWriteCloser) error {
		is := rwc.(network.Stream)
		is.SetProtocol(p)
		handler(is)
		return nil
	})
	bh.emitters.evtLocalProtocolsUpdated.Emit(event.EvtLocalProtocolsUpdated{
		Added: []protocol.ID{pid},
	})
}

// newStreamHandler 是网络的远程打开流处理器
// 参数:
//   - s: network.Stream 网络流
func (bh *BlankHost) newStreamHandler(s network.Stream) {
	// 协商协议
	protoID, handle, err := bh.Mux().Negotiate(s)
	if err != nil {
		log.Infow("协议协商失败", "error", err)
		s.Reset()
		return
	}

	// 设置协议
	s.SetProtocol(protoID)

	// 处理流
	handle(protoID, s)
}

// Mux 返回协议切换器
// 返回:
//   - protocol.Switch 协议切换器
func (bh *BlankHost) Mux() protocol.Switch {
	return bh.mux
}

// Network 返回网络实例
// 返回:
//   - network.Network 网络实例
func (bh *BlankHost) Network() network.Network {
	return bh.n
}

// ConnManager 返回连接管理器
// 返回:
//   - connmgr.ConnManager 连接管理器
func (bh *BlankHost) ConnManager() connmgr.ConnManager {
	return bh.cmgr
}

// EventBus 返回事件总线
// 返回:
//   - event.Bus 事件总线
func (bh *BlankHost) EventBus() event.Bus {
	return bh.eventbus
}
