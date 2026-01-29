package connmgr

import "errors"

// 连接管理器错误定义
var (
	// ErrConnectionDenied 连接被拒绝
	ErrConnectionDenied = errors.New("connmgr: connection denied")

	// ErrPeerBlocked 节点被阻止
	ErrPeerBlocked = errors.New("connmgr: peer blocked")

	// ErrInvalidConfig 配置无效
	ErrInvalidConfig = errors.New("connmgr: invalid config")

	// ErrManagerClosed 管理器已关闭
	ErrManagerClosed = errors.New("connmgr: manager closed")

	// ErrNoHost 未设置 Host
	ErrNoHost = errors.New("connmgr: no host set")
)
