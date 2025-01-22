package autonatv2

import (
	"context"
	"fmt"
	"runtime/debug"
	"sync"
	"time"

	"github.com/dep2p/go-dep2p/core/host"
	"github.com/dep2p/go-dep2p/core/network"
	"github.com/dep2p/go-dep2p/core/peer"
	ma "github.com/dep2p/go-dep2p/multiformats/multiaddr"
	"github.com/dep2p/go-dep2p/p2p/protocol/autonatv2/pb"
	"github.com/dep2p/go-dep2p/p2plib/msgio/pbio"
	"golang.org/x/exp/rand"
)

// client 实现了 AutoNAT v2 的拨号请求客户端。
// 它验证拨号是否成功,并提供发送拨号请求数据的选项。
type client struct {
	// host 是 dep2p 主机实例
	host host.Host
	// dialData 是用于拨号请求的数据
	dialData []byte
	// normalizeMultiaddr 是用于标准化多地址的函数
	normalizeMultiaddr func(ma.Multiaddr) ma.Multiaddr

	mu sync.Mutex
	// dialBackQueues 将 nonce 映射到用于提供接收 nonce 的连接本地多地址的通道
	dialBackQueues map[uint64]chan ma.Multiaddr
}

// normalizeMultiaddrer 是一个接口,定义了标准化多地址的方法
type normalizeMultiaddrer interface {
	NormalizeMultiaddr(ma.Multiaddr) ma.Multiaddr
}

// newClient 创建一个新的 client 实例
// 参数:
//   - h: host.Host dep2p 主机实例
//
// 返回值:
//   - *client 新创建的客户端实例
func newClient(h host.Host) *client {
	// 默认的标准化函数,直接返回原地址
	normalizeMultiaddr := func(a ma.Multiaddr) ma.Multiaddr { return a }
	// 如果主机实现了 normalizeMultiaddrer 接口,使用其标准化方法
	if hn, ok := h.(normalizeMultiaddrer); ok {
		normalizeMultiaddr = hn.NormalizeMultiaddr
	}
	return &client{
		host:               h,
		dialData:           make([]byte, 4000),
		normalizeMultiaddr: normalizeMultiaddr,
		dialBackQueues:     make(map[uint64]chan ma.Multiaddr),
	}
}

// Start 启动客户端,设置回拨流处理器
func (ac *client) Start() {
	ac.host.SetStreamHandler(DialBackProtocol, ac.handleDialBack)
}

// Close 关闭客户端,移除回拨流处理器
func (ac *client) Close() {
	ac.host.RemoveStreamHandler(DialBackProtocol)
}

