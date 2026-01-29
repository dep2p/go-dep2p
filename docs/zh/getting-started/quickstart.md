# 5 分钟上手

本文档将帮助你在 5 分钟内运行第一个 DeP2P 节点并完成基本通信。

---

## 核心理念

```
┌─────────────────────────────────────────────────────────────────────┐
│                    DeP2P 核心理念                                    │
├─────────────────────────────────────────────────────────────────────┤
│                                                                      │
│   让 P2P 成为世界级基础设施：给一个 NodeID，就能跨越网络边界            │
│                                                                      │
│   • 用户只需关心「连接谁」，不需关心「地址在哪」                        │
│   • 复杂的网络细节（NAT 穿透、地址发现、连接管理）由库自动处理          │
│   • 简单场景简单用，复杂场景可扩展                                    │
│                                                                      │
└─────────────────────────────────────────────────────────────────────┘
```

---

## 三步走流程

```
┌─────────────────────────────────────────────────────────────────────────┐
│                      DeP2P 三步走                                        │
├─────────────────────────────────────────────────────────────────────────┤
│  Step 1: New() + Start()   创建并启动节点                                │
│  Step 2: JoinRealm()       加入业务网络                                  │
│  Step 3: Send/Publish      使用业务 API 进行通信                         │
└─────────────────────────────────────────────────────────────────────────┘
```

---

## 最简示例

### 创建并启动节点

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
        log.Fatalf("创建节点失败: %v", err)
    }
    defer node.Close()

    if err := node.Start(ctx); err != nil {
        log.Fatalf("启动节点失败: %v", err)
    }
    
    // 打印节点信息
    fmt.Printf("节点 ID: %s\n", node.ID())
    fmt.Printf("监听地址: %v\n", node.ListenAddrs())
    
    // Step 2: 获取 Realm 并加入（业务网络）
    realm, err := node.Realm("my-first-realm")
    if err != nil {
        log.Fatalf("获取 Realm 失败: %v", err)
    }
    if err := realm.Join(ctx); err != nil {
        log.Fatalf("加入 Realm 失败: %v", err)
    }
    fmt.Printf("已加入 Realm: %s\n", realm.ID())
    
    // Step 3: 现在可以使用业务 API
    messaging := realm.Messaging()
    _ = messaging // 可用于发送消息
    fmt.Println("节点已就绪，可以开始通信！")
}
```

运行：

```bash
go run main.go
# 输出:
# 节点 ID: 12D3KooWxxxxx...  (Base58 编码的公钥)
# 监听地址: [/ip4/0.0.0.0/udp/xxxxx/quic-v1]
# 已加入 Realm: my-first-realm
# 节点已就绪，可以开始通信！
```

---

## 两节点通信（使用 known_peers）

如果你有两台设备或两个终端，可以快速测试节点间通信。

### 节点 A（接收方）

```go
package main

import (
    "context"
    "fmt"
    "log"
    
    "github.com/dep2p/go-dep2p"
)

const echoProtocol = "/echo/1.0.0"

func main() {
    ctx := context.Background()
    
    // 创建节点（固定端口便于连接）
    node, _ := dep2p.New(ctx,
        dep2p.WithPreset(dep2p.PresetMinimal),
        dep2p.WithListenPort(8001),
    )
    _ = node.Start(ctx)
    defer node.Close()
    
    // 注册 Echo 处理器
    node.Endpoint().SetProtocolHandler(echoProtocol, func(stream dep2p.Stream) {
        defer stream.Close()
        buf := make([]byte, 256)
        n, _ := stream.Read(buf)
        fmt.Printf("收到: %s\n", buf[:n])
        stream.Write([]byte("Echo: " + string(buf[:n])))
    })
    
    // 打印连接信息
    fmt.Printf("节点 A 已启动\n")
    fmt.Printf("PeerID: %s\n", node.ID())
    fmt.Printf("地址: %v\n", node.ListenAddrs())
    
    // 等待
    select {}
}
```

### 节点 B（发送方）

```go
package main

import (
    "context"
    "fmt"
    "log"
    "os"
    "time"
    
    "github.com/dep2p/go-dep2p"
    "github.com/dep2p/go-dep2p/config"
    "github.com/dep2p/go-dep2p/pkg/types"
)

const echoProtocol = "/echo/1.0.0"

