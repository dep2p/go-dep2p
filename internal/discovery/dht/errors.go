package dht

import (
	"errors"
	"fmt"
)

// 预定义错误
var (
	// ErrKeyNotFound 键未找到
	ErrKeyNotFound = errors.New("dht: key not found")

	// ErrNoNodes 没有可用节点
	ErrNoNodes = errors.New("dht: no nodes available")

	// ErrDHTClosed DHT 已关闭
	ErrDHTClosed = errors.New("dht: DHT is closed")

	// ErrAlreadyStarted DHT 已启动
	ErrAlreadyStarted = errors.New("dht: DHT already started")

	// ErrNotStarted DHT 未启动
	ErrNotStarted = errors.New("dht: DHT not started")

	// ErrInvalidConfig 无效配置
	ErrInvalidConfig = errors.New("dht: invalid config")

	// ErrNilHost Host 为空
	ErrNilHost = errors.New("dht: host is nil")

	// ErrNetworkClosed 网络已关闭
	ErrNetworkClosed = errors.New("dht: network adapter is closed")

	// ErrSendFailed 发送失败
	ErrSendFailed = errors.New("dht: failed to send message")

	// ErrTimeout 超时
	ErrTimeout = errors.New("dht: request timeout")

	// ErrInvalidResponse 无效响应
	ErrInvalidResponse = errors.New("dht: invalid response")

	// ErrPeerNotFound 节点未找到
	ErrPeerNotFound = errors.New("dht: peer not found")

	// ErrNoNearbyPeers 没有附近节点
	ErrNoNearbyPeers = errors.New("dht: no nearby peers")

	// ErrInvalidKey 无效键
	ErrInvalidKey = errors.New("dht: invalid key")

	// ErrInvalidValue 无效值
	ErrInvalidValue = errors.New("dht: invalid value")
)

// Layer1 安全验证错误
var (
	// ErrNodeIDMismatch NodeID 不匹配
	ErrNodeIDMismatch = errors.New("dht: node ID mismatch")

	// ErrSenderMismatch 发送者身份不匹配
	ErrSenderMismatch = errors.New("dht: sender identity mismatch")

	// ErrRecordExpired 记录已过期
	ErrRecordExpired = errors.New("dht: record expired")

	// ErrSeqnoRollback Seqno 回滚
	ErrSeqnoRollback = errors.New("dht: seqno rollback")

	// ErrRateLimitExceeded 速率限制超限
	ErrRateLimitExceeded = errors.New("dht: rate limit exceeded")

	// ErrInvalidAddress 无效地址
	ErrInvalidAddress = errors.New("dht: invalid address format")

	// ErrUnroutableAddress 不可路由地址
	ErrUnroutableAddress = errors.New("dht: unroutable address")

	// ErrInvalidPort 无效端口
	ErrInvalidPort = errors.New("dht: invalid port number")

	// ErrNotMultiaddr 非 multiaddr 格式
	ErrNotMultiaddr = errors.New("dht: address must be multiaddr format")

	// ErrMissingTransport 缺少传输协议
	ErrMissingTransport = errors.New("dht: multiaddr must include transport protocol")

	// ErrInvalidSignature 无效签名
	ErrInvalidSignature = errors.New("dht: invalid signature")
)

// PeerRecord 相关错误（v2.0 新增）
var (
	// ErrNilPeerRecord 空节点记录
	ErrNilPeerRecord = errors.New("dht: nil peer record")

	// ErrSeqTooOld 序列号过旧
	ErrSeqTooOld = errors.New("dht: sequence number too old")

	// ErrRealmIDMismatch RealmID 不匹配
	ErrRealmIDMismatch = errors.New("dht: realm ID mismatch")

	// ErrInvalidSeq 无效的序列号
	ErrInvalidSeq = errors.New("dht: sequence number must be > 0")
)

// DHTError DHT 错误类型
type DHTError struct {
	Op      string // 操作名称
	Err     error  // 底层错误
	Message string // 错误消息
}

// Error 实现 error 接口
func (e *DHTError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("dht %s: %s: %v", e.Op, e.Message, e.Err)
	}
	return fmt.Sprintf("dht %s: %v", e.Op, e.Err)
}

// Unwrap 实现错误解包
func (e *DHTError) Unwrap() error {
	return e.Err
}

// NewDHTError 创建 DHT 错误
func NewDHTError(op string, err error, message string) *DHTError {
	return &DHTError{
		Op:      op,
		Err:     err,
		Message: message,
	}
}
