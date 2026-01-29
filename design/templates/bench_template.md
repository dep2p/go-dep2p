# BENCH-{NNNN}: 基准测试标题

> 模板版本: v1.0

---

## 元信息

| 字段 | 值 |
|------|-----|
| **ID** | BENCH-{NNNN} |
| **类型** | throughput / latency / resource / stress |
| **Owner** | @xxx |
| **关联模块** | `internal/core/{module}/` |
| **创建日期** | YYYY-MM-DD |
| **更新日期** | YYYY-MM-DD |

---

## 1. 测试目标

[必填] 描述本基准测试要验证的性能指标。

### 1.1 目标指标

| 指标 | 目标值 | 说明 |
|------|--------|------|
| 吞吐量 | ≥ N ops/s | 每秒操作数 |
| 延迟 P50 | ≤ N ms | 50 分位延迟 |
| 延迟 P99 | ≤ N ms | 99 分位延迟 |
| 内存使用 | ≤ N MB | 最大内存占用 |
| CPU 使用 | ≤ N% | 最大 CPU 占用 |

### 1.2 基准线

- **基准版本**：v0.x.x
- **基准环境**：描述基准测试的硬件/软件环境
- **基准数据**：引用历史基准数据

---

## 2. 测试环境

[必填] 测试执行的环境要求。

### 2.1 硬件要求

| 资源 | 规格 |
|------|------|
| CPU | N 核 |
| 内存 | N GB |
| 磁盘 | SSD/HDD, N GB |
| 网络 | N Gbps |

### 2.2 软件要求

| 软件 | 版本 |
|------|------|
| Go | ≥ 1.21 |
| OS | Linux/macOS |

### 2.3 环境配置

```bash
# 环境变量
export GOMAXPROCS=N
export GOGC=100

# 系统配置
ulimit -n 65535
```

---

## 3. 测试场景

[必填] 详细的测试场景定义。

### 3.1 场景 1: 场景名称

**描述**：场景描述...

**参数**：

| 参数 | 值 | 说明 |
|------|-----|------|
| 并发数 | N | 并发连接/goroutine 数 |
| 数据大小 | N KB | 单次操作数据量 |
| 持续时间 | N s | 测试持续时间 |
| 预热时间 | N s | 预热阶段时间 |

**测试代码**：

```go
func BenchmarkXxx(b *testing.B) {
    // 准备
    // ...
    
    b.ResetTimer()
    b.RunParallel(func(pb *testing.PB) {
        for pb.Next() {
            // 被测操作
        }
    })
}
```

---

### 3.2 场景 2: 场景名称

**描述**：场景描述...

**参数**：

| 参数 | 值 | 说明 |
|------|-----|------|

**测试代码**：

```go
func BenchmarkXxx2(b *testing.B) {
    // ...
}
```

---

## 4. 测试数据

[可选] 测试使用的数据集。

### 4.1 数据生成

```go
// 测试数据生成
func generateTestData(size int) []byte {
    // ...
}
```

### 4.2 数据规格

| 数据集 | 大小 | 说明 |
|--------|------|------|
| small | 1 KB | 小数据测试 |
| medium | 100 KB | 中等数据测试 |
| large | 10 MB | 大数据测试 |

---

## 5. 执行方法

[必填] 如何执行基准测试。

### 5.1 运行命令

```bash
# 运行单个基准测试
go test -bench=BenchmarkXxx -benchmem -count=5 ./...

# 运行所有基准测试
go test -bench=. -benchmem -benchtime=10s ./...

# 生成 CPU profile
go test -bench=BenchmarkXxx -cpuprofile=cpu.prof ./...

# 生成内存 profile
go test -bench=BenchmarkXxx -memprofile=mem.prof ./...
```

### 5.2 结果收集

```bash
# 使用 benchstat 分析
go install golang.org/x/perf/cmd/benchstat@latest
benchstat old.txt new.txt
```

---

## 6. 预期结果

[必填] 基准测试的预期输出。

### 6.1 性能指标

| 场景 | 指标 | 预期值 | 允许波动 |
|------|------|--------|----------|
| 场景1 | ops/s | ≥ N | ±10% |
| 场景1 | P99 延迟 | ≤ N ms | ±20% |

### 6.2 资源使用

| 场景 | 资源 | 预期值 | 上限 |
|------|------|--------|------|
| 场景1 | 内存 | N MB | N MB |
| 场景1 | allocs/op | N | N |

---

## 7. 历史数据

[可选] 历史基准测试数据。

### 7.1 版本对比

| 版本 | 日期 | 吞吐量 | P99 延迟 | 备注 |
|------|------|--------|----------|------|
| v0.1.0 | YYYY-MM-DD | N ops/s | N ms | 基准 |
| v0.2.0 | YYYY-MM-DD | N ops/s | N ms | 优化 xxx |

### 7.2 趋势图

```
吞吐量趋势:
v0.1.0  ████████████████ 1000 ops/s
v0.2.0  ████████████████████ 1250 ops/s (+25%)
v0.3.0  ██████████████████████ 1400 ops/s (+12%)
```

---

## 8. 回归检测

[必填] 性能回归检测策略。

### 8.1 回归阈值

| 指标 | 回归阈值 | 动作 |
|------|----------|------|
| 吞吐量下降 | > 10% | 阻止合并 |
| 延迟增加 | > 20% | 警告 |
| 内存增加 | > 15% | 警告 |

### 8.2 CI 集成

```yaml
# .github/workflows/benchmark.yml
- name: Run Benchmarks
  run: go test -bench=. -benchmem ./... > bench.txt

- name: Compare Benchmarks
  run: benchstat base.txt bench.txt
```

---

## 9. 优化建议

[可选] 基于测试结果的优化建议。

### 9.1 发现的瓶颈

- 瓶颈 1：描述
- 瓶颈 2：描述

### 9.2 优化方向

- 优化建议 1
- 优化建议 2

---

## 变更历史

| 版本 | 日期 | 作者 | 变更说明 |
|------|------|------|----------|
| v1.0 | YYYY-MM-DD | @xxx | 初始版本 |
