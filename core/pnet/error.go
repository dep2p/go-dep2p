package pnet

// ErrNotInPrivateNetwork 当 dep2p 在设置了 ForcePrivateNetwork 但没有 PNet Protector 的情况下尝试拨号时返回此错误
var ErrNotInPrivateNetwork = NewError("未配置私有网络,但环境要求必须使用私有网络")

// Error 是一个接口类型，用于简化 PNet 错误的检测
type Error interface {
	// IsPNetError 检查是否为 PNet 错误
	IsPNetError() bool
}

// NewError 创建一个新的 Error 实例
// 参数：
//   - err: string 错误信息字符串
//
// 返回值：
//   - error: 包装后的 PNet 错误
func NewError(err string) error {
	return pnetErr("privnet: " + err)
}

// IsPNetError 检查给定的错误是否为 PNet 错误
// 参数：
//   - err: error 需要检查的错误对象
//
// 返回值：
//   - bool: 如果是 PNet 错误则返回 true，否则返回 false
func IsPNetError(err error) bool {
	v, ok := err.(Error)
	return ok && v.IsPNetError()
}

// pnetErr 是实现了 Error 接口的字符串类型
type pnetErr string

// 确保 pnetErr 类型实现了 Error 接口
var _ Error = (*pnetErr)(nil)

// Error 实现 error 接口，返回错误信息字符串
// 返回值：
//   - string: 错误信息
func (p pnetErr) Error() string {
	return string(p)
}

// IsPNetError 实现 Error 接口，标识这是一个 PNet 错误
// 返回值：
//   - bool: 始终返回 true
func (pnetErr) IsPNetError() bool {
	return true
}
