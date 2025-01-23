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
package protoio_test

import (
	"bytes"
	"io"
	"math/rand"
	"testing"
	"time"

	"github.com/gogo/protobuf/test"

	"github.com/dep2p/go-dep2p/multiformats/varint"
	"github.com/dep2p/go-dep2p/p2plib/msgio/protoio"
)

// TestVarintNormal 测试正常情况下的变长整数编解码
// 参数:
//   - t: 测试对象
func TestVarintNormal(t *testing.T) {
	buf := newBuffer()
	writer := protoio.NewDelimitedWriter(buf)
	reader := protoio.NewDelimitedReader(buf, 1024*1024)
	if err := iotest(writer, reader); err != nil {
		t.Error(err)
	}
	if !buf.closed {
		t.Fatalf("did not close buffer")
	}
}

// TestVarintNoClose 测试不关闭缓冲区的情况
// 参数:
//   - t: 测试对象
func TestVarintNoClose(t *testing.T) {
	buf := bytes.NewBuffer(nil)
	writer := protoio.NewDelimitedWriter(buf)
	reader := protoio.NewDelimitedReader(buf, 1024*1024)
	if err := iotest(writer, reader); err != nil {
		t.Error(err)
	}
}

// TestVarintMaxSize 测试超出最大大小限制的情况
// 参数:
//   - t: 测试对象
func TestVarintMaxSize(t *testing.T) {
	buf := newBuffer()
	writer := protoio.NewDelimitedWriter(buf)
	reader := protoio.NewDelimitedReader(buf, 20)
	if err := iotest(writer, reader); err != io.ErrShortBuffer {
		t.Error(err)
	} else {
		t.Logf("%s", err)
	}
}

// TestVarintError 测试变长整数解码错误的情况
// 参数:
//   - t: 测试对象
func TestVarintError(t *testing.T) {
	buf := newBuffer()
	// 写入超出 uvarint63 容量的数据
	buf.Write([]byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff})
	reader := protoio.NewDelimitedReader(buf, 1024*1024)
	msg := &test.NinOptNative{}
	err := reader.ReadMsg(msg)
	if err != varint.ErrOverflow {
		t.Fatalf("expected varint.ErrOverflow error")
	}
}

// buffer 定义了一个带关闭标志的字节缓冲区
type buffer struct {
	*bytes.Buffer
	closed bool
}

// Close 关闭缓冲区
// 返回值:
//   - error: 错误信息
func (b *buffer) Close() error {
	b.closed = true
	return nil
}

// newBuffer 创建一个新的缓冲区
// 返回值:
//   - *buffer: 缓冲区对象
func newBuffer() *buffer {
	return &buffer{bytes.NewBuffer(nil), false}
}

// iotest 测试消息的写入和读取
// 参数:
//   - writer: 消息写入器
//   - reader: 消息读取器
//
// 返回值:
//   - error: 错误信息
func iotest(writer protoio.WriteCloser, reader protoio.ReadCloser) error {
	size := 1000
	msgs := make([]*test.NinOptNative, size)
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	for i := range msgs {
		msgs[i] = test.NewPopulatedNinOptNative(r, true)
		// https://github.com/gogo/protobuf/issues/31
		if i == 5 {
			msgs[i] = &test.NinOptNative{}
		}
		// https://github.com/gogo/protobuf/issues/31
		if i == 999 {
			msgs[i] = &test.NinOptNative{}
		}
		err := writer.WriteMsg(msgs[i])
		if err != nil {
			return err
		}
	}
	if err := writer.Close(); err != nil {
		return err
	}
	i := 0
	for {
		msg := &test.NinOptNative{}
		if err := reader.ReadMsg(msg); err != nil {
			if err == io.EOF {
				break
			}
			return err
		}
		if err := msg.VerboseEqual(msgs[i]); err != nil {
			return err
		}
		i++
	}
	if i != size {
		panic("not enough messages read")
	}
	if err := reader.Close(); err != nil {
		return err
	}
	return nil
}
