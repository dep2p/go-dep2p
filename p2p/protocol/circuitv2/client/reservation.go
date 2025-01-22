package client

import (
	"context"
	"fmt"
	"time"

	"github.com/dep2p/go-dep2p/core/host"
	"github.com/dep2p/go-dep2p/core/peer"
	"github.com/dep2p/go-dep2p/core/peerstore"
	"github.com/dep2p/go-dep2p/core/record"
	pbv2 "github.com/dep2p/go-dep2p/p2p/protocol/circuitv2/pb"
	"github.com/dep2p/go-dep2p/p2p/protocol/circuitv2/proto"
	"github.com/dep2p/go-dep2p/p2p/protocol/circuitv2/util"

	ma "github.com/dep2p/go-dep2p/multiformats/multiaddr"
)

// 预留槽位的超时时间为1分钟
var ReserveTimeout = time.Minute

// Reservation 是一个包含 relay/v2 槽位预留信息的结构体
type Reservation struct {
	// Expiration 是预留的过期时间
	Expiration time.Time
	// Addrs 包含了经过验证的预留节点的公共地址,可以向网络宣告
	Addrs []ma.Multiaddr

	// LimitDuration 是中继节点保持中继连接打开的时间限制。如果为0,则没有限制
	LimitDuration time.Duration
	// LimitData 是中继节点在重置中继连接前,每个方向可以中继的字节数
	LimitData uint64

	// Voucher 是中继节点提供的已签名的预留凭证
	Voucher *proto.ReservationVoucher
}

// ReservationError 是在预留中继节点槽位失败时返回的错误
type ReservationError struct {
	// Status 是中继节点拒绝预留请求时返回的状态。在其他失败情况下设置为 pbv2.Status_CONNECTION_FAILED
	Status pbv2.Status

	// Reason 是预留失败的原因
	Reason string

	err error
}

// Error 实现了 error 接口,返回格式化的错误信息
func (re ReservationError) Error() string {
	return fmt.Sprintf("预留错误: 状态: %s 原因: %s 错误: %s", pbv2.Status_name[int32(re.Status)], re.Reason, re.err)
}

// Unwrap 返回底层错误
func (re ReservationError) Unwrap() error {
	return re.err
}

