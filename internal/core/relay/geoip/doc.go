// Package geoip 提供 GeoIP 解析功能
//
// 用于 Relay 区域感知选路：根据 IP 地址判断节点所在地理区域。
//
// # 实现
//
// 提供多种实现：
//   - StubResolver: 测试用桩解析器，支持预设映射
//   - SimpleResolver: 基于已知 IP 段的简单区域判断
//
// # 使用示例
//
//	// 使用桩解析器（测试）
//	resolver := geoip.NewStubResolver()
//	resolver.SetRegion("1.2.3.4", "AS")
//	info, _ := resolver.LookupString("1.2.3.4")
//	fmt.Println(info.ContinentCode) // "AS"
//
//	// 使用简单解析器
//	resolver := geoip.NewSimpleResolver()
//	info, _ := resolver.LookupString("8.8.8.8")
//	fmt.Println(info.ToRegionString())
//
// # 区域代码
//
// 使用大洲代码作为主要区域标识：
//   - NA: 北美洲
//   - SA: 南美洲
//   - EU: 欧洲
//   - AF: 非洲
//   - AS: 亚洲
//   - OC: 大洋洲
//   - AN: 南极洲
//
// # 扩展
//
// 如需使用 MaxMind MMDB 数据库，可引入 geoip2-golang 库
// 并实现 Resolver 接口。
package geoip
