// Package pbio 读写带有 varint 前缀的 protobuf 消息，使用 Google 的 Protobuf 包。
package pbio

import (
	"io"

	"google.golang.org/protobuf/proto"
)

// Writer 定义了写入 protobuf 消息的接口
type Writer interface {
	// WriteMsg 写入一个 protobuf 消息
	// 参数:
	//   - proto.Message: 要写入的消息
	//
	// 返回值:
	//   - error: 错误信息
	WriteMsg(proto.Message) error
}

// WriteCloser 组合了 Writer 和 io.Closer 接口
type WriteCloser interface {
	Writer
	io.Closer
}

// Reader 定义了读取 protobuf 消息的接口
type Reader interface {
	// ReadMsg 读取一个 protobuf 消息
	// 参数:
	//   - msg: 用于存储读取结果的消息对象
	//
	// 返回值:
	//   - error: 错误信息
	ReadMsg(msg proto.Message) error
}

// ReadCloser 组合了 Reader 和 io.Closer 接口
type ReadCloser interface {
	Reader
	io.Closer
}

// getSize 获取对象的大小
// 参数:
//   - v: 要获取大小的对象
//
// 返回值:
//   - int: 对象大小
//   - bool: 是否成功获取大小
func getSize(v interface{}) (int, bool) {
	// 尝试使用 Size() 方法获取大小
	if sz, ok := v.(interface {
		Size() (n int)
	}); ok {
		return sz.Size(), true
	} else if sz, ok := v.(interface {
		// 尝试使用 ProtoSize() 方法获取大小
		ProtoSize() (n int)
	}); ok {
		return sz.ProtoSize(), true
	} else {
		// 无法获取大小
		return 0, false
	}
}
