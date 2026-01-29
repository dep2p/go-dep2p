# Realm 群聊：成员管理与断开检测

本教程将指导你构建一个完整的 Realm 群聊应用，深入理解成员管理和断开检测机制。

---

## 教程目标

完成本教程后，你将学会：

- 理解 Realm 的"连接即成员"原则
- 订阅和处理成员加入/离开事件
- 使用断开检测机制处理离线
- 实现带状态同步的群聊应用

---

## 核心概念

### "连接即成员"原则

```
┌─────────────────────────────────────────────────────────────────────┐
│                    "连接即成员"原则                                  │
├─────────────────────────────────────────────────────────────────────┤
│                                                                     │
│  定义：                                                             │
│  与 Realm 中其他成员保持连接 = 成为成员                              │
│  连接断开 = 失去成员身份                                             │
│                                                                     │
│  时序图：                                                           │
│  ─────────────────────────────────────────────────────────────     │
│  节点 A                    Realm                    节点 B          │
│    │                         │                         │            │
│    │── JoinRealm ──────────►│                         │            │
│    │                         │                         │            │
│    │◄─── MemberJoined ──────│                         │            │
│    │      (B 已加入)          │                         │            │
│    │                         │                         │            │
│    │                         │◄──── 连接断开 ─────────│            │
│    │                         │                         │            │
│    │◄─── MemberLeft ────────│                         │            │
│    │      (B 已离开)          │                         │            │
│    │                         │                         │            │
│                                                                     │
│  关键点：                                                           │
│  • 无需显式"加入/退出"协议                                           │
│  • 连接状态决定成员身份                                              │
│  • 断开即离开，自动清理                                              │
└─────────────────────────────────────────────────────────────────────┘
```

### 断开检测机制

```
┌─────────────────────────────────────────────────────────────────────┐
│                    多层断开检测架构                                  │
├─────────────────────────────────────────────────────────────────────┤
│                                                                     │
│  Layer 1: QUIC Keep-Alive (传输层)                                  │
│  ┌─────────────────────────────────────────────────────────────┐   │
│  │  • keep_alive_period: 3s                                     │   │
│  │  • max_idle_timeout: 6s                                      │   │
│  │  • 最快检测时间: ~6s                                          │   │
│  └─────────────────────────────────────────────────────────────┘   │
│                              ↓                                      │
│  Layer 2: Reconnect Grace Period (应用层)                           │
│  ┌─────────────────────────────────────────────────────────────┐   │
│  │  • reconnect_grace_period: 15s                               │   │
│  │  • 允许短暂断线重连，不立即触发 MemberLeft                     │   │
│  │  • 网络抖动场景下减少误报                                      │   │
│  └─────────────────────────────────────────────────────────────┘   │
│                              ↓                                      │
│  Layer 3: Witness (见证人机制)                                      │
│  ┌─────────────────────────────────────────────────────────────┐   │
│  │  • witness_count: 3                                          │   │
│  │  • quorum: 2                                                 │   │
│  │  • 多数见证确认后才判定离线                                    │   │
│  └─────────────────────────────────────────────────────────────┘   │
│                                                                     │
│  最终效果: 准确、及时的成员状态同步                                  │
└─────────────────────────────────────────────────────────────────────┘
```

---

## 完整群聊应用

创建文件 `realm_chat/main.go`：

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
    "sync"
    "syscall"
    "time"

    "github.com/dep2p/go-dep2p"
    "github.com/dep2p/go-dep2p/pkg/types"
)

// 聊天配置
const (
    chatTopic = "realm/chat"
)

// ChatMessage 聊天消息
type ChatMessage struct {
    Type      string    `json:"type"`      // message, join, leave, sync
    From      string    `json:"from"`      // 发送者昵称
    FromID    string    `json:"from_id"`   // 发送者节点 ID
    Content   string    `json:"content"`   // 消息内容
    Timestamp time.Time `json:"timestamp"` // 时间戳
}

// MemberInfo 成员信息
type MemberInfo struct {
    ID       types.NodeID
    Nickname string
    JoinedAt time.Time
    Online   bool
}

// ChatApp 聊天应用
type ChatApp struct {
    node     dep2p.Node
    realmID  types.RealmID
    nickname string

    members     map[string]*MemberInfo // NodeID -> MemberInfo
    membersLock sync.RWMutex

    ctx    context.Context
    cancel context.CancelFunc
}

