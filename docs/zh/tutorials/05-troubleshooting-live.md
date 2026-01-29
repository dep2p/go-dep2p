# 实战故障排查：日志分析框架

本教程将指导你使用 DeP2P 的日志分析框架排查常见问题，包括连接失败、消息丢失、NAT 穿透问题等。

---

## 教程目标

完成本教程后，你将学会：

- 使用 8 维度日志分析框架
- 排查连接失败问题
- 诊断消息丢失原因
- 分析 NAT 穿透效果
- 使用分析脚本快速定位问题

---

## 日志分析框架概述

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                      P2P 日志分析框架 - 8 个维度                             │
├─────────────────────────────────────────────────────────────────────────────┤
│  ┌───────────────┐  ┌───────────────┐  ┌───────────────┐  ┌──────────────┐ │
│  │ 1. 连接质量    │  │ 2. NAT 穿透   │  │ 3. 消息传递    │  │ 4. 节点发现  │ │
│  │ - RTT 延迟    │  │ - 穿透成功率   │  │ - E2E 延迟    │  │ - 发现时间   │ │
│  │ - 连接成功率   │  │ - NAT 类型    │  │ - 消息丢失率   │  │ - 来源分布   │ │
│  └───────────────┘  └───────────────┘  └───────────────┘  └──────────────┘ │
│  ┌───────────────┐  ┌───────────────┐  ┌───────────────┐  ┌──────────────┐ │
│  │ 5. 资源效率    │  │ 6. 故障恢复    │  │ 7. 安全检测    │  │ 8. 地理分布  │ │
│  │ - 带宽使用    │  │ - 重连时间    │  │ - 异常检测    │  │ - 跨区域延迟 │ │
│  │ - CPU/内存    │  │ - 恢复成功率   │  │ - 攻击识别    │  │ - IPv4/v6    │ │
│  └───────────────┘  └───────────────┘  └───────────────┘  └──────────────┘ │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 健康指标参考

| 维度 | 关键指标 | 健康阈值 | 告警阈值 |
|------|---------|---------|---------|
| **连接质量** | 连接成功率 | ≥ 80% | < 50% |
| | 平均 RTT | < 100ms | > 500ms |
| | 断线率 | < 0.1/min | > 1/min |
| **NAT 穿透** | 直连成功率 | ≥ 60% | < 30% |
| | 打洞成功率 | ≥ 50% | < 20% |
| | 中继使用率 | < 20% | > 50% |
| **消息传递** | 端到端延迟 | < 500ms | > 2s |
| | 消息丢失率 | < 1% | > 5% |
| **断开检测** | 检测延迟 | < 6s | > 15s |
| | 见证确认率 | ≥ 50% | < 30% |

---

## 启用详细日志

### 设置日志级别

```go
import "github.com/dep2p/go-dep2p/pkg/lib/log"

// 启用 Debug 级别日志
log.SetLevel("dep2p", log.LevelDebug)

// 或通过环境变量
// export DEP2P_LOG_LEVEL=debug
```

### 日志输出到文件

```go
// 配置日志输出
log.SetOutput(os.Stdout) // 或文件

// 命令行重定向
// go run main.go 2>&1 | tee dep2p.log
```

---

## 场景 1：连接失败排查

### 问题描述

节点 A 无法连接到节点 B，报错 `dial failed` 或超时。

### 排查步骤

#### Step 1: 检查连接日志

```bash
# 查看连接尝试和失败
grep -E "连接节点|dial|connection" dep2p.log | tail -50

# 常见日志模式
# 成功: 连接节点成功 peerID=12D3KooW... connType=direct
# 失败: 连接节点失败 peerID=12D3KooW... error=dial backoff
```

#### Step 2: 分析失败原因

```bash
# 统计连接成功率
echo "=== 连接统计 ==="
echo -n "成功: "; grep -c "连接节点成功" dep2p.log
echo -n "失败: "; grep -c "连接节点失败\|dial.*failed" dep2p.log

# 常见错误类型
grep "dial.*failed" dep2p.log | awk -F'error=' '{print $2}' | sort | uniq -c | sort -rn
```

#### Step 3: 检查目标节点状态

```bash
# 检查目标节点是否被发现
grep "12D3KooWxxxxx" dep2p.log  # 替换为目标 PeerID

# 检查是否有地址
grep -E "addrs.*12D3KooWxxxxx|peerstore.*added" dep2p.log
```

