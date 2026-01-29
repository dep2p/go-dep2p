# 快速断开检测实施指南

**创建日期**: 2026-01-28  
**状态**: ✅ **已实施** (2026-01-28 验证通过)  
**来源讨论**: `_discussions/20260128-fast-disconnect-detection.md`  
**行为文档**: `03_architecture/L3_behavioral/disconnect_detection.md`  
**需求文档**: `01_context/requirements/functional/F3_network/REQ-NET-007.md`  
**架构决策**: `01_context/decisions/ADR-0012-disconnect-detection.md`

> **实施验证**: 所有核心组件已完整实现，代码位于 `internal/realm/` 和 `internal/core/swarm/` 目录。

---

## 一、实施目标

### 1.1 核心目标

> **目标：任何节点（直连或中继）的非优雅断开，都能在 15 秒内被感知并传播。**

| 指标 | 当前状态 | 目标状态 |
|------|----------|----------|
| 优雅断开检测延迟 | 瞬时 ✓ | < 100ms |
| 直连非优雅断开检测 | 30s ~ 2min | **< 10s** |
| 中继非优雅断开检测 | 30s ~ 2min | **< 15s** |
| 见证确认延迟 | 不存在 | **< 3s** |
| 误判率（分区场景） | 未处理 | **< 5%** |

### 1.2 不做什么

| 不做 | 原因 |
|------|------|
| ❌ 复杂的信誉系统 | 初期版本保持简单，后续迭代 |
| ❌ 加权投票 | Quorum 机制已足够，避免过度设计 |
| ❌ 向后兼容旧协议 | 彻底重构，不背包袱 |
| ❌ 伪实现/桩代码 | 每个功能必须完整可用 |

---

## 二、架构决策（基于讨论稿修订）

### 2.1 四层检测架构（保留）

```
Layer 1: QUIC Keep-Alive (3s/6s) ──────────► 直连检测 ~6s
Layer 2: MemberLeave 广播 ──────────────────► 优雅断开 <100ms
Layer 3: 见证人网络（简化版） ────────────────► 非直连传播 ~3s
Layer 4: Liveness Ping（兜底） ────────────────► 最终确认 ~15s
```

### 2.2 关键修订（基于讨论稿缺陷分析）

| 讨论稿缺陷 | 修订方案 |
|-----------|---------|
| QUIC 配置 5s/15s = 20s | **修正为 3s/6s = 9s** |
| 投票要求 ≥2 有效票导致小网络阻塞 | **单见证人 + 强证据可直接确认** |
| QUIC_TIMEOUT 被视为高可信 | **仅 QUIC_CLOSE 为高可信** |
| 非直连节点多为 ABSTAIN | **引入 Relay 代理 Ping 机制** |
| Relay 批量误报风险 | **批量异常抑制 + 健康绑定** |
| Gossip 传播无规范 | **简化为 PubSub 广播** |

### 2.3 最终参数配置

```go
// 推荐配置（实施时使用）
const (
    // Layer 1: QUIC
    QUICKeepAlivePeriod = 3 * time.Second   // 心跳间隔
    QUICMaxIdleTimeout  = 6 * time.Second   // 空闲超时
    // 最长检测延迟 = 6s（错过一次心跳）
    
    // Layer 2: MemberLeave
    MemberLeaveWaitBeforeClose = 50 * time.Millisecond
    
    // Layer 3: 见证人
    WitnessMaxBroadcastDelay       = 500 * time.Millisecond
    WitnessConfirmationTimeout     = 2 * time.Second
    WitnessFastPathMemberThreshold = 10  // 成员数 < 10 可用快速路径
    WitnessReportExpiry            = 10 * time.Second  // 报告过期时间
    WitnessRateLimitPerMinute      = 10  // 每个见证人每分钟最多报告数
    
    // Layer 4: Liveness（兜底机制）
    // 注意：需同步更新 pkg/interfaces/liveness.go 中的 DefaultLivenessConfig()
    LivenessPingInterval = 30 * time.Second  // 与 pkg/interfaces/liveness.go 一致
    LivenessPingTimeout  = 5 * time.Second   // 与 pkg/interfaces/liveness.go 一致（原值，保守设置）
    LivenessMaxFailures  = 3   // 与 pkg/interfaces/liveness.go 一致（FailThreshold）
    // 总兜底检测延迟 ≈ 30s × 3 = 90s（最坏情况）
    
    // 重连宽限期（防止网络切换误判）
    ReconnectGracePeriod     = 15 * time.Second  // 断开后等待重连时间
    MaxGracePeriodExtensions = 2                 // 最多延长次数
    
    // 断开保护期（防止竞态重新添加）
    DisconnectProtection = 30 * time.Second  // 刚断开的成员不被 PubSub/DHT 重新添加
    
    // 震荡检测（防止抖动）
    FlapWindowDuration = 60 * time.Second   // 检测窗口
    FlapThreshold      = 3                  // 窗口内断开/重连次数阈值
    FlappingRecovery   = 5 * time.Minute    // 震荡恢复时间
    
    // Relay 代理
    RelayBatchAnomalyThreshold = 5   // 60s 内报告 >5 个离线触发抑制
    RelayBatchAnomalyWindow    = 60 * time.Second
)

// 协议 ID 构建（使用 pkg/protocol 的 AppBuilder 模式）
// 注意：协议 ID 包含 realmID，需要动态构建
//
// 使用方式：
//   builder := protocol.NewAppBuilder(realmID)
//   memberLeaveID := builder.Custom("member-leave", "1.0.0")
//   witnessReportID := builder.Custom("witness-report", "1.0.0")
//   witnessConfirmID := builder.Custom("witness-confirm", "1.0.0")
//
// 生成的协议 ID 格式：
//   /dep2p/app/{realmID}/member-leave/1.0.0
//   /dep2p/app/{realmID}/witness-report/1.0.0
//   /dep2p/app/{realmID}/witness-confirm/1.0.0

// 协议名称常量（用于 AppBuilder.Custom()）
const (
    AppProtocolMemberLeave    = "member-leave"
    AppProtocolWitnessReport  = "witness-report"
    AppProtocolWitnessConfirm = "witness-confirm"
)
```

