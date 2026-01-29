# 接口层对比分析

> **对比产品**: iroh、go-libp2p、torrent  
> **分析日期**: 2026-01-11

---

## 文档索引

| 文档 | 描述 | 状态 |
|------|------|------|
| [01-api-design.md](01-api-design.md) | API 设计对比 | ✅ |

---

## 分析维度

### 1. API 设计 (01-api-design.md)

- **入口 API 对比**：Endpoint、Host、Client
- **配置方式对比**：Builder、选项函数、配置结构体
- **连接与流 API**：连接管理、流操作
- **事件与通知**：Watcher、Notifiee、Callbacks
- **错误处理**：类型化错误、标准 error
- **高级 API**：发现、路由

---

## 关键对比

### 入口设计

| 产品 | 入口类型 | 特点 |
|------|----------|------|
| **iroh** | `Endpoint` | 简洁、async |
| **go-libp2p** | `Host` | 完整、模块化 |
| **torrent** | `Client` | 专注、简单 |

### 配置方式

```mermaid
flowchart LR
    subgraph Builder[Builder 模式 - iroh]
        B1[builder()] --> B2[.config()] --> B3[.build()]
    end
    
    subgraph Options[选项函数 - libp2p]
        O1["New(opt1, opt2, ...)"]
    end
    
    subgraph Struct[配置结构体 - torrent]
        S1[Config{}] --> S2[NewClient]
    end
```

### 事件通知

| 产品 | 机制 | 特点 |
|------|------|------|
| **iroh** | Watcher | 异步、流式 |
| **go-libp2p** | Notifiee | 同步回调 |
| **torrent** | Callbacks | 同步回调 |

### API 使用示例

#### iroh
```rust
let endpoint = Endpoint::builder()
    .secret_key(key)
    .bind()
    .await?;
```

#### go-libp2p
```go
host, _ := libp2p.New(
    libp2p.Identity(priv),
)
```

#### torrent
```go
client, _ := torrent.NewClient(config)
```

---

## DeP2P 建议

1. 采用 Builder + 选项函数混合配置
2. Channel 模式进行事件通知
3. 类型化错误处理
4. 设计 Realm 感知的 API

---

**更新日期**：2026-01-11
