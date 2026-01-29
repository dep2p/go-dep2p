# 命名规范

> 定义 DeP2P 项目的命名约定

---

## 命名原则

```
┌─────────────────────────────────────────────────────────────┐
│                    命名原则                                  │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│  清晰性                                                     │
│  ──────                                                     │
│  名称应该清楚表达意图                                        │
│                                                             │
│  一致性                                                     │
│  ──────                                                     │
│  相同概念使用相同名称                                        │
│                                                             │
│  简洁性                                                     │
│  ──────                                                     │
│  在不失清晰的前提下尽量简短                                  │
│                                                             │
│  可搜索                                                     │
│  ──────                                                     │
│  便于在代码库中搜索                                          │
│                                                             │
└─────────────────────────────────────────────────────────────┘
```

---

## 包命名

### 基本规则

| 规则 | 说明 | 示例 |
|------|------|------|
| 小写 | 全小写，无下划线 | transport, discovery |
| 简短 | 一个单词最佳 | relay, identity |
| 名词 | 使用名词 | connection, stream |
| 避免通用 | 不用 util, common | - |

### 包命名模式

```
包命名模式：

  核心功能包：
    transport, network, protocol, security
    
  工具包：
    xxxutil（如 addrutil）
    
  接口包：
    interfaces/xxx（如 interfaces/relay）
    
  类型包：
    types（公开类型）
```

---

## 文件命名

### 命名规则

| 文件类型 | 命名规则 | 示例 |
|----------|----------|------|
| 实现文件 | 小写+下划线 | connection_manager.go |
| 测试文件 | xxx_test.go | connection_manager_test.go |
| 模块入口 | module.go | module.go |
| 选项文件 | options.go | options.go |
| 错误定义 | errors.go | errors.go |

### 特殊文件

| 文件 | 用途 |
|------|------|
| doc.go | 包文档 |
| module.go | fx 模块定义 |
| options.go | Option 定义 |
| errors.go | 错误定义 |

---

## 类型命名

### 结构体

```
结构体命名规则：

  格式：
    大驼峰（PascalCase）
    
  示例：
    ConnectionManager
    StreamHandler
    RelayClient
    
  避免：
    connection_manager（下划线）
    connectionManager（不导出时可用）
```

### 接口

| 类型 | 命名规则 | 示例 |
|------|----------|------|
| 单方法 | 动词+er | Reader, Closer |
| 多方法 | 名词 | Connection, Transport |
| 能力 | 形容词 | Closeable |

---

## 变量命名

### 规则

```
变量命名规则：

  局部变量：
    短名，1-3 个字母
    i, j, n, err, ctx, conn
    
  包级变量：
    驼峰，描述性
    defaultTimeout, maxRetries
    
  导出变量：
    大驼峰
    DefaultTimeout, MaxRetries
```

### 常见缩写

| 缩写 | 全称 | 使用场景 |
|------|------|----------|
| ctx | context | Context 变量 |
| err | error | 错误变量 |
| conn | connection | 连接变量 |
| req | request | 请求变量 |
| resp | response | 响应变量 |
| cfg | config | 配置变量 |

---

## 常量命名

### 规则

| 类型 | 命名规则 | 示例 |
|------|----------|------|
| 私有常量 | 驼峰 | maxRetries |
| 导出常量 | 大驼峰 | MaxRetries |
| 枚举值 | 类型前缀 | StatusConnected |

### 枚举模式

```
枚举命名模式：

  类型定义：
    TYPE ConnectionStatus int
    
  枚举值：
    CONST (
      StatusDisconnected ConnectionStatus = iota
      StatusConnecting
      StatusConnected
      StatusClosing
    )
```

---

## 函数命名

### 规则

| 类型 | 命名规则 | 示例 |
|------|----------|------|
| 构造函数 | New + 类型 | NewManager |
| 获取器 | 名词 | Connection, Peers |
| 设置器 | Set + 属性 | SetTimeout |
| 布尔返回 | Is/Has/Can | IsConnected |
| 动作 | 动词 | Connect, Send |

### 方法命名

```
方法命名规则：

  接收者名：
    1-2 个字母，类型首字母
    (m *Manager), (c *Connection)
    
  方法名：
    动词开头（动作）
    名词开头（属性访问）
```

---

## 协议 ID 命名

### 格式

```
协议 ID 格式：

  /dep2p/{domain}/{protocol}/{version}
  
  domain:
    sys - 系统协议
    realm/<id> - Realm 协议
    app/<id> - 应用协议
    
  protocol:
    小写，连字符分隔
    identify, holepunch, relay-control
    
  version:
    语义版本
    1.0.0, 2.1.0
```

### 示例

| 协议 | ID |
|------|-----|
| 身份识别 | /dep2p/sys/identify/1.0.0 |
| 打洞 | /dep2p/sys/holepunch/1.0.0 |
| Realm 聊天 | /dep2p/realm/xxx/chat/1.0.0 |

> **特例**：Relay 的 HOP/STOP 使用 `/dep2p/relay/1.0.0/{hop,stop}`，用于兼容 Circuit v2 流协议。

---

## 命名空间规范

除协议 ID 外，DeP2P 还定义了其他类型的命名空间：

| 命名空间类型 | 用途 | 格式示例 |
|--------------|------|----------|
| **协议 ID** | 流协商 | `/dep2p/sys/identify/1.0.0` |
| **DHT Key** | 分布式存储 | `/dep2p/v2/realm/<H(RealmID)>/peer/<NodeID>` |
| **PubSub Topic** | 消息广播 | `/dep2p/realm/<RealmID>/members` |
| **Rendezvous** | 服务发现 | `/dep2p/rendezvous/<namespace>` |

### 关键约束

- DHT Key 中的 RealmID **必须哈希**（防止热点）
- PubSub Topic 使用**原始 RealmID**（便于路由）
- 两者格式不同是刻意设计

详见：[命名空间规范](../../protocol/namespace.md)

---

## 错误命名

### 规则

```
错误命名规则：

  哨兵错误：
    Err + 描述
    ErrNotMember, ErrTimeout
    
  错误类型：
    描述 + Error
    ConnectionError, ProtocolError
```

### 常见错误命名

| 错误 | 命名 |
|------|------|
| 未找到 | ErrNotFound, ErrXxxNotFound |
| 已存在 | ErrAlreadyExists, ErrAlreadyXxx |
| 无效 | ErrInvalid, ErrInvalidXxx |
| 超时 | ErrTimeout, ErrXxxTimeout |
| 拒绝 | ErrRefused, ErrXxxRefused |

---

## 测试命名

### 测试函数

```
测试函数命名：

  格式：
    Test[被测对象]_[场景]_[预期]
    
  示例：
    TestConnect_ValidPeer_Success
    TestConnect_InvalidAddr_ReturnsError
    TestJoinRealm_AlreadyJoined_ReturnsError
```

### 子测试

```
子测试命名：

  简短描述场景
  "valid peer"
  "nil context"
  "timeout exceeded"
```

---

## 验证清单

| 检查项 | 说明 |
|--------|------|
| 包名小写 | 无大写和下划线 |
| 类型大驼峰 | 导出类型 |
| 接口有意义 | 符合 -er 或名词规则 |
| 缩写一致 | 使用约定缩写 |

---

## 相关文档

- [代码规范](code_standards.md)
- [API 设计](api_standards.md)
- [编码规范](../coding_specs/L0_global/code_style.md)
- [命名空间规范](../../protocol/namespace.md)

---

**最后更新**：2026-01-27
