package rcmgr

import (
	"context"
	"fmt"
	"net/netip"
	"strings"
	"sync"
	"time"

	"github.com/dep2p/go-dep2p/core/network"
	"github.com/dep2p/go-dep2p/core/peer"
	"github.com/dep2p/go-dep2p/core/protocol"

	"github.com/dep2p/go-dep2p/multiformats/multiaddr"
	manet "github.com/dep2p/go-dep2p/multiformats/multiaddr/net"
	logging "github.com/dep2p/log"
)

// 资源管理器的日志记录器
var log = logging.Logger("host-resource-manager")

// resourceManager 实现了资源管理器的核心功能
type resourceManager struct {
	// 资源限制器接口
	limits Limiter

	// 连接限制器
	connLimiter *connLimiter

	// 跟踪器,用于记录资源使用情况
	trace *trace
	// 指标收集器
	metrics *metrics
	// 是否禁用指标收集
	disableMetrics bool

	// 白名单配置
	allowlist *Allowlist

	// 系统级资源作用域
	system *systemScope
	// 临时资源作用域
	transient *transientScope

	// 白名单系统级资源作用域
	allowlistedSystem *systemScope
	// 白名单临时资源作用域
	allowlistedTransient *transientScope

	// 用于取消操作的上下文
	cancelCtx context.Context
	// 取消函数
	cancel func()
	// 等待组,用于优雅关闭
	wg sync.WaitGroup

	// 互斥锁,保护以下映射
	mx sync.Mutex
	// 服务作用域映射
	svc map[string]*serviceScope
	// 协议作用域映射
	proto map[protocol.ID]*protocolScope
	// 对等点作用域映射
	peer map[peer.ID]*peerScope

	// 固定协议集合
	stickyProto map[protocol.ID]struct{}
	// 固定对等点集合
	stickyPeer map[peer.ID]struct{}

	// 连接和流的唯一标识符
	connId, streamId int64
}

// 确保 resourceManager 实现了 network.ResourceManager 接口
var _ network.ResourceManager = (*resourceManager)(nil)

// systemScope 表示系统级资源作用域
type systemScope struct {
	*resourceScope
}

// 确保 systemScope 实现了 network.ResourceScope 接口
var _ network.ResourceScope = (*systemScope)(nil)

// transientScope 表示临时资源作用域
type transientScope struct {
	*resourceScope

	// 关联的系统作用域
	system *systemScope
}

// 确保 transientScope 实现了 network.ResourceScope 接口
var _ network.ResourceScope = (*transientScope)(nil)

// serviceScope 表示服务级资源作用域
type serviceScope struct {
	*resourceScope

	// 服务名称
	service string
	// 所属资源管理器
	rcmgr *resourceManager

	// 服务相关的对等点作用域映射
	peers map[peer.ID]*resourceScope
}

// 确保 serviceScope 实现了 network.ServiceScope 接口
var _ network.ServiceScope = (*serviceScope)(nil)

// protocolScope 表示协议级资源作用域
type protocolScope struct {
	*resourceScope

	// 协议标识符
	proto protocol.ID
	// 所属资源管理器
	rcmgr *resourceManager

	// 协议相关的对等点作用域映射
	peers map[peer.ID]*resourceScope
}

// 确保 protocolScope 实现了 network.ProtocolScope 接口
var _ network.ProtocolScope = (*protocolScope)(nil)

// peerScope 表示对等点级资源作用域
type peerScope struct {
	*resourceScope

	// 对等点标识符
	peer peer.ID
	// 所属资源管理器
	rcmgr *resourceManager
}

// 确保 peerScope 实现了 network.PeerScope 接口
var _ network.PeerScope = (*peerScope)(nil)

// connectionScope 表示连接级资源作用域
type connectionScope struct {
	*resourceScope

	// 连接方向(入站/出站)
	dir network.Direction
	// 是否使用文件描述符
	usefd bool
	// 是否在白名单中
	isAllowlisted bool
	// 所属资源管理器
	rcmgr *resourceManager
	// 关联的对等点作用域
	peer *peerScope
	// 连接端点地址
	endpoint multiaddr.Multiaddr
	// IP地址
	ip netip.Addr
}

// 确保 connectionScope 实现了相关接口
var _ network.ConnScope = (*connectionScope)(nil)
var _ network.ConnManagementScope = (*connectionScope)(nil)

// streamScope 表示流级资源作用域
type streamScope struct {
	*resourceScope

	// 流方向(入站/出站)
	dir network.Direction
	// 所属资源管理器
	rcmgr *resourceManager
	// 关联的对等点作用域
	peer *peerScope
	// 关联的服务作用域
	svc *serviceScope
	// 关联的协议作用域
	proto *protocolScope

	// 对等点-协议组合作用域
	peerProtoScope *resourceScope
	// 对等点-服务组合作用域
	peerSvcScope *resourceScope
}

// 确保 streamScope 实现了相关接口
var _ network.StreamScope = (*streamScope)(nil)
var _ network.StreamManagementScope = (*streamScope)(nil)

// Option 是一个函数类型,用于配置资源管理器
type Option func(*resourceManager) error