---

## 三、实施任务清单

### Phase 1: 传输层优化（P0）

#### 任务 1.1: QUIC Keep-Alive 配置

**文件**: `internal/core/transport/quic/config.go`

```go
// 修改 DefaultConfig
func DefaultConfig() *Config {
    return &Config{
        // 快速检测配置
        KeepAlivePeriod: 3 * time.Second,  // 从 5s 改为 3s
        MaxIdleTimeout:  6 * time.Second,  // 从 15s 改为 6s
        
        // 其他配置保持不变
        HandshakeTimeout: 10 * time.Second,
    }
}
```

**验证**:
```bash
# 测试非优雅断开检测延迟
go test -v -run TestQUICUngracefulDisconnect ./internal/core/transport/quic/
# 期望: 检测延迟 < 10s
```

#### 任务 1.2: 连接断开事件传播

**修改文件**:
1. `pkg/types/events.go` - 扩展 EvtPeerDisconnected
2. `internal/core/swarm/swarm.go` - 触发增强事件

**修改要点**:
1. 扩展现有的 `pkg/types/events.go` 中的 `EvtPeerDisconnected`（而非重新定义）
2. 确保 `EvtPeerDisconnected` 在 QUIC 连接关闭时立即触发
3. 事件中包含断开原因（`Graceful` / `Timeout` / `Error`）

**步骤 1**: 修改 `pkg/types/events.go`

```go
// DisconnectReason 断开原因（新增类型）
type DisconnectReason int

const (
    DisconnectReasonUnknown DisconnectReason = iota
    DisconnectReasonGraceful    // QUIC CLOSE 帧（高可信）
    DisconnectReasonTimeout     // 空闲超时（低可信）
    DisconnectReasonError       // 连接错误
    DisconnectReasonLocal       // 本地主动断开
)

// EvtPeerDisconnected 节点断开事件（扩展现有定义）
// 位置：pkg/types/events.go:59-64
type EvtPeerDisconnected struct {
    BaseEvent
    PeerID   PeerID
    NumConns int
    // v1.1 新增字段
    Reason   DisconnectReason  // 断开原因
}
```

**步骤 2**: 修改 `internal/core/swarm/swarm.go`

---

### Phase 2: 应用层主动通知（P0）

#### 任务 2.1: MemberLeave 协议定义

**文件**: `pkg/lib/proto/realm/memberleave/member_leave.proto` (新建)

> **注意**: 遵循 pkg/lib/proto/realm/ 目录结构

```protobuf
syntax = "proto3";
package realm.memberleave;

option go_package = "github.com/dep2p/go-dep2p/pkg/lib/proto/realm/memberleave;memberleavepb";

// MemberLeave 成员离开通知
message MemberLeave {
    bytes peer_id = 1;      // 使用 bytes 与现有 proto 保持一致
    bytes realm_id = 2;     // 使用 bytes 与现有 proto 保持一致
    LeaveReason reason = 3;
    int64 timestamp = 4;
    bytes signature = 5;
}

// LeaveReason 枚举（与设计文档 liveness.md 一致）
enum LeaveReason {
    LEAVE_REASON_UNKNOWN = 0;
    LEAVE_REASON_GRACEFUL = 1;   // 主动关闭
    LEAVE_REASON_KICKED = 2;     // 被踢出（管理员操作）
    LEAVE_REASON_WITNESS = 3;    // 见证人报告（非优雅断开）
}
```

#### 任务 2.2: MemberLeave 发送逻辑

**文件**: `internal/realm/realm.go`

```go
// BroadcastMemberLeave 优雅关闭时广播离开消息
func (r *realmImpl) BroadcastMemberLeave(ctx context.Context) error {
    msg := &pb.MemberLeave{
        PeerId:    string(r.host.ID()),
        RealmId:   r.id,
        Reason:    pb.LeaveReason_LEAVE_REASON_GRACEFUL,
        Timestamp: time.Now().UnixNano(),
    }
    
    // 签名
    sig, err := r.host.PrivKey().Sign(msg.Marshal())
    if err != nil {
        return err
    }
    msg.Signature = sig
    
    // 通过 PubSub 广播（使用现有 memberSyncTopic）
    data, _ := proto.Marshal(msg)
    return r.memberSyncTopic.Publish(ctx, data)
}

// Stop 修改为先广播再关闭
func (r *realmImpl) Stop(ctx context.Context) error {
    // 1. 广播离开消息
    broadcastCtx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
    defer cancel()
    _ = r.BroadcastMemberLeave(broadcastCtx)
    
    // 2. 短暂等待消息传播
    time.Sleep(MemberLeaveWaitBeforeClose)
    
    // 3. 正常关闭
    return r.close()
}
```

#### 任务 2.3: MemberLeave 接收处理

**文件**: `internal/realm/member/manager.go`

```go
// handleMemberLeave 处理成员离开消息
func (m *Manager) handleMemberLeave(data []byte) {
    var msg pb.MemberLeave
    if err := proto.Unmarshal(data, &msg); err != nil {
        return
    }
    
    // 1. 验证签名
    if !m.verifyMemberLeaveSignature(&msg) {
        m.logger.Warn("MemberLeave 签名验证失败", "from", msg.PeerId[:8])
        return
    }
    
    // 2. 检查时间戳（30s 内有效）
    if time.Since(time.Unix(0, msg.Timestamp)) > 30*time.Second {
        m.logger.Warn("MemberLeave 消息过期", "from", msg.PeerId[:8])
        return
    }
    
    // 3. 移除成员
    m.Remove(peer.ID(msg.PeerId))
    m.logger.Info("收到成员离开通知", "peerID", msg.PeerId[:8])
}
```

