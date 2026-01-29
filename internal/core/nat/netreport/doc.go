// Package netreport 提供网络诊断功能
//
// # IMPL-NETWORK-RESILIENCE Phase 6.3: NetReport 诊断
//
// 本包实现了网络诊断功能，用于：
// - IPv4/IPv6 连通性检测
// - NAT 类型检测（对称 NAT / 非对称 NAT）
// - 中继延迟测量
// - 端口映射协议检测（UPnP/NAT-PMP/PCP）
// - 强制门户检测
//
// # 核心组件
//
// Report - 网络诊断报告:
//   - UDPv4/UDPv6 连通性
//   - 公网 IP 和端口
//   - NAT 类型
//   - 中继延迟
//   - 端口映射可用性
//
// Client - 诊断客户端:
//   - GetReport: 生成完整诊断报告
//   - LastReport: 获取缓存的最后报告
//   - ForceFullReport: 强制完整探测
//
// # NAT 类型检测
//
// 通过多服务器 STUN 探测检测 NAT 类型：
//   - Full Cone (EIM): 映射不随目标变化，任何外部主机可访问
//   - Symmetric: 映射随目标变化，只能被主动联系的地址访问
//
// # 使用示例
//
//	config := netreport.DefaultConfig()
//	client := netreport.NewClient(config)
//
//	report, err := client.GetReport(ctx)
//	if err != nil {
//	    log.Error("诊断失败", "err", err)
//	    return
//	}
//
//	if report.IsSymmetricNAT() {
//	    log.Info("检测到对称 NAT，需要中继")
//	}
package netreport