// NewResourceManager 创建一个新的资源管理器
// 参数:
//   - limits: 资源限制器
//   - opts: 可选的配置选项
//
// 返回:
//   - network.ResourceManager: 资源管理器接口实现
//   - error: 错误信息
func NewResourceManager(limits Limiter, opts ...Option) (network.ResourceManager, error) {
	// 创建新的白名单
	allowlist := newAllowlist()
	// 初始化资源管理器
	r := &resourceManager{
		limits:      limits,                               // 设置限制器
		connLimiter: newConnLimiter(),                     // 创建连接限制器
		allowlist:   &allowlist,                           // 设置白名单
		svc:         make(map[string]*serviceScope),       // 初始化服务作用域映射
		proto:       make(map[protocol.ID]*protocolScope), // 初始化协议作用域映射
		peer:        make(map[peer.ID]*peerScope),         // 初始化对等点作用域映射
	}

	// 应用所有配置选项
	for _, opt := range opts {
		if err := opt(r); err != nil {
			return nil, err
		}
	}

	// 记录已注册的网络前缀限制
	registeredConnLimiterPrefixes := make(map[string]struct{})
	// 记录IPv4网络前缀限制
	for _, npLimit := range r.connLimiter.networkPrefixLimitV4 {
		registeredConnLimiterPrefixes[npLimit.Network.String()] = struct{}{}
	}
	// 记录IPv6网络前缀限制
	for _, npLimit := range r.connLimiter.networkPrefixLimitV6 {
		registeredConnLimiterPrefixes[npLimit.Network.String()] = struct{}{}
	}
	// 处理白名单中的网络
	for _, network := range allowlist.allowedNetworks {
		prefix, err := netip.ParsePrefix(network.String())
		if err != nil {
			log.Debugf("解析白名单前缀失败 %s, %s", network, err)
			continue
		}
		if _, ok := registeredConnLimiterPrefixes[prefix.String()]; !ok {
			// 为未知网络添加限制
			r.connLimiter.addNetworkPrefixLimit(prefix.Addr().Is6(), NetworkPrefixLimit{
				Network:   prefix,
				ConnCount: r.limits.GetAllowlistedSystemLimits().GetConnTotalLimit(),
			})
		}
	}

	// 初始化指标收集
	if !r.disableMetrics {
		var sr TraceReporter
		sr, err := NewStatsTraceReporter()
		if err != nil {
			log.Errorf("初始化统计跟踪报告器失败 %s", err)
		} else {
			if r.trace == nil {
				r.trace = &trace{}
			}
			found := false
			for _, rep := range r.trace.reporters {
				if rep == sr {
					found = true
					break
				}
			}
			if !found {
				r.trace.reporters = append(r.trace.reporters, sr)
			}
		}
	}

	// 启动跟踪
	if err := r.trace.Start(limits); err != nil {
		return nil, err
	}

	// 初始化系统作用域
	r.system = newSystemScope(limits.GetSystemLimits(), r, "system")
	r.system.IncRef()
	// 初始化瞬时作用域
	r.transient = newTransientScope(limits.GetTransientLimits(), r, "transient", r.system.resourceScope)
	r.transient.IncRef()

	// 初始化白名单系统作用域
	r.allowlistedSystem = newSystemScope(limits.GetAllowlistedSystemLimits(), r, "allowlistedSystem")
	r.allowlistedSystem.IncRef()
	// 初始化白名单瞬时作用域
	r.allowlistedTransient = newTransientScope(limits.GetAllowlistedTransientLimits(), r, "allowlistedTransient", r.allowlistedSystem.resourceScope)
	r.allowlistedTransient.IncRef()

	// 创建上下文和取消函数
	r.cancelCtx, r.cancel = context.WithCancel(context.Background())

	// 启动后台任务
	r.wg.Add(1)
	go r.background()

	return r, nil
}

// GetAllowlist 返回资源管理器的白名单
// 参数:
//   - 无
//
// 返回值:
//   - *Allowlist: 资源管理器的白名单
func (r *resourceManager) GetAllowlist() *Allowlist {
	return r.allowlist // 返回资源管理器的白名单
}

// GetAllowlist 尝试从给定的资源管理器接口获取白名单
// 参数:
//   - rcmgr: 资源管理器接口
//
// 返回值:
//   - *Allowlist: 白名单对象,如果获取失败则返回nil
func GetAllowlist(rcmgr network.ResourceManager) *Allowlist {
	r, ok := rcmgr.(*resourceManager) // 尝试将接口转换为具体的resourceManager类型
	if !ok {                          // 如果转换失败
		return nil // 返回nil
	}

	return r.allowlist // 返回白名单
}

// ViewSystem 在系统作用域上执行给定的函数
// 参数:
//   - f: 要在系统作用域上执行的函数
//
// 返回值:
//   - error: 执行过程中的错误,如果成功则为nil
func (r *resourceManager) ViewSystem(f func(network.ResourceScope) error) error {
	return f(r.system) // 在系统作用域上执行函数
}

// ViewTransient 在瞬时作用域上执行给定的函数
// 参数:
//   - f: 要在瞬时作用域上执行的函数
//
// 返回值:
//   - error: 执行过程中的错误,如果成功则为nil
func (r *resourceManager) ViewTransient(f func(network.ResourceScope) error) error {
	return f(r.transient) // 在瞬时作用域上执行函数
}

// ViewService 在服务作用域上执行给定的函数
// 参数:
//   - srv: 服务名称
//   - f: 要在服务作用域上执行的函数
//
// 返回值:
//   - error: 执行过程中的错误,如果成功则为nil
func (r *resourceManager) ViewService(srv string, f func(network.ServiceScope) error) error {
	s := r.getServiceScope(srv) // 获取服务作用域
	defer s.DecRef()            // 在函数返回时减少引用计数

	return f(s) // 在服务作用域上执行函数
}

// ViewProtocol 在协议作用域上执行给定的函数
// 参数:
//   - proto: 协议ID
//   - f: 要在协议作用域上执行的函数
//
// 返回值:
//   - error: 执行过程中的错误,如果成功则为nil
func (r *resourceManager) ViewProtocol(proto protocol.ID, f func(network.ProtocolScope) error) error {
	s := r.getProtocolScope(proto) // 获取协议作用域
	defer s.DecRef()               // 在函数返回时减少引用计数

	return f(s) // 在协议作用域上执行函数
}

