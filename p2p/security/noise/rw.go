package noise

import (
	"encoding/binary"
	"io"

	pool "github.com/dep2p/go-dep2p/p2plib/buffer/pool"
	"golang.org/x/crypto/chacha20poly1305"
)

// MaxTransportMsgLength 是 Noise 协议规定的最大传输消息长度,包含 MAC 大小(16字节,noise-dep2p 使用 Poly1305)
const MaxTransportMsgLength = 0xffff

// MaxPlaintextLength 是最大有效载荷大小
// 它等于 MaxTransportMsgLength 减去 MAC 大小。超过此大小的有效载荷将自动分块
const MaxPlaintextLength = MaxTransportMsgLength - chacha20poly1305.Overhead

// LengthPrefixLength 是长度前缀本身的长度(以字节为单位),用于分隔所有传输消息
const LengthPrefixLength = 2

// Read 从安全连接中读取数据,将明文数据写入 buf
//
// 参数:
//   - buf: []byte 用于存储读取数据的缓冲区
//
// 返回值:
//   - int 实际读取的字节数
//   - error 读取过程中的错误,如果成功则为 nil
//
// 注意:
//   - 遵循 io.Reader 接口的行为规范
//   - 使用读锁保证并发安全
//   - 支持缓冲区管理和零拷贝优化
func (s *secureSession) Read(buf []byte) (int, error) {
	// 获取读锁,确保并发安全
	s.readLock.Lock()
	defer s.readLock.Unlock()

	// 1. 如果有已缓存的接收字节:
	//   1a. 如果 len(buf) < len(queued),填充 buf,更新偏移指针,返回
	//   1b. 如果 len(buf) >= len(queued),将剩余数据复制到 buf,释放缓冲区回池,返回
	//
	// 2. 否则,从连接中读取下一条消息;next_len 是长度前缀
	//   2a. 如果 len(buf) >= next_len,将消息直接复制到输入缓冲区(零分配路径),返回
	//   2b. 如果 len(buf) >= (next_len - 认证标签长度),从池中获取缓冲区,将加密消息读入其中
	//       直接解密消息到输入缓冲区,返回从池中获取的缓冲区
	//   2c. 如果 len(buf) < next_len,从池中获取缓冲区,将整个消息复制到其中,填充 buf,更新偏移指针
	if s.qbuf != nil {
		// 有缓存的字节,复制尽可能多的数据
		copied := copy(buf, s.qbuf[s.qseek:])
		s.qseek += copied
		if s.qseek == len(s.qbuf) {
			// 缓冲区已空,重置并释放
			pool.Put(s.qbuf)
			s.qseek, s.qbuf = 0, nil
		}
		return copied, nil
	}

	// 读取下一个加密消息的长度
	nextMsgLen, err := s.readNextInsecureMsgLen()
	if err != nil {
		log.Debugf("读取下一个加密消息的长度时出错: %s", err)
		return 0, err
	}

	// 如果缓冲区大小大于等于加密消息大小,可以直接读取并解密
	if len(buf) >= nextMsgLen {
		if err := s.readNextMsgInsecure(buf[:nextMsgLen]); err != nil {
			log.Debugf("读取下一个加密消息时出错: %s", err)
			return 0, err
		}

		dbuf, err := s.decrypt(buf[:0], buf[:nextMsgLen])
		if err != nil {
			log.Debugf("解密消息时出错: %s", err)
			return 0, err
		}

		return len(dbuf), nil
	}

	// 否则,从池中获取缓冲区用于读取消息并就地解密
	cbuf := pool.Get(nextMsgLen)
	if err := s.readNextMsgInsecure(cbuf); err != nil {
		log.Debugf("读取下一个加密消息时出错: %s", err)
		return 0, err
	}

	if s.qbuf, err = s.decrypt(cbuf[:0], cbuf); err != nil {
		log.Debugf("解密消息时出错: %s", err)
		return 0, err
	}

	// 复制尽可能多的字节并更新偏移指针
	s.qseek = copy(buf, s.qbuf)

	return s.qseek, nil
}

// Write 加密明文数据并通过安全连接发送
//
// 参数:
//   - data: []byte 要发送的明文数据
//
// 返回值:
//   - int 实际写入的字节数
//   - error 写入过程中的错误,如果成功则为 nil
//
// 注意:
//   - 使用写锁保证并发安全
//   - 支持大消息自动分块
func (s *secureSession) Write(data []byte) (int, error) {
	// 获取写锁,确保并发安全
	s.writeLock.Lock()
	defer s.writeLock.Unlock()

	var (
		written int         // 已写入的字节数
		cbuf    []byte      // 加密缓冲区
		total   = len(data) // 总数据长度
	)

	// 根据数据大小分配合适的缓冲区
	if total < MaxPlaintextLength {
		cbuf = pool.Get(total + chacha20poly1305.Overhead + LengthPrefixLength)
	} else {
		cbuf = pool.Get(MaxTransportMsgLength + LengthPrefixLength)
	}

	defer pool.Put(cbuf)

	// 分块处理大消息
	for written < total {
		end := written + MaxPlaintextLength
		if end > total {
			end = total
		}

		// 加密当前块
		b, err := s.encrypt(cbuf[:LengthPrefixLength], data[written:end])
		if err != nil {
			log.Debugf("加密当前块时出错: %s", err)
			return 0, err
		}

		// 写入长度前缀
		binary.BigEndian.PutUint16(b, uint16(len(b)-LengthPrefixLength))

		// 发送加密消息
		_, err = s.writeMsgInsecure(b)
		if err != nil {
			log.Debugf("发送加密消息时出错: %s", err)
			return 0, err
		}
		written = end
	}
	return written, nil
}

// readNextInsecureMsgLen 读取不安全连接通道上下一条消息的长度
//
// 返回值:
//   - int 下一条消息的长度
//   - error 读取过程中的错误,如果成功则为 nil
func (s *secureSession) readNextInsecureMsgLen() (int, error) {
	_, err := io.ReadFull(s.insecureReader, s.rlen[:])
	if err != nil {
		log.Debugf("读取下一个加密消息的长度时出错: %s", err)
		return 0, err
	}

	return int(binary.BigEndian.Uint16(s.rlen[:])), err
}

// readNextMsgInsecure 尝试从不安全连接通道精确读取 len(buf) 字节到 buf 中
//
// 参数:
//   - buf: []byte 用于存储读取数据的缓冲区
//
// 返回值:
//   - error 读取过程中的错误,如果成功则为 nil
//
// 注意:
//   - 读取消息时,应先调用 readNextInsecureMsgLen 确定下一条消息的大小,
//     然后用精确大小的缓冲区调用此函数
func (s *secureSession) readNextMsgInsecure(buf []byte) error {
	_, err := io.ReadFull(s.insecureReader, buf)
	if err != nil {
		log.Debugf("读取下一个加密消息时出错: %s", err)
		return err
	}
	return nil
}

// writeMsgInsecure 向不安全连接写入数据
//
// 参数:
//   - data: []byte 要写入的数据
//
// 返回值:
//   - int 实际写入的字节数
//   - error 写入过程中的错误,如果成功则为 nil
//
// 注意:
//   - 数据前会添加一个网络字节序的 16 位无符号整数作为长度前缀
func (s *secureSession) writeMsgInsecure(data []byte) (int, error) {
	n, err := s.insecureConn.Write(data)
	if err != nil {
		log.Debugf("发送加密消息时出错: %s", err)
		return 0, err
	}
	return n, nil
}
