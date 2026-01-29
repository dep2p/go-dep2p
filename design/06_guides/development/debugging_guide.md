# 调试指南

> 日志、调试器、性能分析

---

## 1. 日志调试

### 1.1 日志级别

```go
// 设置日志级别
dep2p.WithLogLevel("debug")
```

| 级别 | 用途 |
|------|------|
| trace | 最详细，追踪执行 |
| debug | 调试信息 |
| info | 一般信息 |
| warn | 警告 |
| error | 错误 |

### 1.2 查看日志

```bash
# 运行时设置
DEP2P_LOG_LEVEL=debug ./dep2p

# 过滤特定模块
DEP2P_LOG_LEVEL=transport:debug,relay:info ./dep2p
```

---

## 2. Delve 调试器

### 2.1 安装

```bash
go install github.com/go-delve/delve/cmd/dlv@latest
```

### 2.2 基本使用

```bash
# 调试主程序
dlv debug ./cmd/dep2p

# 调试测试
dlv test ./internal/core/transport/

# 附加到进程
dlv attach <pid>
```

### 2.3 常用命令

| 命令 | 说明 |
|------|------|
| `break main.main` | 设置断点 |
| `break file.go:123` | 在行设置断点 |
| `continue` | 继续执行 |
| `next` | 下一行 |
| `step` | 进入函数 |
| `print var` | 打印变量 |
| `locals` | 打印本地变量 |
| `goroutines` | 列出 goroutine |
| `stack` | 打印调用栈 |

### 2.4 VS Code 集成

`.vscode/launch.json`:

```json
{
  "version": "0.2.0",
  "configurations": [
    {
      "name": "Debug",
      "type": "go",
      "request": "launch",
      "mode": "debug",
      "program": "${workspaceFolder}/cmd/dep2p"
    },
    {
      "name": "Test",
      "type": "go",
      "request": "launch",
      "mode": "test",
      "program": "${workspaceFolder}/internal/core/transport"
    }
  ]
}
```

---

## 3. 性能分析

### 3.1 CPU Profile

```go
import "runtime/pprof"

// 在代码中
f, _ := os.Create("cpu.prof")
pprof.StartCPUProfile(f)
defer pprof.StopCPUProfile()

// 或使用测试
go test -cpuprofile=cpu.prof ./...
```

分析：

```bash
go tool pprof cpu.prof

# Web 界面
go tool pprof -http=:8080 cpu.prof
```

### 3.2 内存 Profile

```go
import "runtime/pprof"

f, _ := os.Create("mem.prof")
pprof.WriteHeapProfile(f)
f.Close()
```

分析：

```bash
go tool pprof mem.prof
```

### 3.3 HTTP pprof

```go
import _ "net/http/pprof"

go func() {
    log.Println(http.ListenAndServe("localhost:6060", nil))
}()
```

访问：

- `http://localhost:6060/debug/pprof/`
- `http://localhost:6060/debug/pprof/goroutine`
- `http://localhost:6060/debug/pprof/heap`

### 3.4 Trace

```bash
go test -trace=trace.out ./...
go tool trace trace.out
```

---

## 4. 竞态检测

```bash
# 运行时检测
go run -race ./cmd/dep2p

# 测试时检测
go test -race ./...
```

---

## 5. 常见问题调试

### 5.1 Goroutine 泄漏

```go
import "runtime"

// 打印 goroutine 数量
fmt.Println("Goroutines:", runtime.NumGoroutine())

// 打印所有 goroutine 栈
buf := make([]byte, 1<<20)
n := runtime.Stack(buf, true)
fmt.Println(string(buf[:n]))
```

### 5.2 内存泄漏

```go
import "runtime"

var m runtime.MemStats
runtime.ReadMemStats(&m)

fmt.Printf("Alloc = %v MiB\n", m.Alloc/1024/1024)
fmt.Printf("Sys = %v MiB\n", m.Sys/1024/1024)
fmt.Printf("NumGC = %v\n", m.NumGC)
```

### 5.3 死锁检测

```bash
# 设置死锁检测
GODEBUG=schedtrace=1000,scheddetail=1 ./dep2p
```

---

## 6. 网络调试

### 6.1 抓包

```bash
# 使用 tcpdump
sudo tcpdump -i any port 8080 -w capture.pcap

# 使用 Wireshark 分析
wireshark capture.pcap
```

### 6.2 连接追踪

```go
// 添加连接日志
transport.WithConnLogger(func(event string, conn Conn) {
    log.Debug("connection event",
        "event", event,
        "remote", conn.RemoteAddr(),
    )
})
```

---

## 7. 测试调试

### 7.1 详细输出

```bash
go test -v ./...
```

### 7.2 运行特定测试

```bash
go test -run TestSpecific ./...
```

### 7.3 失败时停止

```bash
go test -failfast ./...
```

### 7.4 保留临时文件

```bash
go test -work ./...
```

---

## 8. 调试技巧

### 8.1 添加调试日志

```go
// 临时调试日志
log.Debug("DEBUG: entering function",
    "param1", param1,
    "param2", param2,
)
defer log.Debug("DEBUG: exiting function")
```

