# Bootstrap 引导服务实现

## 概述

引导节点服务实现，提供初始节点发现能力。

## 文件结构

```
bootstrap/
├── README.md        # 本文件
└── bootstrap.go     # 引导服务实现
```

## 核心实现

### bootstrap.go

```go
// Bootstrap 引导服务
type Bootstrap struct {
    staticPeers []types.PeerInfo  // 静态引导节点
    dnsPeers    []string          // DNS 引导地址
}

// NewBootstrap 创建引导服务
func NewBootstrap(config Config) *Bootstrap

// GetBootstrapPeers 获取引导节点
func (b *Bootstrap) GetBootstrapPeers(ctx context.Context) ([]types.PeerInfo, error)

// AddBootstrapPeer 添加引导节点
func (b *Bootstrap) AddBootstrapPeer(peer types.PeerInfo)

// RemoveBootstrapPeer 移除引导节点
func (b *Bootstrap) RemoveBootstrapPeer(id types.NodeID)
```

## 引导节点来源

### 1. 静态配置

```go
staticPeers := []types.PeerInfo{
    {
        ID:    parseNodeID("5Q2STWvB..."),
        Addrs: []string{"/ip4/104.131.131.82/udp/4001/quic-v1"},
    },
}
```

### 2. DNS 发现

```go
// DNS TXT 记录格式
// _dnsaddr.bootstrap.dep2p.io TXT "dnsaddr=/ip4/x.x.x.x/udp/4001/quic-v1/p2p/5Q2STWvB..."

func (b *Bootstrap) resolveDNS(ctx context.Context, domain string) ([]types.PeerInfo, error) {
    // 查询 TXT 记录
    // 解析 dnsaddr 格式
    // 返回节点信息
}
```

### 3. 硬编码默认节点

```go
var defaultBootstrapPeers = []types.PeerInfo{
    // dep2p 官方引导节点
    // ...
}
```

## 引导流程

```
1. 加载配置的静态引导节点
2. 解析 DNS 引导地址
3. 返回合并后的节点列表
4. 连接到引导节点
5. 执行 DHT Bootstrap
```

## 配置

```go
type Config struct {
    StaticPeers []types.PeerInfo  // 静态引导节点
    DNSAddrs    []string          // DNS 引导地址
    Timeout     time.Duration     // 连接超时
}
```

