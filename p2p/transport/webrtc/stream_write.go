package dep2pwebrtc

import (
	"errors"
	"os"
	"time"

	"github.com/dep2p/go-dep2p/core/network"
	"github.com/dep2p/go-dep2p/p2p/transport/webrtc/pb"
)

// 写入关闭后再次写入的错误
var errWriteAfterClose = errors.New("写入关闭后再次写入")

// 如果可用空间小于最小消息大小,我们不会在数据通道上放置新消息
// 而是等待更多空间释放
const minMessageSize = 1 << 10

// Write 向流中写入数据
// 参数:
//   - b: []byte 要写入的数据
//
// 返回值:
//   - int 实际写入的字节数
//   - error 写入过程中的错误
//
// 注意:
//   - 写入过程会被写入截止时间限制
//   - 写入会被分片成多个消息发送
func (s *stream) Write(b []byte) (int, error) {
	// 加锁保护并发访问
	s.mx.Lock()
	defer s.mx.Unlock()

	// 检查是否因关闭而终止
	if s.closeForShutdownErr != nil {
		log.Debugf("流已关闭: %s", s.closeForShutdownErr)
		return 0, s.closeForShutdownErr
	}
	// 检查发送状态
	switch s.sendState {
	case sendStateReset:
		log.Debugf("流已重置")
		return 0, network.ErrReset
	case sendStateDataSent, sendStateDataReceived:
		log.Debugf("流已关闭")
		return 0, errWriteAfterClose
	}

	// 检查写入截止时间
	if !s.writeDeadline.IsZero() && time.Now().After(s.writeDeadline) {
		log.Debugf("写入截止时间已过")
		return 0, os.ErrDeadlineExceeded
	}

	// 用于写入截止时间的定时器
	var writeDeadlineTimer *time.Timer
	defer func() {
		if writeDeadlineTimer != nil {
			writeDeadlineTimer.Stop()
		}
	}()

	var n int
	var msg pb.Message
	// 循环写入所有数据
	for len(b) > 0 {
		// 检查是否因关闭而终止
		if s.closeForShutdownErr != nil {
			log.Debugf("流已关闭: %s", s.closeForShutdownErr)
			return n, s.closeForShutdownErr
		}
		// 检查发送状态
		switch s.sendState {
		case sendStateReset:
			log.Debugf("流已重置")
			return n, network.ErrReset
		case sendStateDataSent, sendStateDataReceived:
			log.Debugf("流已关闭")
			return n, errWriteAfterClose
		}

		// 处理写入截止时间
		writeDeadline := s.writeDeadline
		if writeDeadline.IsZero() && writeDeadlineTimer != nil {
			writeDeadlineTimer.Stop()
			writeDeadlineTimer = nil
		}
		var writeDeadlineChan <-chan time.Time
		if !writeDeadline.IsZero() {
			if writeDeadlineTimer == nil {
				writeDeadlineTimer = time.NewTimer(time.Until(writeDeadline))
			} else {
				if !writeDeadlineTimer.Stop() {
					<-writeDeadlineTimer.C
				}
				writeDeadlineTimer.Reset(time.Until(writeDeadline))
			}
			writeDeadlineChan = writeDeadlineTimer.C
		}

		// 检查可用空间
		availableSpace := s.availableSendSpace()
		if availableSpace < minMessageSize {
			log.Debugf("可用空间小于最小消息大小: %d < %d", availableSpace, minMessageSize)
			s.mx.Unlock()
			select {
			case <-writeDeadlineChan:
				log.Debugf("写入截止时间已过")
				s.mx.Lock()
				return n, os.ErrDeadlineExceeded
			case <-s.writeStateChanged:
			}
			s.mx.Lock()
			continue
		}
		// 计算本次写入长度
		end := maxMessageSize
		if end > availableSpace {
			end = availableSpace
		}
		end -= protoOverhead + varintOverhead
		if end > len(b) {
			end = len(b)
		}
		// 构造并发送消息
		msg = pb.Message{Message: b[:end]}
		if err := s.writer.WriteMsg(&msg); err != nil {
			log.Debugf("写入消息失败: %s", err)
			return n, err
		}
		n += end
		b = b[end:]
	}
	return n, nil
}

// SetWriteDeadline 设置写入截止时间
// 参数:
//   - t: time.Time 截止时间
//
// 返回值:
//   - error 设置过程中的错误
func (s *stream) SetWriteDeadline(t time.Time) error {
	s.mx.Lock()
	defer s.mx.Unlock()
	s.writeDeadline = t
	s.notifyWriteStateChanged()
	// log.Infof("设置写入截止时间: %s", t)
	return nil
}

// availableSendSpace 计算可用的发送空间
// 返回值:
//   - int 可用的字节数
func (s *stream) availableSendSpace() int {
	buffered := int(s.dataChannel.BufferedAmount())
	availableSpace := maxSendBuffer - buffered
	if availableSpace+maxTotalControlMessagesSize < 0 { // 这种情况不应该发生,但最好检查一下
		log.Debugf("数据通道缓冲的数据超过最大限制", "最大值", maxSendBuffer, "已缓冲", buffered)
	}
	return availableSpace
}

// cancelWrite 取消写入操作
// 返回值:
//   - error 取消过程中的错误
func (s *stream) cancelWrite() error {
	s.mx.Lock()
	defer s.mx.Unlock()

	// 如果写入半部分已经成功关闭或之前已重置,则无需重置写入半部分
	if s.sendState == sendStateDataReceived || s.sendState == sendStateReset {
		return nil
	}
	s.sendState = sendStateReset
	// 从数据通道中移除对该流的引用
	s.dataChannel.OnBufferedAmountLow(nil)
	s.notifyWriteStateChanged()
	return s.writer.WriteMsg(&pb.Message{Flag: pb.Message_RESET.Enum()})
}

// CloseWrite 关闭写入
// 返回值:
//   - error 关闭过程中的错误
func (s *stream) CloseWrite() error {
	s.mx.Lock()
	defer s.mx.Unlock()

	if s.sendState != sendStateSending {
		return nil
	}
	s.sendState = sendStateDataSent
	// 从数据通道中移除对该流的引用
	s.dataChannel.OnBufferedAmountLow(nil)
	s.notifyWriteStateChanged()
	return s.writer.WriteMsg(&pb.Message{Flag: pb.Message_FIN.Enum()})
}

// notifyWriteStateChanged 通知写入状态发生变化
func (s *stream) notifyWriteStateChanged() {
	select {
	case s.writeStateChanged <- struct{}{}:
	default:
	}
}