// Reserve 在中继节点中预留一个槽位并返回预留信息
// 客户端必须预留槽位才能让中继节点为其中继连接
//
// 参数:
//   - ctx: context.Context 上下文对象
//   - h: host.Host 主机对象
//   - ai: peer.AddrInfo 中继节点的地址信息
//
// 返回值:
//   - *Reservation 预留信息
//   - error 错误信息
func Reserve(ctx context.Context, h host.Host, ai peer.AddrInfo) (*Reservation, error) {
	// 如果提供了地址,将其添加到节点存储中
	if len(ai.Addrs) > 0 {
		h.Peerstore().AddAddrs(ai.ID, ai.Addrs, peerstore.TempAddrTTL)
	}

	// 打开到中继节点的流
	s, err := h.NewStream(ctx, ai.ID, proto.ProtoIDv2Hop)
	if err != nil {
		log.Debugf("打开流失败: %v", err)
		return nil, ReservationError{Status: pbv2.Status_CONNECTION_FAILED, Reason: "打开流失败", err: err}
	}
	defer s.Close()

	// 创建读写器
	rd := util.NewDelimitedReader(s, maxMessageSize)
	wr := util.NewDelimitedWriter(s)
	defer rd.Close()

	// 构造预留请求消息
	var msg pbv2.HopMessage
	msg.Type = pbv2.HopMessage_RESERVE.Enum()

	// 设置超时时间
	s.SetDeadline(time.Now().Add(ReserveTimeout))

	// 发送预留请求
	if err := wr.WriteMsg(&msg); err != nil {
		log.Debugf("写入预留消息失败: %v", err)
		s.Reset()
		return nil, ReservationError{Status: pbv2.Status_CONNECTION_FAILED, Reason: "写入预留消息时出错", err: err}
	}

	msg.Reset()

	// 读取响应
	if err := rd.ReadMsg(&msg); err != nil {
		log.Debugf("读取预留响应消息失败: %v", err)
		s.Reset()
		return nil, ReservationError{Status: pbv2.Status_CONNECTION_FAILED, Reason: "读取预留响应消息时出错: %w", err: err}
	}

	// 验证响应类型
	if msg.GetType() != pbv2.HopMessage_STATUS {
		log.Debugf("意外的中继响应:不是状态消息 (%d)", msg.GetType())
		return nil, ReservationError{Status: pbv2.Status_MALFORMED_MESSAGE, Reason: fmt.Sprintf("意外的中继响应:不是状态消息 (%d)", msg.GetType())}
	}

	// 检查状态
	if status := msg.GetStatus(); status != pbv2.Status_OK {
		log.Debugf("预留失败: %s (%d)", pbv2.Status_name[int32(status)], status)
		return nil, ReservationError{Status: msg.GetStatus(), Reason: "预留失败"}
	}

	// 获取预留信息
	rsvp := msg.GetReservation()
	if rsvp == nil {
		log.Debugf("缺少预留信息")
		return nil, ReservationError{Status: pbv2.Status_MALFORMED_MESSAGE, Reason: "缺少预留信息"}
	}

	// 构造返回结果
	result := &Reservation{}
	result.Expiration = time.Unix(int64(rsvp.GetExpire()), 0)
	if result.Expiration.Before(time.Now()) {
		log.Debugf("收到的预留过期时间在过去: %s", result.Expiration)
		return nil, ReservationError{
			Status: pbv2.Status_MALFORMED_MESSAGE,
			Reason: fmt.Sprintf("收到的预留过期时间在过去: %s", result.Expiration),
		}
	}

	// 解析地址
	addrs := rsvp.GetAddrs()
	result.Addrs = make([]ma.Multiaddr, 0, len(addrs))
	for _, ab := range addrs {
		a, err := ma.NewMultiaddrBytes(ab)
		if err != nil {
			log.Debugf("忽略无法解析的中继地址: %s", err)
			continue
		}
		result.Addrs = append(result.Addrs, a)
	}

	// 验证凭证
	voucherBytes := rsvp.GetVoucher()
	if voucherBytes != nil {
		env, rec, err := record.ConsumeEnvelope(voucherBytes, proto.RecordDomain)
		if err != nil {
			log.Debugf("解析凭证信封时出错: %s", err)
			return nil, ReservationError{
				Status: pbv2.Status_MALFORMED_MESSAGE,
				Reason: fmt.Sprintf("解析凭证信封时出错: %s", err),
				err:    err,
			}
		}

		voucher, ok := rec.(*proto.ReservationVoucher)
		if !ok {
			log.Debugf("意外的凭证记录类型: %+T", rec)
			return nil, ReservationError{
				Status: pbv2.Status_MALFORMED_MESSAGE,
				Reason: fmt.Sprintf("意外的凭证记录类型: %+T", rec),
			}
		}
		signerPeerID, err := peer.IDFromPublicKey(env.PublicKey)
		if err != nil {
			log.Debugf("无效的凭证签名公钥: %s", err)
			return nil, ReservationError{
				Status: pbv2.Status_MALFORMED_MESSAGE,
				Reason: fmt.Sprintf("无效的凭证签名公钥: %s", err),
				err:    err,
			}
		}
		if signerPeerID != voucher.Relay {
			log.Debugf("无效的凭证中继ID: 期望 %s, 得到 %s", signerPeerID, voucher.Relay)
			return nil, ReservationError{
				Status: pbv2.Status_MALFORMED_MESSAGE,
				Reason: fmt.Sprintf("无效的凭证中继ID: 期望 %s, 得到 %s", signerPeerID, voucher.Relay),
			}
		}
		if h.ID() != voucher.Peer {
			log.Debugf("无效的凭证节点ID: 期望 %s, 得到 %s", h.ID(), voucher.Peer)
			return nil, ReservationError{
				Status: pbv2.Status_MALFORMED_MESSAGE,
				Reason: fmt.Sprintf("无效的凭证节点ID: 期望 %s, 得到 %s", h.ID(), voucher.Peer),
			}

		}
		result.Voucher = voucher
	}

	// 设置限制
	limit := msg.GetLimit()
	if limit != nil {
		result.LimitDuration = time.Duration(limit.GetDuration()) * time.Second
		result.LimitData = limit.GetData()
	}

	return result, nil
}
