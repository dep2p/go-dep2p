# 加入第一个 Realm

本文档将介绍 Realm 的概念以及如何加入你的第一个 Realm。

---

## 什么是 Realm？

Realm 是 DeP2P 的**业务隔离单元**，类似于：

- 聊天应用中的"房间"
- 游戏中的"服务器"
- Kubernetes 的 Namespace

```
┌─────────────────────────────────────────────────────────────────────────┐
│                         Realm 业务隔离                                   │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                          │
│   ┌─────────────┐    ┌─────────────┐    ┌─────────────┐                 │
│   │  Realm A    │    │  Realm B    │    │  Realm C    │                 │
│   │  游戏主服    │    │  聊天室 1   │    │  文件同步    │                 │
│   │             │    │             │    │             │                 │
│   │ ├─ 独立发现  │    │ ├─ 独立发现  │    │ ├─ 独立发现  │                 │
│   │ ├─ 独立消息  │    │ ├─ 独立消息  │    │ ├─ 独立消息  │                 │
│   │ └─ 成员隔离  │    │ └─ 成员隔离  │    │ └─ 成员隔离  │                 │
│   └─────────────┘    └─────────────┘    └─────────────┘                 │
│          │                 │                  │                          │
│          └─────────────────┴──────────────────┘                          │
│                            │                                             │
│   ┌────────────────────────┴────────────────────────────────────────┐   │
│   │              系统基础层（共享 DHT/Relay/NAT）                      │   │
│   └─────────────────────────────────────────────────────────────────┘   │
│                                                                          │
└─────────────────────────────────────────────────────────────────────────┘
```

---

## 为什么需要 Realm？

### 核心原则

1. **业务隔离**  
   不同 Realm 的节点无法互相通信，消息不会混淆。

2. **成员管理**  
   Realm 提供成员列表和加入/离开事件通知。

3. **"连接即成员"原则**  
   与 Realm 成员保持连接 = 成为成员，连接断开 = 失去成员身份。

### 使用场景

- **隔离不同应用的数据**：应用 A 的消息不会被应用 B 收到
- **多房间/多频道**：聊天应用的不同房间
- **简化编程模型**：框架层面保证隔离

---

## 加入 Realm

### 基础用法

```go
package main

import (
    "context"
    "fmt"
    "log"
    
    "github.com/dep2p/go-dep2p"
    "github.com/dep2p/go-dep2p/pkg/types"
)

func main() {
    ctx := context.Background()
    
    // Step 1: 创建并启动节点
    node, err := dep2p.New(ctx, dep2p.WithPreset(dep2p.PresetDesktop))
    if err != nil {
        log.Fatal(err)
    }
    defer node.Close()
    
    if err := node.Start(ctx); err != nil {
        log.Fatal(err)
    }
    
    fmt.Printf("节点 ID: %s\n", node.ID())
    fmt.Printf("当前 Realm: %s\n", node.Realm().CurrentRealm())  // 输出: 空
    
    // Step 2: 加入 Realm
    realm, err := node.Realm("my-chat-room")
    if err != nil {
        log.Fatalf("获取 Realm 失败: %v", err)
    }
    if err := realm.Join(ctx); err != nil {
        log.Fatalf("加入 Realm 失败: %v", err)
    }
    
    fmt.Printf("已加入 Realm: %s\n", node.Realm().CurrentRealm())
    
    // Step 3: 现在可以使用业务 API
    fmt.Println("可以开始通信了！")
}
```

---

## 成员事件

Realm 提供成员加入和离开的实时通知：

```go
// 订阅成员事件
events, err := node.Realm().SubscribeMemberEvents(ctx, realmID)
if err != nil {
    log.Fatal(err)
}

go func() {
    for event := range events {
        switch event.Type {
        case dep2p.MemberJoined:
            fmt.Printf("成员加入: %s\n", event.Member.ShortString())
        case dep2p.MemberLeft:
            fmt.Printf("成员离开: %s\n", event.Member.ShortString())
        }
    }
}()

// 获取当前成员列表
members := node.Realm().Members(realmID)
fmt.Printf("当前成员数: %d\n", len(members))
```

---

## "连接即成员"原则

DeP2P 采用"连接即成员"设计：

```
┌─────────────────────────────────────────────────────────────────────────┐
│                    "连接即成员"原则                                      │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│  连接建立 ────────► 成为成员 ────────► 收到 MemberJoined 事件           │
│                                                                         │
│  连接断开 ────────► 失去成员身份 ────► 收到 MemberLeft 事件             │
│                                                                         │
│  优点：                                                                 │
│  • 无需显式"加入/退出"协议                                              │
│  • 连接状态与成员状态自动同步                                           │
│  • 断开即离开，自动清理                                                 │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

### 断开检测配置

可以调整断开检测参数来控制成员状态更新的灵敏度：

```json
{
  "disconnect_detection": {
    "quic": {
      "keep_alive_period": "3s",
      "max_idle_timeout": "6s"
    },
    "reconnect_grace_period": "15s"
  }
}
```

- **keep_alive_period**: QUIC 保活探测间隔
- **max_idle_timeout**: 最大空闲超时
- **reconnect_grace_period**: 重连宽限期，期间不触发 MemberLeft

---

## Realm 切换

**严格单 Realm**：节点同时只能在一个 Realm 中。

```go
// 加入 room-1
realm1, _ := node.Realm("room-1")
_ = realm1.Join(ctx)
fmt.Println(node.Realm().CurrentRealm())  // room-1