#### 常见原因和解决方案

| 错误 | 原因 | 解决方案 |
|------|------|----------|
| `dial backoff` | 连接频率过高触发退避 | 等待退避时间结束，或检查连接逻辑 |
| `no addresses` | 未发现目标地址 | 检查 mDNS/DHT/Bootstrap，或使用 known_peers |
| `connection refused` | 目标端口未监听 | 确认目标节点运行并监听正确端口 |
| `timeout` | 网络不通或防火墙 | 检查网络连通性和防火墙规则 |
| `peer id mismatch` | PeerID 与地址不匹配 | 确认 known_peers 配置正确 |

### 快速诊断脚本

```bash
#!/bin/bash
# check-connection.sh - 连接问题快速诊断

LOG=${1:-dep2p.log}
PEER=${2:-""}

echo "=== 连接问题诊断 ==="
echo "日志文件: $LOG"
echo

# 1. 连接统计
echo "【1. 连接统计】"
TOTAL_SUCCESS=$(grep -c "连接节点成功" "$LOG" 2>/dev/null || echo 0)
TOTAL_FAIL=$(grep -c "连接节点失败\|dial.*failed" "$LOG" 2>/dev/null || echo 0)
TOTAL=$((TOTAL_SUCCESS + TOTAL_FAIL))
if [ $TOTAL -gt 0 ]; then
    RATE=$((TOTAL_SUCCESS * 100 / TOTAL))
    echo "  成功: $TOTAL_SUCCESS, 失败: $TOTAL_FAIL, 成功率: $RATE%"
else
    echo "  无连接记录"
fi
echo

# 2. 错误分布
echo "【2. 错误类型分布】"
grep "dial.*failed\|连接.*失败" "$LOG" 2>/dev/null | \
    sed -n 's/.*error=\([^,}]*\).*/\1/p' | \
    sort | uniq -c | sort -rn | head -5
echo

# 3. 特定节点检查
if [ -n "$PEER" ]; then
    echo "【3. 节点 $PEER 连接记录】"
    grep "$PEER" "$LOG" | grep -E "连接|dial|connect" | tail -10
fi
```

---

## 场景 2：消息丢失排查

### 问题描述

发送的消息对方没有收到，或收到延迟很高。

### 排查步骤

#### Step 1: 追踪消息流

```bash
# 发送端日志
grep -E "发布消息|Publish|message.*sent" dep2p_sender.log

# 接收端日志
grep -E "收到消息|Received|message.*received" dep2p_receiver.log

# 对比时间戳计算延迟
```

#### Step 2: 检查 PubSub 状态

```bash
# 检查话题订阅
grep -E "Subscribe|订阅话题|topic.*joined" dep2p.log

# 检查消息传播
grep -E "gossip|mesh|prune|graft" dep2p.log | tail -20
```

#### Step 3: 检查重复消息

```bash
# 重复消息会被丢弃
grep -E "重复消息|duplicate|already.*seen" dep2p.log

# 如果大量重复，检查发送逻辑
```

#### 常见原因和解决方案

| 现象 | 原因 | 解决方案 |
|------|------|----------|
| 完全收不到 | 话题不匹配 | 确认发送/接收使用相同话题名 |
| 完全收不到 | 不在同一 Realm | 确认双方在同一 Realm |
| 延迟高 | Mesh 尚未建立 | 等待 PubSub 路由建立（~5s） |
| 部分丢失 | 网络抖动 | 检查连接稳定性 |
| 大量重复 | 发送端重复发送 | 检查发送逻辑，添加去重 |

### 消息追踪脚本

```bash
#!/bin/bash
# trace-message.sh - 消息追踪

LOG=${1:-dep2p.log}
MSG_ID=${2:-""}

echo "=== 消息追踪 ==="

# 统计
echo "【消息统计】"
SENT=$(grep -c "发布消息\|Publish" "$LOG" 2>/dev/null || echo 0)
RECV=$(grep -c "收到消息\|Received" "$LOG" 2>/dev/null || echo 0)
DUP=$(grep -c "重复消息\|duplicate" "$LOG" 2>/dev/null || echo 0)
echo "  发送: $SENT, 接收: $RECV, 重复: $DUP"
echo

# 延迟分析（如果日志包含时间戳）
echo "【最近消息】"
grep -E "发布消息|收到消息" "$LOG" | tail -10
```

