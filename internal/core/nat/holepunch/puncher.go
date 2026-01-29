package holepunch

import (
	"context"
	"sync"
	"time"

	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/lib/log"
	"github.com/dep2p/go-dep2p/pkg/lib/proto/holepunch"
	"github.com/dep2p/go-dep2p/pkg/protocol"
	"google.golang.org/protobuf/proto"
)

var logger = log.Logger("core/nat/holepunch")

// HolePunchProtocol 打洞协议 ID（使用统一定义）
var HolePunchProtocol = string(protocol.HolePunch)

// 打洞相关常量
const (
	// DirectDialTimeout 单次直连拨号超时
	DirectDialTimeout = 5 * time.Second

	// MaxHolePunchRetries 打洞最大重试次数
	// 增加重试次数可以提高打洞成功率，特别是对于 NAT 映射需要时间建立的场景
	MaxHolePunchRetries = 3

	// HolePunchRetryDelay 打洞重试间隔
	// 短暂延迟让 NAT 映射有机会建立
	HolePunchRetryDelay = 500 * time.Millisecond
)

// HolePuncher 打洞协调器
//
// 实现 DCUtR (Direct Connection Upgrade through Relay) 协议：
//   - 通过中继连接协商双方的公网地址
//   - 双方同时向对方发起连接，打通 NAT
type HolePuncher struct {
	mu     sync.RWMutex
	active map[string]struct{} // 活跃的打洞尝试

	Swarm pkgif.Swarm
	Host  pkgif.Host

	// DirectDialer 用于直接拨号（不经过 Swarm.DialPeer，避免递归）
	DirectDialer DirectDialer
}

// DirectDialer 定义直接拨号接口
//
// 用于打洞时的同时拨号，不经过 Swarm.DialPeer 的完整流程
type DirectDialer interface {
	// DialDirect 直接拨号到指定地址
	DialDirect(ctx context.Context, peerID string, addr string) (pkgif.Connection, error)
}

// NewHolePuncher 创建打洞器
func NewHolePuncher(swarm pkgif.Swarm, host pkgif.Host) *HolePuncher {
	return &HolePuncher{
		active: make(map[string]struct{}),
		Swarm:  swarm,
		Host:   host,
	}
}

// SetDirectDialer 设置直接拨号器
func (h *HolePuncher) SetDirectDialer(dialer DirectDialer) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.DirectDialer = dialer
}

