// Package reachability 提供可达性协调模块的实现
package reachability

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/dep2p/go-dep2p/internal/core/address"
	addressif "github.com/dep2p/go-dep2p/pkg/interfaces/address"
	"github.com/dep2p/go-dep2p/pkg/interfaces/endpoint"
	reachabilityif "github.com/dep2p/go-dep2p/pkg/interfaces/reachability"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// WitnessService 入站见证服务
//
// 实现 witness-threshold 验证机制：
// - 发送方：出站连接成功后自动发送 WitnessReport
// - 接收方：验证并记录见证，达到阈值后升级 VerifiedDirect
type WitnessService struct {
	coordinator *Coordinator
	endpoint    endpoint.Endpoint

	ctx    context.Context
	cancel context.CancelFunc
}

// NewWitnessService 创建入站见证服务
func NewWitnessService(coordinator *Coordinator, ep endpoint.Endpoint) *WitnessService {
	return &WitnessService{
		coordinator: coordinator,
		endpoint:    ep,
	}
}

// Start 启动服务
func (s *WitnessService) Start(ctx context.Context) error {
	s.ctx, s.cancel = context.WithCancel(ctx)

	// 注册协议处理器
	if s.endpoint != nil {
		s.endpoint.SetProtocolHandler(reachabilityif.WitnessProtocolID, s.handleWitnessStream)
		log.Info("启动入站见证服务")
	}

	return nil
}

// Stop 停止服务
func (s *WitnessService) Stop() error {
	if s.cancel != nil {
		s.cancel()
	}

	// 移除协议处理器
	if s.endpoint != nil {
		s.endpoint.RemoveProtocolHandler(reachabilityif.WitnessProtocolID)
	}

	return nil
}

// handleWitnessStream 处理入站 witness 流
func (s *WitnessService) handleWitnessStream(stream endpoint.Stream) {
	defer func() { _ = stream.Close() }()

	// 设置读取超时
	_ = stream.SetReadDeadline(time.Now().Add(10 * time.Second))

	// 读取 WitnessReport
	data, err := io.ReadAll(io.LimitReader(stream, 4096))
	if err != nil {
		log.Debug("读取 witness 报告失败", "err", err)
		s.sendAck(stream, false, "read error", "")
		return
	}

	var report reachabilityif.WitnessReport
	if err := json.Unmarshal(data, &report); err != nil {
		log.Debug("解析 witness 报告失败", "err", err)
		s.sendAck(stream, false, "parse error", "")
		return
	}

	// 验证 TargetID 匹配
	if s.endpoint == nil {
		s.sendAck(stream, false, "no endpoint", "")
		return
	}
	selfID := s.endpoint.ID()
	reportTargetID, err := types.NodeIDFromBytes(report.TargetID)
	if err != nil || !reportTargetID.Equal(selfID) {
		log.Debug("witness: TargetID 不匹配",
			"expected", selfID.ShortString(),
			"got", reportTargetID.ShortString())
		s.sendAck(stream, false, "target mismatch", "")
		return
	}

	// 获取远程 peer 信息
	conn := stream.Connection()
	if conn == nil {
		s.sendAck(stream, false, "no connection", "")
		return
	}
	remotePeerID := conn.RemoteID()
	// 尝试从 RemoteAddrs 提取远程 IP（注意：这是“对端自报地址”，弱证据，仅用于前缀去重）
	remoteIP := ""
	for _, ra := range conn.RemoteAddrs() {
		if ra == nil {
			continue
		}
		remoteIP = extractIP(ra.String())
		if remoteIP != "" {
			break
		}
	}
	if remoteIP == "" {
		log.Debug("witness: 无法提取远程 IP")
		s.sendAck(stream, false, "no remote ip", "")
		return
	}

	// 上报见证给 Coordinator
	if s.coordinator != nil {
		s.coordinator.OnInboundWitness(report.DialedAddr, remotePeerID, remoteIP)
	}

	// 回传“观测到的对端地址”（弱证据，旁路）：用于让客户端快速看到自己的公网出口
	observed := ""
	for _, ra := range conn.RemoteAddrs() {
		if ra == nil {
			continue
		}
		observed = normalizeObservedRemoteAddr(ra.String())
		if observed != "" {
			break
		}
	}
	s.sendAck(stream, true, "", observed)
}

