# API 设计标准

> 定义 DeP2P 公开 API 的设计原则和规范

---

## 设计原则

```
┌─────────────────────────────────────────────────────────────┐
│                    API 设计原则                              │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│  简单性                                                     │
│  ──────                                                     │
│  简单的事情简单做                                            │
│  复杂的事情可以做                                            │
│                                                             │
│  一致性                                                     │
│  ──────                                                     │
│  相似操作使用相似接口                                        │
│                                                             │
│  可发现性                                                   │
│  ────────                                                   │
│  用户能够探索和理解                                          │
│                                                             │
│  安全性                                                     │
│  ──────                                                     │
│  默认安全，难以误用                                          │
│                                                             │
└─────────────────────────────────────────────────────────────┘
```

---

## Context 传递

### 规则

```
Context 使用规则：

  1. 作为第一个参数
  2. 名称使用 ctx
  3. 不存储在结构体中
  4. 用于取消和超时
```

### 示例

```
正确用法伪代码：

  FUNCTION Connect(ctx, nodeID) -> Connection, error
    // ctx 用于超时和取消
    result = await with_context(ctx, dial(nodeID))
    RETURN result
  END
  
错误用法：

  FUNCTION Connect(nodeID, ctx) -> Connection, error
    // ctx 不应该在后面
  END
```

---

## 函数签名

### 参数规则

| 规则 | 说明 |
|------|------|
| Context 第一 | 始终是第一个参数 |
| 必需在前 | 必需参数在可选参数之前 |
| 相关分组 | 相关参数放在一起 |
| 参数个数 | 超过 4 个使用结构体 |

### 返回值规则

| 规则 | 说明 |
|------|------|
| Error 最后 | error 作为最后一个返回值 |
| 单值或指针 | 成功返回值或 nil |
| 不返回 nil error | 成功时 error 必须是 nil |

---

## Option 模式

### 功能选项

```
Option 模式伪代码：

  TYPE Option = FUNCTION(config)
  
  FUNCTION WithTimeout(d) -> Option
    RETURN FUNCTION(c)
      c.timeout = d
    END
  END
  
  FUNCTION WithRetry(n) -> Option
    RETURN FUNCTION(c)
      c.maxRetries = n
    END
  END
  
  FUNCTION NewClient(opts...) -> Client
    config = defaultConfig()
    
    FOR EACH opt IN opts
      opt(config)
    END
    
    RETURN Client{config: config}
  END
```

### 使用场景

| 场景 | 是否使用 Option |
|------|----------------|
| 可选配置 | 是 |
| 必需参数 | 否，使用普通参数 |
| 互斥选项 | 考虑使用多个构造函数 |

---

## 接口设计

### 接口大小

```
接口大小原则：

  小接口优于大接口：
    • 1-3 个方法为佳
    • 易于实现和 Mock
    
  拆分大接口：
    按职责拆分为多个小接口
```

### 接口命名

| 类型 | 命名规则 | 示例 |
|------|----------|------|
| 单方法 | 动词 + er | Reader, Writer |
| 多方法 | 名词 | Connection, Manager |
| 能力接口 | 形容词 | Closeable, Stringer |

---

## 错误返回

### 错误类型

```
错误设计规则：

  哨兵错误：
    公开的可比较错误
    ErrNotMember, ErrTimeout
    
  错误类型：
    携带更多信息的错误
    ConnectionError{Addr, Reason}
    
  包装错误：
    添加上下文的错误
    "connect failed: ..."
```

### 错误检查

```
错误检查方式：

  使用 errors.Is：
    IF errors.Is(err, ErrNotMember) THEN
      // 处理未加入 Realm
    END
    
  使用 errors.As：
    IF errors.As(err, &connErr) THEN
      // 使用 connErr 的额外信息
    END
```

---

## Builder 模式

### 适用场景

```
Builder 适用场景：

  复杂对象构造：
    • 多个必需参数
    • 复杂验证逻辑
    • 分步构造
```

### Builder 结构

```
Builder 模式伪代码：

  TYPE NodeBuilder
    config: NodeConfig
    errors: []error
  
  FUNCTION (b) WithIdentity(key) -> NodeBuilder
    b.config.identity = key
    RETURN b
  END
  
  FUNCTION (b) WithTransport(t) -> NodeBuilder
    b.config.transport = t
    RETURN b
  END
  
  FUNCTION (b) Build() -> Node, error
    IF len(b.errors) > 0 THEN
      RETURN nil, combine(b.errors)
    END
    
    RETURN createNode(b.config)
  END
```

---

## Preset 配置

### 预设模式

```
Preset 模式伪代码：

  TYPE Preset = []Option
  
  PresetDesktop = Preset{
    WithTransport(QUIC),
    WithDiscovery(DHT, mDNS),
    WithRelay(true),
  }
  
  PresetServer = Preset{
    WithTransport(QUIC),
    WithDiscovery(DHT),
    WithRelay(false),
  }
  
  FUNCTION NewNode(preset, opts...) -> Node
    allOpts = append(preset, opts...)
    RETURN createNode(allOpts)
  END
```

### 预设类型

| 预设 | 适用场景 | 特点 |
|------|----------|------|
| Desktop | 桌面应用 | 全功能 |
| Mobile | 移动应用 | 省电优化 |
| Server | 服务端 | 高性能 |

---

## 回调设计

### 回调规则

```
回调设计规则：

  命名：
    On + 事件名
    OnConnect, OnMessage
    
  签名：
    包含必要上下文
    返回 error 表示处理失败
    
  注册：
    Set 表示替换
    Add 表示追加
```

### 回调示例

```
回调使用伪代码：

  TYPE ConnectionHandler = FUNCTION(conn) -> error
  
  FUNCTION (n) OnConnect(handler)
    n.connectHandler = handler
  END
  
  // 内部调用
  FUNCTION (n) handleNewConnection(conn)
    IF n.connectHandler != nil THEN
      err = n.connectHandler(conn)
      IF err != nil THEN
        log.warn("handler error", "error", err)
      END
    END
  END
```

---

## 版本兼容

### 兼容规则

```
API 兼容规则：

  允许的变更：
    • 新增可选参数（Option）
    • 新增方法
    • 新增错误类型
    
  禁止的变更：
    • 删除公开 API
    • 修改函数签名
    • 修改行为语义
```

### 废弃流程

```
API 废弃流程：

  1. 标记 Deprecated
  2. 提供替代 API
  3. 文档说明迁移方式
  4. 下个大版本移除
```

---

## 验证清单

| 检查项 | 说明 |
|--------|------|
| Context 第一 | 第一个参数是 ctx |
| Error 最后 | 最后一个返回值是 error |
| Option 可选 | 可选配置使用 Option |
| 接口小巧 | 接口 1-3 个方法 |

---

## 相关文档

- [代码规范](code_standards.md)
- [命名规范](naming_conventions.md)
- [编码规范](../coding_specs/)

---

**最后更新**：2026-01-11
