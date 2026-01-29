# 局域网聊天：mDNS 自动发现

本教程将指导你构建一个局域网 P2P 聊天应用，使用 mDNS 自动发现同一网络的节点，并通过 Realm 管理成员。

---

## 教程目标

完成本教程后，你将学会：

- 使用 mDNS 自动发现局域网内的节点
- 使用 Realm 隔离和管理聊天成员
- 订阅 Realm 成员事件（加入/离开）
- 使用 PubSub 实现群聊广播

---

## 应用架构

```
┌─────────────────────────────────────────────────────────────────────┐
│                    局域网聊天应用架构                                │
├─────────────────────────────────────────────────────────────────────┤
│                                                                     │
│                        ┌─────────────────┐                          │
│                        │   Realm 域      │                          │
│                        │  "local-chat"   │                          │
│                        └────────┬────────┘                          │
│                                 │                                   │
│         ┌───────────────────────┼───────────────────────┐          │
│         │                       │                       │          │
│   ┌─────▼─────┐           ┌─────▼─────┐          ┌─────▼─────┐    │
│   │  节点 A   │◄─ mDNS ─►│  节点 B   │◄─ mDNS ─►│  节点 C   │    │
│   │  (Alice)  │           │  (Bob)    │           │  (Carol)  │    │
│   └───────────┘           └───────────┘           └───────────┘    │
│         │                       │                       │          │
│         └───────────────────────┼───────────────────────┘          │
│                                 │                                   │
│                         ┌───────▼───────┐                          │
│                         │   PubSub      │                          │
│                         │  GossipSub    │                          │
│                         └───────────────┘                          │
│                                                                     │
│  • mDNS：同一网络自动发现，零配置                                   │
│  • Realm：成员隔离，只有同域节点才能通信                            │
│  • PubSub：基于 GossipSub 的高效消息广播                            │
└─────────────────────────────────────────────────────────────────────┘
```

---

## 前置条件

- Go 1.21 或更高版本
- DeP2P 已安装
- 同一局域网内的多台设备（或同一台机器的多个终端）

---

## 完整代码

创建文件 `local_chat/main.go`：

