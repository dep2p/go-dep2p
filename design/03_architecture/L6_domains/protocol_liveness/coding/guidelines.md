# Protocol Liveness 编码指南

> **组件**: P6-04 protocol_liveness  
> **路径**: internal/protocol/liveness/

---

## 目录结构

```
internal/protocol/liveness/
├── doc.go              # 包文档
├── errors.go          # 错误定义
├── options.go         # 配置选项
├── protocol.go        # 协议ID管理
├── message.go         # Ping/Pong消息
├── status.go          # 状态管理
├── service.go         # Liveness服务
├── module.go          # Fx模块
├── testing.go         # 测试辅助
└── *_test.go          # 测试文件
```

---

## 核心组件

### Service
- 实现 `interfaces.Liveness`
- 管理节点状态
- 提供Ping/Watch接口

### peerStatus
- 维护单个节点状态
- RTT统计和平均值
- 失败计数和阈值判定

### Ping/Pong
- JSON序列化
- 流式传输
- RTT测量

---

## 关键实现点

### 1. Ping机制
```go
// 1. 查找Realm
// 2. 打开流
// 3. 发送Ping
// 4. 接收Pong
// 5. 计算RTT
// 6. 更新状态
```

### 2. 状态管理
- 滑动窗口平均RTT
- 失败阈值判定
- 并发安全

### 3. Watch通知
- 异步事件通知
- 防止死锁
- 支持多监听者

---

**最后更新**: 2026-01-14
