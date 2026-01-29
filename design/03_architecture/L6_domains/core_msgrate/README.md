# core_msgrate 模块设计

> **版本**: v1.0.0  
> **更新日期**: 2026-01-18  
> **架构层**: Core Layer  
> **代码位置**: `internal/core/msgrate/`

---

## 概述

MsgRate 模块提供消息速率跟踪器，用于动态测量和估计节点的消息处理能力，优化请求大小和超时设置。

## 核心功能

1. **吞吐量追踪** - 记录每种消息类型的处理速率
2. **RTT 估计** - 估计节点往返时间
3. **容量计算** - 计算在目标 RTT 内可处理的消息数量
4. **全局调优** - 基于多节点数据计算全局目标 RTT

## 核心接口

### Tracker（单节点追踪器）

```go
type Tracker interface {
    Capacity(kind uint64, targetRTT time.Duration) int
    Update(kind uint64, elapsed time.Duration, items int)
    RTT() time.Duration
}
```

### Trackers（追踪器集合）

```go
type Trackers interface {
    Track(id string, tracker Tracker) error
    Untrack(id string) error
    TargetRoundTrip() time.Duration
    TargetTimeout() time.Duration
    MedianRoundTrip() time.Duration
    MeanCapacities() map[uint64]float64
    Capacity(id string, kind uint64, targetRTT time.Duration) int
    Update(id string, kind uint64, elapsed time.Duration, items int)
}
```

## 配置

```go
type Config struct {
    CapacityOverestimation float64       // 容量过估计因子，默认 1.01
    MeasurementImpact      float64       // 测量影响因子，默认 0.1
    RTTMinEstimate         time.Duration // 最小 RTT，默认 2s
    RTTMaxEstimate         time.Duration // 最大 RTT，默认 20s
    RTTPushdownFactor      float64       // RTT 降低因子，默认 0.9
    TTLScaling             float64       // 超时缩放因子，默认 3.0
    TTLLimit               time.Duration // 超时上限，默认 60s
    TuningImpact           float64       // 调优影响因子，默认 0.25
    TuningConfidenceCap    int           // 置信度上限节点数，默认 10
}
```

## 算法

### 容量计算

```
Capacity = 1 + CapacityOverestimation * (吞吐量 * targetRTT)
```

### RTT 更新（EWMA）

```
RTT_new = (1 - impact) * RTT_old + impact * measured
```

### 全局 RTT 调优

- 使用中位数 RTT（偏向较小值）
- 置信度随节点数增加
- 定期调优内部缓存

## 依赖关系

- 无外部依赖
- 可被 Protocol 层（Messaging、PubSub）使用

## 来源

设计参考 go-ethereum `eth/fetcher` 的速率限制机制。
来源于 [20260118-additional-feature-absorption.md](../../../_discussions/20260118-additional-feature-absorption.md) Phase 10.1。
