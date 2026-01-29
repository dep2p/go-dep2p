package mocks

import (
	"context"

	"github.com/dep2p/go-dep2p/pkg/interfaces"
)

// MockRealm 模拟 Realm（简化版）
//
// v2.0 统一 Relay 架构：移除 Relay 相关方法
// Relay 功能已移至节点级别
type MockRealm struct {
	// Realm 属性
	RealmIDVal string
	RealmName  string
	MemberList []string
	Joined     bool

	// 可覆盖的方法
	IDFunc        func() string
	NameFunc      func() string
	JoinFunc      func(ctx context.Context) error
	LeaveFunc     func(ctx context.Context) error
	MembersFunc   func() []string
	IsMemberFunc  func(peerID string) bool
	MessagingFunc func() interfaces.Messaging
	PubSubFunc    func() interfaces.PubSub
	StreamsFunc   func() interfaces.Streams
	LivenessFunc  func() interfaces.Liveness
}

// NewMockRealm 创建带有默认值的 MockRealm
func NewMockRealm(id string) *MockRealm {
	return &MockRealm{
		RealmIDVal: id,
		RealmName:  "mock-realm",
		MemberList: make([]string, 0),
		Joined:     false,
	}
}

// ID 返回 Realm ID
func (m *MockRealm) ID() string {
	if m.IDFunc != nil {
		return m.IDFunc()
	}
	return m.RealmIDVal
}

// Name 返回 Realm 名称
func (m *MockRealm) Name() string {
	if m.NameFunc != nil {
		return m.NameFunc()
	}
	return m.RealmName
}

// Join 加入 Realm
func (m *MockRealm) Join(ctx context.Context) error {
	if m.JoinFunc != nil {
		return m.JoinFunc(ctx)
	}
	m.Joined = true
	return nil
}

// Leave 离开 Realm
func (m *MockRealm) Leave(ctx context.Context) error {
	if m.LeaveFunc != nil {
		return m.LeaveFunc(ctx)
	}
	m.Joined = false
	return nil
}

// Members 返回成员列表
func (m *MockRealm) Members() []string {
	if m.MembersFunc != nil {
		return m.MembersFunc()
	}
	return m.MemberList
}

// IsMember 检查节点是否为成员
func (m *MockRealm) IsMember(peerID string) bool {
	if m.IsMemberFunc != nil {
		return m.IsMemberFunc(peerID)
	}
	for _, member := range m.MemberList {
		if member == peerID {
			return true
		}
	}
	return false
}

// Messaging 返回消息服务
func (m *MockRealm) Messaging() interfaces.Messaging {
	if m.MessagingFunc != nil {
		return m.MessagingFunc()
	}
	return nil // 简化实现
}

// PubSub 返回发布订阅服务
func (m *MockRealm) PubSub() interfaces.PubSub {
	if m.PubSubFunc != nil {
		return m.PubSubFunc()
	}
	return nil // 简化实现
}

// Streams 返回流服务
func (m *MockRealm) Streams() interfaces.Streams {
	if m.StreamsFunc != nil {
		return m.StreamsFunc()
	}
	return nil // 简化实现
}

// Liveness 返回存活检测服务
func (m *MockRealm) Liveness() interfaces.Liveness {
	if m.LivenessFunc != nil {
		return m.LivenessFunc()
	}
	return nil // 简化实现
}

// Connect 连接到成员（简化实现）
func (m *MockRealm) Connect(ctx context.Context, target string) (interfaces.Connection, error) {
	return nil, nil
}

// ConnectWithHint 使用地址提示连接（简化实现）
func (m *MockRealm) ConnectWithHint(ctx context.Context, target string, hints []string) (interfaces.Connection, error) {
	return nil, nil
}

// AddMember 添加成员（测试辅助方法）
func (m *MockRealm) AddMember(peerID string) {
	m.MemberList = append(m.MemberList, peerID)
}

// ============================================================================
// 以下方法用于完整实现 interfaces.Realm 接口
// ============================================================================

// PSK 返回预共享密钥
func (m *MockRealm) PSK() []byte {
	return nil
}

// Authenticate 验证对方身份
func (m *MockRealm) Authenticate(_ context.Context, peerID string, proof []byte) (bool, error) {
	return true, nil
}

// GenerateProof 生成认证证明
func (m *MockRealm) GenerateProof(_ context.Context) ([]byte, error) {
	return nil, nil
}

// Close 关闭 Realm
func (m *MockRealm) Close() error {
	return nil
}

// EventBus 返回事件总线
func (m *MockRealm) EventBus() interfaces.EventBus {
	return nil
}

// 确保实现接口
var _ interfaces.Realm = (*MockRealm)(nil)
