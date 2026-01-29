# 云服务器部署：公网 P2P 通信

本教程将指导你在两台云服务器上部署 DeP2P 节点，实现公网 P2P 通信。使用 `known_peers` 和 `trust_stun_addresses` 配置优化连接。

---

## 教程目标

完成本教程后，你将学会：

- 在云服务器上部署 DeP2P 节点
- 使用 `known_peers` 配置节点直连
- 使用 `trust_stun_addresses` 优化公网地址发现
- 配置断开检测参数
- 部署生产级 P2P 应用

---

## 部署架构

```
┌─────────────────────────────────────────────────────────────────────┐
│                    云服务器 P2P 部署架构                             │
├─────────────────────────────────────────────────────────────────────┤
│                                                                     │
│     ┌─────────────────────┐         ┌─────────────────────┐        │
│     │   云服务器 A        │         │   云服务器 B        │        │
│     │   (阿里云/AWS)      │         │   (腾讯云/GCP)      │        │
│     │                     │         │                     │        │
│     │  公网: 1.2.3.4     │◄───────►│  公网: 5.6.7.8     │        │
│     │  端口: 4001/UDP     │  QUIC   │  端口: 4001/UDP     │        │
│     │                     │         │                     │        │
│     │  known_peers:       │         │  known_peers:       │        │
│     │    - Server B       │         │    - Server A       │        │
│     │                     │         │                     │        │
│     │  trust_stun: true   │         │  trust_stun: true   │        │
│     └─────────────────────┘         └─────────────────────┘        │
│                                                                     │
│  特点：                                                             │
│  • 双向配置 known_peers，启动即连接                                  │
│  • trust_stun_addresses 跳过入站验证                                 │
│  • 无需 Bootstrap 节点                                              │
│  • 无需 NAT 穿透                                                    │
└─────────────────────────────────────────────────────────────────────┘
```

---

## 前置条件

- 两台云服务器（可以是不同云厂商）
- 每台服务器有公网 IP
- 防火墙开放 4001/UDP 端口
- Go 1.21 或更高版本

---

## 步骤 1：准备云服务器

### 1.1 开放防火墙端口

**阿里云安全组**：

```bash
# 入站规则：UDP 4001
协议: UDP
端口: 4001
授权对象: 0.0.0.0/0
```

**AWS 安全组**：

```bash
# 入站规则
类型: 自定义 UDP
端口范围: 4001
来源: 0.0.0.0/0
```

**Linux 防火墙**：

```bash
# Ubuntu/Debian
sudo ufw allow 4001/udp

# CentOS/RHEL
sudo firewall-cmd --zone=public --add-port=4001/udp --permanent
sudo firewall-cmd --reload
```

### 1.2 获取公网 IP

```bash
# 获取公网 IP
curl ifconfig.me
# 或
curl ip.sb
```

记录两台服务器的公网 IP：
- 服务器 A: `1.2.3.4`（示例）
- 服务器 B: `5.6.7.8`（示例）

---

## 步骤 2：创建配置文件

### 服务器 A 配置 (`config_a.json`)

```json
{
  "preset": "server",
  "listen_port": 4001,
  
  "identity": {
    "key_file": "/etc/dep2p/identity_a.key"
  },
  
  "known_peers": [
    {
      "peer_id": "待填入服务器B的PeerID",
      "addrs": ["/ip4/5.6.7.8/udp/4001/quic-v1"]
    }
  ],
  
  "reachability": {
    "trust_stun_addresses": true
  },
  
  "disconnect_detection": {
    "quic": {
      "keep_alive_period": "3s",
      "max_idle_timeout": "6s"
    },
    "reconnect_grace_period": "15s"
  },
  
  "connection_limits": {
    "low": 100,
    "high": 500
  }
}
```

### 服务器 B 配置 (`config_b.json`)

```json
{
  "preset": "server",
  "listen_port": 4001,
  
  "identity": {
    "key_file": "/etc/dep2p/identity_b.key"
  },
  
  "known_peers": [
    {
      "peer_id": "待填入服务器A的PeerID",
      "addrs": ["/ip4/1.2.3.4/udp/4001/quic-v1"]
    }
  ],
  
  "reachability": {
    "trust_stun_addresses": true
  },
  
  "disconnect_detection": {
    "quic": {
      "keep_alive_period": "3s",
      "max_idle_timeout": "6s"
    },
    "reconnect_grace_period": "15s"
  },
  
  "connection_limits": {
    "low": 100,
    "high": 500
  }
}
```

