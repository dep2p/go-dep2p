package autonat

import (
	"context"
	"fmt"
	"time"

	"github.com/dep2p/go-dep2p/core/host"
	"github.com/dep2p/go-dep2p/core/network"
	"github.com/dep2p/go-dep2p/core/peer"
	"github.com/dep2p/go-dep2p/p2p/host/autonat/pb"

	"github.com/dep2p/go-dep2p/p2plib/msgio/pbio"
)

// NewAutoNATClient 创建一个新的AutoNAT客户端实例
// 参数:
//   - h: host.Host 主机实例,用于创建流和获取节点ID
//   - addrFunc: AddrFunc 地址获取函数,用于获取本地地址,如果为nil则使用h.Addrs
//   - mt: MetricsTracer 指标追踪器,用于记录指标数据
//
// 返回值:
//   - Client: 返回创建的AutoNAT客户端实例,用于执行回拨测试
func NewAutoNATClient(h host.Host, addrFunc AddrFunc, mt MetricsTracer) Client {
	// 如果未提供地址获取函数,则使用主机的Addrs方法
	if addrFunc == nil {
		addrFunc = h.Addrs
	}
	// 创建并返回客户端实例
	return &client{h: h, addrFunc: addrFunc, mt: mt}
}

// client 实现了AutoNAT客户端
type client struct {
	h        host.Host     // dep2p主机实例,用于创建流和获取节点ID
	addrFunc AddrFunc      // 获取地址的函数,用于获取本地地址
	mt       MetricsTracer // 指标追踪器,用于记录指标数据
}

// DialBack 请求对等节点p回拨我们的所有地址
// 参数:
//   - ctx: context.Context 上下文,用于控制请求的生命周期
//   - p: peer.ID 对等节点ID,指定要请求回拨的节点
//
// 返回值:
//   - error: 如果发生错误则返回错误信息,nil表示回拨成功
//
// 注意: 返回的Message_E_DIAL_ERROR错误并不意味着服务器实际执行了回拨尝试
// 运行版本<v0.20.0的服务器如果由于拨号策略跳过拨号,也会返回Message_E_DIAL_ERROR
func (c *client) DialBack(ctx context.Context, p peer.ID) error {
	// 创建新的流与对等节点通信
	s, err := c.h.NewStream(ctx, p, AutoNATProto)
	if err != nil {
		log.Debugf("创建流与对等节点通信失败: %v", err)
		return err
	}

	// 设置流的服务名称以便识别
	if err := s.Scope().SetService(ServiceName); err != nil {
		log.Debugf("设置autonat服务流时出错: %s", err)
		s.Reset()
		return err
	}

	// 为流预留内存以防止内存溢出
	if err := s.Scope().ReserveMemory(maxMsgSize, network.ReservationPriorityAlways); err != nil {
		log.Debugf("为autonat流预留内存时出错: %s", err)
		s.Reset()
		return err
	}
	defer s.Scope().ReleaseMemory(maxMsgSize)

	// 设置流的截止时间
	deadline := time.Now().Add(streamTimeout)
	if ctxDeadline, ok := ctx.Deadline(); ok {
		if ctxDeadline.Before(deadline) {
			deadline = ctxDeadline
		}
	}

	// 设置流的截止时间
	s.SetDeadline(deadline)
	// 到这里就直接重置流,不需要优雅关闭
	defer s.Close()

	// 创建protobuf消息的读写器
	r := pbio.NewDelimitedReader(s, maxMsgSize)
	w := pbio.NewDelimitedWriter(s)

	// 创建并发送拨号请求消息
	req := newDialMessage(peer.AddrInfo{ID: c.h.ID(), Addrs: c.addrFunc()})
	if err := w.WriteMsg(req); err != nil {
		log.Debugf("发送拨号请求消息失败: %v", err)
		s.Reset()
		return err
	}

	// 读取响应消息
	var res pb.Message
	if err := r.ReadMsg(&res); err != nil {
		log.Debugf("读取响应消息失败: %v", err)
		s.Reset()
		return err
	}
	// 检查响应类型是否正确
	if res.GetType() != pb.Message_DIAL_RESPONSE {
		s.Reset()
		log.Debugf("意外的响应类型: %s", res.GetType().String())
		return fmt.Errorf("意外的响应类型: %s", res.GetType().String())
	}

	// 获取响应状态并记录指标
	status := res.GetDialResponse().GetStatus()
	if c.mt != nil {
		c.mt.ReceivedDialResponse(status)
	}
	// 根据状态返回结果
	switch status {
	case pb.Message_OK:
		return nil
	default:
		return Error{Status: status, Text: res.GetDialResponse().GetStatusText()}
	}
}

// Error 封装了AutoNAT服务返回的错误
type Error struct {
	Status pb.Message_ResponseStatus // 响应状态,表示错误类型
	Text   string                    // 错误文本,提供详细的错误信息
}

// Error 实现error接口
// 返回值:
//   - string: 返回格式化的错误信息
func (e Error) Error() string {
	log.Debugf("AutoNAT错误: %s (%s)", e.Text, e.Status.String())
	return fmt.Sprintf("AutoNAT错误: %s (%s)", e.Text, e.Status.String())
}

// IsDialError 判断是否为回拨失败错误
// 返回值:
//   - bool: 如果状态为E_DIAL_ERROR则返回true
func (e Error) IsDialError() bool {
	return e.Status == pb.Message_E_DIAL_ERROR
}

// IsDialRefused 判断是否为拒绝回拨错误
// 返回值:
//   - bool: 如果状态为E_DIAL_REFUSED则返回true
func (e Error) IsDialRefused() bool {
	return e.Status == pb.Message_E_DIAL_REFUSED
}

// IsDialError 判断错误是否表示AutoNAT对等节点回拨失败
// 参数:
//   - e: error 错误实例,要检查的错误
//
// 返回值:
//   - bool: 如果是回拨失败错误则返回true
func IsDialError(e error) bool {
	ae, ok := e.(Error)
	return ok && ae.IsDialError()
}

// IsDialRefused 判断错误是否表示AutoNAT对等节点拒绝回拨
// 参数:
//   - e: error 错误实例,要检查的错误
//
// 返回值:
//   - bool: 如果是拒绝回拨错误则返回true
func IsDialRefused(e error) bool {
	ae, ok := e.(Error)
	return ok && ae.IsDialRefused()
}
