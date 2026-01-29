// Package pubsub 实现发布订阅协议
package pubsub

import (
	"fmt"

	"github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/protocol"
)

// buildProtocolID 构造协议 ID
//
// 生成格式: /dep2p/app/<realmID>/pubsub/1.0.0
func buildProtocolID(realmID string) interfaces.ProtocolID {
	return interfaces.ProtocolID(protocol.BuildAppProtocol(realmID, protocol.AppProtocolPubSub, protocol.Version10))
}

// messageID 计算消息 ID
//
// 使用 from + seqno 作为唯一标识
func messageID(from string, seqno []byte) string {
	return fmt.Sprintf("%s:%x", from, seqno)
}
