package relay

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dep2p/go-dep2p/core/crypto"
	"github.com/dep2p/go-dep2p/core/host"
	"github.com/dep2p/go-dep2p/core/network"
	"github.com/dep2p/go-dep2p/core/peer"
	"github.com/dep2p/go-dep2p/core/record"
	pbv2 "github.com/dep2p/go-dep2p/p2p/protocol/circuitv2/pb"
	"github.com/dep2p/go-dep2p/p2p/protocol/circuitv2/proto"
	"github.com/dep2p/go-dep2p/p2p/protocol/circuitv2/util"

	ma "github.com/dep2p/go-dep2p/multiformats/multiaddr"
	manet "github.com/dep2p/go-dep2p/multiformats/multiaddr/net"
	pool "github.com/dep2p/go-dep2p/p2plib/buffer/pool"
	logging "github.com/dep2p/log"
)

const (
	// 服务名称
	ServiceName = "dep2p.relay/v2"

	// 预约标签权重
	ReservationTagWeight = 10

	// 流超时时间
	StreamTimeout = time.Minute
	// 连接超时时间
	ConnectTimeout = 30 * time.Second
	// 握手超时时间
	HandshakeTimeout = time.Minute

	// 中继跳数标签
	relayHopTag = "relay-v2-hop"
	// 中继跳数标签值
	relayHopTagValue = 2

	// 最大消息大小
	maxMessageSize = 4096
)

// 日志记录器
var log = logging.Logger("p2p-protocol-circuitv2-relay")

// Relay 是一个有限制的中继服务对象
type Relay struct {
	// 上下文
	ctx context.Context
	// 取消函数
	cancel func()

	// 主机对象
	host host.Host
	// 资源配置
	rc Resources
	// 访问控制过滤器
	acl ACLFilter
	// 约束对象
	constraints *constraints
	// 资源作用域
	scope network.ResourceScopeSpan
	// 网络通知器
	notifiee network.Notifiee

	// 互斥锁
	mx sync.Mutex
	// 预约映射表
	rsvp map[peer.ID]time.Time
	// 连接计数映射表
	conns map[peer.ID]int
	// 是否已关闭
	closed bool

	// 自身地址
	selfAddr ma.Multiaddr

	// 指标追踪器
	metricsTracer MetricsTracer
}

// New 构造一个新的有限制的中继服务
// 参数:
//   - h: host.Host 主机对象
//   - opts: ...Option 配置选项
//
// 返回值:
//   - *Relay 中继服务对象
//   - error 错误信息
func New(h host.Host, opts ...Option) (*Relay, error) {
	// 创建上下文和取消函数
	ctx, cancel := context.WithCancel(context.Background())

	// 初始化中继对象
	r := &Relay{
		ctx:    ctx,
		cancel: cancel,
		host:   h,
		rc:     DefaultResources(),
		acl:    nil,
		rsvp:   make(map[peer.ID]time.Time),
		conns:  make(map[peer.ID]int),
	}

	// 应用配置选项
	for _, opt := range opts {
		err := opt(r)
		if err != nil {
			log.Debugf("应用中继选项时出错: %v", err)
			return nil, err
		}
	}

	// 获取服务级别的资源作用域
	err := h.Network().ResourceManager().ViewService(ServiceName,
		func(s network.ServiceScope) error {
			var err error
			r.scope, err = s.BeginSpan()
			return err
		})
	if err != nil {
		log.Debugf("获取服务级别的资源作用域失败: %v", err)
		return nil, err
	}

	// 初始化约束对象
	r.constraints = newConstraints(&r.rc)
	// 设置自身地址
	r.selfAddr = ma.StringCast(fmt.Sprintf("/p2p/%s", h.ID()))

	// 设置流处理器
	h.SetStreamHandler(proto.ProtoIDv2Hop, r.handleStream)
	// 设置网络通知器
	r.notifiee = &network.NotifyBundle{DisconnectedF: r.disconnected}
	h.Network().Notify(r.notifiee)

	// 更新指标状态
	if r.metricsTracer != nil {
		r.metricsTracer.RelayStatus(true)
	}
	// 启动后台任务
	go r.background()

	return r, nil
}

