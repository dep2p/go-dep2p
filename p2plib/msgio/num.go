package msgio

import (
	"encoding/binary"
	"io"
)

// NBO 是网络字节序
var NBO = binary.BigEndian

// WriteLen 将长度写入指定的写入器
// 参数:
//   - w: 写入器接口
//   - l: 要写入的长度值
//
// 返回值:
//   - error: 写入过程中的错误信息
func WriteLen(w io.Writer, l int) error {
	// 将int类型转换为uint32类型
	ul := uint32(l)
	// 使用网络字节序写入长度值
	return binary.Write(w, NBO, &ul)
}

// ReadLen 从指定的读取器中读取长度
// 参数:
//   - r: 读取器接口
//   - buf: 用于存储读取数据的缓冲区，如果为nil则创建新的缓冲区
//
// 返回值:
//   - int: 读取到的长度值
//   - error: 读取过程中的错误信息
//
// 示例:
//
//	l, err := ReadLen(r, nil)
//	_, err := ReadLen(r, buf)
func ReadLen(r io.Reader, buf []byte) (int, error) {
	// 确保缓冲区至少有4字节大小
	if len(buf) < 4 {
		buf = make([]byte, 4)
	}
	buf = buf[:4]

	// 读取4字节的长度值
	if _, err := io.ReadFull(r, buf); err != nil {
		return 0, err
	}

	// 将字节序列转换为整数
	n := int(NBO.Uint32(buf))
	return n, nil
}
