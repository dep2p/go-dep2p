# Protocol PubSub 编码指南

> **组件**: P6-02 protocol_pubsub  
> **路径**: internal/protocol/pubsub/

---

## 目录结构

```
internal/protocol/pubsub/
├── doc.go                  # 包文档
├── errors.go              # 错误定义
├── options.go             # 配置选项
├── protocol.go            # 协议管理
├── message.go             # 消息处理
├── validator.go           # 消息验证
├── subscription.go        # 订阅实现
├── event_handler.go       # 事件处理
├── topic.go               # 主题管理
├── mesh.go                # Mesh管理
├── heartbeat.go           # 心跳机制
├── gossipsub.go           # GossipSub核心
├── service.go             # PubSub服务
├── module.go              # Fx模块
├── testing.go             # 测试辅助
└── *_test.go              # 测试文件
```

---

## 核心组件

### Service
- 实现 `interfaces.PubSub`
- 聚合 gossipSub 和 validator
- 提供公共API

### GossipSub
- 实现 GossipSub 协议核心逻辑
- 管理 Mesh、消息缓存、心跳

### Topic
- 实现 `interfaces.Topic`
- 管理订阅者和事件处理器

---

**最后更新**: 2026-01-14
