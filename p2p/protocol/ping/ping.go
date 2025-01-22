package ping

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"io"
	mrand "math/rand"
	"time"

	"github.com/dep2p/go-dep2p/core/host"
	"github.com/dep2p/go-dep2p/core/network"
	"github.com/dep2p/go-dep2p/core/peer"
	pool "github.com/dep2p/go-dep2p/p2plib/buffer/pool"
	logging "github.com/dep2p/log"
)

// 用于记录日志的 logger 实例
var log = logging.Logger("p2p-protocol-ping")

const (
	// ping 消息的大小(字节)
	PingSize = 32
	// ping 超时时间
	pingTimeout = 10 * time.Second
	// ping 持续时间
	pingDuration = 30 * time.Second

	// ping 协议标识符
	ID = "/ipfs/ping/1.0.0"

	// ping 服务名称
	ServiceName = "dep2p.ping"
)

// PingService ping 服务结构体
type PingService struct {
	// dep2p 主机实例
	Host host.Host
}

// NewPingService 创建一个新的 ping 服务
// 参数:
//   - h: host.Host dep2p 主机实例
//
// 返回值:
//   - *PingService ping 服务实例
func NewPingService(h host.Host) *PingService {
	ps := &PingService{h}
	// 设置 ping 协议的流处理器
	h.SetStreamHandler(ID, ps.PingHandler)
	return ps
}

// PingHandler 处理接收到的 ping 请求
// 参数:
//   - s: network.Stream 网络流对象
func (p *PingService) PingHandler(s network.Stream) {
	// 设置流的服务名称
	if err := s.Scope().SetService(ServiceName); err != nil {
		log.Debugf("为 ping 流设置服务时出错: %s", err)
		s.Reset()
		return
	}

	// 为 ping 流预留内存
	if err := s.Scope().ReserveMemory(PingSize, network.ReservationPriorityAlways); err != nil {
		log.Debugf("为 ping 流预留内存时出错: %s", err)
		s.Reset()
		return
	}
	defer s.Scope().ReleaseMemory(PingSize)

	// 设置流的截止时间
	s.SetDeadline(time.Now().Add(pingDuration))

	// 从内存池获取缓冲区
	buf := pool.Get(PingSize)
	defer pool.Put(buf)

	// 创建错误通道
	errCh := make(chan error, 1)
	defer close(errCh)
	timer := time.NewTimer(pingTimeout)
	defer timer.Stop()

	// 启动超时检查协程
	go func() {
		select {
		case <-timer.C:
			log.Debugf("ping 超时")
		case err, ok := <-errCh:
			if ok {
				log.Debugf("ping 循环失败: %s", err)
			} else {
				log.Debugf("ping 循环失败且无错误")
			}
		}
		s.Close()
	}()

	// ping 循环
	for {
		// 读取 ping 消息
		_, err := io.ReadFull(s, buf)
		if err != nil {
			errCh <- err
			return
		}

		// 回写 ping 消息
		_, err = s.Write(buf)
		if err != nil {
			errCh <- err
			return
		}

		// 重置超时计时器
		timer.Reset(pingTimeout)
	}
}

// Result ping 操作的结果
type Result struct {
	// RTT 往返时间
	RTT time.Duration
	// Error 错误信息
	Error error
}

// Ping 向指定节点发送 ping 请求
// 参数:
//   - ctx: context.Context 上下文对象
//   - p: peer.ID 目标节点 ID
//
// 返回值:
//   - <-chan Result ping 结果通道
func (ps *PingService) Ping(ctx context.Context, p peer.ID) <-chan Result {
	return Ping(ctx, ps.Host, p)
}

// pingError 创建一个包含错误的结果通道
// 参数:
//   - err: error 错误信息
//
// 返回值:
//   - chan Result 结果通道
func pingError(err error) chan Result {
	ch := make(chan Result, 1)
	ch <- Result{Error: err}
	close(ch)
	return ch
}

// Ping 持续向远程节点发送 ping 请求直到上下文被取消
// 参数:
//   - ctx: context.Context 上下文对象
//   - h: host.Host dep2p 主机实例
//   - p: peer.ID 目标节点 ID
//
// 返回值:
//   - <-chan Result RTT 或错误的结果流
func Ping(ctx context.Context, h host.Host, p peer.ID) <-chan Result {
	// 创建新的流
	s, err := h.NewStream(network.WithAllowLimitedConn(ctx, "ping"), p, ID)
	if err != nil {
		log.Debugf("创建 ping 流失败: %s", err)
		return pingError(err)
	}

	// 设置流的服务名称
	if err := s.Scope().SetService(ServiceName); err != nil {
		log.Debugf("为 ping 流设置服务时出错: %s", err)
		s.Reset()
		return pingError(err)
	}

	// 生成随机数种子
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		log.Errorf("获取加密随机数失败: %s", err)
		s.Reset()
		return pingError(err)
	}
	ra := mrand.New(mrand.NewSource(int64(binary.BigEndian.Uint64(b))))

	ctx, cancel := context.WithCancel(ctx)

	// 创建结果通道
	out := make(chan Result)
	go func() {
		defer close(out)
		defer cancel()

		for ctx.Err() == nil {
			var res Result
			res.RTT, res.Error = ping(s, ra)

			// 如果已取消，忽略所有结果
			if ctx.Err() != nil {
				return
			}

			// 如果没有错误，记录 RTT
			if res.Error == nil {
				h.Peerstore().RecordLatency(p, res.RTT)
			}

			select {
			case out <- res:
			case <-ctx.Done():
				return
			}
		}
	}()
	context.AfterFunc(ctx, func() {
		// 强制中止 ping
		s.Reset()
	})

	return out
}

// ping 执行单次 ping 操作
// 参数:
//   - s: network.Stream 网络流对象
//   - randReader: io.Reader 随机数读取器
//
// 返回值:
//   - time.Duration RTT 时间
//   - error 错误信息
func ping(s network.Stream, randReader io.Reader) (time.Duration, error) {
	// 为 ping 流预留内存
	if err := s.Scope().ReserveMemory(2*PingSize, network.ReservationPriorityAlways); err != nil {
		log.Debugf("为 ping 流预留内存时出错: %s", err)
		s.Reset()
		return 0, err
	}
	defer s.Scope().ReleaseMemory(2 * PingSize)

	// 从内存池获取发送缓冲区
	buf := pool.Get(PingSize)
	defer pool.Put(buf)

	// 生成随机数据
	if _, err := io.ReadFull(randReader, buf); err != nil {
		log.Debugf("生成随机数据失败: %s", err)
		s.Reset()
		return 0, err
	}

	// 记录发送时间并发送数据
	before := time.Now()
	if _, err := s.Write(buf); err != nil {
		log.Debugf("发送 ping 数据失败: %s", err)
		s.Reset()
		return 0, err
	}

	// 从内存池获取接收缓冲区
	rbuf := pool.Get(PingSize)
	defer pool.Put(rbuf)

	// 读取响应数据
	if _, err := io.ReadFull(s, rbuf); err != nil {
		log.Debugf("读取 ping 响应数据失败: %s", err)
		s.Reset()
		return 0, err
	}

	// 验证响应数据是否正确
	if !bytes.Equal(buf, rbuf) {
		log.Debugf("ping 数据包不正确")
		s.Reset()
		return 0, errors.New("ping 数据包不正确")
	}

	return time.Since(before), nil
}