---

## 步骤 3：编写服务端代码

创建文件 `server/main.go`：

```go
package main

import (
    "context"
    "encoding/json"
    "flag"
    "fmt"
    "log"
    "os"
    "os/signal"
    "syscall"
    "time"

    "github.com/dep2p/go-dep2p"
    "github.com/dep2p/go-dep2p/config"
    "github.com/dep2p/go-dep2p/pkg/types"
)

// 服务协议
const (
    heartbeatProtocol = "/cloud/heartbeat/1.0.0"
    messageProtocol   = "/cloud/message/1.0.0"
)

// HeartbeatMessage 心跳消息
type HeartbeatMessage struct {
    From      string    `json:"from"`
    Timestamp time.Time `json:"timestamp"`
    Uptime    string    `json:"uptime"`
}

func main() {
    // 命令行参数
    configFile := flag.String("config", "config.json", "配置文件路径")
    serverName := flag.String("name", "Server", "服务器名称")
    flag.Parse()

    fmt.Println("╔════════════════════════════════════════╗")
    fmt.Printf("║      DeP2P 云服务器 - %s           ║\n", *serverName)
    fmt.Println("╚════════════════════════════════════════╝")
    fmt.Println()

    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()

    // 捕获中断信号
    signalCh := make(chan os.Signal, 1)
    signal.Notify(signalCh, syscall.SIGINT, syscall.SIGTERM)
    go func() {
        <-signalCh
        fmt.Println("\n收到停止信号，正在关闭...")
        cancel()
    }()

    // ========================================
    // Step 1: 加载配置
    // ========================================
    fmt.Println("Step 1: 加载配置...")
    
    configData, err := os.ReadFile(*configFile)
    if err != nil {
        log.Fatalf("读取配置文件失败: %v", err)
    }

    var userConfig dep2p.UserConfig
    if err := json.Unmarshal(configData, &userConfig); err != nil {
        log.Fatalf("解析配置失败: %v", err)
    }

    fmt.Printf("✅ 配置已加载: %s\n", *configFile)
    fmt.Printf("   监听端口: %d\n", userConfig.ListenPort)
    fmt.Printf("   已知节点: %d 个\n", len(userConfig.KnownPeers))
    fmt.Printf("   信任 STUN: %v\n", userConfig.Reachability.TrustSTUNAddresses)
    fmt.Println()

    // ========================================
    // Step 2: 启动节点
    // ========================================
    fmt.Println("Step 2: 启动节点...")

    opts := userConfig.ToOptions()
    node, err := dep2p.New(ctx, opts...)
    if err != nil {
        log.Fatalf("创建节点失败: %v", err)
    }
    defer node.Close()

    if err := node.Start(ctx); err != nil {
        log.Fatalf("启动节点失败: %v", err)
    }

    fmt.Printf("✅ 节点已启动\n")
    fmt.Printf("   节点 ID: %s\n", node.ID())
    fmt.Println("   监听地址:")
    for _, addr := range node.ListenAddrs() {
        fmt.Printf("      %s\n", addr)
    }
    fmt.Println()

    // ========================================
    // Step 3: 加入 Realm
    // ========================================
    fmt.Println("Step 3: 加入 Realm...")
    
    realm, err := node.Realm("cloud-cluster")
    if err != nil {
        log.Fatalf("获取 Realm 失败: %v", err)
    }
    if err := realm.Join(ctx); err != nil {
        log.Fatalf("加入 Realm 失败: %v", err)
    }

    fmt.Printf("✅ 已加入 Realm: %s\n", realmID)
    fmt.Println()

    // ========================================
    // Step 4: 注册协议处理器
    // ========================================
    fmt.Println("Step 4: 注册协议处理器...")

    startTime := time.Now()

    // 心跳处理器
    node.Endpoint().SetProtocolHandler(heartbeatProtocol, func(stream dep2p.Stream) {
        defer stream.Close()

        buf := make([]byte, 4096)
        n, err := stream.Read(buf)
        if err != nil {
            return
        }

        var hb HeartbeatMessage
        if err := json.Unmarshal(buf[:n], &hb); err != nil {
            return
        }

        fmt.Printf("\n💓 收到心跳: %s (运行时间: %s)\n", hb.From, hb.Uptime)

        // 发送响应
        response := HeartbeatMessage{
            From:      *serverName,
            Timestamp: time.Now(),
            Uptime:    time.Since(startTime).Round(time.Second).String(),
        }
        data, _ := json.Marshal(response)
        stream.Write(data)
    })

    // 消息处理器
    node.Endpoint().SetProtocolHandler(messageProtocol, func(stream dep2p.Stream) {
        defer stream.Close()

        buf := make([]byte, 4096)
        n, err := stream.Read(buf)
        if err != nil {
            return
        }

        fmt.Printf("\n📨 收到消息: %s\n", string(buf[:n]))
        fmt.Printf("   来自: %s\n", stream.RemotePeer().ShortString())

        // 发送确认
        stream.Write([]byte("ACK"))
    })

    fmt.Printf("✅ 已注册协议: %s, %s\n", heartbeatProtocol, messageProtocol)
    fmt.Println()

    // ========================================
    // Step 5: 订阅成员事件
    // ========================================
    fmt.Println("Step 5: 订阅成员事件...")

    memberEvents, err := node.Realm().SubscribeMemberEvents(ctx, realmID)
    if err != nil {
        log.Fatalf("订阅成员事件失败: %v", err)
    }

    go func() {
        for event := range memberEvents {
            switch event.Type {
            case dep2p.MemberJoined:
                fmt.Printf("\n🟢 节点上线: %s\n", event.Member.ShortString())
            case dep2p.MemberLeft:
                fmt.Printf("\n🔴 节点离线: %s\n", event.Member.ShortString())
            }
        }
    }()

    fmt.Printf("✅ 成员事件监听已启动\n")
    fmt.Println()

    // ========================================
    // Step 6: 启动心跳任务
    // ========================================
    fmt.Println("Step 6: 启动心跳任务...")

    go func() {
        ticker := time.NewTicker(30 * time.Second)
        defer ticker.Stop()

        for {
            select {
            case <-ctx.Done():
                return
            case <-ticker.C:
                sendHeartbeats(ctx, node, realmID, *serverName, startTime)
            }
        }
    }()

    fmt.Printf("✅ 心跳任务已启动 (间隔: 30s)\n")
    fmt.Println()

    // ========================================
    // 运行中
    // ========================================
    fmt.Println("════════════════════════════════════════")
    fmt.Printf("🚀 %s 已就绪！\n", *serverName)
    fmt.Println()
    fmt.Println("配置信息:")
    fmt.Printf("   节点 ID: %s\n", node.ID())
    fmt.Printf("   Realm: %s\n", realmID)
    fmt.Printf("   公网连接地址:")
    for _, addr := range node.ListenAddrs() {
        fmt.Printf("\n      %s/p2p/%s", addr, node.ID())
    }
    fmt.Println()
    fmt.Println()
    fmt.Println("复制上面的节点 ID 到其他服务器的 known_peers 配置中")
    fmt.Println()
    fmt.Println("按 Ctrl+C 停止服务")
    fmt.Println("════════════════════════════════════════")

    <-ctx.Done()
    fmt.Println("服务已停止")
}

// sendHeartbeats 向所有成员发送心跳
func sendHeartbeats(ctx context.Context, node dep2p.Node, realmID types.RealmID, serverName string, startTime time.Time) {
    members := node.Realm().Members(realmID)
    
    for _, memberID := range members {
        if memberID == node.ID() {
            continue // 跳过自己
        }

        go func(targetID types.NodeID) {
            sendCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
            defer cancel()

            stream, err := node.OpenStream(sendCtx, targetID, heartbeatProtocol)
            if err != nil {
                return
            }
            defer stream.Close()

            hb := HeartbeatMessage{
                From:      serverName,
                Timestamp: time.Now(),
                Uptime:    time.Since(startTime).Round(time.Second).String(),
            }
            data, _ := json.Marshal(hb)
            stream.Write(data)

            // 读取响应
            buf := make([]byte, 4096)
            stream.SetReadDeadline(time.Now().Add(3 * time.Second))
            n, err := stream.Read(buf)
            if err != nil {
                return
            }

            var response HeartbeatMessage
            if json.Unmarshal(buf[:n], &response) == nil {
                fmt.Printf("💓 %s 心跳响应 OK (运行时间: %s)\n", response.From, response.Uptime)
            }
        }(memberID)
    }
}
```