func main() {
    fmt.Println("╔════════════════════════════════════════╗")
    fmt.Println("║      Realm 群聊 - 成员管理演示         ║")
    fmt.Println("╚════════════════════════════════════════╝")
    fmt.Println()

    // 获取参数
    nickname := getInput("请输入昵称: ")
    if nickname == "" {
        nickname = "匿名"
    }

    roomName := getInput("请输入房间名 (默认 general): ")
    if roomName == "" {
        roomName = "general"
    }

    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()

    // 捕获中断信号
    signalCh := make(chan os.Signal, 1)
    signal.Notify(signalCh, syscall.SIGINT, syscall.SIGTERM)
    go func() {
        <-signalCh
        fmt.Println("\n正在退出...")
        cancel()
    }()

    // 创建并启动应用
    app, err := NewChatApp(ctx, nickname, roomName)
    if err != nil {
        log.Fatalf("创建应用失败: %v", err)
    }
    defer app.Close()

    // 运行主循环
    app.Run()
}

// NewChatApp 创建聊天应用
func NewChatApp(ctx context.Context, nickname, roomName string) (*ChatApp, error) {
    ctx, cancel := context.WithCancel(ctx)

    app := &ChatApp{
        nickname: nickname,
        realmID:  types.RealmID(roomName),
        members:  make(map[string]*MemberInfo),
        ctx:      ctx,
        cancel:   cancel,
    }

    // ========================================
    // Step 1: 创建节点
    // ========================================
    fmt.Println("Step 1: 启动节点...")
    node, err := dep2p.New(ctx,
        dep2p.WithPreset(dep2p.PresetDesktop),
    )
    if err != nil {
        cancel()
        return nil, fmt.Errorf("创建节点失败: %w", err)
    }
    app.node = node

    if err := node.Start(ctx); err != nil {
        cancel()
        node.Close()
        return nil, fmt.Errorf("启动节点失败: %w", err)
    }

    fmt.Printf("✅ 节点已启动: %s\n", node.ID().ShortString())
    fmt.Println()

    // ========================================
    // Step 2: 加入 Realm
    // ========================================
    fmt.Println("Step 2: 加入房间...")
    realm, err := node.Realm(string(app.realmID))
    if err != nil {
        cancel()
        node.Close()
        return nil, fmt.Errorf("获取 Realm 失败: %w", err)
    }
    if err := realm.Join(ctx); err != nil {
        cancel()
        node.Close()
        return nil, fmt.Errorf("加入 Realm 失败: %w", err)
    }
    fmt.Printf("✅ 已加入房间: %s\n", roomName)
    fmt.Println()

    // ========================================
    // Step 3: 订阅成员事件
    // ========================================
    fmt.Println("Step 3: 订阅成员事件...")
    if err := app.subscribeMemberEvents(); err != nil {
        cancel()
        node.Close()
        return nil, fmt.Errorf("订阅成员事件失败: %w", err)
    }
    fmt.Printf("✅ 成员事件监听已启动\n")
    fmt.Println()

    // ========================================
    // Step 4: 订阅聊天话题
    // ========================================
    fmt.Println("Step 4: 订阅聊天话题...")
    if err := app.subscribeChatTopic(); err != nil {
        cancel()
        node.Close()
        return nil, fmt.Errorf("订阅聊天话题失败: %w", err)
    }
    fmt.Printf("✅ 聊天话题已订阅\n")
    fmt.Println()

    // 添加自己到成员列表
    app.membersLock.Lock()
    app.members[node.ID().String()] = &MemberInfo{
        ID:       node.ID(),
        Nickname: nickname,
        JoinedAt: time.Now(),
        Online:   true,
    }
    app.membersLock.Unlock()

    // 广播加入消息
    app.broadcastJoin()

    return app, nil
}

// subscribeMemberEvents 订阅成员事件
func (app *ChatApp) subscribeMemberEvents() error {
    events, err := app.node.Realm().SubscribeMemberEvents(app.ctx, app.realmID)
    if err != nil {
        return err
    }

    go func() {
        for event := range events {
            switch event.Type {
            case dep2p.MemberJoined:
                app.handleMemberJoined(event.Member)
            case dep2p.MemberLeft:
                app.handleMemberLeft(event.Member)
            }
        }
    }()

    return nil
}

