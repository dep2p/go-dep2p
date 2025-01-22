package protocol

// ID 是一个用于在流中写入协议头的标识符类型
type ID string

// 预定义的保留协议 ID 常量
const (
	TestingID ID = "/p2p/_testing" // 用于测试目的的协议 ID
)

// ConvertFromStrings 将字符串切片转换为协议 ID 切片
// 参数：
//   - ids: []string 需要转换的字符串切片
//
// 返回值：
//   - []ID: 转换后的协议 ID 切片
func ConvertFromStrings(ids []string) (res []ID) {
	// 创建一个与输入切片容量相同的结果切片
	res = make([]ID, 0, len(ids))
	// 遍历输入的字符串切片，将每个字符串转换为 ID 类型
	for _, id := range ids {
		res = append(res, ID(id))
	}
	return res
}

// ConvertToStrings 将协议 ID 切片转换为字符串切片
// 参数：
//   - ids: []ID 需要转换的协议 ID 切片
//
// 返回值：
//   - []string: 转换后的字符串切片
func ConvertToStrings(ids []ID) (res []string) {
	// 创建一个与输入切片容量相同的结果切片
	res = make([]string, 0, len(ids))
	// 遍历输入的 ID 切片，将每个 ID 转换为字符串类型
	for _, id := range ids {
		res = append(res, string(id))
	}
	return res
}
