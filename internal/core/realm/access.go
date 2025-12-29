// Package realm 提供 Realm 管理实现
package realm

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"sync"
	"time"

	"github.com/dep2p/go-dep2p/internal/util/logger"
	realmif "github.com/dep2p/go-dep2p/pkg/interfaces/realm"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// 包级别日志实例
var log = logger.Logger("realm")

// ============================================================================
//                              错误定义
// ============================================================================

// 访问控制相关错误
var (
	// ErrInvalidInvite 无效的邀请
	ErrInvalidInvite = errors.New("invalid invite")
	ErrInviteExpired     = errors.New("invite expired")
	ErrAlreadyMember     = errors.New("already a member")
	ErrNotAuthorized     = errors.New("not authorized")
	ErrInviteNotForNode  = errors.New("invite not for this node")
)

// ============================================================================
//                              常量定义
// ============================================================================

const (
	// InviteExpiration 邀请过期时间
	InviteExpiration = 24 * time.Hour

	// JoinKeySaltSize 盐值大小
	JoinKeySaltSize = 16

	// InviteTokenSize 邀请令牌大小
	InviteTokenSize = 32
)

// ============================================================================
//                              AccessController 实现
// ============================================================================

// AccessController 访问控制器实现
type AccessController struct {
	// Realm 访问级别
	accessLevels map[types.RealmID]types.AccessLevel
	accessMu     sync.RWMutex

	// JoinKey 存储 (RealmID -> hashed key)
	joinKeys map[types.RealmID][]byte
	keyMu    sync.RWMutex

	// 邀请存储 (token -> invite)
	invites  map[string]*invite
	inviteMu sync.RWMutex

	// 成员管理器引用
	manager *Manager

	// 停止信号
	stopCh    chan struct{}
	closeOnce sync.Once
}

// invite 邀请信息
type invite struct {
	realmID    types.RealmID
	targetNode types.NodeID
	issuer     types.NodeID
	createdAt  time.Time
	expiresAt  time.Time
	token      []byte
}

// NewAccessController 创建访问控制器
func NewAccessController(manager *Manager) *AccessController {
	ac := &AccessController{
		accessLevels: make(map[types.RealmID]types.AccessLevel),
		joinKeys:     make(map[types.RealmID][]byte),
		invites:      make(map[string]*invite),
		manager:      manager,
		stopCh:       make(chan struct{}),
	}

	// 启动清理过期邀请的协程
	go ac.cleanupLoop()

	return ac
}

// SetAccess 设置 Realm 访问级别
func (ac *AccessController) SetAccess(realmID types.RealmID, access types.AccessLevel) error {
	ac.accessMu.Lock()
	ac.accessLevels[realmID] = access
	ac.accessMu.Unlock()

	log.Info("设置 Realm 访问级别",
		"realm", string(realmID),
		"access", access.String())

	return nil
}

// GetAccess 获取 Realm 访问级别
func (ac *AccessController) GetAccess(realmID types.RealmID) types.AccessLevel {
	ac.accessMu.RLock()
	defer ac.accessMu.RUnlock()

	access, ok := ac.accessLevels[realmID]
	if !ok {
		return types.AccessLevelPublic // 默认公开
	}
	return access
}

// SetJoinKey 设置 Realm 加入密钥
func (ac *AccessController) SetJoinKey(realmID types.RealmID, key []byte) error {
	// 生成盐值
	salt := make([]byte, JoinKeySaltSize)
	if _, err := rand.Read(salt); err != nil {
		return err
	}

	// 对密钥进行哈希
	hashedKey := ac.hashJoinKey(key, salt)

	// 存储: salt + hashedKey
	storedKey := append(salt, hashedKey...)

	ac.keyMu.Lock()
	ac.joinKeys[realmID] = storedKey
	ac.keyMu.Unlock()

	log.Info("设置 Realm 加入密钥",
		"realm", string(realmID))

	return nil
}