// ViewPeer 在对等点作用域上执行给定的函数
// 参数:
//   - p: 对等点ID
//   - f: 要在对等点作用域上执行的函数
//
// 返回值:
//   - error: 执行过程中的错误,如果成功则为nil
func (r *resourceManager) ViewPeer(p peer.ID, f func(network.PeerScope) error) error {
	s := r.getPeerScope(p) // 获取对等点作用域
	defer s.DecRef()       // 在函数返回时减少引用计数

	return f(s) // 在对等点作用域上执行函数
}

// getServiceScope 获取或创建服务作用域
// 参数:
//   - svc: 服务名称
//
// 返回值:
//   - *serviceScope: 服务作用域对象
func (r *resourceManager) getServiceScope(svc string) *serviceScope {
	r.mx.Lock()         // 加锁保护并发访问
	defer r.mx.Unlock() // 函数返回时解锁

	s, ok := r.svc[svc] // 尝试获取已存在的服务作用域
	if !ok {            // 如果不存在
		s = newServiceScope(svc, r.limits.GetServiceLimits(svc), r) // 创建新的服务作用域
		r.svc[svc] = s                                              // 将新创建的作用域存储到映射中
	}

	s.IncRef() // 增加引用计数
	return s   // 返回服务作用域
}

// getProtocolScope 获取或创建协议作用域
// 参数:
//   - proto: 协议ID
//
// 返回值:
//   - *protocolScope: 协议作用域对象
func (r *resourceManager) getProtocolScope(proto protocol.ID) *protocolScope {
	r.mx.Lock()         // 加锁保护并发访问
	defer r.mx.Unlock() // 函数返回时解锁

	s, ok := r.proto[proto] // 尝试获取已存在的协议作用域
	if !ok {                // 如果不存在
		s = newProtocolScope(proto, r.limits.GetProtocolLimits(proto), r) // 创建新的协议作用域
		r.proto[proto] = s                                                // 将新创建的作用域存储到映射中
	}

	s.IncRef() // 增加引用计数
	return s   // 返回协议作用域
}

// setStickyProtocol 设置粘性协议
// 参数:
//   - proto: 要设置为粘性的协议ID
func (r *resourceManager) setStickyProtocol(proto protocol.ID) {
	r.mx.Lock()         // 加锁保护并发访问
	defer r.mx.Unlock() // 函数返回时解锁

	if r.stickyProto == nil { // 如果粘性协议映射未初始化
		r.stickyProto = make(map[protocol.ID]struct{}) // 初始化映射
	}
	r.stickyProto[proto] = struct{}{} // 将协议标记为粘性
}

// getPeerScope 获取或创建对等点作用域
// 参数:
//   - p: 对等点ID
//
// 返回值:
//   - *peerScope: 对等点作用域对象
func (r *resourceManager) getPeerScope(p peer.ID) *peerScope {
	r.mx.Lock()         // 加锁保护并发访问
	defer r.mx.Unlock() // 函数返回时解锁

	s, ok := r.peer[p] // 尝试获取已存在的对等点作用域
	if !ok {           // 如果不存在
		s = newPeerScope(p, r.limits.GetPeerLimits(p), r) // 创建新的对等点作用域
		r.peer[p] = s                                     // 将新创建的作用域存储到映射中
	}

	s.IncRef() // 增加引用计数
	return s   // 返回对等点作用域
}

// setStickyPeer 设置粘性对等点
// 参数:
//   - p: 要设置为粘性的对等点ID
func (r *resourceManager) setStickyPeer(p peer.ID) {
	r.mx.Lock()         // 加锁保护并发访问
	defer r.mx.Unlock() // 函数返回时解锁

	if r.stickyPeer == nil { // 如果粘性对等点映射未初始化
		r.stickyPeer = make(map[peer.ID]struct{}) // 初始化映射
	}

	r.stickyPeer[p] = struct{}{} // 将对等点标记为粘性
}

// nextConnId 生成下一个连接ID
// 参数:
//   - 无
//
// 返回值:
//   - int64: 新生成的连接ID
func (r *resourceManager) nextConnId() int64 {
	r.mx.Lock()         // 加锁保护并发访问
	defer r.mx.Unlock() // 函数返回时解锁

	r.connId++      // 递增连接ID计数器
	return r.connId // 返回新的连接ID
}

// nextStreamId 生成下一个流ID
// 参数:
//   - 无
//
// 返回值:
//   - int64: 新生成的流ID
func (r *resourceManager) nextStreamId() int64 {
	r.mx.Lock()         // 加锁保护并发访问
	defer r.mx.Unlock() // 函数返回时解锁

	r.streamId++      // 递增流ID计数器
	return r.streamId // 返回新的流ID
}

// OpenConnectionNoIP 已弃用,将在下一个版本中移除
// 参数:
//   - dir: 连接方向
//   - usefd: 是否使用文件描述符
//   - endpoint: 多地址端点
//
// 返回值:
//   - network.ConnManagementScope: 连接管理作用域
//   - error: 错误信息
func (r *resourceManager) OpenConnectionNoIP(dir network.Direction, usefd bool, endpoint multiaddr.Multiaddr) (network.ConnManagementScope, error) {
	return r.openConnection(dir, usefd, endpoint, netip.Addr{}) // 调用openConnection方法,使用空IP地址
}

