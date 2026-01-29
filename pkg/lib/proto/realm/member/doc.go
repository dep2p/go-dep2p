// Package memberpb 提供 Realm 成员管理的 Protobuf 消息定义。
//
// 本包定义了 Realm 成员管理使用的消息类型，包括：
//   - MemberInfo: 成员信息
//   - MemberList: 成员列表
//   - MemberListRequest/Response: 成员列表请求/响应
//   - MemberDeltaRequest/Response: 增量同步请求/响应
//   - HeartbeatRequest/Response: 心跳请求/响应
//
// 使用示例：
//
//	import pb "github.com/dep2p/go-dep2p/pkg/lib/proto/realm/member"
//
//	info := &pb.MemberInfo{
//	    PeerId:  []byte("peer-123"),
//	    RealmId: "realm-abc",
//	    Role:    1,
//	    Online:  true,
//	}
package memberpb