// handleMemberJoined 处理成员加入
func (app *ChatApp) handleMemberJoined(memberID types.NodeID) {
    if memberID == app.node.ID() {
        return // 忽略自己
    }

    app.membersLock.Lock()
    defer app.membersLock.Unlock()

    // 检查是否是重连
    if info, exists := app.members[memberID.String()]; exists {
        if !info.Online {
            info.Online = true
            fmt.Printf("\n🔄 成员重新上线: %s (%s)\n", info.Nickname, memberID.ShortString())
            fmt.Print("> ")
            return
        }
    }

    // 新成员
    app.members[memberID.String()] = &MemberInfo{
        ID:       memberID,
        Nickname: "未知", // 等待昵称同步
        JoinedAt: time.Now(),
        Online:   true,
    }

    fmt.Printf("\n🟢 新成员加入: %s\n", memberID.ShortString())
    fmt.Print("> ")
}

// handleMemberLeft 处理成员离开
func (app *ChatApp) handleMemberLeft(memberID types.NodeID) {
    if memberID == app.node.ID() {
        return // 忽略自己
    }

    app.membersLock.Lock()
    defer app.membersLock.Unlock()

    if info, exists := app.members[memberID.String()]; exists {
        info.Online = false
        fmt.Printf("\n🔴 成员离开: %s (%s)\n", info.Nickname, memberID.ShortString())
        fmt.Print("> ")
    }
}

// subscribeChatTopic 订阅聊天话题
func (app *ChatApp) subscribeChatTopic() error {
    sub, err := app.node.PubSub().Subscribe(app.ctx, chatTopic)
    if err != nil {
        return err
    }

    go func() {
        for {
            msg, err := sub.Next(app.ctx)
            if err != nil {
                return
            }

            // 忽略自己的消息
            if msg.From == app.node.ID() {
                continue
            }

            var chatMsg ChatMessage
            if err := json.Unmarshal(msg.Data, &chatMsg); err != nil {
                continue
            }

            app.handleChatMessage(chatMsg)
        }
    }()

    return nil
}

// handleChatMessage 处理聊天消息
func (app *ChatApp) handleChatMessage(msg ChatMessage) {
    switch msg.Type {
    case "message":
        timeStr := msg.Timestamp.Format("15:04:05")
        fmt.Printf("\n[%s] <%s> %s\n", timeStr, msg.From, msg.Content)
        fmt.Print("> ")

    case "join":
        // 更新成员昵称
        app.membersLock.Lock()
        if info, exists := app.members[msg.FromID]; exists {
            info.Nickname = msg.From
        }
        app.membersLock.Unlock()
        
        fmt.Printf("\n💬 %s 加入了聊天室\n", msg.From)
        fmt.Print("> ")

        // 回复自己的信息（昵称同步）
        app.broadcastSync()

    case "sync":
        // 更新成员昵称
        app.membersLock.Lock()
        if info, exists := app.members[msg.FromID]; exists {
            info.Nickname = msg.From
        } else {
            app.members[msg.FromID] = &MemberInfo{
                ID:       types.NodeID{}, // 会在后续更新
                Nickname: msg.From,
                JoinedAt: time.Now(),
                Online:   true,
            }
        }
        app.membersLock.Unlock()

    case "leave":
        fmt.Printf("\n👋 %s 离开了聊天室\n", msg.From)
        fmt.Print("> ")
    }
}

// broadcastJoin 广播加入消息
func (app *ChatApp) broadcastJoin() {
    msg := ChatMessage{
        Type:      "join",
        From:      app.nickname,
        FromID:    app.node.ID().String(),
        Content:   "",
        Timestamp: time.Now(),
    }
    app.broadcast(msg)
}

// broadcastSync 广播同步消息（昵称同步）
func (app *ChatApp) broadcastSync() {
    msg := ChatMessage{
        Type:      "sync",
        From:      app.nickname,
        FromID:    app.node.ID().String(),
        Content:   "",
        Timestamp: time.Now(),
    }
    app.broadcast(msg)
}

// broadcastLeave 广播离开消息
func (app *ChatApp) broadcastLeave() {
    msg := ChatMessage{
        Type:      "leave",
        From:      app.nickname,
        FromID:    app.node.ID().String(),
        Content:   "",
        Timestamp: time.Now(),
    }
    app.broadcast(msg)
}

// broadcast 广播消息
func (app *ChatApp) broadcast(msg ChatMessage) {
    data, _ := json.Marshal(msg)
    app.node.PubSub().Publish(app.ctx, chatTopic, data)
}

// sendMessage 发送聊天消息
func (app *ChatApp) sendMessage(content string) {
    msg := ChatMessage{
        Type:      "message",
        From:      app.nickname,
        FromID:    app.node.ID().String(),
        Content:   content,
        Timestamp: time.Now(),
    }
    app.broadcast(msg)
}