---

## 步骤 4：部署流程

### 4.1 第一次部署（获取 PeerID）

**在服务器 A 上**：

```bash
# 创建目录
sudo mkdir -p /etc/dep2p
cd /opt/dep2p

# 复制代码和临时配置（不配置 known_peers）
# 先启动获取 PeerID
go run main.go -config config_a.json -name "ServerA"
```

记录输出的 PeerID，例如：`12D3KooWAbCdEfGhIjKlMnOpQrStUvWxYz123456789`

**在服务器 B 上**：

```bash
# 同样操作
go run main.go -config config_b.json -name "ServerB"
```

记录输出的 PeerID。

### 4.2 更新配置文件

**更新服务器 A 的 `config_a.json`**：

```json
{
  "known_peers": [
    {
      "peer_id": "服务器B的PeerID",
      "addrs": ["/ip4/5.6.7.8/udp/4001/quic-v1"]
    }
  ]
}
```

**更新服务器 B 的 `config_b.json`**：

```json
{
  "known_peers": [
    {
      "peer_id": "服务器A的PeerID",
      "addrs": ["/ip4/1.2.3.4/udp/4001/quic-v1"]
    }
  ]
}
```

### 4.3 使用 systemd 部署

创建服务文件 `/etc/systemd/system/dep2p.service`：