// GetReachability 通过 AutoNAT v2 服务器 p 验证地址的可达性
// 参数:
//   - ctx: context.Context 上下文
//   - p: peer.ID 服务器节点 ID
//   - reqs: []Request 要验证的地址请求列表
//
// 返回值:
//   - Result 验证结果
//   - error 错误信息
func (ac *client) GetReachability(ctx context.Context, p peer.ID, reqs []Request) (Result, error) {
	ctx, cancel := context.WithTimeout(ctx, streamTimeout)
	defer cancel()

	// 创建新的流
	s, err := ac.host.NewStream(ctx, p, DialProtocol)
	if err != nil {
		log.Debugf("打开 %s 流失败: %v", DialProtocol, err)
		return Result{}, fmt.Errorf("打开 %s 流失败: %w", DialProtocol, err)
	}

	// 设置流的服务名
	if err := s.Scope().SetService(ServiceName); err != nil {
		s.Reset()
		log.Debugf("将流 %s 附加到服务 %s 失败: %v", DialProtocol, ServiceName, err)
		return Result{}, fmt.Errorf("将流 %s 附加到服务 %s 失败: %w", DialProtocol, ServiceName, err)
	}

	// 为流预留内存
	if err := s.Scope().ReserveMemory(maxMsgSize, network.ReservationPriorityAlways); err != nil {
		s.Reset()
		log.Debugf("为流 %s 预留内存失败: %v", DialProtocol, err)
		return Result{}, fmt.Errorf("为流 %s 预留内存失败: %w", DialProtocol, err)
	}
	defer s.Scope().ReleaseMemory(maxMsgSize)

	s.SetDeadline(time.Now().Add(streamTimeout))
	defer s.Close()

	// 生成随机 nonce
	nonce := rand.Uint64()
	ch := make(chan ma.Multiaddr, 1)
	ac.mu.Lock()
	ac.dialBackQueues[nonce] = ch
	ac.mu.Unlock()
	defer func() {
		ac.mu.Lock()
		delete(ac.dialBackQueues, nonce)
		ac.mu.Unlock()
	}()

	// 发送拨号请求
	msg := newDialRequest(reqs, nonce)
	w := pbio.NewDelimitedWriter(s)
	if err := w.WriteMsg(&msg); err != nil {
		s.Reset()
		log.Debugf("拨号请求写入失败: %v", err)
		return Result{}, err
	}

	// 读取响应
	r := pbio.NewDelimitedReader(s, maxMsgSize)
	if err := r.ReadMsg(&msg); err != nil {
		s.Reset()
		log.Debugf("拨号消息读取失败: %v", err)
		return Result{}, err
	}

	switch {
	case msg.GetDialResponse() != nil:
		break
	// 如果需要,提供拨号数据
	case msg.GetDialDataRequest() != nil:
		if err := ac.validateDialDataRequest(reqs, &msg); err != nil {
			s.Reset()
			log.Debugf("无效的拨号数据请求: %v", err)
			return Result{}, err
		}
		// 拨号数据请求有效且我们要发送数据
		if err := sendDialData(ac.dialData, int(msg.GetDialDataRequest().GetNumBytes()), w, &msg); err != nil {
			s.Reset()
			log.Debugf("拨号数据发送失败: %v", err)
			return Result{}, err
		}
		if err := r.ReadMsg(&msg); err != nil {
			s.Reset()
			log.Debugf("拨号响应读取失败: %v", err)
			return Result{}, err
		}
		if msg.GetDialResponse() == nil {
			s.Reset()
			log.Debugf("无效的响应类型: %T", msg.Msg)
			return Result{}, err
		}
	default:
		s.Reset()
		log.Debugf("无效的消息类型: %T", msg.Msg)
		return Result{}, fmt.Errorf("无效的消息类型: %T", msg.Msg)
	}

	resp := msg.GetDialResponse()
	if resp.GetStatus() != pb.DialResponse_OK {
		// E_DIAL_REFUSED 对决定未来地址验证优先级有影响,包装一个独特的错误以方便使用 errors.Is
		if resp.GetStatus() == pb.DialResponse_E_DIAL_REFUSED {
			log.Debugf("拨号请求失败: %w", ErrDialRefused)
			return Result{}, ErrDialRefused
		}
		log.Debugf("拨号请求失败: 响应状态 %d %s", resp.GetStatus(),
			pb.DialResponse_ResponseStatus_name[int32(resp.GetStatus())])
		return Result{}, fmt.Errorf("拨号请求失败: 响应状态 %d %s", resp.GetStatus(),
			pb.DialResponse_ResponseStatus_name[int32(resp.GetStatus())])
	}
	if resp.GetDialStatus() == pb.DialStatus_UNUSED {
		log.Debugf("无效响应: 无效的拨号状态 UNUSED")
		return Result{}, fmt.Errorf("无效响应: 无效的拨号状态 UNUSED")
	}
	if int(resp.AddrIdx) >= len(reqs) {
		log.Debugf("无效响应: 地址索引超出范围: %d [0-%d)", resp.AddrIdx, len(reqs))
		return Result{}, fmt.Errorf("无效响应: 地址索引超出范围: %d [0-%d)", resp.AddrIdx, len(reqs))
	}

	// 等待服务器的 nonce
	var dialBackAddr ma.Multiaddr
	if resp.GetDialStatus() == pb.DialStatus_OK {
		timer := time.NewTimer(dialBackStreamTimeout)
		select {
		case at := <-ch:
			dialBackAddr = at
		case <-ctx.Done():
		case <-timer.C:
		}
		timer.Stop()
	}
	return ac.newResult(resp, reqs, dialBackAddr)
}

