package client

import (
	"context"
	"io"
	"sync"

	"github.com/dep2p/go-dep2p/core/host"
	"github.com/dep2p/go-dep2p/core/peer"
	"github.com/dep2p/go-dep2p/core/transport"
	"github.com/dep2p/go-dep2p/p2p/protocol/circuitv2/proto"

	logging "github.com/dep2p/log"
)

var log = logging.Logger("p2p-protocol-circuitv2-client")

// Client 实现了 p2p-circuit/v2 协议的客户端:
// - 实现通过 v2 中继进行拨号
// - 监听来自 v2 中继的传入连接
//
// 为了向后兼容 v1 中继和旧节点，客户端也会接受通过 v1 中继的连接，并使用 p2p-circuit/v1 进行回退拨号。
// 这允许我们在主机中使用 v2 代码作为 v1 的替代品，而不会破坏现有代码和与旧节点的互操作性。
type Client struct {
	// ctx 是客户端的上下文
	ctx context.Context
	// ctxCancel 是取消上下文的函数
	ctxCancel context.CancelFunc
	// host 是客户端所属的主机
	host host.Host
	// upgrader 用于执行连接升级
	upgrader transport.Upgrader

	// incoming 是接收传入连接的通道
	incoming chan accept

	// mx 是用于保护共享资源的互斥锁
	mx sync.Mutex
	// activeDials 记录活跃的拨号操作
	activeDials map[peer.ID]*completion
	// hopCount 记录每个对等点的跳数
	hopCount map[peer.ID]int
}

var _ io.Closer = &Client{}
var _ transport.Transport = &Client{}

// accept 表示接受的连接及其响应写入函数
type accept struct {
	// conn 是接受的连接
	conn *Conn
	// writeResponse 是写入响应的函数
	writeResponse func() error
}

// completion 表示拨号完成的状态
type completion struct {
	// ch 是完成信号通道
	ch chan struct{}
	// relay 是中继节点的ID
	relay peer.ID
	// err 是拨号过程中的错误
	err error
}

// New 创建一个新的 p2p-circuit/v2 客户端
// 参数:
//   - h: host.Host 主机对象
//   - upgrader: transport.Upgrader 连接升级器
//
// 返回值:
//   - *Client 新创建的客户端对象
//   - error 创建过程中的错误
func New(h host.Host, upgrader transport.Upgrader) (*Client, error) {
	cl := &Client{
		host:        h,
		upgrader:    upgrader,
		incoming:    make(chan accept),
		activeDials: make(map[peer.ID]*completion),
		hopCount:    make(map[peer.ID]int),
	}
	cl.ctx, cl.ctxCancel = context.WithCancel(context.Background())
	return cl, nil
}

// Start 注册电路(客户端)协议流处理程序
// 注意:
//   - 此方法会设置 v2 协议的流处理器
func (c *Client) Start() {
	c.host.SetStreamHandler(proto.ProtoIDv2Stop, c.handleStreamV2)
}

// Close 关闭客户端
// 返回值:
//   - error 关闭过程中的错误
//
// 注意:
//   - 此方法会取消上下文并移除流处理器
func (c *Client) Close() error {
	c.ctxCancel()
	c.host.RemoveStreamHandler(proto.ProtoIDv2Stop)
	return nil
}
