package dep2p

import "errors"

// 公共错误定义
var (
	// ────────────────────────────────────────────────────────────────────────
	// 节点生命周期错误
	// ────────────────────────────────────────────────────────────────────────

	// ErrNotStarted 节点未启动
	ErrNotStarted = errors.New("node not started")

	// ErrAlreadyStarted 节点已启动
	ErrAlreadyStarted = errors.New("node already started")

	// ErrNodeClosed 节点已关闭
	ErrNodeClosed = errors.New("node closed")

	// ────────────────────────────────────────────────────────────────────────
	// Realm 相关错误
	// ────────────────────────────────────────────────────────────────────────

	// ErrNotInRealm 未加入 Realm
	ErrNotInRealm = errors.New("not in any realm")

	// ErrAlreadyInRealm 已在 Realm 中
	ErrAlreadyInRealm = errors.New("already in a realm")

	// ErrInvalidRealmKey 无效的 Realm 密钥
	ErrInvalidRealmKey = errors.New("invalid realm key")

	// ────────────────────────────────────────────────────────────────────────
	// 网络相关错误
	// ────────────────────────────────────────────────────────────────────────

	// ErrConnectionFailed 连接失败
	ErrConnectionFailed = errors.New("connection failed")

	// ErrPeerNotFound 节点未找到
	ErrPeerNotFound = errors.New("peer not found")
)
