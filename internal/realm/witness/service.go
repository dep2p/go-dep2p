// Package witness 实现见证人协议，用于 Realm 成员断开检测确认。
//
// 见证人协议是快速断开检测机制的第三层，提供分布式断开确认：
//   - 当节点检测到成员断开时，广播见证报告
//   - 其他成员收到报告后验证并响应确认
//   - 根据投票结果决定是否移除成员
//
// 设计目标：
//   - 直连非优雅断开检测：< 10s
//   - 中继非优雅断开检测：< 15s
//   - 误判率（分区场景）：< 5%
package witness

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"sync"
	"time"

	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/lib/log"
	witnesspb "github.com/dep2p/go-dep2p/pkg/lib/proto/realm/witness"
	"github.com/dep2p/go-dep2p/pkg/types"
	"google.golang.org/protobuf/proto"
)

var logger = log.Logger("realm/witness")

// ============================================================================
//                              配置常量
// ============================================================================

const (
	// WitnessMaxBroadcastDelay 最大广播延迟
	// 避免多个节点同时广播导致网络拥塞
	WitnessMaxBroadcastDelay = 500 * time.Millisecond

	// WitnessConfirmationTimeout 确认超时时间
	WitnessConfirmationTimeout = 2 * time.Second

	// WitnessFastPathMemberThreshold 快速路径成员数阈值
	// 成员数小于此值时，可以使用快速路径确认
	WitnessFastPathMemberThreshold = 10

	// WitnessReportExpiry 报告过期时间
	WitnessReportExpiry = 10 * time.Second

	// WitnessProcessedCacheExpiry 已处理报告缓存过期时间
	WitnessProcessedCacheExpiry = 60 * time.Second
)

// ============================================================================
//                              服务接口
// ============================================================================

// MemberManager 成员管理器接口
type MemberManager interface {
	// IsMember 检查是否为成员
	IsMember(ctx context.Context, peerID string) bool
	// GetTotalCount 获取成员总数
	GetTotalCount() int
	// Remove 移除成员
	Remove(ctx context.Context, peerID string) error
	// UpdateStatus 更新成员在线状态
	UpdateStatus(ctx context.Context, peerID string, online bool) error
}

// TopicPublisher 消息发布接口
type TopicPublisher interface {
	// Publish 发布消息
	Publish(ctx context.Context, data []byte) error
}

// ============================================================================
//                              见证人服务
// ============================================================================

// Service 见证人服务
//
// 负责处理断开检测和见证确认的核心逻辑。
type Service struct {
	mu sync.RWMutex

	// 基础信息
	localID string
	realmID string

	// 依赖注入
	member    MemberManager
	topic     TopicPublisher
	peerstore pkgif.Peerstore
	host      pkgif.Host

	// 报告状态跟踪
	pendingReports   map[string]*PendingReport // report_id -> PendingReport
	processedReports map[string]time.Time      // report_id -> 处理时间
	lastContactTime  map[string]time.Time      // peer_id -> 最后联系时间

	// 投票会话管理
	votingSessions map[string]*VotingSession // report_id -> VotingSession

	// 限速器
	rateLimiter *RateLimiter

	// 生命周期
	ctx    context.Context
	cancel context.CancelFunc
}

// PendingReport 待处理的见证报告
type PendingReport struct {
	Report    *witnesspb.WitnessReport
	CreatedAt time.Time
	Broadcast bool // 是否已广播
}

// NewService 创建见证人服务
func NewService(localID, realmID string, member MemberManager) *Service {
	return &Service{
		localID:          localID,
		realmID:          realmID,
		member:           member,
		pendingReports:   make(map[string]*PendingReport),
		processedReports: make(map[string]time.Time),
		lastContactTime:  make(map[string]time.Time),
		votingSessions:   make(map[string]*VotingSession),
		rateLimiter:      NewRateLimiter(10, time.Minute), // 10/min
	}
}

// SetTopic 设置消息发布 Topic
func (s *Service) SetTopic(topic TopicPublisher) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.topic = topic
}

// SetPeerstore 设置 Peerstore（用于签名验证）
func (s *Service) SetPeerstore(ps pkgif.Peerstore) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.peerstore = ps
}

// SetHost 设置 Host（用于签名）
func (s *Service) SetHost(host pkgif.Host) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.host = host
}