---

## 场景 3：NAT 穿透问题

### 问题描述

两个 NAT 后的节点无法直连，只能通过中继通信。

### 排查步骤

#### Step 1: 检查连接类型

```bash
# 查看连接类型分布
echo "=== 连接类型分布 ==="
grep "connType=" dep2p.log | \
    sed -n 's/.*connType=\([a-z]*\).*/\1/p' | \
    sort | uniq -c | sort -rn

# 预期输出:
#    45 direct      # 直连
#    12 holepunch   # 打洞
#     3 relay       # 中继
```

#### Step 2: 检查 NAT 类型

```bash
# NAT 类型探测结果
grep -E "NAT.*type|NAT.*detected|STUN" dep2p.log

# 查看探测的地址
grep -E "observed.*addr|external.*addr|STUN.*address" dep2p.log
```

#### Step 3: 检查打洞过程

```bash
# 打洞尝试
grep -E "打洞|hole.*punch|DCUtR" dep2p.log

# 成功/失败统计
echo -n "打洞成功: "; grep -c "打洞.*成功\|holepunch.*success" dep2p.log
echo -n "打洞失败: "; grep -c "打洞.*失败\|holepunch.*failed" dep2p.log
```

#### NAT 类型对照表

| NAT 类型 | 直连可能性 | 打洞成功率 | 建议 |
|----------|-----------|-----------|------|
| Full Cone | 高 | 90%+ | 最理想 |
| Restricted Cone | 中 | 70%+ | 可打洞 |
| Port Restricted | 低 | 40%+ | 需要中继备用 |
| Symmetric | 极低 | <10% | 依赖中继 |

### NAT 分析脚本

```bash
#!/bin/bash
# analyze-nat.sh - NAT 穿透分析

LOG=${1:-dep2p.log}

echo "=== NAT 穿透分析 ==="
echo

# 1. 连接类型统计
echo "【连接类型分布】"
grep "connType=" "$LOG" | \
    sed -n 's/.*connType=\([a-z]*\).*/\1/p' | \
    sort | uniq -c | sort -rn
echo

# 2. 计算直连率
DIRECT=$(grep -c "connType=direct" "$LOG" 2>/dev/null || echo 0)
RELAY=$(grep -c "connType=relay" "$LOG" 2>/dev/null || echo 0)
HOLEPUNCH=$(grep -c "connType=holepunch" "$LOG" 2>/dev/null || echo 0)
TOTAL=$((DIRECT + RELAY + HOLEPUNCH))
if [ $TOTAL -gt 0 ]; then
    DIRECT_RATE=$((DIRECT * 100 / TOTAL))
    RELAY_RATE=$((RELAY * 100 / TOTAL))
    echo "【穿透效果】"
    echo "  直连率: $DIRECT_RATE%"
    echo "  中继率: $RELAY_RATE%"
    
    if [ $RELAY_RATE -gt 50 ]; then
        echo "  ⚠️  警告: 中继使用率过高，检查 NAT 配置"
    fi
fi
echo

# 3. 打洞统计
echo "【打洞统计】"
HP_SUCCESS=$(grep -c "打洞.*成功\|holepunch.*success" "$LOG" 2>/dev/null || echo 0)
HP_FAIL=$(grep -c "打洞.*失败\|holepunch.*failed" "$LOG" 2>/dev/null || echo 0)
echo "  成功: $HP_SUCCESS, 失败: $HP_FAIL"
```

---

## 场景 4：断开检测延迟

### 问题描述

节点断开后，其他节点收到 MemberLeft 事件延迟很高。

### 排查步骤

#### Step 1: 检查断开检测日志

```bash
# 连接断开检测
grep -E "检测到连接断开|connection.*closed|idle.*timeout" dep2p.log

# MemberLeft 事件
grep -E "MemberLeft|成员离开" dep2p.log

# 对比时间戳计算检测延迟
```

#### Step 2: 检查见证人机制

```bash
# 见证人报告
grep -E "WitnessReport|见证.*报告" dep2p.log

# 见证人确认
grep -E "witness.*quorum|见证确认" dep2p.log
```

#### Step 3: 检查重连宽限期

```bash
# 宽限期内的重连尝试
grep -E "grace.*period|宽限期|重连" dep2p.log
```

#### 调优建议

