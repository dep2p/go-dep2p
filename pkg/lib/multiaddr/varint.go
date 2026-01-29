package multiaddr

import (
	"encoding/binary"
	"errors"
	"math"
)

// Varint 编解码错误
var (
	ErrVarintOverflow = errors.New("varint: value overflows uint64")
	ErrVarintTooShort = errors.New("varint: buffer too short")
	ErrVarintTooBig   = errors.New("varint: varint too big for uint64")
)

// codeToVarint 将协议代码转换为 varint 编码的字节
func codeToVarint(code int) []byte {
	if code < 0 || code > math.MaxInt32 {
		panic("invalid protocol code")
	}
	return uvarintEncode(uint64(code))
}

// readVarintCode 从字节流中读取 varint 编码的协议代码
// 返回：(code, bytes_read, error)
func readVarintCode(buf []byte) (int, int, error) {
	code, n, err := uvarintDecode(buf)
	if err != nil {
		return 0, 0, err
	}
	if code > math.MaxInt32 {
		// 我们只允许 32位代码
		return 0, 0, ErrVarintOverflow
	}
	return int(code), n, nil
}

// uvarintEncode 编码无符号 varint
// 使用 protobuf 的 varint 编码格式
func uvarintEncode(x uint64) []byte {
	buf := make([]byte, binary.MaxVarintLen64)
	n := binary.PutUvarint(buf, x)
	return buf[:n]
}

// uvarintDecode 解码无符号 varint
// 返回：(value, bytes_read, error)
func uvarintDecode(buf []byte) (uint64, int, error) {
	x, n := binary.Uvarint(buf)
	if n == 0 {
		return 0, 0, ErrVarintTooShort
	}
	if n < 0 {
		return 0, 0, ErrVarintTooBig
	}
	return x, n, nil
}
