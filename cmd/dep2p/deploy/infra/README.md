# DeP2P 一体化基础设施节点部署指南

本指南介绍如何部署一个同时具备 **Bootstrap + Relay + Gateway** 能力的一体化基础设施节点。

## 节点职责

| 功能 | 状态 | 说明 |
|------|------|------|
| **Bootstrap 服务** | ✅ 启用 | 提供 DHT 节点发现服务 |
| **Relay** | ✅ 启用 | 为 NAT 后节点提供中继转发 |
| **Realm Gateway** | ✅ 启用 | 跨 Realm 网关能力 |

## 适用场景

- 开发/测试环境
- 小型生产环境（单节点足够）
- 资源受限场景（只能部署一台服务器）

## 目录结构

```
infra/
├── README.md              # 本文档
├── infra.config.json      # 一体化节点配置模板
├── dep2p-infra.service    # systemd 服务文件模板
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
cd cmd/dep2p/deploy/infra

# 创建数据目录
mkdir -p data

# 复制配置模板
cp infra.config.json config.json
```

### 3. 启动节点

```bash
# 基本启动（端口 4001）
../../dep2p --config config.json --port 4001 \
  --public-addr /ip4/YOUR_PUBLIC_IP/udp/4001/quic-v1 \
  --enable-infra

# 验证启动成功后，会显示：
# Capabilities: Bootstrap, Relay
```

### 4. systemd 服务部署

```bash
# 复制服务文件
sudo cp dep2p-infra.service /etc/systemd/system/

# 编辑修改 IP 和路径
sudo vim /etc/systemd/system/dep2p-infra.service

# 启动服务
sudo systemctl daemon-reload
sudo systemctl enable dep2p-infra
sudo systemctl start dep2p-infra

# 查看状态
sudo systemctl status dep2p-infra
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
    "enable_server": true       // 启用中继服务
  },
  "realm": {
    "enable_gateway": true      // 启用 Realm 网关
  }
}
```

### 资源配置建议

一体化节点需要综合考虑 Bootstrap 和 Relay 的资源需求：

| 场景 | max_connections | max_streams | max_memory |
|------|-----------------|-------------|------------|
| 小型网络 | 500 | 5000 | 512MB |
| 中型网络 | 1000 | 10000 | 1GB |
| 大型网络 | 2000 | 20000 | 2GB |

## 与拆分部署的对比

| 对比项 | 一体化部署 | 拆分部署 |
|--------|-----------|----------|
| 运维复杂度 | 低（单进程） | 高（多进程） |
| 资源隔离 | 无 | 有 |
| 故障影响 | 全部服务不可用 | 仅单一服务受影响 |
| 独立扩展 | 不支持 | 支持 |
| 适用规模 | 小型/测试 | 中大型生产 |

## 迁移到拆分部署

当网络规模增长时，可迁移到拆分部署：

1. 部署独立的 [Bootstrap 节点](../bootstrap/README.md)
2. 部署独立的 [Relay 节点](../relay/README.md)
3. 更新客户端配置，指向新的 Bootstrap 和 Relay 地址
4. 下线一体化节点

## 相关文档

- [Bootstrap 节点部署](../bootstrap/README.md)
- [Relay 节点部署](../relay/README.md)
- [部署模式总览](../README.md)
