// Package pubsub 实现发布订阅协议
package pubsub

import (
	"context"
	"fmt"

	"github.com/dep2p/go-dep2p/pkg/interfaces"
	pb "github.com/dep2p/go-dep2p/pkg/lib/proto/gossipsub"
	"github.com/dep2p/go-dep2p/pkg/protocol"
)

// ValidatorFunc 消息验证函数类型
type ValidatorFunc func(context.Context, string, *pb.Message) bool

// messageValidator 消息验证器
type messageValidator struct {
	realmMgr   interfaces.RealmManager // 全局模式使用
	realm      interfaces.Realm        // Realm-bound 模式使用
	maxSize    int
	validators map[string]ValidatorFunc // topic -> validator
}

// newMessageValidator 创建消息验证器（全局模式）
func newMessageValidator(realmMgr interfaces.RealmManager, maxSize int) *messageValidator {
	return &messageValidator{
		realmMgr:   realmMgr,
		maxSize:    maxSize,
		validators: make(map[string]ValidatorFunc),
	}
}

// newMessageValidatorForRealm 创建消息验证器（Realm-bound 模式）
func newMessageValidatorForRealm(realm interfaces.Realm, maxSize int) *messageValidator {
	return &messageValidator{
		realm:      realm,
		maxSize:    maxSize,
		validators: make(map[string]ValidatorFunc),
	}
}

// Validate 验证消息
func (mv *messageValidator) Validate(ctx context.Context, peerID string, msg *pb.Message) error {
	// 1. 检查消息大小
	if len(msg.Data) > mv.maxSize {
		return fmt.Errorf("%w: size=%d, max=%d", ErrMessageTooLarge, len(msg.Data), mv.maxSize)
	}

	// 2. 检查必要字段
	if len(msg.From) == 0 || len(msg.Data) == 0 || msg.Topic == "" {
		return fmt.Errorf("%w: missing required fields", ErrInvalidMessage)
	}

	// 3. 系统 topic 完全跳过成员验证
	//
	// 成员同步 topic（/dep2p/realm/<realmID>/members）用于内部成员同步通信，
	// 必须跳过成员验证，否则会形成鸡生蛋的死锁：
	//   - 新节点发送 req:sync 请求 → 被拒绝（因为还不是成员）
	//   - 新节点无法收到成员列表 → 永远无法成为成员
	//
	// 安全考虑：
	//   - 成员同步消息本身不包含敏感数据，只是成员 ID 列表
	//   - 真正的成员验证在 Realm 认证阶段完成
	//   - 恶意节点最多能看到成员列表，但无法伪装成员（需要认证）
	if isSystemTopic(msg.Topic) {
		// 完全跳过成员验证，直接进入自定义验证器
		goto customValidator
	}

	// 4. 验证发送者是 Realm 成员
	if !mv.isRealmMember(string(msg.From)) {
		return fmt.Errorf("%w: peer=%s", ErrNotRealmMember, string(msg.From))
	}

customValidator:
	// 5. 调用主题特定的验证器(如果存在)
	if validator, exists := mv.validators[msg.Topic]; exists {
		if !validator(ctx, peerID, msg) {
			return fmt.Errorf("%w: custom validator failed", ErrInvalidMessage)
		}
	}

	return nil
}

// isSystemTopic 检查是否是系统 topic
//
// 系统 topic 用于内部通信，包括：
// - 旧格式：以 "__sys__/" 开头（如 __sys__/members）- 已废弃
// - 新格式：Realm 作用域成员同步（/dep2p/realm/<realmID>/members）
//
// Step B2 对齐：支持新的 Realm 作用域 Topic 格式
func isSystemTopic(topic string) bool {
	// 旧格式：__sys__/xxx（向后兼容）
	const systemTopicPrefix = "__sys__/"
	if len(topic) > len(systemTopicPrefix) &&
		topic[:len(systemTopicPrefix)] == systemTopicPrefix {
		return true
	}

	// 新格式：/dep2p/realm/<realmID>/members
	// 匹配模式：以 "/dep2p/realm/" 开头且以 "/members" 结尾
	realmMemberPrefix := protocol.PrefixRealm + "/"
	const membersSuffix = "/members"
	if len(topic) > len(realmMemberPrefix)+len(membersSuffix) &&
		topic[:len(realmMemberPrefix)] == realmMemberPrefix &&
		topic[len(topic)-len(membersSuffix):] == membersSuffix {
		return true
	}

	return false
}

// RegisterValidator 注册主题验证器
func (mv *messageValidator) RegisterValidator(topic string, validator ValidatorFunc) {
	mv.validators[topic] = validator
}

// UnregisterValidator 注销主题验证器
func (mv *messageValidator) UnregisterValidator(topic string) {
	delete(mv.validators, topic)
}

// isRealmMember 检查节点是否是任何 Realm 的成员
func (mv *messageValidator) isRealmMember(peerID string) bool {
	// Realm-bound 模式：使用绑定的 Realm
	if mv.realm != nil {
		return mv.realm.IsMember(peerID)
	}

	// 全局模式：遍历所有 Realm
	if mv.realmMgr == nil {
		return false
	}

	realms := mv.realmMgr.ListRealms()
	for _, realm := range realms {
		if realm.IsMember(peerID) {
			return true
		}
	}
	return false
}