// 尝试直接切换到 room-2（会失败）
realm2, _ := node.Realm("room-2")
err = realm2.Join(ctx)
// err == ErrAlreadyJoined

// 正确做法：先 Leave 再 Join
realm1.Leave(ctx)
realm2, _ = node.Realm("room-2")
_ = realm2.Join(ctx)
fmt.Println(node.Realm().CurrentRealm())  // room-2
```

---

## 常见错误

### ErrNotMember

未加入 Realm 就调用业务 API：

```go
// ❌ 错误：未加入 Realm
node, _ := dep2p.New(ctx, dep2p.WithPreset(dep2p.PresetDesktop))
node.Start(ctx)
err := node.PubSub().Publish(ctx, "topic", data)
// err == ErrNotMember

// ✅ 正确：先加入 Realm
node, _ := dep2p.New(ctx, dep2p.WithPreset(dep2p.PresetDesktop))
node.Start(ctx)
realm, _ := node.Realm("my-realm")
_ = realm.Join(ctx)
err := node.PubSub().Publish(ctx, "topic", data)
// err == nil
```

### ErrAlreadyJoined

已在某个 Realm 中，又尝试 Join：

```go
// ❌ 错误
realm1, _ := node.Realm("realm-1")
_ = realm1.Join(ctx)
realm2, _ := node.Realm("realm-2")
err := realm2.Join(ctx)
// err == ErrAlreadyJoined

// ✅ 正确
realm1, _ := node.Realm("realm-1")
_ = realm1.Join(ctx)
realm1.Leave(ctx)  // 先离开
realm2, _ = node.Realm("realm-2")
err = realm2.Join(ctx)
// err == nil
```

---

## 完整示例

```go
package main

import (
    "context"
    "fmt"
    "log"
    
    "github.com/dep2p/go-dep2p"
    "github.com/dep2p/go-dep2p/pkg/types"
)

func main() {
    ctx := context.Background()
    
    // 创建并启动节点
    node, err := dep2p.New(ctx, dep2p.WithPreset(dep2p.PresetDesktop))
    if err != nil {
        log.Fatal(err)
    }
    defer node.Close()
    
    if err := node.Start(ctx); err != nil {
        log.Fatal(err)
    }
    
    rm := node.Realm()
    
    // 演示 Realm 生命周期
    fmt.Println("=== Realm 生命周期演示 ===")
    
    // 1. 初始状态
    fmt.Printf("1. 初始 Realm: '%s'\n", rm.CurrentRealm())
    
    // 2. 加入 Realm
    realm, err := node.Realm("demo-realm")
    if err != nil {
        log.Fatal(err)
    }
    if err := realm.Join(ctx); err != nil {
        log.Fatal(err)
    }
    fmt.Printf("2. 加入后: '%s'\n", rm.CurrentRealm())
    
    // 3. 订阅成员事件
    events, _ := rm.SubscribeMemberEvents(ctx, realmID)
    go func() {
        for event := range events {
            switch event.Type {
            case dep2p.MemberJoined:
                fmt.Printf("   [事件] 成员加入: %s\n", event.Member.ShortString())
            case dep2p.MemberLeft:
                fmt.Printf("   [事件] 成员离开: %s\n", event.Member.ShortString())
            }
        }
    }()
    
    // 4. 获取成员列表
    members := rm.Members(realmID)
    fmt.Printf("3. 当前成员数: %d\n", len(members))
    
    // 5. 尝试重复加入（会失败）
    realm2, _ := node.Realm("another-realm")
    err = realm2.Join(ctx)
    fmt.Printf("4. 重复加入结果: %v\n", err)
    
    // 6. 离开 Realm
    if err := realm.Leave(ctx); err != nil {
        log.Fatal(err)
    }
    fmt.Printf("5. 离开后: '%s'\n", rm.CurrentRealm())
    
    // 7. 加入新 Realm
    realm3, err := node.Realm("new-realm")
    if err != nil {
        log.Fatal(err)
    }
    if err := realm3.Join(ctx); err != nil {
        log.Fatal(err)
    }
    fmt.Printf("6. 新 Realm: '%s'\n", rm.CurrentRealm())
}
```

输出：

```
=== Realm 生命周期演示 ===
1. 初始 Realm: ''
2. 加入后: 'demo-realm'
3. 当前成员数: 1
4. 重复加入结果: already joined a realm
5. 离开后: ''
6. 新 Realm: 'new-realm'
```

---

## 下一步

- [常见问题](faq.md) - 更多问题解答
- [Realm 群聊教程](../tutorials/04-realm-chat.md) - 构建群聊应用
- [核心概念](../concepts/core-concepts.md) - 深入理解 Realm
