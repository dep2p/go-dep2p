# DeP2P 配置包

> **版本**: v1.0.0  
> **更新日期**: 2026-01-15

---

## 概述

`config` 包提供 DeP2P 的统一配置管理，采用**双层配置模型**：

- **用户级配置** - 可通过 JSON/ENV/CLI 配置
- **系统级配置** - 框架内部硬编码

---

## 快速开始

### 创建默认配置

```go
import "github.com/dep2p/go-dep2p/config"

// 创建默认配置
cfg := config.NewConfig()
```

### 使用预设配置

```go
// 服务器预设
cfg := config.NewServerConfig()

// 移动端预设
cfg := config.NewMobileConfig()

// 最小化预设
cfg := config.NewMinimalConfig()
```

### 从 JSON 加载

```go
data, _ := os.ReadFile("config.json")
cfg, err := config.FromJSON(data)
if err != nil {
    log.Fatal(err)
}
```

### 应用预设到现有配置

```go
cfg := config.NewConfig()
config.ApplyPreset(cfg, "server")
```

---

## 配置来源优先级

```
CLI > ENV > JSON > Preset > Default
```

---

## 配置文件示例

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
    "high_water": 400
  },
  "discovery": {
    "enable_dht": true,
    "enable_mdns": true,
    "bootstrap_peers": []
  },
  "nat": {
    "enable_autonat": true,
    "enable_upnp": true
  },
  "relay": {
    "enable_client": true,
    "enable_server": false
  }
}
```

---

## 环境变量

| 环境变量 | 说明 |
|----------|------|
| `DEP2P_KEY_FILE` | 密钥文件路径 |
| `DEP2P_LISTEN_PORT` | 监听端口 |
| `DEP2P_BOOTSTRAP_PEERS` | 引导节点（逗号分隔） |
| `DEP2P_ENABLE_RELAY` | 启用中继 |
| `DEP2P_ENABLE_NAT` | 启用 NAT |

---

## 预设配置

| 预设 | 适用场景 |
|------|----------|
| `minimal` | 测试/开发，最小功能 |
| `desktop` | 桌面应用，平衡配置 |
| `server` | 服务器节点，高性能 |
| `mobile` | 移动设备，省电模式 |

---

## 文件结构

```
config/
├── config.go      # Config 主结构体
├── defaults.go    # 预设工厂函数
├── validate.go    # 配置校验
├── convert.go     # JSON 转换
├── identity.go    # 身份配置
├── transport.go   # 传输配置
├── security.go    # 安全配置
├── nat.go         # NAT 配置
├── relay.go       # 中继配置
├── discovery.go   # 发现配置
├── connmgr.go     # 连接管理配置
├── messaging.go   # 消息配置
├── realm.go       # Realm 配置
└── resource.go    # 资源配置
```

---

## 相关文档

- [配置架构设计](../design/03_architecture/L2_structural/config_architecture.md)