// Close 关闭中继服务
// 返回值:
//   - error 关闭过程中的错误,如果成功则返回nil
func (r *Relay) Close() error {
	// 加锁保护并发访问
	r.mx.Lock()
	if !r.closed {
		// 标记为已关闭
		r.closed = true
		r.mx.Unlock()

		// 移除流处理器
		r.host.RemoveStreamHandler(proto.ProtoIDv2Hop)
		// 停止网络通知
		r.host.Network().StopNotify(r.notifiee)
		// 释放资源作用域
		defer r.scope.Done()
		// 取消上下文
		r.cancel()
		// 执行垃圾回收
		r.gc()
		// 更新指标状态
		if r.metricsTracer != nil {
			r.metricsTracer.RelayStatus(false)
		}
		return nil
	}
	r.mx.Unlock()
	return nil
}

// handleStream 处理新的中继流
// 参数:
//   - s: network.Stream 网络流对象
func (r *Relay) handleStream(s network.Stream) {
	// log.Infof("收到来自 %s 的新中继流", s.Conn().RemotePeer())

	// 设置流的服务名称
	if err := s.Scope().SetService(ServiceName); err != nil {
		log.Debugf("为中继服务附加流时出错: %s", err)
		s.Reset()
		return
	}

	// 为流预留内存
	if err := s.Scope().ReserveMemory(maxMessageSize, network.ReservationPriorityAlways); err != nil {
		log.Debugf("为流预留内存时出错: %s", err)
		s.Reset()
		return
	}
	defer s.Scope().ReleaseMemory(maxMessageSize)

	// 创建带分隔符的读取器
	rd := util.NewDelimitedReader(s, maxMessageSize)
	defer rd.Close()

	// 设置读取超时
	s.SetReadDeadline(time.Now().Add(StreamTimeout))

	// 读取消息
	var msg pbv2.HopMessage
	err := rd.ReadMsg(&msg)
	if err != nil {
		r.handleError(s, pbv2.Status_MALFORMED_MESSAGE)
		return
	}
	// 重置流的读取期限
	s.SetReadDeadline(time.Time{})

	// 根据消息类型处理
	switch msg.GetType() {
	case pbv2.HopMessage_RESERVE:
		// 处理预约请求
		status := r.handleReserve(s)
		if r.metricsTracer != nil {
			r.metricsTracer.ReservationRequestHandled(status)
		}
	case pbv2.HopMessage_CONNECT:
		// 处理连接请求
		status := r.handleConnect(s, &msg)
		if r.metricsTracer != nil {
			r.metricsTracer.ConnectionRequestHandled(status)
		}
	default:
		// 处理未知消息类型
		r.handleError(s, pbv2.Status_MALFORMED_MESSAGE)
	}
}

