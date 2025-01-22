package autonatv2

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime/debug"
	"sync"
	"time"

	"github.com/dep2p/go-dep2p/core/host"
	"github.com/dep2p/go-dep2p/core/network"
	"github.com/dep2p/go-dep2p/core/peer"
	"github.com/dep2p/go-dep2p/core/peerstore"
	"github.com/dep2p/go-dep2p/p2p/protocol/autonatv2/pb"
	pool "github.com/dep2p/go-dep2p/p2plib/buffer/pool"
	"github.com/dep2p/go-dep2p/p2plib/msgio/pbio"

	"math/rand"

	ma "github.com/dep2p/go-dep2p/multiformats/multiaddr"
	manet "github.com/dep2p/go-dep2p/multiformats/multiaddr/net"
)

var (
	// 超出资源限制错误
	errResourceLimitExceeded = errors.New("超出资源限制")
	// 错误的请求
	errBadRequest = errors.New("错误的请求")
	// 拒绝拨号数据
	errDialDataRefused = errors.New("拒绝拨号数据")
)

// dataRequestPolicyFunc 定义了一个函数类型,用于判断是否需要拨号数据
type dataRequestPolicyFunc = func(s network.Stream, dialAddr ma.Multiaddr) bool

// EventDialRequestCompleted 表示拨号请求完成的事件
// 包含了请求的结果信息
type EventDialRequestCompleted struct {
	Error            error                          // 错误信息
	ResponseStatus   pb.DialResponse_ResponseStatus // 响应状态
	DialStatus       pb.DialStatus                  // 拨号状态
	DialDataRequired bool                           // 是否需要拨号数据
	DialedAddr       ma.Multiaddr                   // 拨号地址
}

// server 实现了 AutoNATv2 服务端
// 它可以在尝试请求的拨号之前要求客户端提供拨号数据
// 它在全局级别、每个对等点级别以及请求是否需要拨号数据方面限制请求
type server struct {
	host       host.Host    // 主机实例
	dialerHost host.Host    // 拨号器主机实例
	limiter    *rateLimiter // 速率限制器

	// dialDataRequestPolicy 用于确定拨号地址是否需要接收拨号数据
	// 默认设置为防止放大攻击
	dialDataRequestPolicy                dataRequestPolicyFunc
	amplificatonAttackPreventionDialWait time.Duration // 防止放大攻击的等待时间
	metricsTracer                        MetricsTracer // 指标追踪器

	// 用于测试
	now               func() time.Time // 获取当前时间的函数
	allowPrivateAddrs bool             // 是否允许私有地址
}

// newServer 创建一个新的服务器实例
// 参数:
//   - host: 主机实例
//   - dialer: 拨号器主机实例
//   - s: AutoNAT 设置
//
// 返回值:
//   - *server: 新创建的服务器实例
func newServer(host, dialer host.Host, s *autoNATSettings) *server {
	return &server{
		dialerHost:                           dialer,
		host:                                 host,
		dialDataRequestPolicy:                s.dataRequestPolicy,
		amplificatonAttackPreventionDialWait: s.amplificatonAttackPreventionDialWait,
		allowPrivateAddrs:                    s.allowPrivateAddrs,
		limiter: &rateLimiter{
			RPM:         s.serverRPM,
			PerPeerRPM:  s.serverPerPeerRPM,
			DialDataRPM: s.serverDialDataRPM,
			now:         s.now,
		},
		now:           s.now,
		metricsTracer: s.metricsTracer,
	}
}

// Start 将流处理器附加到主机
func (as *server) Start() {
	as.host.SetStreamHandler(DialProtocol, as.handleDialRequest)
}

// Close 关闭服务器
func (as *server) Close() {
	as.host.RemoveStreamHandler(DialProtocol)
	as.dialerHost.Close()
	as.limiter.Close()
}

// handleDialRequest 是拨号请求协议的流处理器
// 参数:
//   - s: network.Stream 网络流对象
func (as *server) handleDialRequest(s network.Stream) {
	defer func() {
		if rerr := recover(); rerr != nil {
			fmt.Fprintf(os.Stderr, "捕获到 panic: %s\n%s\n", rerr, debug.Stack())
			s.Reset()
		}
	}()

	log.Debugf("收到来自 %s 的拨号请求, 地址: %s", s.Conn().RemotePeer(), s.Conn().RemoteMultiaddr())
	evt := as.serveDialRequest(s)
	log.Debugf("完成来自 %s 的拨号请求, 响应状态: %s, 拨号状态: %s, 错误: %s",
		s.Conn().RemotePeer(), evt.ResponseStatus, evt.DialStatus, evt.Error)
	if as.metricsTracer != nil {
		as.metricsTracer.CompletedRequest(evt)
	}
}

