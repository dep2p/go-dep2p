# 测试执行指南

**创建日期**: 2026-01-21  
**适用范围**: 开发者日常测试、CI/CD、发布前验证  
**关联文档**: 
- `../overview.md` (总体概览)
- `../strategy/quality_gates.md` (质量门禁)

---

## 一、快速开始

### 1.1 日常开发（1-2 分钟）

```bash
# 快速验证（短时模式）
go test ./... -short

# 完整单元测试
go test ./...

# 指定模块测试
go test ./internal/core/host/...
```

### 1.2 提交前检查（5-10 分钟）

```bash
# 编译检查
go build ./...

# 单元测试
go test ./...

# Race 检测
go test -race ./... -short -timeout 10m

# 可选：集成测试
go test -tags=integration ./tests/integration/... -timeout 5m
```

### 1.3 发布前验证（15-30 分钟）

```bash
# 完整测试套件
go test ./...
go test -race ./...
go test -tags=integration ./tests/integration/... -timeout 10m
go test -tags=e2e ./tests/e2e/... -timeout 15m
```

---

## 二、测试分层执行

### 2.1 单元测试（Layer 1）

**位置**: `internal/*_test.go`, `pkg/*_test.go`  
**特点**: 快速执行，使用 Mock，测试单个函数/方法

#### 基本命令

```bash
# 运行所有单元测试
go test ./...

# 详细输出
go test -v ./...

# 短时模式（跳过长时间测试）
go test -short ./...

# 指定超时
go test ./... -timeout 5m

# 显示覆盖率
go test -cover ./...

# 生成覆盖率报告
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out -o coverage.html
```

#### 按模块测试

```bash
# 测试单个模块
go test ./internal/core/host/...

# 测试多个模块
go test ./internal/core/identity/... ./internal/core/security/...

# 测试指定层级
go test ./internal/core/...           # Layer 1: Core
go test ./internal/discovery/...      # Layer 2: Discovery
go test ./internal/protocol/...       # Layer 3: Protocol
go test ./internal/realm/...          # Layer 4: Realm
```

#### Race 检测

```bash
# 运行 Race 检测
go test -race ./...

# Race 检测（短时模式）
go test -race ./... -short -timeout 10m

# 指定模块 Race 检测
go test -race ./internal/core/swarm/... -timeout 5m
```

---

### 2.2 集成测试（Layer 2）

**位置**: `tests/integration/`  
**特点**: 跨模块协作，使用真实组件，需要构建标签

#### 基本命令

```bash
# 运行所有集成测试
go test -tags=integration ./tests/integration/...

# 详细输出
go test -tags=integration -v ./tests/integration/...

# 指定超时
go test -tags=integration ./tests/integration/... -timeout 10m

# Race 检测
go test -tags=integration -race ./tests/integration/... -timeout 10m
```

#### 按层级测试

```bash
# Core 层集成测试
go test -tags=integration ./tests/integration/core/... -v

# Discovery 层集成测试
go test -tags=integration ./tests/integration/discovery/... -v

# Protocol 层集成测试
go test -tags=integration ./tests/integration/protocol/... -v

# Realm 层集成测试
go test -tags=integration ./tests/integration/realm/... -v
```

#### 单个集成测试

```bash
# 测试指定文件
go test -tags=integration ./tests/integration/core/connection_test.go -v

# 测试指定函数
go test -tags=integration ./tests/integration/core/ -run TestConnection -v
```

---

### 2.3 E2E 测试（Layer 3）

**位置**: `tests/e2e/`  
**特点**: 完整用户场景，单进程多节点，需要构建标签

#### 基本命令

```bash
# 运行所有 E2E 测试
go test -tags=e2e ./tests/e2e/...

# 详细输出
go test -tags=e2e -v ./tests/e2e/...

# 指定超时
go test -tags=e2e ./tests/e2e/... -timeout 15m

# Race 检测
go test -tags=e2e -race ./tests/e2e/... -timeout 15m
```

#### 按场景测试

```bash
# 用户场景测试
go test -tags=e2e ./tests/e2e/scenario/... -v

# 网络场景测试
go test -tags=e2e ./tests/e2e/network/... -v

# 韧性场景测试
go test -tags=e2e ./tests/e2e/resilience/... -v
```

#### 单个场景测试

```bash
# 聊天场景
go test -tags=e2e ./tests/e2e/scenario/chat_test.go -v

# NAT 穿透场景
go test -tags=e2e ./tests/e2e/network/nat_traversal_test.go -v

# 网络分区场景
go test -tags=e2e ./tests/e2e/resilience/partition_test.go -v
```

---

## 三、高级测试选项

### 3.1 覆盖率分析

```bash
# 生成覆盖率报告
go test -coverprofile=coverage.out ./...

# 查看总体覆盖率
go tool cover -func=coverage.out | grep total

# 按模块查看覆盖率
go tool cover -func=coverage.out | grep "internal/core/host"

# 生成 HTML 报告
go tool cover -html=coverage.out -o coverage.html

# 按模块生成独立报告
go test -coverprofile=host-cov.out ./internal/core/host/...
go tool cover -html=host-cov.out -o host-coverage.html
```

