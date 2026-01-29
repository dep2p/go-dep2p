# Discovery 层集成测试

本目录包含 Discovery 层的集成测试，验证节点发现机制的真实功能。

## 测试文件

| 文件 | 测试内容 | 备注 |
|------|---------|------|
| `mdns_test.go` | mDNS 本地发现 | 需要支持多播的网络环境 |
| `dht_test.go` | DHT 分布式发现 | 使用引导节点模拟 DHT 网络 |

## 运行测试

```bash
# 运行所有 Discovery 集成测试
go test -tags=integration ./tests/integration/discovery/... -v -timeout 5m

# 运行 mDNS 测试
go test -tags=integration ./tests/integration/discovery/mdns_test.go -v

# 运行 DHT 测试
go test -tags=integration ./tests/integration/discovery/dht_test.go -v
```

## 测试说明

### mDNS 测试

mDNS (Multicast DNS) 用于本地网络发现。测试可能因以下原因被跳过：

- 网络不支持多播
- 防火墙阻止 mDNS 流量
- 虚拟机/容器网络配置

如果 mDNS 测试被跳过，请检查网络环境。

### DHT 测试

DHT (Distributed Hash Table) 用于广域网发现。测试使用本地模拟的 DHT 网络：

- 一个引导节点作为 DHT 种子
- 其他节点连接到引导节点后，通过 DHT 协议发现彼此

DHT 路由收敛需要时间，测试超时设置较长。

## 预设要求

| 预设 | mDNS | DHT | 适用测试 |
|------|:----:|:---:|---------|
| minimal | ❌ | ❌ | 基础连接测试 |
| desktop | ✅ | ✅ | mDNS + DHT 测试 |
| server | ❌ | ✅ | DHT 引导节点 |

## 注意事项

1. mDNS 测试在某些 CI 环境可能被跳过
2. DHT 测试需要足够的超时时间 (建议 2 分钟以上)
3. 测试使用真实网络连接，不使用 Mock
