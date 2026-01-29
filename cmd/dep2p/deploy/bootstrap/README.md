# DeP2P Bootstrap 节点部署指南

本指南介绍如何部署一个**专职 Bootstrap 节点**，仅提供节点发现服务，不提供中继功能。

## 节点职责

| 功能 | 状态 | 说明 |
|------|------|------|
| **Bootstrap 服务** | ✅ 启用 | 提供 DHT 节点发现服务 |
| **Relay** | ❌ 禁用 | 不提供中继转发 |
| **Realm Gateway** | ❌ 禁用 | 不提供跨 Realm 网关 |

## 目录结构

```
bootstrap/
├── README.md                  # 本文档
├── bootstrap.config.json      # Bootstrap 节点配置模板
├── dep2p-bootstrap.service    # systemd 服务文件模板
└── data/                      # 运行时数据目录（自动创建）
    └── identity.key           # 节点身份密钥（自动生成）
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
cd cmd/dep2p/deploy/bootstrap

# 创建数据目录
mkdir -p data

# 复制配置模板
cp bootstrap.config.json config.json
```

### 3. 启动节点

```bash
# 基本启动（端口 4001）
../../dep2p --config config.json --port 4001 \
  --public-addr /ip4/YOUR_PUBLIC_IP/udp/4001/quic-v1 \
  --enable-bootstrap

# 验证启动成功后，会显示：
# Capabilities: Bootstrap
```

### 4. systemd 服务部署

```bash
# 复制服务文件
sudo cp dep2p-bootstrap.service /etc/systemd/system/

# 编辑修改 IP 和路径
sudo vim /etc/systemd/system/dep2p-bootstrap.service

# 启动服务
sudo systemctl daemon-reload
sudo systemctl enable dep2p-bootstrap
sudo systemctl start dep2p-bootstrap

# 查看状态
sudo systemctl status dep2p-bootstrap
```

## 配置说明

### 关键配置项

```json
{
  "discovery": {
    "enable_bootstrap": true,
    "bootstrap": {
      "enable_service": true    // 启用 Bootstrap 服务能力
    }
  },
  "relay": {
    "enable_server": false      // 不提供中继服务
  }
}
```

### 资源配置建议

| 场景 | max_connections | max_streams | max_memory |
|------|-----------------|-------------|------------|
| 小型网络（<100 节点） | 500 | 5000 | 256MB |
| 中型网络（100-1000） | 1000 | 10000 | 512MB |
| 大型网络（>1000） | 2000 | 20000 | 1GB |

## 与 Relay 节点配合

Bootstrap 节点通常需要与 Relay 节点配合使用：

```
┌─────────────────┐    ┌─────────────────┐
│  Bootstrap Node │    │   Relay Node    │
│   (端口 4001)   │    │   (端口 4002)   │
│  节点发现服务    │    │  中继转发服务    │
└─────────────────┘    └─────────────────┘
         │                     │
         └─────────┬───────────┘
                   │
           ┌───────┴───────┐
           │  客户端节点    │
           │ 1. 连接 Bootstrap 发现节点
           │ 2. 通过 Relay 穿透 NAT
           └───────────────┘
```

客户端配置示例：

```json
{
  "discovery": {
    "bootstrap": {
      "peers": [
        "/ip4/BOOTSTRAP_IP/udp/4001/quic-v1/p2p/BOOTSTRAP_PEER_ID"
      ]
    }
  },
  "relay": {
    "relay_addr": "/ip4/RELAY_IP/udp/4002/quic-v1/p2p/RELAY_PEER_ID"
  }
}
```

## 相关文档

- [Relay 节点部署](../relay/README.md)
- [一体化部署](../README.md)
- [Bootstrap 节点设计](../../../../design/_discussions/20260116-bootstrap-node-detailed.md)
