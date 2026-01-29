# Protocol PubSub 设计概览

> **组件**: P6-02 protocol_pubsub  
> **协议**: GossipSub

---

## 架构设计

```
Service (interfaces.PubSub)
  ├── gossipSub (核心协议)
  │   ├── mesh (D-regular图)
  │   ├── heartbeat (维护)
  │   ├── messageCache (缓存)
  │   └── seenMessages (去重)
  └── validator (验证器)
```

---

## GossipSub协议

### Mesh维护
- D: 目标度数(6)
- Dlo/Dhi: 度数范围(4-12)
- GRAFT/PRUNE: 节点加入/离开

### 消息传播
- Mesh内全量转发
- 消息去重(seenMessages)
- 消息缓存(messageCache)

### 心跳机制
- 周期性维护Mesh
- 清理过期消息

---

## Realm集成

- 成员验证
- 协议ID: `/dep2p/app/<realmID>/pubsub/1.0.0`

---

**最后更新**: 2026-01-14
