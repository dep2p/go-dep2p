# Discovery Layer (发现层)

发现层提供节点发现服务。

## 模块列表

| 模块 | 说明 |
|------|------|
| `coordinator` | 发现协调器 |
| `dht` | Kademlia DHT |
| `mdns` | 局域网多播发现 |
| `bootstrap` | 引导节点 |
| `rendezvous` | 命名空间发现 |
| `dns` | DNS-SD 发现 |

## 架构原则

1. 多种发现机制可以并行使用
2. Coordinator 负责协调多种发现机制
3. 发现结果统一通过 EventBus 发布
