# Core Layer (核心层)

核心层是 DeP2P 的 P2P 网络引擎，提供基础的网络功能。

## 模块分类

### 基础模块

| 模块 | 说明 | 依赖 |
|------|------|------|
| `host` | 主机服务 | swarm, protocol, nat, relay |
| `identity` | 身份管理 | - |
| `transport` | 传输层 | identity |
| `security` | 安全层 | identity |
| `muxer` | 多路复用 | transport, security |
| `connmgr` | 连接管理 | peerstore, eventbus |
| `relay` | 中继服务 | swarm, nat |
| `nat` | NAT 穿透 | swarm |

### 基础设施模块

| 模块 | 说明 | 依赖 |
|------|------|------|
| `swarm` | 连接池管理 | transport, upgrader, connmgr |
| `upgrader` | 连接升级 | security, muxer |
| `peerstore` | 节点存储 | identity |
| `eventbus` | 事件总线 | - |
| `protocol` | 协议管理 | - |
| `resourcemgr` | 资源管理 | - |
| `storage` | 存储引擎 | - |
| `metrics` | 监控指标 | swarm, discovery |
| `bandwidth` | 带宽统计 | - |

### 网络弹性模块

| 模块 | 说明 | 依赖 |
|------|------|------|
| `netmon` | 网络状态监控 | eventbus |
| `netreport` | 网络诊断报告 | - |
| `reachability` | 可达性协调 | peerstore, eventbus |
| `recovery` | 网络恢复 | swarm, netmon |
| `pathhealth` | 路径健康管理 | peerstore |
| `network` | 网络层封装 | - |

### QoS 与诊断模块

| 模块 | 说明 | 依赖 |
|------|------|------|
| `msgrate` | 消息速率跟踪 | - |
| `nodedb` | 节点数据库 | - |
| `introspect` | 自省诊断服务 | host, connmgr, bandwidth |

## 系统协议

系统协议位于 `protocol/system/` 目录：

- `identify` - 身份识别协议
- `ping` - Ping 协议
- `autonat` - NAT 自动检测
- `holepunch` - NAT 打洞
- `relay` - 中继协议 (Circuit v2)

## 架构原则

1. **依赖倒置**：模块通过接口依赖，不直接依赖实现
2. **Fx 模块化**：每个模块提供 `Module()` 函数作为 Fx 入口
3. **接口隔离**：公共接口在 `pkg/interfaces/`，内部接口在模块的 `interfaces/` 子目录
4. **日志前缀**：使用 `core/<模块名>` 格式，如 `core/introspect`

## 目录结构

```
internal/core/
├── bandwidth/       # 带宽统计
├── connmgr/         # 连接管理
├── eventbus/        # 事件总线
├── host/            # 网络主机
├── identity/        # 身份管理
├── introspect/      # 自省诊断 (QoS)
├── metrics/         # 监控指标
├── msgrate/         # 消息速率跟踪 (QoS)
├── muxer/           # 多路复用
├── nat/             # NAT 穿透
├── netmon/          # 网络监控 (弹性)
├── netreport/       # 网络诊断 (弹性)
├── network/         # 网络层封装
├── nodedb/          # 节点数据库 (QoS)
├── pathhealth/      # 路径健康 (弹性)
├── peerstore/       # 节点存储
├── protocol/        # 协议管理
├── reachability/    # 可达性协调 (弹性)
├── recovery/        # 网络恢复 (弹性)
├── relay/           # 中继服务
├── resourcemgr/     # 资源管理
├── security/        # 安全层
├── storage/         # 存储引擎
├── swarm/           # 连接池
├── transport/       # 传输层
└── upgrader/        # 连接升级
```

## 相关文档

- [domain_map.md](../../design/03_architecture/L1_overview/domain_map.md) - 领域映射图
- [assembly_architecture.md](../../design/03_architecture/L2_structural/assembly_architecture.md) - 组件装配架构