// Start 启动服务
func (s *Service) Start(ctx context.Context) error {
	s.mu.Lock()
	s.ctx, s.cancel = context.WithCancel(ctx)
	s.mu.Unlock()

	// 启动后台清理协程
	go s.cleanupLoop()

	logger.Info("见证人服务已启动", "localID", truncateID(s.localID), "realmID", s.realmID)
	return nil
}

// Stop 停止服务
func (s *Service) Stop(_ context.Context) error {
	s.mu.Lock()
	if s.cancel != nil {
		s.cancel()
	}
	s.mu.Unlock()

	logger.Info("见证人服务已停止", "localID", truncateID(s.localID), "realmID", s.realmID)
	return nil
}

// ============================================================================
//                              核心方法
// ============================================================================

// OnPeerDisconnected 处理节点断开事件
//
// 当检测到成员断开时调用，触发见证报告流程。
//
// 参数：
//   - peerID: 断开的节点 ID
//   - method: 检测方法（QUIC_CLOSE, QUIC_TIMEOUT 等）
//   - lastContact: 最后一次成功通信的时间
func (s *Service) OnPeerDisconnected(ctx context.Context, peerID string, method witnesspb.DetectionMethod, lastContact time.Time) error {
	// 1. 验证目标是 Realm 成员
	if s.member == nil || !s.member.IsMember(ctx, peerID) {
		return nil // 非成员，不需要报告
	}

	// 2. 限速检查
	if !s.rateLimiter.AllowReport(peerID) {
		logger.Debug("见证报告被限速",
			"peerID", truncateID(peerID),
			"localID", truncateID(s.localID))
		return fmt.Errorf("rate limited")
	}

	// 3. 检查是否可以使用快速路径
	if s.canUseFastPath(method) {
		return s.fastPathConfirm(ctx, peerID, method, lastContact)
	}

	// 4. 标准路径：创建并广播见证报告
	report, err := s.createReport(peerID, method, lastContact)
	if err != nil {
		return fmt.Errorf("创建见证报告失败: %w", err)
	}

	// 5. 计算延迟后广播
	delay := s.calculateBroadcastDelay()
	time.AfterFunc(delay, func() {
		if err := s.broadcastReport(report); err != nil {
			logger.Debug("广播见证报告失败",
				"targetID", truncateID(peerID),
				"err", err)
		}
	})

	logger.Info("触发见证报告",
		"targetID", truncateID(peerID),
		"method", method.String(),
		"delay", delay,
		"localID", truncateID(s.localID))

	return nil
}

// canUseFastPath 检查是否可以使用快速路径
//
// 快速路径条件：
//   - 成员数 < 10
//   - 检测方法是 QUIC_CLOSE（优雅关闭）
func (s *Service) canUseFastPath(method witnesspb.DetectionMethod) bool {
	if s.member == nil {
		return false
	}

	memberCount := s.member.GetTotalCount()
	return memberCount < WitnessFastPathMemberThreshold &&
		method == witnesspb.DetectionMethod_DETECTION_METHOD_QUIC_CLOSE
}

// fastPathConfirm 快速路径确认
//
// 对于小型 Realm 的优雅断开，无需等待其他见证人确认。
// 单票 AGREE（本节点作为检测者）且无反对即可确认。
func (s *Service) fastPathConfirm(ctx context.Context, peerID string, method witnesspb.DetectionMethod, _ time.Time) error {
	logger.Info("快速路径确认成员断开",
		"targetID", truncateID(peerID),
		"method", method.String(),
		"localID", truncateID(s.localID))

	// 直接移除成员（QUIC_CLOSE 是可靠的优雅断开信号）
	if err := s.member.Remove(ctx, peerID); err != nil {
		return fmt.Errorf("快速路径移除成员失败: %w", err)
	}

	logger.Info("快速路径确认完成，成员已移除",
		"targetID", truncateID(peerID),
		"localID", truncateID(s.localID))

	return nil
}

// calculateBroadcastDelay 计算广播延迟
//
// 随机延迟 0-500ms，避免多个节点同时广播导致网络拥塞。
func (s *Service) calculateBroadcastDelay() time.Duration {
	var b [2]byte
	if _, err := rand.Read(b[:]); err != nil {
		return WitnessMaxBroadcastDelay / 2
	}
	ms := int(binary.BigEndian.Uint16(b[:])) % int(WitnessMaxBroadcastDelay.Milliseconds())
	return time.Duration(ms) * time.Millisecond
}