// handleReserve 处理预约请求
// 参数:
//   - s: network.Stream 网络流对象
//
// 返回值:
//   - pbv2.Status 处理状态
func (r *Relay) handleReserve(s network.Stream) pbv2.Status {
	defer s.Close()
	// 获取远程对等点信息
	p := s.Conn().RemotePeer()
	a := s.Conn().RemoteMultiaddr()

	// 检查是否是中继地址
	if isRelayAddr(a) {
		log.Debugf("拒绝 %s 的中继预约;通过中继连接尝试预约", p)
		r.handleError(s, pbv2.Status_PERMISSION_DENIED)
		return pbv2.Status_PERMISSION_DENIED
	}

	// 检查访问控制
	if r.acl != nil && !r.acl.AllowReserve(p, a) {
		log.Debugf("拒绝 %s 的中继预约;权限被拒绝", p)
		r.handleError(s, pbv2.Status_PERMISSION_DENIED)
		return pbv2.Status_PERMISSION_DENIED
	}

	// 加锁保护并发访问
	r.mx.Lock()
	// 检查中继是否已关闭
	if r.closed {
		r.mx.Unlock()
		log.Debugf("拒绝 %s 的中继预约;中继已关闭", p)
		r.handleError(s, pbv2.Status_PERMISSION_DENIED)
		return pbv2.Status_PERMISSION_DENIED
	}

	// 计算预约过期时间
	now := time.Now()
	expire := now.Add(r.rc.ReservationTTL)

	// 检查是否已存在预约
	_, exists := r.rsvp[p]
	// 尝试预约资源
	if err := r.constraints.Reserve(p, a, expire); err != nil {
		r.mx.Unlock()
		log.Debugf("拒绝 %s 的中继预约;IP约束违规: %s", p, err)
		r.handleError(s, pbv2.Status_RESERVATION_REFUSED)
		return pbv2.Status_RESERVATION_REFUSED
	}

	// 记录预约信息
	r.rsvp[p] = expire
	r.host.ConnManager().TagPeer(p, "relay-reservation", ReservationTagWeight)
	r.mx.Unlock()

	// 更新指标
	if r.metricsTracer != nil {
		r.metricsTracer.ReservationAllowed(exists)
	}

	log.Debugf("为 %s 预留中继槽位", p)

	// 创建并发送预约响应消息
	// 预约传递可能因多种原因失败
	// 例如,在预约被接收前流可能被重置或连接可能被关闭
	// 在这种情况下,预约将在稍后被垃圾回收
	rsvp := makeReservationMsg(
		r.host.Peerstore().PrivKey(r.host.ID()),
		r.host.ID(),
		r.host.Addrs(),
		p,
		expire)
	if err := r.writeResponse(s, pbv2.Status_OK, rsvp, r.makeLimitMsg(p)); err != nil {
		log.Debugf("写入预约响应时出错;撤销 %s 的预约", p)
		s.Reset()
		return pbv2.Status_CONNECTION_FAILED
	}
	return pbv2.Status_OK
}