// serveDialRequest 处理拨号请求
// 参数:
//   - s: network.Stream 网络流对象
//
// 返回值:
//   - EventDialRequestCompleted 拨号请求完成事件
func (as *server) serveDialRequest(s network.Stream) EventDialRequestCompleted {
	if err := s.Scope().SetService(ServiceName); err != nil {
		s.Reset()
		log.Debugf("无法将流附加到 %s 服务: %w", ServiceName, err)
		return EventDialRequestCompleted{
			Error: errors.New("无法将流附加到 autonat-v2"),
		}
	}

	if err := s.Scope().ReserveMemory(maxMsgSize, network.ReservationPriorityAlways); err != nil {
		s.Reset()
		log.Debugf("为流 %s 预留内存失败: %w", DialProtocol, err)
		return EventDialRequestCompleted{Error: errResourceLimitExceeded}
	}
	defer s.Scope().ReleaseMemory(maxMsgSize)

	deadline := as.now().Add(streamTimeout)
	ctx, cancel := context.WithDeadline(context.Background(), deadline)
	defer cancel()
	s.SetDeadline(as.now().Add(streamTimeout))
	defer s.Close()

	p := s.Conn().RemotePeer()

	var msg pb.Message
	w := pbio.NewDelimitedWriter(s)
	// 在解析请求之前检查速率限制
	if !as.limiter.Accept(p) {
		msg = pb.Message{
			Msg: &pb.Message_DialResponse{
				DialResponse: &pb.DialResponse{
					Status: pb.DialResponse_E_REQUEST_REJECTED,
				},
			},
		}
		if err := w.WriteMsg(&msg); err != nil {
			s.Reset()
			log.Debugf("向 %s 写入请求拒绝响应失败: %s", p, err)
			return EventDialRequestCompleted{
				ResponseStatus: pb.DialResponse_E_REQUEST_REJECTED,
				Error:          fmt.Errorf("写入失败: %w", err),
			}
		}
		log.Debugf("拒绝来自 %s 的请求: 超出速率限制", p)
		return EventDialRequestCompleted{ResponseStatus: pb.DialResponse_E_REQUEST_REJECTED}
	}
	defer as.limiter.CompleteRequest(p)

	r := pbio.NewDelimitedReader(s, maxMsgSize)
	if err := r.ReadMsg(&msg); err != nil {
		s.Reset()
		log.Debugf("读取来自 %s 的请求失败: %s", p, err)
		return EventDialRequestCompleted{Error: fmt.Errorf("读取失败: %w", err)}
	}
	if msg.GetDialRequest() == nil {
		s.Reset()
		log.Debugf("来自 %s 的无效消息类型: %T 期望: DialRequest", p, msg.Msg)
		return EventDialRequestCompleted{Error: errBadRequest}
	}

	// 解析对等点的地址
	var dialAddr ma.Multiaddr
	var addrIdx int
	for i, ab := range msg.GetDialRequest().GetAddrs() {
		if i >= maxPeerAddresses {
			break
		}
		a, err := ma.NewMultiaddrBytes(ab)
		if err != nil {
			continue
		}
		if !as.allowPrivateAddrs && !manet.IsPublicAddr(a) {
			continue
		}
		if !as.dialerHost.Network().CanDial(p, a) {
			continue
		}
		dialAddr = a
		addrIdx = i
		break
	}
	// 没有可拨号的地址
	if dialAddr == nil {
		msg = pb.Message{
			Msg: &pb.Message_DialResponse{
				DialResponse: &pb.DialResponse{
					Status: pb.DialResponse_E_DIAL_REFUSED,
				},
			},
		}
		if err := w.WriteMsg(&msg); err != nil {
			s.Reset()
			log.Debugf("向 %s 写入拨号拒绝响应失败: %s", p, err)
			return EventDialRequestCompleted{
				ResponseStatus: pb.DialResponse_E_DIAL_REFUSED,
				Error:          fmt.Errorf("写入失败: %w", err),
			}
		}
		return EventDialRequestCompleted{
			ResponseStatus: pb.DialResponse_E_DIAL_REFUSED,
		}
	}

	nonce := msg.GetDialRequest().Nonce

	isDialDataRequired := as.dialDataRequestPolicy(s, dialAddr)
	if isDialDataRequired && !as.limiter.AcceptDialDataRequest(p) {
		msg = pb.Message{
			Msg: &pb.Message_DialResponse{
				DialResponse: &pb.DialResponse{
					Status: pb.DialResponse_E_REQUEST_REJECTED,
				},
			},
		}
		if err := w.WriteMsg(&msg); err != nil {
			s.Reset()
			log.Debugf("向 %s 写入请求拒绝响应失败: %s", p, err)
			return EventDialRequestCompleted{
				ResponseStatus:   pb.DialResponse_E_REQUEST_REJECTED,
				Error:            fmt.Errorf("写入失败: %w", err),
				DialDataRequired: true,
			}
		}
		log.Debugf("拒绝来自 %s 的请求: 超出速率限制", p)
		return EventDialRequestCompleted{
			ResponseStatus:   pb.DialResponse_E_REQUEST_REJECTED,
			DialDataRequired: true,
		}
	}

	if isDialDataRequired {
		if err := getDialData(w, s, &msg, addrIdx); err != nil {
			s.Reset()
			log.Debugf("%s 拒绝拨号数据请求: %s", p, err)
			return EventDialRequestCompleted{
				Error:            errDialDataRefused,
				DialDataRequired: true,
				DialedAddr:       dialAddr,
			}
		}
		// 等待一段时间以防止雷鸣般的攻击
		waitTime := time.Duration(rand.Intn(int(as.amplificatonAttackPreventionDialWait) + 1)) // 范围是 [0, n)
		t := time.NewTimer(waitTime)
		defer t.Stop()
		select {
		case <-ctx.Done():
			s.Reset()
			log.Debugf("不进行拨号而拒绝请求: %s %p ", p, ctx.Err())
			return EventDialRequestCompleted{Error: ctx.Err(), DialDataRequired: true, DialedAddr: dialAddr}
		case <-t.C:
		}
	}

	dialStatus := as.dialBack(ctx, s.Conn().RemotePeer(), dialAddr, nonce)
	msg = pb.Message{
		Msg: &pb.Message_DialResponse{
			DialResponse: &pb.DialResponse{
				Status:     pb.DialResponse_OK,
				DialStatus: dialStatus,
				AddrIdx:    uint32(addrIdx),
			},
		},
	}
	if err := w.WriteMsg(&msg); err != nil {
		s.Reset()
		log.Debugf("向 %s 写入响应失败: %s", p, err)
		return EventDialRequestCompleted{
			ResponseStatus:   pb.DialResponse_OK,
			DialStatus:       dialStatus,
			Error:            fmt.Errorf("写入失败: %w", err),
			DialDataRequired: isDialDataRequired,
			DialedAddr:       dialAddr,
		}
	}
	return EventDialRequestCompleted{
		ResponseStatus:   pb.DialResponse_OK,
		DialStatus:       dialStatus,
		Error:            nil,
		DialDataRequired: isDialDataRequired,
		DialedAddr:       dialAddr,
	}
}

