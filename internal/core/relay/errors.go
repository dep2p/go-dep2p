// Package relay 提供中继服务模块的实现
package relay

import (
	"errors"

	"github.com/dep2p/go-dep2p/pkg/protocolids"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              协议 ID
// ============================================================================

// 引用 pkg/protocolids 唯一真源
const (
	// ProtocolRelay 中继协议 (v1.1 scope: sys)
	ProtocolRelay types.ProtocolID = protocolids.SysRelay

	// ProtocolRelayHop 中继跳转协议 (v1.1 scope: sys)
	ProtocolRelayHop types.ProtocolID = protocolids.SysRelayHop

	// ProtocolRelayStop 中继停止协议 (v1.1 scope: sys)
	ProtocolRelayStop types.ProtocolID = protocolids.SysRelayStop
)

// ============================================================================
//                              消息类型
// ============================================================================

const (
	MsgTypeReserve uint8 = iota + 1
	MsgTypeReserveOK
	MsgTypeReserveError
	MsgTypeConnect
	MsgTypeConnectOK
	MsgTypeConnectError
	MsgTypeStatus
)

// ============================================================================
//                              错误定义
// ============================================================================

var (
	// ErrRelayNotFound 中继未找到
	ErrRelayNotFound = errors.New("relay not found")

	// ErrReservationFailed 预留失败
	ErrReservationFailed = errors.New("reservation failed")

	// ErrConnectFailed 中继连接失败
	ErrConnectFailed = errors.New("relay connect failed")

	// ErrRelayBusy 中继繁忙
	ErrRelayBusy = errors.New("relay is busy")

	// ErrDestNotConnected 目标未连接到中继
	ErrDestNotConnected = errors.New("destination not connected to relay")

	// ErrNoRelaysAvailable 没有可用中继
	ErrNoRelaysAvailable = errors.New("no relays available")

	// ============================================================================
	//                              IMPL-1227: Realm Relay 错误
	// ============================================================================

	// ErrInvalidProof 无效的成员证明
	ErrInvalidProof = errors.New("invalid membership proof")

	// ErrNotMember 非 Realm 成员
	ErrNotMember = errors.New("not a realm member")

	// ErrProtocolNotAllowed 协议不允许
	ErrProtocolNotAllowed = errors.New("protocol not allowed on this relay")

	// ErrRealmMismatch Realm 不匹配
	ErrRealmMismatch = errors.New("realm ID mismatch")

	// ErrPSKAuthRequired 需要 PSK 认证
	ErrPSKAuthRequired = errors.New("PSK authentication required for realm relay")
)