// handleConnect 处理中继连接请求
// 参数:
//   - s: network.Stream 网络流对象
//   - msg: *pbv2.HopMessage 中继消息对象
//
// 返回值:
//   - pbv2.Status 处理状态码
//
// 注意:
//   - 该方法实现了中继连接的建立过程
//   - 包括资源预约、权限验证、连接建立等步骤
func (r *Relay) handleConnect(s network.Stream, msg *pbv2.HopMessage) pbv2.Status {
	// 获取源连接的对端信息
	src := s.Conn().RemotePeer()
	a := s.Conn().RemoteMultiaddr()

	// 开始中继事务
	span, err := r.scope.BeginSpan()
	if err != nil {
		log.Debugf("开始中继事务失败: %s", err)
		r.handleError(s, pbv2.Status_RESOURCE_LIMIT_EXCEEDED)
		return pbv2.Status_RESOURCE_LIMIT_EXCEEDED
	}

	// 定义失败处理函数
	fail := func(status pbv2.Status) {
		span.Done()
		r.handleError(s, status)
	}

	// 为中继预留缓冲区
	if err := span.ReserveMemory(2*r.rc.BufferSize, network.ReservationPriorityHigh); err != nil {
		log.Debugf("为中继预留内存时出错: %s", err)
		fail(pbv2.Status_RESOURCE_LIMIT_EXCEEDED)
		return pbv2.Status_RESOURCE_LIMIT_EXCEEDED
	}

	// 检查是否是中继地址
	if isRelayAddr(a) {
		log.Debugf("拒绝来自 %s 的连接;连接尝试通过中继连接")
		fail(pbv2.Status_PERMISSION_DENIED)
		return pbv2.Status_PERMISSION_DENIED
	}

	// 解析目标节点信息
	dest, err := util.PeerToPeerInfoV2(msg.GetPeer())
	if err != nil {
		fail(pbv2.Status_MALFORMED_MESSAGE)
		return pbv2.Status_MALFORMED_MESSAGE
	}

	// 检查访问控制权限
	if r.acl != nil && !r.acl.AllowConnect(src, s.Conn().RemoteMultiaddr(), dest.ID) {
		log.Debugf("拒绝从 %s 到 %s 的连接;权限被拒绝", src, dest.ID)
		fail(pbv2.Status_PERMISSION_DENIED)
		return pbv2.Status_PERMISSION_DENIED
	}

	// 检查预约状态
	r.mx.Lock()
	_, rsvp := r.rsvp[dest.ID]
	if !rsvp {
		r.mx.Unlock()
		log.Debugf("拒绝从 %s 到 %s 的连接;无预约", src, dest.ID)
		fail(pbv2.Status_NO_RESERVATION)
		return pbv2.Status_NO_RESERVATION
	}

	// 检查源节点连接数限制
	srcConns := r.conns[src]
	if srcConns >= r.rc.MaxCircuits {
		r.mx.Unlock()
		log.Debugf("拒绝从 %s 到 %s 的连接;来自 %s 的连接过多", src, dest.ID, src)
		fail(pbv2.Status_RESOURCE_LIMIT_EXCEEDED)
		return pbv2.Status_RESOURCE_LIMIT_EXCEEDED
	}

	// 检查目标节点连接数限制
	destConns := r.conns[dest.ID]
	if destConns >= r.rc.MaxCircuits {
		r.mx.Unlock()
		log.Debugf("拒绝从 %s 到 %s 的连接;到 %s 的连接过多", src, dest.ID, dest.ID)
		fail(pbv2.Status_RESOURCE_LIMIT_EXCEEDED)
		return pbv2.Status_RESOURCE_LIMIT_EXCEEDED
	}

	// 添加连接计数
	r.addConn(src)
	r.addConn(dest.ID)
	r.mx.Unlock()

	// 更新指标
	if r.metricsTracer != nil {
		r.metricsTracer.ConnectionOpened()
	}
	connStTime := time.Now()

	// 定义清理函数
	cleanup := func() {
		defer span.Done()
		r.mx.Lock()
		r.rmConn(src)
		r.rmConn(dest.ID)
		r.mx.Unlock()
		if r.metricsTracer != nil {
			r.metricsTracer.ConnectionClosed(time.Since(connStTime))
		}
	}

	// 创建连接上下文
	ctx, cancel := context.WithTimeout(r.ctx, ConnectTimeout)
	defer cancel()

	ctx = network.WithNoDial(ctx, "relay connect")

	// 创建到目标节点的流
	bs, err := r.host.NewStream(ctx, dest.ID, proto.ProtoIDv2Stop)
	if err != nil {
		log.Debugf("打开到 %s 的中继流时出错: %s", dest.ID, err)
		cleanup()
		r.handleError(s, pbv2.Status_CONNECTION_FAILED)
		return pbv2.Status_CONNECTION_FAILED
	}

	// 更新失败处理函数
	fail = func(status pbv2.Status) {
		bs.Reset()
		cleanup()
		r.handleError(s, status)
	}

	// 设置流服务
	if err := bs.Scope().SetService(ServiceName); err != nil {
		log.Debugf("将流附加到中继服务时出错: %s", err)
		fail(pbv2.Status_RESOURCE_LIMIT_EXCEEDED)
		return pbv2.Status_RESOURCE_LIMIT_EXCEEDED
	}

	// 为流预留内存
	if err := bs.Scope().ReserveMemory(maxMessageSize, network.ReservationPriorityAlways); err != nil {
		log.Debugf("为流预留内存时出错: %s", err)
		fail(pbv2.Status_RESOURCE_LIMIT_EXCEEDED)
		return pbv2.Status_RESOURCE_LIMIT_EXCEEDED
	}
	defer bs.Scope().ReleaseMemory(maxMessageSize)

	// 创建读写器
	rd := util.NewDelimitedReader(bs, maxMessageSize)
	wr := util.NewDelimitedWriter(bs)
	defer rd.Close()

	// 创建停止消息
	var stopmsg pbv2.StopMessage
	stopmsg.Type = pbv2.StopMessage_CONNECT.Enum()
	stopmsg.Peer = util.PeerInfoToPeerV2(peer.AddrInfo{ID: src})
	stopmsg.Limit = r.makeLimitMsg(dest.ID)

	// 设置握手超时
	bs.SetDeadline(time.Now().Add(HandshakeTimeout))

	// 发送停止消息
	err = wr.WriteMsg(&stopmsg)
	if err != nil {
		log.Debugf("写入停止握手时出错")
		fail(pbv2.Status_CONNECTION_FAILED)
		return pbv2.Status_CONNECTION_FAILED
	}

	stopmsg.Reset()

	// 读取响应
	err = rd.ReadMsg(&stopmsg)
	if err != nil {
		log.Debugf("读取停止响应时出错: %s", err.Error())
		fail(pbv2.Status_CONNECTION_FAILED)
		return pbv2.Status_CONNECTION_FAILED
	}

	// 验证响应类型
	if t := stopmsg.GetType(); t != pbv2.StopMessage_STATUS {
		log.Debugf("意外的停止响应;不是状态消息 (%d)", t)
		fail(pbv2.Status_CONNECTION_FAILED)
		return pbv2.Status_CONNECTION_FAILED
	}

	// 检查响应状态
	if status := stopmsg.GetStatus(); status != pbv2.Status_OK {
		log.Debugf("中继停止失败: %d", status)
		fail(pbv2.Status_CONNECTION_FAILED)
		return pbv2.Status_CONNECTION_FAILED
	}

	// 创建并发送响应消息
	var response pbv2.HopMessage
	response.Type = pbv2.HopMessage_STATUS.Enum()
	response.Status = pbv2.Status_OK.Enum()
	response.Limit = r.makeLimitMsg(dest.ID)

	wr = util.NewDelimitedWriter(s)
	err = wr.WriteMsg(&response)
	if err != nil {
		log.Debugf("写入中继响应时出错: %s", err)
		bs.Reset()
		s.Reset()
		cleanup()
		return pbv2.Status_CONNECTION_FAILED
	}

	// 重置超时
	bs.SetDeadline(time.Time{})

	// log.Infof("正在从 %s 中继连接到 %s", src, dest.ID)

	// 创建并发计数器
	var goroutines atomic.Int32
	goroutines.Store(2)

	// 定义完成处理函数
	done := func() {
		if goroutines.Add(-1) == 0 {
			s.Close()
			bs.Close()
			cleanup()
		}
	}

	// 启动中继传输
	if r.rc.Limit != nil {
		deadline := time.Now().Add(r.rc.Limit.Duration)
		s.SetDeadline(deadline)
		bs.SetDeadline(deadline)
		go r.relayLimited(s, bs, src, dest.ID, r.rc.Limit.Data, done)
		go r.relayLimited(bs, s, dest.ID, src, r.rc.Limit.Data, done)
	} else {
		go r.relayUnlimited(s, bs, src, dest.ID, done)
		go r.relayUnlimited(bs, s, dest.ID, src, done)
	}

	return pbv2.Status_OK
}

