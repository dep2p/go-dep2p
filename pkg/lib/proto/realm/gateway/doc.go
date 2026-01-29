// Package gatewaypb 提供 Realm Gateway 的 Protobuf 消息定义。
//
// 本包定义了 Gateway 中继服务使用的消息类型，包括：
//   - RelayRequest: 中继请求
//   - RelayResponse: 中继响应
//   - GatewayCapacity: 网关容量信息
//   - RelayStats: 中继统计
//
// 使用示例：
//
//	import pb "github.com/dep2p/go-dep2p/pkg/lib/proto/realm/gateway"
//
//	req := &pb.RelayRequest{
//	    SourcePeerId: []byte("source-peer"),
//	    TargetPeerId: []byte("target-peer"),
//	    Protocol:     "/dep2p/app/realm-1/messaging/1.0.0",
//	}
package gatewaypb
