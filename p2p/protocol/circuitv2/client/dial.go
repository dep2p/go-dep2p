package client

import (
	"context"
	"fmt"
	"time"

	"github.com/dep2p/go-dep2p/core/network"
	"github.com/dep2p/go-dep2p/core/peer"
	"github.com/dep2p/go-dep2p/core/peerstore"
	pbv2 "github.com/dep2p/go-dep2p/p2p/protocol/circuitv2/pb"
	"github.com/dep2p/go-dep2p/p2p/protocol/circuitv2/proto"
	"github.com/dep2p/go-dep2p/p2p/protocol/circuitv2/util"

	ma "github.com/dep2p/go-dep2p/multiformats/multiaddr"
)

// 最大消息大小
const maxMessageSize = 4096

// 拨号超时时间
var DialTimeout = time.Minute

// 中继拨号超时时间
var DialRelayTimeout = 5 * time.Second

// 中继协议错误,用于信号去重
type relayError struct {
	err string
}

// Error 返回错误信息
// 返回值:
//   - string 错误信息字符串
func (e relayError) Error() string {
	return e.err
}

// newRelayError 创建新的中继错误
// 参数:
//   - t: string 错误模板字符串
//   - args: ...interface{} 错误参数
//
// 返回值:
//   - error 中继错误对象
func newRelayError(t string, args ...interface{}) error {
	return relayError{err: fmt.Sprintf(t, args...)}
}

// isRelayError 判断错误是否为中继错误
// 参数:
//   - err: error 待判断的错误对象
//
// 返回值:
//   - bool 是否为中继错误
func isRelayError(err error) bool {
	_, ok := err.(relayError)
	return ok
}

