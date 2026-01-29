package metrics

import (
	"sync"
	"testing"
)

// ============================================================================
// 并发测试
// ============================================================================

// TestConcurrent_LogMessages 测试并发记录消息
func TestConcurrent_LogMessages(t *testing.T) {
	bwc := NewBandwidthCounter()

	numGoroutines := 100
	numOps := 100

	var wg sync.WaitGroup
	wg.Add(numGoroutines * 2)

	// 并发发送消息
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < numOps; j++ {
				bwc.LogSentMessage(10)
			}
		}()
	}

	// 并发接收消息
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < numOps; j++ {
				bwc.LogRecvMessage(20)
			}
		}()
	}

	wg.Wait()

	// 验证统计
	stats := bwc.GetBandwidthTotals()
	expectedOut := int64(numGoroutines * numOps * 10)
	expectedIn := int64(numGoroutines * numOps * 20)

	if stats.TotalOut != expectedOut {
		t.Errorf("TotalOut = %d, want %d", stats.TotalOut, expectedOut)
	}
	if stats.TotalIn != expectedIn {
		t.Errorf("TotalIn = %d, want %d", stats.TotalIn, expectedIn)
	}
}

// TestConcurrent_GetStats 测试并发获取统计
func TestConcurrent_GetStats(t *testing.T) {
	bwc := NewBandwidthCounter()

	// 先记录一些消息
	for i := 0; i < 100; i++ {
		bwc.LogSentMessage(100)
		bwc.LogRecvMessage(200)
	}

	numGoroutines := 50
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	// 并发获取统计
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				stats := bwc.GetBandwidthTotals()
				if stats.TotalOut < 0 || stats.TotalIn < 0 {
					t.Errorf("Invalid stats: %+v", stats)
				}
			}
		}()
	}

	wg.Wait()
}

// TestConcurrent_RaceDetection 测试竞态条件
// 运行 go test -race 时检测竞态
func TestConcurrent_RaceDetection(t *testing.T) {
	bwc := NewBandwidthCounter()

	peer1 := testPeerID("peer1")
	peer2 := testPeerID("peer2")
	proto1 := testProtocolID("/test/1.0.0")
	proto2 := testProtocolID("/test/2.0.0")

	numGoroutines := 50
	var wg sync.WaitGroup
	wg.Add(numGoroutines * 4)

	// 并发写入
	for i := 0; i < numGoroutines; i++ {
		// 全局消息
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				bwc.LogSentMessage(10)
				bwc.LogRecvMessage(20)
			}
		}()

		// 流消息 - peer1/proto1
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				bwc.LogSentMessageStream(10, proto1, peer1)
				bwc.LogRecvMessageStream(20, proto1, peer1)
			}
		}()

		// 流消息 - peer2/proto2
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				bwc.LogSentMessageStream(10, proto2, peer2)
				bwc.LogRecvMessageStream(20, proto2, peer2)
			}
		}()

		// 并发读取
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				_ = bwc.GetBandwidthTotals()
				_ = bwc.GetBandwidthForPeer(peer1)
				_ = bwc.GetBandwidthForProtocol(proto1)
			}
		}()
	}

	wg.Wait()
}

// TestConcurrent_StreamAndGlobal 测试并发流和全局消息
func TestConcurrent_StreamAndGlobal(t *testing.T) {
	bwc := NewBandwidthCounter()

	peer := testPeerID("peer1")
	proto := testProtocolID("/test/1.0.0")

	numGoroutines := 50
	var wg sync.WaitGroup
	wg.Add(numGoroutines * 2)

	// 并发全局消息
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				bwc.LogSentMessage(10)
			}
		}()
	}

	// 并发流消息
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				bwc.LogSentMessageStream(10, proto, peer)
			}
		}()
	}

	wg.Wait()

	// 验证统计
	globalStats := bwc.GetBandwidthTotals()
	if globalStats.TotalOut != int64(numGoroutines*50*10) {
		t.Errorf("Global TotalOut = %d, want %d", globalStats.TotalOut, numGoroutines*50*10)
	}

	peerStats := bwc.GetBandwidthForPeer(peer)
	if peerStats.TotalOut != int64(numGoroutines*50*10) {
		t.Errorf("Peer TotalOut = %d, want %d", peerStats.TotalOut, numGoroutines*50*10)
	}
}

// TestConcurrent_GetBandwidthByPeerProtocol 测试并发获取所有统计
func TestConcurrent_GetBandwidthByPeerProtocol(t *testing.T) {
	bwc := NewBandwidthCounter()

	// 准备数据
	for i := 0; i < 10; i++ {
		peer := testPeerID("peer" + string(rune('0'+i)))
		proto := testProtocolID("/test/" + string(rune('0'+i)))
		bwc.LogSentMessageStream(100, proto, peer)
	}

	numGoroutines := 20
	var wg sync.WaitGroup
	wg.Add(numGoroutines * 2)

	// 并发获取按节点统计
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			byPeer := bwc.GetBandwidthByPeer()
			if len(byPeer) < 0 {
				t.Error("GetBandwidthByPeer() failed")
			}
		}()
	}

	// 并发获取按协议统计
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			byProtocol := bwc.GetBandwidthByProtocol()
			if len(byProtocol) < 0 {
				t.Error("GetBandwidthByProtocol() failed")
			}
		}()
	}

	wg.Wait()
}
