package resourcemgr

import "errors"

var (
	// ErrResourceLimitExceeded 资源限制超出错误
	ErrResourceLimitExceeded = errors.New("resource limit exceeded")

	// ErrResourceScopeClosed 资源作用域已关闭错误
	ErrResourceScopeClosed = errors.New("resource scope closed")
)
