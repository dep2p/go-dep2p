package mocks

import (
	"context"
	"sync"
	"time"

	"github.com/dep2p/go-dep2p/core/discovery"
	"github.com/dep2p/go-dep2p/core/host"
	"github.com/dep2p/go-dep2p/core/peer"
	logging "github.com/dep2p/log"
)

var log = logging.Logger("discovery-mocks")

// clock 定义了时钟接口
type clock interface {
	Now() time.Time // 获取当前时间
}

// MockDiscoveryServer 实现了一个模拟的发现服务器
type MockDiscoveryServer struct {
	mx    sync.Mutex                                    // 互斥锁
	db    map[string]map[peer.ID]*discoveryRegistration // 存储注册信息的数据库
	clock clock                                         // 时钟接口
}

// discoveryRegistration 保存节点注册信息
type discoveryRegistration struct {
	info       peer.AddrInfo // 节点地址信息
	expiration time.Time     // 过期时间
}

// NewDiscoveryServer 创建一个新的模拟发现服务器
// 参数:
//   - clock: 时钟接口实例
//
// 返回值:
//   - *MockDiscoveryServer: 返回创建的模拟发现服务器实例
func NewDiscoveryServer(clock clock) *MockDiscoveryServer {
	return &MockDiscoveryServer{
		db:    make(map[string]map[peer.ID]*discoveryRegistration),
		clock: clock,
	}
}

// Advertise 在指定命名空间注册节点信息
// 参数:
//   - ns: 命名空间
//   - info: 节点地址信息
//   - ttl: 注册信息的有效期
//
// 返回值:
//   - time.Duration: 返回注册的有效期
//   - error: 如果发生错误则返回错误信息
func (s *MockDiscoveryServer) Advertise(ns string, info peer.AddrInfo, ttl time.Duration) (time.Duration, error) {
	s.mx.Lock()
	defer s.mx.Unlock()

	// 获取或创建命名空间对应的节点映射
	peers, ok := s.db[ns]
	if !ok {
		peers = make(map[peer.ID]*discoveryRegistration)
		s.db[ns] = peers
	}
	// 注册节点信息
	peers[info.ID] = &discoveryRegistration{info, s.clock.Now().Add(ttl)}
	return ttl, nil
}

// FindPeers 在指定命名空间查找节点
// 参数:
//   - ns: 命名空间
//   - limit: 返回结果数量限制
//
// 返回值:
//   - <-chan peer.AddrInfo: 返回节点信息通道
//   - error: 如果发生错误则返回错误信息
func (s *MockDiscoveryServer) FindPeers(ns string, limit int) (<-chan peer.AddrInfo, error) {
	s.mx.Lock()
	defer s.mx.Unlock()

	// 获取命名空间中的节点
	peers, ok := s.db[ns]
	if !ok || len(peers) == 0 {
		emptyCh := make(chan peer.AddrInfo)
		close(emptyCh)
		return emptyCh, nil
	}

	// 计算返回结果数量
	count := len(peers)
	if limit != 0 && count > limit {
		count = limit
	}

	iterTime := s.clock.Now()
	ch := make(chan peer.AddrInfo, count)
	numSent := 0
	// 遍历并发送未过期的节点信息
	for p, reg := range peers {
		if numSent == count {
			break
		}
		if iterTime.After(reg.expiration) {
			delete(peers, p)
			continue
		}

		numSent++
		ch <- reg.info
	}
	close(ch)

	return ch, nil
}

// MockDiscoveryClient 实现了一个模拟的发现客户端
type MockDiscoveryClient struct {
	host   host.Host            // dep2p主机实例
	server *MockDiscoveryServer // 模拟发现服务器
}

// NewDiscoveryClient 创建一个新的模拟发现客户端
// 参数:
//   - h: dep2p主机实例
//   - server: 模拟发现服务器实例
//
// 返回值:
//   - *MockDiscoveryClient: 返回创建的模拟发现客户端实例
func NewDiscoveryClient(h host.Host, server *MockDiscoveryServer) *MockDiscoveryClient {
	return &MockDiscoveryClient{
		host:   h,
		server: server,
	}
}

// Advertise 在指定命名空间注册本节点信息
// 参数:
//   - ctx: 上下文
//   - ns: 命名空间
//   - opts: 可选的发现选项
//
// 返回值:
//   - time.Duration: 返回注册的有效期
//   - error: 如果发生错误则返回错误信息
func (d *MockDiscoveryClient) Advertise(ctx context.Context, ns string, opts ...discovery.Option) (time.Duration, error) {
	var options discovery.Options
	err := options.Apply(opts...)
	if err != nil {
		log.Debugf("应用配置选项失败: %v", err)
		return 0, err
	}

	return d.server.Advertise(ns, *host.InfoFromHost(d.host), options.Ttl)
}

// FindPeers 在指定命名空间查找节点
// 参数:
//   - ctx: 上下文
//   - ns: 命名空间
//   - opts: 可选的发现选项
//
// 返回值:
//   - <-chan peer.AddrInfo: 返回节点信息通道
//   - error: 如果发生错误则返回错误信息
func (d *MockDiscoveryClient) FindPeers(ctx context.Context, ns string, opts ...discovery.Option) (<-chan peer.AddrInfo, error) {
	var options discovery.Options
	err := options.Apply(opts...)
	if err != nil {
		log.Debugf("应用配置选项失败: %v", err)
		return nil, err
	}

	return d.server.FindPeers(ns, options.Limit)
}
