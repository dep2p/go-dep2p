package resourcemgr

import (
	"sync"
	"sync/atomic"

	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/types"
	"github.com/dep2p/go-dep2p/pkg/lib/log"
)

var logger = log.Logger("core/resourcemgr")

// resourceManager 资源管理器实现
type resourceManager struct {
	limits *pkgif.LimitConfig // 资源限制配置

	system    *systemScope    // 系统级作用域
	transient *transientScope // 临时资源作用域

	mu    sync.Mutex                             // 保护以下 map
	svc   map[string]*serviceScope               // 服务作用域
	proto map[types.ProtocolID]*protocolScope    // 协议作用域
	peer  map[types.PeerID]*peerScope            // 节点作用域

	connId   atomic.Int64 // 连接 ID 计数器
	streamId atomic.Int64 // 流 ID 计数器

	closed atomic.Bool // 关闭状态
}

// NewResourceManager 创建资源管理器
func NewResourceManager(limits *pkgif.LimitConfig) (pkgif.ResourceManager, error) {
	if limits == nil {
		limits = DefaultLimitConfig()
	}

	rm := &resourceManager{
		limits: limits,
		svc:    make(map[string]*serviceScope),
		proto:  make(map[types.ProtocolID]*protocolScope),
		peer:   make(map[types.PeerID]*peerScope),
	}

	// 创建系统作用域
	rm.system = &systemScope{
		resourceScope: newResourceScope(&limits.System),
	}
	rm.system.IncRef()

	// 创建临时作用域
	rm.transient = &transientScope{
		resourceScope: newResourceScope(&limits.Transient),
		system:        rm.system,
	}
	rm.transient.IncRef()

	return rm, nil
}

// ViewSystem 查看系统级资源作用域
func (rm *resourceManager) ViewSystem(f func(pkgif.ResourceScope) error) error {
	return f(rm.system)
}

// ViewTransient 查看临时资源作用域
func (rm *resourceManager) ViewTransient(f func(pkgif.ResourceScope) error) error {
	return f(rm.transient)
}

// ViewService 查看服务级资源作用域
func (rm *resourceManager) ViewService(service string, f func(pkgif.ServiceScope) error) error {
	s := rm.getServiceScope(service)
	defer s.DecRef()

	return f(s)
}

// ViewProtocol 查看协议级资源作用域
func (rm *resourceManager) ViewProtocol(proto types.ProtocolID, f func(pkgif.ProtocolScope) error) error {
	s := rm.getProtocolScope(proto)
	defer s.DecRef()

	return f(s)
}

// ViewPeer 查看节点级资源作用域
func (rm *resourceManager) ViewPeer(peer types.PeerID, f func(pkgif.PeerScope) error) error {
	s := rm.getPeerScope(peer)
	defer s.DecRef()

	return f(s)
}

// OpenConnection 打开连接作用域
func (rm *resourceManager) OpenConnection(dir pkgif.Direction, usefd bool, endpoint types.Multiaddr) (pkgif.ConnManagementScope, error) {
	if rm.closed.Load() {
		logger.Warn("资源管理器已关闭，无法打开连接")
		return nil, ErrResourceScopeClosed
	}
	
	logger.Debug("打开连接作用域", "direction", dir, "usefd", usefd)

	// 确定连接数和 FD 数
	nconns, nfd := 1, 0
	var nconnsIn, nconnsOut int
	switch dir {
	case pkgif.DirInbound:
		nconnsIn = 1
	case pkgif.DirOutbound:
		nconnsOut = 1
	}

	if usefd {
		nfd = 1
	}

	// 在系统和临时作用域中预留资源
	if err := rm.system.reserveConns(nconns, nconnsIn, nconnsOut, nfd); err != nil {
		logger.Warn("系统作用域资源预留失败", "error", err)
		return nil, err
	}

	if err := rm.transient.reserveConns(nconns, nconnsIn, nconnsOut, nfd); err != nil {
		logger.Warn("临时作用域资源预留失败", "error", err)
		rm.system.releaseConns(nconns, nconnsIn, nconnsOut, nfd)
		return nil, err
	}

	// 创建连接作用域
	connScope := &connectionScope{
		resourceScope: newResourceScope(&rm.limits.Conn),
		dir:           dir,
		usefd:         usefd,
		endpoint:      endpoint,
		rcmgr:         rm,
	}

	// 预留自己的资源
	if err := connScope.reserveConns(nconns, nconnsIn, nconnsOut, nfd); err != nil {
		rm.transient.releaseConns(nconns, nconnsIn, nconnsOut, nfd)
		rm.system.releaseConns(nconns, nconnsIn, nconnsOut, nfd)
		return nil, err
	}

	rm.connId.Add(1)

	return connScope, nil
}