// createReport 创建见证报告
func (s *Service) createReport(peerID string, method witnesspb.DetectionMethod, lastContact time.Time) (*witnesspb.WitnessReport, error) {
	// 生成报告 ID
	reportID := make([]byte, 16)
	if _, err := rand.Read(reportID); err != nil {
		return nil, fmt.Errorf("生成报告 ID 失败: %w", err)
	}

	timestamp := time.Now().UnixNano()

	report := &witnesspb.WitnessReport{
		ReportId:             reportID,
		ReporterId:           []byte(s.localID),
		TargetId:             []byte(peerID),
		RealmId:              []byte(s.realmID),
		DetectionMethod:      method,
		Timestamp:            timestamp,
		LastContactTimestamp: lastContact.UnixNano(),
	}

	// 签名报告
	sig, err := s.signReport(report)
	if err != nil {
		logger.Debug("签名见证报告失败", "err", err)
		// 继续，签名是可选的安全增强
	} else {
		report.Signature = sig
	}

	// 存储待处理报告
	s.mu.Lock()
	s.pendingReports[string(reportID)] = &PendingReport{
		Report:    report,
		CreatedAt: time.Now(),
		Broadcast: false,
	}
	s.mu.Unlock()

	return report, nil
}

// broadcastReport 广播见证报告
func (s *Service) broadcastReport(report *witnesspb.WitnessReport) error {
	s.mu.RLock()
	topic := s.topic
	s.mu.RUnlock()

	if topic == nil {
		return fmt.Errorf("topic not configured")
	}

	// 序列化报告
	data, err := proto.Marshal(report)
	if err != nil {
		return fmt.Errorf("序列化见证报告失败: %w", err)
	}

	// 添加消息类型前缀
	payload := append([]byte("witness:"), data...)

	// 发布
	if err := topic.Publish(s.ctx, payload); err != nil {
		return fmt.Errorf("发布见证报告失败: %w", err)
	}

	// 标记已广播
	s.mu.Lock()
	reportID := string(report.ReportId)
	if pending, exists := s.pendingReports[reportID]; exists {
		pending.Broadcast = true
	}
	s.mu.Unlock()

	logger.Info("广播见证报告",
		"reportID", truncateID(reportID),
		"targetID", truncateID(string(report.TargetId)),
		"method", report.DetectionMethod.String())

	// 创建投票会话
	s.createVotingSession(report)

	return nil
}

// HandleWitnessReport 处理收到的见证报告
func (s *Service) HandleWitnessReport(ctx context.Context, payload []byte, from string) {
	// 1. 解析报告
	report := &witnesspb.WitnessReport{}
	if err := proto.Unmarshal(payload, report); err != nil {
		logger.Debug("解析见证报告失败", "err", err)
		return
	}

	reportID := string(report.ReportId)
	targetID := string(report.TargetId)
	realmID := string(report.RealmId)

	// 2. 基本验证
	if realmID != s.realmID {
		logger.Debug("见证报告 RealmID 不匹配")
		return
	}

	// 3. 时间戳检查
	msgTime := time.Unix(0, report.Timestamp)
	now := time.Now()
	if now.Sub(msgTime) > WitnessReportExpiry {
		logger.Debug("见证报告已过期",
			"reportID", truncateID(reportID),
			"msgTime", msgTime)
		return
	}

	// 4. 防重放检查
	s.mu.Lock()
	if _, processed := s.processedReports[reportID]; processed {
		s.mu.Unlock()
		return
	}
	s.processedReports[reportID] = now
	s.mu.Unlock()

	logger.Info("收到见证报告",
		"reportID", truncateID(reportID),
		"targetID", truncateID(targetID),
		"from", truncateID(from),
		"method", report.DetectionMethod.String())

	// 5. 验证目标是否为成员
	if s.member == nil || !s.member.IsMember(ctx, targetID) {
		logger.Debug("见证报告目标不是成员",
			"targetID", truncateID(targetID))
		return
	}

	// 6. 创建或加入投票会话
	session := s.getOrCreateVotingSession(report)

	// 7. 生成本节点的确认
	confirmation := s.generateConfirmation(report)

	// 8. 添加到投票会话
	session.AddConfirmation(confirmation)

	// 9. 广播确认
	if err := s.broadcastConfirmation(confirmation); err != nil {
		logger.Debug("广播见证确认失败", "err", err)
	}
}

