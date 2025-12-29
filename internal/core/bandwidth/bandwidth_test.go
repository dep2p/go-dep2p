package bandwidth

import (
	"sync"
	"testing"
	"time"

	bandwidthif "github.com/dep2p/go-dep2p/pkg/interfaces/bandwidth"
	"github.com/dep2p/go-dep2p/pkg/interfaces/endpoint"
)

// ============================================================================
//                              Meter 测试
// ============================================================================

func TestMeter_Mark(t *testing.T) {
	m := NewMeter()

	m.Mark(100)
	m.Mark(200)

	if m.Total() != 300 {
		t.Errorf("Total() = %d, want 300", m.Total())
	}
}

func TestMeter_Snapshot(t *testing.T) {
	m := NewMeter()
	m.Mark(500)

	snap := m.Snapshot()

	if snap.Total != 500 {
		t.Errorf("Snapshot().Total = %d, want 500", snap.Total)
	}
}

func TestMeter_Reset(t *testing.T) {
	m := NewMeter()
	m.Mark(100)
	m.Reset()

	if m.Total() != 0 {
		t.Errorf("Reset 后 Total() = %d, want 0", m.Total())
	}
}

func TestMeter_LastActive(t *testing.T) {
	m := NewMeter()
	before := time.Now()

	time.Sleep(10 * time.Millisecond)
	m.Mark(100)

	after := time.Now()

	lastActive := m.LastActive()
	if lastActive.Before(before) || lastActive.After(after) {
		t.Errorf("LastActive 时间不正确")
	}
}

// ============================================================================
//                              MeterRegistry 测试
// ============================================================================

func TestMeterRegistry_Get(t *testing.T) {
	r := &MeterRegistry{}

	m1 := r.Get("key1")
	m2 := r.Get("key1")

	if m1 != m2 {
		t.Error("Get 应该返回同一个 Meter")
	}
}

func TestMeterRegistry_ForEach(t *testing.T) {
	r := &MeterRegistry{}

	r.Get("key1").Mark(100)
	r.Get("key2").Mark(200)
	r.Get("key3").Mark(300)

	count := 0
	r.ForEach(func(key string, meter *Meter) {
		count++
	})

	if count != 3 {
		t.Errorf("ForEach count = %d, want 3", count)
	}
}

func TestMeterRegistry_Clear(t *testing.T) {
	r := &MeterRegistry{}

	r.Get("key1").Mark(100)
	r.Get("key2").Mark(200)

	r.Clear()

	if r.Count() != 0 {
		t.Errorf("Clear 后 Count() = %d, want 0", r.Count())
	}
}

func TestMeterRegistry_TrimIdle(t *testing.T) {
	r := &MeterRegistry{}

	r.Get("key1").Mark(100)
	time.Sleep(50 * time.Millisecond)
	r.Get("key2").Mark(200)

	// 只清理比 key2 更老的
	r.TrimIdle(time.Now().Add(-25 * time.Millisecond))

	if r.Count() != 1 {
		t.Errorf("TrimIdle 后 Count() = %d, want 1", r.Count())
	}
}

// ============================================================================
//                              Counter 测试
// ============================================================================

func TestCounter_LogMessage(t *testing.T) {
	config := bandwidthif.DefaultConfig()
	c := NewCounter(config)

	c.LogSentMessage(100)
	c.LogRecvMessage(200)

	stats := c.GetBandwidthTotals()

	if stats.TotalOut != 100 {
		t.Errorf("TotalOut = %d, want 100", stats.TotalOut)
	}
	if stats.TotalIn != 200 {
		t.Errorf("TotalIn = %d, want 200", stats.TotalIn)
	}
}