// DirectConnect 尝试直连节点
//
// 完整实现 DCUtR (Direct Connection Upgrade through Relay) 协议：
//  1. 通过中继建立协商流
//  2. 交换观察地址（CONNECT 消息）
//  3. 同步时机（SYNC 消息）
//  4. 同时尝试直连所有观测地址
//  5. 等待连接建立
//
// hintAddrs 参数是可选提示，不再是必需参数。
// 打洞协议通过 CONNECT 消息交换双方的观测地址（ShareableAddrs），
// 而不是依赖 Peerstore 中的 directAddrs。
func (h *HolePuncher) DirectConnect(ctx context.Context, peerID string, hintAddrs []string) error {
	peerShort := peerID
	if len(peerShort) > 8 {
		peerShort = peerShort[:8]
	}

	// hintAddrs 是可选提示，不再要求非空
	// 打洞协议会通过 CONNECT 消息交换双方的观测地址
	logger.Info("开始 HolePunch 打洞协商",
		"peerID", peerShort,
		"hintAddrs", len(hintAddrs))

	// 检查是否已有活跃打洞
	h.mu.RLock()
	_, exists := h.active[peerID]
	h.mu.RUnlock()

	if exists {
		logger.Debug("已有活跃打洞，跳过", "peerID", peerShort)
		return ErrHolePunchActive
	}

	// 标记为活跃
	h.MarkActive(peerID)
	defer h.ClearActive(peerID)

	// 1. 通过中继建立协商流
	logger.Info("打开 HolePunch 协商流", "peerID", peerShort)
	stream, err := h.openRelayStream(ctx, peerID)
	if err != nil {
		logger.Warn("打开协商流失败", "peerID", peerShort, "error", err)
		return &HolePunchError{
			Message: "failed to open relay stream",
			Cause:   err,
		}
	}
	defer stream.Close()

	// 2. 发送 CONNECT 消息，包含本地观测地址
	// 使用 getObservedAddrs() 获取 ShareableAddrs，而非 Peerstore 地址
	localObsAddrs := h.getObservedAddrs()
	logger.Info("发送 CONNECT 消息",
		"peerID", peerShort,
		"localAddrsCount", len(localObsAddrs),
		"localAddrs", h.addrsToStrings(localObsAddrs))

	connectMsg := &holepunch.HolePunch{
		Type:     holepunch.Type_CONNECT,
		ObsAddrs: localObsAddrs,
	}

	if err := h.writeMessage(stream, connectMsg); err != nil {
		logger.Warn("发送 CONNECT 失败", "peerID", peerShort, "error", err)
		return &HolePunchError{
			Message: "failed to send CONNECT",
			Cause:   err,
		}
	}

	// 3. 读取对方的 CONNECT 响应
	response, err := h.readMessage(stream)
	if err != nil {
		logger.Warn("读取 CONNECT 响应失败", "peerID", peerShort, "error", err)
		return &HolePunchError{
			Message: "failed to read CONNECT response",
			Cause:   err,
		}
	}

	if response.Type != holepunch.Type_CONNECT {
		logger.Warn("收到非 CONNECT 消息", "peerID", peerShort, "type", response.Type)
		return &HolePunchError{
			Message: "unexpected message type",
		}
	}

	remoteAddrs := make([]string, len(response.ObsAddrs))
	for i, addr := range response.ObsAddrs {
		remoteAddrs[i] = string(addr)
	}
	logger.Info("收到对方观测地址",
		"peerID", peerShort,
		"remoteAddrsCount", len(remoteAddrs),
		"remoteAddrs", remoteAddrs)

	// 4. 发送 SYNC 消息
	logger.Debug("发送 SYNC 消息", "peerID", peerShort)
	syncMsg := &holepunch.HolePunch{
		Type: holepunch.Type_SYNC,
	}

	if err := h.writeMessage(stream, syncMsg); err != nil {
		logger.Warn("发送 SYNC 失败", "peerID", peerShort, "error", err)
		return &HolePunchError{
			Message: "failed to send SYNC",
			Cause:   err,
		}
	}

	// 5. 等待对方 SYNC
	syncResponse, err := h.readMessage(stream)
	if err != nil {
		logger.Warn("读取 SYNC 响应失败", "peerID", peerShort, "error", err)
		return &HolePunchError{
			Message: "failed to read SYNC response",
			Cause:   err,
		}
	}

	if syncResponse.Type != holepunch.Type_SYNC {
		logger.Warn("收到非 SYNC 消息", "peerID", peerShort, "type", syncResponse.Type)
		return &HolePunchError{
			Message: "unexpected SYNC message type",
		}
	}

	logger.Info("SYNC 完成，开始同时拨号打洞",
		"peerID", peerShort,
		"targetAddrsCount", len(remoteAddrs),
		"targetAddrs", remoteAddrs)

	// 6. 同时尝试直连所有观测地址（这才是真正的打洞！）
	return h.simultaneousDial(ctx, peerID, remoteAddrs)
}

// openRelayStream 通过中继打开流
func (h *HolePuncher) openRelayStream(ctx context.Context, peerID string) (pkgif.Stream, error) {
	if h.Host == nil {
		return nil, ErrHolePunchFailed
	}

	// 使用 Host 的 NewStream 打开协商流
	stream, err := h.Host.NewStream(ctx, peerID, HolePunchProtocol)
	if err != nil {
		return nil, err
	}

	return stream, nil
}