// HandleWitnessConfirmation 处理收到的见证确认
func (s *Service) HandleWitnessConfirmation(_ context.Context, payload []byte, from string) {
	// 1. 解析确认
	confirmation := &witnesspb.WitnessConfirmation{}
	if err := proto.Unmarshal(payload, confirmation); err != nil {
		logger.Debug("解析见证确认失败", "err", err)
		return
	}

	reportID := string(confirmation.ReportId)

	// 2. 查找对应的投票会话
	s.mu.RLock()
	session, exists := s.votingSessions[reportID]
	s.mu.RUnlock()

	if !exists {
		logger.Debug("见证确认对应的投票会话不存在",
			"reportID", truncateID(reportID))
		return
	}

	// 3. 添加确认到会话
	session.AddConfirmation(confirmation)

	logger.Debug("收到见证确认",
		"reportID", truncateID(reportID),
		"from", truncateID(from),
		"type", confirmation.ConfirmationType.String())
}

// generateConfirmation 生成本节点的见证确认
func (s *Service) generateConfirmation(report *witnesspb.WitnessReport) *witnesspb.WitnessConfirmation {
	reporterLastContact := time.Unix(0, report.LastContactTimestamp)

	// 获取本节点与目标的最后联系时间
	s.mu.RLock()
	myLastContact, hasContact := s.lastContactTime[string(report.TargetId)]
	s.mu.RUnlock()

	var confirmType witnesspb.ConfirmationType
	var reason string

	if !hasContact {
		// 未与目标节点有过联系，弃权
		confirmType = witnesspb.ConfirmationType_CONFIRMATION_TYPE_ABSTAIN
		reason = "no contact history"
	} else if myLastContact.After(reporterLastContact) {
		// 本节点有更新的联系记录，反对
		confirmType = witnesspb.ConfirmationType_CONFIRMATION_TYPE_DISAGREE
		reason = fmt.Sprintf("newer contact at %v", myLastContact)
	} else {
		// 同意断开报告
		confirmType = witnesspb.ConfirmationType_CONFIRMATION_TYPE_AGREE
		reason = "confirmed disconnect"
	}

	confirmation := &witnesspb.WitnessConfirmation{
		ReportId:             report.ReportId,
		WitnessId:            []byte(s.localID),
		ConfirmationType:     confirmType,
		Timestamp:            time.Now().UnixNano(),
		LastContactTimestamp: myLastContact.UnixNano(),
		Reason:               reason,
	}

	// 签名确认
	sig, err := s.signConfirmation(confirmation)
	if err == nil {
		confirmation.Signature = sig
	}

	return confirmation
}

// broadcastConfirmation 广播见证确认
func (s *Service) broadcastConfirmation(confirmation *witnesspb.WitnessConfirmation) error {
	s.mu.RLock()
	topic := s.topic
	s.mu.RUnlock()

	if topic == nil {
		return fmt.Errorf("topic not configured")
	}

	data, err := proto.Marshal(confirmation)
	if err != nil {
		return fmt.Errorf("序列化见证确认失败: %w", err)
	}

	payload := append([]byte("wconfirm:"), data...)

	return topic.Publish(s.ctx, payload)
}

// ============================================================================
//                              投票会话管理
// ============================================================================

// createVotingSession 创建投票会话
func (s *Service) createVotingSession(report *witnesspb.WitnessReport) *VotingSession {
	reportID := string(report.ReportId)
	targetID := string(report.TargetId)

	memberCount := 0
	if s.member != nil {
		memberCount = s.member.GetTotalCount()
	}

	session := NewVotingSession(reportID, targetID, memberCount, s.onVotingComplete)

	s.mu.Lock()
	s.votingSessions[reportID] = session
	s.mu.Unlock()

	// 启动超时定时器
	session.StartTimeout(WitnessConfirmationTimeout)

	return session
}

// getOrCreateVotingSession 获取或创建投票会话
func (s *Service) getOrCreateVotingSession(report *witnesspb.WitnessReport) *VotingSession {
	s.mu.RLock()
	session, exists := s.votingSessions[string(report.ReportId)]
	s.mu.RUnlock()

	if exists {
		return session
	}

	return s.createVotingSession(report)
}

