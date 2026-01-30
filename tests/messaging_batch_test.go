package tests

// ════════════════════════════════════════════════════════════════════════════
//                   批量消息发送 API 测试 (v1.1 TASK-003)
// ════════════════════════════════════════════════════════════════════════════

import (
	"errors"
	"testing"
	"time"

	"github.com/dep2p/go-dep2p/pkg/interfaces"
)

// 测试用错误
var (
	errTestTimeout    = errors.New("test: timeout")
	errTestConnFailed = errors.New("test: connection failed")
	errTestInvalid    = errors.New("test: invalid protocol")
)

// TestSendResultType 测试 SendResult 类型
func TestSendResultType(t *testing.T) {
	result := interfaces.SendResult{
		PeerID:   "12D3KooWTestPeer",
		Response: []byte("hello"),
		Error:    nil,
		Latency:  100 * time.Millisecond,
	}

	if result.PeerID != "12D3KooWTestPeer" {
		t.Errorf("PeerID 期望 12D3KooWTestPeer，得到 %s", result.PeerID)
	}

	if string(result.Response) != "hello" {
		t.Errorf("Response 期望 hello，得到 %s", string(result.Response))
	}

	if result.Error != nil {
		t.Errorf("Error 期望 nil，得到 %v", result.Error)
	}

	if result.Latency != 100*time.Millisecond {
		t.Errorf("Latency 期望 100ms，得到 %v", result.Latency)
	}
}

// TestBroadcastResultType 测试 BroadcastResult 类型
func TestBroadcastResultType(t *testing.T) {
	// 测试完全成功的情况
	successResult := &interfaces.BroadcastResult{
		TotalCount:   5,
		SuccessCount: 5,
		FailedCount:  0,
		Results:      make([]interfaces.SendResult, 5),
	}

	if !successResult.Success() {
		t.Error("Success() 应该返回 true")
	}
	if successResult.PartialSuccess() {
		t.Error("PartialSuccess() 应该返回 false")
	}
	if successResult.AllFailed() {
		t.Error("AllFailed() 应该返回 false")
	}

	// 测试部分成功的情况
	partialResult := &interfaces.BroadcastResult{
		TotalCount:   5,
		SuccessCount: 3,
		FailedCount:  2,
		Results:      make([]interfaces.SendResult, 5),
	}

	if partialResult.Success() {
		t.Error("Success() 应该返回 false")
	}
	if !partialResult.PartialSuccess() {
		t.Error("PartialSuccess() 应该返回 true")
	}
	if partialResult.AllFailed() {
		t.Error("AllFailed() 应该返回 false")
	}

	// 测试完全失败的情况
	failedResult := &interfaces.BroadcastResult{
		TotalCount:   5,
		SuccessCount: 0,
		FailedCount:  5,
		Results:      make([]interfaces.SendResult, 5),
	}

	if failedResult.Success() {
		t.Error("Success() 应该返回 false")
	}
	if failedResult.PartialSuccess() {
		t.Error("PartialSuccess() 应该返回 false")
	}
	if !failedResult.AllFailed() {
		t.Error("AllFailed() 应该返回 true")
	}

	// 测试空结果的情况
	emptyResult := &interfaces.BroadcastResult{
		TotalCount:   0,
		SuccessCount: 0,
		FailedCount:  0,
		Results:      nil,
	}

	if !emptyResult.Success() {
		t.Error("空结果的 Success() 应该返回 true (没有失败)")
	}
	if emptyResult.PartialSuccess() {
		t.Error("空结果的 PartialSuccess() 应该返回 false")
	}
	if emptyResult.AllFailed() {
		t.Error("空结果的 AllFailed() 应该返回 false (TotalCount=0)")
	}
}

// TestSendResultWithError 测试带错误的 SendResult
func TestSendResultWithError(t *testing.T) {
	result := interfaces.SendResult{
		PeerID:   "12D3KooWTestPeer",
		Response: nil,
		Error:    errTestInvalid,
		Latency:  50 * time.Millisecond,
	}

	if result.Error == nil {
		t.Error("Error 应该不为 nil")
	}

	if result.Response != nil {
		t.Error("失败时 Response 应该为 nil")
	}
}

// TestBroadcastResultStatistics 测试广播结果统计
func TestBroadcastResultStatistics(t *testing.T) {
	results := []interfaces.SendResult{
		{PeerID: "peer1", Error: nil},
		{PeerID: "peer2", Error: errTestTimeout},
		{PeerID: "peer3", Error: nil},
		{PeerID: "peer4", Error: errTestConnFailed},
		{PeerID: "peer5", Error: nil},
	}

	// 计算统计
	successCount := 0
	failedCount := 0
	for _, r := range results {
		if r.Error == nil {
			successCount++
		} else {
			failedCount++
		}
	}

	broadcastResult := &interfaces.BroadcastResult{
		TotalCount:   len(results),
		SuccessCount: successCount,
		FailedCount:  failedCount,
		Results:      results,
	}

	if broadcastResult.TotalCount != 5 {
		t.Errorf("TotalCount 期望 5，得到 %d", broadcastResult.TotalCount)
	}

	if broadcastResult.SuccessCount != 3 {
		t.Errorf("SuccessCount 期望 3，得到 %d", broadcastResult.SuccessCount)
	}

	if broadcastResult.FailedCount != 2 {
		t.Errorf("FailedCount 期望 2，得到 %d", broadcastResult.FailedCount)
	}

	if !broadcastResult.PartialSuccess() {
		t.Error("应该是部分成功")
	}
}