// getDialData 从客户端获取用于拨号地址的数据
// 参数:
//   - w: pbio.Writer 写入器
//   - s: network.Stream 网络流
//   - msg: *pb.Message 消息对象
//   - addrIdx: int 地址索引
//
// 返回值:
//   - error 错误信息
func getDialData(w pbio.Writer, s network.Stream, msg *pb.Message, addrIdx int) error {
	numBytes := minHandshakeSizeBytes + rand.Intn(maxHandshakeSizeBytes-minHandshakeSizeBytes)
	*msg = pb.Message{
		Msg: &pb.Message_DialDataRequest{
			DialDataRequest: &pb.DialDataRequest{
				AddrIdx:  uint32(addrIdx),
				NumBytes: uint64(numBytes),
			},
		},
	}
	if err := w.WriteMsg(msg); err != nil {
		log.Debugf("拨号数据写入失败: %v", err)
		return err
	}
	// 到目前为止在此流上使用的 pbio.Reader 是带缓冲的。但此时流上没有未读的内容。
	// 因此使用原始流进行读取是安全的,可以减少分配。
	return readDialData(numBytes, s)
}

// readDialData 读取拨号数据
// 参数:
//   - numBytes: int 要读取的字节数
//   - r: io.Reader 读取器
//
// 返回值:
//   - error 错误信息
func readDialData(numBytes int, r io.Reader) error {
	mr := &msgReader{R: r, Buf: pool.Get(maxMsgSize)}
	defer pool.Put(mr.Buf)
	for remain := numBytes; remain > 0; {
		msg, err := mr.ReadMsg()
		if err != nil {
			log.Debugf("拨号数据读取失败: %v", err)
			return err
		}
		// protobuf 格式为:
		// (oneof dialDataResponse:<fieldTag><len varint>)(dial data:<fieldTag><len varint><bytes>)
		bytesLen := len(msg)
		bytesLen -= 2 // fieldTag + varint 第一个字节
		if bytesLen > 127 {
			bytesLen -= 1 // varint 第二个字节
		}
		bytesLen -= 2 // 第二个 fieldTag + varint 第一个字节
		if bytesLen > 127 {
			bytesLen -= 1 // varint 第二个字节
		}
		if bytesLen > 0 {
			remain -= bytesLen
		}
		// 检查对等点是否发送太少的数据导致我们只是做了大量计算
		if bytesLen < 100 && remain > 0 {
			log.Debugf("拨号数据消息太小: %d", bytesLen)
			return fmt.Errorf("拨号数据消息太小: %d", bytesLen)
		}
	}
	return nil
}

