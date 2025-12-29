# mDNS 本地发现实现

## 概述

基于 `grandcat/zeroconf` 库的 mDNS/DNS-SD 本地网络发现实现。

## 文件结构

```
mdns/
├── README.md        # 本文件
└── mdns.go          # mDNS 服务实现
```

## 核心实现

### mdns.go

```go
// MDNS mDNS 发现服务
type MDNS struct {
    nodeID    types.NodeID
    server    *zeroconf.Server
    resolver  *zeroconf.Resolver
    peers     map[types.NodeID]types.PeerInfo
    mu        sync.RWMutex
}

// NewMDNS 创建 mDNS 服务
func NewMDNS(nodeID types.NodeID, port int, serviceTag string) (*MDNS, error)

// Start 启动服务
func (m *MDNS) Start(ctx context.Context) error

// Stop 停止服务
func (m *MDNS) Stop() error

// Peers 返回发现的节点
func (m *MDNS) Peers() []types.PeerInfo
```

## mDNS 服务注册

```go
// 服务类型
const ServiceType = "_dep2p._udp"

// TXT 记录
txtRecords := []string{
    "node=" + nodeID.String(),
    "version=1.0.0",
}

// 注册服务
server, err := zeroconf.Register(
    nodeID.String()[:12],  // 实例名
    ServiceType,           // 服务类型
    "local.",              // 域
    port,                  // 端口
    txtRecords,            // TXT 记录
    nil,                   // 网络接口（nil = 所有）
)
```

## 服务发现

```go
func (m *MDNS) discover(ctx context.Context) {
    entries := make(chan *zeroconf.ServiceEntry)
    
    go func() {
        for entry := range entries {
            // 解析 TXT 记录获取 NodeID
            // 提取地址和端口
            // 添加到已发现节点列表
        }
    }()
    
    m.resolver.Browse(ctx, ServiceType, "local.", entries)
}
```

## 适用场景

- 局域网节点发现
- 开发测试环境
- 无需 Bootstrap 节点的本地网络

## 依赖

- `github.com/grandcat/zeroconf` - mDNS/DNS-SD 库

