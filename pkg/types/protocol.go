// Package types 定义 DeP2P 公共类型
//
// 本文件定义协议相关类型。
package types

import (
	"strings"
)

// ProtocolID 协议标识符
type ProtocolID string

// String 返回协议 ID 的字符串表示
func (p ProtocolID) String() string {
	return string(p)
}

// IsEmpty 检查协议 ID 是否为空
func (p ProtocolID) IsEmpty() bool {
	return p == ""
}

// Version 返回协议版本
func (p ProtocolID) Version() string {
	parts := strings.Split(string(p), "/")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return ""
}

// Name 返回协议名称（不含版本）
func (p ProtocolID) Name() string {
	s := string(p)
	lastSlash := strings.LastIndex(s, "/")
	if lastSlash > 0 {
		return s[:lastSlash]
	}
	return s
}

// IsSystem 检查是否为系统协议
// 系统协议包括 /dep2p/sys/* 和 /dep2p/relay/*（Relay 是系统协议特例）
func (p ProtocolID) IsSystem() bool {
	s := string(p)
	return strings.HasPrefix(s, "/dep2p/sys/") || strings.HasPrefix(s, "/dep2p/relay/")
}

// IsRealm 检查是否为 Realm 协议
func (p ProtocolID) IsRealm() bool {
	return strings.HasPrefix(string(p), "/dep2p/realm/")
}

// IsApp 检查是否为应用协议
func (p ProtocolID) IsApp() bool {
	return strings.HasPrefix(string(p), "/dep2p/app/")
}

// RealmID 如果是 Realm 或 App 协议，返回 RealmID
func (p ProtocolID) RealmID() string {
	s := string(p)
	if strings.HasPrefix(s, "/dep2p/realm/") {
		rest := s[len("/dep2p/realm/"):]
		if idx := strings.Index(rest, "/"); idx > 0 {
			return rest[:idx]
		}
	}
	if strings.HasPrefix(s, "/dep2p/app/") {
		rest := s[len("/dep2p/app/"):]
		if idx := strings.Index(rest, "/"); idx > 0 {
			return rest[:idx]
		}
	}
	return ""
}

// 协议前缀常量
//
// Deprecated: 请使用 github.com/dep2p/go-dep2p/pkg/protocol 包中的常量
const (
	// ProtocolPrefixSys 系统协议前缀
	// Deprecated: 使用 protocol.PrefixSys
	ProtocolPrefixSys = "/dep2p/sys"

	// ProtocolPrefixRealm Realm 协议前缀
	// Deprecated: 使用 protocol.PrefixRealm
	ProtocolPrefixRealm = "/dep2p/realm"

	// ProtocolPrefixApp App 协议前缀
	// Deprecated: 使用 protocol.PrefixApp
	ProtocolPrefixApp = "/dep2p/app"
)

// 系统协议常量已移至 github.com/dep2p/go-dep2p/pkg/protocol 包
// 以下为向后兼容的别名，请迁移到新包
//
// 迁移指南:
//   types.ProtocolPing → protocol.Ping
//   types.ProtocolDHT  → protocol.DHT
//   types.ProtocolRelayHop → protocol.RelayHop

// BuildRealmProtocolID 构建 Realm 协议 ID
//
// Deprecated: 请使用 protocol.BuildRealmProtocol 或 protocol.NewRealmBuilder
func BuildRealmProtocolID(realmID, proto, version string) ProtocolID {
	return ProtocolID("/dep2p/realm/" + realmID + "/" + proto + "/" + version)
}

// BuildAppProtocolID 构建应用协议 ID
//
// Deprecated: 请使用 protocol.BuildAppProtocol 或 protocol.NewAppBuilder
func BuildAppProtocolID(realmID, proto, version string) ProtocolID {
	return ProtocolID("/dep2p/app/" + realmID + "/" + proto + "/" + version)
}

// ProtocolNegotiation 协议协商结果
type ProtocolNegotiation struct {
	// Selected 选中的协议
	Selected ProtocolID

	// Candidates 候选协议列表
	Candidates []ProtocolID
}