// Run 运行主循环
func (app *ChatApp) Run() {
    fmt.Println("════════════════════════════════════════")
    fmt.Printf("聊天室 [%s] 已就绪!\n", app.realmID)
    fmt.Println()
    fmt.Println("命令:")
    fmt.Println("  /members  - 查看成员列表")
    fmt.Println("  /status   - 查看连接状态")
    fmt.Println("  /quit     - 退出聊天室")
    fmt.Println()
    fmt.Println("直接输入消息发送")
    fmt.Println("════════════════════════════════════════")
    fmt.Println()

    reader := bufio.NewReader(os.Stdin)
    for {
        select {
        case <-app.ctx.Done():
            return
        default:
        }

        fmt.Print("> ")
        input, err := reader.ReadString('\n')
        if err != nil {
            continue
        }
        input = strings.TrimSpace(input)
        if input == "" {
            continue
        }

        if strings.HasPrefix(input, "/") {
            app.handleCommand(input)
        } else {
            app.sendMessage(input)
        }
    }
}

// handleCommand 处理命令
func (app *ChatApp) handleCommand(cmd string) {
    switch cmd {
    case "/members":
        app.showMembers()

    case "/status":
        app.showStatus()

    case "/quit", "/exit":
        app.broadcastLeave()
        app.cancel()

    default:
        fmt.Println("未知命令，可用命令: /members, /status, /quit")
    }
}

// showMembers 显示成员列表
func (app *ChatApp) showMembers() {
    app.membersLock.RLock()
    defer app.membersLock.RUnlock()

    online := 0
    offline := 0

    fmt.Println("\n成员列表:")
    fmt.Println("─────────────────────────────────")
    for _, info := range app.members {
        status := "🟢 在线"
        if !info.Online {
            status = "🔴 离线"
            offline++
        } else {
            online++
        }

        self := ""
        if info.ID == app.node.ID() {
            self = " (我)"
        }

        fmt.Printf("  %s %s%s\n", status, info.Nickname, self)
    }
    fmt.Println("─────────────────────────────────")
    fmt.Printf("  在线: %d, 离线: %d, 总计: %d\n", online, offline, len(app.members))
    fmt.Println()
}

// showStatus 显示状态
func (app *ChatApp) showStatus() {
    fmt.Println("\n连接状态:")
    fmt.Println("─────────────────────────────────")
    fmt.Printf("  节点 ID: %s\n", app.node.ID().ShortString())
    fmt.Printf("  房间: %s\n", app.realmID)
    fmt.Printf("  昵称: %s\n", app.nickname)

    // 获取 Realm 成员数
    members := app.node.Realm().Members(app.realmID)
    fmt.Printf("  Realm 成员: %d\n", len(members))

    fmt.Println("─────────────────────────────────")
    fmt.Println()
}

