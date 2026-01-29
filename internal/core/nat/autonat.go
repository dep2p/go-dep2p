package nat

import (
	"context"
	"math/rand"
	"sync"
	"time"

	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/lib/proto/autonat"
	"github.com/dep2p/go-dep2p/pkg/protocol"
	"github.com/dep2p/go-dep2p/pkg/types"
	"google.golang.org/protobuf/proto"
)

// AutoNAT NAT 自动检测器
type AutoNAT struct {
	service *Service
	config  *Config
	host    pkgif.Host
	swarm   pkgif.Swarm

	mu            sync.RWMutex
	currentStatus Reachability
	confidence    int
	recentProbes  map[string]time.Time
	successCount  int
	failureCount  int

	// 用于测试的钩子函数
	probeFunc func() error
}

// newAutoNAT 创建 AutoNAT 检测器
func newAutoNAT(config *Config) *AutoNAT {
	return &AutoNAT{
		config:        config,
		currentStatus: ReachabilityUnknown,
		recentProbes:  make(map[string]time.Time),
	}
}

// recordSuccess 记录成功探测
func (a *AutoNAT) recordSuccess() {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.successCount++
	a.failureCount = 0 // 重置失败计数

	// 更新置信度
	if a.currentStatus != ReachabilityPublic {
		a.confidence++
		if a.confidence >= a.config.ConfidenceThreshold {
			a.currentStatus = ReachabilityPublic
			a.confidence = a.config.ConfidenceThreshold

			// 更新服务状态
			if a.service != nil {
				a.service.SetReachability(ReachabilityPublic)
			}
		}
	}
}

// recordFailure 记录失败探测
func (a *AutoNAT) recordFailure() {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.failureCount++
	a.successCount = 0 // 重置成功计数

	// 更新置信度
	if a.currentStatus != ReachabilityPrivate {
		a.confidence++
		if a.confidence >= a.config.ConfidenceThreshold {
			a.currentStatus = ReachabilityPrivate
			a.confidence = a.config.ConfidenceThreshold

			// 更新服务状态
			if a.service != nil {
				a.service.SetReachability(ReachabilityPrivate)
			}
		}
	}
}

// runProbeLoop 运行探测循环
func (a *AutoNAT) runProbeLoop(ctx context.Context) {
	ticker := time.NewTicker(a.config.ProbeInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			a.probe(ctx)
		}
	}
}

// probe 执行一次探测
//
// 完整实现 (TD-002)：
//  1. 获取支持 AutoNAT 的节点
//  2. 选择一个随机节点
//  3. 发送 Dial 请求
//  4. 等待拨回结果
//  5. 根据结果更新可达性状态
func (a *AutoNAT) probe(ctx context.Context) {
	// 使用测试钩子
	if a.probeFunc != nil {
		err := a.probeFunc()
		if err == nil {
			a.recordSuccess()
		} else {
			a.recordFailure()
		}
		return
	}

	// 1. 获取支持 AutoNAT 的节点
	peers := a.getAutoNATServers()
	if len(peers) == 0 {
		// 没有可用的 AutoNAT 服务节点
		return
	}

	// 2. 选择一个随机节点（非加密场景，math/rand 足够）
	peer := peers[rand.Intn(len(peers))] //nolint:gosec // G404: 选择随机 peer 不需要加密级随机

	// 3. 打开 AutoNAT 流
	stream, err := a.host.NewStream(ctx, peer, AutoNATProtocol)
	if err != nil {
		a.recordFailure()
		return
	}
	defer stream.Close()

	// 4. 发送 Dial 请求
	localAddrs := a.host.Addrs()
	addrsBytes := make([][]byte, len(localAddrs))
	for i, addr := range localAddrs {
		addrsBytes[i] = []byte(addr)
	}

	req := &autonat.Message{
		Type: autonat.MessageType_DIAL,
		Dial: &autonat.Dial{
			Peer: &autonat.PeerInfo{
				Id:    []byte(a.host.ID()),
				Addrs: addrsBytes,
			},
		},
	}

	if err := a.sendRequest(stream, req); err != nil {
		a.recordFailure()
		return
	}

	// 5. 读取响应
	response, err := a.readResponse(stream)
	if err != nil {
		a.recordFailure()
		return
	}

	// 6. 根据结果更新状态
	if response.DialResponse != nil {
		if response.DialResponse.Status == autonat.ResponseStatus_OK {
			a.recordSuccess()
		} else {
			a.recordFailure()
		}
	}
}

// getAutoNATServers 获取支持 AutoNAT 的节点
//
// Phase 11 修复：从 peerstore 中查找支持 AutoNAT 协议的节点
func (a *AutoNAT) getAutoNATServers() []string {
	if a.swarm == nil {
		return nil
	}

	// 获取所有已连接的节点
	peers := a.swarm.Peers()
	if len(peers) == 0 {
		return nil
	}

	// 尝试获取 peerstore 来检查协议支持
	var peerstore pkgif.Peerstore
	if a.host != nil {
		peerstore = a.host.Peerstore()
	}

	// 过滤出支持 AutoNAT 协议的节点
	var autoNATServers []string
	autoNATProtocol := protocol.AutoNAT

	for _, peerID := range peers {
		// 优先使用 peerstore 检查协议支持
		if peerstore != nil {
			supported, err := peerstore.SupportsProtocols(types.PeerID(peerID), types.ProtocolID(autoNATProtocol))
			if err == nil && len(supported) > 0 {
				autoNATServers = append(autoNATServers, peerID)
				continue
			}
		}
		// 如果没有 peerstore 信息，检查最近是否成功探测过该节点
		a.mu.RLock()
		if _, ok := a.recentProbes[peerID]; ok {
			autoNATServers = append(autoNATServers, peerID)
		}
		a.mu.RUnlock()
	}

	// 如果没有找到支持 AutoNAT 的节点，回退到所有已连接节点（尝试探测）
	if len(autoNATServers) == 0 {
		// 限制数量，避免浪费资源
		maxTry := 5
		if len(peers) < maxTry {
			maxTry = len(peers)
		}
		// 随机选择节点尝试（非加密场景，math/rand 足够）
		for i := 0; i < maxTry; i++ {
			idx := rand.Intn(len(peers)) //nolint:gosec // G404: 选择随机 peer 不需要加密级随机
			autoNATServers = append(autoNATServers, peers[idx])
		}
	}

	return autoNATServers
}

// sendRequest 发送请求
func (a *AutoNAT) sendRequest(stream pkgif.Stream, msg *autonat.Message) error {
	data, err := proto.Marshal(msg)
	if err != nil {
		return err
	}

	_, err = stream.Write(data)
	return err
}

// readResponse 读取响应
func (a *AutoNAT) readResponse(stream pkgif.Stream) (*autonat.Message, error) {
	buf := make([]byte, 4096)
	n, err := stream.Read(buf)
	if err != nil {
		return nil, err
	}

	msg := &autonat.Message{}
	if err := proto.Unmarshal(buf[:n], msg); err != nil {
		return nil, err
	}

	return msg, nil
}
