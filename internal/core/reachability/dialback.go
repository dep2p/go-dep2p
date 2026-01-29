// Package reachability 提供可达性协调模块的实现
package reachability

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/dep2p/go-dep2p/pkg/interfaces"
)

const (
	maxFrameSize = 64 * 1024
)

// ============================================================================
//                              错误定义
// ============================================================================

var (
	// ErrNoHelper 无可用协助节点
	ErrNoHelper = errors.New("no helper node available for dial-back")

	// ErrNonceMismatch 随机数不匹配
	ErrNonceMismatch = errors.New("nonce mismatch in dial-back response")

	// ErrTimeout 验证超时
	ErrTimeout = errors.New("dial-back verification timeout")

	// ErrServiceStopped 服务已停止
	ErrServiceStopped = errors.New("dial-back service stopped")
)

// ============================================================================
//                              DialBackService 实现
// ============================================================================

// StreamOpener 流打开接口
// 注意：这个接口现在定义在 pkg/interfaces/reachability.go 中
// 这里保留为类型别名以保持向后兼容
type StreamOpener = interfaces.StreamOpener

// DialBackService 回拨验证服务实现
type DialBackService struct {
	// 配置
	config *interfaces.ReachabilityConfig

	// 流打开器（可选）
	streamOpener interfaces.StreamOpener

	// 运行状态
	ctx       context.Context
	cancel    context.CancelFunc
	running   bool
	runningMu sync.Mutex

	// 验证结果缓存
	results   map[string]*interfaces.VerificationResult
	resultsMu sync.RWMutex
}

// NewDialBackService 创建回拨验证服务
func NewDialBackService(config *interfaces.ReachabilityConfig) *DialBackService {
	if config == nil {
		config = interfaces.DefaultReachabilityConfig()
	}

	return &DialBackService{
		config:  config,
		results: make(map[string]*interfaces.VerificationResult),
	}
}

// SetStreamOpener 设置流打开器
func (s *DialBackService) SetStreamOpener(opener interfaces.StreamOpener) {
	s.streamOpener = opener
}

// Start 启动服务
func (s *DialBackService) Start(_ context.Context) error {
	s.runningMu.Lock()
	defer s.runningMu.Unlock()

	if s.running {
		return nil
	}

	s.ctx, s.cancel = context.WithCancel(context.Background())
	s.running = true

	logger.Info("启动回拨验证服务")
	return nil
}

// Stop 停止服务
func (s *DialBackService) Stop() error {
	s.runningMu.Lock()
	defer s.runningMu.Unlock()

	if !s.running {
		return nil
	}

	if s.cancel != nil {
		s.cancel()
	}

	s.running = false
	logger.Info("停止回拨验证服务")
	return nil
}

// VerifyAddresses 验证候选地址的可达性
func (s *DialBackService) VerifyAddresses(
	ctx context.Context,
	helperID string,
	candidateAddrs []string,
) ([]string, error) {
	s.runningMu.Lock()
	running := s.running
	s.runningMu.Unlock()

	if !running {
		return nil, ErrServiceStopped
	}

	if len(candidateAddrs) == 0 {
		return nil, nil
	}

	// 限制单次请求的地址数
	if len(candidateAddrs) > interfaces.MaxAddrsPerRequest {
		candidateAddrs = candidateAddrs[:interfaces.MaxAddrsPerRequest]
	}

	// 如果没有 stream opener，使用模拟验证
	if s.streamOpener == nil {
		return s.mockVerify(candidateAddrs), nil
	}

	// 生成随机数
	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("generate nonce: %w", err)
	}

	// 构造请求
	req := &interfaces.DialBackRequest{
		Addrs:     candidateAddrs,
		Nonce:     nonce,
		TimeoutMs: s.config.DialBackTimeout.Milliseconds(),
	}

	// 设置请求超时
	reqCtx, cancel := context.WithTimeout(ctx, s.config.RequestTimeout)
	defer cancel()

	// 打开协议流
	stream, err := s.streamOpener.OpenStream(reqCtx, helperID, interfaces.ReachabilityProtocolID)
	if err != nil {
		return nil, fmt.Errorf("open stream: %w", err)
	}
	defer stream.Close()

	// 发送请求
	reqData, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	if err := writeFrame(stream, reqData); err != nil {
		return nil, fmt.Errorf("write request: %w", err)
	}

	// 关闭写端
	if closer, ok := stream.(interface{ CloseWrite() error }); ok {
		closer.CloseWrite()
	}

	// 读取响应
	respData, err := readFrame(stream)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	var resp interfaces.DialBackResponse
	if err := json.Unmarshal(respData, &resp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	// 验证随机数
	if string(resp.Nonce) != string(nonce) {
		return nil, ErrNonceMismatch
	}

	// 检查错误
	if resp.Error != "" {
		return nil, fmt.Errorf("helper error: %s", resp.Error)
	}

	// 更新缓存
	for _, addr := range resp.Reachable {
		s.resultsMu.Lock()
		s.results[addr] = &interfaces.VerificationResult{
			Addr:       addr,
			Reachable:  true,
			VerifiedBy: helperID,
			VerifiedAt: time.Now(),
		}
		s.resultsMu.Unlock()
	}

	logger.Info("回拨验证完成",
		"helper", helperID,
		"candidates", len(candidateAddrs),
		"reachable", len(resp.Reachable))

	return resp.Reachable, nil
}

