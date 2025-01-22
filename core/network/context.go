package network

import (
	"context"
	"time"
)

// DialPeerTimeout 是单个 `DialPeer` 调用的默认超时时间。
// 当有多个并发的 `DialPeer` 调用时,此超时时间将独立应用于每个调用。
var DialPeerTimeout = 60 * time.Second

type noDialCtxKey struct{}
type dialPeerTimeoutCtxKey struct{}
type forceDirectDialCtxKey struct{}
type allowLimitedConnCtxKey struct{}
type simConnectCtxKey struct{ isClient bool }

var noDial = noDialCtxKey{}
var forceDirectDial = forceDirectDialCtxKey{}
var allowLimitedConn = allowLimitedConnCtxKey{}
var simConnectIsServer = simConnectCtxKey{}
var simConnectIsClient = simConnectCtxKey{isClient: true}

// EXPERIMENTAL
// WithForceDirectDial 构造一个新的上下文,其中包含一个选项,该选项指示网络尝试通过拨号强制与对等点建立直接连接,即使已存在到该对等点的代理连接。
// 参数:
//   - ctx: 父上下文
//   - reason: 强制直接拨号的原因
//
// 返回:
//   - context.Context: 带有强制直接拨号选项的新上下文
func WithForceDirectDial(ctx context.Context, reason string) context.Context {
	return context.WithValue(ctx, forceDirectDial, reason)
}

// EXPERIMENTAL
// GetForceDirectDial 返回上下文中是否设置了强制直接拨号选项。
// 参数:
//   - ctx: 要检查的上下文
//
// 返回:
//   - forceDirect: 是否设置了强制直接拨号
//   - reason: 设置强制直接拨号的原因
func GetForceDirectDial(ctx context.Context) (forceDirect bool, reason string) {
	v := ctx.Value(forceDirectDial)
	if v != nil {
		return true, v.(string)
	}

	return false, ""
}

// WithSimultaneousConnect 构造一个新的上下文,其中包含一个选项,该选项指示传输在适用的情况下应用打洞逻辑。
// EXPERIMENTAL
// 参数:
//   - ctx: 父上下文
//   - isClient: 是否为客户端
//   - reason: 使用同步连接的原因
//
// 返回:
//   - context.Context: 带有同步连接选项的新上下文
func WithSimultaneousConnect(ctx context.Context, isClient bool, reason string) context.Context {
	if isClient {
		return context.WithValue(ctx, simConnectIsClient, reason)
	}
	return context.WithValue(ctx, simConnectIsServer, reason)
}

// GetSimultaneousConnect 返回上下文中是否设置了同步连接选项。
// EXPERIMENTAL
// 参数:
//   - ctx: 要检查的上下文
//
// 返回:
//   - simconnect: 是否设置了同步连接
//   - isClient: 是否为客户端
//   - reason: 使用同步连接的原因
func GetSimultaneousConnect(ctx context.Context) (simconnect bool, isClient bool, reason string) {
	if v := ctx.Value(simConnectIsClient); v != nil {
		return true, true, v.(string)
	}
	if v := ctx.Value(simConnectIsServer); v != nil {
		return true, false, v.(string)
	}
	return false, false, ""
}

// WithNoDial 构造一个新的上下文,其中包含一个选项,该选项指示网络在打开流时不要尝试新的拨号。
// 参数:
//   - ctx: 父上下文
//   - reason: 禁用拨号的原因
//
// 返回:
//   - context.Context: 带有禁用拨号选项的新上下文
func WithNoDial(ctx context.Context, reason string) context.Context {
	return context.WithValue(ctx, noDial, reason)
}

// GetNoDial 返回上下文中是否设置了禁用拨号选项。
// 参数:
//   - ctx: 要检查的上下文
//
// 返回:
//   - nodial: 是否禁用拨号
//   - reason: 禁用拨号的原因
func GetNoDial(ctx context.Context) (nodial bool, reason string) {
	v := ctx.Value(noDial)
	if v != nil {
		return true, v.(string)
	}

	return false, ""
}

// GetDialPeerTimeout 返回当前的 DialPeer 超时时间(或默认值)。
// 参数:
//   - ctx: 要检查的上下文
//
// 返回:
//   - time.Duration: 拨号超时时间
func GetDialPeerTimeout(ctx context.Context) time.Duration {
	if to, ok := ctx.Value(dialPeerTimeoutCtxKey{}).(time.Duration); ok {
		return to
	}
	return DialPeerTimeout
}

// WithDialPeerTimeout 返回一个应用了 DialPeer 超时时间的新上下文。
//
// 此超时时间会覆盖默认的 DialPeerTimeout,并独立应用于每个拨号。
// 参数:
//   - ctx: 父上下文
//   - timeout: 要设置的超时时间
//
// 返回:
//   - context.Context: 带有拨号超时选项的新上下文
func WithDialPeerTimeout(ctx context.Context, timeout time.Duration) context.Context {
	return context.WithValue(ctx, dialPeerTimeoutCtxKey{}, timeout)
}

// WithAllowLimitedConn 构造一个新的上下文,其中包含一个选项,该选项指示
// 网络在打开新流时可以使用受限连接。
// 参数:
//   - ctx: 父上下文
//   - reason: 允许受限连接的原因
//
// 返回:
//   - context.Context: 带有允许受限连接选项的新上下文
func WithAllowLimitedConn(ctx context.Context, reason string) context.Context {
	return context.WithValue(ctx, allowLimitedConn, reason)
}

// WithUseTransient 构造一个新的上下文,其中包含一个选项,该选项指示网络
// 在打开新流时可以使用临时连接。
//
// 已弃用: 请改用 WithAllowLimitedConn。
// 参数:
//   - ctx: 父上下文
//   - reason: 使用临时连接的原因
//
// 返回:
//   - context.Context: 带有允许临时连接选项的新上下文
func WithUseTransient(ctx context.Context, reason string) context.Context {
	return context.WithValue(ctx, allowLimitedConn, reason)
}

// GetAllowLimitedConn 返回上下文中是否设置了允许受限连接选项。
// 参数:
//   - ctx: 要检查的上下文
//
// 返回:
//   - usetransient: 是否允许受限连接
//   - reason: 允许受限连接的原因
func GetAllowLimitedConn(ctx context.Context) (usetransient bool, reason string) {
	v := ctx.Value(allowLimitedConn)
	if v != nil {
		return true, v.(string)
	}
	return false, ""
}

// GetUseTransient 返回上下文中是否设置了使用临时连接选项。
//
// 已弃用: 请改用 GetAllowLimitedConn。
// 参数:
//   - ctx: 要检查的上下文
//
// 返回:
//   - usetransient: 是否允许临时连接
//   - reason: 允许临时连接的原因
func GetUseTransient(ctx context.Context) (usetransient bool, reason string) {
	v := ctx.Value(allowLimitedConn)
	if v != nil {
		return true, v.(string)
	}
	return false, ""
}
