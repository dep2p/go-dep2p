# 创建第一个节点

本文档将详细介绍如何创建和配置你的第一个 DeP2P 节点。

---

## 节点创建流程

```
┌─────────────────────────────────────────────────────────────────────┐
│                      节点创建流程                                    │
├─────────────────────────────────────────────────────────────────────┤
│                                                                      │
│  1. 选择预设配置 (WithPreset)                                        │
│          ↓                                                          │
│  2. 可选：自定义配置 (WithXxx)                                       │
│          ↓                                                          │
│  3. 创建节点 (dep2p.New)                                             │
│          ↓                                                          │
│  4. 启动节点 (node.Start)                                            │
│          ↓                                                          │
│  5. 节点就绪                                                         │
│                                                                      │
└─────────────────────────────────────────────────────────────────────┘
```

---

## 使用预设配置（推荐）

### 基础创建

```go
package main

import (
    "context"
    "fmt"
    "log"
    
    "github.com/dep2p/go-dep2p"
)

func main() {
    ctx := context.Background()
    
    // 创建节点
    node, err := dep2p.New(ctx, dep2p.WithPreset(dep2p.PresetDesktop))
    if err != nil {
        log.Fatalf("创建节点失败: %v", err)
    }
    defer node.Close()
    
    // 启动节点
    if err := node.Start(ctx); err != nil {
        log.Fatalf("启动节点失败: %v", err)
    }
    
    // 打印节点信息
    fmt.Printf("节点 ID: %s\n", node.ID())
    fmt.Printf("监听地址: %v\n", node.ListenAddrs())
}
```

### 预设配置对比

| 预设 | 场景 | 连接数 | mDNS | Relay | 说明 |
|------|------|--------|------|-------|------|
| `PresetMinimal` | 测试/教程 | 10/20 | ❌ | ❌ | 最小配置 |
| `PresetDesktop` | 桌面端 | 50/100 | ✅ | ✅ | 默认推荐 |
| `PresetServer` | 服务器 | 200/500 | ✅ | ✅ | 可作中继 |
| `PresetMobile` | 移动端 | 20/50 | ✅ | ✅ | 省电优化 |

---

## 自定义配置

### 指定监听端口

```go
node, err := dep2p.New(ctx,
    dep2p.WithPreset(dep2p.PresetDesktop),
    dep2p.WithListenPort(4001),  // 指定固定端口
)
```

### 使用固定身份

默认情况下，每次启动会生成新的临时身份。如需固定身份（推荐用于服务器）：

```go
// 方法一：使用身份文件（推荐）
node, err := dep2p.New(ctx,
    dep2p.WithPreset(dep2p.PresetServer),
    dep2p.WithIdentityFile("./node.key"),  // 首次运行自动生成
)

// 方法二：编程式生成密钥
key, err := dep2p.GenerateKey()
if err != nil {
    log.Fatal(err)
}

node, err := dep2p.New(ctx,
    dep2p.WithPreset(dep2p.PresetDesktop),
    dep2p.WithIdentity(key),
)
```

### 配置连接限制

```go
node, err := dep2p.New(ctx,
    dep2p.WithPreset(dep2p.PresetDesktop),
    dep2p.WithConnectionLimits(100, 200),  // LowWater, HighWater
)
```

---

## 配置已知节点（known_peers）

`known_peers` 是 DeP2P 的核心连接机制之一，适用于已知节点地址的场景。

### 使用场景

- **私有集群**：节点间互相知道对方地址
- **云服务器**：有固定公网 IP 的服务器
- **无 Bootstrap**：不依赖公共引导节点

### 配置方法

```go
import "github.com/dep2p/go-dep2p/config"

node, err := dep2p.New(ctx,
    dep2p.WithPreset(dep2p.PresetDesktop),
    dep2p.WithKnownPeers(
        config.KnownPeer{
            PeerID: "12D3KooWxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
            Addrs:  []string{"/ip4/1.2.3.4/udp/4001/quic-v1"},
        },
        config.KnownPeer{
            PeerID: "12D3KooWyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyy",
            Addrs:  []string{"/ip4/5.6.7.8/udp/4001/quic-v1"},
        },
    ),
)
```

### known_peers vs Bootstrap

| 特性 | known_peers | Bootstrap |
|------|-------------|-----------|
| 用途 | 直接连接 | DHT 引导 |
| 依赖 | 无 | Bootstrap 节点运行 |
| 连接时机 | 启动即连接 | DHT 初始化后 |
| 适用场景 | 私有网络、已知节点 | 公共网络、动态发现 |

### 配置文件方式

也可以通过 JSON 配置文件配置 known_peers：

```json
{
  "preset": "desktop",
  "known_peers": [
    {
      "peer_id": "12D3KooWxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
      "addrs": ["/ip4/1.2.3.4/udp/4001/quic-v1"]
    }
  ]
}
```

---

## 配置 Bootstrap 节点

Bootstrap 节点用于 DHT 引导和公网节点发现：

```go
bootstrapPeers := []string{
    "/ip4/1.2.3.4/udp/4001/quic-v1/p2p/12D3KooWxxxxx...",
    "/dns4/bootstrap.example.com/udp/4001/quic-v1/p2p/12D3KooWxxxxx...",
}

node, err := dep2p.New(ctx,
    dep2p.WithPreset(dep2p.PresetDesktop),
    dep2p.WithBootstrapPeers(bootstrapPeers),
)
```

