package dns

import "errors"

// 预定义错误
var (
	// ErrInvalidDomain 无效的域名
	ErrInvalidDomain = errors.New("dns: invalid domain")

	// ErrInvalidDNSAddr 无效的 dnsaddr 记录
	ErrInvalidDNSAddr = errors.New("dns: invalid dnsaddr record")

	// ErrMaxDepthExceeded 超过最大递归深度
	ErrMaxDepthExceeded = errors.New("dns: max recursion depth exceeded")

	// ErrNoRecordsFound 未找到 DNS 记录
	ErrNoRecordsFound = errors.New("dns: no DNS records found")

	// ErrInvalidMultiaddr dnsaddr 中的 multiaddr 无效
	ErrInvalidMultiaddr = errors.New("dns: invalid multiaddr in dnsaddr")

	// ErrAlreadyStarted 已启动
	ErrAlreadyStarted = errors.New("dns: already started")

	// ErrNotStarted 未启动
	ErrNotStarted = errors.New("dns: not started")

	// ErrEmptyDomain 空域名
	ErrEmptyDomain = errors.New("dns: empty domain")
)
