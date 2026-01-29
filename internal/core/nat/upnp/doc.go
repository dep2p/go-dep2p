// Package upnp 实现 UPnP 端口映射
//
// upnp 使用 UPnP IGD 协议自动配置路由器的端口映射，
// 支持大多数家用路由器。
//
// # 功能
//
//   - 自动发现 IGD 设备（支持 IGDv1 和 IGDv2）
//   - 创建端口映射
//   - 映射续期（自动）
//   - 删除映射
//   - 获取外部 IP（从 UPnP 网关动态获取）
//   - 上报候选地址到 ReachabilityCoordinator
//
// # 使用示例
//
//	mapper, err := upnp.NewUPnPMapper()
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	// 设置 ReachabilityCoordinator（可选，用于地址上报）
//	mapper.SetReachabilityCoordinator(coordinator)
//
//	// 创建映射（成功后会自动获取外部 IP 并上报到 Coordinator）
//	externalPort, err := mapper.MapPort(ctx, "UDP", 4001)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer mapper.UnmapPort("UDP", externalPort)
package upnp
