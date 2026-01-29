# Protocol Streams 编码指南

> **组件**: P6-03 protocol_streams  
> **路径**: internal/protocol/streams/

---

## 目录结构

```
internal/protocol/streams/
├── doc.go              # 包文档
├── errors.go          # 错误定义
├── options.go         # 配置选项
├── protocol.go        # 协议ID管理
├── stream.go          # BiStream包装器
├── service.go         # Streams服务
├── module.go          # Fx模块
├── testing.go         # 测试辅助
└── *_test.go          # 测试文件
```

---

## 核心组件

### Service
- 实现 `interfaces.Streams`
- 管理处理器注册
- 提供流打开接口

### streamWrapper
- 实现 `interfaces.BiStream`
- 将 `interfaces.Stream` 适配为应用层流
- 提供统计信息

---

## 关键实现点

### 1. 流包装
```go
type streamWrapper struct {
    stream   interfaces.Stream
    protocol string
    opened   int64
    mu       sync.RWMutex
    closed   bool
}
```

### 2. 处理器注册
- 用户协议 → 完整协议ID
- 为所有Realm注册处理器
- 自动适配 Stream → BiStream

### 3. 协议ID格式
`/dep2p/app/<realmID>/streams/<protocol>/1.0.0`

---

**最后更新**: 2026-01-14