func main() {
    if len(os.Args) < 3 {
        fmt.Println("用法: go run main.go <PeerID> <地址>")
        fmt.Println("示例: go run main.go 12D3KooW... /ip4/127.0.0.1/udp/8001/quic-v1")
        os.Exit(1)
    }
    peerID, addr := os.Args[1], os.Args[2]
    
    ctx := context.Background()
    
    // 使用 known_peers 直接连接节点 A
    node, _ := dep2p.New(ctx,
        dep2p.WithPreset(dep2p.PresetMinimal),
        dep2p.WithKnownPeers(config.KnownPeer{
            PeerID: peerID,
            Addrs:  []string{addr},
        }),
    )
    _ = node.Start(ctx)
    defer node.Close()
    
    // 等待连接建立
    time.Sleep(2 * time.Second)
    
    // 发送消息
    targetID, _ := types.ParseNodeID(peerID)
    stream, _ := node.OpenStream(ctx, targetID, echoProtocol)
    defer stream.Close()
    
    stream.Write([]byte("Hello, DeP2P!"))
    
    buf := make([]byte, 256)
    n, _ := stream.Read(buf)
    fmt.Printf("响应: %s\n", buf[:n])
}
```

### 运行测试

```bash
# 终端 1: 启动节点 A
go run node_a/main.go

# 终端 2: 启动节点 B（使用节点 A 的信息）
go run node_b/main.go 12D3KooW... /ip4/127.0.0.1/udp/8001/quic-v1

# 节点 A 输出: 收到: Hello, DeP2P!
# 节点 B 输出: 响应: Echo: Hello, DeP2P!
```

---

## 连接方式对比

DeP2P 支持多种节点发现和连接方式：

| 方式 | 配置 | 适用场景 |
|------|------|----------|
| **known_peers** | `WithKnownPeers()` | 已知节点地址，直接连接 |
| **mDNS** | 默认启用 | 同一局域网自动发现 |
| **Bootstrap** | `WithBootstrapPeers()` | 公网 DHT 发现 |
| **手动连接** | `ConnectToAddr()` | 调试和测试 |

```go
// 1. known_peers: 启动即连接
node, _ := dep2p.New(ctx, dep2p.WithKnownPeers(
    config.KnownPeer{PeerID: "12D3KooW...", Addrs: []string{"/ip4/1.2.3.4/udp/4001/quic-v1"}},
))

// 2. mDNS: 自动发现局域网节点（默认启用）
node, _ := dep2p.New(ctx, dep2p.WithPreset(dep2p.PresetDesktop))

// 3. Bootstrap: 通过引导节点发现
node, _ := dep2p.New(ctx, dep2p.WithBootstrapPeers([]string{
    "/ip4/1.2.3.4/udp/4001/quic-v1/p2p/12D3KooW...",
}))
```

---

## 预设配置

DeP2P 提供多种预设配置，适应不同场景：

| 预设 | 适用场景 | 连接限制 | mDNS | 说明 |
|------|---------|----------|------|------|
| `PresetMinimal` | 测试/教程 | 10/20 | ❌ | 最小配置 |
| `PresetDesktop` | PC/笔记本 | 50/100 | ✅ | 默认推荐 |
| `PresetServer` | 服务器 | 200/500 | ✅ | 高性能 |
| `PresetMobile` | 手机/平板 | 20/50 | ✅ | 省电优化 |

```go
// 使用预设
node, _ := dep2p.New(ctx, dep2p.WithPreset(dep2p.PresetDesktop))

// 服务器场景
node, _ := dep2p.New(ctx, dep2p.WithPreset(dep2p.PresetServer))

// 测试场景（禁用自动发现）
node, _ := dep2p.New(ctx, dep2p.WithPreset(dep2p.PresetMinimal))
```

---

## 常见错误

### ErrNotMember

```go
// ❌ 错误：未加入 Realm 就调用业务 API
node, _ := dep2p.New(ctx, dep2p.WithPreset(dep2p.PresetDesktop))
_ = node.Start(ctx)
// err := realm.PubSub().Publish(...)  // 会返回 ErrNotMember

// ✅ 正确：先获取 Realm 并加入
node, _ := dep2p.New(ctx, dep2p.WithPreset(dep2p.PresetDesktop))
_ = node.Start(ctx)
realm, _ := node.Realm("my-realm")
_ = realm.Join(ctx)
err := realm.PubSub().Publish(ctx, "topic", data)  // err == nil
```

---

## 遇到问题？

```
┌─────────────────────────────────────────────────────────────────────┐
│                         遇到问题？快速排查                           │
├─────────────────────────────────────────────────────────────────────┤
│                                                                      │
│  ErrNotMember         → 先调用 JoinRealm()                          │
│  连接超时             → 检查网络、防火墙、地址是否正确                 │
│  peer id mismatch     → known_peers 中的 PeerID 与实际不符            │
│  dial backoff         → 连接频率过高，等待退避时间                    │
│                                                                      │
└─────────────────────────────────────────────────────────────────────┘
```

---

## 下一步

- [创建第一个节点](first-node.md) - 详细了解节点配置
- [加入第一个 Realm](first-realm.md) - 理解 Realm 概念
- [Hello World 教程](../tutorials/01-hello-world.md) - 完整两节点通信示例
- [局域网聊天](../tutorials/02-local-chat.md) - mDNS 多人聊天