```ini
[Unit]
Description=DeP2P Node Service
After=network.target

[Service]
Type=simple
User=dep2p
Group=dep2p
WorkingDirectory=/opt/dep2p
ExecStart=/opt/dep2p/server -config /etc/dep2p/config.json -name "ServerA"
Restart=always
RestartSec=10
LimitNOFILE=65535

# 环境变量
Environment=GOMAXPROCS=4

[Install]
WantedBy=multi-user.target
```

启动服务：

```bash
# 编译
go build -o /opt/dep2p/server ./main.go

# 创建用户
sudo useradd -r -s /bin/false dep2p

# 设置权限
sudo chown -R dep2p:dep2p /opt/dep2p /etc/dep2p

# 启动服务
sudo systemctl daemon-reload
sudo systemctl enable dep2p
sudo systemctl start dep2p

# 查看日志
sudo journalctl -u dep2p -f
```

---

## 步骤 5：验证部署

### 5.1 检查节点状态

```bash
# 查看服务状态
sudo systemctl status dep2p

# 查看日志
sudo journalctl -u dep2p -n 100

# 预期看到
# 🟢 节点上线: 12D3KooW...
# 💓 ServerB 心跳响应 OK (运行时间: 1h30m)
```

### 5.2 网络连通性测试

```bash
# 从服务器 A ping 服务器 B 的 UDP 端口
nc -u 5.6.7.8 4001

# 使用 curl 检查端口（QUIC 不支持 HTTP，但可以检查 UDP 响应）
```

---

## 关键配置说明

### known_peers

```json
"known_peers": [
  {
    "peer_id": "12D3KooW...",
    "addrs": ["/ip4/5.6.7.8/udp/4001/quic-v1"]
  }
]
```

**作用**：
- 启动时自动连接指定节点
- 无需依赖 Bootstrap 或 DHT 发现
- 适合固定 IP 的云服务器

**与 Bootstrap 的区别**：

| 特性 | known_peers | Bootstrap |
|------|-------------|-----------|
| 用途 | 直接连接 | DHT 引导 |
| 依赖 | 无 | Bootstrap 节点运行 |
| 地址要求 | 完整地址 | 完整地址 |
| 适用场景 | 私有集群 | 公共网络 |

### trust_stun_addresses

```json
"reachability": {
  "trust_stun_addresses": true
}
```

