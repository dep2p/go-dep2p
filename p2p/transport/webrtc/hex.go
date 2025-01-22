package dep2pwebrtc

// 此文件中的代码改编自 Go 标准库的 hex 包。
// 源自 https://cs.opensource.google/go/go/+/refs/tags/go1.20.2:src/encoding/hex/hex.go
//
// 我们修改原始代码的原因是为了能够在进行十六进制编码/解码的同时处理分隔符要求，而不必分两步进行。

import (
	"encoding/hex"
	"errors"
)

// encodeInterspersedHex 将字节切片编码为十六进制字符串，每个编码字节之间用冒号(':')分隔。
//
// 参数:
//   - src: []byte 待编码的字节切片
//
// 返回值:
//   - string 编码后的十六进制字符串
//
// 示例: { 0x01, 0x02, 0x03 } -> "01:02:03"
func encodeInterspersedHex(src []byte) string {
	// 如果输入为空，返回空字符串
	if len(src) == 0 {
		return ""
	}
	// 先将字节切片编码为普通的十六进制字符串
	s := hex.EncodeToString(src)
	n := len(s)
	// 计算需要插入的冒号数量
	colons := n / 2
	if n%2 == 0 {
		colons--
	}
	// 创建足够大的缓冲区以容纳结果
	buffer := make([]byte, n+colons)

	// 将十六进制字符和冒号交替复制到缓冲区
	for i, j := 0, 0; i < n; i, j = i+2, j+3 {
		copy(buffer[j:j+2], s[i:i+2])
		if j+3 < len(buffer) {
			buffer[j+2] = ':'
		}
	}
	return string(buffer)
}

var errUnexpectedIntersperseHexChar = errors.New("分隔十六进制字符串中出现意外字符")

// decodeInterspersedHexFromASCIIString 解码一个ASCII十六进制字符串为字节切片，其中十六进制字符预期用冒号(':')分隔。
//
// 参数:
//   - s: string 待解码的ASCII字符串
//
// 返回值:
//   - []byte 解码后的字节切片
//   - error 解码过程中的错误
//
// 注意:
//   - 如果输入字符串包含非ASCII字符，此函数将返回错误
//
// 示例: "01:02:03" -> { 0x01, 0x02, 0x03 }
func decodeInterspersedHexFromASCIIString(s string) ([]byte, error) {
	// 获取输入字符串长度
	n := len(s)
	// 创建缓冲区存储去除冒号后的十六进制字符
	buffer := make([]byte, n/3*2+n%3)
	j := 0
	// 遍历输入字符串，去除冒号并验证格式
	for i := 0; i < n; i++ {
		if i%3 == 2 {
			if s[i] != ':' {
				log.Debugf("分隔十六进制字符串中出现意外字符: %s", s[i])
				return nil, errUnexpectedIntersperseHexChar
			}
		} else {
			if s[i] == ':' {
				log.Debugf("分隔十六进制字符串中出现意外字符: %s", s[i])
				return nil, errUnexpectedIntersperseHexChar
			}
			buffer[j] = s[i]
			j++
		}
	}
	// 创建目标缓冲区并解码十六进制字符
	dst := make([]byte, hex.DecodedLen(len(buffer)))
	if _, err := hex.Decode(dst, buffer); err != nil {
		log.Debugf("解码十六进制字符串时出错: %s", err)
		return nil, err
	}
	return dst, nil
}
