# 测试策略

> 整体测试方法论、目标和工具选型

---

## 元信息

| 字段 | 值 |
|------|-----|
| **状态** | approved |
| **Owner** | DeP2P Team |
| **创建日期** | 2026-01-11 |
| **更新日期** | 2026-01-20 |

---

## 1. 测试目标

### 1.1 质量目标

| 目标 | 指标 | 说明 |
|------|------|------|
| **正确性** | 功能符合规范 | 所有需求有对应测试 |
| **可靠性** | 无崩溃、无数据丢失 | 边界条件和异常路径覆盖 |
| **性能** | 满足性能指标 | 延迟、吞吐量基准 |
| **安全性** | 无安全漏洞 | 安全测试覆盖 |

### 1.2 覆盖率目标

| 模块类型 | 行覆盖率 | 分支覆盖率 |
|----------|----------|------------|
| 核心模块 (`internal/core/`) | ≥ 80% | ≥ 70% |
| 公共接口 (`pkg/interfaces/`) | ≥ 90% | ≥ 80% |
| 工具包 (`internal/util/`) | ≥ 70% | ≥ 60% |

---

## 2. 测试方法论

### 2.1 测试驱动开发 (TDD)

对于核心功能，推荐采用 TDD：

1. **Red**：先写失败的测试
2. **Green**：写最少代码使测试通过
3. **Refactor**：重构代码，保持测试通过

### 2.2 行为驱动开发 (BDD)

对于复杂业务逻辑，采用 BDD 风格：

```go
func TestNodeID_Generation(t *testing.T) {
    t.Run("Given a new key pair", func(t *testing.T) {
        t.Run("When generating NodeID", func(t *testing.T) {
            t.Run("Then NodeID should be derived from public key", func(t *testing.T) {
                // 测试代码
            })
        })
    })
}
```

### 2.3 表驱动测试

Go 标准的表驱动测试方式：

```go
func TestXxx(t *testing.T) {
    tests := []struct {
        name    string
        input   Type
        want    Type
        wantErr bool
    }{
        {"case1", input1, want1, false},
        {"case2", input2, want2, true},
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got, err := Xxx(tt.input)
            if (err != nil) != tt.wantErr {
                t.Errorf("Xxx() error = %v, wantErr %v", err, tt.wantErr)
                return
            }
            if got != tt.want {
                t.Errorf("Xxx() = %v, want %v", got, tt.want)
            }
        })
    }
}
```

---

## 3. 测试工具

### 3.1 核心工具

| 工具 | 用途 | 说明 |
|------|------|------|
| `go test` | 测试执行 | Go 标准测试框架 |
| `testify` | 断言库 | `assert`、`require`、`mock` |
| `gomock` | Mock 生成 | 接口 Mock |
| `httptest` | HTTP 测试 | 标准库 |

### 3.2 辅助工具

| 工具 | 用途 |
|------|------|
| `go test -cover` | 覆盖率收集 |
| `go test -race` | 竞态检测 |
| `go test -bench` | 基准测试 |
| `dlv test` | 测试调试 |

### 3.3 CI 工具

| 工具 | 用途 |
|------|------|
| GitHub Actions | CI/CD 执行 |
| Codecov | 覆盖率报告 |
| benchstat | 基准对比 |

---

## 4. 测试分类

### 4.1 按层次分类（四层架构）

P2P 系统的特殊性决定了需要在传统三层测试金字塔上增加**第四层：真实网络验证**。

```
┌─────────────────────────────────────────────────────────────┐
│               Level 4: 真实网络验证                          │
│   - NAT 穿透、中继通信、跨网络连接                            │
│   - 无法本地模拟，需要真实网络环境                            │
│   - 手动执行 + AI 日志分析                                   │
├─────────────────────────────────────────────────────────────┤
│               Level 3: E2E 测试                              │
│   - 完整用户流程                                              │
│   - 本地多节点场景                                            │
├─────────────────────────────────────────────────────────────┤
│               Level 2: 集成测试                              │
│   - 模块间交互                                                │
│   - 单进程内部依赖                                            │
├─────────────────────────────────────────────────────────────┤
│               Level 1: 单元测试                              │
│   - 函数/方法级别                                             │
│   - 完全隔离测试                                              │
└─────────────────────────────────────────────────────────────┘
```

**为什么需要第四层？**

| 场景 | 本地测试能力 | 原因 |
|------|:------------:|------|
| NAT 穿透 | ❌ | 需要真实 NAT 设备 |
| STUN 地址发现 | ❌ | 需要真实公网 STUN |
| 中继跨网络通信 | ⚠️ 部分 | 可模拟但不完整 |
| 4G/WiFi 切换 | ❌ | 需要真实移动网络 |

**详细说明**：[network_validation.md](network_validation.md)

### 4.2 按类型分类

