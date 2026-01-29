# Protocol Layer (协议层)

协议层实现用户级应用协议。

## 模块列表

| 模块 | 说明 | 协议 ID |
|------|------|---------|
| `messaging` | 请求/响应消息 | `/dep2p/app/<realmID>/messaging/1.0.0` |
| `pubsub` | 发布/订阅 | `/dep2p/app/<realmID>/pubsub/1.0.0` |
| `streams` | 双向流 | `/dep2p/app/<realmID>/streams/1.0.0` |
| `liveness` | 存活检测 | `/dep2p/app/<realmID>/liveness/1.0.0` |

## 架构原则

1. 所有应用协议都在 Realm 上下文中运行
2. 协议 ID 包含 RealmID，确保隔离
3. 每个模块提供 `Module()` 函数作为 Fx 入口