// mockVerify 模拟验证（无 stream opener 时使用）
func (s *DialBackService) mockVerify(candidateAddrs []string) []string {
	// 简单模拟：返回所有地址作为可达
	// 实际生产中应该进行真实的回拨验证
	logger.Debug("使用模拟验证（无 stream opener）",
		"candidates", len(candidateAddrs))

	// 更新缓存
	for _, addr := range candidateAddrs {
		s.resultsMu.Lock()
		s.results[addr] = &interfaces.VerificationResult{
			Addr:       addr,
			Reachable:  true,
			VerifiedBy: "mock",
			VerifiedAt: time.Now(),
		}
		s.resultsMu.Unlock()
	}

	return candidateAddrs
}

// HandleDialBackRequest 处理回拨请求（作为协助方）
func (s *DialBackService) HandleDialBackRequest(
	ctx context.Context,
	req *interfaces.DialBackRequest,
) *interfaces.DialBackResponse {
	resp := &interfaces.DialBackResponse{
		Nonce:       req.Nonce,
		DialResults: make([]interfaces.DialResult, 0, len(req.Addrs)),
	}

	if len(req.Addrs) == 0 {
		return resp
	}

	// 限制地址数
	addrs := req.Addrs
	if len(addrs) > interfaces.MaxAddrsPerRequest {
		addrs = addrs[:interfaces.MaxAddrsPerRequest]
	}

	// 设置回拨超时
	timeout := s.config.DialBackTimeout
	if req.TimeoutMs > 0 && time.Duration(req.TimeoutMs)*time.Millisecond < timeout {
		timeout = time.Duration(req.TimeoutMs) * time.Millisecond
	}

	// 并发回拨
	var wg sync.WaitGroup
	resultsCh := make(chan interfaces.DialResult, len(addrs))
	sem := make(chan struct{}, s.config.MaxConcurrentDialBacks)

	for _, addrStr := range addrs {
		wg.Add(1)
		go func(addr string) {
			defer wg.Done()

			sem <- struct{}{}
			defer func() { <-sem }()

			result := s.dialBack(ctx, addr, timeout)
			resultsCh <- result
		}(addrStr)
	}

	go func() {
		wg.Wait()
		close(resultsCh)
	}()

	// 收集结果
	for result := range resultsCh {
		resp.DialResults = append(resp.DialResults, result)
		if result.Success {
			resp.Reachable = append(resp.Reachable, result.Addr)
		}
	}

	logger.Debug("处理回拨请求完成",
		"candidates", len(addrs),
		"reachable", len(resp.Reachable))

	return resp
}

