// Package stun 实现 STUN 客户端
//
// stun 使用 STUN 协议（RFC 5389）获取节点的公网 IP 地址和端口，
// 并检测 NAT 类型。
//
// # 功能
//
//   - 获取公网 IP 和端口
//   - NAT 类型检测
//   - 多服务器探测
//
// # NAT 类型
//
//   - NATTypeUnknown: 未知
//   - NATTypeNone: 无 NAT（公网）
//   - NATTypeFullCone: 完全锥形
//   - NATTypeRestrictedCone: 受限锥形
//   - NATTypePortRestricted: 端口受限
//   - NATTypeSymmetric: 对称 NAT
//
// # 使用示例
//
//	client := stun.NewClient(stun.DefaultServers)
//	addr, err := client.GetExternalAddr(ctx)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	fmt.Println("External address:", addr)
//
//	natType, err := client.DetectNATType(ctx)
package stun
