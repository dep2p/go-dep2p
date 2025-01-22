package msgio

import (
	"bytes"
	"testing"
)

// TestLimitReader 测试带限制的读取器功能
// 参数:
//   - t: 测试用例上下文
func TestLimitReader(t *testing.T) {
	// 创建一个空的字节缓冲区
	buf := bytes.NewBuffer(nil)
	// 创建一个限制为0的读取器
	reader, _ := LimitedReader(buf)
	// 尝试读取空字节切片
	n, err := reader.Read([]byte{})
	// 验证读取结果是否符合预期:读取字节数为0且返回EOF错误
	if n != 0 || err.Error() != "EOF" {
		t.Fatal("Expected not to read anything")
	}
}

// TestLimitWriter 测试带限制的写入器功能
// 参数:
//   - t: 测试用例上下文
func TestLimitWriter(t *testing.T) {
	// 创建一个空的字节缓冲区
	buf := bytes.NewBuffer(nil)
	// 创建一个新的限制写入器
	writer := NewLimitedWriter(buf)
	// 写入3个字节的数据
	n, err := writer.Write([]byte{1, 2, 3})
	// 验证写入是否成功:写入3个字节且无错误
	if n != 3 || err != nil {
		t.Fatal("Expected to write 3 bytes with no errors")
	}
	// 刷新写入器缓冲区
	err = writer.Flush()
	// 检查刷新操作是否出错
	if err != nil {
		t.Fatal(err)
	}
}