// sendAck 发送确认响应
func (s *WitnessService) sendAck(stream endpoint.Stream, accepted bool, reason string, observedRemoteAddr string) {
	ack := reachabilityif.WitnessAck{
		Accepted:          accepted,
		Reason:            reason,
		ObservedRemoteAddr: observedRemoteAddr,
	}
	data, _ := json.Marshal(ack)
	_ = stream.SetWriteDeadline(time.Now().Add(5 * time.Second))
	_, _ = stream.Write(data)
}

// SendWitnessReport 发送见证报告（出站连接成功后调用）
func (s *WitnessService) SendWitnessReport(ctx context.Context, conn endpoint.Connection, dialedAddr string) error {
	if conn == nil || dialedAddr == "" {
		return nil
	}

	// 打开 witness 协议流
	stream, err := conn.OpenStream(ctx, reachabilityif.WitnessProtocolID)
	if err != nil {
		log.Debug("打开 witness 流失败", "err", err)
		return err
	}
	defer func() { _ = stream.Close() }()

	// 构建并发送 WitnessReport
	report := reachabilityif.WitnessReport{
		DialedAddr: dialedAddr,
		TargetID:   conn.RemoteID().Bytes(),
		Timestamp:  time.Now().Unix(),
	}

	data, err := json.Marshal(report)
	if err != nil {
		return err
	}

	_ = stream.SetWriteDeadline(time.Now().Add(5 * time.Second))
	if _, err := stream.Write(data); err != nil {
		return err
	}

	// 读取确认
	_ = stream.SetReadDeadline(time.Now().Add(5 * time.Second))
	ackData, err := io.ReadAll(io.LimitReader(stream, 1024))
	if err != nil {
		return err
	}

	var ack reachabilityif.WitnessAck
	if err := json.Unmarshal(ackData, &ack); err != nil {
		return err
	}

	if !ack.Accepted {
		log.Debug("witness 报告被拒绝", "reason", ack.Reason)
	} else {
		log.Debug("witness 报告已接受", "dialedAddr", dialedAddr)
	}

	// 将接收方回传的"观测到的本端公网地址"记录为候选（旁路/弱证据）
	if s.coordinator != nil && ack.Accepted && ack.ObservedRemoteAddr != "" {
		s.coordinator.OnDirectAddressCandidate(address.NewAddr(types.Multiaddr(ack.ObservedRemoteAddr)), "observed-remote", addressif.PriorityUnverified)
	}

	return nil
}

// normalizeObservedRemoteAddr 将观测到的地址标准化为 QUIC multiaddr（用于候选展示）
func normalizeObservedRemoteAddr(addr string) string {
	if addr == "" {
		return ""
	}
	// 已经是 multiaddr
	if strings.Contains(addr, "/ip4/") || strings.Contains(addr, "/ip6/") {
		return addr
	}
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return ""
	}
	port, err := strconv.Atoi(portStr)
	if err != nil || port <= 0 || port > 65535 {
		return ""
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return ""
	}
	if ip.To4() != nil {
		return "/ip4/" + ip.String() + "/udp/" + strconv.Itoa(port) + "/quic-v1"
	}
	return "/ip6/" + ip.String() + "/udp/" + strconv.Itoa(port) + "/quic-v1"
}

// extractIP 从 multiaddr 或地址字符串中提取 IP
func extractIP(addr string) string {
	// 尝试解析为 host:port
	host, _, err := net.SplitHostPort(addr)
	if err == nil {
		return host
	}

	// 尝试从 multiaddr 中提取
	// /ip4/1.2.3.4/udp/... -> 1.2.3.4
	parts := splitMultiaddr(addr)
	for i, part := range parts {
		if part == "ip4" || part == "ip6" {
			if i+1 < len(parts) {
				return parts[i+1]
			}
		}
	}

	return ""
}

// splitMultiaddr 分割 multiaddr
func splitMultiaddr(addr string) []string {
	var parts []string
	current := ""
	for _, c := range addr {
		if c == '/' {
			if current != "" {
				parts = append(parts, current)
				current = ""
			}
		} else {
			current += string(c)
		}
	}
	if current != "" {
		parts = append(parts, current)
	}
	return parts
}

