package member

import "errors"

var (
	// ErrMemberNotFound 成员不存在
	ErrMemberNotFound = errors.New("member: member not found")

	// ErrMemberExists 成员已存在
	ErrMemberExists = errors.New("member: member already exists")

	// ErrInvalidMember 无效的成员信息
	ErrInvalidMember = errors.New("member: invalid member info")

	// ErrInvalidPeerID 无效的 PeerID
	ErrInvalidPeerID = errors.New("member: invalid peer ID")

	// ErrInvalidRealmID 无效的 RealmID
	ErrInvalidRealmID = errors.New("member: invalid realm ID")

	// ErrInvalidRole 无效的角色
	ErrInvalidRole = errors.New("member: invalid role")

	// ErrInvalidConfig 无效的配置
	ErrInvalidConfig = errors.New("member: invalid config")

	// ErrCacheFull 缓存已满
	ErrCacheFull = errors.New("member: cache is full")

	// ErrStoreNotFound 存储不存在
	ErrStoreNotFound = errors.New("member: store not found")

	// ErrStoreClosed 存储已关闭
	ErrStoreClosed = errors.New("member: store is closed")

	// ErrSyncFailed 同步失败
	ErrSyncFailed = errors.New("member: sync failed")

	// ErrHeartbeatTimeout 心跳超时
	ErrHeartbeatTimeout = errors.New("member: heartbeat timeout")

	// ErrManagerClosed 管理器已关闭
	ErrManagerClosed = errors.New("member: manager is closed")

	// ErrNotStarted 未启动
	ErrNotStarted = errors.New("member: not started")

	// ErrAlreadyStarted 已经启动
	ErrAlreadyStarted = errors.New("member: already started")
)
