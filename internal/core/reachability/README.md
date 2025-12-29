# Reachability 可达性协调模块

## 概述

可达性协调模块实现"可达性优先（Reachability-First）"策略，统一管理来自 NAT、Relay、AddressManager 的地址发布。

**核心设计理念**：**先保证连得上（Relay 兜底），再争取直连更优路径**

## 职责

1. **地址聚合**：统一收集来自不同来源的地址
2. **优先级排序**：按可达性和性能排序地址
3. **变更通知**：在地址列表变化时通知订阅者
4. **生命周期管理**：协调各地址来源的启动/停止

## 地址优先级

| 优先级 | 地址类型 | 来源 | 说明 |
|--------|----------|------|------|
| 100 | 已验证直连地址 | UPnP/NAT-PMP 端口映射 + 可达性验证 | 延迟最低，优先使用 |
| 80 | STUN 反射地址 | STUN 查询 + 可达性验证 | 可用于对称 NAT 穿透 |
| 50 | Relay 中继地址 | AutoRelay 预留 | 始终保留，保证可达 |
| 10 | 本地监听地址 | 本地绑定 | 仅在无其他地址时使用 |
| 0 | 未验证候选地址 | 推断 | 不发布 |

## 组件

### Coordinator

可达性协调器，核心组件：

```go
coordinator := reachability.NewCoordinator(nat, autoRelay, addrManager)

// 获取按优先级排序的通告地址
addrs := coordinator.AdvertisedAddrs()

// 注册地址变更回调
coordinator.SetOnAddressChanged(func(addrs []endpoint.Address) {
    // 地址列表变化时触发
})

// Relay 预留成功时调用
coordinator.OnRelayReserved(relayAddrs)

// 直连地址候选上报（未验证，不发布）
coordinator.OnDirectAddressCandidate(addr, "port-mapping", address.PriorityUnverified)

// dial-back 验证通过后，才标记为 VerifiedDirect 并发布
coordinator.OnDirectAddressVerified(addr, "dial-back", address.PriorityVerifiedDirect)
```

## 与其他模块的集成

### NAT 模块

- 发现直连地址候选
- 端口映射成功后通知 Coordinator

### Relay 模块

- AutoRelay 预留成功后通知 Coordinator
- Relay 地址始终保留在发布列表中

### Endpoint 模块

- 调用 `Coordinator.AdvertisedAddrs()` 获取统一的通告地址
- 在发现服务中使用聚合后的地址

## 未来规划

- [x] 实现 `/dep2p/sys/reachability/1.0.0` 回拨验证协议（Handshake 判定）
- [ ] 增强观测地址机制（Observed Address）
- [ ] 支持地址置信度评估

## 相关文档

- [地址管理协议](../../../docs/01-design/protocols/network/04-address-management.md)
- [NAT 穿透协议](../../../docs/01-design/protocols/network/02-nat.md)
- [Relay 协议](../../../docs/01-design/protocols/network/03-relay.md)