// addConn 增加节点的连接计数
// 参数:
//   - p: peer.ID 节点ID
func (r *Relay) addConn(p peer.ID) {
	conns := r.conns[p]
	conns++
	r.conns[p] = conns
	if conns == 1 {
		r.host.ConnManager().TagPeer(p, relayHopTag, relayHopTagValue)
	}
}

// rmConn 减少节点的连接计数
// 参数:
//   - p: peer.ID 节点ID
func (r *Relay) rmConn(p peer.ID) {
	conns := r.conns[p]
	conns--
	if conns > 0 {
		r.conns[p] = conns
	} else {
		delete(r.conns, p)
		r.host.ConnManager().UntagPeer(p, relayHopTag)
	}
}

// relayLimited 在有限制的情况下执行中继传输
// 参数:
//   - src: network.Stream 源流
//   - dest: network.Stream 目标流
//   - srcID: peer.ID 源节点ID
//   - destID: peer.ID 目标节点ID
//   - limit: int64 传输限制
//   - done: func() 完成回调函数
func (r *Relay) relayLimited(src, dest network.Stream, srcID, destID peer.ID, limit int64, done func()) {
	defer done()

	buf := pool.Get(r.rc.BufferSize)
	defer pool.Put(buf)

	limitedSrc := io.LimitReader(src, limit)

	count, err := r.copyWithBuffer(dest, limitedSrc, buf)
	if err != nil {
		log.Debugf("中继复制出错: %s", err)
		// 重置两端
		src.Reset()
		dest.Reset()
	} else {
		// 传播关闭
		dest.CloseWrite()
		if count == limit {
			// 已达到限制,丢弃后续输入
			src.CloseRead()
		}
	}

	log.Debugf("从 %s 到 %s 中继了 %d 字节", srcID, destID, count)
}

