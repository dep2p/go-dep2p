// Package pbio 提供了带有 varint 前缀的 protobuf 消息读写功能。
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
package pbio

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"runtime/debug"

	"google.golang.org/protobuf/proto"

	"github.com/multiformats/go-varint"
)

// uvarintReader 实现了带有 varint 长度前缀的 protobuf 消息读取器
type uvarintReader struct {
	r       *bufio.Reader // 底层读取器
	buf     []byte        // 用于存储消息的缓冲区
	maxSize int           // 允许的最大消息大小
	closer  io.Closer     // 可选的关闭器接口
}

// NewDelimitedReader 创建一个新的带分隔符的读取器
// 参数:
//   - r: 底层读取器
//   - maxSize: 允许的最大消息大小
//
// 返回值:
//   - ReadCloser: 读取器接口
func NewDelimitedReader(r io.Reader, maxSize int) ReadCloser {
	var closer io.Closer
	// 如果底层读取器实现了 Closer 接口，保存它
	if c, ok := r.(io.Closer); ok {
		closer = c
	}
	return &uvarintReader{bufio.NewReader(r), nil, maxSize, closer}
}

// ReadMsg 读取一个 protobuf 消息
// 参数:
//   - msg: 用于存储读取结果的消息对象
//
// 返回值:
//   - error: 错误信息
func (ur *uvarintReader) ReadMsg(msg proto.Message) (err error) {
	// 设置 panic 恢复
	defer func() {
		if rerr := recover(); rerr != nil {
			fmt.Fprintf(os.Stderr, "caught panic: %s\n%s\n", rerr, debug.Stack())
			err = fmt.Errorf("panic reading message: %s", rerr)
		}
	}()

	// 读取消息长度
	length64, err := varint.ReadUvarint(ur.r)
	if err != nil {
		return err
	}

	// 验证消息长度
	length := int(length64)
	if length < 0 || length > ur.maxSize {
		return io.ErrShortBuffer
	}

	// 确保缓冲区足够大
	if len(ur.buf) < length {
		ur.buf = make([]byte, length)
	}
	buf := ur.buf[:length]

	// 读取消息内容
	if _, err := io.ReadFull(ur.r, buf); err != nil {
		return err
	}

	// 解析消息
	return proto.Unmarshal(buf, msg)
}

// Close 关闭读取器
// 返回值:
//   - error: 错误信息
func (ur *uvarintReader) Close() error {
	if ur.closer != nil {
		return ur.closer.Close()
	}
	return nil
}
