// Package authpb 提供 Realm 认证协议的 Protobuf 消息定义。
//
// 本包定义了 Realm 认证流程中使用的所有消息类型，包括：
//   - Message: 认证协议消息封装
//   - AuthRequest: 认证请求
//   - AuthChallenge: 认证挑战
//   - AuthResponse: 认证响应
//   - AuthResult: 认证结果
//
// 认证模式支持：
//   - PSK (Pre-Shared Key): 预共享密钥认证
//   - Cert: 证书认证
//   - Custom: 自定义认证
//
// 使用示例：
//
//	import pb "github.com/dep2p/go-dep2p/pkg/lib/proto/realm/auth"
//
//	req := &pb.AuthRequest{
//	    PeerId:  "peer-123",
//	    RealmId: "realm-abc",
//	    Mode:    pb.AuthMode_AUTH_MODE_PSK,
//	}
package authpb
