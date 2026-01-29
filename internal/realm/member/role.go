package member

import (
	"github.com/dep2p/go-dep2p/internal/realm/interfaces"
)

// ============================================================================
//                              角色管理
// ============================================================================

// Permission 权限定义
type Permission int

const (
	// PermRead 读取权限
	PermRead Permission = iota
	// PermWrite 写入权限
	PermWrite
	// PermAdmin 管理权限
	PermAdmin
	// PermRelay 中继权限
	PermRelay
)

// String 返回权限名称
func (p Permission) String() string {
	switch p {
	case PermRead:
		return "Read"
	case PermWrite:
		return "Write"
	case PermAdmin:
		return "Admin"
	case PermRelay:
		return "Relay"
	default:
		return "Unknown"
	}
}

// ============================================================================
//                              角色权限检查
// ============================================================================

// HasPermission 检查成员是否有指定权限
func HasPermission(member *interfaces.MemberInfo, perm Permission) bool {
	if member == nil {
		return false
	}

	switch perm {
	case PermRead:
		// 所有成员都有读取权限
		return true

	case PermWrite:
		// 所有成员都有写入权限
		return true

	case PermAdmin:
		// 只有管理员有管理权限
		return member.Role == interfaces.RoleAdmin

	case PermRelay:
		// 只有中继节点有中继权限
		return member.Role == interfaces.RoleRelay

	default:
		return false
	}
}

// RequireAdmin 要求管理员权限
func RequireAdmin(member *interfaces.MemberInfo) error {
	if member == nil {
		return ErrInvalidMember
	}

	if member.Role != interfaces.RoleAdmin {
		return ErrInvalidRole
	}

	return nil
}

// RequireRelay 要求中继权限
func RequireRelay(member *interfaces.MemberInfo) error {
	if member == nil {
		return ErrInvalidMember
	}

	if member.Role != interfaces.RoleRelay {
		return ErrInvalidRole
	}

	return nil
}

// IsAdmin 检查是否为管理员
func IsAdmin(member *interfaces.MemberInfo) bool {
	return member != nil && member.Role == interfaces.RoleAdmin
}

// IsRelay 检查是否为中继节点
func IsRelay(member *interfaces.MemberInfo) bool {
	return member != nil && member.Role == interfaces.RoleRelay
}

// IsMember 检查是否为普通成员
func IsMember(member *interfaces.MemberInfo) bool {
	return member != nil && member.Role == interfaces.RoleMember
}

// CanManageMembers 检查是否可以管理成员
func CanManageMembers(member *interfaces.MemberInfo) bool {
	return IsAdmin(member)
}

// CanRelay 检查是否可以中继
func CanRelay(member *interfaces.MemberInfo) bool {
	return IsRelay(member) || IsAdmin(member)
}