func TestCounter_LogMessageStream(t *testing.T) {
	config := bandwidthif.DefaultConfig()
	c := NewCounter(config)

	// 创建测试 NodeID
	nodeID := endpoint.NodeID{1, 2, 3, 4, 5, 6, 7, 8}
	proto := endpoint.ProtocolID("/test/1.0")

	c.LogSentMessageStream(100, proto, nodeID)
	c.LogRecvMessageStream(200, proto, nodeID)

	// 检查总量
	total := c.GetBandwidthTotals()
	if total.TotalOut != 100 {
		t.Errorf("Total TotalOut = %d, want 100", total.TotalOut)
	}
	if total.TotalIn != 200 {
		t.Errorf("Total TotalIn = %d, want 200", total.TotalIn)
	}

	// 检查按 Peer
	peerStats := c.GetBandwidthForPeer(nodeID)
	if peerStats.TotalOut != 100 {
		t.Errorf("Peer TotalOut = %d, want 100", peerStats.TotalOut)
	}
	if peerStats.TotalIn != 200 {
		t.Errorf("Peer TotalIn = %d, want 200", peerStats.TotalIn)
	}

	// 检查按 Protocol
	protoStats := c.GetBandwidthForProtocol(proto)
	if protoStats.TotalOut != 100 {
		t.Errorf("Protocol TotalOut = %d, want 100", protoStats.TotalOut)
	}
	if protoStats.TotalIn != 200 {
		t.Errorf("Protocol TotalIn = %d, want 200", protoStats.TotalIn)
	}
}

func TestCounter_GetBandwidthByPeer(t *testing.T) {
	config := bandwidthif.DefaultConfig()
	c := NewCounter(config)

	nodeID1 := endpoint.NodeID{1, 2, 3, 4, 5, 6, 7, 8}
	nodeID2 := endpoint.NodeID{8, 7, 6, 5, 4, 3, 2, 1}
	proto := endpoint.ProtocolID("/test/1.0")

	c.LogSentMessageStream(100, proto, nodeID1)
	c.LogSentMessageStream(200, proto, nodeID2)

	peers := c.GetBandwidthByPeer()

	if len(peers) != 2 {
		t.Errorf("Peer count = %d, want 2", len(peers))
	}
}

func TestCounter_GetBandwidthByProtocol(t *testing.T) {
	config := bandwidthif.DefaultConfig()
	c := NewCounter(config)

	nodeID := endpoint.NodeID{1, 2, 3, 4, 5, 6, 7, 8}
	proto1 := endpoint.ProtocolID("/test/1.0")
	proto2 := endpoint.ProtocolID("/chat/1.0")

	c.LogSentMessageStream(100, proto1, nodeID)
	c.LogSentMessageStream(200, proto2, nodeID)

	protocols := c.GetBandwidthByProtocol()

	if len(protocols) != 2 {
		t.Errorf("Protocol count = %d, want 2", len(protocols))
	}
}

func TestCounter_Reset(t *testing.T) {
	config := bandwidthif.DefaultConfig()
	c := NewCounter(config)

	nodeID := endpoint.NodeID{1, 2, 3, 4, 5, 6, 7, 8}
	proto := endpoint.ProtocolID("/test/1.0")

	c.LogSentMessageStream(100, proto, nodeID)
	c.Reset()

	stats := c.GetBandwidthTotals()
	if stats.TotalOut != 0 {
		t.Errorf("Reset 后 TotalOut = %d, want 0", stats.TotalOut)
	}

	if c.PeerCount() != 0 {
		t.Errorf("Reset 后 PeerCount = %d, want 0", c.PeerCount())
	}
}

func TestCounter_Disabled(t *testing.T) {
	config := bandwidthif.DefaultConfig()
	config.Enabled = false
	c := NewCounter(config)

	c.LogSentMessage(100)
	c.LogRecvMessage(200)

	stats := c.GetBandwidthTotals()
	if stats.TotalOut != 0 || stats.TotalIn != 0 {
		t.Error("禁用时不应该记录流量")
	}
}

func TestCounter_TopPeers(t *testing.T) {
	config := bandwidthif.DefaultConfig()
	c := NewCounter(config)

	proto := endpoint.ProtocolID("/test/1.0")

	// 创建多个 Peer 的流量
	for i := 0; i < 10; i++ {
		nodeID := endpoint.NodeID{byte(i), 2, 3, 4, 5, 6, 7, 8}
		c.LogSentMessageStream(int64((i+1)*100), proto, nodeID)
	}

	topPeers := c.TopPeers(3)

	if len(topPeers) != 3 {
		t.Errorf("TopPeers count = %d, want 3", len(topPeers))
	}

	// 验证是流量最大的
	if topPeers[0].Stats.TotalOut != 1000 {
		t.Errorf("Top peer TotalOut = %d, want 1000", topPeers[0].Stats.TotalOut)
	}
}