// dial 通过中继节点拨号连接到目标节点
// 参数:
//   - ctx: context.Context 上下文对象
//   - a: ma.Multiaddr 目标多地址
//   - p: peer.ID 目标节点ID
//
// 返回值:
//   - *Conn 连接对象
//   - error 错误信息
func (c *Client) dial(ctx context.Context, a ma.Multiaddr, p peer.ID) (*Conn, error) {
	// 将 /a/p2p-circuit/b 分割为 (/a, /p2p-circuit/b)
	relayaddr, destaddr := ma.SplitFunc(a, func(c ma.Component) bool {
		return c.Protocol().Code == ma.P_CIRCUIT
	})

	// 如果地址不包含 /p2p-circuit 部分,第二部分为空
	if destaddr == nil {
		log.Debugf("%s 不是一个中继地址", a)
		return nil, fmt.Errorf("%s 不是一个中继地址", a)
	}

	if relayaddr == nil {
		log.Debugf("无法在不指定中继的情况下拨号 p2p-circuit: %s", a)
		return nil, fmt.Errorf("无法在不指定中继的情况下拨号 p2p-circuit: %s", a)
	}

	dinfo := peer.AddrInfo{ID: p}

	// 从目标地址中去除 /p2p-circuit 前缀,以便可以传递目标地址(如果存在)用于活动中继
	_, destaddr = ma.SplitFirst(destaddr)
	if destaddr != nil {
		dinfo.Addrs = append(dinfo.Addrs, destaddr)
	}

	rinfo, err := peer.AddrInfoFromP2pAddr(relayaddr)
	if err != nil {
		log.Debugf("解析中继多地址 '%s' 出错: %v", relayaddr, err)
		return nil, fmt.Errorf("解析中继多地址 '%s' 出错: %w", relayaddr, err)
	}

	// 对同一节点的活动中继拨号去重
retry:
	c.mx.Lock()
	dedup, active := c.activeDials[p]
	if !active {
		dedup = &completion{ch: make(chan struct{}), relay: rinfo.ID}
		c.activeDials[p] = dedup
	}
	c.mx.Unlock()

	if active {
		select {
		case <-dedup.ch:
			if dedup.err != nil {
				if dedup.relay != rinfo.ID {
					// 不同的中继,重试
					goto retry
				}

				if !isRelayError(dedup.err) {
					// 非中继协议错误,重试
					goto retry
				}

				// 如果中继因协议错误而连接失败,不要尝试使用相同的中继
				log.Debugf("通过相同中继的并发活动拨号因协议错误而失败")
				return nil, fmt.Errorf("通过相同中继的并发活动拨号因协议错误而失败")
			}

			log.Debugf("并发活动拨号已成功")
			return nil, fmt.Errorf("并发活动拨号已成功")

		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	conn, err := c.dialPeer(ctx, *rinfo, dinfo)

	c.mx.Lock()
	dedup.err = err
	close(dedup.ch)
	delete(c.activeDials, p)
	c.mx.Unlock()

	return conn, err
}

// dialPeer 通过中继节点拨号连接到目标节点
// 参数:
//   - ctx: context.Context 上下文对象
//   - relay: peer.AddrInfo 中继节点信息
//   - dest: peer.AddrInfo 目标节点信息
//
// 返回值:
//   - *Conn 连接对象
//   - error 错误信息
func (c *Client) dialPeer(ctx context.Context, relay, dest peer.AddrInfo) (*Conn, error) {
	log.Debugf("通过中继 %s 拨号连接节点 %s", dest.ID, relay.ID)

	if len(relay.Addrs) > 0 {
		c.host.Peerstore().AddAddrs(relay.ID, relay.Addrs, peerstore.TempAddrTTL)
	}

	dialCtx, cancel := context.WithTimeout(ctx, DialRelayTimeout)
	defer cancel()
	s, err := c.host.NewStream(dialCtx, relay.ID, proto.ProtoIDv2Hop)
	if err != nil {
		log.Debugf("打开到中继的 hop 流时出错: %v", err)
		return nil, err
	}
	return c.connect(s, dest)
}

// connect 建立到目标节点的连接
// 参数:
//   - s: network.Stream 网络流
//   - dest: peer.AddrInfo 目标节点信息
//
// 返回值:
//   - *Conn 连接对象
//   - error 错误信息
func (c *Client) connect(s network.Stream, dest peer.AddrInfo) (*Conn, error) {
	if err := s.Scope().ReserveMemory(maxMessageSize, network.ReservationPriorityAlways); err != nil {
		log.Debugf("为流 %s 预留内存失败: %v", s.ID(), err)
		s.Reset()
		return nil, err
	}
	defer s.Scope().ReleaseMemory(maxMessageSize)

	rd := util.NewDelimitedReader(s, maxMessageSize)
	wr := util.NewDelimitedWriter(s)
	defer rd.Close()

	var msg pbv2.HopMessage

	msg.Type = pbv2.HopMessage_CONNECT.Enum()
	msg.Peer = util.PeerInfoToPeerV2(dest)

	s.SetDeadline(time.Now().Add(DialTimeout))

	err := wr.WriteMsg(&msg)
	if err != nil {
		log.Debugf("写入拨号消息失败: %v", err)
		s.Reset()
		return nil, err
	}

	msg.Reset()

	err = rd.ReadMsg(&msg)
	if err != nil {
		log.Debugf("读取拨号消息失败: %v", err)
		s.Reset()
		return nil, err
	}

	s.SetDeadline(time.Time{})

	if msg.GetType() != pbv2.HopMessage_STATUS {
		log.Debugf("意外的中继响应;不是状态消息 (%d)", msg.GetType())
		s.Reset()
		return nil, newRelayError("意外的中继响应;不是状态消息 (%d)", msg.GetType())
	}

	status := msg.GetStatus()
	if status != pbv2.Status_OK {
		log.Debugf("打开中继电路时出错: %s (%d)", pbv2.Status_name[int32(status)], status)
		s.Reset()
		return nil, newRelayError("打开中继电路时出错: %s (%d)", pbv2.Status_name[int32(status)], status)
	}

	// 检查中继提供的限制;如果限制不为空,则这是一个受限的中继连接,我们将连接标记为临时的
	var stat network.ConnStats
	if limit := msg.GetLimit(); limit != nil {
		stat.Limited = true
		stat.Extra = make(map[interface{}]interface{})
		stat.Extra[StatLimitDuration] = time.Duration(limit.GetDuration()) * time.Second
		stat.Extra[StatLimitData] = limit.GetData()
	}

	return &Conn{stream: s, remote: dest, stat: stat, client: c}, nil
}
