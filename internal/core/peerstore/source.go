// Package peerstore 实现节点信息存储
//
// 本文件重新导出 addrbook 包中的地址来源相关类型。
package peerstore

import (
	"time"

	"github.com/dep2p/go-dep2p/internal/core/peerstore/addrbook"
)

// 重新导出地址来源类型，方便外部使用
type (
	// AddressSource 地址来源枚举
	AddressSource = addrbook.AddressSource

	// AddressWithSource 带来源的地址
	AddressWithSource = addrbook.AddressWithSource
)

// 重新导出地址来源常量
const (
	SourceUnknown    = addrbook.SourceUnknown
	SourceManual     = addrbook.SourceManual
	SourcePeerstore  = addrbook.SourcePeerstore
	SourceMemberList = addrbook.SourceMemberList
	SourceRelay      = addrbook.SourceRelay
	SourceDHT        = addrbook.SourceDHT
)

// DefaultTTLForSource 返回不同来源的默认 TTL
func DefaultTTLForSource(source AddressSource) time.Duration {
	switch source {
	case SourceManual:
		return 24 * time.Hour // 手动配置保留较长
	case SourcePeerstore:
		return 1 * time.Hour // 直连地址
	case SourceMemberList:
		return 30 * time.Minute // MemberList 同步
	case SourceRelay:
		return 15 * time.Minute // Relay 查询
	case SourceDHT:
		return 10 * time.Minute // DHT 发现
	default:
		return 10 * time.Minute
	}
}