> ⚠️ **注意**：Bootstrap 地址必须使用完整格式（含 `/p2p/<NodeID>`）。

---

## 云服务器配置

在云服务器（有公网 IP）场景下，推荐以下配置：

```go
node, err := dep2p.New(ctx,
    dep2p.WithPreset(dep2p.PresetServer),
    dep2p.WithListenPort(4001),
    dep2p.WithIdentityFile("/etc/dep2p/identity.key"),
    
    // 信任 STUN 探测地址（跳过入站验证）
    dep2p.WithTrustSTUNAddresses(true),
    
    // 配置已知节点
    dep2p.WithKnownPeers(
        config.KnownPeer{
            PeerID: "12D3KooWxxxxx...",
            Addrs:  []string{"/ip4/peer1.example.com/udp/4001/quic-v1"},
        },
    ),
)
```

### trust_stun_addresses 说明

`TrustSTUNAddresses` 配置用于云服务器场景：

- **作用**：信任 STUN 探测发现的公网地址
- **好处**：跳过入站连接验证，加速地址发布
- **适用**：云服务器有真实公网 IP，且网络配置确保入站可达

---

## 节点状态检查

### 基本信息

```go
// 节点 ID（公钥身份，Base58 编码）
fmt.Printf("节点 ID: %s\n", node.ID())

// 简短 ID（便于日志显示）
fmt.Printf("简短 ID: %s\n", node.ID().ShortString())

// 本地监听地址
fmt.Printf("监听地址: %v\n", node.ListenAddrs())
```

### 检查子系统

```go
// 检查 Realm 管理器
if rm := node.Realm(); rm != nil {
    fmt.Printf("当前 Realm: %s\n", rm.CurrentRealm())
}

// 检查 Endpoint
if ep := node.Endpoint(); ep != nil {
    fmt.Printf("Endpoint 就绪\n")
}

// 检查连接状态
if node.IsConnected(targetID) {
    fmt.Printf("已连接到: %s\n", targetID.ShortString())
}
```

---

## 完整示例

```go
package main

import (
    "context"
    "fmt"
    "log"
    "os"
    "os/signal"
    "syscall"
    
    "github.com/dep2p/go-dep2p"
)

func main() {
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()
    
    // 捕获中断信号
    signalCh := make(chan os.Signal, 1)
    signal.Notify(signalCh, syscall.SIGINT, syscall.SIGTERM)
    go func() {
        <-signalCh
        fmt.Println("\n正在关闭节点...")
        cancel()
    }()
    
    // 创建节点
    node, err := dep2p.New(ctx,
        dep2p.WithPreset(dep2p.PresetDesktop),
        dep2p.WithListenPort(4001),
    )
    if err != nil {
        log.Fatalf("创建节点失败: %v", err)
    }
    defer node.Close()
    
    // 启动节点
    if err := node.Start(ctx); err != nil {
        log.Fatalf("启动节点失败: %v", err)
    }
    
    // 打印节点信息
    fmt.Println("=== DeP2P 节点已启动 ===")
    fmt.Printf("节点 ID: %s\n", node.ID())
    fmt.Println()
    
    fmt.Println("监听地址:")
    for i, addr := range node.ListenAddrs() {
        fmt.Printf("  [%d] %s\n", i+1, addr)
    }
    fmt.Println()
    
    fmt.Println("按 Ctrl+C 退出")
    
    // 等待退出
    <-ctx.Done()
    fmt.Println("节点已关闭")
}
```

---

## 地址格式说明

DeP2P 使用 Multiaddr 格式表示地址：

| 地址类型 | 格式示例 | 说明 |
|----------|----------|------|
| IPv4 + QUIC | `/ip4/192.168.1.1/udp/4001/quic-v1` | 本地/局域网 |
| IPv6 + QUIC | `/ip6/::1/udp/4001/quic-v1` | IPv6 地址 |
| DNS + QUIC | `/dns4/node.example.com/udp/4001/quic-v1` | DNS 解析 |
| 完整地址 | `/ip4/.../udp/4001/quic-v1/p2p/<NodeID>` | 含身份信息 |

---

## 常见问题

### Q: 端口被占用

```bash
# 错误: bind: address already in use
```

**解决方案**：

```go
// 使用随机端口
node, _ := dep2p.New(ctx,
    dep2p.WithPreset(dep2p.PresetDesktop),
    dep2p.WithListenPort(0),  // 0 = 随机端口
)
```

### Q: 节点 ID 每次启动都变化

**原因**：默认使用临时身份。

**解决方案**：

```go
// 使用身份文件持久化
node, _ := dep2p.New(ctx,
    dep2p.WithPreset(dep2p.PresetServer),
    dep2p.WithIdentityFile("./node.key"),
)
```

### Q: known_peers 连接失败

**检查项**：
1. PeerID 是否正确（完整的 Base58 编码）
2. 地址格式是否正确
3. 目标节点是否在运行
4. 防火墙是否允许 UDP 流量

---

## 下一步

- [加入第一个 Realm](first-realm.md) - 加入业务网络
- [云服务器部署](../tutorials/03-cloud-deploy.md) - 公网部署教程
- [常见问题](faq.md) - 更多问题解答
