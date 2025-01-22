package util

import (
	"errors"
	"io"

	"github.com/dep2p/go-dep2p/multiformats/varint"
	pool "github.com/dep2p/go-dep2p/p2plib/buffer/pool"
	"github.com/dep2p/go-dep2p/p2plib/msgio/pbio"
	logging "github.com/dep2p/log"
	"google.golang.org/protobuf/proto"
)

var log = logging.Logger("p2p-protocol-circuitv2-util-io")

// DelimitedReader 实现了一个无缓冲的分隔消息读取器
type DelimitedReader struct {
	r   io.Reader // 底层读取器
	buf []byte    // 用于读取的缓冲区
}

// NewDelimitedReader 创建一个新的分隔消息读取器
// gogo protobuf 的 NewDelimitedReader 是有缓冲的,可能会吞掉流数据
// 所以我们需要实现一个兼容的无缓冲读取器
// 无缓冲读取会导致性能下降:读取消息时需要多次单字节读取来获取长度,再读取一次获取消息内容
// 但这种性能下降并不严重,因为:
//   - 读取器在握手阶段只读取一到两条消息(dialer/stop或hop),相对于连接生命周期来说微不足道
//   - 消息很小(最大4k),长度只占用几个字节,所以每条消息最多只需要三次读取
//
// 参数:
//   - r: io.Reader 底层读取器
//   - maxSize: int 最大消息大小
//
// 返回值:
//   - *DelimitedReader 新创建的分隔消息读取器
func NewDelimitedReader(r io.Reader, maxSize int) *DelimitedReader {
	return &DelimitedReader{r: r, buf: pool.Get(maxSize)}
}

// Close 关闭读取器并释放缓冲区
//
// 注意:
//   - 会将内部缓冲区归还到内存池
func (d *DelimitedReader) Close() {
	if d.buf != nil {
		pool.Put(d.buf)
		d.buf = nil
	}
}

// ReadByte 读取单个字节
//
// 返回值:
//   - byte: 读取的字节
//   - error: 读取过程中的错误
func (d *DelimitedReader) ReadByte() (byte, error) {
	buf := d.buf[:1]
	_, err := d.r.Read(buf)
	return buf[0], err
}

// ReadMsg 读取一个完整的protobuf消息
//
// 参数:
//   - msg: proto.Message 用于存储读取结果的protobuf消息对象
//
// 返回值:
//   - error: 读取过程中的错误,如果消息过大会返回"消息太大"错误
func (d *DelimitedReader) ReadMsg(msg proto.Message) error {
	mlen, err := varint.ReadUvarint(d)
	if err != nil {
		log.Debugf("读取消息长度失败: %v", err)
		return err
	}

	if uint64(len(d.buf)) < mlen {
		log.Debugf("消息太大: %d", mlen)
		return errors.New("消息太大")
	}

	buf := d.buf[:mlen]
	_, err = io.ReadFull(d.r, buf)
	if err != nil {
		log.Debugf("读取消息内容失败: %v", err)
		return err
	}

	return proto.Unmarshal(buf, msg)
}

// NewDelimitedWriter 创建一个新的分隔消息写入器
//
// 参数:
//   - w: io.Writer 底层写入器
//
// 返回值:
//   - pbio.WriteCloser 新创建的分隔消息写入器
func NewDelimitedWriter(w io.Writer) pbio.WriteCloser {
	return pbio.NewDelimitedWriter(w)
}
