// Package witnesspb 提供见证人协议的 Protobuf 消息定义。
//
// 本包是快速断开检测机制的第三层（见证人确认），定义了：
//   - WitnessReport: 断开检测报告消息
//   - WitnessConfirmation: 见证人确认响应
//   - WitnessVotingResult: 投票结果（内部使用）
//   - DetectionMethod: 检测方法枚举
//   - ConfirmationType: 确认类型枚举
//
// 协议 ID 格式（动态构建）:
//
//	使用 AppBuilder.Custom("witness", "1.0.0") 生成:
//	  /dep2p/app/{realmID}/witness/1.0.0
//
// 使用示例：
//
//	import pb "github.com/dep2p/go-dep2p/pkg/lib/proto/realm/witness"
//
//	// 创建见证报告
//	report := &pb.WitnessReport{
//	    ReportId:           uuid.New().Bytes(),
//	    ReporterId:         []byte("reporter-peer-id"),
//	    TargetId:           []byte("disconnected-peer-id"),
//	    RealmId:            []byte("realm-abc"),
//	    DetectionMethod:    pb.DetectionMethod_DETECTION_METHOD_QUIC_TIMEOUT,
//	    Timestamp:          time.Now().UnixNano(),
//	    LastContactTimestamp: lastContact.UnixNano(),
//	    Signature:          signature,
//	}
//
//	// 创建见证确认
//	confirmation := &pb.WitnessConfirmation{
//	    ReportId:         report.ReportId,
//	    WitnessId:        []byte("witness-peer-id"),
//	    ConfirmationType: pb.ConfirmationType_CONFIRMATION_TYPE_AGREE,
//	    Timestamp:        time.Now().UnixNano(),
//	    Signature:        witnessSignature,
//	}
//
// 投票规则：
//   - 快速路径：成员数 < 10 且 QUIC_CLOSE → 单票 AGREE 无反对即确认
//   - 标准路径：简单多数（> 50% AGREE）确认
//   - 任何 DISAGREE 触发重新验证
//
// 安全约束：
//   - 报告时间戳有效期：10 秒
//   - 限速：每分钟最多 10 个报告
//   - 所有消息必须包含签名
//
// 参考：
//   - design/02_constraints/protocol/L4_application/liveness.md
//   - design/03_architecture/L3_behavioral/disconnect_detection.md
package witnesspb
