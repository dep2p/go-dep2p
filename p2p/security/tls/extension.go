package dep2ptls

// extensionPrefix 定义了用于x509证书的对象标识符前缀
var extensionPrefix = []int{1, 3, 6, 1, 4, 1, 53594}

// getPrefixedExtensionID 返回可用于x509证书的对象标识符
// 参数:
//   - suffix: []int 要追加的后缀标识符
//
// 返回值:
//   - []int 完整的对象标识符
//
// 说明:
//   - 将给定的后缀标识符追加到预定义的前缀后面
func getPrefixedExtensionID(suffix []int) []int {
	// 将后缀追加到前缀后并返回完整的标识符
	return append(extensionPrefix, suffix...)
}

// extensionIDEqual 比较两个扩展标识符是否相等
// 参数:
//   - a: []int 第一个扩展标识符
//   - b: []int 第二个扩展标识符
//
// 返回值:
//   - bool 如果两个标识符完全相等返回true,否则返回false
//
// 说明:
//   - 首先比较长度是否相等
//   - 然后逐个比较每个位置的值是否相等
func extensionIDEqual(a, b []int) bool {
	// 比较长度是否相等
	if len(a) != len(b) {
		return false
	}
	// 逐个比较每个位置的值
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