### 8.2 条件断点

在 Delve 中：

```
break main.go:123 if x > 10
```

### 8.3 打印调用栈

```go
import "runtime/debug"

debug.PrintStack()
```

---

## 9. P2P 日志分析

DeP2P 提供了完整的日志分析框架，涵盖 8 个核心分析维度，帮助开发者和运维人员识别网络性能瓶颈、评估 NAT 穿透效果、监控消息传递质量。

### 9.1 分析维度总览

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                      P2P 日志分析框架                                         │
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

### 9.2 各维度关键指标

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

### 9.3 日志关键词参考

```bash
# === 连接质量分析 ===
# 连接成功/失败
grep -E "连接节点成功|连接节点失败|dial.*failed|connection.*established" $LOG

# RTT 延迟
grep -E "rtt=|latency=|ping.*ms" $LOG

# 断线事件
grep -E "断开连接|connection.*closed|检测到连接断开" $LOG

# === NAT 穿透分析 ===
# 连接类型
grep -E "connType=direct|connType=relay|connType=holepunch" $LOG

# 打洞事件
grep -E "打洞.*成功|打洞.*失败|hole.*punch|DCUtR" $LOG

# NAT 类型探测
grep -E "NAT.*type|NAT.*detected|STUN.*response" $LOG

# === 消息传递分析 ===
# 消息发送/接收
grep -E "发布消息|消息发送|Publish|message.*sent" $LOG
grep -E "收到消息|消息接收|Received|message.*received" $LOG

# 消息重复
grep -E "重复消息|duplicate|跳过.*已处理" $LOG

# === 断开检测分析 ===
# 见证人报告
grep -E "WitnessReport|见证.*报告|witness.*quorum" $LOG

# 重连宽限期
grep -E "grace.*period|宽限期|重连.*成功" $LOG

# MemberLeave 通知
grep -E "MemberLeave|成员离开通知" $LOG

# === 节点发现分析 ===
# 各发现来源
grep -E "mDNS.*发现|SourceMDNS" $LOG
grep -E "DHT.*FindPeers|SourceDHT" $LOG
grep -E "Bootstrap.*连接|SourceBootstrap" $LOG

# === 故障恢复分析 ===
# 重连事件
grep -E "重连|reconnect|retry.*connect" $LOG

# 恢复事件
grep -E "恢复.*连接|recovered|reconnected" $LOG
```

### 9.4 快速分析脚本

将以下脚本保存为 `p2p-log-analyze.sh`：

```bash
#!/bin/bash
# p2p-log-analyze.sh - P2P 日志快速分析脚本
# 用法: ./p2p-log-analyze.sh <log_file>

LOG_FILE=$1

if [ -z "$LOG_FILE" ]; then
    echo "用法: $0 <log_file>"
    exit 1
fi

echo "======================================"
echo "       P2P 日志分析报告"
echo "======================================"
echo "日志文件: $LOG_FILE"
echo "分析时间: $(date)"
echo ""

# 1. 基本统计
echo "=== 1. 基本统计 ==="
TOTAL_LINES=$(wc -l < "$LOG_FILE")
ERROR_COUNT=$(grep -c "level=ERROR" "$LOG_FILE" 2>/dev/null || echo 0)
WARN_COUNT=$(grep -c "level=WARN" "$LOG_FILE" 2>/dev/null || echo 0)
echo "总行数: $TOTAL_LINES"
echo "ERROR: $ERROR_COUNT"
echo "WARN:  $WARN_COUNT"
echo ""

# 2. 连接质量
echo "=== 2. 连接质量 ==="
CONN_SUCCESS=$(grep -c "连接节点成功" "$LOG_FILE" 2>/dev/null || echo 0)
CONN_FAIL=$(grep -c "连接节点失败\|dial.*failed\|拨号失败" "$LOG_FILE" 2>/dev/null || echo 0)
CONN_TOTAL=$((CONN_SUCCESS + CONN_FAIL))
if [ $CONN_TOTAL -gt 0 ]; then
    CONN_RATE=$(echo "scale=2; $CONN_SUCCESS * 100 / $CONN_TOTAL" | bc)
    echo "连接成功率: $CONN_RATE% ($CONN_SUCCESS/$CONN_TOTAL)"
else
    echo "连接成功率: N/A (无连接数据)"
fi

# RTT 统计
echo ""
echo "RTT 统计:"
grep -oP 'rtt=\K[0-9.]+' "$LOG_FILE" 2>/dev/null | \
    awk 'BEGIN {min=999999; max=0} 
         {sum+=$1; count++; if($1<min)min=$1; if($1>max)max=$1} 
         END {if(count>0) printf "  平均: %.2fms, 最小: %.2fms, 最大: %.2fms (样本: %d)\n", sum/count, min, max, count
              else print "  无 RTT 数据"}'
echo ""

# 3. 断线统计
echo "=== 3. 断线与检测统计 ==="
DISCONNECT_COUNT=$(grep -c "断开连接\|connection.*closed\|检测到连接断开" "$LOG_FILE" 2>/dev/null || echo 0)
WITNESS_REPORTS=$(grep -c "WitnessReport\|见证.*报告" "$LOG_FILE" 2>/dev/null || echo 0)
MEMBER_LEAVE=$(grep -c "MemberLeave\|成员离开" "$LOG_FILE" 2>/dev/null || echo 0)
echo "断线次数: $DISCONNECT_COUNT"
echo "见证人报告: $WITNESS_REPORTS"
echo "MemberLeave 通知: $MEMBER_LEAVE"
echo ""

# 4. 连接类型分布
echo "=== 4. 连接类型分布 ==="
DIRECT=$(grep -c "connType=direct" "$LOG_FILE" 2>/dev/null || echo 0)
RELAY=$(grep -c "connType=relay" "$LOG_FILE" 2>/dev/null || echo 0)
HOLEPUNCH=$(grep -c "connType=holepunch" "$LOG_FILE" 2>/dev/null || echo 0)
echo "直连: $DIRECT"
echo "中继: $RELAY"
echo "打洞: $HOLEPUNCH"
echo ""

# 5. 消息统计
echo "=== 5. 消息统计 ==="
MSG_SENT=$(grep -c "发布消息\|消息发送" "$LOG_FILE" 2>/dev/null || echo 0)
MSG_RECV=$(grep -c "收到.*消息\|投递消息" "$LOG_FILE" 2>/dev/null || echo 0)
echo "发送消息: $MSG_SENT"
echo "接收消息: $MSG_RECV"
echo ""

# 6. TOP 5 错误
echo "=== 6. TOP 5 错误类型 ==="
grep "level=ERROR\|level=WARN" "$LOG_FILE" 2>/dev/null | \
    sed 's/.*msg="\([^"]*\)".*/\1/' | \
    sort | uniq -c | sort -rn | head -5
echo ""

echo "======================================"
echo "         分析完成"
echo "======================================"
```