// dialBack 执行回拨操作
// 参数:
//   - ctx: context.Context 上下文
//   - p: peer.ID 对等点 ID
//   - addr: ma.Multiaddr 地址
//   - nonce: uint64 随机数
//
// 返回值:
//   - pb.DialStatus 拨号状态
func (as *server) dialBack(ctx context.Context, p peer.ID, addr ma.Multiaddr, nonce uint64) pb.DialStatus {
	ctx, cancel := context.WithTimeout(ctx, dialBackDialTimeout)
	ctx = network.WithForceDirectDial(ctx, "autonatv2")
	as.dialerHost.Peerstore().AddAddr(p, addr, peerstore.TempAddrTTL)
	defer func() {
		cancel()
		as.dialerHost.Network().ClosePeer(p)
		as.dialerHost.Peerstore().ClearAddrs(p)
		as.dialerHost.Peerstore().RemovePeer(p)
	}()

	err := as.dialerHost.Connect(ctx, peer.AddrInfo{ID: p})
	if err != nil {
		log.Debugf("拨号失败: %v", err)
		return pb.DialStatus_E_DIAL_ERROR
	}

	s, err := as.dialerHost.NewStream(ctx, p, DialBackProtocol)
	if err != nil {
		log.Debugf("创建回拨流失败: %v", err)
		return pb.DialStatus_E_DIAL_BACK_ERROR
	}

	defer s.Close()
	s.SetDeadline(as.now().Add(dialBackStreamTimeout))

	w := pbio.NewDelimitedWriter(s)
	if err := w.WriteMsg(&pb.DialBack{Nonce: nonce}); err != nil {
		s.Reset()
		log.Debugf("写入回拨消息失败: %v", err)
		return pb.DialStatus_E_DIAL_BACK_ERROR
	}

	// 由于底层连接在单独的拨号器上,它将在此函数返回后关闭。
	// 连接关闭将丢弃所有排队的写入。
	// 为确保消息传递,执行 CloseWrite 并从流中读取一个字节。
	// 对等点实际上发送了 DialBackResponse 类型的响应,但我们只关心 DialBack 消息是否已到达对等点。所以我们在读取端忽略该消息。
	s.CloseWrite()
	s.SetDeadline(as.now().Add(5 * time.Second)) // 5 是一个魔法数字
	b := make([]byte, 1)                         // 这里读取 1 个字节,因为 0 长度读取可以立即返回 (0, nil)
	s.Read(b)

	return pb.DialStatus_OK
}

// rateLimiter 实现了每分钟请求的滑动窗口速率限制。它允许每个对等点 1 个并发请求。
// 它在全局级别、对等点级别以及是否需要拨号数据方面限制请求。
type rateLimiter struct {
	// PerPeerRPM 是每个对等点的速率限制
	PerPeerRPM int
	// RPM 是全局速率限制
	RPM int
	// DialDataRPM 是需要拨号数据的请求的速率限制
	DialDataRPM int

	mu           sync.Mutex
	closed       bool
	reqs         []entry
	peerReqs     map[peer.ID][]time.Time
	dialDataReqs []time.Time
	// ongoingReqs 跟踪正在进行的请求。
	// 这用于禁止同一对等点的多个并发请求
	// TODO: 我们是否应该允许每个对等点几个并发请求?
	ongoingReqs map[peer.ID]struct{}

	now func() time.Time // 用于测试
}

