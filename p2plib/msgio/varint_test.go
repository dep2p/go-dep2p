// Package msgio 提供了消息读写功能
package msgio

import (
	"bytes"
	"encoding/binary"
	"io"
	"testing"

	"github.com/dep2p/go-dep2p/multiformats/varint"
)

// TestVarintReadWrite 测试 varint 读写功能
// 参数:
//   - t: 测试对象
func TestVarintReadWrite(t *testing.T) {
	buf := bytes.NewBuffer(nil)
	writer := NewVarintWriter(buf)
	reader := NewVarintReader(buf)
	SubtestReadWrite(t, writer, reader)
}

// TestVarintReadWriteMsg 测试 varint 消息读写功能
// 参数:
//   - t: 测试对象
func TestVarintReadWriteMsg(t *testing.T) {
	buf := bytes.NewBuffer(nil)
	writer := NewVarintWriter(buf)
	reader := NewVarintReader(buf)
	SubtestReadWriteMsg(t, writer, reader)
}

// TestVarintReadWriteMsgSync 测试 varint 同步消息读写功能
// 参数:
//   - t: 测试对象
func TestVarintReadWriteMsgSync(t *testing.T) {
	buf := bytes.NewBuffer(nil)
	writer := NewVarintWriter(buf)
	reader := NewVarintReader(buf)
	SubtestReadWriteMsgSync(t, writer, reader)
}

// TestVarintWrite 测试 varint 写入功能
// 参数:
//   - t: 测试对象
func TestVarintWrite(t *testing.T) {
	SubtestVarintWrite(t, []byte("hello world"))
	SubtestVarintWrite(t, []byte("hello world hello world hello world"))
	SubtestVarintWrite(t, make([]byte, 1<<20))
	SubtestVarintWrite(t, []byte(""))
}

// SubtestVarintWrite 测试 varint 写入子功能
// 参数:
//   - t: 测试对象
//   - msg: 要写入的消息
func SubtestVarintWrite(t *testing.T, msg []byte) {
	buf := bytes.NewBuffer(nil)
	writer := NewVarintWriter(buf)

	if err := writer.WriteMsg(msg); err != nil {
		t.Fatal(err)
	}

	bb := buf.Bytes()

	sbr := simpleByteReader{R: buf}
	length, err := varint.ReadUvarint(&sbr)
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("checking varint is %d", len(msg))
	if int(length) != len(msg) {
		t.Fatalf("incorrect varint: %d != %d", length, len(msg))
	}

	lbuf := make([]byte, binary.MaxVarintLen64)
	n := varint.PutUvarint(lbuf, length)

	bblen := int(length) + n
	t.Logf("checking wrote (%d + %d) bytes", length, n)
	if len(bb) != bblen {
		t.Fatalf("wrote incorrect number of bytes: %d != %d", len(bb), bblen)
	}
}

// TestVarintReadClose 测试 varint 读取关闭功能
// 参数:
//   - t: 测试对象
func TestVarintReadClose(t *testing.T) {
	r, w := io.Pipe()
	writer := NewVarintWriter(w)
	reader := NewVarintReader(r)
	SubtestReadClose(t, writer, reader)
}

// TestVarintWriteClose 测试 varint 写入关闭功能
// 参数:
//   - t: 测试对象
func TestVarintWriteClose(t *testing.T) {
	r, w := io.Pipe()
	writer := NewVarintWriter(w)
	reader := NewVarintReader(r)
	SubtestWriteClose(t, writer, reader)
}
