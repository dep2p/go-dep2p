// Package routingpb 提供 Realm 路由的 Protobuf 消息定义。
//
// 本包定义了 Realm 路由系统使用的消息类型，包括：
//   - RouteQuery/Response: 路由查询
//   - RouteInfo: 路由信息
//   - PathInfo/Query/Response: 路径信息
//   - NodeMetrics: 节点指标
//   - LoadReport/Query/Response: 负载信息
//   - RelayRequest/Response: 中继请求
//   - ReachabilityQuery/Response: 可达性查询
//   - GatewayStateSync: Gateway 状态同步
//
// 使用示例：
//
//	import pb "github.com/dep2p/go-dep2p/pkg/lib/proto/realm/routing"
//
//	query := &pb.RouteQuery{
//	    SourcePeerId: []byte("source"),
//	    TargetPeerId: []byte("target"),
//	    MaxHops:      3,
//	}
package routingpb
