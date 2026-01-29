package protocol

import (
	"errors"
	"strings"
)

// 验证错误
var (
	// ErrInvalidProtocol 无效的协议 ID
	ErrInvalidProtocol = errors.New("invalid protocol ID")

	// ErrEmptyProtocol 空协议 ID
	ErrEmptyProtocol = errors.New("empty protocol ID")

	// ErrInvalidPrefix 无效的协议前缀
	ErrInvalidPrefix = errors.New("invalid protocol prefix")

	// ErrMissingVersion 缺少版本号
	ErrMissingVersion = errors.New("missing protocol version")
)

// IsSystem 检查是否为系统协议
// 系统协议包括 /dep2p/sys/* 和 /dep2p/relay/*
func IsSystem(id ID) bool {
	s := string(id)
	return strings.HasPrefix(s, PrefixSys+"/") || strings.HasPrefix(s, PrefixRelay+"/")
}

// IsRelay 检查是否为 Relay 协议
func IsRelay(id ID) bool {
	return strings.HasPrefix(string(id), PrefixRelay+"/")
}

// IsRealm 检查是否为 Realm 协议
func IsRealm(id ID) bool {
	return strings.HasPrefix(string(id), PrefixRealm+"/")
}

// IsApp 检查是否为 App 协议
func IsApp(id ID) bool {
	return strings.HasPrefix(string(id), PrefixApp+"/")
}

// Validate 验证协议 ID 格式
// 返回 nil 表示格式正确
func Validate(id ID) error {
	s := string(id)

	// 检查空值
	if s == "" {
		return ErrEmptyProtocol
	}

	// 检查前缀
	if !strings.HasPrefix(s, "/dep2p/") {
		return ErrInvalidPrefix
	}

	// 检查路径段数量
	parts := strings.Split(s, "/")
	// 格式: /dep2p/type/...，至少 4 段（空, dep2p, type, protocol）
	if len(parts) < 4 {
		return ErrInvalidProtocol
	}

	// 检查类型
	protocolType := parts[2]
	switch protocolType {
	case "sys":
		// /dep2p/sys/<protocol>/<version>
		if len(parts) < 5 {
			return ErrMissingVersion
		}
	case "relay":
		// /dep2p/relay/<version>/{hop,stop}
		if len(parts) < 5 {
			return ErrInvalidProtocol
		}
	case "realm", "app":
		// /dep2p/realm/<realmID>/<protocol>/<version>
		// /dep2p/app/<realmID>/<protocol>/<version>
		if len(parts) < 6 {
			return ErrMissingVersion
		}
	default:
		return ErrInvalidPrefix
	}

	return nil
}

// ExtractRealmID 从 Realm/App 协议中提取 RealmID
// 如果不是 Realm/App 协议，返回空字符串
func ExtractRealmID(id ID) string {
	s := string(id)
	if strings.HasPrefix(s, PrefixRealm+"/") {
		rest := s[len(PrefixRealm)+1:]
		if idx := strings.Index(rest, "/"); idx > 0 {
			return rest[:idx]
		}
	}
	if strings.HasPrefix(s, PrefixApp+"/") {
		rest := s[len(PrefixApp)+1:]
		if idx := strings.Index(rest, "/"); idx > 0 {
			return rest[:idx]
		}
	}
	return ""
}

// ExtractName 从协议 ID 中提取协议名称（不含版本）
func ExtractName(id ID) string {
	s := string(id)
	lastSlash := strings.LastIndex(s, "/")
	if lastSlash > 0 {
		return s[:lastSlash]
	}
	return s
}

// ExtractVersion 从协议 ID 中提取版本号
func ExtractVersion(id ID) string {
	s := string(id)
	lastSlash := strings.LastIndex(s, "/")
	if lastSlash > 0 && lastSlash < len(s)-1 {
		return s[lastSlash+1:]
	}
	return ""
}

// Match 检查两个协议是否匹配（忽略版本）
func Match(a, b ID) bool {
	return ExtractName(a) == ExtractName(b)
}