### 3.2 并行测试

```bash
# 指定并行数（默认为 CPU 核心数）
go test ./... -parallel 4

# 单进程运行（调试用）
go test ./... -parallel 1

# 最大并行
go test ./... -parallel 16
```

### 3.3 测试筛选

```bash
# 运行指定名称的测试
go test ./... -run TestHost_New

# 运行匹配正则的测试
go test ./... -run "TestHost_.*"

# 运行指定文件的测试
go test ./internal/core/host/host_test.go

# 跳过指定测试
go test ./... -skip TestSlow
```

### 3.4 详细输出

```bash
# 详细输出（-v）
go test -v ./...

# 显示每个测试耗时
go test -v ./... | grep -E "PASS|FAIL"

# 只显示失败测试
go test ./... 2>&1 | grep -A 10 FAIL

# 显示测试日志
go test -v ./... | tee test.log
```

---

## 四、常见任务

### 4.1 调试失败的测试

```bash
# 步骤 1: 运行失败的测试
go test ./internal/core/host/... -v

# 步骤 2: 运行指定测试函数
go test ./internal/core/host/... -run TestHost_New -v

# 步骤 3: 添加更多日志
# 在测试代码中添加 t.Logf("...")

# 步骤 4: 单独运行（禁用缓存）
go test -count=1 ./internal/core/host/... -run TestHost_New -v

# 步骤 5: 检查 Race 问题
go test -race ./internal/core/host/... -run TestHost_New -v
```

### 4.2 检查数据竞争

```bash
# 快速 Race 检测
go test -race ./... -short

# 完整 Race 检测
go test -race ./...

# 指定模块 Race 检测
go test -race ./internal/core/swarm/...

# 检查集成测试 Race
go test -tags=integration -race ./tests/integration/...

# 保存 Race 报告
go test -race ./... 2>&1 | tee race-report.txt
```

### 4.3 性能测试

```bash
# 运行 Benchmark
go test -bench=. ./...

# 指定 Benchmark 函数
go test -bench=BenchmarkHost ./internal/core/host/...

# Benchmark with 内存分析
go test -bench=. -benchmem ./...

# 指定运行时间
go test -bench=. -benchtime=10s ./...

# 对比 Benchmark
go test -bench=. ./... > old.txt
# 修改代码后
go test -bench=. ./... > new.txt
benchstat old.txt new.txt
```

### 4.4 测试覆盖率提升

```bash
# 步骤 1: 查看当前覆盖率
go test -coverprofile=cov.out ./internal/core/host/...
go tool cover -func=cov.out

# 步骤 2: 查看未覆盖代码
go tool cover -html=cov.out

# 步骤 3: 补充测试
# （编写测试覆盖红色部分）

# 步骤 4: 验证覆盖率提升
go test -coverprofile=cov-new.out ./internal/core/host/...
go tool cover -func=cov-new.out
```

---

## 五、CI/CD 集成

### 5.1 GitHub Actions 示例

```yaml
name: Tests

on: [push, pull_request]

jobs:
  unit:
    name: Unit Tests
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.22'
      
      - name: Run Unit Tests
        run: go test ./... -short -timeout 5m

  race:
    name: Race Detection
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.22'
      
      - name: Race Detection
        run: go test -race ./... -short -timeout 10m

  integration:
    name: Integration Tests
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.22'
      
      - name: Run Integration Tests
        run: go test -tags=integration -race ./tests/integration/... -v -timeout 10m

  e2e:
    name: E2E Tests
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.22'
      
      - name: Run E2E Tests
        run: go test -tags=e2e -race ./tests/e2e/... -v -timeout 15m
```

### 5.2 Makefile 快捷命令

```makefile
.PHONY: test test-unit test-race test-integration test-e2e test-all coverage

# 快速测试
test: test-unit

# 单元测试
test-unit:
	@echo "Running unit tests..."
	@go test ./... -short

# Race 检测
test-race:
	@echo "Running race detection..."
	@go test -race ./... -short -timeout 10m

# 集成测试
test-integration:
	@echo "Running integration tests..."
	@go test -tags=integration -race ./tests/integration/... -v -timeout 10m

# E2E 测试
test-e2e:
	@echo "Running E2E tests..."
	@go test -tags=e2e -race ./tests/e2e/... -v -timeout 15m

# 完整测试套件
test-all: test-unit test-race test-integration test-e2e
	@echo "All tests completed!"

# 覆盖率报告
coverage:
	@echo "Generating coverage report..."
	@go test -coverprofile=coverage.out ./...
	@go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

# 清理
clean:
	@rm -f coverage.out coverage.html *.test
```

使用方式：

```bash
# 快速测试
make test

# Race 检测
make test-race

# 集成测试
make test-integration

# 完整测试
make test-all

# 生成覆盖率
make coverage
```

---

## 六、测试脚本

### 6.1 测试基线脚本

创建 `scripts/test-baseline.sh`:

