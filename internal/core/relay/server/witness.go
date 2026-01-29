// Package server 实现中继服务端
//
// 本文件实现 Relay 见证扩展，用于检测中继电路断开。
package server

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"sync"
	"time"

	witnesspb "github.com/dep2p/go-dep2p/pkg/lib/proto/realm/witness"
	"google.golang.org/protobuf/proto"
)

// ============================================================================
//                              配置常量
// ============================================================================

const (
	// RelayBatchAnomalyThreshold 批量异常阈值
	// 在时间窗口内超过此数量的断开报告将触发异常检测
	RelayBatchAnomalyThreshold = 5

	// RelayBatchAnomalyWindow 批量异常检测时间窗口
	RelayBatchAnomalyWindow = 60 * time.Second

	// RelayWitnessReportExpiry 见证报告过期时间
	RelayWitnessReportExpiry = 10 * time.Second
)

// ============================================================================
//                              见证扩展
// ============================================================================

// WitnessExtension Relay 见证扩展
//
// 当 Relay 服务器检测到中继电路关闭时，可以向 Realm 成员广播见证报告。
// 这是快速断开检测的第四层（Relay 代理见证）。
//
// 设计目标：
//   - 中继非优雅断开检测：< 15s
//   - 批量异常抑制：防止网络分区时误报
type WitnessExtension struct {
	mu sync.RWMutex

	// 本地节点信息
	localID string

	// 电路关闭记录（用于批量异常检测）
	circuitCloseRecords map[string][]time.Time // peer_id -> 关闭时间列表

	// 报告发送记录（用于限速）
	reportsSent map[string]time.Time // report_id -> 发送时间

	// 回调函数（用于发送见证报告）
	onWitnessReport func(ctx context.Context, report *witnesspb.WitnessReport)

	// 生命周期
	ctx    context.Context
	cancel context.CancelFunc
}

// NewWitnessExtension 创建见证扩展
func NewWitnessExtension(localID string) *WitnessExtension {
	return &WitnessExtension{
		localID:             localID,
		circuitCloseRecords: make(map[string][]time.Time),
		reportsSent:         make(map[string]time.Time),
	}
}

// SetOnWitnessReport 设置见证报告回调
func (we *WitnessExtension) SetOnWitnessReport(callback func(ctx context.Context, report *witnesspb.WitnessReport)) {
	we.mu.Lock()
	defer we.mu.Unlock()
	we.onWitnessReport = callback
}

// Start 启动见证扩展
func (we *WitnessExtension) Start(ctx context.Context) error {
	we.mu.Lock()
	we.ctx, we.cancel = context.WithCancel(ctx)
	we.mu.Unlock()

	// 启动后台清理
	go we.cleanupLoop()

	serverLogger.Info("Relay 见证扩展已启动", "localID", truncateID(we.localID))
	return nil
}

// Stop 停止见证扩展
func (we *WitnessExtension) Stop() error {
	we.mu.Lock()
	if we.cancel != nil {
		we.cancel()
	}
	we.mu.Unlock()

	serverLogger.Info("Relay 见证扩展已停止", "localID", truncateID(we.localID))
	return nil
}

// OnCircuitClosed 处理电路关闭事件
//
// 当 Relay 服务器检测到中继电路关闭时调用。
// 如果未检测到批量异常，则发送代理见证报告。
//
// 参数：
//   - peerID: 断开的节点 ID
//   - realmID: 关联的 Realm ID（可能为空）
//   - lastContact: 最后一次成功通信的时间
func (we *WitnessExtension) OnCircuitClosed(ctx context.Context, peerID string, realmID string, lastContact time.Time) {
	// 记录关闭事件
	we.recordCircuitClose(peerID)

	// 检查是否检测到批量异常
	if we.isAnomalyDetected() {
		serverLogger.Warn("Relay 检测到批量电路关闭异常，抑制见证报告",
			"peerID", truncateID(peerID),
			"threshold", RelayBatchAnomalyThreshold)
		return
	}

	// 如果没有指定 Realm，跳过
	if realmID == "" {
		serverLogger.Debug("Relay 电路关闭但无 Realm 信息，跳过见证报告",
			"peerID", truncateID(peerID))
		return
	}

	// 发送代理见证报告
	we.sendProxyWitness(ctx, peerID, realmID, lastContact)
}

// recordCircuitClose 记录电路关闭事件
func (we *WitnessExtension) recordCircuitClose(peerID string) {
	we.mu.Lock()
	defer we.mu.Unlock()

	now := time.Now()
	we.circuitCloseRecords[peerID] = append(we.circuitCloseRecords[peerID], now)
}

