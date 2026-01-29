# REQ-NET-004: 网络变化处理

> 定义节点在网络环境变化时的行为要求

---

## 基本信息

| 属性 | 值 |
|------|-----|
| **ID** | REQ-NET-004 |
| **标题** | 网络变化处理 |
| **优先级** | P1 |
| **状态** | draft |
| **创建日期** | 2026-01-15 |

---

## 需求描述

### 背景

移动设备和网络环境多变的场景下，节点会经历多种网络变化：

- **接口切换**：4G ↔ WiFi、WiFi A → WiFi B
- **IP 地址变化**：DHCP 续约、NAT 映射变化
- **短暂断网**：电梯、隧道、信号盲区
- **网络质量变化**：带宽下降、延迟增加

### 功能需求

#### FR-1: 网络变化检测

| ID | 描述 | 验收标准 |
|----|------|----------|
| FR-1.1 | 支持主动检测网络接口变化 | 在支持的平台上自动检测 |
| FR-1.2 | 支持被动检测（连接断开） | 连接断开时触发重连流程 |
| FR-1.3 | 支持外部通知网络变化 | 提供 `NetworkChange()` API |
| FR-1.4 | 区分主要变化和次要变化 | 接口切换为主要，IP 微调为次要 |

#### FR-2: 抖动容忍

| ID | 描述 | 验收标准 |
|----|------|----------|
| FR-2.1 | 短暂断连不立即移除节点状态 | 默认 5 秒容错窗口 |
| FR-2.2 | 断连后保持节点状态一段时间 | 默认 30 秒状态保持 |
| FR-2.3 | 支持指数退避重连 | 1s → 2s → 4s → ... → 30s（封顶） |
| FR-2.4 | 支持配置最大重连次数 | 默认 5 次 |
| FR-2.5 | 退避抖动 | ±20% jitter 防止雷群效应 |

**重试策略参数**（与概念澄清文档一致）：

| 参数 | 值 | 说明 |
|------|-----|------|
| 初始间隔 | 1s | InitialInterval |
| 退避因子 | 2 | Multiplier |
| 最大间隔 | 30s | MaxInterval |
| 最大重试 | 5 次 | MaxRetries |
| 抖动因子 | ±20% | Jitter |

#### FR-3: 地址更新

| ID | 描述 | 验收标准 |
|----|------|----------|
| FR-3.1 | 主要变化时重新获取外部地址 | 触发 STUN 重新探测 |
| FR-3.2 | 更新本地地址缓存 | 立即更新本地缓存 |
| FR-3.3 | 通知 Relay 地址簿 | 地址变化后 5 秒内通知 |
| FR-3.4 | 更新 DHT 中的地址广播 | 地址变化后 30 秒内更新 |
| FR-3.5 | 通知 Realm 成员地址变化 | 广播新的 Capability |

**地址更新流程**：

```
网络变化检测
    │
    ▼
1. STUN 重新探测（获取新外部地址）
    │
    ▼
2. 更新本地缓存
    │
    ▼
3. 通知 Relay 地址簿（AddressRegister 消息）
    │
    ▼
4. DHT 重新发布（仅发布已验证地址 + Relay 地址）
    │
    ▼
5. 广播给 Realm 成员
```

#### FR-4: 连接恢复

| ID | 描述 | 验收标准 |
|----|------|----------|
| FR-4.1 | 主要变化时重绑定 Socket | 释放旧 Socket，绑定新接口 |
| FR-4.2 | 关闭失效的 Relay 连接 | 源地址不再有效时关闭 |
| FR-4.3 | 重新建立关键连接 | Bootstrap、Relay 连接优先恢复 |

### 非功能需求

| ID | 描述 | 验收标准 |
|----|------|----------|
| NFR-1 | 恢复时间 | 主要变化后 < 5 秒恢复可用 |
| NFR-2 | 资源占用 | 监控服务 < 1MB 内存 |
| NFR-3 | 平台支持 | 支持 Linux、macOS、Windows、Android、iOS |

---

## 用户场景

### 场景 1: 4G → WiFi 切换