// Close 关闭应用
func (app *ChatApp) Close() {
    if app.node != nil {
        app.node.Close()
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

### 终端 1：Alice

```bash
go run main.go
```

```
请输入昵称: Alice
请输入房间名 (默认 general): dev-team

Step 1: 启动节点...
✅ 节点已启动: 12D3KooWxx...

Step 2: 加入房间...
✅ 已加入房间: dev-team

Step 3: 订阅成员事件...
✅ 成员事件监听已启动

Step 4: 订阅聊天话题...
✅ 聊天话题已订阅

════════════════════════════════════════
聊天室 [dev-team] 已就绪!

命令:
  /members  - 查看成员列表
  /status   - 查看连接状态
  /quit     - 退出聊天室

直接输入消息发送
════════════════════════════════════════

> 
```

### 终端 2：Bob

```bash
go run main.go
```

```
请输入昵称: Bob
请输入房间名 (默认 general): dev-team
```

### 观察成员事件

**Alice 终端：**

```
🟢 新成员加入: 12D3KooWyy...
> 
💬 Bob 加入了聊天室
> 
```

**Bob 终端：**

```
🟢 新成员加入: 12D3KooWxx...
> 
💬 Alice 加入了聊天室
> 
```

### 发送消息

**Alice：**

```
> Hello Bob!
```

**Bob 看到：**

```
[14:30:15] <Alice> Hello Bob!
> 
```

### 查看成员列表

```
> /members

成员列表:
─────────────────────────────────
  🟢 在线 Alice (我)
  🟢 在线 Bob
─────────────────────────────────
  在线: 2, 离线: 0, 总计: 2

> 
```

### 模拟断开

强制关闭 Bob 的终端（Ctrl+C 或 kill），Alice 会看到：

```
🔴 成员离开: Bob (12D3KooWyy...)
> 
```

如果 Bob 在宽限期内重新连接：

```
🔄 成员重新上线: Bob (12D3KooWyy...)
> 
```

---

## 断开检测配置

### 调整断开检测参数

```go
// 创建节点时指定断开检测配置
node, err := dep2p.New(ctx,
    dep2p.WithPreset(dep2p.PresetDesktop),
    dep2p.WithDisconnectDetection(config.DisconnectDetectionConfig{
        QUIC: config.QUICDisconnectConfig{
            KeepAlivePeriod: 3 * time.Second,
            MaxIdleTimeout:  6 * time.Second,
        },
        ReconnectGracePeriod: 15 * time.Second,
        Witness: config.WitnessConfig{
            Enabled: true,
            Count:   3,
            Quorum:  2,
            Timeout: 5 * time.Second,
        },
        Flapping: config.FlappingConfig{
            Enabled:   true,
            Window:    60 * time.Second,
            Threshold: 3,
            Cooldown:  120 * time.Second,
        },
    }),
)
```

### 不同场景的推荐配置

| 场景 | KeepAlive | IdleTimeout | GracePeriod | 说明 |
|------|-----------|-------------|-------------|------|
| 稳定网络 | 3s | 6s | 10s | 快速检测，较短宽限 |
| 移动网络 | 5s | 10s | 30s | 容忍抖动，较长宽限 |
| 实时游戏 | 1s | 3s | 5s | 极速检测，短宽限 |
| 后台同步 | 10s | 30s | 60s | 低功耗，长宽限 |

---

## 成员状态机

```
┌─────────────────────────────────────────────────────────────────────┐
│                    成员连接状态机                                    │
├─────────────────────────────────────────────────────────────────────┤
│                                                                     │
│      ┌─────────────┐                                                │
│      │   Unknown   │ ◄─── 初始状态                                   │
│      └──────┬──────┘                                                │
│             │ JoinRealm                                             │
│             ▼                                                       │
│      ┌─────────────┐                                                │
│  ┌──►│   Online    │ ◄─── 正常状态                                   │
│  │   └──────┬──────┘                                                │
│  │          │ 连接断开                                               │
│  │          ▼                                                       │
│  │   ┌─────────────┐                                                │
│  │   │ Suspending  │ ◄─── 断开保护期 (等待重连)                       │
│  │   └──────┬──────┘                                                │
│  │          │                                                       │
│  │    ┌─────┴─────┐                                                 │
│  │    │           │                                                 │
│  │    ▼           ▼                                                 │
│  │  重连成功    超时                                                 │
│  │    │           │                                                 │
│  └────┘           ▼                                                 │
│            ┌─────────────┐                                          │
│            │   Offline   │ ◄─── 触发 MemberLeft                      │
│            └─────────────┘                                          │
│                                                                     │
└─────────────────────────────────────────────────────────────────────┘
```

---

## 故障排查

### 问题 1：成员加入事件不触发

**可能原因**：
- mDNS 发现延迟
- 不在同一 Realm

**解决方案**：

```go
// 确保在同一 Realm
fmt.Printf("当前 Realm: %s\n", node.Realm().CurrentRealm())

// 手动检查成员列表
members := node.Realm().Members(realmID)
fmt.Printf("当前成员数: %d\n", len(members))
```

### 问题 2：频繁的加入/离开事件

**原因**：网络不稳定导致连接震荡

**解决方案**：启用震荡检测

```go
Flapping: config.FlappingConfig{
    Enabled:   true,
    Window:    60 * time.Second,  // 60秒窗口
    Threshold: 3,                  // 3次断线触发
    Cooldown:  120 * time.Second, // 2分钟冷却
},
```

### 问题 3：成员离开事件延迟

**原因**：断开检测和宽限期导致延迟

**调优**：减小相关参数

```go
// 快速检测配置
QUIC: config.QUICDisconnectConfig{
    KeepAlivePeriod: 1 * time.Second,
    MaxIdleTimeout:  3 * time.Second,
},
ReconnectGracePeriod: 5 * time.Second,
```

---

## 下一步

- [故障排查](05-troubleshooting-live.md) - 使用日志分析框架排查问题
- [配置参考](../reference/configuration.md) - 完整配置选项说明
- [核心概念](../concepts/core-concepts.md) - 深入理解架构
