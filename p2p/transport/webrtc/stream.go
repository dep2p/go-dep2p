package dep2pwebrtc

import (
	"errors"
	"os"
	"sync"
	"time"

	"github.com/dep2p/go-dep2p/core/network"
	"github.com/dep2p/go-dep2p/p2p/transport/webrtc/pb"
	"github.com/dep2p/go-dep2p/p2plib/msgio/pbio"

	"github.com/pion/datachannel"
	"github.com/pion/webrtc/v4"
)

const (
	// maxMessageSize 是我们发送/接收的 Protobuf 消息的最大大小
	maxMessageSize = 16384
	// maxSendBuffer 是我们在底层数据通道上写入的最大数据量
	// 底层 SCTP 层对写入有无限缓冲区
	// 我们限制每个流的排队数据量以避免单个流独占整个连接
	maxSendBuffer = 2 * maxMessageSize
	// sendBufferLowThreshold 是我们在底层数据通道上写入更多数据的阈值
	// 我们希望一旦可以写入一个完整大小的消息就得到通知
	sendBufferLowThreshold = maxSendBuffer - maxMessageSize
	// maxTotalControlMessagesSize 是我们在此流上写入的所有控制消息的最大总大小
	// 4个大小为10字节的控制消息 + 10字节缓冲区
	// 这个数字不需要精确
	// 在最坏的情况下,我们在 webrtc 对等连接发送队列中多排队这么多字节
	maxTotalControlMessagesSize = 50

	// protoOverhead 是协议开销,假定为5字节
	protoOverhead = 5
	// varintOverhead 假定为2字节,这是安全的因为:
	// 1. 这仅在写入消息时使用
	// 2. 我们只以 `maxMessageSize - varintOverhead` 的块发送消息
	// 包括数据和 protobuf 头。由于 `maxMessageSize` 小于等于 2^14,varint 长度不会超过2字节
	varintOverhead = 2
	// maxFINACKWait 是流在关闭数据通道前等待读取 FIN_ACK 的最长时间
	maxFINACKWait = 10 * time.Second
)

// receiveState 表示接收状态的枚举类型
type receiveState uint8

const (
	receiveStateReceiving receiveState = iota // 正在接收数据
	receiveStateDataRead                      // 已接收并读取 FIN
	receiveStateReset                         // 通过本地调用 CloseRead 或接收到 RESET
)

// sendState 表示发送状态的枚举类型
type sendState uint8

const (
	sendStateSending      sendState = iota // 正在发送数据
	sendStateDataSent                      // 数据已发送
	sendStateDataReceived                  // 数据已接收
	sendStateReset                         // 已重置
)

// stream 实现了 network.MuxedStream 接口
// 将 pion 的数据通道封装为 net.Conn 然后转换为 network.MuxedStream
type stream struct {
	// mx 保护流的所有字段的并发访问
	mx sync.Mutex

	// readerMx 确保只有一个 goroutine 从 reader 读取
	// Read 不是线程安全的,但我们可能需要从不同的 goroutine 读取控制消息
	readerMx sync.Mutex
	// reader 用于从数据通道读取消息的 protobuf 读取器
	reader pbio.Reader

	// nextMessage 缓冲区限制为单个消息
	// 需要它是因为读取器可能会中途读取消息,所以我们需要缓冲它直到剩余部分被读取
	nextMessage *pb.Message
	// receiveState 表示流的接收状态
	receiveState receiveState

	// writer 用于向数据通道写入消息的 protobuf 写入器,并发写入由 mx 互斥锁防止
	writer pbio.Writer
	// writeStateChanged 用于通知写入状态发生变化的通道
	writeStateChanged chan struct{}
	// sendState 表示流的发送状态
	sendState sendState
	// writeDeadline 表示写入操作的截止时间
	writeDeadline time.Time

	// controlMessageReaderOnce 确保控制消息读取器只启动一次
	controlMessageReaderOnce sync.Once
	// controlMessageReaderEndTime 是从控制消息读取器读取 FIN_ACK 的结束时间
	// 我们不能依赖 SetReadDeadline 来实现这一点,因为它容易出现竞态条件
	// 即之前的截止时间定时器在最新调用 SetReadDeadline 后触发
	// 参见: https://github.com/pion/sctp/pull/290
	controlMessageReaderEndTime time.Time

	// onDoneOnce 确保 onDone 回调只执行一次
	onDoneOnce sync.Once
	// onDone 是流关闭时的回调函数
	onDone func()
	// id 是数据通道的标识符,用于日志记录
	id uint16
	// dataChannel 是底层的 WebRTC 数据通道
	dataChannel *datachannel.DataChannel
	// closeForShutdownErr 存储关闭时的错误信息
	closeForShutdownErr error
}

