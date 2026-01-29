package ping

import (
	"context"
	"crypto/rand"
	"errors"
	"io"
	"time"

	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/protocol"
)

// ProtocolID Ping 协议 ID（使用统一定义）
var ProtocolID = string(protocol.Ping)

const (

	// PingSize Ping 消息大小（32 字节）
	PingSize = 32

	// PingTimeout Ping 超时时间
	PingTimeout = 10 * time.Second

	// HandlerIdleTimeout Handler 空闲超时时间
	// BUG FIX #B16: 防止恶意客户端长时间占用资源
	HandlerIdleTimeout = 60 * time.Second
)

// BUG FIX #B14: 定义专门的错误类型，而不是复用 io.ErrUnexpectedEOF
var (
	// ErrDataMismatch Ping 回显数据不匹配
	ErrDataMismatch = errors.New("ping: echo data mismatch")
)

// Service Ping 服务
type Service struct {
	// 无需字段，handler 是无状态的
}

// NewService 创建 Ping 服务
func NewService() *Service {
	return &Service{}
}

// Handler 处理 Ping 请求（服务器端）
// 读取数据并回显
func (s *Service) Handler(stream pkgif.Stream) {
	defer stream.Close()

	buf := make([]byte, PingSize)

	// 循环处理 Ping 请求（支持连续 ping）
	for {
		// BUG FIX #B16: 设置读取超时，防止恶意客户端占用资源
		// FIX #L1: 直接调用接口方法，无需类型断言
		_ = stream.SetReadDeadline(time.Now().Add(HandlerIdleTimeout))

		// 读取 32 字节
		_, err := io.ReadFull(stream, buf)
		if err != nil {
			// 连接关闭或读取失败（包括超时）
			return
		}

		// 回显
		_, err = stream.Write(buf)
		if err != nil {
			// 写入失败
			return
		}
	}
}

// Ping 主动 Ping 节点（客户端）
// 返回往返时间（RTT）
func Ping(ctx context.Context, host pkgif.Host, peer string) (time.Duration, error) {
	// 1. 创建流
	stream, err := host.NewStream(ctx, peer, ProtocolID)
	if err != nil {
		return 0, err
	}
	defer stream.Close()

	// BUG FIX #B15: 设置流读写超时，防止永久阻塞
	// FIX #L1: 直接调用接口方法，无需类型断言
	_ = stream.SetDeadline(time.Now().Add(PingTimeout))

	// 2. 生成随机数据
	buf := make([]byte, PingSize)
	_, err = rand.Read(buf)
	if err != nil {
		return 0, err
	}

	// 3. 测量 RTT
	start := time.Now()

	// 发送
	_, err = stream.Write(buf)
	if err != nil {
		return 0, err
	}

	// 接收回显
	echo := make([]byte, PingSize)
	_, err = io.ReadFull(stream, echo)
	if err != nil {
		return 0, err
	}

	rtt := time.Since(start)

	// 4. 验证回显数据
	// BUG FIX #B14: 使用专门的错误类型 ErrDataMismatch
	for i := 0; i < PingSize; i++ {
		if buf[i] != echo[i] {
			return 0, ErrDataMismatch
		}
	}

	return rtt, nil
}