### 9.5 实时监控脚本

将以下脚本保存为 `p2p-log-monitor.sh`：

```bash
#!/bin/bash
# p2p-log-monitor.sh - P2P 日志实时监控
# 用法: ./p2p-log-monitor.sh <log_file>

LOG_FILE=$1

if [ -z "$LOG_FILE" ]; then
    echo "用法: $0 <log_file>"
    exit 1
fi

echo "监控 P2P 日志: $LOG_FILE"
echo "按 Ctrl+C 退出"
echo ""

tail -f "$LOG_FILE" | while read line; do
    # 高亮 ERROR
    if echo "$line" | grep -q "level=ERROR"; then
        echo -e "\033[31m[ERROR]\033[0m $line"
    # 高亮 WARN
    elif echo "$line" | grep -q "level=WARN"; then
        echo -e "\033[33m[WARN]\033[0m $line"
    # 高亮连接事件
    elif echo "$line" | grep -q "连接节点成功"; then
        echo -e "\033[32m[CONN+]\033[0m $line"
    elif echo "$line" | grep -q "断开连接\|连接节点失败"; then
        echo -e "\033[35m[CONN-]\033[0m $line"
    # 高亮断开检测
    elif echo "$line" | grep -q "WitnessReport\|MemberLeave\|检测到"; then
        echo -e "\033[36m[DETECT]\033[0m $line"
    # 高亮消息
    elif echo "$line" | grep -q "收到.*消息\|发布消息"; then
        echo -e "\033[36m[MSG]\033[0m $line"
    fi
done
```

### 9.6 断开检测专项分析

针对快速断开检测机制，重点关注以下日志：

```bash
# 分析断开检测层次
echo "=== QUIC Keep-Alive 层 ==="
grep -E "MaxIdleTimeout|KeepAlivePeriod|idle.*timeout" $LOG

echo "=== MemberLeave 通知层 ==="
grep -E "MemberLeave|优雅离开|graceful.*leave" $LOG

echo "=== 见证人网络层 ==="
grep -E "WitnessReport|witness.*quorum|见证确认" $LOG

echo "=== Liveness Ping 层 ==="
grep -E "Liveness.*Ping|探活失败|heartbeat.*timeout" $LOG

echo "=== 重连宽限期 ==="
grep -E "grace.*period|宽限期.*开始|重连.*成功" $LOG

echo "=== 震荡检测 ==="
grep -E "flapping|震荡检测|Debounce|抖动抑制" $LOG
```

### 9.7 周期性指标快照

DeP2P 会每 30 秒输出一次指标快照，用于趋势分析：

```
time=2026-01-28T21:00:00+08:00 level=INFO msg="P2P 指标快照" 
    uptime=3600 
    connectedPeers=15 
    directConns=12 
    relayConns=3 
    avgRTT=25.5 
    goroutines=127 
    heapAllocMB=45.2
```

分析指标快照：

```bash
# 提取连接趋势
grep "P2P 指标快照" $LOG | \
    awk -F'connectedPeers=' '{print $2}' | \
    awk '{print $1}'

# 检查内存泄漏
grep "P2P 指标快照" $LOG | \
    awk -F'heapAllocMB=' '{print $2}' | \
    awk '{print $1}'
```

---

**最后更新**：2026-01-29
