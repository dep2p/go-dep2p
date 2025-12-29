# cmd/dep2p - 命令行入口

dep2p 命令行工具，用于启动 dep2p 节点。

## 构建

```bash
go build -o dep2p ./cmd/dep2p
```

带版本信息构建:

```bash
go build -ldflags "-X main.version=v0.1.0 -X main.commit=$(git rev-parse --short HEAD) -X main.buildDate=$(date -u +%Y-%m-%d)" -o dep2p ./cmd/dep2p
```

## 使用

### 基本启动

```bash
# 使用默认配置
./dep2p

# 指定端口
./dep2p -port 4002

# 使用配置文件
./dep2p -config config.json
```

### 预设配置

```bash
# 移动端模式
./dep2p -preset mobile

# 桌面端模式（默认）
./dep2p -preset desktop

# 服务器模式
./dep2p -preset server

# 最小模式（测试）
./dep2p -preset minimal
```

### 发现配置

```bash
# 禁用 DHT
./dep2p -dht=false

# 禁用 mDNS
./dep2p -mdns=false

# 指定引导节点
./dep2p -bootstrap "/ip4/1.2.3.4/udp/4001/quic-v1/p2p/12D3KooW..."
```

### 连接配置

```bash
# 设置连接限制
./dep2p -low-water 20 -high-water 50
```

## 配置文件

配置文件使用 JSON 格式:

```json
{
  "preset": "desktop",
  "listen_addrs": [
    "/ip4/0.0.0.0/udp/4001/quic-v1"
  ],
  "identity": {
    "key_file": "~/.dep2p/identity.key"
  },
  "connection_limits": {
    "low": 50,
    "high": 100
  },
  "discovery": {
    "enable_dht": true,
    "enable_mdns": true
  }
}
```

## 命令行参数

| 参数 | 默认值 | 描述 |
|------|--------|------|
| -listen | /ip4/0.0.0.0/udp/4001/quic-v1 | 监听地址 |
| -port | 0 | 监听端口（覆盖 listen） |
| -identity | | 身份密钥文件 |
| -config | | 配置文件路径 |
| -preset | desktop | 预设配置 |
| -dht | true | 启用 DHT |
| -mdns | true | 启用 mDNS |
| -relay | true | 启用中继 |
| -nat | true | 启用 NAT 穿透 |
| -low-water | 50 | 连接低水位线 |
| -high-water | 100 | 连接高水位线 |
| -bootstrap | | 引导节点 |
| -version | | 显示版本 |
| -help | | 显示帮助 |

