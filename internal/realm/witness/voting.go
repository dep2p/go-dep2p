package witness

import (
	"sync"
	"time"

	witnesspb "github.com/dep2p/go-dep2p/pkg/lib/proto/realm/witness"
)

// VotingSession 见证投票会话
//
// 管理单个见证报告的投票过程：
//   - 收集来自其他见证人的确认
//   - 根据投票规则判断是否确认断开
//   - 超时后自动结束投票
type VotingSession struct {
	mu sync.RWMutex

	// 基础信息
	reportID    string
	targetID    string
	memberCount int

	// 投票统计
	agreeCount    int
	disagreeCount int
	abstainCount  int

	// 已投票的见证人
	voted map[string]struct{}

	// 回调
	onComplete func(*witnesspb.WitnessVotingResult)

	// 状态
	completed bool
	timer     *time.Timer
}

// NewVotingSession 创建投票会话
func NewVotingSession(reportID, targetID string, memberCount int, onComplete func(*witnesspb.WitnessVotingResult)) *VotingSession {
	return &VotingSession{
		reportID:    reportID,
		targetID:    targetID,
		memberCount: memberCount,
		voted:       make(map[string]struct{}),
		onComplete:  onComplete,
	}
}

// StartTimeout 启动超时定时器
func (vs *VotingSession) StartTimeout(timeout time.Duration) {
	vs.mu.Lock()
	defer vs.mu.Unlock()

	if vs.timer != nil {
		vs.timer.Stop()
	}

	vs.timer = time.AfterFunc(timeout, func() {
		vs.mu.Lock()
		defer vs.mu.Unlock()

		if vs.completed {
			return
		}

		// 超时，根据当前票数决定结果
		vs.finalize()
	})
}

// AddConfirmation 添加见证确认
func (vs *VotingSession) AddConfirmation(confirmation *witnesspb.WitnessConfirmation) {
	vs.mu.Lock()
	defer vs.mu.Unlock()

	if vs.completed {
		return
	}

	witnessID := string(confirmation.WitnessId)

	// 防止重复投票
	if _, voted := vs.voted[witnessID]; voted {
		return
	}
	vs.voted[witnessID] = struct{}{}

	// 统计票数
	switch confirmation.ConfirmationType {
	case witnesspb.ConfirmationType_CONFIRMATION_TYPE_AGREE:
		vs.agreeCount++
	case witnesspb.ConfirmationType_CONFIRMATION_TYPE_DISAGREE:
		vs.disagreeCount++
	case witnesspb.ConfirmationType_CONFIRMATION_TYPE_ABSTAIN:
		vs.abstainCount++
	}

	logger.Debug("收到见证投票",
		"reportID", truncateID(vs.reportID),
		"witnessID", truncateID(witnessID),
		"type", confirmation.ConfirmationType.String(),
		"agree", vs.agreeCount,
		"disagree", vs.disagreeCount,
		"abstain", vs.abstainCount)

	// 检查是否可以提前结束
	vs.checkResult()
}

// checkResult 检查投票结果
//
// 投票规则：
//   - 快速路径（成员数 < 10）：单票 AGREE 且无反对即可确认
//   - 标准路径：简单多数（> 50% 响应者 AGREE）确认
//   - 任何 DISAGREE 触发重新验证（暂不实现）
func (vs *VotingSession) checkResult() {
	totalVotes := vs.agreeCount + vs.disagreeCount + vs.abstainCount

	// 快速路径判断
	if vs.memberCount < WitnessFastPathMemberThreshold {
		// 单票 AGREE 且无反对即可确认
		if vs.agreeCount >= 1 && vs.disagreeCount == 0 {
			vs.finalize()
			return
		}

		// 有反对票，继续等待更多确认
		if vs.disagreeCount > 0 && totalVotes < vs.memberCount/2 {
			return // 等待更多投票
		}
	}

	// 标准路径：等待足够的投票数
	// 至少需要一半成员响应
	minResponses := vs.memberCount / 2
	if minResponses < 1 {
		minResponses = 1
	}

	if totalVotes < minResponses {
		return // 继续等待
	}

	// 检查是否达到多数
	effectiveVotes := vs.agreeCount + vs.disagreeCount // 弃权不计入
	if effectiveVotes == 0 {
		return // 全部弃权，继续等待
	}

	// 简单多数确认
	if vs.agreeCount > effectiveVotes/2 {
		vs.finalize()
		return
	}

	// 如果有反对票且反对占多数，否决
	if vs.disagreeCount > effectiveVotes/2 {
		vs.finalize()
		return
	}
}

// finalize 结束投票并生成结果
func (vs *VotingSession) finalize() {
	if vs.completed {
		return
	}
	vs.completed = true

	if vs.timer != nil {
		vs.timer.Stop()
	}

	// 计算最终结果
	effectiveVotes := vs.agreeCount + vs.disagreeCount
	confirmed := false

	if vs.memberCount < WitnessFastPathMemberThreshold {
		// 快速路径：单票 AGREE 无反对
		confirmed = vs.agreeCount >= 1 && vs.disagreeCount == 0
	} else {
		// 标准路径：简单多数
		if effectiveVotes > 0 {
			confirmed = vs.agreeCount > effectiveVotes/2
		}
	}

	result := &witnesspb.WitnessVotingResult{
		ReportId:          []byte(vs.reportID),
		TargetId:          []byte(vs.targetID),
		Confirmed:         confirmed,
		AgreeCount:        int32(vs.agreeCount),
		DisagreeCount:     int32(vs.disagreeCount),
		AbstainCount:      int32(vs.abstainCount),
		TotalWitnesses:    int32(vs.memberCount),
		DecisionTimestamp: time.Now().UnixNano(),
	}

	// 调用回调
	if vs.onComplete != nil {
		go vs.onComplete(result)
	}
}

// IsCompleted 检查投票是否已完成
func (vs *VotingSession) IsCompleted() bool {
	vs.mu.RLock()
	defer vs.mu.RUnlock()
	return vs.completed
}

// GetStats 获取投票统计
func (vs *VotingSession) GetStats() (agree, disagree, abstain int) {
	vs.mu.RLock()
	defer vs.mu.RUnlock()
	return vs.agreeCount, vs.disagreeCount, vs.abstainCount
}
