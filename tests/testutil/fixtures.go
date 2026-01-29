// Package testutil 提供测试辅助工具
package testutil

// 测试数据固件
//
// 提供测试中常用的常量值，确保测试一致性。

const (
	// DefaultTestPSK 默认测试 PSK
	//
	// 所有使用此 PSK 的节点会加入同一个 Realm。
	DefaultTestPSK = "test-psk-for-dep2p-integration-testing"

	// DefaultTestTopic 默认测试主题
	//
	// 用于 PubSub 测试的主题名称。
	DefaultTestTopic = "test/chat"

	// DefaultTestProtocol 默认测试协议 ID
	//
	// 用于 Streams 测试的协议标识。
	DefaultTestProtocol = "test-protocol"
)
