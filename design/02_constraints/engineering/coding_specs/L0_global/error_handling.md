# 错误处理规范

> 定义 DeP2P 项目的错误处理模式和最佳实践

---

## 核心原则

### 错误不可忽略

```
┌─────────────────────────────────────────┐
│            错误处理原则                  │
├─────────────────────────────────────────┤
│                                         │
│  1. 所有错误必须处理                     │
│  2. 忽略时必须显式说明                   │
│  3. 错误信息要有上下文                   │
│  4. 错误链要可追溯                       │
│                                         │
└─────────────────────────────────────────┘
```

---

## 错误定义

### 领域错误

每个包应定义自己的领域错误：

```
错误定义模式：
  
  ErrXxx = errors.New("简短描述")
  
示例：
  ErrConnectionRefused  = "connection refused"
  ErrRealmNotJoined     = "realm not joined"
  ErrIdentityMismatch   = "identity mismatch"
```

### 错误分类

| 类别 | 命名模式 | 用途 |
|------|----------|------|
| 参数错误 | ErrInvalidXxx | 输入验证失败 |
| 状态错误 | ErrNotXxx/ErrAlreadyXxx | 状态不满足 |
| 资源错误 | ErrXxxNotFound/ErrXxxExists | 资源问题 |
| 操作错误 | ErrXxxFailed | 操作失败 |

---

## 错误包装

### 包装规则

```
错误包装伪代码：

  WHEN error occurs
    IF need_context THEN
      RETURN wrap(error, "context: operation failed")
    ELSE
      RETURN error
    END
  END
```

### 上下文添加

| 场景 | 是否包装 | 示例 |
|------|----------|------|
| 边界调用 | 包装 | 调用外部包时 |
| 内部传递 | 视情况 | 已有足够上下文则不包装 |
| 直接返回 | 不包装 | 错误已足够清晰 |

### 包装格式

```
包装格式规范：
  
  "操作描述: 原因"
  
示例：
  "connect to peer: connection refused"
  "join realm abc: not authorized"
  "send message: stream closed"
```

---

## 错误检查

### 类型检查

```
错误检查伪代码：

  IF errors.Is(err, ErrNotMember) THEN
    handle_not_member()
  END
  
  IF errors.As(err, &connError) THEN
    handle_connection_error(connError)
  END
```

### 检查优先级

```
┌─────────────────────────────────────────┐
│            错误检查优先级                │
├─────────────────────────────────────────┤
│                                         │
│  1. errors.Is() — 检查特定错误          │
│  2. errors.As() — 检查错误类型          │
│  3. 字符串比较  — 最后手段              │
│                                         │
└─────────────────────────────────────────┘
```

---

## Panic 使用

### 使用场景

| 场景 | 是否 Panic | 说明 |
|------|------------|------|
| 程序 bug | 是 | 不应发生的情况 |
| 初始化失败 | 是 | 无法恢复 |
| 外部输入错误 | 否 | 返回 error |
| 资源不可用 | 否 | 返回 error |

### Panic 规则

```
Panic 使用规则：

  • 只在"不可能发生"的情况使用
  • 只在初始化阶段使用
  • 公开 API 绝不 panic
  • 必须有 recover 保护
```

---

## 错误恢复

### Recover 使用

```
Recover 使用伪代码：

  DEFER
    IF panic_occurred THEN
      log_error(panic_info)
      convert_to_error()
    END
  END
```

### Goroutine 保护

```
Goroutine 错误处理：

  GO FUNC
    DEFER recover_and_log()
    
    do_work()
  END
```

---

## 错误日志

### 日志级别

| 错误类型 | 日志级别 | 示例 |
|----------|----------|------|
| 临时错误 | Debug | 网络抖动 |
| 可恢复错误 | Warn | 重试成功 |
| 业务错误 | Info | 用户操作失败 |
| 系统错误 | Error | 无法恢复 |

### 日志内容

```
错误日志应包含：

  • 错误消息
  • 相关上下文（peer ID、地址等）
  • 错误链（完整堆栈）
```

---

## P2P 特有错误

### 连接错误

| 错误 | 处理方式 |
|------|----------|
| ErrConnectionRefused | 标记节点不可达，稍后重试 |
| ErrIdentityMismatch | 关闭连接，记录告警 |
| ErrTimeout | 重试或切换路径 |

### Realm 错误

| 错误 | 处理方式 |
|------|----------|
| ErrNotMember | 提示用户加入 Realm |
| ErrAlreadyJoined | 忽略或提示 |
| ErrRealmNotFound | 检查 Realm 名称 |

### 协议错误

| 错误 | 处理方式 |
|------|----------|
| ErrProtocolNotSupported | 协商备选协议 |
| ErrVersionMismatch | 降级或拒绝 |
| ErrMalformedMessage | 关闭流，记录告警 |

### Relay 特定错误（来自实测验证）

> 以下错误类型来自 2026-01-22 Bootstrap/Relay 拆分部署测试

| 错误 | 处理方式 | 来源 BUG |
|------|----------|---------|
| ErrProtocolNegotiationEOF | 检查 STOP 流处理是否交给 Host | BUG-13 |
| ErrRelayProtocolNotSupported | 检查 Relay 协议处理器是否注册 | BUG-7 |
| ErrNoRelayAddresses | 检查地址是否存入 Peerstore | BUG-6 |
| ErrRelayNotConfigured | 检查 Manager 是否注入 Swarm | BUG-A |
| ErrRelayDHTEmpty | 检查 Relay 是否连接 Bootstrap | BUG-2 |

### Relay 错误诊断指南

```
Relay 错误诊断伪代码：

  FUNCTION diagnose_relay_error(err)
    SWITCH err
      CASE ErrProtocolNegotiationEOF:
        log.error("协议协商 EOF",
          "hint", "检查 STOP 处理器是否调用 host.HandleInboundStream()")
        
      CASE ErrRelayProtocolNotSupported:
        log.error("Relay 协议不支持",
          "hint", "检查 server.SetHost() 是否在 Start() 之前调用")
        
      CASE ErrNoRelayAddresses:
        log.error("无 Relay 地址",
          "hint", "检查 RelayAddr 是否写入 Peerstore")
        
      CASE ErrRelayNotConfigured:
        log.error("Relay 未配置",
          "hint", "检查 Manager 是否注入 Relay Dialer")
        
      CASE ErrRelayDHTEmpty:
        log.error("Relay 节点 DHT 为空",
          "hint", "检查 Relay 是否配置 Bootstrap peers")
    END
  END
```

---

## 验证清单

| 检查项 | 说明 |
|--------|------|
| 无忽略的错误 | 除非显式 _ = err |
| 错误有上下文 | 包装时添加操作描述 |
| 使用 errors.Is/As | 而非字符串比较 |
| 公开 API 不 panic | 返回 error |

---

## 相关文档

- [日志规范](logging.md)
- [ADR-0001](../../../../01_context/decisions/ADR-0001-identity-first.md): ErrIdentityMismatch
- [INV-002](../../../../01_context/decisions/invariants/INV-002-realm-membership.md): ErrNotMember
- [Relay 中继规范](../../../protocol/L2_transport/relay.md)
- [拆分部署测试计划](../../../../_discussions/20260122-split-infra-test-plan.md)

---

**最后更新**：2026-01-23