```go
package main

import (
    "bufio"
    "context"
    "encoding/json"
    "fmt"
    "log"
    "os"
    "os/signal"
    "strings"
    "syscall"
    "time"

    "github.com/dep2p/go-dep2p"
    "github.com/dep2p/go-dep2p/pkg/types"
)

// 聊天配置
const (
    realmName  = "local-chat"        // Realm 名称
    chatTopic  = "chat/general"      // PubSub 话题
)

// ChatMessage 聊天消息结构
type ChatMessage struct {
    From      string    `json:"from"`       // 发送者昵称
    Content   string    `json:"content"`    // 消息内容
    Timestamp time.Time `json:"timestamp"`  // 发送时间
}

func main() {
    fmt.Println("╔════════════════════════════════════════╗")
    fmt.Println("║      局域网聊天 - mDNS 自动发现        ║")
    fmt.Println("╚════════════════════════════════════════╝")
    fmt.Println()

    // 获取昵称
    nickname := getInput("请输入你的昵称: ")
    if nickname == "" {
        nickname = "匿名用户"
    }
    fmt.Printf("欢迎, %s!\n\n", nickname)

    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()

    // 捕获中断信号
    signalCh := make(chan os.Signal, 1)
    signal.Notify(signalCh, syscall.SIGINT, syscall.SIGTERM)
    go func() {
        <-signalCh
        fmt.Println("\n再见! 👋")
        cancel()
    }()

    // ========================================
    // Step 1: 创建节点（启用 mDNS）
    // ========================================
    fmt.Println("Step 1: 启动节点...")
    node, err := dep2p.New(ctx,
        dep2p.WithPreset(dep2p.PresetDesktop), // Desktop 预设自动启用 mDNS
    )
    if err != nil {
        log.Fatalf("启动节点失败: %v", err)
    }
    defer node.Close()

    if err := node.Start(ctx); err != nil {
        log.Fatalf("节点启动失败: %v", err)
    }

    fmt.Printf("✅ 节点已启动\n")
    fmt.Printf("   节点 ID: %s\n", node.ID().ShortString())
    fmt.Printf("   mDNS 已启用，将自动发现同网络节点\n")
    fmt.Println()

    // ========================================
    // Step 2: 创建并加入 Realm
    // ========================================
    fmt.Println("Step 2: 加入 Realm...")
    
    realm, err := node.Realm(realmName)
    if err != nil {
        log.Fatalf("获取 Realm 失败: %v", err)
    }
    if err := realm.Join(ctx); err != nil {
        log.Fatalf("加入 Realm 失败: %v", err)
    }

    fmt.Printf("✅ 已加入 Realm: %s\n", realmName)
    fmt.Println()

    // ========================================
    // Step 3: 订阅 Realm 成员事件
    // ========================================
    fmt.Println("Step 3: 订阅成员事件...")
    
    memberEvents, err := node.Realm().SubscribeMemberEvents(ctx, realmID)
    if err != nil {
        log.Fatalf("订阅成员事件失败: %v", err)
    }

    // 处理成员加入/离开事件
    go func() {
        for event := range memberEvents {
            switch event.Type {
            case dep2p.MemberJoined:
                fmt.Printf("\n🟢 成员加入: %s\n", event.Member.ShortString())
                fmt.Print("> ")
            case dep2p.MemberLeft:
                fmt.Printf("\n🔴 成员离开: %s\n", event.Member.ShortString())
                fmt.Print("> ")
            }
        }
    }()

    fmt.Printf("✅ 已订阅成员事件\n")
    fmt.Println()

    // ========================================
    // Step 4: 订阅聊天话题
    // ========================================
    fmt.Println("Step 4: 订阅聊天话题...")
    
    sub, err := node.PubSub().Subscribe(ctx, chatTopic)
    if err != nil {
        log.Fatalf("订阅话题失败: %v", err)
    }
    defer sub.Cancel()

    // 处理接收到的消息
    go func() {
        for {
            msg, err := sub.Next(ctx)
            if err != nil {
                return
            }

            // 忽略自己的消息
            if msg.From == node.ID() {
                continue
            }

            // 解析消息
            var chatMsg ChatMessage
            if err := json.Unmarshal(msg.Data, &chatMsg); err != nil {
                continue
            }

            // 显示消息
            timeStr := chatMsg.Timestamp.Format("15:04:05")
            fmt.Printf("\n[%s] <%s> %s\n", timeStr, chatMsg.From, chatMsg.Content)
            fmt.Print("> ")
        }
    }()

    fmt.Printf("✅ 已订阅话题: %s\n", chatTopic)
    fmt.Println()

    // ========================================
    // Step 5: 开始聊天
    // ========================================
    fmt.Println("════════════════════════════════════════")
    fmt.Println("聊天室已就绪!")
    fmt.Println()
    fmt.Println("提示:")
    fmt.Println("  • 同一局域网的节点会自动发现并加入")
    fmt.Println("  • 输入消息后按 Enter 发送")
    fmt.Println("  • 输入 /members 查看当前成员")
    fmt.Println("  • 输入 /quit 退出")
    fmt.Println("════════════════════════════════════════")
    fmt.Println()

    // 输入循环
    reader := bufio.NewReader(os.Stdin)
    for {
        fmt.Print("> ")
        input, err := reader.ReadString('\n')
        if err != nil {
            continue
        }
        input = strings.TrimSpace(input)
        if input == "" {
            continue
        }

        // 处理命令
        if strings.HasPrefix(input, "/") {
            handleCommand(ctx, node, realmID, input)
            continue
        }

        // 构造消息
        chatMsg := ChatMessage{
            From:      nickname,
            Content:   input,
            Timestamp: time.Now(),
        }
        data, _ := json.Marshal(chatMsg)

        // 发布消息
        if err := node.PubSub().Publish(ctx, chatTopic, data); err != nil {
            fmt.Printf("发送失败: %v\n", err)
            continue
        }
    }
}

// handleCommand 处理命令
func handleCommand(ctx context.Context, node dep2p.Node, realmID types.RealmID, cmd string) {
    switch cmd {
    case "/quit", "/exit":
        fmt.Println("再见!")
        os.Exit(0)

    case "/members":
        members := node.Realm().Members(realmID)
        fmt.Printf("\n当前成员 (%d):\n", len(members))
        for i, m := range members {
            marker := ""
            if m == node.ID() {
                marker = " (我)"
            }
            fmt.Printf("  %d. %s%s\n", i+1, m.ShortString(), marker)
        }
        fmt.Println()

    case "/help":
        fmt.Println("\n可用命令:")
        fmt.Println("  /members  - 查看当前成员列表")
        fmt.Println("  /quit     - 退出聊天")
        fmt.Println("  /help     - 显示帮助")
        fmt.Println()

    default:
        fmt.Println("未知命令，输入 /help 查看帮助")
    }
}

// getInput 获取用户输入
func getInput(prompt string) string {
    fmt.Print(prompt)
    reader := bufio.NewReader(os.Stdin)
    input, _ := reader.ReadString('\n')
    return strings.TrimSpace(input)
}
```

