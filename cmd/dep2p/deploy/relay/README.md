# DeP2P Relay 节点部署指南

本指南介绍如何部署一个**专职 Relay（中继）节点**，提供 NAT 穿透中继服务。

## 节点职责

| 功能 | 状态 | 说明 |
|------|------|------|
| **Bootstrap 服务** | ❌ 禁用 | 不提供节点发现服务 |
| **Relay** | ✅ 启用 | 为 NAT 后节点提供中继转发 |
| **Realm Gateway** | ✅ 可选 | 可启用跨 Realm 网关能力 |

## 目录结构

```
relay/
├── README.md              # 本文档
├── relay.config.json      # Relay 节点配置模板
├── dep2p-relay.service    # systemd 服务文件模板
└── data/                  # 运行时数据目录（自动创建）
    └── identity.key       # 节点身份密钥（自动生成）
```

## 快速开始

### 1. 编译

```bash
cd /path/to/dep2p.git
go build -o dep2p ./cmd/dep2p/
```

### 2. 准备部署目录

```bash
# 进入部署目录
cd cmd/dep2p/deploy/relay

# 创建数据目录
mkdir -p data

# 复制配置模板
cp relay.config.json config.json
```

### 3. 修改配置

编辑 `config.json`，**必须修改** Bootstrap 节点地址：

```json
{
  "discovery": {
    "bootstrap": {
      "peers": [
        "/ip4/BOOTSTRAP_IP/udp/4001/quic-v1/p2p/BOOTSTRAP_PEER_ID"
      ]
    }
  }
}
```

### 4. 启动节点

```bash
# 基本启动（端口 4002，与 Bootstrap 节点错开）
../../dep2p --config config.json --port 4002 \
  --public-addr /ip4/YOUR_PUBLIC_IP/udp/4002/quic-v1 \
  --enable-relay

# 验证启动成功后，会显示：
# Capabilities: Relay
```

### 5. systemd 服务部署

```bash
# 复制服务文件
sudo cp dep2p-relay.service /etc/systemd/system/

# 编辑修改 IP 和路径
sudo vim /etc/systemd/system/dep2p-relay.service

# 启动服务
sudo systemctl daemon-reload
sudo systemctl enable dep2p-relay
sudo systemctl start dep2p-relay

# 查看状态
sudo systemctl status dep2p-relay
```

## 配置说明

### 关键配置项

```json
{
  "discovery": {
    "enable_bootstrap": false,  // 不提供 Bootstrap 服务
    "bootstrap": {
      "enable_service": false,
      "peers": ["..."]          // 必须指定 Bootstrap 节点
    }
  },
  "relay": {
    "enable_server": true,      // 启用中继服务器
    "reservation_limit": 128,   // 最大预约数
    "reservation_ttl": "3600s", // 预约有效期
    "max_circuit_duration": "1800s",  // 单次中继最长时间
    "max_data_limit": 134217728       // 单次中继最大数据量 (128MB)
  }
}
```

### 资源配置建议

Relay 节点需要更多资源来处理中继流量：

| 场景 | max_connections | max_streams | max_memory | 带宽建议 |
|------|-----------------|-------------|------------|----------|
| 小型（<50 中继） | 500 | 5000 | 512MB | 10Mbps |
| 中型（50-200） | 1000 | 10000 | 1GB | 50Mbps |
| 大型（>200） | 2000 | 20000 | 2GB | 100Mbps+ |

### 中继参数调优

```json
{
  "relay": {
    "reservation_limit": 128,        // 同时服务的最大客户端数
    "reservation_ttl": "3600s",      // 预约有效期（1小时）
    "max_circuit_duration": "1800s", // 单次中继最长30分钟
    "max_data_limit": 134217728      // 单次最多传输 128MB
  }
}
```

## 与 Bootstrap 节点配合

Relay 节点**必须**连接到 Bootstrap 节点才能被发现：

```
┌─────────────────┐    ┌─────────────────┐
│  Bootstrap Node │    │   Relay Node    │
│   (端口 4001)   │◄───│   (端口 4002)   │
│  节点发现服务    │    │  中继转发服务    │
└─────────────────┘    └─────────────────┘
         │                     │
         │                     │
   ┌─────┴─────┐         ┌─────┴─────┐
   │ 客户端 A  │         │ 客户端 B  │
   │ (NAT 后)  │◄────────│ (NAT 后)  │
   │           │  通过中继  │           │
   └───────────┘         └───────────┘
```

## 同机部署多节点

如果 Bootstrap 和 Relay 部署在同一台服务器：

```bash
# 终端 1：启动 Bootstrap（端口 4001）
cd ../bootstrap
./dep2p --config config.json --port 4001 \
  --public-addr /ip4/YOUR_IP/udp/4001/quic-v1

# 终端 2：启动 Relay（端口 4002）
cd ../relay
./dep2p --config config.json --port 4002 \
  --public-addr /ip4/YOUR_IP/udp/4002/quic-v1
```

**注意事项**：
- 两个节点必须使用不同端口
- 两个节点必须使用不同的数据目录（不同的 identity.key）
- 建议使用独立日志目录

## 监控指标

关键监控指标：

| 指标 | 说明 | 告警阈值 |
|------|------|----------|
| 活跃预约数 | 当前中继客户端数 | > 80% reservation_limit |
| 中继带宽 | 中继流量 | 根据服务器带宽 |
| 连接数 | 活跃连接 | > 80% max_connections |
| 内存使用 | 进程内存 | > 80% max_memory |

## 相关文档

- [Bootstrap 节点部署](../bootstrap/README.md)
- [一体化部署](../README.md)
- [统一 Relay 设计](../../../../design/_discussions/20260123-nat-relay-concept-clarification.md)