// OpenStream 打开流作用域
func (rm *resourceManager) OpenStream(peer types.PeerID, dir pkgif.Direction) (pkgif.StreamManagementScope, error) {
	if rm.closed.Load() {
		logger.Warn("资源管理器已关闭，无法打开流")
		return nil, ErrResourceScopeClosed
	}
	
	logger.Debug("打开流作用域", "peerID", log.TruncateID(string(peer), 8), "direction", dir)

	// 确定流数
	nstreams := 1
	var nstreamsIn, nstreamsOut int
	switch dir {
	case pkgif.DirInbound:
		nstreamsIn = 1
	case pkgif.DirOutbound:
		nstreamsOut = 1
	}

	// 在系统和临时作用域中预留资源
	if err := rm.system.reserveStreams(nstreams, nstreamsIn, nstreamsOut); err != nil {
		logger.Warn("系统作用域流资源预留失败", "error", err)
		return nil, err
	}

	if err := rm.transient.reserveStreams(nstreams, nstreamsIn, nstreamsOut); err != nil {
		logger.Warn("临时作用域流资源预留失败", "error", err)
		rm.system.releaseStreams(nstreams, nstreamsIn, nstreamsOut)
		return nil, err
	}

	// 获取节点作用域
	peerScope := rm.getPeerScope(peer)

	// 在节点作用域中预留资源
	if err := peerScope.reserveStreams(nstreams, nstreamsIn, nstreamsOut); err != nil {
		logger.Warn("节点作用域流资源预留失败", "peerID", log.TruncateID(string(peer), 8), "error", err)
		rm.transient.releaseStreams(nstreams, nstreamsIn, nstreamsOut)
		rm.system.releaseStreams(nstreams, nstreamsIn, nstreamsOut)
		peerScope.DecRef()
		return nil, err
	}

	// 创建流作用域
	streamScope := &streamScope{
		resourceScope: newResourceScope(&rm.limits.Stream),
		dir:           dir,
		peer:          peerScope,
		rcmgr:         rm,
	}

	// 预留自己的资源
	if err := streamScope.reserveStreams(nstreams, nstreamsIn, nstreamsOut); err != nil {
		logger.Warn("流作用域资源预留失败", "error", err)
		peerScope.releaseStreams(nstreams, nstreamsIn, nstreamsOut)
		rm.transient.releaseStreams(nstreams, nstreamsIn, nstreamsOut)
		rm.system.releaseStreams(nstreams, nstreamsIn, nstreamsOut)
		peerScope.DecRef()
		return nil, err
	}

	rm.streamId.Add(1)
	logger.Debug("流作用域打开成功", "streamID", rm.streamId.Load(), "peerID", log.TruncateID(string(peer), 8))

	return streamScope, nil
}

// Close 关闭资源管理器
func (rm *resourceManager) Close() error {
	rm.closed.Store(true)
	return nil
}

// getServiceScope 获取服务作用域（不存在则创建）
func (rm *resourceManager) getServiceScope(service string) *serviceScope {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	s, ok := rm.svc[service]
	if !ok {
		s = &serviceScope{
			resourceScope: newResourceScope(&rm.limits.ServiceDefault),
			service:       service,
			rcmgr:         rm,
		}
		rm.svc[service] = s
	}

	s.IncRef()
	return s
}

// getProtocolScope 获取协议作用域（不存在则创建）
func (rm *resourceManager) getProtocolScope(proto types.ProtocolID) *protocolScope {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	s, ok := rm.proto[proto]
	if !ok {
		s = &protocolScope{
			resourceScope: newResourceScope(&rm.limits.ProtocolDefault),
			proto:         proto,
			rcmgr:         rm,
		}
		rm.proto[proto] = s
	}

	s.IncRef()
	return s
}

// getPeerScope 获取节点作用域（不存在则创建）
func (rm *resourceManager) getPeerScope(peer types.PeerID) *peerScope {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	s, ok := rm.peer[peer]
	if !ok {
		s = &peerScope{
			resourceScope: newResourceScope(&rm.limits.PeerDefault),
			peer:          peer,
			rcmgr:         rm,
		}
		rm.peer[peer] = s
	}

	s.IncRef()
	return s
}

// ============================================================================
// systemScope - 系统级作用域
// ============================================================================

type systemScope struct {
	*resourceScope
}

// ============================================================================
// transientScope - 临时资源作用域
// ============================================================================

type transientScope struct {
	*resourceScope
	system *systemScope
}

// ============================================================================
// serviceScope - 服务级作用域
// ============================================================================

type serviceScope struct {
	*resourceScope
	service string
	rcmgr   *resourceManager
}

// Name 返回服务名称
func (s *serviceScope) Name() string {
	return s.service
}

// ============================================================================
// protocolScope - 协议级作用域
// ============================================================================

type protocolScope struct {
	*resourceScope
	proto types.ProtocolID
	rcmgr *resourceManager
}

// Protocol 返回协议 ID
func (s *protocolScope) Protocol() types.ProtocolID {
	return s.proto
}

// ============================================================================
// peerScope - 节点级作用域
// ============================================================================

type peerScope struct {
	*resourceScope
	peer  types.PeerID
	rcmgr *resourceManager
}

// Peer 返回节点 ID
func (s *peerScope) Peer() types.PeerID {
	return s.peer
}
