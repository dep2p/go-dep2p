// Package proto 定义 DeP2P 的网络协议消息（wire format）
//
// 本包包含多个子包，每个子包定义一组相关的 Protobuf 消息：
//
// # 子包
//
//   - gossipsub: GossipSub 发布订阅协议消息
//   - rendezvous: Rendezvous 命名空间发现协议消息
//   - realm: Realm 成员认证协议消息
//   - peer: 节点记录（用于 DHT 存储）
//   - key: 密钥序列化格式
//
// # 职能
//
// pkg/proto 的职能是定义 **跨网络传输** 的协议消息：
//   - 用于异构设备通信
//   - 支持跨语言序列化（Protobuf）
//   - 需要版本兼容（向后/向前兼容）
//   - 变更成本高（影响网络协议）
//
// # 与 pkg/types 的区别
//
// pkg/proto 定义网络协议消息（wire format），
// pkg/types 定义 Go 内部数据结构（内存结构）。
//
// 当 proto 消息需要在 Go 内部使用时，应通过 pkg/types/convert.go 转换。
//
// # 使用示例
//
//	import "github.com/dep2p/go-dep2p/pkg/lib/proto/gossipsub"
//
//	msg := &gossipsub.Message{
//	    From:  nodeID.Bytes(),
//	    Topic: "my-topic",
//	    Data:  []byte("hello"),
//	}
package proto