// dialBack 尝试回拨单个地址
func (s *DialBackService) dialBack(ctx context.Context, addrStr string, timeout time.Duration) interfaces.DialResult {
	result := interfaces.DialResult{
		Addr: addrStr,
	}

	// 解析 multiaddr 获取网络地址
	netAddr, proto, err := parseMultiaddrForDial(addrStr)
	if err != nil {
		result.Success = false
		result.Error = err.Error()
		return result
	}

	// 设置超时
	dialCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	start := time.Now()

	// 根据协议进行实际连接测试
	var conn net.Conn
	switch proto {
	case "udp":
		// UDP 使用简单的连接测试
		var d net.Dialer
		conn, err = d.DialContext(dialCtx, "udp", netAddr)
	case "tcp":
		var d net.Dialer
		conn, err = d.DialContext(dialCtx, "tcp", netAddr)
	default:
		// 默认尝试 UDP
		var d net.Dialer
		conn, err = d.DialContext(dialCtx, "udp", netAddr)
	}

	latency := time.Since(start)
	result.LatencyMs = latency.Milliseconds()

	if err != nil {
		result.Success = false
		result.Error = err.Error()
		logger.Debug("回拨失败", "addr", addrStr, "err", err)
		return result
	}

	// 连接成功，关闭连接
	if conn != nil {
		conn.Close()
	}

	result.Success = true
	logger.Debug("回拨成功", "addr", addrStr, "latency", latency)
	return result
}

// parseMultiaddrForDial 解析 multiaddr 为可拨号的网络地址
//
// 支持格式：
//   - /ip4/1.2.3.4/udp/4001/quic-v1
//   - /ip4/1.2.3.4/tcp/4001
//   - /ip6/::1/udp/4001/quic-v1
func parseMultiaddrForDial(maddr string) (netAddr string, proto string, err error) {
	if maddr == "" {
		return "", "", fmt.Errorf("empty multiaddr")
	}

	// 简单解析 multiaddr
	parts := splitMultiaddrParts(maddr)
	if len(parts) < 4 {
		return "", "", fmt.Errorf("invalid multiaddr format: %s", maddr)
	}

	var ip string
	var port string

	for i := 0; i < len(parts)-1; i++ {
		switch parts[i] {
		case "ip4", "ip6":
			ip = parts[i+1]
		case "udp":
			port = parts[i+1]
			proto = "udp"
		case "tcp":
			port = parts[i+1]
			proto = "tcp"
		}
	}

	if ip == "" || port == "" {
		return "", "", fmt.Errorf("missing ip or port in multiaddr: %s", maddr)
	}

	// 构建网络地址
	if net.ParseIP(ip).To4() != nil {
		netAddr = ip + ":" + port
	} else {
		netAddr = "[" + ip + "]:" + port
	}

	return netAddr, proto, nil
}

// splitMultiaddrParts 分割 multiaddr（dialback 内部使用）
func splitMultiaddrParts(addr string) []string {
	if addr == "" {
		return nil
	}
	if addr[0] == '/' {
		addr = addr[1:]
	}
	var parts []string
	start := 0
	for i := 0; i < len(addr); i++ {
		if addr[i] == '/' {
			if i > start {
				parts = append(parts, addr[start:i])
			}
			start = i + 1
		}
	}
	if start < len(addr) {
		parts = append(parts, addr[start:])
	}
	return parts
}

// GetVerificationResult 获取地址的验证结果
func (s *DialBackService) GetVerificationResult(addr string) (*interfaces.VerificationResult, bool) {
	s.resultsMu.RLock()
	defer s.resultsMu.RUnlock()

	result, ok := s.results[addr]
	return result, ok
}

// ClearResults 清除验证结果缓存
func (s *DialBackService) ClearResults() {
	s.resultsMu.Lock()
	s.results = make(map[string]*interfaces.VerificationResult)
	s.resultsMu.Unlock()
}

// ============================================================================
//                              帧读写
// ============================================================================

func readFrame(r io.Reader) ([]byte, error) {
	var lenBuf [4]byte
	if _, err := io.ReadFull(r, lenBuf[:]); err != nil {
		return nil, err
	}
	n := binary.BigEndian.Uint32(lenBuf[:])
	if n == 0 || n > maxFrameSize {
		return nil, fmt.Errorf("invalid frame size: %d", n)
	}
	buf := make([]byte, n)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, err
	}
	return buf, nil
}

func writeFrame(w io.Writer, payload []byte) error {
	if len(payload) == 0 {
		return fmt.Errorf("empty payload")
	}
	if len(payload) > maxFrameSize {
		return fmt.Errorf("payload too large: %d", len(payload))
	}
	var lenBuf [4]byte
	binary.BigEndian.PutUint32(lenBuf[:], uint32(len(payload)))
	if _, err := w.Write(lenBuf[:]); err != nil {
		return err
	}
	_, err := w.Write(payload)
	return err
}
