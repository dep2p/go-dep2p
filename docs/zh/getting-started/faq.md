# 常见问题

本文档汇总了 DeP2P 使用过程中的常见问题和解决方案。

---

## 快速索引

| 问题类型 | 常见问题 |
|----------|----------|
| [连接问题](#连接问题) | 连接超时、dial backoff、peer id mismatch |
| [Realm 问题](#realm-问题) | ErrNotMember、ErrAlreadyJoined |
| [地址问题](#地址问题) | 无公网地址、地址格式 |
| [配置问题](#配置问题) | known_peers、Bootstrap、预设选择 |
| [性能问题](#性能问题) | 连接数、内存、断开检测 |

---

## 连接问题

### Q: 连接超时

**症状**：`dial timeout` 或长时间等待

**可能原因**：
1. 目标节点不在线
2. 网络不通或防火墙阻止
3. DHT 未找到地址
4. known_peers 配置错误

**解决方案**：

```go
// 1. 检查 known_peers 配置
node, _ := dep2p.New(ctx, dep2p.WithKnownPeers(
    config.KnownPeer{
        PeerID: "12D3KooWxxxxx...",  // 确保完整正确
        Addrs:  []string{"/ip4/127.0.0.1/udp/4001/quic-v1"},
    },
))

// 2. 使用完整地址直接连接（跳过发现）
fullAddr := "/ip4/1.2.3.4/udp/4001/quic-v1/p2p/12D3KooWxxxxx..."
conn, err := node.ConnectToAddr(ctx, fullAddr)

// 3. 增加超时时间
ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()
```

### Q: dial backoff 错误

**症状**：`dial backoff` 错误信息

**原因**：连接频率过高触发退避机制

**解决方案**：
- 等待退避时间结束（通常几秒到几分钟）
- 检查是否有重复连接逻辑
- 使用 `node.IsConnected()` 先检查连接状态

### Q: peer id mismatch 错误

**症状**：`peer id mismatch` 错误

**原因**：known_peers 或 Bootstrap 中配置的 PeerID 与实际节点不符

**解决方案**：
1. 确认目标节点的真实 PeerID
2. 更新配置中的 PeerID

```bash
# 获取节点真实 ID
# 在目标节点运行时查看日志或调用 node.ID()
```

### Q: 无法发现其他节点

**症状**：节点启动后看不到其他节点

**排查步骤**：

```go
// 1. 检查是否在同一 Realm
fmt.Printf("当前 Realm: %s\n", node.Realm().CurrentRealm())

// 2. 检查 mDNS 是否启用（PresetDesktop 默认启用）
// 如果使用 PresetMinimal，mDNS 是禁用的

// 3. 检查成员列表
members := node.Realm().Members(realmID)
fmt.Printf("当前成员: %d\n", len(members))

// 4. 使用 known_peers 直接连接
node, _ := dep2p.New(ctx, 
    dep2p.WithPreset(dep2p.PresetDesktop),
    dep2p.WithKnownPeers(config.KnownPeer{
        PeerID: "12D3KooW...",
        Addrs: []string{"/ip4/192.168.1.100/udp/4001/quic-v1"},
    }),
)
```

---

## Realm 问题

### Q: ErrNotMember 错误

**症状**：调用业务 API 返回 `ErrNotMember`

**原因**：未加入 Realm 就调用业务 API

**解决方案**：

```go
// ❌ 错误
node, _ := dep2p.New(ctx, dep2p.WithPreset(dep2p.PresetDesktop))
node.Start(ctx)
err := node.PubSub().Publish(ctx, "topic", data)  // ErrNotMember

// ✅ 正确
node, _ := dep2p.New(ctx, dep2p.WithPreset(dep2p.PresetDesktop))
node.Start(ctx)
realm, _ := node.Realm("my-realm")
_ = realm.Join(ctx)  // 先加入
err := node.PubSub().Publish(ctx, "topic", data)  // 成功
```

### Q: ErrAlreadyJoined 错误

**症状**：加入 Realm 返回 `ErrAlreadyJoined`

**原因**：已在某个 Realm 中，又尝试加入另一个

**解决方案**：

```go
// 检查当前状态
current := node.Realm().CurrentRealm()
if current != "" {
    // 先离开当前 Realm
    node.Realm().LeaveRealm(ctx)
}
// 再加入新 Realm
realm, _ := node.Realm("new-realm")
_ = realm.Join(ctx)
```

### Q: 成员加入/离开事件延迟

**原因**：断开检测有宽限期，防止网络抖动误报

**调优**：

```json
{
  "disconnect_detection": {
    "reconnect_grace_period": "10s",  // 减小宽限期
    "quic": {
      "keep_alive_period": "3s",
      "max_idle_timeout": "6s"
    }
  }
}
```

---

## 地址问题

### Q: 无法获取公网地址

**原因**：
1. NAT 探测需要时间
2. 防火墙阻止 STUN
3. 需要 Relay 支持

**解决方案**：

```go
// 1. 等待地址就绪
time.Sleep(5 * time.Second)
fmt.Println("监听地址:", node.ListenAddrs())

// 2. 云服务器场景：启用 trust_stun_addresses
node, _ := dep2p.New(ctx,
    dep2p.WithPreset(dep2p.PresetServer),
    dep2p.WithTrustSTUNAddresses(true),
)

// 3. 检查防火墙
// 确保 UDP 端口开放
```

### Q: 地址格式说明

**Full Address（完整地址）**：

```
/ip4/192.168.1.1/udp/4001/quic-v1/p2p/12D3KooWxxxxx...
                                      ↑
                                   NodeID 部分
```

**Dial Address（拨号地址）**：

```
/ip4/192.168.1.1/udp/4001/quic-v1
```

- **Full Address**：用于 Bootstrap、known_peers、分享给用户
- **Dial Address**：可用于 known_peers 的 addrs 字段

---

## 配置问题

### Q: known_peers 何时使用？

**适用场景**：
- 私有集群中的节点互联
- 云服务器部署
- 不依赖公共 Bootstrap 节点

**配置示例**：

```go
node, _ := dep2p.New(ctx,
    dep2p.WithPreset(dep2p.PresetDesktop),
    dep2p.WithKnownPeers(
        config.KnownPeer{
            PeerID: "12D3KooWxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
            Addrs:  []string{"/ip4/1.2.3.4/udp/4001/quic-v1"},
        },
    ),
)
```

### Q: Bootstrap vs known_peers

| 特性 | known_peers | Bootstrap |
|------|-------------|-----------|
| 用途 | 直接连接已知节点 | DHT 引导发现 |
| 连接时机 | 启动即连接 | DHT 初始化后 |
| 依赖 | 仅目标节点在线 | Bootstrap 节点提供服务 |
| 适用场景 | 私有网络、固定节点 | 公共网络、动态发现 |

### Q: 如何选择预设？

| 预设 | 适用场景 | 特点 |
|------|----------|------|
| `PresetMinimal` | 测试、教程 | 禁用自动发现 |
| `PresetDesktop` | PC/笔记本 | 默认推荐 |
| `PresetServer` | 云服务器 | 高连接数 |
| `PresetMobile` | 手机/平板 | 省电优化 |

### Q: trust_stun_addresses 何时启用？

**适用场景**：
- 云服务器有真实公网 IP
- 网络配置确保入站流量可达

**不适用**：
- NAT 后的节点
- 网络环境复杂

```go
// 仅云服务器场景启用
node, _ := dep2p.New(ctx,
    dep2p.WithPreset(dep2p.PresetServer),
    dep2p.WithTrustSTUNAddresses(true),
)
```

---

## 性能问题

### Q: 连接数过多

**解决方案**：调整连接管理器水位线

```go
node, _ := dep2p.New(ctx,
    dep2p.WithPreset(dep2p.PresetDesktop),
    dep2p.WithConnectionLimits(30, 60),  // 降低限制
)
```

### Q: 内存占用过高

**解决方案**：
1. 使用低资源预设
2. 减少连接数限制

```go
// 使用移动端预设
node, _ := dep2p.New(ctx, dep2p.WithPreset(dep2p.PresetMobile))
```

### Q: 断开检测太慢

**解决方案**：调整断开检测参数

```go
// 通过配置文件
{
  "disconnect_detection": {
    "quic": {
      "keep_alive_period": "2s",
      "max_idle_timeout": "5s"
    },
    "reconnect_grace_period": "10s"
  }
}
```

---

## 调试技巧

### 启用详细日志

```go
import "github.com/dep2p/go-dep2p/pkg/lib/log"

// 设置日志级别
log.SetLevel("dep2p", log.LevelDebug)

// 或通过环境变量
// export DEP2P_LOG_LEVEL=debug
```

### 检查节点状态

```go
fmt.Printf("节点 ID: %s\n", node.ID())
fmt.Printf("监听地址: %v\n", node.ListenAddrs())
fmt.Printf("当前 Realm: %s\n", node.Realm().CurrentRealm())

// 检查连接
if node.IsConnected(targetID) {
    fmt.Println("已连接")
}
```

### 使用日志分析脚本

```bash
# 分析连接质量
./p2p-log-analyze.sh dep2p.log connection

# 分析 NAT 穿透
./p2p-log-analyze.sh dep2p.log nat

# 完整分析
./p2p-log-analyze.sh dep2p.log all
```

---

## 更多资源

- [安装](installation.md) - 安装和环境配置
- [5 分钟上手](quickstart.md) - 快速入门
- [创建第一个节点](first-node.md) - 节点配置详解
- [加入第一个 Realm](first-realm.md) - Realm 使用指南
- [故障排查教程](../tutorials/05-troubleshooting-live.md) - 实战故障排查
- [配置参考](../reference/configuration.md) - 完整配置说明