// OpenConnection 打开一个新的连接
// 参数:
//   - dir: network.Direction - 连接方向(入站/出站)
//   - usefd: bool - 是否使用文件描述符
//   - endpoint: multiaddr.Multiaddr - 连接端点地址
//
// 返回值:
//   - network.ConnManagementScope - 连接管理作用域
//   - error 错误信息
func (r *resourceManager) OpenConnection(dir network.Direction, usefd bool, endpoint multiaddr.Multiaddr) (network.ConnManagementScope, error) {
	ip, err := manet.ToIP(endpoint) // 从端点地址提取IP地址
	if err != nil {
		// 没有IP地址,使用空IP地址打开连接
		return r.openConnection(dir, usefd, endpoint, netip.Addr{})
	}

	ipAddr, ok := netip.AddrFromSlice(ip) // 将IP地址转换为netip.Addr类型
	if !ok {
		return nil, fmt.Errorf("无法将IP转换为netip.Addr类型")
	}
	return r.openConnection(dir, usefd, endpoint, ipAddr) // 使用转换后的IP地址打开连接
}

// openConnection 打开一个新的连接(内部方法)
// 参数:
//   - dir: network.Direction - 连接方向(入站/出站)
//   - usefd: bool - 是否使用文件描述符
//   - endpoint: multiaddr.Multiaddr - 连接端点地址
//   - ip: netip.Addr - IP地址
//
// 返回值:
//   - network.ConnManagementScope - 连接管理作用域
//   - error 错误信息
func (r *resourceManager) openConnection(dir network.Direction, usefd bool, endpoint multiaddr.Multiaddr, ip netip.Addr) (network.ConnManagementScope, error) {
	if ip.IsValid() { // 如果IP地址有效
		if ok := r.connLimiter.addConn(ip); !ok { // 尝试添加连接到限制器
			return nil, fmt.Errorf("IP %s 的连接数超出限制", endpoint)
		}
	}

	var conn *connectionScope
	conn = newConnectionScope(dir, usefd, r.limits.GetConnLimits(), r, endpoint, ip) // 创建新的连接作用域

	err := conn.AddConn(dir, usefd) // 添加连接
	if err != nil && ip.IsValid() { // 如果添加失败且IP有效
		// 检查是否为白名单连接并重试
		allowed := r.allowlist.Allowed(endpoint) // 检查端点是否在白名单中
		if allowed {
			conn.Done()                                                                             // 清理原连接作用域
			conn = newAllowListedConnectionScope(dir, usefd, r.limits.GetConnLimits(), r, endpoint) // 创建白名单连接作用域
			err = conn.AddConn(dir, usefd)                                                          // 重试添加连接
		}
	}

	if err != nil { // 如果添加连接失败
		conn.Done()                     // 清理连接作用域
		r.metrics.BlockConn(dir, usefd) // 记录阻塞连接指标
		return nil, err
	}

	r.metrics.AllowConn(dir, usefd) // 记录允许连接指标
	return conn, nil
}

// OpenStream 打开一个新的流
// 参数:
//   - p: peer.ID - 对等节点ID
//   - dir: network.Direction - 流方向(入站/出站)
//
// 返回值:
//   - network.StreamManagementScope - 流管理作用域
//   - error 错误信息
func (r *resourceManager) OpenStream(p peer.ID, dir network.Direction) (network.StreamManagementScope, error) {
	peer := r.getPeerScope(p)                                           // 获取对等节点作用域
	stream := newStreamScope(dir, r.limits.GetStreamLimits(p), peer, r) // 创建新的流作用域
	peer.DecRef()                                                       // 减少引用计数(edges中已有引用)

	err := stream.AddStream(dir) // 添加流
	if err != nil {              // 如果添加失败
		stream.Done()                 // 清理流作用域
		r.metrics.BlockStream(p, dir) // 记录阻塞流指标
		return nil, err
	}

	r.metrics.AllowStream(p, dir) // 记录允许流指标
	return stream, nil
}

// Close 关闭资源管理器
// 参数:
//   - 无
//
// 返回值:
//   - error 错误信息
func (r *resourceManager) Close() error {
	r.cancel()      // 取消上下文
	r.wg.Wait()     // 等待所有后台任务完成
	r.trace.Close() // 关闭跟踪器

	return nil
}

// background 运行资源管理器的后台任务
// 参数:
//   - 无
//
// 返回值:
//   - 无
func (r *resourceManager) background() {
	defer r.wg.Done() // 完成后减少等待组计数

	// 定期清理未使用的对等节点和协议作用域
	ticker := time.NewTicker(time.Minute) // 创建定时器
	defer ticker.Stop()                   // 函数返回时停止定时器

	for {
		select {
		case <-ticker.C: // 定时器触发
			r.gc() // 执行垃圾回收
		case <-r.cancelCtx.Done(): // 上下文取消
			return
		}
	}
}

// gc 执行垃圾回收,清理未使用的资源作用域
// 参数:
//   - 无
//
// 返回值:
//   - 无
func (r *resourceManager) gc() {
	r.mx.Lock()         // 加锁保护并发访问
	defer r.mx.Unlock() // 函数返回时解锁

	// 清理未使用的协议作用域
	for proto, s := range r.proto {
		_, sticky := r.stickyProto[proto] // 检查是否为固定协议
		if sticky {
			continue
		}
		if s.IsUnused() { // 如果作用域未使用
			s.Done()               // 清理作用域
			delete(r.proto, proto) // 从映射中删除
		}
	}

	var deadPeers []peer.ID // 存储要清理的对等节点ID
	// 清理未使用的对等节点作用域
	for p, s := range r.peer {
		_, sticky := r.stickyPeer[p] // 检查是否为固定对等节点
		if sticky {
			continue
		}

		if s.IsUnused() { // 如果作用域未使用
			s.Done()                         // 清理作用域
			delete(r.peer, p)                // 从映射中删除
			deadPeers = append(deadPeers, p) // 添加到待清理列表
		}
	}

	// 清理服务中的死亡对等节点
	for _, s := range r.svc {
		s.Lock() // 加锁保护服务作用域
		for _, p := range deadPeers {
			ps, ok := s.peers[p]
			if ok {
				ps.Done()          // 清理对等节点作用域
				delete(s.peers, p) // 从映射中删除
			}
		}
		s.Unlock() // 解锁服务作用域
	}

	// 清理协议中的死亡对等节点
	for _, s := range r.proto {
		s.Lock() // 加锁保护协议作用域
		for _, p := range deadPeers {
			ps, ok := s.peers[p]
			if ok {
				ps.Done()          // 清理对等节点作用域
				delete(s.peers, p) // 从映射中删除
			}
		}
		s.Unlock() // 解锁协议作用域
	}
}

