package autonatv2

import (
	"io"

	"github.com/dep2p/go-dep2p/multiformats/varint"
)

// msgReader 从 R 中读取一个 varint 前缀的消息，不使用任何缓冲
type msgReader struct {
	R   io.Reader // 底层读取器
	Buf []byte    // 用于存储读取数据的缓冲区
}

// ReadByte 实现了 io.ByteReader 接口，读取单个字节
// 返回值:
//   - byte: 读取的字节
//   - error: 读取过程中的错误
func (m *msgReader) ReadByte() (byte, error) {
	buf := m.Buf[:1]        // 使用缓冲区的第一个字节
	_, err := m.R.Read(buf) // 从底层读取器读取一个字节
	return buf[0], err
}

// ReadMsg 读取一个完整的消息
// 返回值:
//   - []byte: 读取的消息内容
//   - error: 读取过程中的错误，可能是:
//   - io.ErrShortBuffer: 消息大小超过缓冲区容量
//   - 其他IO错误
func (m *msgReader) ReadMsg() ([]byte, error) {
	// 读取消息长度的 varint 编码
	sz, err := varint.ReadUvarint(m)
	if err != nil {
		log.Debugf("读取消息长度失败: %v", err)
		return nil, err
	}

	// 检查消息是否超过缓冲区大小
	if sz > uint64(len(m.Buf)) {
		log.Debugf("消息大小超过缓冲区容量: %d > %d", sz, len(m.Buf))
		return nil, io.ErrShortBuffer
	}

	// 读取完整消息内容
	n := 0
	for n < int(sz) {
		nr, err := m.R.Read(m.Buf[n:sz])
		if err != nil {
			log.Debugf("读取消息内容失败: %v", err)
			return nil, err
		}
		n += nr
	}
	return m.Buf[:sz], nil
}
