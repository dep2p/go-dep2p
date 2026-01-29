# 性能基准 (Benchmarking)

> 性能指标、基准测试、回归检测

---

## 目录结构

```
benchmarking/
├── README.md              # 本文件
├── metrics.md             # 性能指标定义
└── plans/                 # 基准测试计划
    └── README.md
```

---

## 概述

本目录定义 DeP2P 的性能基准测试框架，用于：

- 建立性能基准线
- 检测性能回归
- 指导性能优化
- 发布前性能验证

---

## 核心指标

| 指标类别 | 指标 | 目标值 |
|----------|------|--------|
| **延迟** | 连接建立 P99 | ≤ 100ms |
| **延迟** | 消息传递 P99 | ≤ 50ms |
| **吞吐量** | 数据传输 | ≥ 100MB/s |
| **吞吐量** | 消息处理 | ≥ 10,000 msg/s |
| **资源** | 内存/连接 | ≤ 10KB |
| **资源** | CPU/1000msg | ≤ 1% |

---

## 测试类型

| 类型 | 说明 | 执行频率 |
|------|------|----------|
| 微基准 | 单函数/方法性能 | 每次 PR |
| 组件基准 | 模块级性能 | 每周 |
| 系统基准 | 端到端性能 | 发布前 |
| 压力测试 | 极限场景 | 发布前 |

---

## 运行方式

### 微基准测试

```bash
# 运行所有基准测试
go test -bench=. -benchmem ./...

# 运行特定模块
go test -bench=. -benchmem ./internal/core/identity/...

# 指定运行时间
go test -bench=. -benchtime=5s ./...

# 生成 CPU profile
go test -bench=BenchmarkXxx -cpuprofile=cpu.prof ./...
```

### 基准对比

```bash
# 安装 benchstat
go install golang.org/x/perf/cmd/benchstat@latest

# 运行并保存结果
go test -bench=. -count=5 ./... > old.txt
# ... 代码修改 ...
go test -bench=. -count=5 ./... > new.txt

# 对比结果
benchstat old.txt new.txt
```

---

## CI 集成

```yaml
# .github/workflows/benchmark.yml
name: Benchmark

on:
  push:
    branches: [main]
  pull_request:
    branches: [main]

jobs:
  benchmark:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
      
      - name: Run benchmarks
        run: go test -bench=. -benchmem -count=5 ./... > bench.txt
      
      - name: Upload results
        uses: actions/upload-artifact@v4
        with:
          name: benchmark-results
          path: bench.txt
      
      - name: Compare with base (PR only)
        if: github.event_name == 'pull_request'
        run: |
          # 获取基准分支结果
          git checkout ${{ github.base_ref }}
          go test -bench=. -benchmem -count=5 ./... > base.txt
          git checkout ${{ github.head_ref }}
          
          # 对比
          benchstat base.txt bench.txt
```

---

## 回归检测

### 阈值定义

| 指标 | 回归阈值 | 动作 |
|------|----------|------|
| 吞吐量下降 | > 10% | 🔴 阻断 |
| 延迟增加 | > 20% | 🟡 警告 |
| 内存增加 | > 15% | 🟡 警告 |
| allocs/op 增加 | > 20% | 🟡 警告 |

### 自动检测

```bash
#!/bin/bash
# scripts/check-regression.sh

benchstat old.txt new.txt | grep -E "^\+" | while read line; do
    delta=$(echo "$line" | grep -oE "[0-9]+\.[0-9]+%" | head -1)
    if (( $(echo "$delta > 10" | bc -l) )); then
        echo "::error::Performance regression: $line"
        exit 1
    fi
done
```

---

## 快速链接

| 文档 | 说明 |
|------|------|
| [metrics.md](metrics.md) | 性能指标定义 |
| [plans/](plans/) | 基准测试计划 |

---

**最后更新**：2026-01-11