// newSystemScope 创建新的系统作用域
// 参数:
//   - limit: Limit - 资源限制
//   - rcmgr: *resourceManager - 资源管理器
//   - name: string - 作用域名称
//
// 返回值:
//   - *systemScope - 系统作用域
func newSystemScope(limit Limit, rcmgr *resourceManager, name string) *systemScope {
	return &systemScope{
		resourceScope: newResourceScope(limit, nil, name, rcmgr.trace, rcmgr.metrics),
	}
}

// newTransientScope 创建新的临时作用域
// 参数:
//   - limit: Limit - 资源限制
//   - rcmgr: *resourceManager - 资源管理器
//   - name: string - 作用域名称
//   - systemScope: *resourceScope - 系统作用域
//
// 返回值:
//   - *transientScope - 临时作用域
func newTransientScope(limit Limit, rcmgr *resourceManager, name string, systemScope *resourceScope) *transientScope {
	return &transientScope{
		resourceScope: newResourceScope(limit,
			[]*resourceScope{systemScope},
			name, rcmgr.trace, rcmgr.metrics),
		system: rcmgr.system,
	}
}

// newServiceScope 创建新的服务作用域
// 参数:
//   - service: string - 服务名称
//   - limit: Limit - 资源限制
//   - rcmgr: *resourceManager - 资源管理器
//
// 返回值:
//   - *serviceScope - 服务作用域
func newServiceScope(service string, limit Limit, rcmgr *resourceManager) *serviceScope {
	return &serviceScope{
		resourceScope: newResourceScope(limit,
			[]*resourceScope{rcmgr.system.resourceScope}, // 设置系统作用域作为父作用域
			fmt.Sprintf("service:%s", service),           // 格式化作用域名称
			rcmgr.trace, rcmgr.metrics),                  // 设置跟踪和指标
		service: service, // 设置服务名称
		rcmgr:   rcmgr,   // 设置资源管理器引用
	}
}

// newProtocolScope 创建新的协议作用域
// 参数:
//   - proto: protocol.ID - 协议ID
//   - limit: Limit - 资源限制
//   - rcmgr: *resourceManager - 资源管理器
//
// 返回值:
//   - *protocolScope - 协议作用域
func newProtocolScope(proto protocol.ID, limit Limit, rcmgr *resourceManager) *protocolScope {
	return &protocolScope{
		resourceScope: newResourceScope(limit,
			[]*resourceScope{rcmgr.system.resourceScope}, // 设置系统作用域作为父作用域
			fmt.Sprintf("protocol:%s", proto),            // 格式化作用域名称
			rcmgr.trace, rcmgr.metrics),                  // 设置跟踪和指标
		proto: proto, // 设置协议ID
		rcmgr: rcmgr, // 设置资源管理器引用
	}
}

// newPeerScope 创建新的对等节点作用域
// 参数:
//   - p: peer.ID - 对等节点ID
//   - limit: Limit - 资源限制
//   - rcmgr: *resourceManager - 资源管理器
//
// 返回值:
//   - *peerScope - 对等节点作用域
func newPeerScope(p peer.ID, limit Limit, rcmgr *resourceManager) *peerScope {
	return &peerScope{
		resourceScope: newResourceScope(limit,
			[]*resourceScope{rcmgr.system.resourceScope}, // 设置系统作用域作为父作用域
			peerScopeName(p),            // 生成对等节点作用域名称
			rcmgr.trace, rcmgr.metrics), // 设置跟踪和指标
		peer:  p,     // 设置对等节点ID
		rcmgr: rcmgr, // 设置资源管理器引用
	}
}

// newConnectionScope 创建新的连接作用域
// 参数:
//   - dir: network.Direction - 连接方向
//   - usefd: bool - 是否使用文件描述符
//   - limit: Limit - 资源限制
//   - rcmgr: *resourceManager - 资源管理器
//   - endpoint: multiaddr.Multiaddr - 端点地址
//   - ip: netip.Addr - IP地址
//
// 返回值:
//   - *connectionScope - 连接作用域
func newConnectionScope(dir network.Direction, usefd bool, limit Limit, rcmgr *resourceManager, endpoint multiaddr.Multiaddr, ip netip.Addr) *connectionScope {
	return &connectionScope{
		resourceScope: newResourceScope(limit,
			[]*resourceScope{rcmgr.transient.resourceScope, rcmgr.system.resourceScope}, // 设置临时和系统作用域作为父作用域
			connScopeName(rcmgr.nextConnId()),                                           // 生成连接作用域名称
			rcmgr.trace, rcmgr.metrics),                                                 // 设置跟踪和指标
		dir:      dir,      // 设置连接方向
		usefd:    usefd,    // 设置是否使用文件描述符
		rcmgr:    rcmgr,    // 设置资源管理器引用
		endpoint: endpoint, // 设置端点地址
		ip:       ip,       // 设置IP地址
	}
}

