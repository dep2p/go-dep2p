// Package relay 实现统一中继服务
//
// v2.0 统一 Relay 架构：
// - 单一 Relay 服务，废弃双层中继（System/Realm）
// - 三大职责：缓存加速 + 打洞协调 + 数据保底
//
// # 架构组件
//
// relay 包包含以下核心组件：
//
//	┌────────────────────────────────────────────────────────────┐
//	│                        Manager                              │
//	│         (统一入口，协调所有 Relay 功能)                       │
//	├─────────────────────┬──────────────────────────────────────┤
//	│                     │                                      │
//	│   RelayService      │              AutoRelay               │
//	│  (单连接管理)        │           (多 Relay 策略)             │
//	│                     │                                      │
//	│  • 状态机           │  • 候选发现                           │
//	│  • 连接/断开        │  • 故障切换                           │
//	│  • 单个 Relay 会话   │  • 预留管理                           │
//	│                     │  • 首选中继                           │
//	└─────────────────────┴──────────────────────────────────────┘
//
// # 组件职责
//
// ## Manager (manager.go)
//
// 统一入口，协调所有 Relay 相关功能：
//   - DialWithPriority(): 按优先级连接（直连 > 打洞 > 中继）
//   - EnableRelay()/DisableRelay(): 启用/禁用 Relay 服务能力
//   - SetRelayAddr(): 配置中继地址
//
// ## RelayService (service.go)
//
// 底层单连接管理，负责与单个 Relay 节点的交互：
//   - SetRelay(): 配置中继地址
//   - Connect(): 建立中继连接
//   - State(): 连接状态机（None -> Configured -> Connecting -> Connected）
//   - DialViaRelay(): 通过中继拨号
//
// ## AutoRelay (client/autorelay.go)
//
// 上层多 Relay 策略，实现自动化中继管理：
//   - 候选发现：从 DHT/配置中发现可用 Relay
//   - 故障切换：当前 Relay 不可用时自动切换
//   - 预留管理：管理多个 Relay 预留（MinRelays=2, MaxRelays=4）
//   - 首选中继：维护 preferredRelays 列表
//
// # 协作关系
//
//	用户调用 Manager → Manager 使用 AutoRelay 管理多个候选
//	                 → AutoRelay 内部使用 client.Client 管理单个连接
//	                 → 当需要时，通过 RelayService 进行数据转发
//
// # 设计原则
//
// 统一中继遵循高内聚低耦合和依赖倒置原则：
//   - Relay 只负责消息转发，不依赖 Realm 实现
//   - 通过接口抽象依赖，支持灵活配置
//   - 资源限制通过统一的 RelayLimiter 实现
//
// # 惰性连接设计
//
// 配置 ≠ 连接：
//
//	node.SetRelayAddr(addr) // 仅保存配置
//
// 按需连接：
//
//	当需要通过中继连接时，自动建立中继连接
//	连接优先级：直连 > 打洞 > 中继
//
// # 使用示例
//
//	// 配置 Relay（客户端）
//	node.SetRelayAddr(relayAddr)
//
//	// 成为 Relay 服务器
//	node.EnableRelay(ctx)
//
// # 依赖
//
// 内部模块依赖：
//   - internal/core/swarm: 连接管理
//   - internal/core/nat: NAT 穿透
//
// # 架构层
//
// Core Layer
//
// # 设计文档
//
//   - design/_discussions/20260123-nat-relay-concept-clarification.md
//   - internal/core/relay/ARCHITECTURE.md (详细架构说明)
package relay

import (
	"github.com/dep2p/go-dep2p/pkg/lib/log"
)

var logger = log.Logger("core/relay")
