# Protocol Liveness 设计概览

> **组件**: P6-04 protocol_liveness  
> **协议**: Ping/Pong

---

## 架构设计

```
Service (interfaces.Liveness)
  ├── peerStatus (状态管理)
  │   ├── RTT样本
  │   ├── 失败计数
  │   └── 存活状态
  ├── watches (事件通知)
  └── realmMgr (协议ID生成)
```

---

## Ping/Pong 机制

### 消息格式
```json
PingRequest: {
    "id": "uuid",
    "timestamp": 1234567890
}

PongResponse: {
    "id": "uuid",
    "timestamp": 1234567891
}
```

### RTT计算
```
RTT = 接收Pong时间 - 发送Ping时间
```

### 失败判定
- 连续失败 >= FailThreshold → 下线
- 成功 → 重置失败计数 → 上线

---

## Watch/Unwatch

### 事件类型
- Pong: 收到响应
- Timeout: 超时
- Up: 节点上线
- Down: 节点下线

### 通知机制
- 创建事件通道
- 异步通知监听者
- 防止阻塞

---

## Realm集成

### 协议ID
- 格式: `/dep2p/app/<realmID>/liveness/ping/1.0.0`
- 示例: `/dep2p/app/my-realm/liveness/ping/1.0.0`

### 成员验证
- 仅Realm成员可Ping
- 通过RealmManager检查

---

**最后更新**: 2026-01-14