// newAllowListedConnectionScope 创建新的白名单连接作用域
// 参数:
//   - dir: network.Direction - 连接方向
//   - usefd: bool - 是否使用文件描述符
//   - limit: Limit - 资源限制
//   - rcmgr: *resourceManager - 资源管理器
//   - endpoint: multiaddr.Multiaddr - 端点地址
//
// 返回值:
//   - *connectionScope - 连接作用域
func newAllowListedConnectionScope(dir network.Direction, usefd bool, limit Limit, rcmgr *resourceManager, endpoint multiaddr.Multiaddr) *connectionScope {
	return &connectionScope{
		resourceScope: newResourceScope(limit,
			[]*resourceScope{rcmgr.allowlistedTransient.resourceScope, rcmgr.allowlistedSystem.resourceScope}, // 设置白名单临时和系统作用域作为父作用域
			connScopeName(rcmgr.nextConnId()), // 生成连接作用域名称
			rcmgr.trace, rcmgr.metrics),       // 设置跟踪和指标
		dir:           dir,      // 设置连接方向
		usefd:         usefd,    // 设置是否使用文件描述符
		rcmgr:         rcmgr,    // 设置资源管理器引用
		endpoint:      endpoint, // 设置端点地址
		isAllowlisted: true,     // 标记为白名单连接
	}
}

// newStreamScope 创建新的流作用域
// 参数:
//   - dir: network.Direction - 流方向
//   - limit: Limit - 资源限制
//   - peer: *peerScope - 对等节点作用域
//   - rcmgr: *resourceManager - 资源管理器
//
// 返回值:
//   - *streamScope - 流作用域
func newStreamScope(dir network.Direction, limit Limit, peer *peerScope, rcmgr *resourceManager) *streamScope {
	return &streamScope{
		resourceScope: newResourceScope(limit,
			[]*resourceScope{peer.resourceScope, rcmgr.transient.resourceScope, rcmgr.system.resourceScope}, // 设置对等节点、临时和系统作用域作为父作用域
			streamScopeName(rcmgr.nextStreamId()), // 生成流作用域名称
			rcmgr.trace, rcmgr.metrics),           // 设置跟踪和指标
		dir:   dir,        // 设置流方向
		rcmgr: peer.rcmgr, // 设置资源管理器引用
		peer:  peer,       // 设置对等节点作用域
	}
}

// IsSystemScope 检查给定名称是否为系统作用域
// 参数:
//   - name: string - 作用域名称
//
// 返回值:
//   - bool - 是否为系统作用域
func IsSystemScope(name string) bool {
	return name == "system"
}

// IsTransientScope 检查给定名称是否为临时作用域
// 参数:
//   - name: string - 作用域名称
//
// 返回值:
//   - bool - 是否为临时作用域
func IsTransientScope(name string) bool {
	return name == "transient"
}

// streamScopeName 生成流作用域名称
// 参数:
//   - streamId: int64 - 流ID
//
// 返回值:
//   - string - 流作用域名称
func streamScopeName(streamId int64) string {
	return fmt.Sprintf("stream-%d", streamId)
}

// IsStreamScope 检查给定名称是否为流作用域
// 参数:
//   - name: string - 作用域名称
//
// 返回值:
//   - bool - 是否为流作用域
func IsStreamScope(name string) bool {
	return strings.HasPrefix(name, "stream-") && !IsSpan(name)
}

// connScopeName 生成连接作用域名称
// 参数:
//   - streamId: int64 - 流ID
//
// 返回值:
//   - string - 连接作用域名称
func connScopeName(streamId int64) string {
	return fmt.Sprintf("conn-%d", streamId)
}

// IsConnScope 检查给定名称是否为连接作用域
// 参数:
//   - name: string - 作用域名称
//
// 返回值:
//   - bool - 是否为连接作用域
func IsConnScope(name string) bool {
	return strings.HasPrefix(name, "conn-") && !IsSpan(name)
}

// peerScopeName 生成对等节点作用域名称
// 参数:
//   - p: peer.ID - 对等节点ID
//
// 返回值:
//   - string - 对等节点作用域名称
func peerScopeName(p peer.ID) string {
	return fmt.Sprintf("peer:%s", p)
}

// PeerStrInScopeName 从作用域名称中提取对等节点ID字符串
// 参数:
//   - name: string - 作用域名称
//
// 返回值:
//   - string - 对等节点ID字符串，如果不是对等节点作用域则返回空字符串
func PeerStrInScopeName(name string) string {
	if !strings.HasPrefix(name, "peer:") || IsSpan(name) {
		return ""
	}
	// 使用Index避免分配新字符串
	peerSplitIdx := strings.Index(name, "peer:")
	if peerSplitIdx == -1 {
		return ""
	}
	p := (name[peerSplitIdx+len("peer:"):])
	return p
}

// ParseProtocolScopeName 从作用域名称中解析协议名称
// 参数:
//   - name: string - 作用域名称
//
// 返回值:
//   - string - 协议名称，如果不是协议作用域则返回空字符串
func ParseProtocolScopeName(name string) string {
	if strings.HasPrefix(name, "protocol:") && !IsSpan(name) {
		if strings.Contains(name, "peer:") {
			// 这是一个协议对等节点作用域
			return ""
		}

		// 使用Index避免分配新字符串
		separatorIdx := strings.Index(name, ":")
		if separatorIdx == -1 {
			return ""
		}
		return name[separatorIdx+1:]
	}
	return ""
}

// Name 获取服务作用域名称
// 返回值:
//   - string - 服务名称
func (s *serviceScope) Name() string {
	return s.service
}