// validateDialDataRequest 验证拨号数据请求的有效性
// 参数:
//   - reqs: []Request 拨号请求列表
//   - msg: *pb.Message 消息
//
// 返回值:
//   - error 错误信息
func (ac *client) validateDialDataRequest(reqs []Request, msg *pb.Message) error {
	idx := int(msg.GetDialDataRequest().AddrIdx)
	if idx >= len(reqs) { // 无效的地址索引
		log.Debugf("地址索引超出范围: %d [0-%d)", idx, len(reqs))
		return fmt.Errorf("地址索引超出范围: %d [0-%d)", idx, len(reqs))
	}
	if msg.GetDialDataRequest().NumBytes > maxHandshakeSizeBytes { // 数据请求过大
		log.Debugf("请求的数据过大: %d", msg.GetDialDataRequest().NumBytes)
		return fmt.Errorf("请求的数据过大: %d", msg.GetDialDataRequest().NumBytes)
	}
	if !reqs[idx].SendDialData { // 低优先级地址
		log.Debugf("低优先级地址: %s 索引 %d", reqs[idx].Addr, idx)
		return fmt.Errorf("低优先级地址: %s 索引 %d", reqs[idx].Addr, idx)
	}
	return nil
}

// newResult 创建新的结果
// 参数:
//   - resp: *pb.DialResponse 拨号响应
//   - reqs: []Request 请求列表
//   - dialBackAddr: ma.Multiaddr 回拨地址
//
// 返回值:
//   - Result 结果
//   - error 错误信息
func (ac *client) newResult(resp *pb.DialResponse, reqs []Request, dialBackAddr ma.Multiaddr) (Result, error) {
	idx := int(resp.AddrIdx)
	addr := reqs[idx].Addr

	var rch network.Reachability
	switch resp.DialStatus {
	case pb.DialStatus_OK:
		if !ac.areAddrsConsistent(dialBackAddr, addr) {
			// 服务器错误地告知我们它成功拨号的地址
			// 要么我们没有收到回拨,要么回拨的地址与服务器告诉我们的不一致
			log.Debugf("无效响应: 回拨地址: %s, 响应地址: %s", dialBackAddr, addr)
			return Result{}, fmt.Errorf("无效响应: 回拨地址: %s, 响应地址: %s", dialBackAddr, addr)
		}
		rch = network.ReachabilityPublic
	case pb.DialStatus_E_DIAL_ERROR:
		rch = network.ReachabilityPrivate
	case pb.DialStatus_E_DIAL_BACK_ERROR:
		if ac.areAddrsConsistent(dialBackAddr, addr) {
			// 我们收到了回拨但服务器声称回拨出错。
			// 只要我们在回拨中收到了正确的 nonce,就可以安全地假设我们是公开的。
			rch = network.ReachabilityPublic
		} else {
			rch = network.ReachabilityUnknown
		}
	default:
		// 意外的响应代码。丢弃响应并失败。
		log.Debugf("收到地址 %s 的无效状态码: %d", addr, resp.DialStatus)
		return Result{}, fmt.Errorf("无效响应: 地址 %s 的无效状态码: %d", addr, resp.DialStatus)
	}

	return Result{
		Addr:         addr,
		Reachability: rch,
		Status:       resp.DialStatus,
	}, nil
}

// sendDialData 发送拨号数据
// 参数:
//   - dialData: []byte 拨号数据
//   - numBytes: int 字节数
//   - w: pbio.Writer 写入器
//   - msg: *pb.Message 消息
//
// 返回值:
//   - error 错误信息
func sendDialData(dialData []byte, numBytes int, w pbio.Writer, msg *pb.Message) (err error) {
	ddResp := &pb.DialDataResponse{Data: dialData}
	*msg = pb.Message{
		Msg: &pb.Message_DialDataResponse{
			DialDataResponse: ddResp,
		},
	}
	for remain := numBytes; remain > 0; {
		if remain < len(ddResp.Data) {
			ddResp.Data = ddResp.Data[:remain]
		}
		if err := w.WriteMsg(msg); err != nil {
			log.Debugf("写入失败: %v", err)
			return err
		}
		remain -= len(dialData)
	}
	return nil
}