```bash
#!/bin/bash
# DeP2P 测试基线脚本

set -e

echo "=== DeP2P 测试基线报告 ==="
echo "生成时间: $(date)"
echo "Go 版本: $(go version)"
echo ""

# 1. 编译检查
echo "1. 编译检查..."
go build ./... && echo "✅ 编译通过" || echo "❌ 编译失败"
echo ""

# 2. 单元测试
echo "2. 单元测试..."
go test ./... -short -timeout 5m 2>&1 | tee test-short.log
PASS_COUNT=$(grep -c "^PASS" test-short.log || true)
FAIL_COUNT=$(grep -c "^FAIL" test-short.log || true)
echo "通过: $PASS_COUNT, 失败: $FAIL_COUNT"
echo ""

# 3. Race 检测
echo "3. Race 检测（短时模式）..."
go test -race ./... -short -timeout 10m 2>&1 | tee race-short.log
RACE_COUNT=$(grep -c "WARNING: DATA RACE" race-short.log || true)
if [ "$RACE_COUNT" -eq 0 ]; then
  echo "✅ 无数据竞争"
else
  echo "❌ 发现 $RACE_COUNT 处数据竞争"
fi
echo ""

# 4. 覆盖率
echo "4. 覆盖率统计..."
go test -coverprofile=coverage.out ./... -timeout 5m
go tool cover -func=coverage.out | grep total
echo ""

echo "=== 报告生成完成 ==="
echo "详细日志: test-short.log, race-short.log"
echo "覆盖率报告: coverage.out"
```

### 6.2 模块测试脚本

创建 `scripts/test-module.sh`:

```bash
#!/bin/bash
# 测试指定模块

if [ -z "$1" ]; then
  echo "用法: $0 <模块路径>"
  echo "示例: $0 internal/core/host"
  exit 1
fi

MODULE=$1

echo "=== 测试模块: $MODULE ==="
echo ""

# 1. 单元测试
echo "1. 单元测试..."
go test -v ./$MODULE/... -timeout 5m

# 2. Race 检测
echo ""
echo "2. Race 检测..."
go test -race -v ./$MODULE/... -timeout 5m

# 3. 覆盖率
echo ""
echo "3. 覆盖率..."
go test -coverprofile=module-cov.out ./$MODULE/...
go tool cover -func=module-cov.out
go tool cover -html=module-cov.out -o module-coverage.html
echo "覆盖率报告: module-coverage.html"
```

---

## 七、故障排查

### 7.1 测试失败

**问题**: 测试随机失败（Flaky Test）

```bash
# 多次运行测试
for i in {1..10}; do
  echo "运行 $i/10..."
  go test ./internal/core/host/... -run TestHost_New || break
done

# 使用 -count 多次运行
go test ./internal/core/host/... -run TestHost_New -count=10
```

**问题**: 测试超时

```bash
# 增加超时时间
go test ./... -timeout 30m

# 查看慢测试
go test -v ./... | grep -E "PASS|FAIL" | sort -k3 -n
```

### 7.2 Race 检测问题

**问题**: Race 检测报告难以理解

```bash
# 保存完整报告
go test -race ./... 2>&1 | tee race-full.txt

# 查找具体文件
grep "goroutine" race-full.txt -A 10

# 运行单个测试的 Race 检测
go test -race ./internal/core/swarm/... -run TestSwarm -v
```

### 7.3 覆盖率问题

**问题**: 覆盖率突然下降

```bash
# 对比两次覆盖率
go test -coverprofile=cov-old.out ./...
# （修改代码）
go test -coverprofile=cov-new.out ./...

# 查看差异
go tool cover -func=cov-old.out | grep total
go tool cover -func=cov-new.out | grep total
```

---

## 八、最佳实践

### 8.1 测试原则

1. **快速反馈**: 日常开发使用 `-short` 模式
2. **完整验证**: 提交前运行完整测试
3. **并发测试**: 共享状态必须有 Race 检测
4. **覆盖率目标**: 核心模块 ≥ 80%，其他 ≥ 60%
5. **失败必修**: 不允许提交失败的测试

### 8.2 命令速查

| 场景 | 命令 | 耗时 |
|------|------|------|
| 快速验证 | `go test ./... -short` | 1-2 分钟 |
| 完整单元测试 | `go test ./...` | 3-5 分钟 |
| Race 检测 | `go test -race ./... -short` | 5-10 分钟 |
| 集成测试 | `go test -tags=integration ./tests/integration/...` | 5-10 分钟 |
| E2E 测试 | `go test -tags=e2e ./tests/e2e/...` | 10-15 分钟 |
| 完整测试套件 | `make test-all` | 20-30 分钟 |

---

## 九、相关文档

| 文档 | 描述 |
|------|------|
| `../overview.md` | 测试策略改进概览 |
| `../strategy/quality_gates.md` | 质量门禁 |
| `../implementation/plan.md` | 实施计划 |
| `../audits/module_audits.md` | 模块审查报告 |

---

**最后更新**: 2026-01-21  
**维护者**: DeP2P Team
