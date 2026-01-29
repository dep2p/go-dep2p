# Protocol Streams 设计概览

> **组件**: P6-03 protocol_streams  
> **协议**: 双向流

---

## 架构设计

```
Service (interfaces.Streams)
  ├── streamWrapper (interfaces.BiStream)
  │   └── interfaces.Stream (Host层)
  ├── handlers (协议处理器注册表)
  └── realmMgr (协议ID生成)
```

---

## 流包装机制

### Host层流 → BiStream适配
- Host层提供 `interfaces.Stream`
- Streams层提供 `interfaces.BiStream`
- `streamWrapper` 负责适配和增强

### 增强功能
- 统计信息
- 协议管理
- 并发安全

---

## Realm集成

### 协议ID生成
- 格式: `/dep2p/app/<realmID>/streams/<protocol>/1.0.0`
- 示例: `/dep2p/app/my-realm/streams/file-transfer/1.0.0`

### 成员验证
- 通过 RealmManager 检查成员资格
- 支持多Realm环境

---

## 处理器注册

### 用户视角
```go
streams.RegisterHandler("my-protocol", handler)
```

### 内部实现
```go
host.SetStreamHandler(
    "/dep2p/app/<realmID>/streams/my-protocol/1.0.0",
    wrapHandler(handler)
)
```

---

**最后更新**: 2026-01-14
