// Package bandwidth 提供带宽统计模块的实现
package bandwidth

import (
	"testing"
	"time"

	"github.com/dep2p/go-dep2p/pkg/interfaces"
)

// TestCounter_Basic 测试基本流量记录
func TestCounter_Basic(t *testing.T) {
	config := interfaces.BandwidthConfig{
		Enabled:         true,
		TrackByPeer:     true,
		TrackByProtocol: true,
	}
	counter := NewCounter(config)

	// 记录发送
	counter.LogSentMessage(1024)
	counter.LogSentMessage(2048)

	// 记录接收
	counter.LogRecvMessage(512)
	counter.LogRecvMessage(1024)

	// 检查总量
	stats := counter.GetTotals()
	if stats.TotalOut != 3072 {
		t.Errorf("TotalOut = %d, want 3072", stats.TotalOut)
	}
	if stats.TotalIn != 1536 {
		t.Errorf("TotalIn = %d, want 1536", stats.TotalIn)
	}
}

// TestCounter_StreamTracking 测试流跟踪
func TestCounter_StreamTracking(t *testing.T) {
	config := interfaces.BandwidthConfig{
		Enabled:         true,
		TrackByPeer:     true,
		TrackByProtocol: true,
	}
	counter := NewCounter(config)

	// 记录流数据
	counter.LogSentStream(1024, "/chat/1.0", "peer1")
	counter.LogSentStream(2048, "/chat/1.0", "peer2")
	counter.LogRecvStream(512, "/file/1.0", "peer1")

	// 检查总量
	stats := counter.GetTotals()
	if stats.TotalOut != 3072 {
		t.Errorf("TotalOut = %d, want 3072", stats.TotalOut)
	}
	if stats.TotalIn != 512 {
		t.Errorf("TotalIn = %d, want 512", stats.TotalIn)
	}

	// 检查按 Peer 统计
	peer1Stats := counter.GetForPeer("peer1")
	if peer1Stats.TotalOut != 1024 {
		t.Errorf("peer1 TotalOut = %d, want 1024", peer1Stats.TotalOut)
	}
	if peer1Stats.TotalIn != 512 {
		t.Errorf("peer1 TotalIn = %d, want 512", peer1Stats.TotalIn)
	}

	// 检查按协议统计
	chatStats := counter.GetForProtocol("/chat/1.0")
	if chatStats.TotalOut != 3072 {
		t.Errorf("/chat/1.0 TotalOut = %d, want 3072", chatStats.TotalOut)
	}
}

// TestCounter_Disabled 测试禁用状态
func TestCounter_Disabled(t *testing.T) {
	config := interfaces.BandwidthConfig{
		Enabled:         false,
		TrackByPeer:     true,
		TrackByProtocol: true,
	}
	counter := NewCounter(config)

	// 记录应该被忽略
	counter.LogSentMessage(1024)
	counter.LogRecvMessage(512)
	counter.LogSentStream(2048, "/test", "peer1")

	stats := counter.GetTotals()
	if stats.TotalOut != 0 {
		t.Errorf("TotalOut should be 0 when disabled, got %d", stats.TotalOut)
	}
	if stats.TotalIn != 0 {
		t.Errorf("TotalIn should be 0 when disabled, got %d", stats.TotalIn)
	}
}

// TestCounter_DisabledPeerTracking 测试禁用 Peer 跟踪
func TestCounter_DisabledPeerTracking(t *testing.T) {
	config := interfaces.BandwidthConfig{
		Enabled:         true,
		TrackByPeer:     false,
		TrackByProtocol: true,
	}
	counter := NewCounter(config)

	counter.LogSentStream(1024, "/test", "peer1")

	// 总量应该记录
	stats := counter.GetTotals()
	if stats.TotalOut != 1024 {
		t.Errorf("TotalOut = %d, want 1024", stats.TotalOut)
	}

	// 但 Peer 统计应该为空
	peerStats := counter.GetForPeer("peer1")
	if peerStats.TotalOut != 0 {
		t.Errorf("Peer tracking should be disabled, got TotalOut = %d", peerStats.TotalOut)
	}
}

// TestCounter_GetByPeer 测试获取所有 Peer 统计
func TestCounter_GetByPeer(t *testing.T) {
	config := interfaces.BandwidthConfig{
		Enabled:         true,
		TrackByPeer:     true,
		TrackByProtocol: true,
	}
	counter := NewCounter(config)

	counter.LogSentStream(1024, "/test", "peer1")
	counter.LogSentStream(2048, "/test", "peer2")
	counter.LogRecvStream(512, "/test", "peer1")

	byPeer := counter.GetByPeer()

	if len(byPeer) != 2 {
		t.Errorf("Expected 2 peers, got %d", len(byPeer))
	}

	peer1 := byPeer["peer1"]
	if peer1.TotalOut != 1024 || peer1.TotalIn != 512 {
		t.Errorf("peer1 stats incorrect: out=%d, in=%d", peer1.TotalOut, peer1.TotalIn)
	}
}

