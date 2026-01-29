# 日志规范

> 定义 DeP2P 项目的日志记录标准

---

## 核心原则

```
┌─────────────────────────────────────────┐
│              日志原则                    │
├─────────────────────────────────────────┤
│                                         │
│  1. 结构化日志优先                       │
│  2. 日志级别要准确                       │
│  3. 敏感信息要脱敏                       │
│  4. 性能影响要考虑                       │
│                                         │
└─────────────────────────────────────────┘
```

---

## 日志级别

### 级别定义

| 级别 | 用途 | 生产环境 |
|------|------|----------|
| **Debug** | 调试信息、详细流程 | 关闭 |
| **Info** | 正常业务事件 | 开启 |
| **Warn** | 可恢复的异常 | 开启 |
| **Error** | 需要关注的错误 | 开启 |
| **Fatal** | 致命错误，程序退出 | 开启 |

### 级别选择

```
日志级别选择：

  Debug：
    • 函数入口/出口
    • 详细的状态变化
    • 临时调试信息
    
  Info：
    • 服务启动/停止
    • 连接建立/断开
    • 重要业务事件
    
  Warn：
    • 重试成功
    • 降级处理
    • 非预期但可处理
    
  Error：
    • 操作失败
    • 无法恢复
    • 需要人工介入
```

---

## 结构化日志

### 格式要求

```
日志格式：
  
  时间戳 | 级别 | 消息 | 结构化字段
  
示例（伪代码）：
  log.Info("connection established",
    "peer", peerID,
    "addr", addr,
    "latency_ms", latency)
```

### 字段命名

| 类型 | 命名规则 | 示例 |
|------|----------|------|
| 标识符 | 名词 | peer, node, stream |
| 数值 | 名词_单位 | latency_ms, size_bytes |
| 布尔 | is_/has_ | is_relay, has_direct |
| 错误 | error | error |

---

## 上下文传递

### Logger 注入

```
Logger 传递方式：

  方式1：结构体字段
    Manager 包含 logger 字段
    
  方式2：Context 携带
    从 context 中获取 logger
```

### 字段继承

```
字段继承伪代码：

  baseLogger = logger.With("component", "transport")
  
  connLogger = baseLogger.With("conn_id", connID)
  
  streamLogger = connLogger.With("stream_id", streamID)
```

---

## P2P 特有日志

### 连接日志

```
连接相关日志：

  Info:
    "connection established"    peer, addr, path_type
    "connection closed"         peer, reason, duration
    
  Debug:
    "attempting connection"     peer, addrs
    "path upgraded"            peer, old_path, new_path
    
  Warn:
    "connection failed"        peer, addr, error
    "reconnecting"             peer, attempt, delay
```

### Realm 日志

```
Realm 相关日志：

  Info:
    "realm joined"             realm, member_count
    "realm left"               realm, reason
    
  Debug:
    "realm announcement"       realm, source
    
  Warn:
    "realm join failed"        realm, error
```

### 协议日志

```
协议相关日志：

  Debug:
    "protocol negotiated"      peer, protocol, version
    "message received"         peer, protocol, size
    "message sent"             peer, protocol, size
    
  Warn:
    "protocol not supported"   peer, protocol
```

---

## 敏感信息处理

### 脱敏规则

| 信息类型 | 处理方式 | 示例 |
|----------|----------|------|
| 私钥 | 不记录 | - |
| PSK | 不记录 | - |
| 完整地址 | 部分隐藏 | 192.168.*.* |
| NodeID | 可记录 | 完整或缩写 |
| 消息内容 | 仅记录大小 | size_bytes |

### 脱敏示例

```
脱敏处理伪代码：

  FUNCTION log_peer(peer)
    IF production THEN
      RETURN peer.short_id()    // 前8字符
    ELSE
      RETURN peer.full_id()
    END
  END
```

---

## 性能考虑

### 避免高开销

| 操作 | 影响 | 建议 |
|------|------|------|
| 高频日志 | CPU、内存 | 使用 Debug 级别 |
| 大对象序列化 | CPU | 延迟计算 |
| 字符串拼接 | 内存分配 | 使用结构化字段 |

### 采样策略

```
高频日志采样：

  IF message_rate > threshold THEN
    sample_log(rate = 1%)
  ELSE
    log_all()
  END
```

---

## 日志输出

### 输出目标

| 环境 | 输出 | 格式 |
|------|------|------|
| 开发 | 控制台 | 人类可读 |
| 生产 | 文件/采集 | JSON |

### 轮转策略

```
日志轮转：
  
  • 按大小：100MB
  • 按时间：每天
  • 保留：7天
```

---

## 验证清单

| 检查项 | 说明 |
|--------|------|
| 级别正确 | 不滥用 Error |
| 结构化 | 使用字段而非格式化字符串 |
| 无敏感信息 | 私钥、PSK 不记录 |
| 有上下文 | 包含必要标识符 |

---

## 相关文档

- [错误处理](error_handling.md)
- [隔离约束](../../isolation/)

---

**最后更新**：2026-01-11