---

## 运行示例

### 终端 1：启动 Alice

```bash
cd local_chat
go run main.go
```

输入昵称 `Alice`，预期输出：

```
╔════════════════════════════════════════╗
║      局域网聊天 - mDNS 自动发现        ║
╚════════════════════════════════════════╝

请输入你的昵称: Alice
欢迎, Alice!

Step 1: 启动节点...
✅ 节点已启动
   节点 ID: 12D3KooWxx...
   mDNS 已启用，将自动发现同网络节点

Step 2: 加入 Realm...
✅ 已加入 Realm: local-chat

Step 3: 订阅成员事件...
✅ 已订阅成员事件

Step 4: 订阅聊天话题...
✅ 已订阅话题: chat/general

════════════════════════════════════════
聊天室已就绪!

提示:
  • 同一局域网的节点会自动发现并加入
  • 输入消息后按 Enter 发送
  • 输入 /members 查看当前成员
  • 输入 /quit 退出
════════════════════════════════════════

> 
```

### 终端 2：启动 Bob

```bash
go run main.go
```

输入昵称 `Bob`。

### 观察 mDNS 发现过程

几秒钟后，两个终端都会显示成员事件：

**Alice 终端：**

```
> 
🟢 成员加入: 12D3KooWyy...
> 
```

**Bob 终端：**

```
> 
🟢 成员加入: 12D3KooWxx...
> 
```

### 发送消息

**Alice 输入：**

```
> Hello, Bob!
```

**Bob 看到：**

```
[14:30:15] <Alice> Hello, Bob!
> 
```

### 查看成员列表

输入 `/members`：

```
> /members

当前成员 (2):
  1. 12D3KooWxx... (我)
  2. 12D3KooWyy...

> 
```

---

## 关键概念

### 1. mDNS 自动发现

```go
dep2p.WithPreset(dep2p.PresetDesktop) // 自动启用 mDNS
```

mDNS（多播 DNS）工作原理：

- 节点启动时向局域网广播自己的存在
- 同一网络的节点接收广播并自动连接
- **零配置**：无需指定任何地址

**适用场景**：
- 同一 WiFi 网络的设备
- 同一以太网段的服务器
- 本地开发和测试

### 2. Realm 成员管理

```go
// 加入 Realm
realm, _ := node.Realm("realm-name")
_ = realm.Join(ctx)

// 订阅成员事件
events, _ := node.Realm().SubscribeMemberEvents(ctx, realmID)

// 获取成员列表
members := node.Realm().Members(realmID)
```

Realm 提供逻辑隔离：

- **成员隔离**：只有同一 Realm 的节点才能通信
- **事件通知**：成员加入/离开时收到通知
- **"连接即成员"原则**：与 Realm 成员保持连接，断开即离开

### 3. PubSub 群聊

```go
// 订阅话题
sub, _ := node.PubSub().Subscribe(ctx, chatTopic)

// 接收消息
msg, _ := sub.Next(ctx)

// 发布消息
node.PubSub().Publish(ctx, chatTopic, data)
```

PubSub 基于 GossipSub 协议：

