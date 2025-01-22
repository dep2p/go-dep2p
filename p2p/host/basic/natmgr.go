package basichost

import (
	"context"
	"io"
	"net"
	"net/netip"
	"strconv"
	"sync"
	"time"

	"github.com/dep2p/go-dep2p/core/network"
	inat "github.com/dep2p/go-dep2p/p2p/net/nat"

	ma "github.com/dep2p/go-dep2p/multiformats/multiaddr"
	manet "github.com/dep2p/go-dep2p/multiformats/multiaddr/net"
)

// NATManager 是一个管理 NAT 设备的简单接口。
// 它监听来自 network.Network 的 Listen 和 ListenClose 通知,并尝试为这些通知获取端口映射。
type NATManager interface {
	// GetMapping 获取给定多地址的 NAT 映射地址
	// 参数:
	//   - ma.Multiaddr 原始多地址
	// 返回:
	//   - ma.Multiaddr NAT 映射后的多地址
	GetMapping(ma.Multiaddr) ma.Multiaddr

	// HasDiscoveredNAT 检查是否已发现 NAT 设备
	// 返回:
	//   - bool 是否发现 NAT 设备
	HasDiscoveredNAT() bool

	// Close 关闭 NAT 管理器
	// 返回:
	//   - error 关闭过程中的错误
	io.Closer
}

// NewNATManager 创建一个 NAT 管理器。
// 参数:
//   - net: network.Network 网络实例
//
// 返回:
//   - NATManager NAT 管理器实例
func NewNATManager(net network.Network) NATManager {
	return newNATManager(net)
}

// entry 表示一个端口映射条目
type entry struct {
	protocol string // 协议类型(tcp/udp)
	port     int    // 端口号
}

// nat 定义了 NAT 设备的基本操作接口
type nat interface {
	// AddMapping 添加端口映射
	// 参数:
	//   - ctx: context.Context 上下文
	//   - protocol: string 协议类型
	//   - port: int 端口号
	// 返回:
	//   - error 添加映射过程中的错误
	AddMapping(ctx context.Context, protocol string, port int) error

	// RemoveMapping 移除端口映射
	// 参数:
	//   - ctx: context.Context 上下文
	//   - protocol: string 协议类型
	//   - port: int 端口号
	// 返回:
	//   - error 移除映射过程中的错误
	RemoveMapping(ctx context.Context, protocol string, port int) error

	// GetMapping 获取端口映射
	// 参数:
	//   - protocol: string 协议类型
	//   - port: int 端口号
	// 返回:
	//   - netip.AddrPort 映射后的地址和端口
	//   - bool 是否存在映射
	GetMapping(protocol string, port int) (netip.AddrPort, bool)

	// Close 关闭 NAT 设备
	// 返回:
	//   - error 关闭过程中的错误
	io.Closer
}

// discoverNAT 用于发现 NAT 设备的函数变量,便于测试时进行模拟
var discoverNAT = func(ctx context.Context) (nat, error) { return inat.DiscoverNAT(ctx) }

// natManager 负责向 NAT 添加和移除端口映射。
// 如果主机启用了 NATPortMap 选项,则会初始化。
// natManager 接收来自网络的信号,并检查 NAT 映射:
//   - natManager 监听网络,并在网络发出 Listen() 或 ListenClose() 信号时添加或关闭端口映射。
//   - 关闭 natManager 会关闭 NAT 及其映射。
type natManager struct {
	net   network.Network // 网络实例
	natMx sync.RWMutex    // NAT 读写锁
	nat   nat             // NAT 设备实例

	syncFlag chan struct{} // 同步标志通道,容量为 1

	tracked map[entry]bool // 已跟踪的端口映射,布尔值仅在 doSync 中使用

	refCount  sync.WaitGroup     // 引用计数
	ctx       context.Context    // 上下文
	ctxCancel context.CancelFunc // 取消函数
}

