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
	"io"

	"github.com/gogo/protobuf/proto"
)

// Writer 定义了写入 protobuf 消息的接口
type Writer interface {
	// WriteMsg 将 protobuf 消息写入底层存储
	// 参数:
	//   - msg: 要写入的 protobuf 消息
	//
	// 返回值:
	//   - error: 写入过程中的错误信息
	WriteMsg(proto.Message) error
}

// WriteCloser 组合了 Writer 接口和 io.Closer 接口
type WriteCloser interface {
	Writer
	io.Closer
}

// Reader 定义了读取 protobuf 消息的接口
type Reader interface {
	// ReadMsg 从底层存储读取 protobuf 消息
	// 参数:
	//   - msg: 用于存储读取数据的 protobuf 消息对象
	//
	// 返回值:
	//   - error: 读取过程中的错误信息
	ReadMsg(msg proto.Message) error
}

// ReadCloser 组合了 Reader 接口和 io.Closer 接口
type ReadCloser interface {
	Reader
	io.Closer
}

// getSize 获取给定对象的大小
// 参数:
//   - v: 要获取大小的对象
//
// 返回值:
//   - int: 对象的大小
//   - bool: 是否成功获取大小
func getSize(v interface{}) (int, bool) {
	// 尝试使用 Size() 方法获取大小
	if sz, ok := v.(interface {
		Size() (n int)
	}); ok {
		return sz.Size(), true
	} else if sz, ok := v.(interface {
		ProtoSize() (n int)
	}); ok {
		// 尝试使用 ProtoSize() 方法获取大小
		return sz.ProtoSize(), true
	} else {
		// 无法获取大小
		return 0, false
	}
}
