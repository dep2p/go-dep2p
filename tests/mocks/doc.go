// Package mocks 提供统一的测试 Mock 实现
//
// 本包整合了项目中分散的 Mock 实现，提供标准化的测试双（Test Doubles）。
// 新测试应优先使用这些统一 Mock，以保持一致性和可维护性。
//
// # 核心 Mock
//
//   - MockHost: 模拟 interfaces.Host，支持自定义 ID、地址、流处理等
//   - MockStream: 模拟 interfaces.Stream，支持读写数据模拟
//   - MockConnection: 模拟 interfaces.Connection，支持连接属性和流创建
//
// # 身份 Mock
//
//   - MockIdentity: 模拟 interfaces.Identity，包含签名验证功能
//   - MockPublicKey: 模拟 interfaces.PublicKey
//   - MockPrivateKey: 模拟 interfaces.PrivateKey
//
// # 发现 Mock
//
//   - MockDiscovery: 模拟 interfaces.Discovery，支持 FindPeers/Advertise
//
// # 传输 Mock
//
//   - MockTransport: 模拟 interfaces.Transport
//   - MockListener: 模拟 interfaces.Listener
//
// # 连接管理 Mock
//
//   - MockConnMgr: 模拟 interfaces.ConnManager，支持标签和保护功能
//
// # 消息 Mock
//
//   - MockMessaging: 简化的消息服务 Mock
//   - MockPubSub: 简化的发布订阅 Mock
//   - MockTopic: 简化的主题 Mock
//   - MockSubscription: 简化的订阅 Mock
//
// # 其他 Mock
//
//   - MockRealm: 简化的 Realm Mock
//   - MockNode: 简化的节点 Mock
//
// # 设计原则
//
// 1. 函数式注入: 每个 Mock 都支持通过 XxxFunc 字段注入自定义行为
// 2. 调用记录: 关键 Mock 记录调用历史，便于验证测试行为
// 3. 简化实现: 部分 Mock 不完全实现接口，仅提供测试所需的核心功能
//
// # 使用示例
//
// 基础用法:
//
//	import "github.com/dep2p/go-dep2p/tests/mocks"
//
//	func TestMyFunction(t *testing.T) {
//	    host := mocks.NewMockHost("test-peer-id")
//	    // 使用默认行为
//	    if host.ID() != "test-peer-id" {
//	        t.Error("unexpected ID")
//	    }
//	}
//
// 自定义行为:
//
//	func TestWithCustomBehavior(t *testing.T) {
//	    host := &mocks.MockHost{
//	        IDValue: "custom-id",
//	        ConnectFunc: func(ctx context.Context, peerID string, addrs []string) error {
//	            return errors.New("connection refused")
//	        },
//	    }
//	    err := host.Connect(context.Background(), "target", nil)
//	    if err == nil {
//	        t.Error("expected error")
//	    }
//	}
//
// 验证调用:
//
//	func TestCallRecording(t *testing.T) {
//	    host := mocks.NewMockHost("test")
//	    host.Connect(context.Background(), "peer1", []string{"/ip4/1.2.3.4/tcp/4001"})
//	    host.Connect(context.Background(), "peer2", nil)
//
//	    if len(host.ConnectCalls) != 2 {
//	        t.Errorf("expected 2 Connect calls, got %d", len(host.ConnectCalls))
//	    }
//	    if host.ConnectCalls[0].PeerID != "peer1" {
//	        t.Error("unexpected first peer")
//	    }
//	}
//
// # 迁移指南
//
// 现有测试文件中的本地 Mock 可逐步迁移到本包。迁移步骤：
//
//  1. 导入本包: import "github.com/dep2p/go-dep2p/tests/mocks"
//  2. 删除本地 Mock 定义
//  3. 将 &LocalMockHost{} 替换为 mocks.NewMockHost() 或 &mocks.MockHost{}
//  4. 验证测试通过
package mocks
