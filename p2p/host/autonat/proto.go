package autonat

import (
	"github.com/dep2p/go-dep2p/core/peer"
	"github.com/dep2p/go-dep2p/p2p/host/autonat/pb"

	ma "github.com/dep2p/go-dep2p/multiformats/multiaddr"
)

// AutoNATProto 标识 autonat 服务协议的版本号
const AutoNATProto = "/libp2p/autonat/1.0.0"

// newDialMessage 创建一个新的拨号消息
// 参数:
//   - pi: peer.AddrInfo 对等节点的地址信息
//
// 返回值:
//   - *pb.Message: 返回构造的拨号消息
func newDialMessage(pi peer.AddrInfo) *pb.Message {
	msg := new(pb.Message)                              // 创建新的消息对象
	msg.Type = pb.Message_DIAL.Enum()                   // 设置消息类型为拨号
	msg.Dial = new(pb.Message_Dial)                     // 创建拨号消息
	msg.Dial.Peer = new(pb.Message_PeerInfo)            // 创建对等节点信息
	msg.Dial.Peer.Id = []byte(pi.ID)                    // 设置对等节点ID
	msg.Dial.Peer.Addrs = make([][]byte, len(pi.Addrs)) // 初始化地址数组
	for i, addr := range pi.Addrs {                     // 遍历所有地址
		msg.Dial.Peer.Addrs[i] = addr.Bytes() // 将地址转换为字节数组
	}

	return msg
}

// newDialResponseOK 创建一个成功的拨号响应消息
// 参数:
//   - addr: ma.Multiaddr 成功连接的多地址
//
// 返回值:
//   - *pb.Message_DialResponse: 返回构造的拨号响应消息
func newDialResponseOK(addr ma.Multiaddr) *pb.Message_DialResponse {
	dr := new(pb.Message_DialResponse) // 创建新的拨号响应对象
	dr.Status = pb.Message_OK.Enum()   // 设置状态为成功
	dr.Addr = addr.Bytes()             // 设置连接的地址
	return dr
}

// newDialResponseError 创建一个错误的拨号响应消息
// 参数:
//   - status: pb.Message_ResponseStatus 响应状态码
//   - text: string 错误信息文本
//
// 返回值:
//   - *pb.Message_DialResponse: 返回构造的拨号响应消息
func newDialResponseError(status pb.Message_ResponseStatus, text string) *pb.Message_DialResponse {
	dr := new(pb.Message_DialResponse) // 创建新的拨号响应对象
	dr.Status = status.Enum()          // 设置错误状态码
	dr.StatusText = &text              // 设置错误信息
	return dr
}