// newNATManager 创建新的 NAT 管理器实例
// 参数:
//   - net: network.Network 网络实例
//
// 返回:
//   - *natManager NAT 管理器实例
func newNATManager(net network.Network) *natManager {
	// 创建带取消的上下文
	ctx, cancel := context.WithCancel(context.Background())
	// 初始化 NAT 管理器
	nmgr := &natManager{
		net:       net,
		syncFlag:  make(chan struct{}, 1),
		ctx:       ctx,
		ctxCancel: cancel,
		tracked:   make(map[entry]bool),
	}
	// 增加引用计数
	nmgr.refCount.Add(1)
	// 启动后台任务
	go nmgr.background(ctx)
	return nmgr
}

// Close 关闭 natManager,关闭底层 NAT 并取消注册网络事件。
// 返回:
//   - error 关闭过程中的错误
func (nmgr *natManager) Close() error {
	// 取消上下文
	nmgr.ctxCancel()
	// 等待所有引用计数归零
	nmgr.refCount.Wait()
	return nil
}

// HasDiscoveredNAT 检查是否已发现 NAT 设备
// 返回:
//   - bool 是否发现 NAT 设备
func (nmgr *natManager) HasDiscoveredNAT() bool {
	// 加读锁
	nmgr.natMx.RLock()
	defer nmgr.natMx.RUnlock()
	return nmgr.nat != nil
}

// background 运行 NAT 管理器的后台任务
// 参数:
//   - ctx: context.Context 上下文
func (nmgr *natManager) background(ctx context.Context) {
	// 完成时减少引用计数
	defer nmgr.refCount.Done()

	// 确保关闭 NAT 设备
	defer func() {
		nmgr.natMx.Lock()
		defer nmgr.natMx.Unlock()

		if nmgr.nat != nil {
			nmgr.nat.Close()
		}
	}()

	// 创建超时上下文用于发现 NAT
	discoverCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	// 发现 NAT 设备
	natInstance, err := discoverNAT(discoverCtx)
	if err != nil {
		log.Debugf("发现 NAT 错误: %v", err)
		return
	}

	// 保存 NAT 实例
	nmgr.natMx.Lock()
	nmgr.nat = natInstance
	nmgr.natMx.Unlock()

	// 注册网络通知
	nmgr.net.Notify((*nmgrNetNotifiee)(nmgr))
	defer nmgr.net.StopNotify((*nmgrNetNotifiee)(nmgr))

	// 首次同步
	nmgr.doSync()
	// 持续监听同步信号
	for {
		select {
		case <-nmgr.syncFlag:
			nmgr.doSync() // 当监听地址改变时同步
		case <-ctx.Done():
			return
		}
	}
}

// sync 触发同步操作
func (nmgr *natManager) sync() {
	// 尝试发送同步信号,如果通道已满则跳过
	select {
	case nmgr.syncFlag <- struct{}{}:
	default:
	}
}

// doSync 同步当前的 NAT 映射,移除过期的映射并添加新的映射。
func (nmgr *natManager) doSync() {
	// 重置所有跟踪的映射状态
	for e := range nmgr.tracked {
		nmgr.tracked[e] = false
	}
	// 存储新的地址
	var newAddresses []entry
	// 遍历所有监听地址
	for _, maddr := range nmgr.net.ListenAddresses() {
		// 分离 IP 地址部分
		maIP, rest := ma.SplitFirst(maddr)
		if maIP == nil || rest == nil {
			continue
		}

		// 只处理 IPv4 和 IPv6 地址
		switch maIP.Protocol().Code {
		case ma.P_IP6, ma.P_IP4:
		default:
			continue
		}

		// 检查 IP 地址类型
		ip := net.IP(maIP.RawValue())
		if !ip.IsGlobalUnicast() && !ip.IsUnspecified() {
			continue
		}

		// 提取协议和端口信息
		proto, _ := ma.SplitFirst(rest)
		if proto == nil {
			continue
		}

		// 确定协议类型
		var protocol string
		switch proto.Protocol().Code {
		case ma.P_TCP:
			protocol = "tcp"
		case ma.P_UDP:
			protocol = "udp"
		default:
			continue
		}
		// 解析端口号
		port, err := strconv.ParseUint(proto.Value(), 10, 16)
		if err != nil {
			// multiaddr 中的 bug
			panic(err)
		}
		// 创建映射条目
		e := entry{protocol: protocol, port: int(port)}
		if _, ok := nmgr.tracked[e]; ok {
			nmgr.tracked[e] = true
		} else {
			newAddresses = append(newAddresses, e)
		}
	}

	var wg sync.WaitGroup
	defer wg.Wait()

	// 移除过期的映射
	for e, v := range nmgr.tracked {
		if !v {
			nmgr.nat.RemoveMapping(nmgr.ctx, e.protocol, e.port)
			delete(nmgr.tracked, e)
		}
	}

	// 添加新的映射
	for _, e := range newAddresses {
		if err := nmgr.nat.AddMapping(nmgr.ctx, e.protocol, e.port); err != nil {
			log.Errorf("端口映射 %s 端口 %d 失败: %s", e.protocol, e.port, err)
		}
		nmgr.tracked[e] = false
	}
}

