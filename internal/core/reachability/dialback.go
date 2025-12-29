// Package reachability 提供可达性验证的实现
package reachability

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/dep2p/go-dep2p/internal/core/address"
	"github.com/dep2p/go-dep2p/pkg/interfaces/endpoint"
	reachabilityif "github.com/dep2p/go-dep2p/pkg/interfaces/reachability"
	"github.com/dep2p/go-dep2p/pkg/types"
)

const (
	maxFrameSize = 64 * 1024
)

type outboundHandshakeVerifier interface {
	// VerifyOutboundHandshake 尝试对目标地址发起 dep2p Dial+Handshake，并验证对端身份与 nodeID 匹配。
	//
	// 注意：该验证必须绕开“已有连接直接复用”的逻辑，否则无法验证候选地址。
	VerifyOutboundHandshake(ctx context.Context, nodeID types.NodeID, addr endpoint.Address) (time.Duration, error)
}

// dialBackEndpoint 是 dial-back 所需的最小 Endpoint 能力集合。
type dialBackEndpoint interface {
	Connect(ctx context.Context, nodeID endpoint.NodeID) (endpoint.Connection, error)
	Connection(nodeID endpoint.NodeID) (endpoint.Connection, bool)

	SetProtocolHandler(protocolID endpoint.ProtocolID, handler endpoint.ProtocolHandler)
	RemoveProtocolHandler(protocolID endpoint.ProtocolID)
}

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

// DialBackService 回拨验证服务实现
type DialBackService struct {
	// 依赖组件
	endpoint dialBackEndpoint
	verifier outboundHandshakeVerifier

	// 配置
	config *reachabilityif.Config

	// 运行状态
	ctx       context.Context
	cancel    context.CancelFunc
	running   bool
	runningMu sync.Mutex

	// 验证结果缓存
	results   map[string]*reachabilityif.VerificationResult
	resultsMu sync.RWMutex
}