---

### Phase 3: 见证人机制（P1）

#### 任务 3.1: 见证人服务

**文件**: `internal/realm/witness/service.go` (新建)

```go
package witness

import (
    "context"
    "sync"
    "time"
    
    "dep2p/pkg/interfaces"
)

// Service 见证人服务
type Service struct {
    mu sync.RWMutex
    
    host   interfaces.Host
    realm  interfaces.Realm
    config *Config
    
    // 待广播报告（延迟去重）
    pendingReports map[string]*pendingReport
    
    // 已处理报告（防重复）
    processedReports map[string]time.Time
}

// Config 配置
type Config struct {
    MaxBroadcastDelay   time.Duration
    ConfirmationTimeout time.Duration
    FastPathThreshold   int
    TrustedRoles        []string  // "bootstrap", "relay"
}

func DefaultConfig() *Config {
    return &Config{
        MaxBroadcastDelay:   500 * time.Millisecond,
        ConfirmationTimeout: 2 * time.Second,
        FastPathThreshold:   10,
        TrustedRoles:        []string{"bootstrap", "relay"},
    }
}

// OnPeerDisconnected 当检测到节点断开时调用
func (s *Service) OnPeerDisconnected(peerID string, method DetectionMethod) {
    s.mu.Lock()
    defer s.mu.Unlock()
    
    // 检查是否已有待处理
    if _, exists := s.pendingReports[peerID]; exists {
        return
    }
    
    // 创建报告
    report := &WitnessReport{
        WitnessID:  string(s.host.ID()),
        TargetID:   peerID,
        RealmID:    s.realm.ID(),
        Method:     method,
        DetectedAt: time.Now().UnixNano(),
    }
    
    // 检查快速路径
    if s.canUseFastPath(report) {
        s.fastPathConfirm(report)
        return
    }
    
    // 计算延迟广播
    delay := s.calculateBroadcastDelay(peerID)
    pending := &pendingReport{
        report: report,
        timer: time.AfterFunc(delay, func() {
            s.broadcastReport(report)
        }),
    }
    s.pendingReports[peerID] = pending
}

// canUseFastPath 检查是否可以使用快速路径
func (s *Service) canUseFastPath(report *WitnessReport) bool {
    // 条件 1: 成员数少
    if s.realm.MemberManager().Count() >= s.config.FastPathThreshold {
        return false
    }
    
    // 条件 2: 检测方式是高可信（仅 QUIC_CLOSE）
    if report.Method != DetectionMethod_QUIC_CLOSE {
        return false
    }
    
    // 条件 3: 见证人是受信任角色（可选）
    // 简化版不检查角色
    
    return true
}

// fastPathConfirm 快速路径确认
func (s *Service) fastPathConfirm(report *WitnessReport) {
    s.mu.Lock()
    s.processedReports[report.TargetID] = time.Now()
    s.mu.Unlock()
    
    // 直接移除成员
    s.realm.MemberManager().Remove(peer.ID(report.TargetID))
    
    // 广播 WitnessReport（通知其他节点）
    s.broadcastReport(report)
}
```

#### 任务 3.2: 见证确认协议

**文件**: `pkg/lib/proto/realm/witness/witness.proto` (新建)

> **注意**: 遵循 pkg/lib/proto/realm/ 目录结构

```protobuf
syntax = "proto3";
package realm.witness;

option go_package = "github.com/dep2p/go-dep2p/pkg/lib/proto/realm/witness;witnesspb";

// WitnessReport 见证人报告
message WitnessReport {
    string report_id = 1;
    string witness_id = 2;
    string target_id = 3;
    string realm_id = 4;
    DetectionMethod method = 5;
    int64 detected_at = 6;
    int64 reported_at = 7;
    bytes signature = 8;
    bytes last_contact_proof = 9;  // 可选：最后通信证据（消息哈希+时间戳）
}

enum DetectionMethod {
    DETECTION_UNKNOWN = 0;
    DETECTION_QUIC_CLOSE = 1;      // QUIC 主动关闭（高可信）
    DETECTION_QUIC_TIMEOUT = 2;    // QUIC 超时（中可信）
    DETECTION_PING_FAILED = 3;     // Ping 失败
    DETECTION_RELAY_CIRCUIT = 4;   // Relay Circuit 断开
}

// WitnessConfirmation 见证确认
message WitnessConfirmation {
    string report_id = 1;
    string confirmer_id = 2;
    string target_id = 3;
    ConfirmationType type = 4;
    int64 timestamp = 5;
    bytes signature = 6;
}

enum ConfirmationType {
    CONFIRM_UNKNOWN = 0;
    CONFIRM_AGREE = 1;      // 我也检测到离线
    CONFIRM_DISAGREE = 2;   // 我仍能连接
    CONFIRM_ABSTAIN = 3;    // 我与目标无连接
}
```

#### 任务 3.3: 见证确认投票

**文件**: `internal/realm/witness/voting.go` (新建)