**作用**：
- 信任 STUN 探测发现的公网地址
- 跳过入站连接验证
- 加速地址发布

**为什么云服务器需要这个配置**：
1. 云服务器有真实公网 IP
2. STUN 探测的地址是准确的
3. 不需要等待入站连接验证
4. 可以更快地被其他节点发现

### disconnect_detection

```json
"disconnect_detection": {
  "quic": {
    "keep_alive_period": "3s",
    "max_idle_timeout": "6s"
  },
  "reconnect_grace_period": "15s"
}
```

**云服务器推荐配置**：
- `keep_alive_period`: 3s（频繁检测）
- `max_idle_timeout`: 6s（快速发现断开）
- `reconnect_grace_period`: 15s（允许短暂网络抖动）

---

## 监控与运维

### Prometheus 指标

```bash
# 暴露指标端口
curl http://localhost:9090/metrics

# 关键指标
dep2p_connections_total{type="direct"}
dep2p_connections_active
dep2p_bandwidth_in_bytes_total
dep2p_bandwidth_out_bytes_total
dep2p_disconnect_latency_seconds
```

### 日志分析

```bash
# 查看连接日志
journalctl -u dep2p | grep "节点上线\|节点离线"

# 查看心跳日志
journalctl -u dep2p | grep "心跳"

# 查看错误
journalctl -u dep2p | grep -i "error\|failed"
```

### 健康检查脚本

```bash
#!/bin/bash
# /opt/dep2p/health_check.sh

# 检查服务是否运行
if ! systemctl is-active --quiet dep2p; then
    echo "ERROR: dep2p service is not running"
    exit 1
fi

# 检查连接数
CONNECTIONS=$(journalctl -u dep2p -n 100 | grep -c "节点上线")
if [ "$CONNECTIONS" -lt 1 ]; then
    echo "WARNING: No peer connections"
    exit 1
fi

echo "OK: dep2p is healthy"
exit 0
```

---

## 故障排查

### 问题 1：节点无法互联

**症状**：启动后看不到"节点上线"日志

**排查步骤**：

```bash
# 1. 检查防火墙
sudo ufw status
sudo iptables -L -n | grep 4001

# 2. 检查端口监听
ss -ulnp | grep 4001

# 3. 测试网络连通性
nc -vzu 5.6.7.8 4001

# 4. 检查配置
cat /etc/dep2p/config.json | jq '.known_peers'
```

### 问题 2：连接不稳定

**症状**：频繁的上线/下线事件

**解决方案**：

```json
{
  "disconnect_detection": {
    "quic": {
      "keep_alive_period": "5s",
      "max_idle_timeout": "15s"
    },
    "reconnect_grace_period": "30s",
    "flapping": {
      "enabled": true,
      "window": "60s",
      "threshold": 3,
      "cooldown": "120s"
    }
  }
}
```

### 问题 3：PeerID 不匹配

**症状**：日志显示 "peer id mismatch"

**原因**：known_peers 中配置的 PeerID 与实际不符

**解决方案**：重新获取正确的 PeerID 并更新配置

---

## 扩展部署

### 添加第三台服务器

当需要添加服务器 C 时：

1. 部署服务器 C，获取其 PeerID
2. 更新服务器 A 和 B 的配置，添加服务器 C 到 known_peers
3. 配置服务器 C 的 known_peers，包含 A 和 B
4. 重启所有服务

```json
// 服务器 A/B/C 的 known_peers 都应包含其他两个节点
"known_peers": [
  { "peer_id": "ServerA-PeerID", "addrs": ["/ip4/1.2.3.4/udp/4001/quic-v1"] },
  { "peer_id": "ServerB-PeerID", "addrs": ["/ip4/5.6.7.8/udp/4001/quic-v1"] },
  { "peer_id": "ServerC-PeerID", "addrs": ["/ip4/9.10.11.12/udp/4001/quic-v1"] }
]
```

---

## 下一步

- [Realm 群聊](04-realm-chat.md) - 使用 Realm 构建群组应用
- [故障排查](05-troubleshooting-live.md) - 使用日志分析框架排查问题
- [配置参考](../reference/configuration.md) - 完整配置选项说明