// relayUnlimited 在无限制的情况下执行中继传输
// 参数:
//   - src: network.Stream 源流
//   - dest: network.Stream 目标流
//   - srcID: peer.ID 源节点ID
//   - destID: peer.ID 目标节点ID
//   - done: func() 完成回调函数
func (r *Relay) relayUnlimited(src, dest network.Stream, srcID, destID peer.ID, done func()) {
	// 延迟执行完成回调
	defer done()

	// 从缓冲池获取缓冲区
	buf := pool.Get(r.rc.BufferSize)
	// 使用完后归还缓冲区
	defer pool.Put(buf)

	// 执行数据复制
	count, err := r.copyWithBuffer(dest, src, buf)
	if err != nil {
		log.Debugf("中继复制出错: %s", err)
		// 重置两端流
		src.Reset()
		dest.Reset()
	} else {
		// 传播关闭
		dest.CloseWrite()
	}

	log.Debugf("从 %s 到 %s 中继了 %d 字节", srcID, destID, count)
}

// errInvalidWrite 表示写入返回了一个不可能的计数
// 从 io.errInvalidWrite 复制而来
var errInvalidWrite = errors.New("无效的写入结果")

// copyWithBuffer 使用提供的缓冲区从源复制到目标,直到源达到EOF或发生错误
// 参数:
//   - dst: io.Writer 目标写入器
//   - src: io.Reader 源读取器
//   - buf: []byte 缓冲区
//
// 返回值:
//   - written: int64 已写入的字节数
//   - err: error 错误信息
//
// 注意:
//   - 这是 io.CopyBuffer 的修改版本,支持指标追踪
func (r *Relay) copyWithBuffer(dst io.Writer, src io.Reader, buf []byte) (written int64, err error) {
	for {
		// 从源读取数据
		nr, er := src.Read(buf)
		if nr > 0 {
			// 写入目标
			nw, ew := dst.Write(buf[0:nr])
			if nw < 0 || nr < nw {
				nw = 0
				if ew == nil {
					ew = errInvalidWrite
				}
			}
			written += int64(nw)
			if ew != nil {
				err = ew
				break
			}
			if nr != nw {
				err = io.ErrShortWrite
				break
			}
			// 更新传输字节指标
			if r.metricsTracer != nil {
				r.metricsTracer.BytesTransferred(nw)
			}
		}
		if er != nil {
			if er != io.EOF {
				err = er
			}
			break
		}
	}
	return written, err
}

// handleError 处理中继错误
// 参数:
//   - s: network.Stream 网络流对象
//   - status: pbv2.Status 状态码
func (r *Relay) handleError(s network.Stream, status pbv2.Status) {
	log.Debugf("中继错误: %s (%d)", pbv2.Status_name[int32(status)], status)
	err := r.writeResponse(s, status, nil, nil)
	if err != nil {
		s.Reset()
		log.Debugf("写入中继响应时出错: %s", err.Error())
	} else {
		s.Close()
	}
}

// writeResponse 写入中继响应
// 参数:
//   - s: network.Stream 网络流对象
//   - status: pbv2.Status 状态码
//   - rsvp: *pbv2.Reservation 预约信息
//   - limit: *pbv2.Limit 限制信息
//
// 返回值:
//   - error 错误信息
func (r *Relay) writeResponse(s network.Stream, status pbv2.Status, rsvp *pbv2.Reservation, limit *pbv2.Limit) error {
	// 设置写入超时
	s.SetWriteDeadline(time.Now().Add(StreamTimeout))
	defer s.SetWriteDeadline(time.Time{})
	wr := util.NewDelimitedWriter(s)

	// 构造响应消息
	var msg pbv2.HopMessage
	msg.Type = pbv2.HopMessage_STATUS.Enum()
	msg.Status = status.Enum()
	msg.Reservation = rsvp
	msg.Limit = limit

	return wr.WriteMsg(&msg)
}