```json
{
  "disconnect_detection": {
    "quic": {
      "keep_alive_period": "3s",    // 减小：更快检测
      "max_idle_timeout": "6s"       // 减小：更快超时
    },
    "reconnect_grace_period": "10s", // 减小：更快触发 MemberLeft
    "witness": {
      "enabled": true,
      "count": 3,
      "quorum": 2,
      "timeout": "3s"                // 减小：更快见证确认
    }
  }
}
```

---

## 综合分析脚本

将以下脚本保存为 `p2p-log-analyze.sh`：

```bash
#!/bin/bash
# p2p-log-analyze.sh - P2P 日志综合分析脚本
#
# 用法: ./p2p-log-analyze.sh [日志文件] [分析维度]
# 维度: all, connection, nat, message, discovery, disconnect

LOG=${1:-dep2p.log}
MODE=${2:-all}

echo "╔════════════════════════════════════════════════════════════════╗"
echo "║              P2P 日志分析报告                                   ║"
echo "╠════════════════════════════════════════════════════════════════╣"
echo "║  日志文件: $LOG"
echo "║  分析模式: $MODE"
echo "║  生成时间: $(date '+%Y-%m-%d %H:%M:%S')"
echo "╚════════════════════════════════════════════════════════════════╝"
echo

# 检查日志文件
if [ ! -f "$LOG" ]; then
    echo "错误: 日志文件不存在: $LOG"
    exit 1
fi

# 连接质量分析
analyze_connection() {
    echo "━━━ 1. 连接质量分析 ━━━"
    echo
    
    # 连接统计
    SUCCESS=$(grep -c "连接节点成功\|connection.*established" "$LOG" 2>/dev/null || echo 0)
    FAIL=$(grep -c "连接节点失败\|dial.*failed" "$LOG" 2>/dev/null || echo 0)
    TOTAL=$((SUCCESS + FAIL))
    
    echo "【连接统计】"
    echo "  成功: $SUCCESS"
    echo "  失败: $FAIL"
    if [ $TOTAL -gt 0 ]; then
        RATE=$((SUCCESS * 100 / TOTAL))
        echo "  成功率: $RATE%"
        if [ $RATE -lt 50 ]; then
            echo "  ⚠️  警告: 连接成功率低于 50%"
        fi
    fi
    echo
    
    # 错误类型
    echo "【错误类型 Top 5】"
    grep "dial.*failed\|连接.*失败" "$LOG" 2>/dev/null | \
        sed -n 's/.*error=\([^,}]*\).*/\1/p' | \
        sort | uniq -c | sort -rn | head -5
    echo
}

# NAT 穿透分析
analyze_nat() {
    echo "━━━ 2. NAT 穿透分析 ━━━"
    echo
    
    # 连接类型
    echo "【连接类型分布】"
    DIRECT=$(grep -c "connType=direct" "$LOG" 2>/dev/null || echo 0)
    RELAY=$(grep -c "connType=relay" "$LOG" 2>/dev/null || echo 0)
    HOLEPUNCH=$(grep -c "connType=holepunch" "$LOG" 2>/dev/null || echo 0)
    
    echo "  直连: $DIRECT"
    echo "  打洞: $HOLEPUNCH"
    echo "  中继: $RELAY"
    
    TOTAL=$((DIRECT + RELAY + HOLEPUNCH))
    if [ $TOTAL -gt 0 ]; then
        DIRECT_RATE=$((DIRECT * 100 / TOTAL))
        RELAY_RATE=$((RELAY * 100 / TOTAL))
        echo "  直连率: $DIRECT_RATE%"
        if [ $RELAY_RATE -gt 50 ]; then
            echo "  ⚠️  警告: 中继使用率 $RELAY_RATE% 过高"
        fi
    fi
    echo
}

# 消息传递分析
analyze_message() {
    echo "━━━ 3. 消息传递分析 ━━━"
    echo
    
    SENT=$(grep -c "发布消息\|Publish\|message.*sent" "$LOG" 2>/dev/null || echo 0)
    RECV=$(grep -c "收到消息\|Received\|message.*received" "$LOG" 2>/dev/null || echo 0)
    DUP=$(grep -c "重复消息\|duplicate" "$LOG" 2>/dev/null || echo 0)
    
    echo "【消息统计】"
    echo "  发送: $SENT"
    echo "  接收: $RECV"
    echo "  重复: $DUP"
    echo
}

# 节点发现分析
analyze_discovery() {
    echo "━━━ 4. 节点发现分析 ━━━"
    echo
    
    MDNS=$(grep -c "mDNS.*发现\|SourceMDNS" "$LOG" 2>/dev/null || echo 0)
    DHT=$(grep -c "DHT.*Find\|SourceDHT" "$LOG" 2>/dev/null || echo 0)
    BOOTSTRAP=$(grep -c "Bootstrap.*连接\|SourceBootstrap" "$LOG" 2>/dev/null || echo 0)
    KNOWN=$(grep -c "known.*peer\|KnownPeers" "$LOG" 2>/dev/null || echo 0)
    
    echo "【发现来源分布】"
    echo "  mDNS: $MDNS"
    echo "  DHT: $DHT"
    echo "  Bootstrap: $BOOTSTRAP"
    echo "  KnownPeers: $KNOWN"
    echo
}

# 断开检测分析
analyze_disconnect() {
    echo "━━━ 5. 断开检测分析 ━━━"
    echo
    
    DISCONNECT=$(grep -c "检测到连接断开\|connection.*closed" "$LOG" 2>/dev/null || echo 0)
    MEMBER_LEFT=$(grep -c "MemberLeft\|成员离开" "$LOG" 2>/dev/null || echo 0)
    WITNESS=$(grep -c "WitnessReport\|见证" "$LOG" 2>/dev/null || echo 0)
    RECONNECT=$(grep -c "重连.*成功\|reconnect.*success" "$LOG" 2>/dev/null || echo 0)
    
    echo "【断开检测统计】"
    echo "  断开检测: $DISCONNECT"
    echo "  MemberLeft: $MEMBER_LEFT"
    echo "  见证报告: $WITNESS"
    echo "  重连成功: $RECONNECT"
    echo
}

# 执行分析
case $MODE in
    connection)
        analyze_connection
        ;;
    nat)
        analyze_nat
        ;;
    message)
        analyze_message
        ;;
    discovery)
        analyze_discovery
        ;;
    disconnect)
        analyze_disconnect
        ;;
    all|*)
        analyze_connection
        analyze_nat
        analyze_message
        analyze_discovery
        analyze_disconnect
        ;;
esac

echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "分析完成"
```

