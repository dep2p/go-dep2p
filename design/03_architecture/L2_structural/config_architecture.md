# 配置架构 (Configuration Architecture)

> **版本**: v1.1.0  
> **更新日期**: 2026-01-23  
> **定位**: DeP2P 配置管理架构设计

---

## 概述

本文档定义 DeP2P 的**配置管理架构**，采用**双层配置模型**：

- **系统级配置** - 框架内部硬编码，不对外暴露
- **用户级配置** - 用户可通过 JSON/ENV/CLI 配置

---

## 双层配置模型

```
┌─────────────────────────────────────────────────────────────────────────┐
│                           配置层次结构                                    │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│  ┌───────────────────────────────────────────────────────────────────┐  │
│  │                    用户级配置 (User Config)                        │  │
│  │                                                                   │  │
│  │  特征：                                                           │  │
│  │  • 用户可感知、可修改                                              │  │
│  │  • 通过 JSON 文件、环境变量、命令行参数配置                         │  │
│  │  • 有合理的默认值                                                  │  │
│  │  • 提供校验和错误提示                                              │  │
│  │                                                                   │  │
│  │  示例：                                                           │  │
│  │  • 监听端口、密钥文件路径                                          │  │
│  │  • Bootstrap 节点列表                                             │  │
│  │  • 功能开关 (EnableXXX)                                           │  │
│  │  • 连接限制 (HighWater/LowWater)                                  │  │
│  │  • 超时时间                                                       │  │
│  └───────────────────────────────────────────────────────────────────┘  │
│                                 │                                       │
│                                 ▼                                       │
│  ┌───────────────────────────────────────────────────────────────────┐  │
│  │                   系统级配置 (System Config)                       │  │
│  │                                                                   │  │
│  │  特征：                                                           │  │
│  │  • 框架内部使用，不对外暴露                                         │  │
│  │  • 代码硬编码，不可通过外部配置修改                                  │  │
│  │  • 涉及协议规范、安全边界、内部算法                                  │  │
│  │                                                                   │  │
│  │  示例：                                                           │  │
│  │  • 协议版本号、Magic Numbers                                       │  │
│  │  • TLS 最低版本 (1.3)                                             │  │
│  │  • 内部超时倍数关系                                                │  │
│  │  • 安全参数边界值                                                  │  │
│  │  • 内部缓冲区大小                                                  │  │
│  └───────────────────────────────────────────────────────────────────┘  │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

---

## 配置来源优先级

用户级配置支持多种来源，按以下优先级（从高到低）：

```
命令行参数 (CLI)
      │
      ▼
环境变量 (ENV)
      │
      ▼
配置文件 (JSON)
      │
      ▼
预设配置 (Preset)
      │
      ▼