// getPeerScope 获取服务中的对等节点作用域
// 参数:
//   - p: peer.ID - 对等节点ID
//
// 返回值:
//   - *resourceScope - 对等节点资源作用域
func (s *serviceScope) getPeerScope(p peer.ID) *resourceScope {
	s.Lock() // 加锁保护并发访问
	defer s.Unlock()

	ps, ok := s.peers[p] // 查找现有的对等节点作用域
	if ok {
		ps.IncRef() // 增加引用计数
		return ps
	}

	l := s.rcmgr.limits.GetServicePeerLimits(s.service) // 获取服务对等节点限制

	if s.peers == nil {
		s.peers = make(map[peer.ID]*resourceScope) // 初始化对等节点映射
	}

	ps = newResourceScope(l, nil, fmt.Sprintf("%s.peer:%s", s.name, p), s.rcmgr.trace, s.rcmgr.metrics) // 创建新的资源作用域
	s.peers[p] = ps                                                                                     // 保存到映射中

	ps.IncRef() // 增加引用计数
	return ps
}

// Protocol 获取协议作用域的协议ID
// 返回值:
//   - protocol.ID - 协议ID
func (s *protocolScope) Protocol() protocol.ID {
	return s.proto
}

// getPeerScope 获取协议中的对等节点作用域
// 参数:
//   - p: peer.ID - 对等节点ID
//
// 返回值:
//   - *resourceScope - 对等节点资源作用域
func (s *protocolScope) getPeerScope(p peer.ID) *resourceScope {
	s.Lock() // 加锁保护并发访问
	defer s.Unlock()

	ps, ok := s.peers[p] // 查找现有的对等节点作用域
	if ok {
		ps.IncRef() // 增加引用计数
		return ps
	}

	l := s.rcmgr.limits.GetProtocolPeerLimits(s.proto) // 获取协议对等节点限制

	if s.peers == nil {
		s.peers = make(map[peer.ID]*resourceScope) // 初始化对等节点映射
	}

	ps = newResourceScope(l, nil, fmt.Sprintf("%s.peer:%s", s.name, p), s.rcmgr.trace, s.rcmgr.metrics) // 创建新的资源作用域
	s.peers[p] = ps                                                                                     // 保存到映射中

	ps.IncRef() // 增加引用计数
	return ps
}

// Peer 获取对等节点作用域的节点ID
// 返回值:
//   - peer.ID - 对等节点ID
func (s *peerScope) Peer() peer.ID {
	return s.peer
}

// PeerScope 获取连接作用域关联的对等节点作用域
// 返回值:
//   - network.PeerScope - 对等节点作用域
func (s *connectionScope) PeerScope() network.PeerScope {
	s.Lock() // 加锁保护并发访问
	defer s.Unlock()

	// 避免 nil 不等于 nil 的 Go 语言陷阱
	if s.peer == nil {
		return nil
	}

	return s.peer
}

// Done 完成连接作用域的使用
func (s *connectionScope) Done() {
	s.Lock() // 加锁保护并发访问
	defer s.Unlock()
	if s.done { // 如果已经完成则直接返回
		return
	}
	if s.ip.IsValid() { // 如果IP有效则移除连接
		s.rcmgr.connLimiter.rmConn(s.ip)
	}
	s.resourceScope.doneUnlocked() // 释放资源作用域
}

// transferAllowedToStandard 将连接作用域从白名单集合转移到标准集合
// 当我们最初因为IP将连接加入白名单，但后来发现对等节点ID不是预期的时候会发生这种情况
// 返回值:
//   - error 转移过程中的错误
func (s *connectionScope) transferAllowedToStandard() (err error) {
	systemScope := s.rcmgr.system.resourceScope       // 获取系统资源作用域
	transientScope := s.rcmgr.transient.resourceScope // 获取临时资源作用域

	stat := s.resourceScope.rc.stat() // 获取当前资源状态

	for _, scope := range s.edges { // 释放所有边缘作用域
		scope.ReleaseForChild(stat)
		scope.DecRef() // 从边缘移除
	}
	s.edges = nil

	if err := systemScope.ReserveForChild(stat); err != nil { // 为系统作用域预留资源
		return err
	}
	systemScope.IncRef()

	// 如果后续失败则撤销操作
	defer func() {
		if err != nil {
			systemScope.ReleaseForChild(stat)
			systemScope.DecRef()
		}
	}()

	if err := transientScope.ReserveForChild(stat); err != nil { // 为临时作用域预留资源
		return err
	}
	transientScope.IncRef()

	// 更新边缘作用域
	s.edges = []*resourceScope{
		systemScope,
		transientScope,
	}
	return nil
}

// SetPeer 设置连接作用域关联的对等节点
// 参数:
//   - p: peer.ID - 对等节点ID
//
// 返回值:
//   - error 设置过程中的错误
func (s *connectionScope) SetPeer(p peer.ID) error {
	s.Lock() // 加锁保护并发访问
	defer s.Unlock()

	if s.peer != nil {
		return fmt.Errorf("连接作用域已经关联到一个对等节点")
	}

	system := s.rcmgr.system       // 获取系统作用域
	transient := s.rcmgr.transient // 获取临时作用域

	if s.isAllowlisted { // 如果在白名单中
		system = s.rcmgr.allowlistedSystem       // 使用白名单系统作用域
		transient = s.rcmgr.allowlistedTransient // 使用白名单临时作用域

		if !s.rcmgr.allowlist.AllowedPeerAndMultiaddr(p, s.endpoint) { // 检查对等节点和多地址组合是否在白名单中
			s.isAllowlisted = false

			// 这不是允许的对等节点和多地址组合
			// 需要将此连接转移到一般作用域
			// 首先将连接转移到系统和临时作用域，然后继续执行此函数
			// 这样做是为了避免连接因为几乎是白名单连接而逃避临时作用域的限制
			if err := s.transferAllowedToStandard(); err != nil {
				return err // 转移到标准作用域失败
			}

			// 设置系统和临时作用域为非白名单作用域
			system = s.rcmgr.system
			transient = s.rcmgr.transient
		}
	}

	s.peer = s.rcmgr.getPeerScope(p) // 获取对等节点作用域

	// 将资源从临时作用域转移到对等节点作用域
	stat := s.resourceScope.rc.stat()
	if err := s.peer.ReserveForChild(stat); err != nil {
		s.peer.DecRef()
		s.peer = nil
		s.rcmgr.metrics.BlockPeer(p)
		return err
	}

	transient.ReleaseForChild(stat)
	transient.DecRef() // 从边缘移除

	// 更新边缘作用域
	edges := []*resourceScope{
		s.peer.resourceScope,
		system.resourceScope,
	}
	s.resourceScope.edges = edges

	s.rcmgr.metrics.AllowPeer(p) // 记录允许对等节点的指标
	return nil
}

