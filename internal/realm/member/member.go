package member

import (
	"time"

	"github.com/dep2p/go-dep2p/internal/realm/interfaces"
	pb "github.com/dep2p/go-dep2p/pkg/lib/proto/realm/member"
)

// ============================================================================
//                              成员结构
// ============================================================================

// Member 成员（内部使用，与 manager.go 中的旧定义兼容）
type Member struct {
	// 基础信息
	PeerID  string
	RealmID string
	Role    int

	// 状态信息
	Online   bool
	JoinedAt time.Time
	LastSeen time.Time

	// 网络信息
	Addrs []string

	// 元数据
	Metadata map[string]string

	// 统计信息
	BytesSent     int64
	BytesReceived int64
	MessagesSent  int64
}

// ============================================================================
//                              成员信息转换
// ============================================================================

// ToMemberInfo 转换为接口 MemberInfo
func (m *Member) ToMemberInfo() *interfaces.MemberInfo {
	return &interfaces.MemberInfo{
		PeerID:        m.PeerID,
		RealmID:       m.RealmID,
		Role:          interfaces.Role(m.Role),
		Online:        m.Online,
		JoinedAt:      m.JoinedAt,
		LastSeen:      m.LastSeen,
		Addrs:         m.Addrs,
		Metadata:      m.Metadata,
		BytesSent:     m.BytesSent,
		BytesReceived: m.BytesReceived,
		MessagesSent:  m.MessagesSent,
	}
}

// FromMemberInfo 从接口 MemberInfo 转换
func FromMemberInfo(info *interfaces.MemberInfo) *Member {
	if info == nil {
		return nil
	}

	return &Member{
		PeerID:        info.PeerID,
		RealmID:       info.RealmID,
		Role:          int(info.Role),
		Online:        info.Online,
		JoinedAt:      info.JoinedAt,
		LastSeen:      info.LastSeen,
		Addrs:         info.Addrs,
		Metadata:      info.Metadata,
		BytesSent:     info.BytesSent,
		BytesReceived: info.BytesReceived,
		MessagesSent:  info.MessagesSent,
	}
}

// ============================================================================
//                              Protobuf 转换
// ============================================================================

// ToProto 转换为 Protobuf 格式
func (m *Member) ToProto() *pb.MemberInfo {
	addrs := make([][]byte, len(m.Addrs))
	for i, addr := range m.Addrs {
		addrs[i] = []byte(addr)
	}

	metadata := make(map[string]string)
	for k, v := range m.Metadata {
		metadata[k] = v
	}

	return &pb.MemberInfo{
		PeerId:        []byte(m.PeerID),
		RealmId:       m.RealmID,
		Role:          int32(m.Role),
		Online:        m.Online,
		JoinedAt:      m.JoinedAt.Unix(),
		LastSeen:      m.LastSeen.Unix(),
		Addrs:         addrs,
		Metadata:      metadata,
		BytesSent:     m.BytesSent,
		BytesReceived: m.BytesReceived,
		MessagesSent:  m.MessagesSent,
	}
}

// FromProto 从 Protobuf 格式转换
func FromProto(pbMember *pb.MemberInfo) *Member {
	if pbMember == nil {
		return nil
	}

	addrs := make([]string, len(pbMember.Addrs))
	for i, addr := range pbMember.Addrs {
		addrs[i] = string(addr)
	}

	metadata := make(map[string]string)
	for k, v := range pbMember.Metadata {
		metadata[k] = v
	}

	return &Member{
		PeerID:        string(pbMember.PeerId),
		RealmID:       pbMember.RealmId,
		Role:          int(pbMember.Role),
		Online:        pbMember.Online,
		JoinedAt:      time.Unix(pbMember.JoinedAt, 0),
		LastSeen:      time.Unix(pbMember.LastSeen, 0),
		Addrs:         addrs,
		Metadata:      metadata,
		BytesSent:     pbMember.BytesSent,
		BytesReceived: pbMember.BytesReceived,
		MessagesSent:  pbMember.MessagesSent,
	}
}

// ============================================================================
//                              辅助方法
// ============================================================================

// IsOnline 检查是否在线
func (m *Member) IsOnline() bool {
	return m.Online
}

// IsAdmin 检查是否为管理员
func (m *Member) IsAdmin() bool {
	return m.Role == int(interfaces.RoleAdmin)
}

// IsRelay 检查是否为中继节点
func (m *Member) IsRelay() bool {
	return m.Role == int(interfaces.RoleRelay)
}

// HasRole 检查是否有指定角色
func (m *Member) HasRole(role interfaces.Role) bool {
	return m.Role == int(role)
}

// UpdateLastSeen 更新最后活跃时间
func (m *Member) UpdateLastSeen() {
	m.LastSeen = time.Now()
	m.Online = true
}

// SetOnline 设置在线状态
func (m *Member) SetOnline(online bool) {
	m.Online = online
	if online {
		m.LastSeen = time.Now()
	}
}

// Clone 克隆成员
func (m *Member) Clone() *Member {
	if m == nil {
		return nil
	}

	// 复制地址
	addrs := make([]string, len(m.Addrs))
	copy(addrs, m.Addrs)

	// 复制元数据
	metadata := make(map[string]string)
	for k, v := range m.Metadata {
		metadata[k] = v
	}

	return &Member{
		PeerID:        m.PeerID,
		RealmID:       m.RealmID,
		Role:          m.Role,
		Online:        m.Online,
		JoinedAt:      m.JoinedAt,
		LastSeen:      m.LastSeen,
		Addrs:         addrs,
		Metadata:      metadata,
		BytesSent:     m.BytesSent,
		BytesReceived: m.BytesReceived,
		MessagesSent:  m.MessagesSent,
	}
}