func TestCounter_TopProtocols(t *testing.T) {
	config := bandwidthif.DefaultConfig()
	c := NewCounter(config)

	nodeID := endpoint.NodeID{1, 2, 3, 4, 5, 6, 7, 8}

	c.LogSentMessageStream(300, "/proto/3", nodeID)
	c.LogSentMessageStream(100, "/proto/1", nodeID)
	c.LogSentMessageStream(200, "/proto/2", nodeID)

	topProtocols := c.TopProtocols(2)

	if len(topProtocols) != 2 {
		t.Errorf("TopProtocols count = %d, want 2", len(topProtocols))
	}

	// 验证是流量最大的
	if topProtocols[0].Stats.TotalOut != 300 {
		t.Errorf("Top protocol TotalOut = %d, want 300", topProtocols[0].Stats.TotalOut)
	}
}

// ============================================================================
//                              Stats 测试
// ============================================================================

func TestStats_TotalBytes(t *testing.T) {
	stats := bandwidthif.Stats{
		TotalIn:  100,
		TotalOut: 200,
	}

	if stats.TotalBytes() != 300 {
		t.Errorf("TotalBytes() = %d, want 300", stats.TotalBytes())
	}
}

func TestStats_TotalRate(t *testing.T) {
	stats := bandwidthif.Stats{
		RateIn:  100.5,
		RateOut: 200.5,
	}

	if stats.TotalRate() != 301.0 {
		t.Errorf("TotalRate() = %f, want 301.0", stats.TotalRate())
	}
}

// ============================================================================
//                              Config 测试
// ============================================================================

func TestConfig_Default(t *testing.T) {
	config := bandwidthif.DefaultConfig()

	if !config.Enabled {
		t.Error("默认应该启用")
	}
	if !config.TrackByPeer {
		t.Error("默认应该按 Peer 跟踪")
	}
	if !config.TrackByProtocol {
		t.Error("默认应该按 Protocol 跟踪")
	}
	if config.IdleTimeout != time.Hour {
		t.Errorf("IdleTimeout = %v, want 1h", config.IdleTimeout)
	}
}

// ============================================================================
//                              格式化测试
// ============================================================================

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		bytes int64
		want  string
	}{
		{0, "0.0 B"},
		{100, "100 B"},
		{1024, "1.0 KB"},
		{1024 * 1024, "1.0 MB"},
		{1024 * 1024 * 1024, "1.0 GB"},
	}

	for _, tt := range tests {
		got := FormatBytes(tt.bytes)
		if got != tt.want {
			t.Errorf("FormatBytes(%d) = %s, want %s", tt.bytes, got, tt.want)
		}
	}
}

func TestFormatRate(t *testing.T) {
	tests := []struct {
		rate float64
		want string
	}{
		{0, "0.0 B/s"},
		{100, "100 B/s"},
		{1024, "1.0 KB/s"},
		{1024 * 1024, "1.0 MB/s"},
	}

	for _, tt := range tests {
		got := FormatRate(tt.rate)
		if got != tt.want {
			t.Errorf("FormatRate(%f) = %s, want %s", tt.rate, got, tt.want)
		}
	}
}

// ============================================================================
//                              Reporter 测试
// ============================================================================

func TestReporter_Report(t *testing.T) {
	config := bandwidthif.DefaultConfig()
	c := NewCounter(config)
	r := NewReporter(c)

	nodeID := endpoint.NodeID{1, 2, 3, 4, 5, 6, 7, 8}
	proto := endpoint.ProtocolID("/test/1.0")

	c.LogSentMessageStream(100, proto, nodeID)

	report := r.Report()

	if report.Total.TotalOut != 100 {
		t.Errorf("Report Total.TotalOut = %d, want 100", report.Total.TotalOut)
	}
	if len(report.ByPeer) == 0 {
		t.Error("Report ByPeer 应该不为空")
	}
	if len(report.ByProtocol) == 0 {
		t.Error("Report ByProtocol 应该不为空")
	}
}