// ProtocolScope 获取流作用域关联的协议作用域
// 返回值:
//   - network.ProtocolScope - 协议作用域
func (s *streamScope) ProtocolScope() network.ProtocolScope {
	s.Lock() // 加锁保护并发访问
	defer s.Unlock()

	// 避免 nil 不等于 nil 的 Go 语言陷阱
	if s.proto == nil {
		return nil
	}

	return s.proto
}

// SetProtocol 设置流作用域关联的协议
// 参数:
//   - proto: protocol.ID - 协议ID
//
// 返回值:
//   - error 设置过程中的错误
func (s *streamScope) SetProtocol(proto protocol.ID) error {
	s.Lock() // 加锁保护并发访问
	defer s.Unlock()

	if s.proto != nil {
		return fmt.Errorf("流作用域已经关联到一个协议")
	}

	s.proto = s.rcmgr.getProtocolScope(proto) // 获取协议作用域

	// 将资源从临时作用域转移到协议作用域
	stat := s.resourceScope.rc.stat()
	if err := s.proto.ReserveForChild(stat); err != nil {
		s.proto.DecRef()
		s.proto = nil
		s.rcmgr.metrics.BlockProtocol(proto)
		return err
	}

	s.peerProtoScope = s.proto.getPeerScope(s.peer.peer) // 获取对等节点的协议作用域
	if err := s.peerProtoScope.ReserveForChild(stat); err != nil {
		s.proto.ReleaseForChild(stat)
		s.proto.DecRef()
		s.proto = nil
		s.peerProtoScope.DecRef()
		s.peerProtoScope = nil
		s.rcmgr.metrics.BlockProtocolPeer(proto, s.peer.peer)
		return err
	}

	s.rcmgr.transient.ReleaseForChild(stat)
	s.rcmgr.transient.DecRef() // 从边缘移除

	// 更新边缘作用域
	edges := []*resourceScope{
		s.peer.resourceScope,
		s.peerProtoScope,
		s.proto.resourceScope,
		s.rcmgr.system.resourceScope,
	}
	s.resourceScope.edges = edges

	s.rcmgr.metrics.AllowProtocol(proto) // 记录允许协议的指标
	return nil
}

// ServiceScope 获取流作用域关联的服务作用域
// 返回值:
//   - network.ServiceScope - 服务作用域
func (s *streamScope) ServiceScope() network.ServiceScope {
	s.Lock() // 加锁保护并发访问
	defer s.Unlock()

	// 避免 nil 不等于 nil 的 Go 语言陷阱
	if s.svc == nil {
		return nil
	}

	return s.svc
}

// SetService 设置流作用域关联的服务
// 参数:
//   - svc: string - 服务名称
//
// 返回值:
//   - error 设置过程中的错误
func (s *streamScope) SetService(svc string) error {
	s.Lock() // 加锁保护并发访问
	defer s.Unlock()

	if s.svc != nil {
		return fmt.Errorf("流作用域已经关联到一个服务")
	}
	if s.proto == nil {
		return fmt.Errorf("流作用域未关联到协议")
	}

	s.svc = s.rcmgr.getServiceScope(svc) // 获取服务作用域

	// 在服务中预留资源
	stat := s.resourceScope.rc.stat()
	if err := s.svc.ReserveForChild(stat); err != nil {
		s.svc.DecRef()
		s.svc = nil
		s.rcmgr.metrics.BlockService(svc)
		return err
	}

	// 获取对等节点的服务作用域约束(如果有的话)
	s.peerSvcScope = s.svc.getPeerScope(s.peer.peer)
	if err := s.peerSvcScope.ReserveForChild(stat); err != nil {
		s.svc.ReleaseForChild(stat)
		s.svc.DecRef()
		s.svc = nil
		s.peerSvcScope.DecRef()
		s.peerSvcScope = nil
		s.rcmgr.metrics.BlockServicePeer(svc, s.peer.peer)
		return err
	}

	// 更新边缘作用域
	edges := []*resourceScope{
		s.peer.resourceScope,
		s.peerProtoScope,
		s.peerSvcScope,
		s.proto.resourceScope,
		s.svc.resourceScope,
		s.rcmgr.system.resourceScope,
	}
	s.resourceScope.edges = edges

	s.rcmgr.metrics.AllowService(svc) // 记录允许服务的指标
	return nil
}

// PeerScope 获取流作用域关联的对等节点作用域
// 返回值:
//   - network.PeerScope - 对等节点作用域
func (s *streamScope) PeerScope() network.PeerScope {
	s.Lock() // 加锁保护并发访问
	defer s.Unlock()

	// 避免 nil 不等于 nil 的 Go 语言陷阱
	if s.peer == nil {
		return nil
	}

	return s.peer
}
