// Package natpmp 实现 NAT-PMP 端口映射
//
// natpmp 使用 NAT-PMP 协议（RFC 6886）自动配置 NAT 设备的端口映射，
// 主要用于 Apple 路由器和其他支持该协议的设备。
//
// # 功能
//
//   - 自动发现网关
//   - 创建端口映射
//   - 映射续期
//   - 删除映射
//
// # 使用示例
//
//	mapper, err := natpmp.NewMapper()
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	mapping, err := mapper.MapPort(ctx, "udp", 4001, 4001, 3600)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer mapper.DeleteMapping(mapping)
package natpmp
