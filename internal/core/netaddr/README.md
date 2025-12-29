# netaddr 模块

网络地址类型核心实现。

## 职责

- 对应 `pkg/interfaces/netaddr` 接口包
- 提供 `Address` 接口的基础类型定义
- 位于依赖层次底层，不依赖其他 core 模块

## 设计说明

`netaddr` 是一个轻量级模块，主要目的是：

1. **避免循环依赖**：`Address` 接口需要被多个包引用（endpoint、relay、transport 等），将其放在独立的底层包中避免循环依赖。

2. **类型统一**：所有网络地址相关的类型定义集中在此，便于维护。

## 接口定义

```go
type Address interface {
    Network() string      // 网络类型: "ip4", "ip6", "dns", "relay"
    String() string       // 地址字符串表示
    Bytes() []byte        // 字节表示
    Equal(other Address) bool
    IsPublic() bool
    IsPrivate() bool
    IsLoopback() bool
}
```

## 实现分布

`Address` 接口的具体实现分布在以下模块：

- `internal/core/address` - 通用地址实现
- `internal/core/endpoint` - Endpoint 地址封装
- `internal/core/relay` - Relay 电路地址

## 模块依赖

```
netaddr (本模块)
    ↑
address, endpoint, relay, transport ...
```

