package protocolids

import "github.com/dep2p/go-dep2p/pkg/types"

// ============================================================================
// 应用协议 ID（/dep2p/app/...）
// 应用协议默认需要 RealmAuth 验证，是业务层协议
// ============================================================================

// ----------------------------------------------------------------------------
// 消息传递协议（应用级）
// ----------------------------------------------------------------------------

// AppMessagingRequest 请求-响应消息协议
const AppMessagingRequest types.ProtocolID = "/dep2p/app/messaging/request/1.0.0"

// AppMessagingNotify 单向通知消息协议
const AppMessagingNotify types.ProtocolID = "/dep2p/app/messaging/notify/1.0.0"

// AppMessagingPubsub 发布-订阅消息协议
const AppMessagingPubsub types.ProtocolID = "/dep2p/app/messaging/pubsub/1.0.0"

// AppMessagingQuery 查询消息协议
const AppMessagingQuery types.ProtocolID = "/dep2p/app/messaging/query/1.0.0"

// AppMessagingQueryResponse 查询响应消息协议
const AppMessagingQueryResponse types.ProtocolID = "/dep2p/app/messaging/query-response/1.0.0"

// ----------------------------------------------------------------------------
// 示例协议（供 examples/ 和演示使用）
// ----------------------------------------------------------------------------

// AppChat 聊天示例协议
const AppChat types.ProtocolID = "/dep2p/app/chat/1.0.0"

// AppRelayDemo 中继演示协议
const AppRelayDemo types.ProtocolID = "/dep2p/app/relay-demo/1.0.0"

// AppTest 通用测试协议（用于示例和集成测试中的应用层测试）
const AppTest types.ProtocolID = "/dep2p/app/test/1.0.0"

