package holepunch

import (
	"context"

	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/lib/proto/holepunch"
	"google.golang.org/protobuf/proto"
)

// ============================================================================
//                       Hole Punch 协议处理器
// ============================================================================

// Handler Hole Punch 协议处理器
//
// 处理入站的 Hole Punch 协商请求，实现 DCUtR (Direct Connection Upgrade through Relay) 协议。
type Handler struct {
	puncher *HolePuncher
	swarm   pkgif.Swarm
	host    pkgif.Host
}

// NewHandler 创建协议处理器
func NewHandler(puncher *HolePuncher, swarm pkgif.Swarm, host pkgif.Host) *Handler {
	return &Handler{
		puncher: puncher,
		swarm:   swarm,
		host:    host,
	}
}

// HandleStream 处理 Hole Punch 协商流
//
// 流程（作为响应方）：
//  1. 接收 CONNECT 消息（包含发起方观测地址）
//  2. 回复 CONNECT 消息（包含本地观测地址）
//  3. 接收 SYNC 消息
//  4. 回复 SYNC 消息
//  5. 同时尝试直连对方的观测地址
//
// 使用 ShareableAddrs 作为本地观测地址
func (h *Handler) HandleStream(stream pkgif.Stream) {
	defer stream.Close()

	ctx := context.Background()
	remotePeerID := string(stream.Conn().RemotePeer())
	peerShort := remotePeerID
	if len(peerShort) > 8 {
		peerShort = peerShort[:8]
	}

	logger.Info("收到 HolePunch 协商请求（响应方）", "remotePeer", peerShort)

	// 1. 读取 CONNECT 消息
	connectMsg, err := h.readMessage(stream)
	if err != nil {
		logger.Warn("读取 CONNECT 消息失败", "remotePeer", peerShort, "error", err)
		return
	}

	if connectMsg.Type != holepunch.Type_CONNECT {
		logger.Warn("收到非 CONNECT 消息", "remotePeer", peerShort, "type", connectMsg.Type)
		return
	}

	remoteAddrs := make([]string, len(connectMsg.ObsAddrs))
	for i, addr := range connectMsg.ObsAddrs {
		remoteAddrs[i] = string(addr)
	}
	logger.Info("收到发起方观测地址",
		"remotePeer", peerShort,
		"count", len(remoteAddrs),
		"addrs", remoteAddrs)

	// 2. 回复 CONNECT 消息（包含本地观测地址）
	// 使用 getObservedAddrs() 获取 ShareableAddrs
	localObsAddrs := h.getObservedAddrs()
	logger.Info("发送本地观测地址",
		"remotePeer", peerShort,
		"count", len(localObsAddrs),
		"addrs", h.addrsToStrings(localObsAddrs))

	responseMsg := &holepunch.HolePunch{
		Type:     holepunch.Type_CONNECT,
		ObsAddrs: localObsAddrs,
	}

	if err := h.writeMessage(stream, responseMsg); err != nil {
		logger.Warn("发送 CONNECT 响应失败", "remotePeer", peerShort, "error", err)
		return
	}

	// 3. 接收 SYNC 消息
	syncMsg, err := h.readMessage(stream)
	if err != nil {
		logger.Warn("读取 SYNC 消息失败", "remotePeer", peerShort, "error", err)
		return
	}

	if syncMsg.Type != holepunch.Type_SYNC {
		logger.Warn("收到非 SYNC 消息", "remotePeer", peerShort, "type", syncMsg.Type)
		return
	}

	// 4. 回复 SYNC 消息
	syncResponse := &holepunch.HolePunch{
		Type: holepunch.Type_SYNC,
	}

	if err := h.writeMessage(stream, syncResponse); err != nil {
		logger.Warn("发送 SYNC 响应失败", "remotePeer", peerShort, "error", err)
		return
	}

	logger.Info("SYNC 完成（响应方），开始同时拨号打洞",
		"remotePeer", peerShort,
		"targetAddrs", remoteAddrs)

	// 5. 异步尝试直连对方的观测地址（真正的打洞！）
	go h.puncher.simultaneousDial(ctx, remotePeerID, remoteAddrs)
}

// readMessage 读取 Hole Punch 消息
func (h *Handler) readMessage(stream pkgif.Stream) (*holepunch.HolePunch, error) {
	// 读取消息长度前缀（varint）
	buf := make([]byte, 4096)
	n, err := stream.Read(buf)
	if err != nil {
		return nil, err
	}

	// 解码 protobuf 消息
	msg := &holepunch.HolePunch{}
	if err := proto.Unmarshal(buf[:n], msg); err != nil {
		return nil, err
	}

	return msg, nil
}

// writeMessage 写入 Hole Punch 消息
func (h *Handler) writeMessage(stream pkgif.Stream, msg *holepunch.HolePunch) error {
	// 编码 protobuf 消息
	data, err := proto.Marshal(msg)
	if err != nil {
		return err
	}

	// 写入消息
	_, err = stream.Write(data)
	return err
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
func (h *Handler) getObservedAddrs() [][]byte {
	if h.host == nil {
		logger.Warn("Handler.host 为 nil，无法获取观测地址")
		return nil
	}

	// 优先使用 HolePunchAddrs（包含 STUN 候选地址）
	addrs := h.host.HolePunchAddrs()
	source := "HolePunchAddrs"

	// 回退策略
	if len(addrs) == 0 {
		addrs = h.host.ShareableAddrs()
		source = "ShareableAddrs"
	}

	if len(addrs) == 0 {
		addrs = h.host.AdvertisedAddrs()
		source = "AdvertisedAddrs"
	}

	if len(addrs) == 0 {
		addrs = h.host.Addrs()
		source = "Addrs"
	}

	logger.Info("Handler 获取本地打洞地址",
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
func (h *Handler) addrsToStrings(addrs [][]byte) []string {
	result := make([]string, len(addrs))
	for i, addr := range addrs {
		result[i] = string(addr)
	}
	return result
}

// RegisterProtocol 注册 Hole Punch 协议
func (h *Handler) RegisterProtocol() error {
	if h.host == nil {
		return ErrHolePunchFailed
	}

	h.host.SetStreamHandler(HolePunchProtocol, func(stream pkgif.Stream) {
		h.HandleStream(stream)
	})

	return nil
}