// simultaneousDial 同时尝试直连多个地址（带重试）
//
// 使用 DirectDialer 直接拨号，避免递归
// 这是真正的"打洞"操作：向对方的外部地址同时发包
//
// 重试机制：
//   - 打洞可能因为 NAT 映射未及时建立而失败
//   - 通过重试给双方 NAT 更多时间建立映射
func (h *HolePuncher) simultaneousDial(ctx context.Context, peerID string, addrs []string) error {
	peerShort := peerID
	if len(peerShort) > 8 {
		peerShort = peerShort[:8]
	}

	// 检查地址列表
	if len(addrs) == 0 {
		logger.Warn("打洞目标地址为空，无法打洞", "peerID", peerShort)
		return ErrNoAddresses
	}

	h.mu.RLock()
	directDialer := h.DirectDialer
	h.mu.RUnlock()

	if directDialer == nil {
		logger.Warn("DirectDialer 未设置，无法打洞", "peerID", peerShort)
		return ErrHolePunchFailed
	}

	var lastErr error

	// 重试循环
	for retry := 0; retry < MaxHolePunchRetries; retry++ {
		if retry > 0 {
			logger.Info("打洞重试",
				"peerID", peerShort,
				"retry", retry,
				"maxRetries", MaxHolePunchRetries)

			// 重试前短暂延迟，让 NAT 映射有机会建立
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(HolePunchRetryDelay):
			}
		}

		// 尝试打洞
		err := h.doSimultaneousDial(ctx, peerID, addrs, directDialer)
		if err == nil {
			return nil // 成功
		}
		lastErr = err
	}

	// 打洞失败是 NAT 限制下的预期行为（如对称 NAT），降级到 DEBUG
	logger.Debug("打洞重试全部失败（NAT 限制，将使用 Relay）",
		"peerID", peerShort,
		"retries", MaxHolePunchRetries,
		"lastError", lastErr)
	return ErrHolePunchFailed
}

// doSimultaneousDial 执行单次打洞尝试
func (h *HolePuncher) doSimultaneousDial(ctx context.Context, peerID string, addrs []string, directDialer DirectDialer) error {
	peerShort := peerID
	if len(peerShort) > 8 {
		peerShort = peerShort[:8]
	}

	// 创建超时上下文
	dialCtx, cancel := context.WithTimeout(ctx, DirectDialTimeout)
	defer cancel()

	// 结果通道
	type dialResult struct {
		addr string
		conn pkgif.Connection
		err  error
	}
	results := make(chan dialResult, len(addrs))

	// 并发尝试所有地址（真正的打洞！）
	logger.Info("开始同时拨号打洞",
		"peerID", peerShort,
		"addrsCount", len(addrs),
		"timeout", DirectDialTimeout)

	for _, addr := range addrs {
		go func(a string) {
			logger.Info("尝试直连地址（打洞）", "peerID", peerShort, "addr", a)
			conn, err := directDialer.DialDirect(dialCtx, peerID, a)
			results <- dialResult{addr: a, conn: conn, err: err}
		}(addr)
	}

	// 等待第一个成功的连接
	var lastErr error
	successCount := 0
	failCount := 0

	for i := 0; i < len(addrs); i++ {
		select {
		case res := <-results:
			if res.err == nil && res.conn != nil {
				successCount++
				// 成功建立直连
				logger.Info("HolePunch 打洞成功！",
					"peerID", peerShort,
					"addr", res.addr,
					"successCount", successCount)
				return nil
			}
			failCount++
			lastErr = res.err
			logger.Debug("地址拨号失败",
				"peerID", peerShort,
				"addr", res.addr,
				"error", res.err,
				"failCount", failCount,
				"totalAddrs", len(addrs))
		case <-dialCtx.Done():
			// 打洞超时是 NAT 限制下的预期行为，降级到 DEBUG
			logger.Debug("打洞同时拨号超时（NAT 限制）",
				"peerID", peerShort,
				"timeout", DirectDialTimeout,
				"failCount", failCount,
				"totalAddrs", len(addrs))
			return ErrHolePunchFailed
		}
	}

	// 所有地址拨号失败是 NAT 限制下的预期行为，降级到 DEBUG
	logger.Debug("打洞所有地址拨号失败（NAT 限制）",
		"peerID", peerShort,
		"failCount", failCount,
		"lastError", lastErr)
	return ErrHolePunchFailed
}