### 使用方法

```bash
# 赋予执行权限
chmod +x p2p-log-analyze.sh

# 全面分析
./p2p-log-analyze.sh dep2p.log

# 仅分析连接质量
./p2p-log-analyze.sh dep2p.log connection

# 仅分析 NAT 穿透
./p2p-log-analyze.sh dep2p.log nat
```

---

## 实时监控脚本

```bash
#!/bin/bash
# p2p-log-monitor.sh - 实时日志监控

LOG=${1:-dep2p.log}

echo "实时监控 P2P 日志: $LOG"
echo "按 Ctrl+C 停止"
echo "═════════════════════════════════════════════"

tail -f "$LOG" | while read line; do
    # 高亮重要事件
    if echo "$line" | grep -qE "连接节点成功|MemberJoined"; then
        echo -e "\033[32m$line\033[0m"  # 绿色
    elif echo "$line" | grep -qE "连接节点失败|MemberLeft|error"; then
        echo -e "\033[31m$line\033[0m"  # 红色
    elif echo "$line" | grep -qE "打洞|holepunch|WitnessReport"; then
        echo -e "\033[33m$line\033[0m"  # 黄色
    elif echo "$line" | grep -qE "收到消息|Received"; then
        echo -e "\033[36m$line\033[0m"  # 青色
    fi
done
```

---

## 总结

| 问题类型 | 关键日志 | 首先检查 |
|----------|---------|----------|
| 连接失败 | `dial failed`, `connection refused` | 网络连通性、防火墙、地址发现 |
| 消息丢失 | `Publish`, `Received`, `duplicate` | Realm 匹配、话题名、PubSub 路由 |
| NAT 问题 | `connType=`, `holepunch` | NAT 类型、中继可用性 |
| 断开延迟 | `MemberLeft`, `WitnessReport` | 断开检测配置、宽限期设置 |

---

## 下一步

- [配置参考](../reference/configuration.md) - 完整配置选项说明
- [NAT 穿透](../how-to/nat-traversal.md) - NAT 穿透详细指南
- [可观测性](../how-to/observability.md) - Prometheus 指标和监控
