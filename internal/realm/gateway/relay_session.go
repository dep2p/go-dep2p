package gateway

import (
	"context"
	"fmt"
	"io"
	"sync/atomic"
	"time"

	"github.com/dep2p/go-dep2p/internal/realm/interfaces"
)

// ============================================================================
//                              中继会话
// ============================================================================

// RelaySession 中继会话
type RelaySession struct {
	// 会话信息
	id       string
	source   string
	target   string
	protocol string

	// 源连接（来自请求方的连接）
	sourceConn io.ReadWriteCloser

	// 时间统计
	startTime time.Time
	endTime   time.Time

	// 流量统计
	bytesSent atomic.Int64
	bytesRecv atomic.Int64

	// 状态
	closed atomic.Bool
}

// NewRelaySession 创建中继会话
func NewRelaySession(req *interfaces.RelayRequest) *RelaySession {
	return &RelaySession{
		id:        generateSessionID(),
		source:    req.SourcePeerID,
		target:    req.TargetPeerID,
		protocol:  req.Protocol,
		startTime: time.Now(),
	}
}

// NewRelaySessionWithConn 创建带源连接的中继会话
func NewRelaySessionWithConn(req *interfaces.RelayRequest, sourceConn io.ReadWriteCloser) *RelaySession {
	return &RelaySession{
		id:         generateSessionID(),
		source:     req.SourcePeerID,
		target:     req.TargetPeerID,
		protocol:   req.Protocol,
		sourceConn: sourceConn,
		startTime:  time.Now(),
	}
}

// SetSourceConn 设置源连接
func (s *RelaySession) SetSourceConn(conn io.ReadWriteCloser) {
	s.sourceConn = conn
}

// ============================================================================
//                              双向转发
// ============================================================================

// Transfer 执行双向转发
//
// 实现真正的双向流转发：
// - 使用两个 goroutine 并行转发数据
// - 统计发送和接收的字节数
// - 任一方向错误或 context 取消时终止
//
// 参数:
//   - conn: 目标节点连接（targetConn）
//
// 要求:
//   - 必须先通过 SetSourceConn 或 NewRelaySessionWithConn 设置源连接
func (s *RelaySession) Transfer(ctx context.Context, targetConn io.ReadWriteCloser) error {
	if targetConn == nil {
		return ErrNoConnection
	}

	if s.sourceConn == nil {
		return fmt.Errorf("source connection not set, call SetSourceConn first")
	}

	if s.closed.Load() {
		return ErrSessionClosed
	}

	// 创建带取消的上下文
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	errCh := make(chan error, 2)

	// goroutine 1: sourceConn -> targetConn（转发请求到目标）
	go func() {
		written, err := s.copyWithStats(ctx, targetConn, s.sourceConn, &s.bytesSent)
		if err != nil && err != io.EOF {
			errCh <- fmt.Errorf("forward error after %d bytes: %w", written, err)
		} else {
			errCh <- nil
		}
	}()

	// goroutine 2: targetConn -> sourceConn（返回响应给源）
	go func() {
		written, err := s.copyWithStats(ctx, s.sourceConn, targetConn, &s.bytesRecv)
		if err != nil && err != io.EOF {
			errCh <- fmt.Errorf("backward error after %d bytes: %w", written, err)
		} else {
			errCh <- nil
		}
	}()

	// 等待任一方向完成或出错
	var firstErr error
	for i := 0; i < 2; i++ {
		select {
		case err := <-errCh:
			if err != nil && firstErr == nil {
				firstErr = err
				cancel() // 取消另一个方向
			}
		case <-ctx.Done():
			if firstErr == nil {
				firstErr = ctx.Err()
			}
		}
	}

	s.endTime = time.Now()
	return firstErr
}

// copyWithStats 带统计的数据复制
//
// 实现零拷贝的流转发，同时统计传输字节数。
// 支持 context 取消以实现优雅关闭。
func (s *RelaySession) copyWithStats(ctx context.Context, dst io.Writer, src io.Reader, counter *atomic.Int64) (int64, error) {
	buf := make([]byte, 32*1024) // 32KB 缓冲区
	var total int64

	for {
		// 检查 context 是否已取消
		select {
		case <-ctx.Done():
			return total, ctx.Err()
		default:
		}

		// 读取数据
		nr, rerr := src.Read(buf)
		if nr > 0 {
			// 写入数据
			nw, werr := dst.Write(buf[:nr])
			if nw > 0 {
				total += int64(nw)
				counter.Add(int64(nw))
			}
			if werr != nil {
				return total, werr
			}
			if nr != nw {
				return total, io.ErrShortWrite
			}
		}
		// 处理读取错误（包括 EOF）
		if rerr != nil {
			return total, rerr
		}
	}
}

// ============================================================================
//                              统计信息
// ============================================================================

// GetStats 获取会话统计
func (s *RelaySession) GetStats() *interfaces.SessionStats {
	duration := time.Duration(0)
	if !s.endTime.IsZero() {
		duration = s.endTime.Sub(s.startTime)
	} else if s.closed.Load() {
		duration = time.Since(s.startTime)
	}

	return &interfaces.SessionStats{
		ID:        s.id,
		Source:    s.source,
		Target:    s.target,
		Protocol:  s.protocol,
		StartTime: s.startTime,
		BytesSent: s.bytesSent.Load(),
		BytesRecv: s.bytesRecv.Load(),
		Duration:  duration,
	}
}

// ID 返回会话 ID
func (s *RelaySession) ID() string {
	return s.id
}

// ============================================================================
//                              生命周期
// ============================================================================

// Close 关闭会话
func (s *RelaySession) Close() error {
	if s.closed.Load() {
		return nil
	}

	s.closed.Store(true)
	s.endTime = time.Now()

	return nil
}

// ============================================================================
//                              辅助函数
// ============================================================================

// generateSessionID 生成会话 ID
func generateSessionID() string {
	return fmt.Sprintf("session-%d", time.Now().UnixNano())
}

// 确保实现接口
var _ interfaces.RelaySession = (*RelaySession)(nil)