```go
package witness

import (
    "sync"
    "time"
)

// VotingSession 投票会话
type VotingSession struct {
    mu sync.Mutex
    
    reportID      string
    targetID      string
    startTime     time.Time
    confirmations map[string]*WitnessConfirmation
    
    result chan VotingResult
    done   bool
}

// VotingResult 投票结果
type VotingResult struct {
    TargetID  string
    Confirmed bool
    Agree     int
    Disagree  int
    Abstain   int
}

// AddConfirmation 添加确认
func (vs *VotingSession) AddConfirmation(c *WitnessConfirmation) {
    vs.mu.Lock()
    defer vs.mu.Unlock()
    
    if vs.done {
        return
    }
    
    vs.confirmations[c.ConfirmerId] = c
    vs.checkResult()
}

// checkResult 检查是否可以得出结论
func (vs *VotingSession) checkResult() {
    agree := 0
    disagree := 0
    abstain := 0
    
    for _, c := range vs.confirmations {
        switch c.Type {
        case ConfirmationType_CONFIRM_AGREE:
            agree++
        case ConfirmationType_CONFIRM_DISAGREE:
            disagree++
        case ConfirmationType_CONFIRM_ABSTAIN:
            abstain++
        }
    }
    
    // 修订：单个 AGREE 且无 DISAGREE 即可确认
    // 解决小网络无法形成 Quorum 的问题
    effectiveVotes := agree + disagree
    
    var confirmed bool
    if effectiveVotes > 0 {
        if disagree == 0 && agree >= 1 {
            // 无反对，有一个同意即确认
            confirmed = true
        } else if effectiveVotes >= 2 && float64(agree)/float64(effectiveVotes) > 0.5 {
            // 多票时，简单多数
            confirmed = true
        }
    }
    
    // 有结论才发送
    if confirmed || (effectiveVotes >= 2 && float64(disagree)/float64(effectiveVotes) >= 0.5) {
        vs.done = true
        select {
        case vs.result <- VotingResult{
            TargetID:  vs.targetID,
            Confirmed: confirmed,
            Agree:     agree,
            Disagree:  disagree,
            Abstain:   abstain,
        }:
        default:
        }
    }
}
```

---

### Phase 4: Relay 代理见证（P1）

#### 任务 4.1: Relay 见证扩展

**文件**: `internal/core/relay/server/witness.go` (新建)

```go
package server

import (
    "sync"
    "time"
)

// WitnessExtension Relay 见证扩展
type WitnessExtension struct {
    mu sync.RWMutex
    
    server   *Server
    circuits map[string]*circuitInfo
    
    // 批量异常检测
    recentReports []reportRecord
}

type circuitInfo struct {
    circuitID    string
    srcPeerID    string
    dstPeerID    string
    createdAt    time.Time
    lastActivity time.Time
}

type reportRecord struct {
    targetID  string
    timestamp time.Time
}

// OnCircuitClosed 电路关闭时的见证处理
func (e *WitnessExtension) OnCircuitClosed(circuitID string, reason CircuitCloseReason) {
    e.mu.Lock()
    info, exists := e.circuits[circuitID]
    delete(e.circuits, circuitID)
    e.mu.Unlock()
    
    if !exists {
        return
    }
    
    // 检查批量异常（防止 Relay 故障导致误报）
    if e.isAnomalyDetected() {
        e.server.logger.Warn("检测到批量异常，抑制见证报告")
        return
    }
    
    // 根据原因发送代理见证报告
    switch reason {
    case CircuitCloseReasonSrcDisconnect:
        e.sendProxyWitness(info.dstPeerID, info.srcPeerID)
    case CircuitCloseReasonDstDisconnect:
        e.sendProxyWitness(info.srcPeerID, info.dstPeerID)
    case CircuitCloseReasonTimeout:
        // 超时需要调查
        e.investigateTimeout(info)
    }
}

// isAnomalyDetected 检测是否存在批量异常
func (e *WitnessExtension) isAnomalyDetected() bool {
    e.mu.RLock()
    defer e.mu.RUnlock()
    
    // 清理过期记录
    cutoff := time.Now().Add(-RelayBatchAnomalyWindow)
    validReports := make([]reportRecord, 0)
    for _, r := range e.recentReports {
        if r.timestamp.After(cutoff) {
            validReports = append(validReports, r)
        }
    }
    e.recentReports = validReports
    
    // 检查是否超过阈值
    return len(validReports) >= RelayBatchAnomalyThreshold
}

// sendProxyWitness 发送代理见证报告
func (e *WitnessExtension) sendProxyWitness(onBehalfOf, targetID string) {
    e.mu.Lock()
    e.recentReports = append(e.recentReports, reportRecord{
        targetID:  targetID,
        timestamp: time.Now(),
    })
    e.mu.Unlock()
    
    // 通过 WitnessService 广播
    if ws := e.server.getWitnessService(); ws != nil {
        ws.OnPeerDisconnected(targetID, DetectionMethod_RELAY_CIRCUIT)
    }
}
```

---

### Phase 5: Liveness 兜底（P2）

#### 任务 5.1: 健康检查器增强

**文件**: `internal/core/swarm/health.go`

```go
// HealthChecker 连接健康检查器
type HealthChecker struct {
    swarm  *Swarm
    config *HealthConfig
    
    // 失败计数
    failureCount map[peer.ID]int
}

// HealthConfig 健康检查配置
type HealthConfig struct {
    Interval    time.Duration  // 检查间隔
    Timeout     time.Duration  // Ping 超时
    MaxFailures int            // 最大失败次数
}

func DefaultHealthConfig() *HealthConfig {
    // 与 pkg/interfaces/liveness.go 的 DefaultLivenessConfig() 保持一致
    return &HealthConfig{
        Interval:    LivenessPingInterval,  // 30s
        Timeout:     LivenessPingTimeout,   // 5s（与 pkg 一致）
        MaxFailures: LivenessMaxFailures,   // 3
    }
}

// checkAllConnections 检查所有连接
func (hc *HealthChecker) checkAllConnections(ctx context.Context) {
    peers := hc.swarm.Peers()
    
    for _, peerID := range peers {
        go hc.checkPeer(ctx, peerID)
    }
}

// checkPeer 检查单个 peer
func (hc *HealthChecker) checkPeer(ctx context.Context, peerID peer.ID) {
    ctx, cancel := context.WithTimeout(ctx, hc.config.Timeout)
    defer cancel()
    
    // 使用 Liveness Ping
    _, err := hc.swarm.liveness.Ping(ctx, peerID)
    
    if err != nil {
        hc.failureCount[peerID]++
        
        if hc.failureCount[peerID] >= hc.config.MaxFailures {
            // 触发断开
            hc.swarm.logger.Info("健康检查失败，关闭连接",
                "peer", peerID.String()[:8],
                "failures", hc.failureCount[peerID])
            
            hc.swarm.ClosePeer(peerID)
            delete(hc.failureCount, peerID)
        }
    } else {
        // 重置计数
        delete(hc.failureCount, peerID)
    }
}
```

