package client

import (
	"context"
	"fmt"
	"io"

	"github.com/dep2p/go-dep2p/core/host"
	"github.com/dep2p/go-dep2p/core/network"
	"github.com/dep2p/go-dep2p/core/peer"
	"github.com/dep2p/go-dep2p/core/transport"

	ma "github.com/dep2p/go-dep2p/multiformats/multiaddr"
)

// 中继协议的多地址协议代码
var circuitProtocol = ma.ProtocolWithCode(ma.P_CIRCUIT)

// 中继地址的多地址表示
var circuitAddr = ma.Cast(circuitProtocol.VCode)

// AddTransport 构造一个新的 p2p-circuit/v2 客户端并将其作为传输层添加到主机网络中
// 参数:
//   - h: host.Host 主机对象
//   - upgrader: transport.Upgrader 传输升级器
//
// 返回值:
//   - error 如果出现错误则返回错误信息
func AddTransport(h host.Host, upgrader transport.Upgrader) error {
	// 尝试将主机网络转换为传输网络接口
	n, ok := h.Network().(transport.TransportNetwork)
	if !ok {
		log.Debugf("主机网络 %v 不是传输网络", h.Network())
		return fmt.Errorf("主机网络 %v 不是传输网络", h.Network())
	}

	// 创建新的中继客户端
	c, err := New(h, upgrader)
	if err != nil {
		log.Debugf("构造中继客户端时出错: %w", err)
		return err
	}

	// 将中继传输添加到网络中
	err = n.AddTransport(c)
	if err != nil {
		log.Debugf("添加中继传输时出错: %w", err)
		return err
	}

	// 监听中继地址
	err = n.Listen(circuitAddr)
	if err != nil {
		log.Debugf("监听中继地址时出错: %w", err)
		return err
	}

	// 启动中继客户端
	c.Start()

	return nil
}

// 确保 Client 实现了 transport.Transport 接口
var _ transport.Transport = (*Client)(nil)

// p2p-circuit 实现了 SkipResolver 接口，这样底层传输可以在后面进行地址解析
// 如果你包装这个传输层，确保也要实现 SkipResolver 接口
var _ transport.SkipResolver = (*Client)(nil)
var _ io.Closer = (*Client)(nil)

// SkipResolve 总是返回 true，因为我们始终将实际连接委托给内部传输
// 通过在这里跳过解析，我们让内部传输决定如何解析多地址
// 参数:
//   - ctx: context.Context 上下文对象
//   - maddr: ma.Multiaddr 多地址
//
// 返回值:
//   - bool 始终返回 true
func (c *Client) SkipResolve(ctx context.Context, maddr ma.Multiaddr) bool {
	return true
}

// Dial 拨号连接到指定的对等节点
// 参数:
//   - ctx: context.Context 上下文对象
//   - a: ma.Multiaddr 目标多地址
//   - p: peer.ID 目标对等节点 ID
//
// 返回值:
//   - transport.CapableConn 建立的传输连接
//   - error 如果出现错误则返回错误信息
func (c *Client) Dial(ctx context.Context, a ma.Multiaddr, p peer.ID) (transport.CapableConn, error) {
	// 打开一个新的出站连接资源
	connScope, err := c.host.Network().ResourceManager().OpenConnection(network.DirOutbound, false, a)

	if err != nil {
		log.Debugf("打开连接资源时出错: %w", err)
		return nil, err
	}
	// 拨号并升级连接
	conn, err := c.dialAndUpgrade(ctx, a, p, connScope)
	if err != nil {
		connScope.Done()
		log.Debugf("拨号并升级连接时出错: %w", err)
		return nil, err
	}
	return conn, nil
}

// dialAndUpgrade 拨号并升级连接
// 参数:
//   - ctx: context.Context 上下文对象
//   - a: ma.Multiaddr 目标多地址
//   - p: peer.ID 目标对等节点 ID
//   - connScope: network.ConnManagementScope 连接管理范围
//
// 返回值:
//   - transport.CapableConn 升级后的传输连接
//   - error 如果出现错误则返回错误信息
func (c *Client) dialAndUpgrade(ctx context.Context, a ma.Multiaddr, p peer.ID, connScope network.ConnManagementScope) (transport.CapableConn, error) {
	if err := connScope.SetPeer(p); err != nil {
		log.Debugf("设置对等节点ID失败: %w", err)
		return nil, err
	}
	conn, err := c.dial(ctx, a, p)
	if err != nil {
		log.Debugf("拨号失败: %w", err)
		return nil, err
	}
	conn.tagHop()
	cc, err := c.upgrader.Upgrade(ctx, c, conn, network.DirOutbound, p, connScope)
	if err != nil {
		log.Debugf("升级连接失败: %w", err)
		return nil, err
	}
	return capableConn{cc.(capableConnWithStat)}, nil
}

// CanDial 检查是否可以拨号到指定地址
// 参数:
//   - addr: ma.Multiaddr 要检查的多地址
//
// 返回值:
//   - bool 如果地址包含中继协议则返回 true
func (c *Client) CanDial(addr ma.Multiaddr) bool {
	_, err := addr.ValueForProtocol(ma.P_CIRCUIT)
	if err != nil {
		log.Debugf("地址 %s 不是中继地址: %s", addr, err)
	}
	return err == nil
}

// Listen 在指定地址上监听连接
// 参数:
//   - addr: ma.Multiaddr 要监听的多地址
//
// 返回值:
//   - transport.Listener 传输监听器
//   - error 如果出现错误则返回错误信息
func (c *Client) Listen(addr ma.Multiaddr) (transport.Listener, error) {
	// TODO: 如果指定了中继，则连接到中继并保留槽位
	if _, err := addr.ValueForProtocol(ma.P_CIRCUIT); err != nil {
		log.Debugf("检查中继地址失败: %v", err)
		return nil, err
	}

	return c.upgrader.UpgradeListener(c, c.Listener()), nil
}

// Protocols 返回支持的协议列表
// 返回值:
//   - []int 支持的协议代码列表
func (c *Client) Protocols() []int {
	return []int{ma.P_CIRCUIT}
}

// Proxy 表示这是一个代理传输
// 返回值:
//   - bool 始终返回 true
func (c *Client) Proxy() bool {
	return true
}
