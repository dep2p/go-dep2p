# DeP2P 基础设施节点部署指南

本目录提供三种部署模式，根据实际需求选择。

## 部署模式

```
deploy/
├── bootstrap/     # 仅 Bootstrap（节点发现服务）
├── relay/         # 仅 Relay（中继转发服务）
└── infra/         # Bootstrap + Relay 一体化
```

| 模式 | Bootstrap | Relay | Gateway | 适用场景 |
|------|-----------|-------|---------|----------|
| [bootstrap/](./bootstrap/) | ✅ | ❌ | ❌ | 大型网络，独立扩展发现服务 |
| [relay/](./relay/) | ❌ | ✅ | ✅ | 大型网络，独立扩展中继服务 |
| [infra/](./infra/) | ✅ | ✅ | ✅ | 开发测试、小型生产环境 |

## 模式选择建议

### 开发/测试环境
→ 使用 **infra/** 一体化部署，简单省事

### 小型生产环境（<100 节点）
→ 使用 **infra/** 一体化部署 + 资源调优

### 中大型生产环境
→ 拆分部署 **bootstrap/** + **relay/**
- 资源隔离，故障影响范围小
- 可独立扩展（如多个 Relay 节点分担流量）

## 典型部署架构

### 单节点（一体化）

```
┌─────────────────────────────┐
│     Infra Node (4001)       │
│  Bootstrap + Relay + Gateway│
└─────────────────────────────┘
              │
    ┌─────────┼─────────┐
    │         │         │
 Client A  Client B  Client C
```

### 拆分部署

```
┌─────────────────┐    ┌─────────────────┐
│  Bootstrap Node │    │   Relay Node    │
│   (端口 4001)   │    │   (端口 4002)   │
└─────────────────┘    └─────────────────┘
         │                     │
         └──────────┬──────────┘
                    │
    ┌───────────────┼───────────────┐
    │               │               │
 Client A        Client B        Client C
```

### 多 Relay 扩展

```
┌─────────────────┐
│  Bootstrap Node │
│   (端口 4001)   │
└─────────────────┘
         │
    ┌────┴────┬────────────┐
    │         │            │
┌───┴───┐ ┌───┴───┐  ┌─────┴─────┐
│Relay 1│ │Relay 2│  │ Relay N   │
│ 4002  │ │ 4003  │  │   ...     │
└───────┘ └───────┘  └───────────┘
```

## 快速开始

### 一体化部署

```bash
cd infra
cp infra.config.json config.json
mkdir -p data
../../dep2p --config config.json --port 4001 \
  --public-addr /ip4/YOUR_IP/udp/4001/quic-v1 --enable-infra
```

### 拆分部署

> **重要**：同机运行多个节点时，必须使用不同的数据目录，否则会出现数据库锁冲突。
> 配置模板已预设独立数据目录：`./data/bootstrap`、`./data/relay`、`./data/infra`。

```bash
# 终端 1：Bootstrap
cd bootstrap
# 配置已包含 "data_dir": "./data/bootstrap"
../../dep2p --config bootstrap.config.json --port 4001 \
  --public-addr /ip4/YOUR_IP/udp/4001/quic-v1 --enable-bootstrap

# 终端 2：Relay（修改 config.json 中的 bootstrap peers）
cd relay
# 配置已包含 "data_dir": "./data/relay"
# 编辑 relay.config.json，填入 Bootstrap 节点地址
../../dep2p --config relay.config.json --port 4002 \
  --public-addr /ip4/YOUR_IP/udp/4002/quic-v1 --enable-system-relay
```

或使用命令行参数指定数据目录（不依赖配置文件）：

```bash
# 方式 1：使用 --data-dir 参数
dep2p --data-dir ./data/node1 --port 4001 --enable-bootstrap ...
dep2p --data-dir ./data/node2 --port 4002 --enable-system-relay ...

# 方式 2：使用环境变量
DEP2P_DATA_DIR=./data/node1 dep2p --port 4001 --enable-bootstrap ...
DEP2P_DATA_DIR=./data/node2 dep2p --port 4002 --enable-system-relay ...
```

## 客户端配置

客户端需要配置 Bootstrap 和 Relay 地址：

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
    "system_relay_addr": "/ip4/RELAY_IP/udp/4002/quic-v1/p2p/RELAY_PEER_ID"
  }
}
```

> **提示**：一体化部署时，Bootstrap 和 Relay 是同一地址。

## 防火墙/安全组

确保开放相应端口：

| 端口 | 协议 | 用途 |
|------|------|------|
| 4001 | UDP/TCP | Bootstrap / Infra |
| 4002 | UDP/TCP | Relay（拆分部署时） |

```bash
# Ubuntu/Debian
sudo ufw allow 4001/udp
sudo ufw allow 4001/tcp
sudo ufw allow 4002/udp
sudo ufw allow 4002/tcp
```

## 相关文档

- [cmd/dep2p 命令行说明](../README.md)
- [Bootstrap 节点设计](../../../design/_discussions/20260116-bootstrap-node-detailed.md)
- [Relay 节点设计](../../../design/_discussions/20260116-relay-node-detailed.md)