// TestCounter_GetByProtocol 测试获取所有协议统计
func TestCounter_GetByProtocol(t *testing.T) {
	config := interfaces.BandwidthConfig{
		Enabled:         true,
		TrackByPeer:     true,
		TrackByProtocol: true,
	}
	counter := NewCounter(config)

	counter.LogSentStream(1024, "/chat/1.0", "peer1")
	counter.LogSentStream(2048, "/file/1.0", "peer1")

	byProto := counter.GetByProtocol()

	if len(byProto) != 2 {
		t.Errorf("Expected 2 protocols, got %d", len(byProto))
	}

	chatStats := byProto["/chat/1.0"]
	if chatStats.TotalOut != 1024 {
		t.Errorf("/chat/1.0 TotalOut = %d, want 1024", chatStats.TotalOut)
	}
}

// TestCounter_Reset 测试重置
func TestCounter_Reset(t *testing.T) {
	config := interfaces.BandwidthConfig{
		Enabled:         true,
		TrackByPeer:     true,
		TrackByProtocol: true,
	}
	counter := NewCounter(config)

	counter.LogSentStream(1024, "/test", "peer1")

	stats := counter.GetTotals()
	if stats.TotalOut == 0 {
		t.Error("Stats should not be zero before reset")
	}

	counter.Reset()

	stats = counter.GetTotals()
	if stats.TotalOut != 0 {
		t.Errorf("TotalOut should be 0 after reset, got %d", stats.TotalOut)
	}

	byPeer := counter.GetByPeer()
	if len(byPeer) != 0 {
		t.Errorf("Peer stats should be empty after reset, got %d entries", len(byPeer))
	}
}

// TestCounter_TrimIdle 测试清理空闲条目
func TestCounter_TrimIdle(t *testing.T) {
	config := interfaces.BandwidthConfig{
		Enabled:         true,
		TrackByPeer:     true,
		TrackByProtocol: true,
	}
	counter := NewCounter(config)

	// 记录一些数据
	counter.LogSentStream(1024, "/test", "peer1")

	// 清理 "最近" 的条目 - 实际上应该清理所有
	counter.TrimIdle(time.Now().Add(time.Hour))

	// 验证被清理
	byPeer := counter.GetByPeer()
	if len(byPeer) != 0 {
		t.Errorf("Peer stats should be empty after trim, got %d entries", len(byPeer))
	}
}

// TestCounter_NegativeSize 测试负数大小被忽略
func TestCounter_NegativeSize(t *testing.T) {
	config := interfaces.BandwidthConfig{
		Enabled:         true,
		TrackByPeer:     true,
		TrackByProtocol: true,
	}
	counter := NewCounter(config)

	counter.LogSentMessage(-100)
	counter.LogRecvMessage(0)

	stats := counter.GetTotals()
	if stats.TotalOut != 0 {
		t.Errorf("Negative size should be ignored, TotalOut = %d", stats.TotalOut)
	}
	if stats.TotalIn != 0 {
		t.Errorf("Zero size should be ignored, TotalIn = %d", stats.TotalIn)
	}
}

// TestCounter_PeerCount 测试 Peer 计数
func TestCounter_PeerCount(t *testing.T) {
	config := interfaces.BandwidthConfig{
		Enabled:         true,
		TrackByPeer:     true,
		TrackByProtocol: true,
	}
	counter := NewCounter(config)

	counter.LogSentStream(1024, "/test", "peer1")
	counter.LogSentStream(1024, "/test", "peer2")
	counter.LogSentStream(1024, "/test", "peer3")

	count := counter.PeerCount()
	if count != 3 {
		t.Errorf("PeerCount = %d, want 3", count)
	}
}

// TestCounter_ProtocolCount 测试协议计数
func TestCounter_ProtocolCount(t *testing.T) {
	config := interfaces.BandwidthConfig{
		Enabled:         true,
		TrackByPeer:     true,
		TrackByProtocol: true,
	}
	counter := NewCounter(config)

	counter.LogSentStream(1024, "/chat/1.0", "peer1")
	counter.LogSentStream(1024, "/file/1.0", "peer1")

	count := counter.ProtocolCount()
	if count != 2 {
		t.Errorf("ProtocolCount = %d, want 2", count)
	}
}