| 类型 | 标签 | 说明 |
|------|------|------|
| 功能测试 | - | 功能正确性 |
| 边界测试 | - | 边界条件 |
| 错误测试 | - | 错误处理 |
| 并发测试 | `-race` | 并发安全 |
| 性能测试 | `-bench` | 性能指标 |
| 模糊测试 | `-fuzz` | 随机输入 |

---

## 5. 测试命名规范

### 5.1 测试文件

```
{source}_test.go          # 单元测试
{source}_integration_test.go  # 集成测试（需要 build tag）
```

### 5.2 测试函数

```go
// 单元测试
func TestXxx(t *testing.T) {}

// 子测试
func TestXxx_MethodName(t *testing.T) {}
func TestXxx_MethodName_Scenario(t *testing.T) {}

// 基准测试
func BenchmarkXxx(b *testing.B) {}

// 模糊测试
func FuzzXxx(f *testing.F) {}

// 示例测试
func ExampleXxx() {}
```

### 5.3 测试用例命名

```go
t.Run("should return error when input is nil", func(t *testing.T) {})
t.Run("should succeed with valid input", func(t *testing.T) {})
```

---

## 6. 测试数据管理

### 6.1 测试夹具

```
tests/
├── fixtures/              # 测试夹具
│   ├── keys/              # 测试密钥
│   ├── certs/             # 测试证书
│   └── data/              # 测试数据
└── testdata/              # Go 标准测试数据目录
```

### 6.2 数据生成

```go
// 使用 helper 生成测试数据
func newTestNode(t *testing.T) *Node {
    t.Helper()
    // 创建测试节点
    return node
}
```

---

## 7. Mock 策略

### 7.1 何时使用 Mock

- 外部依赖（网络、文件系统）
- 慢操作（数据库、远程调用）
- 不确定性操作（时间、随机数）

### 7.2 Mock 生成

```bash
# 使用 gomock
mockgen -source=interface.go -destination=mock_interface.go -package=mocks
```

### 7.3 Mock 示例

```go
func TestWithMock(t *testing.T) {
    ctrl := gomock.NewController(t)
    defer ctrl.Finish()
    
    mockService := mocks.NewMockService(ctrl)
    mockService.EXPECT().Method(gomock.Any()).Return(expected, nil)
    
    // 使用 mock 进行测试
}
```

---

## 8. 测试执行

### 8.1 本地执行（四层）

```bash
# ===== Level 1: 单元测试 =====
go test ./...                                    # 运行所有单元测试
go test ./internal/core/identity/...             # 运行特定包
go test -run TestNodeID ./internal/core/identity/ # 运行特定测试
go test -cover ./...                             # 带覆盖率
go test -race ./...                              # 竞态检测

# ===== Level 2: 集成测试 =====
go test -tags=integration ./tests/integration/... -timeout 5m

# ===== Level 3: E2E 测试 =====
go test -tags=e2e ./tests/e2e/... -timeout 10m

# ===== Level 4: 真实网络验证 =====
# 1. 部署节点（云服务器、WiFi、4G）
# 2. 执行 Chat 示例操作
# 3. 收集日志
# 4. 提交 AI 分析
# 详细步骤见 network_validation.md
```

### 8.2 CI 执行

```yaml
# .github/workflows/test.yml
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.21'
      - name: Run tests
        run: go test -race -coverprofile=coverage.out ./...
      - name: Upload coverage
        uses: codecov/codecov-action@v4
```

---

## 9. 测试报告

### 9.1 覆盖率报告

```bash
# 生成覆盖率文件
go test -coverprofile=coverage.out ./...

# 查看 HTML 报告
go tool cover -html=coverage.out -o coverage.html

# 查看函数覆盖率
go tool cover -func=coverage.out
```

### 9.2 测试结果报告

```bash
# JSON 格式输出
go test -json ./... > test-report.json

# 使用 gotestsum
gotestsum --format testname --junitfile junit.xml ./...
```

---

## 10. 持续改进

### 10.1 测试债务管理

- 定期审查测试覆盖率
- 识别和修复 Flaky 测试
- 优化慢测试

### 10.2 测试文化

- Code Review 必须包含测试
- 新功能必须有测试
- Bug 修复必须有回归测试

---

## 11. 相关文档

| 文档 | 说明 |
|------|------|
| [test_pyramid.md](test_pyramid.md) | 测试金字塔（四层架构详细） |
| [test_matrix.md](test_matrix.md) | 模块覆盖矩阵 |
| [network_validation.md](network_validation.md) | **真实网络验证策略** |
| [quality_gates.md](quality_gates.md) | 质量门禁 |

---

## 变更历史

| 版本 | 日期 | 作者 | 变更说明 |
|------|------|------|----------|
| v1.0 | 2026-01-11 | DeP2P Team | 初始版本 |
| v1.1 | 2026-01-20 | DeP2P Team | 添加四层测试架构说明，增加真实网络验证引用 |