默认值 (Default)
```

| 来源 | 优先级 | 说明 |
|------|--------|------|
| CLI | 最高 | 命令行参数，如 `--port 4001` |
| ENV | 高 | 环境变量，如 `DEP2P_LISTEN_PORT=4001` |
| JSON | 中 | 配置文件，如 `config.json` |
| Preset | 低 | 预设配置，如 `mobile`/`server` |
| Default | 最低 | 代码中的默认值 |

---

## 用户级配置分类

### 1. 身份配置 (Identity)

| 字段 | JSON 键 | 说明 | 默认值 |
|------|---------|------|--------|
| KeyType | `key_type` | 密钥类型 | `"Ed25519"` |
| KeyFile | `key_file` | 密钥文件路径 | `""` (内存生成) |
| AutoGenerate | `auto_generate` | 自动生成密钥 | `true` |

### 2. 传输配置 (Transport)

| 字段 | JSON 键 | 说明 | 默认值 |
|------|---------|------|--------|
| EnableQUIC | `enable_quic` | 启用 QUIC | `true` |
| EnableTCP | `enable_tcp` | 启用 TCP | `true` |
| DialTimeout | `dial_timeout` | 拨号超时 | `"30s"` |

### 3. 连接管理 (ConnMgr)

| 字段 | JSON 键 | 说明 | 默认值 |
|------|---------|------|--------|
| LowWater | `low_water` | 低水位连接数 | `100` |
| HighWater | `high_water` | 高水位连接数 | `400` |
| GracePeriod | `grace_period` | 新连接保护期 | `"20s"` |

### 4. NAT 配置

| 字段 | JSON 键 | 说明 | 默认值 |
|------|---------|------|--------|
| EnableAutoNAT | `enable_autonat` | 启用 AutoNAT（可达性检测） | `true` |
| EnableUPnP | `enable_upnp` | 启用 UPnP 端口映射 | `true` |
| EnableHolePunch | `enable_holepunch` | 启用打洞（★ 需要 Relay 信令通道） | `true` |
| IPv6NATMode | `ipv6_nat_mode` | ★ IPv6 穿透模式 | `"simplified"` |

> **★ NAT 三层能力说明**：
> - 外部地址发现（STUN/观察地址）≠ 可发布地址，需可达性验证
> - 打洞需要信令通道（通常由 Relay 连接提供）
> - 若启用 `EnableHolePunch` 但未启用 Relay Client，打洞可能不可用
>
> **★ IPv6 穿透模式**：
> - `full`：完整 NAT 检测流程（保守，兼容 NAT64/DS-Lite）
> - `simplified`：跳过 NAT 类型检测，保留可达性检测（推荐）
> - `disabled`：完全跳过（仅确认纯 IPv6 无防火墙环境）

### 5. 发现配置 (Discovery)

| 字段 | JSON 键 | 说明 | 默认值 |
|------|---------|------|--------|
| EnableDHT | `enable_dht` | 启用 DHT（★ 仅发布可达地址） | `true` |
| EnableMDNS | `enable_mdns` | 启用 mDNS | `true` |
| Bootstrap.Peers | `bootstrap.peers` | 引导节点 | `[]` |

### 6. 中继配置 (Relay)

| 字段 | JSON 键 | 说明 | 默认值 |
|------|---------|------|--------|
| EnableClient | `enable_client` | 启用 Relay 客户端（缓存加速 + 信令 + 保底） | `true` |
| EnableServer | `enable_server` | 启用 Relay 服务端（缓存地址簿 + 数据转发） | `false` |
| KeepConnection | `keep_connection` | 打洞成功后保留 Relay 作为备份 | `true` |

> **★ Relay 三大职责 (v2.0)**：
> - 缓存加速层：维护地址簿，作为 DHT 本地缓存（非权威，DHT 是权威目录）
> - 数据通信保底：直连/打洞失败时转发数据
> - 信令通道：打洞协调的前置依赖
> - 惰性连接：配置 ≠ 立即连接，按需建立

### 7. 资源配置 (Resource)

| 字段 | JSON 键 | 说明 | 默认值 |
|------|---------|------|--------|
| EnableResourceManager | `enable_resource_manager` | 启用资源管理 | `true` |
| System.MaxConnections | `system.max_connections` | 最大连接数 | `1000` |
| System.MaxStreams | `system.max_streams` | 最大流数 | `10000` |

### 8. 存储配置 (Storage)

| 字段 | JSON 键 | 说明 | 默认值 |
|------|---------|------|--------|
| DataDir | `data_dir` | 数据目录路径 | `"./data"` |
| LogDir | `log_dir` | 日志目录路径 | `"${DataDir}/logs"` |

**说明**：
- 所有组件统一使用 BadgerDB 持久化存储
- 不提供内存模式选项
- DataDir 是必需的，默认为当前目录下的 `data/`
- 数据库文件存储在 `${DataDir}/dep2p.db/`

---

## 系统级配置（硬编码）

以下配置**不对外暴露**，在代码中硬编码：

### 协议相关

```pseudocode
const:
    // 协议版本
    ProtocolVersion = "dep2p/1.0.0"
    
    // 系统协议 ID
    PingProtocolID     = "/dep2p/ping/1.0.0"
    IdentifyProtocolID = "/dep2p/identify/1.0.0"
```

### 安全边界

```pseudocode
const:
    // TLS 最低版本 - 强制 TLS 1.3
    TLSMinVersion = TLS_1_3
    
    // RSA 最小密钥位数
    RSAMinBits = 2048
    
    // 签名验证超时
    SignatureVerifyTimeout = 5s
```

### 内部算法参数

```pseudocode
const:
    // DHT K-bucket 大小（Kademlia 规范）
    KBucketSize = 20
    
    // DHT Alpha 并发度
    DHTAlpha = 3
    
    // 连接衰减基础间隔
    DecayBaseInterval = 1min
