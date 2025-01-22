package dep2phttp

// RoundTripperOption 是一个函数类型,用于配置 RoundTripper 选项
// 参数:
//   - o: roundTripperOpts - RoundTripper 选项
//
// 返回:
//   - roundTripperOpts: 更新后的 RoundTripper 选项
type RoundTripperOption func(o roundTripperOpts) roundTripperOpts

// roundTripperOpts 定义了 RoundTripper 的配置选项
type roundTripperOpts struct {
	// preferHTTPTransport 表示是否优先使用 HTTP 传输
	preferHTTPTransport bool
	// serverMustAuthenticatePeerID 表示是否必须验证服务器的对等节点 ID
	serverMustAuthenticatePeerID bool
}

// PreferHTTPTransport 告诉 RoundTripper 构造器优先使用 HTTP 传输
// (而不是 dep2p 流传输)。这在需要利用 HTTP 缓存等场景下很有用。
// 参数:
//   - o: roundTripperOpts - RoundTripper 选项
//
// 返回:
//   - roundTripperOpts: 更新后的 RoundTripper 选项
func PreferHTTPTransport(o roundTripperOpts) roundTripperOpts {
	// 设置优先使用 HTTP 传输
	o.preferHTTPTransport = true
	return o
}

// ServerMustAuthenticatePeerID 告诉 RoundTripper 构造器必须验证服务器的对等节点 ID。
// 注意:目前这意味着我们不能使用原生 HTTP 传输(HTTP 对等节点 ID 验证尚未实现:
// https://github.com/dep2p/specs/pull/564)。
// 参数:
//   - o: roundTripperOpts - RoundTripper 选项
//
// 返回:
//   - roundTripperOpts: 更新后的 RoundTripper 选项
func ServerMustAuthenticatePeerID(o roundTripperOpts) roundTripperOpts {
	// 设置必须验证服务器的对等节点 ID
	o.serverMustAuthenticatePeerID = true
	return o
}
