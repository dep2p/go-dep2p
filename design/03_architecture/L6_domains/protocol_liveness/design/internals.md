# Protocol Liveness 实现细节

> **组件**: P6-04 protocol_liveness

---

## 核心实现

### 1. peerStatus

**职责**: 管理单个节点的状态

```go
type peerStatus struct {
    peerID     string
    alive      bool
    lastSeen   time.Time
    lastRTT    time.Duration
    rttSamples []time.Duration  // 滑动窗口
    failCount  int
    mu         sync.RWMutex
}
```

**RTT平均值**:
- 使用滑动窗口（最多10个样本）
- 计算所有样本的平均值

**失败阈值**:
- 连续失败 >= 3次 → alive = false
- 成功 → failCount = 0, alive = true

---

### 2. Service

**职责**: Liveness服务管理

```go
type Service struct {
    host     interfaces.Host
    realmMgr interfaces.RealmManager
    statuses map[string]*peerStatus
    watches  map[string][]chan interfaces.LivenessEvent
    mu       sync.RWMutex
    started  bool
    ctx      context.Context
    cancel   context.CancelFunc
    config   *Config
}
```

---

### 3. Ping实现流程

```
1. 验证服务已启动
2. 查找节点所在Realm
3. 构建协议ID
4. 打开流（带超时）
5. 发送Ping请求
6. 接收Pong响应
7. 验证响应ID
8. 计算RTT
9. 更新状态
10. 通知监听者
```

---

### 4. 并发安全

**竞态修复**:
- `Unwatch()`: 先从map删除，再关闭通道
- `notifyWatchers()`: 持有锁时发送，使用 select+default 防止阻塞
- `peerStatus`: 独立的RWMutex保护

**关键点**:
- 避免在持有锁时执行阻塞操作
- 使用 select+default 实现非阻塞发送
- Unwatch 先删除后关闭

---

### 5. 消息编解码

**格式**: JSON

**发送**:
```
[4字节长度][JSON数据]
```

**接收**:
```
读取4字节长度 → 读取数据 → JSON解码
```

---

**最后更新**: 2026-01-14
