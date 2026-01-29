package metrics

import (
	"sync"
	"time"

	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
)

// TopologySnapshot 网络拓扑快照
//
// P2: 拓扑快照功能，用于网络健康诊断
// 实现文档：design/_discussions/20260128-p2p-log-analysis-framework.md
type TopologySnapshot struct {
	// 时间信息
	Timestamp     time.Time `json:"timestamp"`
	UptimeSeconds int64     `json:"uptimeSeconds"`

	// 节点统计
	TotalPeers  int `json:"totalPeers"`
	DirectPeers int `json:"directPeers"`
	RelayPeers  int `json:"relayPeers"`

	// 连接分布
	ConnPerPeer map[string]int `json:"connPerPeer"` // peerID -> 连接数

	// 连接类型分布
	DirectConns int `json:"directConns"`
	RelayConns  int `json:"relayConns"`

	// 地址分布
	ListenAddrs int `json:"listenAddrs"`

	// 协议分布
	ProtocolCounts map[string]int `json:"protocolCounts"` // protocol -> 使用次数
}

// PeerConnInfo 节点连接信息
type PeerConnInfo struct {
	PeerID      string   `json:"peerID"`
	ConnCount   int      `json:"connCount"`
	ConnType    string   `json:"connType"` // "direct" | "relay" | "mixed"
	RemoteAddrs []string `json:"remoteAddrs"`
}

// TopologyCollector 拓扑收集器
type TopologyCollector struct {
	mu sync.RWMutex

	// 启动时间
	startTime time.Time

	// 数据源
	swarm pkgif.Swarm

	// 上次快照
	lastSnapshot *TopologySnapshot
}

// NewTopologyCollector 创建拓扑收集器
func NewTopologyCollector(swarm pkgif.Swarm) *TopologyCollector {
	return &TopologyCollector{
		startTime: time.Now(),
		swarm:     swarm,
	}
}

// Collect 收集拓扑快照
func (c *TopologyCollector) Collect() *TopologySnapshot {
	now := time.Now()

	snapshot := &TopologySnapshot{
		Timestamp:      now,
		UptimeSeconds:  int64(now.Sub(c.startTime).Seconds()),
		ConnPerPeer:    make(map[string]int),
		ProtocolCounts: make(map[string]int),
	}

	if c.swarm == nil {
		return snapshot
	}

	// 收集连接信息
	conns := c.swarm.Conns()
	peers := c.swarm.Peers()

	snapshot.TotalPeers = len(peers)

	// 统计连接
	directPeers := make(map[string]bool)
	relayPeers := make(map[string]bool)

	for _, conn := range conns {
		if conn == nil {
			continue
		}

		peerID := string(conn.RemotePeer())
		snapshot.ConnPerPeer[peerID]++

		if conn.ConnType().IsDirect() {
			snapshot.DirectConns++
			directPeers[peerID] = true
		} else {
			snapshot.RelayConns++
			relayPeers[peerID] = true
		}

		// 收集流协议统计
		streams := conn.GetStreams()
		for _, stream := range streams {
			if stream == nil {
				continue
			}
			proto := stream.Protocol()
			if proto != "" {
				snapshot.ProtocolCounts[proto]++
			}
		}
	}

	// 计算 Direct/Relay 节点数
	for peerID := range directPeers {
		if !relayPeers[peerID] {
			snapshot.DirectPeers++
		}
	}
	for peerID := range relayPeers {
		if !directPeers[peerID] {
			snapshot.RelayPeers++
		}
	}

	// 监听地址
	snapshot.ListenAddrs = len(c.swarm.ListenAddrs())

	c.mu.Lock()
	c.lastSnapshot = snapshot
	c.mu.Unlock()

	return snapshot
}

// LogSnapshot 输出拓扑快照日志
func (c *TopologyCollector) LogSnapshot(snapshot *TopologySnapshot) {
	if snapshot == nil {
		return
	}

	// 计算 top 3 协议
	topProtos := getTopN(snapshot.ProtocolCounts, 3)

	logger.Info("P2P 拓扑快照",
		// 时间
		"uptime", snapshot.UptimeSeconds,
		// 节点
		"totalPeers", snapshot.TotalPeers,
		"directPeers", snapshot.DirectPeers,
		"relayPeers", snapshot.RelayPeers,
		// 连接
		"directConns", snapshot.DirectConns,
		"relayConns", snapshot.RelayConns,
		"listenAddrs", snapshot.ListenAddrs,
		// 协议 Top 3
		"topProtocols", formatTopN(topProtos),
	)
}

// GetPeerConnInfo 获取所有节点连接信息
func (c *TopologyCollector) GetPeerConnInfo() []PeerConnInfo {
	if c.swarm == nil {
		return nil
	}

	conns := c.swarm.Conns()
	peerConns := make(map[string]*PeerConnInfo)

	for _, conn := range conns {
		if conn == nil {
			continue
		}

		peerID := string(conn.RemotePeer())
		peerIDShort := peerID
		if len(peerIDShort) > 8 {
			peerIDShort = peerIDShort[:8]
		}

		info, exists := peerConns[peerID]
		if !exists {
			info = &PeerConnInfo{
				PeerID:      peerIDShort,
				RemoteAddrs: []string{},
			}
			peerConns[peerID] = info
		}

		info.ConnCount++
		remoteAddr := conn.RemoteMultiaddr()
		if remoteAddr != nil {
			info.RemoteAddrs = append(info.RemoteAddrs, remoteAddr.String())
		}

		// 确定连接类型
		if conn.ConnType().IsDirect() {
			switch info.ConnType {
			case "":
				info.ConnType = "direct"
			case "relay":
				info.ConnType = "mixed"
			}
		} else {
			switch info.ConnType {
			case "":
				info.ConnType = "relay"
			case "direct":
				info.ConnType = "mixed"
			}
		}
	}

	result := make([]PeerConnInfo, 0, len(peerConns))
	for _, info := range peerConns {
		result = append(result, *info)
	}

	return result
}

// GetLastSnapshot 获取最新快照
func (c *TopologyCollector) GetLastSnapshot() *TopologySnapshot {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.lastSnapshot
}

// topNEntry top N 条目
type topNEntry struct {
	Key   string
	Count int
}

// getTopN 获取 map 中值最大的 N 个键
func getTopN(m map[string]int, n int) []topNEntry {
	entries := make([]topNEntry, 0, len(m))
	for k, v := range m {
		entries = append(entries, topNEntry{Key: k, Count: v})
	}

	// 简单冒泡排序（n 很小）
	for i := 0; i < len(entries) && i < n; i++ {
		for j := i + 1; j < len(entries); j++ {
			if entries[j].Count > entries[i].Count {
				entries[i], entries[j] = entries[j], entries[i]
			}
		}
	}

	if len(entries) > n {
		entries = entries[:n]
	}
	return entries
}

// formatTopN 格式化 top N 条目为字符串
func formatTopN(entries []topNEntry) string {
	if len(entries) == 0 {
		return "none"
	}

	result := ""
	for i, e := range entries {
		if i > 0 {
			result += ", "
		}
		// 截断协议名
		key := e.Key
		if len(key) > 20 {
			key = key[:20] + "..."
		}
		result += key + ":" + intToString(int64(e.Count))
	}
	return result
}