// ============================================================================
//                              并发安全测试
// ============================================================================

func TestReporter_StartStop_Idempotent(t *testing.T) {
	config := bandwidthif.DefaultConfig()
	c := NewCounter(config)
	r := NewReporter(c)

	// 启动两次应该是幂等的
	r.Start(time.Second)
	r.Start(time.Second)

	// 停止两次应该是幂等的
	r.Stop()
	r.Stop()
}

func TestReporter_StartStop_Concurrent(t *testing.T) {
	config := bandwidthif.DefaultConfig()
	c := NewCounter(config)
	r := NewReporter(c)

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			r.Start(time.Second)
		}()
		go func() {
			defer wg.Done()
			r.Stop()
		}()
	}
	wg.Wait()

	// 最终停止
	r.Stop()
}

func TestCounter_Concurrent(t *testing.T) {
	config := bandwidthif.DefaultConfig()
	c := NewCounter(config)

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			nodeID := endpoint.NodeID{byte(idx), 2, 3, 4, 5, 6, 7, 8}
			proto := endpoint.ProtocolID("/test/1.0")

			for j := 0; j < 100; j++ {
				c.LogSentMessageStream(100, proto, nodeID)
				c.LogRecvMessageStream(200, proto, nodeID)
				c.GetBandwidthTotals()
				c.GetBandwidthForPeer(nodeID)
				c.GetBandwidthForProtocol(proto)
			}
		}(i)
	}
	wg.Wait()
}

func TestMeter_Concurrent(t *testing.T) {
	m := NewMeter()

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				m.Mark(uint64(j))
				m.Total()
				m.Rate()
				m.Snapshot()
			}
		}()
	}
	wg.Wait()
}

func TestMeterRegistry_Concurrent(t *testing.T) {
	r := &MeterRegistry{}

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			key := string(rune('a' + idx))
			for j := 0; j < 100; j++ {
				meter := r.Get(key)
				meter.Mark(uint64(j))
				r.Exists(key)
				r.Load(key)
				r.Count()
			}
		}(i)
	}
	wg.Wait()
}

func TestCounter_GetBandwidthForPeer_NotExist(t *testing.T) {
	config := bandwidthif.DefaultConfig()
	c := NewCounter(config)

	// 查询不存在的 peer 应该返回空 Stats，而不是创建条目
	nodeID := endpoint.NodeID{1, 2, 3, 4, 5, 6, 7, 8}
	stats := c.GetBandwidthForPeer(nodeID)

	if stats.TotalIn != 0 || stats.TotalOut != 0 {
		t.Error("不存在的 peer 应该返回空 Stats")
	}

	// 检查没有创建新条目
	if c.PeerCount() != 0 {
		t.Errorf("不应该创建新条目，但 PeerCount = %d", c.PeerCount())
	}
}

func TestCounter_GetBandwidthForProtocol_NotExist(t *testing.T) {
	config := bandwidthif.DefaultConfig()
	c := NewCounter(config)

	// 查询不存在的 protocol 应该返回空 Stats，而不是创建条目
	proto := endpoint.ProtocolID("/notexist/1.0")
	stats := c.GetBandwidthForProtocol(proto)

	if stats.TotalIn != 0 || stats.TotalOut != 0 {
		t.Error("不存在的 protocol 应该返回空 Stats")
	}

	// 检查没有创建新条目
	if c.ProtocolCount() != 0 {
		t.Errorf("不应该创建新条目，但 ProtocolCount = %d", c.ProtocolCount())
	}
}

func TestMeterRegistry_LoadNotExist(t *testing.T) {
	r := &MeterRegistry{}

	// Load 不存在的 key 应该返回 nil, false
	meter, ok := r.Load("notexist")
	if ok || meter != nil {
		t.Error("Load 不存在的 key 应该返回 nil, false")
	}
}

func TestMeterRegistry_Exists(t *testing.T) {
	r := &MeterRegistry{}

	// 不存在
	if r.Exists("key1") {
		t.Error("key1 不应该存在")
	}

	// 创建
	r.Get("key1").Mark(100)

	// 应该存在
	if !r.Exists("key1") {
		t.Error("key1 应该存在")
	}
}

