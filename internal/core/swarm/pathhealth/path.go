// Package pathhealth 提供路径健康管理功能
package pathhealth

import (
	"strings"
	"sync"
	"time"

	"github.com/dep2p/go-dep2p/pkg/interfaces"
)

// Path 路径信息
type Path struct {
	mu sync.RWMutex

	// 基本信息
	pathID   interfaces.PathID
	addr     string
	pathType interfaces.PathType

	// RTT 统计
	ewmaRTT  time.Duration
	lastRTT  time.Duration
	minRTT   time.Duration
	maxRTT   time.Duration
	rttCount int64

	// 成功/失败统计
	successCount        int64
	failureCount        int64
	consecutiveFailures int

	// 时间戳
	firstSeen        time.Time
	lastSeen         time.Time
	lastStateChange  time.Time
	lastProbeSuccess time.Time

	// 状态
	state interfaces.PathState

	// 配置引用
	config *Config
}

// NewPath 创建新路径
func NewPath(addr string, pathType interfaces.PathType, config *Config) *Path {
	now := time.Now()
	pathID := GeneratePathID(addr, pathType)

	return &Path{
		pathID:          pathID,
		addr:            addr,
		pathType:        pathType,
		state:           interfaces.PathStateUnknown,
		firstSeen:       now,
		lastSeen:        now,
		lastStateChange: now,
		minRTT:          time.Hour, // 初始设为很大的值
		config:          config,
	}
}

// GeneratePathID 生成路径 ID
func GeneratePathID(addr string, pathType interfaces.PathType) interfaces.PathID {
	// 格式: "type:addr"
	return interfaces.PathID(pathType.String() + ":" + addr)
}

// RecordProbe 记录探测结果
func (p *Path) RecordProbe(rtt time.Duration, err error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	now := time.Now()

	if err != nil {
		// 失败
		p.failureCount++
		p.consecutiveFailures++
		p.updateState(now)
		return
	}

	// 成功
	p.successCount++
	p.consecutiveFailures = 0
	p.lastSeen = now
	p.lastProbeSuccess = now

	// 更新 RTT
	p.lastRTT = rtt
	p.rttCount++

	if rtt < p.minRTT {
		p.minRTT = rtt
	}
	if rtt > p.maxRTT {
		p.maxRTT = rtt
	}

	// EWMA 计算
	if p.ewmaRTT == 0 {
		p.ewmaRTT = rtt
	} else {
		alpha := p.config.EWMAAlpha
		p.ewmaRTT = time.Duration(float64(p.ewmaRTT)*(1-alpha) + float64(rtt)*alpha)
	}

	p.updateState(now)
}

// updateState 更新路径状态
func (p *Path) updateState(now time.Time) {
	oldState := p.state
	var newState interfaces.PathState

	// 连续失败达到阈值 → Dead
	if p.consecutiveFailures >= p.config.DeadFailureThreshold {
		newState = interfaces.PathStateDead
	} else if p.ewmaRTT == 0 {
		// 从未成功过
		newState = interfaces.PathStateUnknown
	} else if p.ewmaRTT > p.config.SuspectRTTThreshold {
		// RTT 过高
		newState = interfaces.PathStateSuspect
	} else if p.ewmaRTT > p.config.HealthyRTTThreshold {
		// RTT 略高
		newState = interfaces.PathStateSuspect
	} else if p.consecutiveFailures > 0 {
		// 有失败但未达阈值
		newState = interfaces.PathStateSuspect
	} else {
		// 健康
		newState = interfaces.PathStateHealthy
	}

	if newState != oldState {
		p.state = newState
		p.lastStateChange = now
	}
}

// CalculateScore 计算路径评分（越低越好）
func (p *Path) CalculateScore() float64 {
	p.mu.RLock()
	defer p.mu.RUnlock()

	// 死亡路径返回极大值
	if p.state == interfaces.PathStateDead {
		return 1e9
	}

	// 未知路径返回较大值
	if p.state == interfaces.PathStateUnknown || p.ewmaRTT == 0 {
		return 1e6
	}

	// 基础评分：RTT (毫秒)
	baseScore := float64(p.ewmaRTT.Milliseconds())

	// 成功率惩罚
	successRate := p.calculateSuccessRate()
	if successRate < 1 {
		baseScore += (1 - successRate) * 1000
	}

	// 可疑状态惩罚
	if p.state == interfaces.PathStateSuspect {
		baseScore *= 1.5
	}

	// 路径类型加成
	switch p.pathType {
	case interfaces.PathTypeDirect:
		baseScore *= p.config.DirectPathBonus
	case interfaces.PathTypeRelay:
		// 中继路径不加成
	}

	return baseScore
}

// calculateSuccessRate 计算成功率
func (p *Path) calculateSuccessRate() float64 {
	total := p.successCount + p.failureCount
	if total == 0 {
		return 0
	}
	return float64(p.successCount) / float64(total)
}

// ToStats 转换为统计结构
func (p *Path) ToStats() *interfaces.PathStats {
	p.mu.RLock()
	defer p.mu.RUnlock()

	return &interfaces.PathStats{
		PathID:              p.pathID,
		PathType:            p.pathType,
		State:               p.state,
		EWMARTT:             p.ewmaRTT,
		LastRTT:             p.lastRTT,
		MinRTT:              p.minRTT,
		MaxRTT:              p.maxRTT,
		SuccessCount:        p.successCount,
		FailureCount:        p.failureCount,
		ConsecutiveFailures: p.consecutiveFailures,
		LastSeen:            p.lastSeen,
		FirstSeen:           p.firstSeen,
		Score:               p.CalculateScore(),
	}
}

// GetState 获取当前状态
func (p *Path) GetState() interfaces.PathState {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.state
}

// GetAddr 获取地址
func (p *Path) GetAddr() string {
	return p.addr
}

// GetPathID 获取路径 ID
func (p *Path) GetPathID() interfaces.PathID {
	return p.pathID
}

// GetLastSeen 获取最后活动时间
func (p *Path) GetLastSeen() time.Time {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.lastSeen
}

// GetLastStateChange 获取最后状态变更时间
func (p *Path) GetLastStateChange() time.Time {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.lastStateChange
}

// IsStable 检查路径是否稳定
func (p *Path) IsStable(window time.Duration) bool {
	// 如果窗口为 0，视为总是稳定
	if window <= 0 {
		return true
	}

	p.mu.RLock()
	defer p.mu.RUnlock()

	return time.Since(p.lastStateChange) >= window
}

// DetectPathType 从地址检测路径类型
func DetectPathType(addr string) interfaces.PathType {
	// 简单检测：如果地址包含 "relay" 或 "p2p-circuit"，则为中继
	lowerAddr := strings.ToLower(addr)
	if strings.Contains(lowerAddr, "relay") || strings.Contains(lowerAddr, "p2p-circuit") {
		return interfaces.PathTypeRelay
	}
	return interfaces.PathTypeDirect
}