// isAnomalyDetected 检测批量异常
//
// 如果在时间窗口内有超过阈值数量的不同节点断开，
// 说明可能发生了网络分区或 Relay 服务器自身问题，
// 应该抑制见证报告以避免误判。
func (we *WitnessExtension) isAnomalyDetected() bool {
	we.mu.RLock()
	defer we.mu.RUnlock()

	now := time.Now()
	windowStart := now.Add(-RelayBatchAnomalyWindow)

	// 统计时间窗口内有断开记录的节点数
	affectedPeers := 0
	for _, times := range we.circuitCloseRecords {
		for _, t := range times {
			if t.After(windowStart) {
				affectedPeers++
				break // 每个 peer 只计一次
			}
		}
	}

	return affectedPeers > RelayBatchAnomalyThreshold
}

// sendProxyWitness 发送代理见证报告
func (we *WitnessExtension) sendProxyWitness(ctx context.Context, peerID string, realmID string, lastContact time.Time) {
	we.mu.RLock()
	callback := we.onWitnessReport
	we.mu.RUnlock()

	if callback == nil {
		serverLogger.Debug("见证报告回调未设置，跳过发送",
			"peerID", truncateID(peerID))
		return
	}

	// 生成报告 ID
	reportID := make([]byte, 16)
	if _, err := rand.Read(reportID); err != nil {
		serverLogger.Debug("生成报告 ID 失败", "err", err)
		return
	}

	// 创建见证报告
	report := &witnesspb.WitnessReport{
		ReportId:             reportID,
		ReporterId:           []byte(we.localID),
		TargetId:             []byte(peerID),
		RealmId:              []byte(realmID),
		DetectionMethod:      witnesspb.DetectionMethod_DETECTION_METHOD_RELAY_CIRCUIT,
		Timestamp:            time.Now().UnixNano(),
		LastContactTimestamp: lastContact.UnixNano(),
	}

	// 记录发送
	we.mu.Lock()
	we.reportsSent[string(reportID)] = time.Now()
	we.mu.Unlock()

	serverLogger.Info("Relay 发送代理见证报告",
		"reportID", truncateID(string(reportID)),
		"targetID", truncateID(peerID),
		"realmID", realmID)

	// 调用回调发送
	callback(ctx, report)
}

// cleanupLoop 后台清理协程
func (we *WitnessExtension) cleanupLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-we.ctx.Done():
			return
		case <-ticker.C:
			we.cleanup()
		}
	}
}

// cleanup 清理过期数据
func (we *WitnessExtension) cleanup() {
	we.mu.Lock()
	defer we.mu.Unlock()

	now := time.Now()
	windowStart := now.Add(-RelayBatchAnomalyWindow * 2)

	// 清理电路关闭记录
	for peerID, times := range we.circuitCloseRecords {
		validTimes := make([]time.Time, 0, len(times))
		for _, t := range times {
			if t.After(windowStart) {
				validTimes = append(validTimes, t)
			}
		}
		if len(validTimes) == 0 {
			delete(we.circuitCloseRecords, peerID)
		} else {
			we.circuitCloseRecords[peerID] = validTimes
		}
	}

	// 清理发送记录
	reportExpiry := now.Add(-RelayWitnessReportExpiry * 2)
	for reportID, sentAt := range we.reportsSent {
		if sentAt.Before(reportExpiry) {
			delete(we.reportsSent, reportID)
		}
	}
}

// GetAnomalyStatus 获取异常状态（用于调试）
func (we *WitnessExtension) GetAnomalyStatus() (anomaly bool, affectedPeers int, threshold int) {
	we.mu.RLock()
	defer we.mu.RUnlock()

	now := time.Now()
	windowStart := now.Add(-RelayBatchAnomalyWindow)

	affected := 0
	for _, times := range we.circuitCloseRecords {
		for _, t := range times {
			if t.After(windowStart) {
				affected++
				break
			}
		}
	}

	return affected > RelayBatchAnomalyThreshold, affected, RelayBatchAnomalyThreshold
}

// ============================================================================
//                              辅助函数
// ============================================================================

// truncateID 安全截断 ID 用于日志
func truncateID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

// SerializeWitnessReport 序列化见证报告
func SerializeWitnessReport(report *witnesspb.WitnessReport) ([]byte, error) {
	data, err := proto.Marshal(report)
	if err != nil {
		return nil, err
	}
	return append([]byte("relay-witness:"), data...), nil
}

// buildSignData 构建签名数据（保留用于未来签名支持）
func buildSignData(report *witnesspb.WitnessReport) []byte {
	totalLen := len(report.ReportId) + len(report.ReporterId) + len(report.TargetId) +
		len(report.RealmId) + 4 + 8

	data := make([]byte, totalLen)
	offset := 0

	copy(data[offset:], report.ReportId)
	offset += len(report.ReportId)

	copy(data[offset:], report.ReporterId)
	offset += len(report.ReporterId)

	copy(data[offset:], report.TargetId)
	offset += len(report.TargetId)

	copy(data[offset:], report.RealmId)
	offset += len(report.RealmId)

	binary.BigEndian.PutUint32(data[offset:], uint32(report.DetectionMethod))
	offset += 4

	binary.BigEndian.PutUint64(data[offset:], uint64(report.Timestamp))

	return data
}
