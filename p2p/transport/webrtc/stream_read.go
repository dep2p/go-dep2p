package dep2pwebrtc

import (
	"io"
	"time"

	"github.com/dep2p/go-dep2p/core/network"
	"github.com/dep2p/go-dep2p/p2p/transport/webrtc/pb"
)

// Read 从流中读取数据到缓冲区
// 参数:
//   - b: []byte 用于存储读取数据的缓冲区
//
// 返回值:
//   - int 实际读取的字节数
//   - error 读取过程中的错误,如果到达流末尾返回 io.EOF
func (s *stream) Read(b []byte) (int, error) {
	// 锁定读取器互斥锁
	s.readerMx.Lock()
	defer s.readerMx.Unlock()

	// 锁定流互斥锁
	s.mx.Lock()
	defer s.mx.Unlock()

	// 检查是否因关闭而发生错误
	if s.closeForShutdownErr != nil {
		log.Debugf("流已关闭: %s", s.closeForShutdownErr)
		return 0, s.closeForShutdownErr
	}

	// 根据接收状态返回相应错误
	switch s.receiveState {
	case receiveStateDataRead:
		log.Debugf("流已读取完毕")
		return 0, io.EOF
	case receiveStateReset:
		log.Debugf("流已重置")
		return 0, network.ErrReset
	}

	// 如果缓冲区长度为0,直接返回
	if len(b) == 0 {
		return 0, nil
	}

	var read int
	for {
		if s.nextMessage == nil {
			// 加载下一条消息
			s.mx.Unlock()
			var msg pb.Message
			err := s.reader.ReadMsg(&msg)
			s.mx.Lock()
			if err != nil {
				// 连接已关闭
				if s.closeForShutdownErr != nil {
					log.Debugf("流已关闭: %s", s.closeForShutdownErr)
					return 0, s.closeForShutdownErr
				}
				if err == io.EOF {
					// 如果通道正常关闭,返回EOF
					if s.receiveState == receiveStateDataRead {
						log.Debugf("流已读取完毕")
						return 0, io.EOF
					}
					// 当远程端在不写入FIN消息的情况下关闭数据通道时会发生这种情况
					// 某些实现在关闭数据通道时会丢弃缓冲的数据
					// 对于这些实现,流重置将表现为数据通道的突然关闭
					s.receiveState = receiveStateReset
					return 0, network.ErrReset
				}
				if s.receiveState == receiveStateReset {
					log.Debugf("流已重置")
					return 0, network.ErrReset
				}
				if s.receiveState == receiveStateDataRead {
					log.Debugf("流已读取完毕")
					return 0, io.EOF
				}
				return 0, err
			}
			s.nextMessage = &msg
		}

		// 复制消息数据到缓冲区
		if len(s.nextMessage.Message) > 0 {
			n := copy(b, s.nextMessage.Message)
			read += n
			s.nextMessage.Message = s.nextMessage.Message[n:]
			return read, nil
		}

		// 读取完所有数据后处理消息标志
		s.processIncomingFlag(s.nextMessage.Flag)
		s.nextMessage = nil
		if s.closeForShutdownErr != nil {
			log.Debugf("流已关闭: %s", s.closeForShutdownErr)
			return read, s.closeForShutdownErr
		}
		switch s.receiveState {
		case receiveStateDataRead:
			return read, io.EOF
		case receiveStateReset:
			return read, network.ErrReset
		}
	}
}

// SetReadDeadline 设置读取操作的截止时间
// 参数:
//   - t: time.Time 截止时间
//
// 返回值:
//   - error 设置过程中的错误
func (s *stream) SetReadDeadline(t time.Time) error {
	s.mx.Lock()
	defer s.mx.Unlock()
	if s.receiveState == receiveStateReceiving {
		s.setDataChannelReadDeadline(t)
	}
	return nil
}

// setDataChannelReadDeadline 设置数据通道的读取截止时间
// 参数:
//   - t: time.Time 截止时间
//
// 返回值:
//   - error 设置过程中的错误
func (s *stream) setDataChannelReadDeadline(t time.Time) error {
	return s.dataChannel.SetReadDeadline(t)
}

// CloseRead 关闭流的读取端
// 返回值:
//   - error 关闭过程中的错误
func (s *stream) CloseRead() error {
	s.mx.Lock()
	defer s.mx.Unlock()
	var err error
	if s.receiveState == receiveStateReceiving && s.closeForShutdownErr == nil {
		// 发送停止发送消息
		err = s.writer.WriteMsg(&pb.Message{Flag: pb.Message_STOP_SENDING.Enum()})
		s.receiveState = receiveStateReset
	}
	s.spawnControlMessageReader()
	return err
}
