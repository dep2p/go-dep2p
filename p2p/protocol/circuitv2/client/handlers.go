package client

import (
	"time"

	"github.com/dep2p/go-dep2p/core/network"
	pbv2 "github.com/dep2p/go-dep2p/p2p/protocol/circuitv2/pb"
	"github.com/dep2p/go-dep2p/p2p/protocol/circuitv2/util"
)

// 连接超时相关常量
var (
	// StreamTimeout 流操作超时时间
	StreamTimeout = 1 * time.Minute
	// AcceptTimeout 接受连接超时时间
	AcceptTimeout = 10 * time.Second
)

// handleStreamV2 处理 relay/v2 协议的流
// 参数:
//   - s: network.Stream 网络流对象
//
// 注意:
//   - 该方法负责处理来自中继节点的连接请求
//   - 包含流的读写超时控制
//   - 处理各种协议错误情况
func (c *Client) handleStreamV2(s network.Stream) {
	// 记录来自远程节点的新流
	log.Debugf("来自节点的新 relay/v2 流: %s", s.Conn().RemotePeer())

	// 设置读取超时
	s.SetReadDeadline(time.Now().Add(StreamTimeout))

	// 创建带限制的分隔读取器
	rd := util.NewDelimitedReader(s, maxMessageSize)
	defer rd.Close()

	// writeResponse 用于写入响应消息
	// 参数:
	//   - status: pbv2.Status 状态码
	// 返回值:
	//   - error 写入过程中的错误
	writeResponse := func(status pbv2.Status) error {
		s.SetWriteDeadline(time.Now().Add(StreamTimeout))
		defer s.SetWriteDeadline(time.Time{})
		wr := util.NewDelimitedWriter(s)

		var msg pbv2.StopMessage
		msg.Type = pbv2.StopMessage_STATUS.Enum()
		msg.Status = status.Enum()

		return wr.WriteMsg(&msg)
	}

	// handleError 处理协议错误
	// 参数:
	//   - status: pbv2.Status 错误状态码
	handleError := func(status pbv2.Status) {
		log.Debugf("协议错误: %s (%d)", pbv2.Status_name[int32(status)], status)
		err := writeResponse(status)
		if err != nil {
			s.Reset()
			log.Debugf("写入电路响应时出错: %s", err.Error())
		} else {
			s.Close()
		}
	}

	var msg pbv2.StopMessage

	// 读取消息
	err := rd.ReadMsg(&msg)
	if err != nil {
		handleError(pbv2.Status_MALFORMED_MESSAGE)
		return
	}
	// 重置流的读取期限，因为消息已读取
	s.SetReadDeadline(time.Time{})

	// 检查消息类型是否为 CONNECT
	if msg.GetType() != pbv2.StopMessage_CONNECT {
		handleError(pbv2.Status_UNEXPECTED_MESSAGE)
		return
	}

	// 解析对等节点信息
	src, err := util.PeerToPeerInfoV2(msg.GetPeer())
	if err != nil {
		handleError(pbv2.Status_MALFORMED_MESSAGE)
		return
	}

	// 检查中继节点提供的限制
	// 如果限制不为空，则这是一个受限的中继连接，将连接标记为临时的
	var stat network.ConnStats
	if limit := msg.GetLimit(); limit != nil {
		stat.Limited = true
		stat.Extra = make(map[interface{}]interface{})
		stat.Extra[StatLimitDuration] = time.Duration(limit.GetDuration()) * time.Second
		stat.Extra[StatLimitData] = limit.GetData()
	}

	log.Debugf("收到来自节点的中继连接: %s", src.ID)

	// 尝试接受连接或超时
	select {
	case c.incoming <- accept{
		conn: &Conn{stream: s, remote: src, stat: stat, client: c},
		writeResponse: func() error {
			return writeResponse(pbv2.Status_OK)
		},
	}:
	case <-time.After(AcceptTimeout):
		handleError(pbv2.Status_CONNECTION_FAILED)
	}
}