// VerifyAddressesWithHelperPool 使用混合 helper 池执行 dial-back 验证并做阈值聚合
//
// helper 选择策略：
// - 优先 trustedHelpers（配置的可信 helper）
// - 不足时退化到 connectedPeers（当前已连接 peers）
//
// 聚合策略：
// - 同一地址被 >= MinVerifications 个 helper 回拨成功，才认为“已验证可达”
func (s *DialBackService) VerifyAddressesWithHelperPool(
	ctx context.Context,
	trustedHelpers []types.NodeID,
	connectedPeers []endpoint.Connection,
	candidateAddrs []endpoint.Address,
) ([]endpoint.Address, error) {
	// 组装 helper 列表（去重）
	seen := make(map[types.NodeID]struct{})
	helpers := make([]types.NodeID, 0, len(trustedHelpers)+len(connectedPeers))

	for _, id := range trustedHelpers {
		if id.Equal(types.EmptyNodeID) {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		helpers = append(helpers, id)
	}

	for _, c := range connectedPeers {
		if c == nil {
			continue
		}
		id := c.RemoteID()
		if id.Equal(types.EmptyNodeID) {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		helpers = append(helpers, id)
	}

	if len(helpers) == 0 {
		return nil, ErrNoHelper
	}

	// 阈值：至少 1
	minV := s.config.MinVerifications
	if minV <= 0 {
		minV = 1
	}

	// 并发请求多个 helper
	type helperResult struct {
		reachable []endpoint.Address
		err       error
	}

	sem := make(chan struct{}, 3) // 控制并发，避免放大网络压力
	resultsCh := make(chan helperResult, len(helpers))

	for _, helperID := range helpers {
		helperID := helperID
		go func() {
			sem <- struct{}{}
			defer func() { <-sem }()

			reachable, err := s.VerifyAddresses(ctx, helperID, candidateAddrs)
			resultsCh <- helperResult{reachable: reachable, err: err}
		}()
	}

	// 聚合 reachable 计数
	addrByKey := make(map[string]endpoint.Address, len(candidateAddrs))
	for _, a := range candidateAddrs {
		if a == nil {
			continue
		}
		addrByKey[a.String()] = a
	}

	counts := make(map[string]int)
	var errs []error

	for i := 0; i < len(helpers); i++ {
		r := <-resultsCh
		if r.err != nil {
			errs = append(errs, r.err)
			continue
		}
		for _, a := range r.reachable {
			if a == nil {
				continue
			}
			counts[a.String()]++
		}
	}

	// 选择达到阈值的地址
	out := make([]endpoint.Address, 0, len(counts))
	for key, n := range counts {
		if n >= minV {
			if a, ok := addrByKey[key]; ok && a != nil {
				out = append(out, a)
			}
		}
	}

	if len(out) == 0 && len(errs) > 0 {
		// 全部失败：返回一个代表性的错误
		return nil, errs[0]
	}

	return out, nil
}

// NewDialBackService 创建回拨验证服务
func NewDialBackService(ep dialBackEndpoint, config *reachabilityif.Config) *DialBackService {
	if config == nil {
		config = reachabilityif.DefaultConfig()
	}

	var verifier outboundHandshakeVerifier
	if ep != nil {
		if v, ok := ep.(outboundHandshakeVerifier); ok {
			verifier = v
		}
	}

	return &DialBackService{
		endpoint: ep,
		verifier: verifier,
		config:   config,
		results:  make(map[string]*reachabilityif.VerificationResult),
	}
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

	// 注册协议处理器（作为协助方）
	if s.config.EnableAsHelper && s.endpoint != nil {
		s.endpoint.SetProtocolHandler(reachabilityif.ProtocolID, s.handleStream)
		log.Info("启动回拨验证服务（作为协助方）")
	}

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

	// 移除协议处理器
	if s.endpoint != nil {
		s.endpoint.RemoveProtocolHandler(reachabilityif.ProtocolID)
	}

	s.running = false
	log.Info("停止回拨验证服务")
	return nil
}

// VerifyAddresses 验证候选地址的可达性
func (s *DialBackService) VerifyAddresses(
	ctx context.Context,
	helperID types.NodeID,
	candidateAddrs []endpoint.Address,
) ([]endpoint.Address, error) {
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
	if len(candidateAddrs) > reachabilityif.MaxAddrsPerRequest {
		candidateAddrs = candidateAddrs[:reachabilityif.MaxAddrsPerRequest]
	}

	// 生成随机数
	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("generate nonce: %w", err)
	}

	// 构造请求
	addrs := make([]string, len(candidateAddrs))
	for i, addr := range candidateAddrs {
		addrs[i] = addr.String()
	}

	req := &reachabilityif.DialBackRequest{
		Addrs:     addrs,
		Nonce:     nonce,
		TimeoutMs: s.config.DialBackTimeout.Milliseconds(),
	}

	// 设置请求超时
	reqCtx, cancel := context.WithTimeout(ctx, s.config.RequestTimeout)
	defer cancel()

	// 连接协助节点
	// 重要：dial-back 不应关闭连接，无论是复用的还是新建的。
	// 原因：
	// 1. 复用的连接：承载其他业务流（chat/realm/dht），关闭会导致 muxer closed 级联断连
	// 2. 新建的连接：由 Endpoint 的 connManager 通过空闲超时/水位线统一管理生命周期
	// 3. 业务协议不应决定连接生命周期，避免 TOCTOU 竞态条件
	conn, err := s.endpoint.Connect(reqCtx, helperID)
	if err != nil {
		return nil, fmt.Errorf("connect to helper %s: %w", helperID, err)
	}

	// 打开协议流
	stream, err := conn.OpenStream(reqCtx, reachabilityif.ProtocolID)
	if err != nil {
		return nil, fmt.Errorf("open stream: %w", err)
	}
	defer func() { _ = stream.Close() }()

	// 发送请求
	reqData, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	if err := writeFrame(stream, reqData); err != nil {
		return nil, fmt.Errorf("write request: %w", err)
	}

	// 关闭写端，表示请求发送完成
	if closer, ok := stream.(interface{ CloseWrite() error }); ok {
		closer.CloseWrite()
	}

	// 读取响应
	respData, err := readFrame(stream)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	var resp reachabilityif.DialBackResponse
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

	// 解析可达地址
	var reachable []endpoint.Address
	reachableSet := make(map[string]bool)
	for _, addrStr := range resp.Reachable {
		reachableSet[addrStr] = true
	}

	for _, addr := range candidateAddrs {
		if reachableSet[addr.String()] {
			reachable = append(reachable, addr)

			// 更新缓存
			s.resultsMu.Lock()
			s.results[addr.String()] = &reachabilityif.VerificationResult{
				Addr:       addr,
				Reachable:  true,
				VerifiedBy: helperID,
				VerifiedAt: time.Now(),
			}
			s.resultsMu.Unlock()
		}
	}

	log.Info("回拨验证完成",
		"helper", helperID,
		"candidates", len(candidateAddrs),
		"reachable", len(reachable))

	return reachable, nil
}

// HandleDialBackRequest 处理回拨请求（作为协助方）
func (s *DialBackService) HandleDialBackRequest(
	ctx context.Context,
	req *reachabilityif.DialBackRequest,
) *reachabilityif.DialBackResponse {
	resp := &reachabilityif.DialBackResponse{
		Nonce:       req.Nonce,
		DialResults: make([]reachabilityif.DialResult, 0, len(req.Addrs)),
	}

	if len(req.Addrs) == 0 {
		return resp
	}

	// 限制地址数
	addrs := req.Addrs
	if len(addrs) > reachabilityif.MaxAddrsPerRequest {
		addrs = addrs[:reachabilityif.MaxAddrsPerRequest]
	}

	// 设置回拨超时
	timeout := s.config.DialBackTimeout
	if req.TimeoutMs > 0 && time.Duration(req.TimeoutMs)*time.Millisecond < timeout {
		timeout = time.Duration(req.TimeoutMs) * time.Millisecond
	}

	// 并发回拨
	var wg sync.WaitGroup
	resultsCh := make(chan reachabilityif.DialResult, len(addrs))
	sem := make(chan struct{}, s.config.MaxConcurrentDialBacks)

	for _, addrStr := range addrs {
		wg.Add(1)
		go func(addr string) {
			defer wg.Done()

			sem <- struct{}{}        // 获取信号量
			defer func() { <-sem }() // 释放信号量

			// requesterID 由协议 handler 在调用时注入（见 handleStream）
			result := s.dialBack(ctx, types.EmptyNodeID, addr, timeout)
			resultsCh <- result
		}(addrStr)
	}

	// 等待所有回拨完成
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

	log.Debug("处理回拨请求完成",
		"candidates", len(addrs),
		"reachable", len(resp.Reachable))

	return resp
}

// dialBack 尝试回拨单个地址（Dial + dep2p Handshake）
func (s *DialBackService) dialBack(ctx context.Context, requesterID types.NodeID, addrStr string, timeout time.Duration) reachabilityif.DialResult {
	result := reachabilityif.DialResult{
		Addr: addrStr,
	}

	dialCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	if s.verifier == nil {
		result.Error = "outbound handshake verifier not available"
		return result
	}

	// 使用统一地址实现作为拨号输入
	addr := address.NewAddr(types.Multiaddr(addrStr))
	latency, err := s.verifier.VerifyOutboundHandshake(dialCtx, requesterID, addr)
	if err != nil {
		result.Error = fmt.Sprintf("dial+handshake: %v", err)
		return result
	}

	result.Success = true
	result.LatencyMs = latency.Milliseconds()

	return result
}

// handleStream 处理入站协议流
func (s *DialBackService) handleStream(stream endpoint.Stream) {
	defer func() { _ = stream.Close() }()

	// 读取请求
	reqData, err := readFrame(stream)
	if err != nil {
		log.Debug("读取回拨请求失败", "err", err)
		return
	}

	var req reachabilityif.DialBackRequest
	if err := json.Unmarshal(reqData, &req); err != nil {
		log.Debug("解析回拨请求失败", "err", err)
		return
	}

	// 处理请求：协助方回拨的目标是“当前流的对端”（Requester）
	requesterID := stream.Connection().RemoteID()

	// 将 requesterID 注入 dialBack 过程：复用 HandleDialBackRequest 的并发框架，
	// 但在 dialBack 执行时使用 requesterID 做身份校验。
	resp := &reachabilityif.DialBackResponse{
		Nonce:       req.Nonce,
		DialResults: make([]reachabilityif.DialResult, 0, len(req.Addrs)),
	}

	// 限制地址数
	addrs := req.Addrs
	if len(addrs) > reachabilityif.MaxAddrsPerRequest {
		addrs = addrs[:reachabilityif.MaxAddrsPerRequest]
	}

	// 设置回拨超时
	timeout := s.config.DialBackTimeout
	if req.TimeoutMs > 0 && time.Duration(req.TimeoutMs)*time.Millisecond < timeout {
		timeout = time.Duration(req.TimeoutMs) * time.Millisecond
	}

	var wg sync.WaitGroup
	resultsCh := make(chan reachabilityif.DialResult, len(addrs))
	sem := make(chan struct{}, s.config.MaxConcurrentDialBacks)

	for _, addrStr := range addrs {
		wg.Add(1)
		go func(addr string) {
			defer wg.Done()

			sem <- struct{}{}
			defer func() { <-sem }()

			resultsCh <- s.dialBack(s.ctx, requesterID, addr, timeout)
		}(addrStr)
	}
	go func() {
		wg.Wait()
		close(resultsCh)
	}()

	for r := range resultsCh {
		resp.DialResults = append(resp.DialResults, r)
		if r.Success {
			resp.Reachable = append(resp.Reachable, r.Addr)
		}
	}

	// 发送响应
	respData, err := json.Marshal(resp)
	if err != nil {
		log.Debug("序列化回拨响应失败", "err", err)
		return
	}

	if err := writeFrame(stream, respData); err != nil {
		log.Debug("发送回拨响应失败", "err", err)
		return
	}
}

// stringAddress 已删除，统一使用 address.Addr

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

// GetVerificationResult 获取地址的验证结果
func (s *DialBackService) GetVerificationResult(addr endpoint.Address) (*reachabilityif.VerificationResult, bool) {
	s.resultsMu.RLock()
	defer s.resultsMu.RUnlock()

	result, ok := s.results[addr.String()]
	return result, ok
}

// ClearResults 清除验证结果缓存
func (s *DialBackService) ClearResults() {
	s.resultsMu.Lock()
	s.results = make(map[string]*reachabilityif.VerificationResult)
	s.resultsMu.Unlock()
}