// 确保 stream 实现了 network.MuxedStream 接口
var _ network.MuxedStream = &stream{}

// newStream 创建一个新的流对象
// 参数:
//   - channel: *webrtc.DataChannel WebRTC 数据通道
//   - rwc: datachannel.ReadWriteCloser 读写关闭接口
//   - onDone: func() 完成时的回调函数
//
// 返回值:
//   - *stream 新创建的流对象
func newStream(
	channel *webrtc.DataChannel,
	rwc datachannel.ReadWriteCloser,
	onDone func(),
) *stream {
	s := &stream{
		reader:            pbio.NewDelimitedReader(rwc, maxMessageSize),
		writer:            pbio.NewDelimitedWriter(rwc),
		writeStateChanged: make(chan struct{}, 1),
		id:                *channel.ID(),
		dataChannel:       rwc.(*datachannel.DataChannel),
		onDone:            onDone,
	}
	// 设置缓冲区低水位阈值
	s.dataChannel.SetBufferedAmountLowThreshold(sendBufferLowThreshold)
	// 设置缓冲区低水位回调
	s.dataChannel.OnBufferedAmountLow(func() {
		s.notifyWriteStateChanged()
	})
	return s
}

// Close 关闭流
// 返回值:
//   - error 关闭过程中的错误
func (s *stream) Close() error {
	s.mx.Lock()
	isClosed := s.closeForShutdownErr != nil
	s.mx.Unlock()
	if isClosed {
		return nil
	}
	defer s.cleanup()
	closeWriteErr := s.CloseWrite()
	closeReadErr := s.CloseRead()
	if closeWriteErr != nil || closeReadErr != nil {
		log.Debugf("关闭流失败: %s, %s", closeWriteErr, closeReadErr)
		s.Reset()
		return errors.Join(closeWriteErr, closeReadErr)
	}

	s.mx.Lock()
	if s.controlMessageReaderEndTime.IsZero() {
		s.controlMessageReaderEndTime = time.Now().Add(maxFINACKWait)
		s.setDataChannelReadDeadline(time.Now().Add(-1 * time.Hour))
	}
	s.mx.Unlock()
	return nil
}

// Reset 重置流
// 返回值:
//   - error 重置过程中的错误
func (s *stream) Reset() error {
	s.mx.Lock()
	isClosed := s.closeForShutdownErr != nil
	s.mx.Unlock()
	if isClosed {
		return nil
	}

	defer s.cleanup()
	cancelWriteErr := s.cancelWrite()
	closeReadErr := s.CloseRead()
	s.setDataChannelReadDeadline(time.Now().Add(-1 * time.Hour))
	log.Debugf("关闭流失败: %s, %s", closeReadErr, cancelWriteErr)
	return errors.Join(closeReadErr, cancelWriteErr)
}

// closeForShutdown 关闭流以进行关闭
// 参数:
//   - closeErr: error 关闭错误
func (s *stream) closeForShutdown(closeErr error) {
	defer s.cleanup()

	s.mx.Lock()
	defer s.mx.Unlock()

	s.closeForShutdownErr = closeErr
	s.notifyWriteStateChanged()
}

// SetDeadline 设置读写截止时间
// 参数:
//   - t: time.Time 截止时间
//
// 返回值:
//   - error 设置过程中的错误
func (s *stream) SetDeadline(t time.Time) error {
	_ = s.SetReadDeadline(t)
	return s.SetWriteDeadline(t)
}

