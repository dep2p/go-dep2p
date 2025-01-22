//go:build gofuzz

package msgio

import "bytes"

// 获取 go-fuzz 工具并构建模糊测试器
// $ go get -u github.com/dvyukov/go-fuzz/...
// $ go-fuzz-build github.com/dep2p/p2plib/msgio

// 在 corpus 目录中放入随机(最好是实际的、结构化的)数据
// $ go-fuzz -bin ./msgio-fuzz -corpus corpus -workdir=wdir -timeout=15

// Fuzz 实现模糊测试函数
// 参数:
//   - data: 输入的字节切片数据
//
// 返回值:
//   - int: 0 表示测试失败,1 表示测试成功
func Fuzz(data []byte) int {
	// 创建一个新的 Reader 对象来读取输入数据
	rc := NewReader(bytes.NewReader(data))
	// 也可以使用 VarintReader:
	// rc := NewVarintReader(bytes.NewReader(data))

	// 尝试读取消息,如果出错则返回 0
	if _, err := rc.ReadMsg(); err != nil {
		return 0
	}

	// 读取成功返回 1
	return 1
}