---

### Phase 6: 防误判机制（P1）

#### 任务 6.1: 成员状态机

**文件**: `internal/realm/member/state.go` (新建)

> **注意**: 复用 `pkg/types/connection.go` 中已有的 `ConnState` 类型

```go
package member

import (
    "sync"
    "time"
    
    "github.com/libp2p/go-libp2p/core/peer"
    "github.com/dep2p/go-dep2p/pkg/types"
)

// MemberConnectionState 使用 types.ConnState 作为状态类型
// 复用现有定义（pkg/types/connection.go:65-76）:
//   ConnStateConnecting    = 0  // 连接中
//   ConnStateConnected     = 1  // 已连接
//   ConnStateDisconnecting = 2  // 断开中（宽限期内）
//   ConnStateDisconnected  = 3  // 已断开

// MemberConnectionState 成员连接状态跟踪
type MemberConnectionState struct {
    mu sync.RWMutex
    
    peerID           peer.ID
    state            types.ConnState  // 复用 pkg/types 定义
    disconnectTime   time.Time
    graceExtensions  int
    graceTimer       *time.Timer
}

// NewMemberConnectionState 创建成员连接状态
func NewMemberConnectionState(peerID peer.ID) *MemberConnectionState {
    return &MemberConnectionState{
        peerID: peerID,
        state:  types.ConnStateConnected,  // 使用 types.ConnState
    }
}

// OnDisconnect 处理断开事件
func (mcs *MemberConnectionState) OnDisconnect(onGraceExpired func()) {
    mcs.mu.Lock()
    defer mcs.mu.Unlock()
    
    if mcs.state != types.ConnStateConnected {
        return
    }
    
    mcs.state = types.ConnStateDisconnecting
    mcs.disconnectTime = time.Now()
    mcs.graceExtensions = 0
    
    // 启动宽限期定时器
    mcs.graceTimer = time.AfterFunc(ReconnectGracePeriod, func() {
        mcs.onGraceTimeout(onGraceExpired)
    })
}

// OnReconnect 处理重连事件
func (mcs *MemberConnectionState) OnReconnect() bool {
    mcs.mu.Lock()
    defer mcs.mu.Unlock()
    
    if mcs.state == types.ConnStateDisconnecting {
        // 取消宽限期定时器
        if mcs.graceTimer != nil {
            mcs.graceTimer.Stop()
            mcs.graceTimer = nil
        }
        mcs.state = types.ConnStateConnected
        return true
    }
    
    return false
}

// onGraceTimeout 宽限期超时处理
func (mcs *MemberConnectionState) onGraceTimeout(onExpired func()) {
    mcs.mu.Lock()
    
    if mcs.state != types.ConnStateDisconnecting {
        mcs.mu.Unlock()
        return
    }
    
    // 检查是否可以延长宽限期
    if mcs.graceExtensions < MaxGracePeriodExtensions {
        mcs.graceExtensions++
        mcs.graceTimer = time.AfterFunc(ReconnectGracePeriod, func() {
            mcs.onGraceTimeout(onExpired)
        })
        mcs.mu.Unlock()
        return
    }
    
    // 宽限期用尽，标记为断开
    mcs.state = types.ConnStateDisconnected
    mcs.graceTimer = nil
    mcs.mu.Unlock()
    
    // 执行移除回调
    if onExpired != nil {
        onExpired()
    }
}

// State 获取当前状态
func (mcs *MemberConnectionState) State() types.ConnState {
    mcs.mu.RLock()
    defer mcs.mu.RUnlock()
    return mcs.state
}
```

#### 任务 6.2: 震荡检测器

**文件**: `internal/realm/stability/tracker.go` (新建)

