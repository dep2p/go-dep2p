# UPnP 端口映射实现

## 概述

基于 `huin/goupnp` 库的 UPnP 端口映射实现。

## 文件结构

```
upnp/
├── README.md        # 本文件
└── mapper.go        # UPnP 端口映射器
```

## 核心实现

### mapper.go

```go
// Mapper UPnP 端口映射器
type Mapper struct {
    gateway *upnp.Device
    mu      sync.Mutex
}

// NewMapper 创建 UPnP 映射器
func NewMapper() (*Mapper, error)

// Available 检查 UPnP 是否可用
func (m *Mapper) Available() bool

// GetExternalAddress 获取外部地址
func (m *Mapper) GetExternalAddress() (core.Address, error)

// AddMapping 添加端口映射
func (m *Mapper) AddMapping(protocol string, internalPort int, description string, duration time.Duration) (int, error)

// DeleteMapping 删除端口映射
func (m *Mapper) DeleteMapping(protocol string, externalPort int) error
```

## UPnP 设备发现

```go
func discoverGateway() (*upnp.Device, error) {
    // 发送 SSDP M-SEARCH
    // 查找 InternetGatewayDevice
    // 获取 WANIPConnection 服务
}
```

## 支持的 UPnP 服务

- `urn:schemas-upnp-org:service:WANIPConnection:1`
- `urn:schemas-upnp-org:service:WANIPConnection:2`
- `urn:schemas-upnp-org:service:WANPPPConnection:1`

## 依赖

- `github.com/huin/goupnp` - UPnP 库