// ValidateJoinKey 验证加入密钥
func (ac *AccessController) ValidateJoinKey(realmID types.RealmID, key []byte) bool {
	ac.keyMu.RLock()
	storedKey, ok := ac.joinKeys[realmID]
	ac.keyMu.RUnlock()

	if !ok {
		// 没有设置密钥，检查访问级别
		access := ac.GetAccess(realmID)
		// 公开 Realm 不需要密钥
		return access == types.AccessLevelPublic
	}

	// 验证密钥长度
	if len(storedKey) < JoinKeySaltSize {
		return false
	}

	// 提取盐值和哈希值
	salt := storedKey[:JoinKeySaltSize]
	expectedHash := storedKey[JoinKeySaltSize:]

	// 计算提供密钥的哈希
	actualHash := ac.hashJoinKey(key, salt)

	return hmac.Equal(expectedHash, actualHash)
}

// hashJoinKey 对密钥进行哈希
func (ac *AccessController) hashJoinKey(key, salt []byte) []byte {
	h := hmac.New(sha256.New, salt)
	h.Write(key)
	return h.Sum(nil)
}

// GenerateInvite 生成邀请
func (ac *AccessController) GenerateInvite(realmID types.RealmID, targetNode types.NodeID) ([]byte, error) {
	// 检查是否有权限生成邀请
	access := ac.GetAccess(realmID)
	if access != types.AccessLevelPrivate {
		return nil, ErrNotAuthorized
	}

	// 生成令牌
	token := make([]byte, InviteTokenSize)
	if _, err := rand.Read(token); err != nil {
		return nil, err
	}

	now := time.Now()
	inv := &invite{
		realmID:    realmID,
		targetNode: targetNode,
		createdAt:  now,
		expiresAt:  now.Add(InviteExpiration),
		token:      token,
	}

	// 存储邀请
	tokenKey := string(token)
	ac.inviteMu.Lock()
	ac.invites[tokenKey] = inv
	ac.inviteMu.Unlock()

	// 构建邀请数据
	// 格式: [RealmID长度(2)][RealmID][TargetNodeID(32)][ExpiresAt(8)][Token(32)]
	var buf bytes.Buffer
	realmBytes := []byte(realmID)
	_ = binary.Write(&buf, binary.BigEndian, uint16(len(realmBytes)))
	buf.Write(realmBytes)
	buf.Write(targetNode[:])
	_ = binary.Write(&buf, binary.BigEndian, inv.expiresAt.Unix())
	buf.Write(token)

	log.Info("生成邀请",
		"realm", string(realmID),
		"target", targetNode.ShortString())

	return buf.Bytes(), nil
}

// ValidateInvite 验证邀请
func (ac *AccessController) ValidateInvite(realmID types.RealmID, inviteData []byte, nodeID types.NodeID) bool {
	// 解析邀请数据
	buf := bytes.NewReader(inviteData)

	// RealmID
	var realmLen uint16
	if err := binary.Read(buf, binary.BigEndian, &realmLen); err != nil {
		return false
	}
	realmBytes := make([]byte, realmLen)
	if _, err := buf.Read(realmBytes); err != nil {
		return false
	}
	invRealmID := types.RealmID(realmBytes)

	// 验证 Realm ID 匹配
	if invRealmID != realmID {
		return false
	}

	// TargetNodeID
	var targetNode types.NodeID
	if _, err := buf.Read(targetNode[:]); err != nil {
		return false
	}

	// 验证目标节点
	if targetNode != nodeID {
		log.Debug("邀请目标节点不匹配",
			"expected", targetNode.ShortString(),
			"actual", nodeID.ShortString())
		return false
	}

	// ExpiresAt
	var expiresUnix int64
	if err := binary.Read(buf, binary.BigEndian, &expiresUnix); err != nil {
		return false
	}
	expiresAt := time.Unix(expiresUnix, 0)

	// 验证过期时间
	if time.Now().After(expiresAt) {
		log.Debug("邀请已过期")
		return false
	}

	// Token
	token := make([]byte, InviteTokenSize)
	if _, err := buf.Read(token); err != nil {
		return false
	}

	// 验证令牌
	tokenKey := string(token)
	ac.inviteMu.RLock()
	inv, ok := ac.invites[tokenKey]
	ac.inviteMu.RUnlock()

	if !ok {
		log.Debug("邀请令牌无效")
		return false
	}

	// 额外验证
	if inv.realmID != realmID || inv.targetNode != nodeID {
		return false
	}

	// 使用后删除邀请（一次性）
	ac.inviteMu.Lock()
	delete(ac.invites, tokenKey)
	ac.inviteMu.Unlock()

	log.Info("邀请验证成功",
		"realm", string(realmID),
		"node", nodeID.ShortString())

	return true
}