```go
package stability

import (
    "sync"
    "time"
    
    "github.com/libp2p/go-libp2p/core/peer"
)

// ConnectionStabilityTracker 连接稳定性跟踪器
type ConnectionStabilityTracker struct {
    mu sync.RWMutex
    
    // 每个节点的状态变化记录
    transitions map[peer.ID][]transitionRecord
    
    // 标记为震荡的节点
    flappingPeers map[peer.ID]time.Time
    
    config *StabilityConfig
}

type transitionRecord struct {
    timestamp  time.Time
    connected  bool  // true=连接, false=断开
}

// StabilityConfig 稳定性配置
type StabilityConfig struct {
    WindowDuration   time.Duration  // 检测窗口
    FlapThreshold    int            // 震荡阈值
    RecoveryDuration time.Duration  // 恢复时间
}

func DefaultStabilityConfig() *StabilityConfig {
    return &StabilityConfig{
        WindowDuration:   FlapWindowDuration,   // 60s
        FlapThreshold:    FlapThreshold,        // 3
        RecoveryDuration: FlappingRecovery,     // 5min
    }
}

func NewConnectionStabilityTracker(config *StabilityConfig) *ConnectionStabilityTracker {
    return &ConnectionStabilityTracker{
        transitions:   make(map[peer.ID][]transitionRecord),
        flappingPeers: make(map[peer.ID]time.Time),
        config:        config,
    }
}

// RecordTransition 记录状态转换
func (cst *ConnectionStabilityTracker) RecordTransition(peerID peer.ID, connected bool) {
    cst.mu.Lock()
    defer cst.mu.Unlock()
    
    record := transitionRecord{
        timestamp: time.Now(),
        connected: connected,
    }
    
    cst.transitions[peerID] = append(cst.transitions[peerID], record)
    cst.cleanOldTransitions(peerID)
    cst.checkFlapping(peerID)
}

// cleanOldTransitions 清理过期记录
func (cst *ConnectionStabilityTracker) cleanOldTransitions(peerID peer.ID) {
    cutoff := time.Now().Add(-cst.config.WindowDuration)
    records := cst.transitions[peerID]
    
    validRecords := make([]transitionRecord, 0)
    for _, r := range records {
        if r.timestamp.After(cutoff) {
            validRecords = append(validRecords, r)
        }
    }
    cst.transitions[peerID] = validRecords
}

// checkFlapping 检查是否触发震荡
func (cst *ConnectionStabilityTracker) checkFlapping(peerID peer.ID) {
    records := cst.transitions[peerID]
    
    // 统计断开/重连周期数
    cycles := 0
    for i := 1; i < len(records); i++ {
        if records[i-1].connected && !records[i].connected {
            cycles++  // 连接 → 断开
        }
    }
    
    if cycles >= cst.config.FlapThreshold {
        cst.flappingPeers[peerID] = time.Now()
    }
}

// IsFlapping 检查节点是否处于震荡状态
func (cst *ConnectionStabilityTracker) IsFlapping(peerID peer.ID) bool {
    cst.mu.RLock()
    defer cst.mu.RUnlock()
    
    flapTime, exists := cst.flappingPeers[peerID]
    if !exists {
        return false
    }
    
    // 检查是否已恢复
    if time.Since(flapTime) > cst.config.RecoveryDuration {
        delete(cst.flappingPeers, peerID)
        return false
    }
    
    return true
}

// ShouldSuppressStateChange 是否应该抑制状态变更
func (cst *ConnectionStabilityTracker) ShouldSuppressStateChange(peerID peer.ID) bool {
    return cst.IsFlapping(peerID)
}
```

#### 任务 6.3: 断开保护期

**文件**: `internal/realm/member/protection.go` (新建)

```go
package member

import (
    "sync"
    "time"
    
    "github.com/libp2p/go-libp2p/core/peer"
)

// DisconnectProtectionTracker 断开保护期跟踪
type DisconnectProtectionTracker struct {
    mu sync.RWMutex
    
    // 记录最近断开的成员
    recentlyDisconnected map[peer.ID]time.Time
    
    protectionDuration time.Duration
}

func NewDisconnectProtectionTracker() *DisconnectProtectionTracker {
    return &DisconnectProtectionTracker{
        recentlyDisconnected: make(map[peer.ID]time.Time),
        protectionDuration:   DisconnectProtection,  // 30s
    }
}

// OnMemberRemoved 成员被移除时调用
func (dpt *DisconnectProtectionTracker) OnMemberRemoved(peerID peer.ID) {
    dpt.mu.Lock()
    defer dpt.mu.Unlock()
    
    dpt.recentlyDisconnected[peerID] = time.Now()
}

// IsProtected 检查是否在保护期内
// 保护期内的成员不应被 PubSub/DHT 消息重新添加
func (dpt *DisconnectProtectionTracker) IsProtected(peerID peer.ID) bool {
    dpt.mu.RLock()
    defer dpt.mu.RUnlock()
    
    disconnectTime, exists := dpt.recentlyDisconnected[peerID]
    if !exists {
        return false
    }
    
    if time.Since(disconnectTime) > dpt.protectionDuration {
        // 保护期已过，清理记录
        delete(dpt.recentlyDisconnected, peerID)
        return false
    }
    
    return true
}

// Cleanup 清理过期记录
func (dpt *DisconnectProtectionTracker) Cleanup() {
    dpt.mu.Lock()
    defer dpt.mu.Unlock()
    
    cutoff := time.Now().Add(-dpt.protectionDuration)
    for peerID, t := range dpt.recentlyDisconnected {
        if t.Before(cutoff) {
            delete(dpt.recentlyDisconnected, peerID)
        }
    }
}
```

#### 任务 6.4: 见证人限速器

**文件**: `internal/realm/witness/ratelimit.go` (新建)

```go
package witness

import (
    "sync"
    "time"
)

// WitnessRateLimiter 见证报告限速器
type WitnessRateLimiter struct {
    mu sync.Mutex
    
    // 每个见证人的报告记录
    reports map[string][]time.Time
    
    limit  int           // 每分钟限制
    window time.Duration // 时间窗口
}

func NewWitnessRateLimiter() *WitnessRateLimiter {
    return &WitnessRateLimiter{
        reports: make(map[string][]time.Time),
        limit:   WitnessRateLimitPerMinute,  // 10
        window:  time.Minute,
    }
}

// AllowReport 检查是否允许发送报告
func (wrl *WitnessRateLimiter) AllowReport(witnessID string) bool {
    wrl.mu.Lock()
    defer wrl.mu.Unlock()
    
    now := time.Now()
    cutoff := now.Add(-wrl.window)
    
    // 清理过期记录
    records := wrl.reports[witnessID]
    validRecords := make([]time.Time, 0)
    for _, t := range records {
        if t.After(cutoff) {
            validRecords = append(validRecords, t)
        }
    }
    wrl.reports[witnessID] = validRecords
    
    // 检查是否超过限制
    if len(validRecords) >= wrl.limit {
        return false
    }
    
    // 记录本次报告
    wrl.reports[witnessID] = append(validRecords, now)
    return true
}
```

