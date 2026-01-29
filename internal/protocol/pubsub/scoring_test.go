// Package pubsub 实现发布订阅协议
package pubsub

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewPeerScorer(t *testing.T) {
	ps := NewPeerScorer(nil)
	require.NotNil(t, ps)
	assert.NotNil(t, ps.params)
	assert.NotNil(t, ps.topicParams)
	assert.NotNil(t, ps.peerStats)
}

func TestPeerScorer_AddRemovePeer(t *testing.T) {
	ps := NewPeerScorer(nil)

	// 添加 peer
	ps.AddPeer("peer1", "192.168.1.1")
	assert.Equal(t, float64(0), ps.Score("peer1"))

	// 移除 peer
	ps.RemovePeer("peer1")
	assert.Equal(t, float64(0), ps.Score("peer1")) // 断开后仍保留统计
}

func TestPeerScorer_GraftPrune(t *testing.T) {
	ps := NewPeerScorer(nil)
	topic := "test-topic"

	ps.AddPeer("peer1", "")
	ps.Graft("peer1", topic)

	// 检查 mesh 状态
	ps.mu.RLock()
	stats := ps.peerStats["peer1"]
	tStats := stats.topicStats[topic]
	ps.mu.RUnlock()

	assert.True(t, tStats.inMesh)

	// Prune
	ps.Prune("peer1", topic)

	ps.mu.RLock()
	tStats = ps.peerStats["peer1"].topicStats[topic]
	ps.mu.RUnlock()

	assert.False(t, tStats.inMesh)
}

func TestPeerScorer_ValidateMessage(t *testing.T) {
	ps := NewPeerScorer(nil)
	topic := "test-topic"

	ps.AddPeer("peer1", "")
	ps.Graft("peer1", topic)

	// 有效的首次消息
	ps.ValidateMessage("peer1", topic, true, true)

	ps.mu.RLock()
	stats := ps.peerStats["peer1"]
	tStats := stats.topicStats[topic]
	ps.mu.RUnlock()

	assert.Equal(t, float64(1), tStats.firstMessageDeliveries)
	assert.Equal(t, float64(1), tStats.meshMessageDeliveries)

	// 无效消息
	ps.ValidateMessage("peer1", topic, false, false)

	ps.mu.RLock()
	tStats = ps.peerStats["peer1"].topicStats[topic]
	ps.mu.RUnlock()

	assert.Equal(t, float64(1), tStats.invalidMessages)
}

func TestPeerScorer_BrokenPromise(t *testing.T) {
	ps := NewPeerScorer(nil)

	ps.AddPeer("peer1", "")
	ps.BrokenPromise("peer1")

	ps.mu.RLock()
	stats := ps.peerStats["peer1"]
	ps.mu.RUnlock()

	assert.Equal(t, float64(1), stats.behaviourPenalty)
}

func TestPeerScorer_SetAppScore(t *testing.T) {
	ps := NewPeerScorer(nil)

	ps.AddPeer("peer1", "")
	ps.SetAppScore("peer1", 50.0)

	ps.mu.RLock()
	stats := ps.peerStats["peer1"]
	ps.mu.RUnlock()

	assert.Equal(t, float64(50), stats.appScore)
}

func TestPeerScorer_IPColocation(t *testing.T) {
	params := DefaultScoreParams()
	params.IPColocationFactorThreshold = 2
	ps := NewPeerScorer(params)

	// 添加同 IP 的多个 peer
	ip := "192.168.1.1"
	ps.AddPeer("peer1", ip)
	ps.AddPeer("peer2", ip)
	ps.AddPeer("peer3", ip)
	ps.AddPeer("peer4", ip)

	// 超过阈值后应该有惩罚
	score := ps.computeIPColocationScore("peer1")
	assert.Greater(t, score, float64(0))
}

func TestPeerScorer_IPWhitelist(t *testing.T) {
	params := DefaultScoreParams()
	params.IPColocationFactorThreshold = 1
	params.IPColocationFactorWhitelist["192.168.1.1"] = struct{}{}
	ps := NewPeerScorer(params)

	ip := "192.168.1.1"
	ps.AddPeer("peer1", ip)
	ps.AddPeer("peer2", ip)
	ps.AddPeer("peer3", ip)

	// 白名单 IP 不应该有惩罚
	score := ps.computeIPColocationScore("peer1")
	assert.Equal(t, float64(0), score)
}

func TestPeerScorer_Thresholds(t *testing.T) {
	ps := NewPeerScorerWithThresholds(nil, -100, -200, -300, 20)

	gossip, publish, graylist, acceptPX := ps.GetThresholds()
	assert.Equal(t, float64(-100), gossip)
	assert.Equal(t, float64(-200), publish)
	assert.Equal(t, float64(-300), graylist)
	assert.Equal(t, float64(20), acceptPX)
}

