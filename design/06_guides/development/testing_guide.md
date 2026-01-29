# 测试指南

> 测试编写、Mock 使用、测试技巧

---

## 1. 测试基础

### 1.1 测试文件

```
module.go        # 源代码
module_test.go   # 测试代码
```

### 1.2 基本测试

```go
func TestFunctionName(t *testing.T) {
    // 准备
    input := "test"
    expected := "result"
    
    // 执行
    result := FunctionName(input)
    
    // 断言
    if result != expected {
        t.Errorf("got %v, want %v", result, expected)
    }
}
```

---

## 2. 表驱动测试

```go
func TestAdd(t *testing.T) {
    tests := []struct {
        name     string
        a, b     int
        expected int
    }{
        {"positive", 1, 2, 3},
        {"negative", -1, -2, -3},
        {"zero", 0, 0, 0},
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            result := Add(tt.a, tt.b)
            if result != tt.expected {
                t.Errorf("Add(%d, %d) = %d, want %d",
                    tt.a, tt.b, result, tt.expected)
            }
        })
    }
}
```

---

## 3. 使用 testify

### 3.1 断言

```go
import "github.com/stretchr/testify/assert"

func TestWithAssert(t *testing.T) {
    result := DoSomething()
    
    assert.Equal(t, expected, result)
    assert.NoError(t, err)
    assert.True(t, condition)
    assert.Contains(t, slice, element)
}
```

### 3.2 Require（失败立即停止）

```go
import "github.com/stretchr/testify/require"

func TestWithRequire(t *testing.T) {
    result, err := DoSomething()
    require.NoError(t, err)  // 失败则停止
    
    assert.Equal(t, expected, result)
}
```

---

## 4. Mock 使用

### 4.1 生成 Mock

```bash
# 使用 mockgen
mockgen -source=interface.go -destination=mock_interface.go -package=mocks
```

### 4.2 使用 Mock

```go
func TestWithMock(t *testing.T) {
    ctrl := gomock.NewController(t)
    defer ctrl.Finish()
    
    mockService := mocks.NewMockService(ctrl)
    mockService.EXPECT().
        Method(gomock.Any()).
        Return(expected, nil).
        Times(1)
    
    // 使用 mock
    result, err := UseService(mockService)
    require.NoError(t, err)
    assert.Equal(t, expected, result)
}
```

---

## 5. 测试辅助函数

### 5.1 Helper 函数

```go
func createTestNode(t *testing.T) *Node {
    t.Helper()  // 标记为 helper
    
    node, err := NewNode(DefaultConfig())
    require.NoError(t, err)
    
    t.Cleanup(func() {
        node.Close()
    })
    
    return node
}
```

### 5.2 临时资源

```go
func TestWithTempDir(t *testing.T) {
    dir := t.TempDir()  // 自动清理
    
    // 使用临时目录
}
```

---

## 6. 并行测试

```go
func TestParallel(t *testing.T) {
    tests := []struct {
        name string
        // ...
    }{
        {"case1", ...},
        {"case2", ...},
    }
    
    for _, tt := range tests {
        tt := tt  // 捕获变量
        t.Run(tt.name, func(t *testing.T) {
            t.Parallel()  // 并行执行
            // 测试逻辑
        })
    }
}
```

---

## 7. 集成测试

### 7.1 Build Tag

```go
//go:build integration

package integration

func TestIntegration(t *testing.T) {
    // 集成测试
}
```

### 7.2 运行集成测试

```bash
go test -tags=integration ./tests/integration/...
```

---

## 8. 基准测试

```go
func BenchmarkFunction(b *testing.B) {
    // 准备
    data := prepareData()
    
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        Function(data)
    }
}

func BenchmarkParallel(b *testing.B) {
    b.RunParallel(func(pb *testing.PB) {
        for pb.Next() {
            Function()
        }
    })
}
```

运行：

```bash
go test -bench=. -benchmem ./...
```

---

## 9. 模糊测试

```go
func FuzzParse(f *testing.F) {
    // 添加种子
    f.Add([]byte("valid input"))
    f.Add([]byte(""))
    
    f.Fuzz(func(t *testing.T, data []byte) {
        result, err := Parse(data)
        if err != nil {
            return  // 预期可能失败
        }
        // 验证结果
        _ = result
    })
}
```

运行：

```bash
go test -fuzz=FuzzParse ./...
```

---

## 10. 测试覆盖率

### 10.1 生成覆盖率

```bash
# 运行测试并生成覆盖率
go test -coverprofile=coverage.out ./...

# 查看覆盖率报告
go tool cover -func=coverage.out

# HTML 报告
go tool cover -html=coverage.out -o coverage.html
```

### 10.2 覆盖率目标

| 模块类型 | 目标覆盖率 |
|----------|-----------|
| 核心模块 | ≥ 80% |
| 公共接口 | ≥ 90% |
| 工具函数 | ≥ 70% |

---

## 11. 常见问题

### 11.1 Flaky 测试

避免时间依赖：

```go
// 不好
time.Sleep(time.Second)

// 好：使用条件等待
require.Eventually(t, func() bool {
    return condition()
}, 5*time.Second, 100*time.Millisecond)
```

### 11.2 测试隔离

确保测试独立：

```go
func TestIsolated(t *testing.T) {
    // 使用随机端口
    listener, _ := net.Listen("tcp", "127.0.0.1:0")
    defer listener.Close()
    
    // 使用临时目录
    dir := t.TempDir()
}
```

---

**最后更新**：2026-01-11