---

## 四、测试验证

### 4.1 单元测试

```bash
# Phase 1: QUIC
go test -v -run TestQUIC ./internal/core/transport/quic/

# Phase 2: MemberLeave
go test -v -run TestMemberLeave ./internal/realm/

# Phase 3: Witness
go test -v -run TestWitness ./internal/realm/witness/

# Phase 4: Relay Witness
go test -v -run TestRelayWitness ./internal/core/relay/server/

# Phase 5: Liveness
go test -v -run TestLiveness ./internal/core/swarm/

# Phase 6: 防误判机制
go test -v -run TestMemberState ./internal/realm/member/
go test -v -run TestStability ./internal/realm/stability/
go test -v -run TestRateLimit ./internal/realm/witness/
```

### 4.2 集成测试场景

| 场景 | 测试方法 | 期望结果 |
|------|---------|---------|
| 优雅断开 | `node.Stop()` | < 100ms 所有成员感知 |
| 直连非优雅断开 | `kill -9` | < 10s 直连节点感知 |
| 中继非优雅断开 | 断开 NAT 节点网络 | < 15s Relay 代理通知 |
| 网络分区 | 模拟对称分区 | 不误移除，进入分区模式 |
| Relay 批量异常 | 关闭 Relay | 抑制报告，不批量误删 |
| **4G/WiFi 切换** | 模拟网络切换（断开5s后重连） | 宽限期内重连成功，不移除成员 |
| **网络震荡** | 模拟快速断开/重连（60s内3次） | 触发震荡抑制，暂停状态变更 |
| **断开保护期** | 断开后立即通过 PubSub 重新添加 | 保护期内被拒绝 |
| **见证人限速** | 单见证人连续发送 15 个报告 | 超过 10 个后被限速 |

### 4.3 性能测试

```go
// 测试见证广播开销
func BenchmarkWitnessBroadcast(b *testing.B) {
    // 100 节点网络，1 节点断开
    // 期望：广播延迟 < 500ms，CPU 开销 < 1%
}

// 测试 QUIC Keep-Alive 开销
func BenchmarkQUICKeepAlive(b *testing.B) {
    // 100 连接，3s 间隔
    // 期望：带宽 < 1 kbps，CPU < 0.1%
}
```

---

## 五、实施顺序

| 优先级 | 任务 | 依赖 | 预计工作量 |
|:------:|------|------|-----------|
| P0.1 | QUIC Keep-Alive 配置 | 无 | 0.5d |
| P0.2 | 连接断开事件增强 | P0.1 | 0.5d |
| P0.3 | MemberLeave 协议 | 无 | 1d |
| P0.4 | MemberLeave 发送/接收 | P0.3 | 1d |
| P1.1 | Witness 服务骨架 | P0.2 | 1d |
| P1.2 | Witness 投票机制 | P1.1 | 1d |
| P1.3 | Relay 见证扩展 | P1.1 | 1d |
| **P1.4** | **成员状态机** | P0.2 | **1d** |
| **P1.5** | **震荡检测器** | P1.4 | **0.5d** |
| **P1.6** | **断开保护期** | P1.4 | **0.5d** |
| **P1.7** | **见证人限速器** | P1.1 | **0.5d** |
| P2.1 | Liveness 健康检查器 | P0.2 | 0.5d |
| P2.2 | 分区检测（可选） | P1.2 | 2d |

**总预计工作量**: 11-13 人日

---

## 六、风险与缓解

| 风险 | 影响 | 缓解措施 |
|------|------|---------|
| QUIC 3s/6s 过于激进 | 移动网络误断 | 重连宽限期 + 监控误断率 |
| 见证广播风暴 | 网络拥塞 | 延迟去重 + 批量抑制 + 限速 |
| 单见证人快速路径被滥用 | 恶意踢人 | 限制为高可信检测方式（仅 QUIC_CLOSE）|
| Relay 故障导致批量误判 | 大面积离线 | 批量异常抑制机制 |
| 网络震荡导致频繁状态变更 | 成员列表不稳定 | 震荡检测 + 抑制机制 |
| 断开后竞态重新添加 | 成员状态与连接不一致 | 断开保护期 30s |
| 宽限期滥用（永不移除） | 僵尸成员 | 最多延长 2 次 + 总上限 45s |

---

## 七、验收标准

| 标准 | 要求 |
|------|------|
| 功能完整 | 四层检测 + 防误判机制全部实现且可用 |
| 延迟达标 | 直连 < 10s，中继 < 15s |
| 误判可控 | 正常网络误断率 < 1%，分区场景 < 5% |
| 宽限期有效 | 4G/WiFi 切换（5s 断开）不触发成员移除 |
| 震荡抑制有效 | 60s 内 3 次断开/重连触发抑制 |
| 保护期有效 | 断开后 30s 内不被 PubSub 重新添加 |
| 限速有效 | 单见证人每分钟最多 10 个报告 |
| 测试覆盖 | 核心路径覆盖率 > 80% |
| 文档同步 | 行为文档与代码一致 |

---

## 八、设计文档一致性对照

| 设计文档条目 | 实施方案状态 |
|-------------|-------------|
| QUIC Keep-Alive 3s/6s | ✓ 一致 |
| MemberLeave 协议（含 KICKED 枚举） | ✓ 已修正 |
| WitnessReport 消息（含 last_contact_proof） | ✓ 已补充 |
| WitnessConfirmation 消息 | ✓ 一致 |
| 见证人快速路径（仅 QUIC_CLOSE） | ✓ 一致 |
| Quorum 投票规则 | ✓ 一致 |
| Relay 批量异常抑制 | ✓ 一致 |
| 重连宽限期 15s + 最多延长 2 次 | ✓ 已补充 |
| 断开保护期 30s | ✓ 已补充 |
| 震荡检测 60s/3 次 | ✓ 已补充 |
| 见证人限速 10/min | ✓ 已补充 |
| 协议 ID 使用 AppBuilder | ✓ 已修正 |
| Liveness 配置 30s/5s/3 次 | ✓ 已对齐 pkg |
| 成员状态机（DISCONNECTING 中间状态） | ✓ 已补充 |

