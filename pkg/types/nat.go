package types

import "time"

// ============================================================================
//                              Mapping - 端口映射信息
// ============================================================================

// Mapping 端口映射信息
type Mapping struct {
	// Protocol 协议（"tcp" 或 "udp"）
	Protocol string

	// InternalPort 内部端口
	InternalPort int

	// ExternalPort 外部端口
	ExternalPort int

	// ExternalAddr 外部地址（字符串格式）
	ExternalAddr string

	// Description 映射描述
	Description string

	// Expiry 过期时间
	Expiry time.Time
}

// IsExpired 检查映射是否过期
func (m Mapping) IsExpired() bool {
	return time.Now().After(m.Expiry)
}

// TTL 返回剩余有效时间
func (m Mapping) TTL() time.Duration {
	remaining := time.Until(m.Expiry)
	if remaining < 0 {
		return 0
	}
	return remaining
}