```
前置条件：
  - 用户设备通过 4G 网络连接
  - 已加入 Realm，与多个成员有活跃连接

触发：
  - 用户进入 WiFi 覆盖范围
  - 系统自动切换到 WiFi 网络

预期行为：
  1. 检测到网络接口变化（主要变化）
  2. 重新绑定 Socket 到 WiFi 接口
  3. 重新 STUN 获取新的外部地址
  4. 触发重连恢复断开的连接
  5. 更新 DHT 中的地址广播
  6. 应用层恢复正常通信

验收标准：
  - 从切换开始到恢复通信 < 5 秒
  - Realm 成员关系保持
  - 进行中的流传输自动恢复或报错重试
```

### 场景 2: 短暂信号丢失

```
前置条件：
  - 用户在电梯/隧道中
  - 网络信号短暂丢失（< 10 秒）

触发：
  - 网络信号恢复

预期行为：
  1. 检测到连接恢复
  2. 在 ToleranceWindow 内恢复
  3. 节点状态保持不变

验收标准：
  - 无需重新加入 Realm
  - 消息队列中的待发消息自动重发
```

### 场景 3: Android 应用切换

```
前置条件：
  - Android 应用在后台
  - 系统检测到网络变化

触发：
  - 应用收到系统网络变化广播
  - 应用调用 node.NetworkChange()

预期行为：
  1. 触发网络变化处理流程
  2. 同场景 1 后续步骤

验收标准：
  - 即使系统无法自动检测，也能正确响应
```

---

## 接口设计

### 用户 API

```go
// Node 级别
type Node interface {
    // NetworkChange 通知节点网络可能已变化
    // 在 Android 等平台上，应用需要在收到系统广播时调用此方法
    NetworkChange()
    
    // OnNetworkChange 注册网络变化回调
    OnNetworkChange(callback func(event NetworkChangeEvent))
}

// NetworkChangeEvent 网络变化事件
type NetworkChangeEvent struct {
    Type      NetworkChangeType  // Major / Minor
    OldAddrs  []string
    NewAddrs  []string
    Timestamp time.Time
}
```

### 配置

```go
type NetworkChangeConfig struct {
    // 抖动容忍配置
    ToleranceWindow       time.Duration  // 默认 5s
    StateHoldTime         time.Duration  // 默认 30s
    ReconnectEnabled      bool           // 默认 true
    InitialReconnectDelay time.Duration  // 默认 1s
    MaxReconnectDelay     time.Duration  // 默认 30s（与概念澄清文档一致）
    MaxReconnectAttempts  int            // 默认 5
    BackoffMultiplier     float64        // 默认 2.0
    BackoffJitter         float64        // 默认 0.2（±20%）
}
```

---

## 竞品参考

| 竞品 | 实现方式 | 特点 |
|------|----------|------|
| **go-dep2p** | JitterTolerance | 抖动容忍 + 指数退避重连 |
| **iroh** | Network Monitor + Rebind | 主动检测 + Socket 重绑定 |
| **libp2p** | Swarm 重连 | 依赖 Identify 协议更新地址 |

---

## 相关文档

| 类型 | 链接 |
|------|------|
| **概念澄清** | [NAT/Relay 概念澄清](../../../_discussions/20260123-nat-relay-concept-clarification.md) |
| **需求** | [REQ-NET-005](REQ-NET-005.md): 网络弹性与恢复 |
| **需求** | [REQ-NET-003](REQ-NET-003.md): Relay 中继 |
| **规范** | [NAT 穿透规范](../../../../02_constraints/protocol/L3_network/nat.md) |
| **架构** | [连接流程](../../../../03_architecture/L3_behavioral/connection_flow.md) |

---

## 变更历史

| 日期 | 版本 | 变更说明 |
|------|------|----------|
| 2026-01-15 | 1.0 | 初始版本 |
| 2026-01-23 | 1.1 | 根据概念澄清文档同步：补充地址更新流程（本地缓存→Relay 地址簿→DHT）、重试策略参数对齐（30s 封顶、±20% jitter） |