// entry 表示一个请求条目
type entry struct {
	PeerID peer.ID   // 对等点 ID
	Time   time.Time // 时间戳
}

// Accept 检查是否接受请求
// 参数:
//   - p: peer.ID 对等点 ID
//
// 返回值:
//   - bool 是否接受请求
func (r *rateLimiter) Accept(p peer.ID) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return false
	}
	if r.peerReqs == nil {
		r.peerReqs = make(map[peer.ID][]time.Time)
		r.ongoingReqs = make(map[peer.ID]struct{})
	}

	nw := r.now()
	r.cleanup(nw)

	if _, ok := r.ongoingReqs[p]; ok {
		return false
	}
	if len(r.reqs) >= r.RPM || len(r.peerReqs[p]) >= r.PerPeerRPM {
		return false
	}

	r.ongoingReqs[p] = struct{}{}
	r.reqs = append(r.reqs, entry{PeerID: p, Time: nw})
	r.peerReqs[p] = append(r.peerReqs[p], nw)
	return true
}

// AcceptDialDataRequest 检查是否接受拨号数据请求
// 参数:
//   - p: peer.ID 对等点 ID
//
// 返回值:
//   - bool 是否接受请求
func (r *rateLimiter) AcceptDialDataRequest(p peer.ID) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return false
	}
	if r.peerReqs == nil {
		r.peerReqs = make(map[peer.ID][]time.Time)
		r.ongoingReqs = make(map[peer.ID]struct{})
	}
	nw := r.now()
	r.cleanup(nw)
	if len(r.dialDataReqs) >= r.DialDataRPM {
		return false
	}
	r.dialDataReqs = append(r.dialDataReqs, nw)
	return true
}

// cleanup 移除过期的请求。
//
// 在速率受限的情况下这足够快,并且状态足够小以便在阻塞请求时快速清理。
// 参数:
//   - now: time.Time 当前时间
func (r *rateLimiter) cleanup(now time.Time) {
	idx := len(r.reqs)
	for i, e := range r.reqs {
		if now.Sub(e.Time) >= time.Minute {
			pi := len(r.peerReqs[e.PeerID])
			for j, t := range r.peerReqs[e.PeerID] {
				if now.Sub(t) < time.Minute {
					pi = j
					break
				}
			}
			r.peerReqs[e.PeerID] = r.peerReqs[e.PeerID][pi:]
			if len(r.peerReqs[e.PeerID]) == 0 {
				delete(r.peerReqs, e.PeerID)
			}
		} else {
			idx = i
			break
		}
	}
	r.reqs = r.reqs[idx:]

	idx = len(r.dialDataReqs)
	for i, t := range r.dialDataReqs {
		if now.Sub(t) < time.Minute {
			idx = i
			break
		}
	}
	r.dialDataReqs = r.dialDataReqs[idx:]
}

// CompleteRequest 完成请求
// 参数:
//   - p: peer.ID 对等点 ID
func (r *rateLimiter) CompleteRequest(p peer.ID) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.ongoingReqs, p)
}

// Close 关闭速率限制器
func (r *rateLimiter) Close() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.closed = true
	r.peerReqs = nil
	r.ongoingReqs = nil
	r.dialDataReqs = nil
}

// amplificationAttackPrevention 是一个 dialDataRequestPolicy,当对等点的观察到的 IP 地址与回拨 IP 地址不同时请求数据
// 参数:
//   - s: network.Stream 网络流
//   - dialAddr: ma.Multiaddr 拨号地址
//
// 返回值:
//   - bool 是否需要拨号数据
func amplificationAttackPrevention(s network.Stream, dialAddr ma.Multiaddr) bool {
	connIP, err := manet.ToIP(s.Conn().RemoteMultiaddr())
	if err != nil {
		log.Debugf("获取连接 IP 失败: %v", err)
		return true
	}
	dialIP, _ := manet.ToIP(s.Conn().LocalMultiaddr()) // 必须是 IP 多地址
	return !connIP.Equal(dialIP)
}
