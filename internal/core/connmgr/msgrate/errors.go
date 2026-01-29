package msgrate

import "errors"

var (
	// ErrAlreadyTracking 已经在追踪该节点
	ErrAlreadyTracking = errors.New("already tracking peer")

	// ErrNotTracking 没有追踪该节点
	ErrNotTracking = errors.New("not tracking peer")
)