// GetMapping 获取给定多地址的 NAT 映射地址
// 参数:
//   - addr: ma.Multiaddr 原始多地址
//
// 返回:
//   - ma.Multiaddr NAT 映射后的多地址
func (nmgr *natManager) GetMapping(addr ma.Multiaddr) ma.Multiaddr {
	// 加锁保护
	nmgr.natMx.Lock()
	defer nmgr.natMx.Unlock()

	// 检查 NAT 是否已初始化
	if nmgr.nat == nil {
		return nil
	}

	// 解析协议和地址
	var found bool
	var proto int
	transport, rest := ma.SplitFunc(addr, func(c ma.Component) bool {
		if found {
			return true
		}
		proto = c.Protocol().Code
		found = proto == ma.P_TCP || proto == ma.P_UDP
		return false
	})
	if !manet.IsThinWaist(transport) {
		return nil
	}

	// 转换为网络地址
	naddr, err := manet.ToNetAddr(transport)
	if err != nil {
		log.Debugf("解析网络多地址 %q 错误: %v", transport, err)
		return nil
	}

	// 提取 IP、端口和协议信息
	var (
		ip       net.IP
		port     int
		protocol string
	)
	switch naddr := naddr.(type) {
	case *net.TCPAddr:
		ip = naddr.IP
		port = naddr.Port
		protocol = "tcp"
	case *net.UDPAddr:
		ip = naddr.IP
		port = naddr.Port
		protocol = "udp"
	default:
		return nil
	}

	// 检查 IP 地址类型
	if !ip.IsGlobalUnicast() && !ip.IsUnspecified() {
		return nil
	}

	// 获取映射地址
	extAddr, ok := nmgr.nat.GetMapping(protocol, port)
	if !ok {
		return nil
	}

	// 转换为对应类型的网络地址
	var mappedAddr net.Addr
	switch naddr.(type) {
	case *net.TCPAddr:
		mappedAddr = net.TCPAddrFromAddrPort(extAddr)
	case *net.UDPAddr:
		mappedAddr = net.UDPAddrFromAddrPort(extAddr)
	}
	// 转换为多地址格式
	mappedMaddr, err := manet.FromNetAddr(mappedAddr)
	if err != nil {
		log.Errorf("映射地址无法转换为多地址 %q: %s", mappedAddr, err)
		return nil
	}
	// 组合最终地址
	extMaddr := mappedMaddr
	if rest != nil {
		extMaddr = ma.Join(extMaddr, rest)
	}
	return extMaddr
}

// nmgrNetNotifiee 实现网络通知接口
type nmgrNetNotifiee natManager

// natManager 返回 NAT 管理器实例
func (nn *nmgrNetNotifiee) natManager() *natManager { return (*natManager)(nn) }

// Listen 处理网络监听事件
func (nn *nmgrNetNotifiee) Listen(network.Network, ma.Multiaddr) { nn.natManager().sync() }

// ListenClose 处理网络监听关闭事件
func (nn *nmgrNetNotifiee) ListenClose(n network.Network, addr ma.Multiaddr) { nn.natManager().sync() }

// Connected 处理网络连接事件
func (nn *nmgrNetNotifiee) Connected(network.Network, network.Conn) {}

// Disconnected 处理网络断开连接事件
func (nn *nmgrNetNotifiee) Disconnected(network.Network, network.Conn) {}