func TestPeerScorer_ThresholdChecks(t *testing.T) {
	ps := NewPeerScorerWithThresholds(nil, -100, -200, -300, 20)

	ps.AddPeer("peer1", "")

	// 默认分数为 0，高于所有负阈值
	assert.False(t, ps.IsBelowGossipThreshold("peer1"))
	assert.False(t, ps.IsBelowPublishThreshold("peer1"))
	assert.False(t, ps.IsBelowGraylistThreshold("peer1"))
	assert.False(t, ps.IsAboveAcceptPXThreshold("peer1"))

	// 设置高应用层评分
	ps.SetAppScore("peer1", 50.0)
	assert.True(t, ps.IsAboveAcceptPXThreshold("peer1"))
}

func TestPeerScorer_SetTopicParams(t *testing.T) {
	ps := NewPeerScorer(nil)

	params := &TopicScoreParams{
		TopicWeight:     2.0,
		TimeInMeshCap:   1000,
	}
	ps.SetTopicParams("custom-topic", params)

	ps.mu.RLock()
	retrieved := ps.topicParams["custom-topic"]
	ps.mu.RUnlock()

	assert.Equal(t, float64(2.0), retrieved.TopicWeight)
	assert.Equal(t, float64(1000), retrieved.TimeInMeshCap)

	// 移除
	ps.RemoveTopicParams("custom-topic")
	ps.mu.RLock()
	_, exists := ps.topicParams["custom-topic"]
	ps.mu.RUnlock()
	assert.False(t, exists)
}

func TestPeerScorer_GetPeerScore(t *testing.T) {
	ps := NewPeerScorer(nil)
	topic := "test-topic"

	ps.AddPeer("peer1", "")
	ps.Graft("peer1", topic)
	ps.ValidateMessage("peer1", topic, true, true)

	totalScore, topicScores := ps.GetPeerScore("peer1")
	assert.NotNil(t, topicScores)
	assert.Contains(t, topicScores, topic)
	// 首次有效消息投递应该贡献正分
	assert.GreaterOrEqual(t, totalScore, float64(0))
}

func TestPeerScorer_Reset(t *testing.T) {
	ps := NewPeerScorer(nil)

	ps.AddPeer("peer1", "192.168.1.1")
	ps.AddPeer("peer2", "192.168.1.2")

	ps.Reset()

	ps.mu.RLock()
	peerCount := len(ps.peerStats)
	ipCount := len(ps.peerIPs)
	ps.mu.RUnlock()

	assert.Equal(t, 0, peerCount)
	assert.Equal(t, 0, ipCount)
}

func TestPeerScorer_Decay(t *testing.T) {
	params := DefaultScoreParams()
	params.DecayInterval = 10 * time.Millisecond
	ps := NewPeerScorer(params)

	topic := "test-topic"
	ps.AddPeer("peer1", "")
	ps.Graft("peer1", topic)

	// 记录一些消息
	for i := 0; i < 10; i++ {
		ps.ValidateMessage("peer1", topic, true, true)
	}

	ps.mu.RLock()
	initialDeliveries := ps.peerStats["peer1"].topicStats[topic].firstMessageDeliveries
	ps.mu.RUnlock()

	assert.Equal(t, float64(10), initialDeliveries)

	// 等待衰减
	time.Sleep(50 * time.Millisecond)
	ps.Decay()

	ps.mu.RLock()
	decayedDeliveries := ps.peerStats["peer1"].topicStats[topic].firstMessageDeliveries
	ps.mu.RUnlock()

	// 衰减后应该减少
	assert.Less(t, decayedDeliveries, initialDeliveries)
}

func TestDefaultScoreParams(t *testing.T) {
	params := DefaultScoreParams()

	assert.Equal(t, time.Second, params.DecayInterval)
	assert.Equal(t, 0.01, params.DecayToZero)
	assert.Equal(t, 10*time.Minute, params.RetainScore)
	assert.NotNil(t, params.IPColocationFactorWhitelist)
}

func TestDefaultTopicScoreParams(t *testing.T) {
	params := DefaultTopicScoreParams()

	assert.Equal(t, 1.0, params.TopicWeight)
	assert.Equal(t, time.Second, params.TimeInMeshQuantum)
	assert.Equal(t, 3600.0, params.TimeInMeshCap)
}

func TestPeerScorer_DuplicateMessage(t *testing.T) {
	ps := NewPeerScorer(nil)
	topic := "test-topic"

	ps.AddPeer("peer1", "")
	ps.Graft("peer1", topic)

	// 重复消息（wasFirst = true 表示在窗口内）
	ps.DuplicateMessage("peer1", topic, true)

	ps.mu.RLock()
	tStats := ps.peerStats["peer1"].topicStats[topic]
	ps.mu.RUnlock()

	assert.Equal(t, float64(1), tStats.meshMessageDeliveries)
}
