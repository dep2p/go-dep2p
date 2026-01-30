package dep2p

import (
	"context"
	"fmt"
	"time"

	"github.com/dep2p/go-dep2p/pkg/types"
)

// ════════════════════════════════════════════════════════════════════════════
//                              网络诊断
// ════════════════════════════════════════════════════════════════════════════

// GetNetworkDiagnostics 获取网络诊断报告
//
// 运行全面的网络诊断，检测：
//   - IPv4/IPv6 可用性和外部地址
//   - NAT 类型
//   - 端口映射协议可用性（UPnP、NAT-PMP、PCP）
//   - 是否存在强制门户
//   - 中继服务器延迟
//
// 示例：
//
//	report, err := node.GetNetworkDiagnostics(ctx)
//	if err != nil {
//	    log.Printf("诊断失败: %v", err)
//	    return
//	}
//	fmt.Printf("IPv4 外部地址: %s:%d\n", report.IPv4GlobalIP, report.IPv4Port)
//	fmt.Printf("NAT 类型: %s\n", report.NATType)
func (n *Node) GetNetworkDiagnostics(ctx context.Context) (*NetworkDiagnosticReport, error) {
	if n.netReportClient == nil {
		return nil, fmt.Errorf("network diagnostics not available (NAT module not loaded)")
	}

	report, err := n.netReportClient.GetReport(ctx)
	if err != nil {
		return nil, err
	}

	// 转换为用户友好格式
	result := &NetworkDiagnosticReport{
		IPv4Available:   report.UDPv4,
		IPv6Available:   report.UDPv6,
		UPnPAvailable:   report.UPnPAvailable,
		NATPMPAvailable: report.NATPMPAvailable,
		PCPAvailable:    report.PCPAvailable,
		Duration:        report.Duration.Milliseconds(),
		RelayLatencies:  make(map[string]int64),
	}

	// CaptivePortal 是指针类型
	if report.CaptivePortal != nil {
		result.CaptivePortal = *report.CaptivePortal
	}

	if report.GlobalV4 != nil {
		result.IPv4GlobalIP = report.GlobalV4.String()
		result.IPv4Port = int(report.GlobalV4Port)
	}

	if report.GlobalV6 != nil {
		result.IPv6GlobalIP = report.GlobalV6.String()
	}

	// NAT 类型
	result.NATType = report.NATType.String()

	// 中继延迟
	for url, latency := range report.RelayLatencies {
		result.RelayLatencies[url] = latency.Milliseconds()
	}

	return result, nil
}

// ════════════════════════════════════════════════════════════════════════════
//                              种子节点恢复
// ════════════════════════════════════════════════════════════════════════════

// RecoverSeeds 从节点缓存恢复种子节点并尝试连接
//
// 用于节点重启后快速恢复网络连接，而无需从头开始发现。
//
// 参数:
//   - ctx: 上下文
//   - count: 最大恢复节点数
//   - maxAge: 节点最大年龄（超过此时间的节点不恢复）
//
// 返回:
//   - 成功连接的节点数
//   - 恢复的种子节点列表
//   - 错误信息
//
// 示例:
//
//	connected, seeds, err := node.RecoverSeeds(ctx, 50, 24*time.Hour)
//	if err != nil {
//	    log.Printf("恢复种子失败: %v", err)
//	}
//	log.Printf("从 %d 个种子中成功连接 %d 个", len(seeds), connected)
func (n *Node) RecoverSeeds(ctx context.Context, count int, maxAge time.Duration) (int, []SeedRecord, error) {
	if n.host == nil {
		return 0, nil, fmt.Errorf("host not available")
	}

	ps := n.host.Peerstore()
	if ps == nil {
		return 0, nil, fmt.Errorf("peerstore not available")
	}

	// 查询种子节点
	seeds := ps.QuerySeeds(count, maxAge)
	if len(seeds) == 0 {
		return 0, nil, nil
	}

	// 转换为用户友好格式
	records := make([]SeedRecord, 0, len(seeds))
	for _, seed := range seeds {
		records = append(records, SeedRecord{
			ID:       seed.ID,
			Addrs:    seed.Addrs,
			LastSeen: seed.LastSeen,
			LastPong: seed.LastPong,
		})
	}

	// 尝试连接种子节点
	connected := 0
	for _, seed := range seeds {
		if len(seed.Addrs) == 0 {
			continue
		}

		// 尝试连接（传入地址列表）
		if err := n.host.Connect(ctx, seed.ID, seed.Addrs); err == nil {
			connected++
			// 更新拨号成功状态
			ps.UpdateDialAttempt(types.PeerID(seed.ID), true)
		} else {
			// 更新拨号失败状态
			ps.UpdateDialAttempt(types.PeerID(seed.ID), false)
		}
	}

	logger.Info("种子节点恢复完成", "total", len(seeds), "connected", connected)
	return connected, records, nil
}

// GetSeedCount 获取节点缓存中的种子节点数量
func (n *Node) GetSeedCount() int {
	if n.host == nil {
		return 0
	}
	ps := n.host.Peerstore()
	if ps == nil {
		return 0
	}
	return ps.NodeDBSize()
}

// ════════════════════════════════════════════════════════════════════════════
//                              自省服务
// ════════════════════════════════════════════════════════════════════════════

// GetIntrospectInfo 获取自省服务信息
//
// 返回自省服务的状态和可用端点。
//
// 示例:
//
//	info := node.GetIntrospectInfo()
//	if info.Enabled {
//	    fmt.Printf("自省服务地址: %s\n", info.Addr)
//	    fmt.Printf("可用端点: %v\n", info.Endpoints)
//	}
func (n *Node) GetIntrospectInfo() IntrospectInfo {
	if n.introspectServer == nil {
		return IntrospectInfo{Enabled: false}
	}

	return IntrospectInfo{
		Enabled: true,
		Addr:    n.introspectServer.Addr(),
		Endpoints: []string{
			"/debug/introspect",
			"/debug/introspect/node",
			"/debug/introspect/connections",
			"/debug/introspect/peers",
			"/debug/introspect/bandwidth",
			"/debug/introspect/runtime",
			"/debug/pprof/",
			"/health",
		},
	}
}

// GetIntrospectAddr 获取自省服务监听地址
//
// 如果自省服务未启用，返回空字符串。
func (n *Node) GetIntrospectAddr() string {
	if n.introspectServer == nil {
		return ""
	}
	return n.introspectServer.Addr()
}

// IsIntrospectEnabled 检查自省服务是否启用
func (n *Node) IsIntrospectEnabled() bool {
	return n.introspectServer != nil
}
