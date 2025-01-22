// Package protoio 提供了带有 varint 前缀的 protobuf 消息读写功能。
// 本代码改编自 gogo/protobuf，使用 multiformats/go-varint 实现高效、可互操作的长度前缀。
//
// # Go 语言的 Protocol Buffers 增强版
//
// 版权所有 (c) 2013, GoGo 作者保留所有权利。
// http://github.com/gogo/protobuf
//
// 在遵循以下条件的情况下，允许以源代码和二进制形式重新分发和使用，无论是否修改:
//
//   - 源代码的重新分发必须保留上述版权声明
//
// 本声明、以下条件和免责声明。
//   - 二进制形式的重新分发必须在随分发提供的文档和/或其他材料中复制上述
//
// 版权声明、本条件列表和以下免责声明。
//
// 本软件由版权所有者和贡献者"按原样"提供，不提供任何明示或暗示的保证，包括但不限于对适销性和特定用途适用性的保证。
// 在任何情况下，版权所有者或贡献者均不对任何直接、间接、偶然、特殊、惩戒性或后果性损害(包括但不限于采购替代商品或服务；使用、数据或利润损失；
// 或业务中断)承担责任，无论是基于合同、严格责任或侵权(包括疏忽或其他)的任何责任理论，即使事先被告知可能发生此类损害。
package protoio

import (
	"fmt"
	"io"
	"os"
	"runtime/debug"

	"github.com/gogo/protobuf/proto"

	"github.com/multiformats/go-varint"
)

// uvarintWriter 实现了带有 varint 长度前缀的 protobuf 消息写入器
type uvarintWriter struct {
	w      io.Writer // 底层写入器
	lenBuf []byte    // 用于存储长度前缀的缓冲区
	buffer []byte    // 用于存储消息的缓冲区
}

// NewDelimitedWriter 创建一个新的带分隔符的写入器
// 参数:
//   - w: 底层写入器
//
// 返回值:
//   - WriteCloser: 写入器接口
func NewDelimitedWriter(w io.Writer) WriteCloser {
	return &uvarintWriter{w, make([]byte, varint.MaxLenUvarint63), nil}
}

// WriteMsg 写入一个 protobuf 消息
// 参数:
//   - msg: 要写入的消息对象
//
// 返回值:
//   - error: 错误信息
func (uw *uvarintWriter) WriteMsg(msg proto.Message) (err error) {
	// 设置 panic 恢复
	defer func() {
		if rerr := recover(); rerr != nil {
			fmt.Fprintf(os.Stderr, "caught panic: %s\n%s\n", rerr, debug.Stack())
			err = fmt.Errorf("panic reading message: %s", rerr)
		}
	}()

	var data []byte
	// 尝试使用 MarshalTo 方法优化写入
	if m, ok := msg.(interface {
		MarshalTo(data []byte) (n int, err error)
	}); ok {
		// 获取消息大小
		n, ok := getSize(m)
		if ok {
			// 确保缓冲区足够大
			if n+varint.MaxLenUvarint63 >= len(uw.buffer) {
				uw.buffer = make([]byte, n+varint.MaxLenUvarint63)
			}
			// 写入长度前缀
			lenOff := varint.PutUvarint(uw.buffer, uint64(n))
			// 写入消息内容
			_, err = m.MarshalTo(uw.buffer[lenOff:])
			if err != nil {
				return err
			}
			// 写入完整的消息
			_, err = uw.w.Write(uw.buffer[:lenOff+n])
			return err
		}
	}

	// 回退到标准序列化方式
	data, err = proto.Marshal(msg)
	if err != nil {
		return err
	}
	// 获取消息长度
	length := uint64(len(data))
	// 写入长度前缀
	n := varint.PutUvarint(uw.lenBuf, length)
	_, err = uw.w.Write(uw.lenBuf[:n])
	if err != nil {
		return err
	}
	// 写入消息内容
	_, err = uw.w.Write(data)
	return err
}

// Close 关闭写入器
// 返回值:
//   - error: 错误信息
func (uw *uvarintWriter) Close() error {
	if closer, ok := uw.w.(io.Closer); ok {
		return closer.Close()
	}
	return nil
}