// getObservedAddrs 获取本地观测地址（用于打洞协商）
//
// 使用 HolePunchAddrs 获取打洞专用地址
// - HolePunchAddrs 优先返回 STUN 候选地址（真正的外部地址）
// - 对于 NAT 节点，dial-back 验证无法成功，但 STUN 地址是真实可用的
//
// 地址优先级：
//  1. STUN/UPnP/NAT-PMP 候选地址（★ 打洞核心）
//  2. 已验证的直连地址
//
// 注意：不使用 Relay 地址。Relay 只作为信令通道，不是打洞目标。
//
// 回退策略：HolePunchAddrs → ShareableAddrs → AdvertisedAddrs → Addrs
func (h *HolePuncher) getObservedAddrs() [][]byte {
	if h.Host == nil {
		logger.Warn("Host 为 nil，无法获取观测地址")
		return nil
	}

	// 优先使用 HolePunchAddrs（包含 STUN 候选地址）
	addrs := h.Host.HolePunchAddrs()
	source := "HolePunchAddrs"

	// 回退策略
	if len(addrs) == 0 {
		addrs = h.Host.ShareableAddrs()
		source = "ShareableAddrs"
	}

	if len(addrs) == 0 {
		addrs = h.Host.AdvertisedAddrs()
		source = "AdvertisedAddrs"
	}

	if len(addrs) == 0 {
		addrs = h.Host.Addrs()
		source = "Addrs"
	}

	logger.Info("获取本地打洞地址",
		"source", source,
		"count", len(addrs),
		"addrs", addrs)

	result := make([][]byte, 0, len(addrs))
	for _, addr := range addrs {
		result = append(result, []byte(addr))
	}

	return result
}

// addrsToStrings 将 [][]byte 转换为 []string（用于日志）
func (h *HolePuncher) addrsToStrings(addrs [][]byte) []string {
	result := make([]string, len(addrs))
	for i, addr := range addrs {
		result[i] = string(addr)
	}
	return result
}

// readMessage 读取 Hole Punch 消息
func (h *HolePuncher) readMessage(stream pkgif.Stream) (*holepunch.HolePunch, error) {
	buf := make([]byte, 4096)
	n, err := stream.Read(buf)
	if err != nil {
		return nil, err
	}

	msg := &holepunch.HolePunch{}
	if err := proto.Unmarshal(buf[:n], msg); err != nil {
		return nil, err
	}

	return msg, nil
}

// writeMessage 写入 Hole Punch 消息
func (h *HolePuncher) writeMessage(stream pkgif.Stream, msg *holepunch.HolePunch) error {
	data, err := proto.Marshal(msg)
	if err != nil {
		return err
	}

	_, err = stream.Write(data)
	return err
}

// MarkActive 标记节点为活跃
func (h *HolePuncher) MarkActive(peerID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.active[peerID] = struct{}{}
}

// ClearActive 清除活跃标记
func (h *HolePuncher) ClearActive(peerID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.active, peerID)
}

// IsActive 检查节点是否活跃
func (h *HolePuncher) IsActive(peerID string) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	_, exists := h.active[peerID]
	return exists
}

// ActiveCount 返回活跃打洞数量
func (h *HolePuncher) ActiveCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.active)
}

// Errors
var (
	ErrNoAddresses     = &HolePunchError{Message: "no addresses"}
	ErrHolePunchActive = &HolePunchError{Message: "hole punch already active"}
	ErrHolePunchFailed = &HolePunchError{Message: "hole punch failed"}
)

// HolePunchError 打洞错误
type HolePunchError struct {
	Message string
	Cause   error
}

func (e *HolePunchError) Error() string {
	if e.Cause != nil {
		return "holepunch: " + e.Message + ": " + e.Cause.Error()
	}
	return "holepunch: " + e.Message
}

func (e *HolePunchError) Unwrap() error {
	return e.Cause
}
