# Relay Server 中继服务器实现

## 概述

中继服务器实现，为 NAT 后的节点提供连接中转服务。

## 文件结构

```
server/
├── README.md        # 本文件
└── server.go        # 服务器实现
```

## 核心实现

### server.go

```go
// Server 中继服务器
type Server struct {
    transport    transportif.Transport
    identity     identityif.Identity
    reservations map[types.NodeID]*reservation
    circuits     map[circuitKey]*circuit
    config       ServerConfig
    stats        types.RelayStats
    mu           sync.RWMutex
}

// NewServer 创建中继服务器
func NewServer(transport transportif.Transport, identity identityif.Identity, config ServerConfig) *Server

// Start 启动服务器
func (s *Server) Start(ctx context.Context) error

// Stop 停止服务器
func (s *Server) Stop() error

// Stats 返回统计信息
func (s *Server) Stats() types.RelayStats
```

## 资源管理

### 预留管理

```go
type reservation struct {
    peerID      types.NodeID
    addrs       []string
    createdAt   time.Time
    expiry      time.Time
    connections int
}

func (s *Server) handleReserve(peer types.NodeID) (*reservation, error) {
    // 检查是否达到限制
    // 分配中继地址
    // 记录预留
    // 设置过期清理
}
```

### 电路管理

```go
type circuit struct {
    src    types.NodeID
    dst    types.NodeID
    srcConn transportif.Conn
    dstConn transportif.Conn
    created time.Time
}

func (s *Server) handleConnect(src types.NodeID, dst types.NodeID, srcConn transportif.Conn) error {
    // 查找目标预留
    // 通知目标节点
    // 建立电路
    // 开始数据转发
}
```

### 数据转发

```go
func (c *circuit) forward() {
    go io.Copy(c.dstConn, c.srcConn)
    go io.Copy(c.srcConn, c.dstConn)
}
```

## 限制策略

```go
type ServerConfig struct {
    // 连接限制
    MaxReservations    int  // 最大预留数，默认 128
    MaxCircuits        int  // 最大电路数，默认 16
    MaxCircuitsPerPeer int  // 每节点最大电路数，默认 4
    
    // 时间限制
    ReservationTTL time.Duration  // 预留有效期，默认 1 小时
    MaxDuration    time.Duration  // 最大连接时长，默认 2 分钟
    
    // 流量限制
    MaxDataRate int64  // 最大速率，默认 1 MB/s
}
```

## 协议

### RESERVE 请求

```
+-------+----------+
| Type  | Reserved |
| 1byte | 3bytes   |
+-------+----------+
```

### RESERVE 响应

```
+-------+--------+-----------+-------+
| Type  | Status | AddrCount | Addrs |
| 1byte | 1byte  | 2bytes    | var   |
+-------+--------+-----------+-------+
```

### CONNECT 请求

```
+-------+----------+----------+
| Type  | DestLen  | DestID   |
| 1byte | 1byte    | 32bytes  |
+-------+----------+----------+
```