// onVotingComplete 投票完成回调
func (s *Service) onVotingComplete(result *witnesspb.WitnessVotingResult) {
	targetID := string(result.TargetId)
	reportID := string(result.ReportId)

	if result.Confirmed {
		logger.Info("见证投票确认成员断开",
			"reportID", truncateID(reportID),
			"targetID", truncateID(targetID),
			"agree", result.AgreeCount,
			"disagree", result.DisagreeCount,
			"abstain", result.AbstainCount)

		// 移除成员
		if s.member != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := s.member.Remove(ctx, targetID); err != nil {
				logger.Debug("投票确认后移除成员失败",
					"targetID", truncateID(targetID),
					"err", err)
			}
		}
	} else {
		logger.Info("见证投票未确认成员断开",
			"reportID", truncateID(reportID),
			"targetID", truncateID(targetID),
			"agree", result.AgreeCount,
			"disagree", result.DisagreeCount,
			"abstain", result.AbstainCount)
	}

	// 清理投票会话
	s.mu.Lock()
	delete(s.votingSessions, reportID)
	s.mu.Unlock()
}

// ============================================================================
//                              联系时间跟踪
// ============================================================================

// UpdateLastContact 更新与节点的最后联系时间
func (s *Service) UpdateLastContact(peerID string, timestamp time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()

	current, exists := s.lastContactTime[peerID]
	if !exists || timestamp.After(current) {
		s.lastContactTime[peerID] = timestamp
	}
}

// ============================================================================
//                              签名
// ============================================================================

// signReport 签名见证报告
func (s *Service) signReport(report *witnesspb.WitnessReport) ([]byte, error) {
	s.mu.RLock()
	host := s.host
	ps := s.peerstore
	s.mu.RUnlock()

	if host == nil || ps == nil {
		return nil, fmt.Errorf("host or peerstore not available")
	}

	privKey, err := ps.PrivKey(types.PeerID(s.localID))
	if err != nil {
		return nil, fmt.Errorf("获取私钥失败: %w", err)
	}

	// 构建签名数据
	data := s.buildReportSignData(report)
	return privKey.Sign(data)
}

// signConfirmation 签名见证确认
func (s *Service) signConfirmation(confirmation *witnesspb.WitnessConfirmation) ([]byte, error) {
	s.mu.RLock()
	host := s.host
	ps := s.peerstore
	s.mu.RUnlock()

	if host == nil || ps == nil {
		return nil, fmt.Errorf("host or peerstore not available")
	}

	privKey, err := ps.PrivKey(types.PeerID(s.localID))
	if err != nil {
		return nil, fmt.Errorf("获取私钥失败: %w", err)
	}

	// 构建签名数据
	data := s.buildConfirmationSignData(confirmation)
	return privKey.Sign(data)
}

// buildReportSignData 构建报告签名数据
func (s *Service) buildReportSignData(report *witnesspb.WitnessReport) []byte {
	// report_id || reporter_id || target_id || realm_id || detection_method || timestamp
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

// buildConfirmationSignData 构建确认签名数据
func (s *Service) buildConfirmationSignData(confirmation *witnesspb.WitnessConfirmation) []byte {
	// report_id || witness_id || confirmation_type || timestamp
	totalLen := len(confirmation.ReportId) + len(confirmation.WitnessId) + 4 + 8

	data := make([]byte, totalLen)
	offset := 0

	copy(data[offset:], confirmation.ReportId)
	offset += len(confirmation.ReportId)

	copy(data[offset:], confirmation.WitnessId)
	offset += len(confirmation.WitnessId)

	binary.BigEndian.PutUint32(data[offset:], uint32(confirmation.ConfirmationType))
	offset += 4

	binary.BigEndian.PutUint64(data[offset:], uint64(confirmation.Timestamp))

	return data
}

// ============================================================================
//                              后台清理
// ============================================================================

// cleanupLoop 后台清理协程
func (s *Service) cleanupLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.cleanup()
		}
	}
}

// cleanup 清理过期数据
func (s *Service) cleanup() {
	now := time.Now()

	s.mu.Lock()
	defer s.mu.Unlock()

	// 清理已处理报告缓存
	for reportID, processedAt := range s.processedReports {
		if now.Sub(processedAt) > WitnessProcessedCacheExpiry {
			delete(s.processedReports, reportID)
		}
	}

	// 清理过期的待处理报告
	for reportID, pending := range s.pendingReports {
		if now.Sub(pending.CreatedAt) > WitnessReportExpiry*2 {
			delete(s.pendingReports, reportID)
		}
	}
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
