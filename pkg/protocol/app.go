package protocol

import "fmt"

// ============================================================================
//                          App 协议构建器
// ============================================================================
//
// App 协议是业务协议，RealmID 嵌入协议路径，需要 Realm 成员资格。
// 格式: /dep2p/app/<realmID>/<protocol>/<version>

// App 协议名称常量
const (
	AppProtocolMessaging = "messaging"
	AppProtocolPubSub    = "pubsub"
	AppProtocolStreams   = "streams"
	AppProtocolLiveness  = "liveness"
)

// AppBuilder App 协议构建器
type AppBuilder struct {
	realmID string
}

// NewAppBuilder 创建 App 协议构建器
func NewAppBuilder(realmID string) *AppBuilder {
	return &AppBuilder{realmID: realmID}
}

// RealmID 返回构建器关联的 RealmID
func (b *AppBuilder) RealmID() string {
	return b.realmID
}

// Messaging 返回消息协议 ID
// 用于点对点消息通信
func (b *AppBuilder) Messaging() ID {
	return ID(fmt.Sprintf("/dep2p/app/%s/messaging/1.0.0", b.realmID))
}

// PubSub 返回发布订阅协议 ID
// 用于发布订阅消息模式
func (b *AppBuilder) PubSub() ID {
	return ID(fmt.Sprintf("/dep2p/app/%s/pubsub/1.0.0", b.realmID))
}

// Streams 返回流协议 ID
// 用于流式数据传输
func (b *AppBuilder) Streams() ID {
	return ID(fmt.Sprintf("/dep2p/app/%s/streams/1.0.0", b.realmID))
}

// Liveness 返回存活检测协议 ID
// 用于应用层存活检测
func (b *AppBuilder) Liveness() ID {
	return ID(fmt.Sprintf("/dep2p/app/%s/liveness/1.0.0", b.realmID))
}

// Custom 返回自定义协议 ID
func (b *AppBuilder) Custom(name, version string) ID {
	return ID(fmt.Sprintf("/dep2p/app/%s/%s/%s", b.realmID, name, version))
}

// BuildAppProtocol 便捷函数：构建 App 协议 ID
func BuildAppProtocol(realmID, name, version string) ID {
	return ID(fmt.Sprintf("/dep2p/app/%s/%s/%s", realmID, name, version))
}
