# Delivery 模块设计

> Protocol Layer - 可靠消息投递

## 概述

delivery 模块提供可靠消息投递功能，通过消息队列和 ACK 确认机制保证消息可靠性。

## 核心功能

1. **消息队列** - 网络不可用时缓存消息
2. **自动重发** - 网络恢复后自动发送
3. **ACK 确认** - 关键节点确认机制
4. **状态回调** - 通知发送状态

## 代码位置

```
internal/protocol/delivery/
├── doc.go           # 包文档
├── publisher.go     # 可靠发布器
├── publisher_test.go
├── queue.go         # 消息队列
├── ack.go           # ACK 处理
├── errors.go        # 错误定义
└── testing.go       # 测试辅助
```

## ACK 协议

```
消息格式: [ack_request_len(2bytes)][ack_request][payload]
```

## 依赖

- PubSub 模块（底层发布）
- EventBus 模块（状态事件）
