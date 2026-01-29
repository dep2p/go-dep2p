package multiaddr

import "errors"

// 通用错误
var (
	ErrInvalidMultiaddr = errors.New("invalid multiaddr")
	ErrInvalidProtocol  = errors.New("invalid protocol")
	ErrUnmarshalFailed  = errors.New("failed to unmarshal multiaddr")
)