// newDialRequest 创建新的拨号请求
// 参数:
//   - reqs: []Request 请求列表
//   - nonce: uint64 随机数
//
// 返回值:
//   - pb.Message 拨号请求消息
func newDialRequest(reqs []Request, nonce uint64) pb.Message {
	addrbs := make([][]byte, len(reqs))
	for i, r := range reqs {
		addrbs[i] = r.Addr.Bytes()
	}
	return pb.Message{
		Msg: &pb.Message_DialRequest{
			DialRequest: &pb.DialRequest{
				Addrs: addrbs,
				Nonce: nonce,
			},
		},
	}
}

// handleDialBack 接收回拨流上的 nonce
// 参数:
//   - s: network.Stream 网络流
func (ac *client) handleDialBack(s network.Stream) {
	defer func() {
		if rerr := recover(); rerr != nil {
			log.Debugf("捕获到 panic: %s\n%s\n", rerr, debug.Stack())
		}
		s.Reset()
	}()

	if err := s.Scope().SetService(ServiceName); err != nil {
		log.Debugf("将流附加到服务 %s 失败: %w", ServiceName, err)
		s.Reset()
		return
	}

	if err := s.Scope().ReserveMemory(dialBackMaxMsgSize, network.ReservationPriorityAlways); err != nil {
		log.Debugf("为流 %s 预留内存失败: %w", DialBackProtocol, err)
		s.Reset()
		return
	}
	defer s.Scope().ReleaseMemory(dialBackMaxMsgSize)

	s.SetDeadline(time.Now().Add(dialBackStreamTimeout))
	defer s.Close()

	r := pbio.NewDelimitedReader(s, dialBackMaxMsgSize)
	var msg pb.DialBack
	if err := r.ReadMsg(&msg); err != nil {
		log.Debugf("从 %s 读取回拨消息失败: %s", s.Conn().RemotePeer(), err)
		s.Reset()
		return
	}
	nonce := msg.GetNonce()

	ac.mu.Lock()
	ch := ac.dialBackQueues[nonce]
	ac.mu.Unlock()
	if ch == nil {
		log.Debugf("收到无效 nonce 的回拨: 本地地址: %s 对等点: %s nonce: %d", s.Conn().LocalMultiaddr(), s.Conn().RemotePeer(), nonce)
		s.Reset()
		return
	}
	select {
	case ch <- s.Conn().LocalMultiaddr():
	default:
		log.Debugf("收到多个回拨: 本地地址: %s 对等点: %s", s.Conn().LocalMultiaddr(), s.Conn().RemotePeer())
		s.Reset()
		return
	}
	w := pbio.NewDelimitedWriter(s)
	res := pb.DialBackResponse{}
	if err := w.WriteMsg(&res); err != nil {
		log.Debugf("写入回拨响应失败: %s", err)
		s.Reset()
	}
}

// areAddrsConsistent 检查连接的本地地址和拨号地址是否一致
// 参数:
//   - connLocalAddr: ma.Multiaddr 连接本地地址
//   - dialedAddr: ma.Multiaddr 拨号地址
//
// 返回值:
//   - bool 是否一致
func (ac *client) areAddrsConsistent(connLocalAddr, dialedAddr ma.Multiaddr) bool {
	if connLocalAddr == nil || dialedAddr == nil {
		return false
	}
	connLocalAddr = ac.normalizeMultiaddr(connLocalAddr)
	dialedAddr = ac.normalizeMultiaddr(dialedAddr)

	localProtos := connLocalAddr.Protocols()
	externalProtos := dialedAddr.Protocols()
	if len(localProtos) != len(externalProtos) {
		return false
	}
	for i := 0; i < len(localProtos); i++ {
		if i == 0 {
			switch externalProtos[i].Code {
			case ma.P_DNS, ma.P_DNSADDR:
				if localProtos[i].Code == ma.P_IP4 || localProtos[i].Code == ma.P_IP6 {
					continue
				}
				return false
			case ma.P_DNS4:
				if localProtos[i].Code == ma.P_IP4 {
					continue
				}
				return false
			case ma.P_DNS6:
				if localProtos[i].Code == ma.P_IP6 {
					continue
				}
				return false
			}
			if localProtos[i].Code != externalProtos[i].Code {
				return false
			}
		} else {
			if localProtos[i].Code != externalProtos[i].Code {
				return false
			}
		}
	}
	return true
}