// KickMember 踢出成员
//
// 此方法会：
// 1. 向被踢节点发送 Goodbye 消息通知（如果 liveness 服务可用）
// 2. 从本地 Realm 成员列表中移除该节点
func (ac *AccessController) KickMember(realmID types.RealmID, nodeID types.NodeID) error {
	if ac.manager == nil {
		return ErrNotAuthorized
	}

	// 向被踢节点发送 Goodbye 通知
	if ac.manager.liveness != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		// 使用 GoodbyeReason 类型（被踢出类似于被禁止）
		reason := types.GoodbyeReason("kicked:" + string(realmID))
		if err := ac.manager.liveness.SendGoodbyeTo(ctx, nodeID, reason); err != nil {
			log.Warn("发送踢出通知失败",
				"realm", string(realmID),
				"node", nodeID.ShortString(),
				"err", err)
			// 继续执行移除操作，即使通知失败
		} else {
			log.Debug("已发送踢出通知",
				"realm", string(realmID),
				"node", nodeID.ShortString())
		}
	}

	// 从 Realm 中移除节点
	ac.manager.removeRealmPeer(realmID, nodeID)

	log.Info("踢出成员",
		"realm", string(realmID),
		"node", nodeID.ShortString())

	return nil
}

// CanJoin 检查是否可以加入 Realm
func (ac *AccessController) CanJoin(realmID types.RealmID, nodeID types.NodeID, joinKey []byte, inviteData []byte) error {
	access := ac.GetAccess(realmID)

	switch access {
	case types.AccessLevelPublic:
		// 公开 Realm，任何人可加入
		return nil

	case types.AccessLevelProtected:
		// 保护 Realm，需要 JoinKey
		if len(joinKey) == 0 {
			return ErrAccessDenied
		}
		if !ac.ValidateJoinKey(realmID, joinKey) {
			return ErrInvalidJoinKey
		}
		return nil

	case types.AccessLevelPrivate:
		// 私有 Realm，需要邀请
		if len(inviteData) == 0 {
			return ErrAccessDenied
		}
		if !ac.ValidateInvite(realmID, inviteData, nodeID) {
			return ErrInvalidInvite
		}
		return nil

	default:
		return ErrAccessDenied
	}
}

// cleanupLoop 清理过期邀请
func (ac *AccessController) cleanupLoop() {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ac.stopCh:
			return
		case <-ticker.C:
			ac.cleanupExpiredInvites()
		}
	}
}

// Close 关闭访问控制器
func (ac *AccessController) Close() {
	ac.closeOnce.Do(func() {
		close(ac.stopCh)
	})
}

// cleanupExpiredInvites 清理过期邀请
func (ac *AccessController) cleanupExpiredInvites() {
	now := time.Now()

	ac.inviteMu.Lock()
	defer ac.inviteMu.Unlock()

	for key, inv := range ac.invites {
		if now.After(inv.expiresAt) {
			delete(ac.invites, key)
		}
	}
}

// 确保实现接口
var _ realmif.RealmAccessController = (*AccessController)(nil)