// makeReservationMsg 创建预约消息
// 参数:
//   - signingKey: crypto.PrivKey 签名密钥
//   - selfID: peer.ID 自身节点ID
//   - selfAddrs: []ma.Multiaddr 自身地址列表
//   - p: peer.ID 目标节点ID
//   - expire: time.Time 过期时间
//
// 返回值:
//   - *pbv2.Reservation 预约消息对象
func makeReservationMsg(
	signingKey crypto.PrivKey,
	selfID peer.ID,
	selfAddrs []ma.Multiaddr,
	p peer.ID,
	expire time.Time,
) *pbv2.Reservation {
	// 转换过期时间为Unix时间戳
	expireUnix := uint64(expire.Unix())

	rsvp := &pbv2.Reservation{Expire: &expireUnix}

	// 创建自身P2P地址组件
	selfP2PAddr, err := ma.NewComponent("p2p", selfID.String())
	if err != nil {
		log.Debugf("创建p2p组件时出错: %s", err)
		return rsvp
	}

	// 处理地址列表
	var addrBytes [][]byte
	for _, addr := range selfAddrs {
		if !manet.IsPublicAddr(addr) {
			continue
		}

		id, _ := peer.IDFromP2PAddr(addr)
		switch {
		case id == "":
			// 无ID,添加一个到地址中
			addr = addr.Encapsulate(selfP2PAddr)
		case id == selfID:
			// 地址已包含我们的ID
			// 不做任何处理
		case id != selfID:
			// 地址包含了不同的ID,跳过
			log.Warnf("跳过地址 %s: 包含了意外的ID", addr)
			continue
		}
		addrBytes = append(addrBytes, addr.Bytes())
	}

	rsvp.Addrs = addrBytes

	// 创建预约凭证
	voucher := &proto.ReservationVoucher{
		Relay:      selfID,
		Peer:       p,
		Expiration: expire,
	}

	// 签名凭证
	envelope, err := record.Seal(voucher, signingKey)
	if err != nil {
		log.Errorf("为 %s 签名凭证时出错: %s", p, err)
		return rsvp
	}

	// 序列化凭证
	blob, err := envelope.Marshal()
	if err != nil {
		log.Errorf("为 %s 序列化凭证时出错: %s", p, err)
		return rsvp
	}

	rsvp.Voucher = blob

	return rsvp
}

// makeLimitMsg 创建限制消息
// 参数:
//   - p: peer.ID 节点ID
//
// 返回值:
//   - *pbv2.Limit 限制消息对象
func (r *Relay) makeLimitMsg(p peer.ID) *pbv2.Limit {
	if r.rc.Limit == nil {
		return nil
	}

	duration := uint32(r.rc.Limit.Duration / time.Second)
	data := uint64(r.rc.Limit.Data)

	return &pbv2.Limit{
		Duration: &duration,
		Data:     &data,
	}
}

// background 执行后台任务
func (r *Relay) background() {
	// 创建定时器
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			r.gc()
		case <-r.ctx.Done():
			return
		}
	}
}

// gc 执行垃圾回收
func (r *Relay) gc() {
	r.mx.Lock()
	defer r.mx.Unlock()

	// 清理过期预约
	now := time.Now()
	cnt := 0
	for p, expire := range r.rsvp {
		if r.closed || expire.Before(now) {
			delete(r.rsvp, p)
			r.host.ConnManager().UntagPeer(p, "relay-reservation")
			cnt++
		}
	}
	if r.metricsTracer != nil {
		r.metricsTracer.ReservationClosed(cnt)
	}

	// 清理空连接
	for p, count := range r.conns {
		if count == 0 {
			delete(r.conns, p)
		}
	}
}

// disconnected 处理节点断开连接事件
// 参数:
//   - n: network.Network 网络对象
//   - c: network.Conn 连接对象
func (r *Relay) disconnected(n network.Network, c network.Conn) {
	p := c.RemotePeer()
	if n.Connectedness(p) == network.Connected {
		return
	}

	r.mx.Lock()
	_, ok := r.rsvp[p]
	if ok {
		delete(r.rsvp, p)
	}
	r.constraints.cleanupPeer(p)
	r.mx.Unlock()

	if ok && r.metricsTracer != nil {
		r.metricsTracer.ReservationClosed(1)
	}
}

// isRelayAddr 检查是否是中继地址
// 参数:
//   - a: ma.Multiaddr 多地址对象
//
// 返回值:
//   - bool 是否是中继地址
func isRelayAddr(a ma.Multiaddr) bool {
	_, err := a.ValueForProtocol(ma.P_CIRCUIT)
	return err == nil
}