```

---

## 配置文件结构

### JSON 配置示例

```json
{
  "identity": {
    "key_file": "~/.dep2p/identity.key",
    "key_type": "Ed25519"
  },
  
  "transport": {
    "enable_quic": true,
    "enable_tcp": true,
    "dial_timeout": "30s"
  },
  
  "conn_mgr": {
    "low_water": 100,
    "high_water": 400,
    "grace_period": "20s"
  },
  
  "discovery": {
    "enable_dht": true,
    "enable_mdns": true,
    "bootstrap": {
      "peers": [
        "/dns4/bootstrap.dep2p.io/udp/4001/quic-v1/p2p/12D3KooW..."
      ]
    }
  },
  
  "nat": {
    "enable_autonat": true,
    "enable_upnp": true,
    "enable_holepunch": true
  },
  
  "relay": {
    "enable_client": true,
    "enable_server": false
  },
  
  "resource": {
    "enable_resource_manager": true,
    "system": {
      "max_connections": 1000
    }
  },
  
  "storage": {
    "data_dir": "./data"
  }
}
```

### 环境变量映射

| 环境变量 | 配置项 |
|----------|--------|
| `DEP2P_IDENTITY_KEY_FILE` | `identity.key_file` |
| `DEP2P_LISTEN_PORT` | 运行时参数 |
| `DEP2P_BOOTSTRAP_PEERS` | `discovery.bootstrap.peers` |
| `DEP2P_ENABLE_RELAY` | `relay.enable_client` |
| `DEP2P_ENABLE_NAT` | `nat.enable_autonat` 等 |
| `DEP2P_DATA_DIR` | `storage.data_dir` |

---

## 预设配置 (Presets)

提供四种预设配置，覆盖常见场景：

| 预设 | 适用场景 | 特点 |
|------|----------|------|
| `minimal` | 测试/开发 | 最小功能集，资源占用低 |
| `desktop` | 桌面应用 | 平衡资源与功能 |
| `server` | 服务器节点 | 高连接数，启用中继服务 |
| `mobile` | 移动设备 | 省电，低资源占用 |

### 预设差异对比

| 配置项 | minimal | default | server | mobile |
|--------|---------|---------|--------|--------|
| HighWater | 50 | 400 | 2000 | 100 |
| LowWater | 10 | 100 | 500 | 20 |
| EnableDHT | ✗ | ✓ | ✓ | ✓ |
| EnableMDNS | ✓ | ✓ | ✓ | ✓ |
| EnableRelay | ✗ | Client | Server | Client |
| EnableHolePunch | ✓ | ✓ | ✓ | ✓ |

---

## 代码结构

```
config/
├── config.go          # Config 主结构体（嵌入所有子配置）
├── defaults.go        # 预设配置工厂函数
├── validate.go        # 配置校验
├── convert.go         # JSON 转换
│
├── identity.go        # IdentityConfig
├── transport.go       # TransportConfig
├── security.go        # SecurityConfig
├── nat.go             # NATConfig
├── relay.go           # RelayConfig
├── discovery.go       # DiscoveryConfig
├── connmgr.go         # ConnManagerConfig
├── messaging.go       # MessagingConfig
├── realm.go           # RealmConfig
├── resource.go        # ResourceConfig
└── storage.go         # StorageConfig
```

---

## 配置流转

```
┌──────────────────────────────────────────────────────────────────────┐
│                        配置流转流程                                    │
├──────────────────────────────────────────────────────────────────────┤
│                                                                      │
│  1. 加载阶段                                                          │
│  ┌────────────┐    ┌────────────┐    ┌────────────┐                  │
│  │ Default    │───▶│  Preset    │───▶│   JSON     │                  │
│  │ Config     │    │  Apply     │    │   Load     │                  │
│  └────────────┘    └────────────┘    └────────────┘                  │
│        │                 │                 │                         │
│        └─────────────────┴─────────────────┘                         │
│                          │                                           │
│                          ▼                                           │
│  2. 覆盖阶段                                                          │
│  ┌────────────┐    ┌────────────┐                                    │
│  │   ENV      │───▶│   CLI      │                                    │
│  │  Override  │    │  Override  │                                    │
│  └────────────┘    └────────────┘                                    │
│                          │                                           │
│                          ▼                                           │
│  3. 校验阶段                                                          │
│  ┌────────────┐                                                      │
│  │  Validate  │───▶ 校验通过 ───▶ 使用配置                            │
│  │  Config    │                                                      │
│  └────────────┘                                                      │
│        │                                                             │
│        └───▶ 校验失败 ───▶ 返回错误                                   │
│                                                                      │
│  4. 注入阶段                                                          │
│  ┌────────────┐    ┌────────────┐                                    │
│  │ Fx Supply  │───▶│  各模块    │                                    │
│  │ (*Config)  │    │  读取配置  │                                    │
│  └────────────┘    └────────────┘                                    │
│                                                                      │
└──────────────────────────────────────────────────────────────────────┘
```

---

## 模块配置读取模式

内部模块通过 `ConfigFromUnified` 从统一配置读取：

```pseudocode
// internal/core/connmgr/module.go

// ConfigFromUnified 从统一配置创建连接管理配置
function ConfigFromUnified(cfg: Config) -> Config:
    if cfg == nil:
        return DefaultConfig()
    return Config{
        LowWater:      cfg.ConnMgr.LowWater,
        HighWater:     cfg.ConnMgr.HighWater,
        GracePeriod:   cfg.ConnMgr.GracePeriod,
        DecayInterval: cfg.ConnMgr.DecayInterval
    }

// Params Fx 模块输入
struct Params:
    UnifiedCfg: Config [optional]

// ProvideConnManager 提供连接管理器
function ProvideConnManager(p: Params) -> ConnManager:
    cfg = ConfigFromUnified(p.UnifiedCfg)
    return New(cfg)
```

---

## 相关文档

| 文档 | 说明 |
|------|------|
| [target_structure.md](target_structure.md) | 目录结构 |
| [fx_lifecycle.md](../L4_interfaces/fx_lifecycle.md) | Fx 生命周期 |
| [../../config/README.md](../../../config/README.md) | 配置包说明 |

---

**最后更新**：2026-01-17  
**架构版本**：v1.1.0