---

## 九、与 pkg 目录一致性对照

> **重要**: 实施时必须与 `pkg/` 目录现有代码保持一致

| 现有代码 | 实施方案对照 | 状态 |
|---------|-------------|------|
| `pkg/protocol/app.go` - AppBuilder 模式 | 协议 ID 使用 `builder.Custom()` 动态构建 | ✓ 已对齐 |
| `pkg/lib/proto/realm/` 目录结构 | proto 文件放置在正确目录 | ✓ 已修正 |
| `pkg/types/events.go` - EvtPeerDisconnected | 扩展现有定义，新增 Reason 字段 | ✓ 已对齐 |
| `pkg/types/connection.go` - ConnState | 复用现有 ConnState 类型 | ✓ 已复用 |
| `pkg/interfaces/liveness.go` - LivenessConfig | 配置值与 DefaultLivenessConfig() 一致 | ✓ 已对齐 |
| `pkg/lib/proto/realm/member/` - MemberInfo | peer_id/realm_id 使用 bytes 类型 | ✓ 已对齐 |

### 需修改的现有文件

| 文件 | 修改内容 |
|------|---------|
| `pkg/types/events.go` | 扩展 EvtPeerDisconnected，新增 DisconnectReason 类型和 Reason 字段 |
| `pkg/protocol/app.go` | 可选：添加 `AppProtocolMemberLeave` 等常量 |
| `pkg/interfaces/liveness.go` | 可选：考虑是否需要调整 Timeout 为 10s（需评估影响） |

### 新增文件（遵循现有目录结构）

| 文件路径 | 说明 |
|---------|------|
| `pkg/lib/proto/realm/memberleave/member_leave.proto` | MemberLeave 消息定义 |
| `pkg/lib/proto/realm/memberleave/doc.go` | 包文档 |
| `pkg/lib/proto/realm/witness/witness.proto` | Witness 消息定义 |
| `pkg/lib/proto/realm/witness/doc.go` | 包文档 |

**参考文档**:
- `03_architecture/L3_behavioral/disconnect_detection.md` - 行为设计
- `02_constraints/protocol/L4_application/liveness.md` - 协议约束
- `01_context/requirements/functional/F3_network/REQ-NET-007.md` - 需求规范
- `01_context/decisions/ADR-0012-disconnect-detection.md` - 架构决策
- `01_context/decisions/invariants/INV-003-connection-membership.md` - 不变量

---

---

## 十、实施验证清单

> **验证日期**: 2026-01-28

### 10.1 已实现文件清单

| 组件 | 文件路径 | 行数 | 状态 |
|------|---------|------|------|
| 见证人服务 | `internal/realm/witness/service.go` | 773 | ✅ |
| 投票会话 | `internal/realm/witness/voting.go` | 220 | ✅ |
| 限速器 | `internal/realm/witness/ratelimit.go` | 148 | ✅ |
| 成员状态机 | `internal/realm/member/state.go` | 239 | ✅ |
| 震荡检测器 | `internal/realm/stability/tracker.go` | 263 | ✅ |
| 断开保护期 | `internal/realm/member/protection.go` | 175 | ✅ |
| 健康检查器 | `internal/core/swarm/health.go` | 331 | ✅ |
| 配置常量 | `internal/realm/config/disconnect.go` | 131 | ✅ |
| Relay 见证扩展 | `internal/core/relay/server/witness.go` | 333 | ✅ |
| Witness Proto | `pkg/lib/proto/realm/witness/witness.proto` | 172 | ✅ |
| MemberLeave Proto | `pkg/lib/proto/realm/memberleave/member_leave.proto` | 73 | ✅ |
| DisconnectReason | `pkg/types/events.go` | - | ✅ |
| ConnState | `pkg/types/connection.go` | - | ✅ |

### 10.2 功能验证对照

| 功能点 | 设计要求 | 实现状态 |
|-------|---------|---------|
| QUIC Keep-Alive 3s/6s | 快速检测直连断开 | ✅ 配置已定义 |
| MemberLeave 协议 | 优雅断开 < 100ms | ✅ Proto + 处理逻辑 |
| 见证人快速路径 | 小网络单票确认 | ✅ `canUseFastPath()` |
| 投票规则 | 简单多数 / 无反对 | ✅ `checkResult()` |
| 限速 10/min | 防 DoS | ✅ `AllowReport()` |
| 重连宽限期 15s | 防误判 | ✅ `OnDisconnect()` |
| 震荡检测 60s/3次 | 抑制抖动 | ✅ `RecordTransition()` |
| 断开保护期 30s | 防竞态重新添加 | ✅ `IsProtected()` |
| Relay 批量异常抑制 | 防 Relay 故障误报 | ✅ `isAnomalyDetected()` |
| 健康检查兜底 | 30s × 3 = 90s | ✅ `checkLoop()` |

### 10.3 待完成项

| 项目 | 说明 | 优先级 |
|------|------|--------|
| 单元测试补充 | 各组件的边界条件测试 | P2 |
| 集成测试 | 多节点场景验证 | P2 |
| 性能基准测试 | 见证广播开销、Keep-Alive 带宽 | P3 |

> **注**: 所有核心功能已实现，剩余工作为测试和性能验证。

---

**最后更新**: 2026-01-28（v1.2 - 实施验证完成）