- **高效广播**：消息通过 Gossip 协议高效传播
- **去重**：自动去除重复消息
- **可靠性**：多路径传播确保消息送达

### 4. 消息结构

```go
type ChatMessage struct {
    From      string    `json:"from"`       // 发送者昵称
    Content   string    `json:"content"`    // 消息内容
    Timestamp time.Time `json:"timestamp"`  // 发送时间
}
```

使用 JSON 格式便于扩展和调试。

---

## mDNS 工作原理

```
┌─────────────────────────────────────────────────────────────────────┐
│                     mDNS 发现过程                                    │
├─────────────────────────────────────────────────────────────────────┤
│                                                                     │
│  1. 节点 A 启动                                                      │
│     └─> 广播: "_dep2p._udp.local" TXT "peer-id=12D3KooW..."        │
│                                                                     │
│  2. 节点 B 启动                                                      │
│     └─> 广播: "_dep2p._udp.local" TXT "peer-id=12D3KooW..."        │
│     └─> 收到 A 的广播                                                │
│         └─> 解析地址，发起连接                                       │
│                                                                     │
│  3. 双向确认                                                         │
│     A ◄──────────────────────────────────────────────────────► B   │
│         QUIC 握手 → 加密通道建立 → Realm 成员同步                    │
│                                                                     │
└─────────────────────────────────────────────────────────────────────┘
```

---

## 故障排查

### 问题 1：节点无法发现彼此

**症状**：启动多个节点，但看不到成员加入事件

**可能原因**：
- 不在同一网络（不同 WiFi/VLAN）
- 防火墙阻止 mDNS 流量（UDP 5353）
- 路由器禁用了多播

**解决方案**：

```bash
# 检查是否在同一网络
ifconfig | grep inet

# macOS：检查 mDNS 服务
dns-sd -B _dep2p._udp local

# Linux：安装 avahi 并检查
sudo systemctl status avahi-daemon
avahi-browse -a
```

### 问题 2：消息发送失败

**症状**：输入消息后提示"发送失败"

**可能原因**：
- PubSub 话题尚未建立路由
- 连接已断开

**解决方案**：

```go
// 等待 PubSub 路由建立
time.Sleep(2 * time.Second)

// 检查是否有其他成员
if len(node.Realm().Members(realmID)) <= 1 {
    fmt.Println("提示：还没有其他成员，消息无法广播")
}
```

### 问题 3：频繁的成员加入/离开事件

**症状**：同一节点反复出现加入/离开事件

**可能原因**：
- 网络不稳定
- 连接震荡（flapping）

**解决方案**：

DeP2P 内置了震荡检测机制，默认配置：

```json
{
  "disconnect_detection": {
    "flapping": {
      "enabled": true,
      "window": "60s",
      "threshold": 3,
      "cooldown": "120s"
    }
  }
}
```

如果节点在 60 秒内断线 3 次，会触发冷却期，暂停重连。

---

## 扩展功能

### 添加私聊功能

```go
// 私聊协议
const privateProtocol = "/chat/private/1.0.0"

// 注册私聊处理器
node.Endpoint().SetProtocolHandler(privateProtocol, func(stream dep2p.Stream) {
    defer stream.Close()
    // 处理私聊消息...
})

// 发送私聊
func sendPrivateMessage(ctx context.Context, node dep2p.Node, targetID types.NodeID, msg string) error {
    stream, err := node.OpenStream(ctx, targetID, privateProtocol)
    if err != nil {
        return err
    }
    defer stream.Close()
    _, err = stream.Write([]byte(msg))
    return err
}
```

### 消息持久化

```go
// 使用本地数据库存储历史消息
import "github.com/syndtr/goleveldb/leveldb"

db, _ := leveldb.OpenFile("chat_history", nil)
defer db.Close()

// 保存消息
key := fmt.Sprintf("msg:%d", time.Now().UnixNano())
db.Put([]byte(key), data, nil)
```

---

## 下一步

- [云服务器部署](03-cloud-deploy.md) - 在公网部署 P2P 节点
- [Realm 群聊](04-realm-chat.md) - 深入理解 Realm 和成员管理
- [故障排查](05-troubleshooting-live.md) - 使用日志分析框架排查问题
