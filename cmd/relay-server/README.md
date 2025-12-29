# DeP2P Relay Server

独立的 Relay 服务器，用于帮助 NAT 后的节点建立连接。

## 功能

- 为无法直接连接的节点提供中继服务
- 支持预留槽位机制
- 流量限制和连接管理
- 支持 Docker 部署

## 使用方法

### 本地运行

```bash
cd cmd/relay-server
go run main.go -port 4001
```

### Docker 运行

```bash
# 构建镜像
docker build -t dep2p-relay -f cmd/relay-server/Dockerfile .

# 运行容器
docker run -d -p 4001:4001 --name dep2p-relay dep2p-relay
```

### Docker Compose

```yaml
version: '3'
services:
  relay:
    build:
      context: .
      dockerfile: cmd/relay-server/Dockerfile
    ports:
      - "4001:4001"
    restart: unless-stopped
```

## 命令行参数

| 参数 | 说明 | 默认值 |
|------|------|--------|
| `-port` | 监听端口 | 4001 |
| `-max-conns` | 最大连接数 | 1000 |
| `-max-reservations` | 最大预留数 | 128 |

## 工作原理

```
┌──────────────────────────────────────────────────────────────┐
│                         互联网                               │
│                                                              │
│    ┌─────────┐                           ┌─────────┐        │
│    │ Client A │                           │ Client B │        │
│    │ (NAT后)  │                           │ (NAT后)  │        │
│    └────┬────┘                           └────┬────┘        │
│         │                                      │             │
│         │    ┌────────────────────┐           │             │
│         └────►    Relay Server    ◄───────────┘             │
│              │   (公网 IP)        │                          │
│              └────────────────────┘                          │
│                                                              │
└──────────────────────────────────────────────────────────────┘
```

1. **预留**: Client A 向 Relay 预留一个槽位
2. **连接**: Client B 通过 Relay 地址连接到 Client A
3. **中继**: Relay 在两个客户端之间转发数据
4. **升级**: 如果可能，客户端会尝试建立直连（打洞）

## 部署建议

### 生产环境

1. 使用公网 IP 或负载均衡器
2. 配置 TLS（如果需要）
3. 设置适当的连接限制
4. 启用监控和日志

### 安全考虑

- Relay 服务器会看到所有中继流量（但 dep2p 流量是端到端加密的）
- 建议限制每个节点的预留数量
- 考虑实施速率限制

## 监控

服务器每 30 秒输出一次统计信息：

```
[Stats] 当前连接数: 42
```

## 参考

- [go-libp2p Circuit Relay](https://github.com/libp2p/go-libp2p)
- [libp2p Relay Spec](https://github.com/libp2p/specs/tree/master/relay)

