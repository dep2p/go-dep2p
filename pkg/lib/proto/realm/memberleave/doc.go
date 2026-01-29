// Package memberleavepb 提供成员离开通知协议的 Protobuf 消息定义。
//
// 本包是快速断开检测机制的第二层（应用层主动通知），定义了：
//   - MemberLeave: 成员优雅离开时的广播消息
//   - LeaveReason: 离开原因枚举（GRACEFUL, KICKED, WITNESS）
//   - MemberLeaveAck: 可选的离开确认消息
//
// 协议 ID 格式（动态构建）:
//
//	使用 AppBuilder.Custom("memberleave", "1.0.0") 生成:
//	  /dep2p/app/{realmID}/memberleave/1.0.0
//
// 使用示例：
//
//	import pb "github.com/dep2p/go-dep2p/pkg/lib/proto/realm/memberleave"
//
//	msg := &pb.MemberLeave{
//	    PeerId:    []byte("peer-123"),
//	    RealmId:   []byte("realm-abc"),
//	    Reason:    pb.LeaveReason_LEAVE_REASON_GRACEFUL,
//	    Timestamp: time.Now().UnixNano(),
//	    Signature: signature,
//	}
//
// 安全约束：
//   - 消息必须包含发送者签名
//   - 时间戳有效期：30 秒
//   - 防重放机制：每个 peer_id + realm_id + timestamp 组合只处理一次
//
// 参考：
//   - design/02_constraints/protocol/L4_application/liveness.md
//   - design/03_architecture/L3_behavioral/disconnect_detection.md
package memberleavepb