// processIncomingFlag 处理传入消息的标志
// 参数:
//   - flag: *pb.Message_Flag 消息标志
//
// 注意:
//   - 调用此方法时必须持有互斥锁
func (s *stream) processIncomingFlag(flag *pb.Message_Flag) {
	if flag == nil {
		return
	}

	switch *flag {
	case pb.Message_STOP_SENDING:
		// 我们必须在发送 FIN(sendStateDataSent) 后处理 STOP_SENDING
		// 远程对等方在发送 STOP_SENDING 后可能不会发送 FIN_ACK
		if s.sendState == sendStateSending || s.sendState == sendStateDataSent {
			s.sendState = sendStateReset
		}
		s.notifyWriteStateChanged()
	case pb.Message_FIN_ACK:
		s.sendState = sendStateDataReceived
		s.notifyWriteStateChanged()
	case pb.Message_FIN:
		if s.receiveState == receiveStateReceiving {
			s.receiveState = receiveStateDataRead
		}
		if err := s.writer.WriteMsg(&pb.Message{Flag: pb.Message_FIN_ACK.Enum()}); err != nil {
			log.Debugf("发送 FIN_ACK 失败: %s", err)
			// 远程已完成写入所有数据,它最终会停止等待 FIN_ACK 或在我们关闭数据通道时得到通知
		}
		s.spawnControlMessageReader()
	case pb.Message_RESET:
		if s.receiveState == receiveStateReceiving {
			s.receiveState = receiveStateReset
		}
		s.spawnControlMessageReader()
	}
}

// spawnControlMessageReader 用于在读取器关闭后处理控制消息
func (s *stream) spawnControlMessageReader() {
	s.controlMessageReaderOnce.Do(func() {
		// 启动 goroutine 以确保不持有任何锁
		go func() {
			// 清理 sctp 截止时间定时器 goroutine
			defer s.setDataChannelReadDeadline(time.Time{})

			defer s.dataChannel.Close()

			// 解除任何等待 reader.ReadMsg 的 Read 调用的阻塞
			s.setDataChannelReadDeadline(time.Now().Add(-1 * time.Hour))

			s.readerMx.Lock()
			// 我们持有锁:任何阻塞在 reader.ReadMsg 上的读取器都已退出
			s.mx.Lock()
			defer s.mx.Unlock()
			// 从此时起只有这个 goroutine 会执行 reader.ReadMsg
			// 我们只是想确保任何现有的读取器都已退出
			// 从此时起的 Read 调用在检查 s.readState 时会立即退出
			s.readerMx.Unlock()

			if s.nextMessage != nil {
				s.processIncomingFlag(s.nextMessage.Flag)
				s.nextMessage = nil
			}
			var msg pb.Message
			for {
				// 连接已关闭,无需清理数据通道
				if s.closeForShutdownErr != nil {
					return
				}
				// 流的写入半部分已完成
				if s.sendState == sendStateDataReceived || s.sendState == sendStateReset {
					return
				}
				// FIN_ACK 等待超时
				if !s.controlMessageReaderEndTime.IsZero() && time.Now().After(s.controlMessageReaderEndTime) {
					return
				}

				s.setDataChannelReadDeadline(s.controlMessageReaderEndTime)
				s.mx.Unlock()
				err := s.reader.ReadMsg(&msg)
				s.mx.Lock()
				if err != nil {
					// 我们必须手动管理截止时间超时错误,因为 pion/sctp 可能会对已取消的截止时间返回超时错误
					// 参见: https://github.com/pion/sctp/pull/290/files
					if errors.Is(err, os.ErrDeadlineExceeded) {
						continue
					}
					return
				}
				s.processIncomingFlag(msg.Flag)
			}
		}()
	})
}

// cleanup 执行清理操作
func (s *stream) cleanup() {
	s.onDoneOnce.Do(func() {
		if s.onDone != nil {
			s.onDone()
		}
	})
}
