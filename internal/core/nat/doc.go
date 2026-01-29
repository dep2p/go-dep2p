// Package nat 实现 NAT 穿透功能
//
// # 模块概述
//
// nat 提供 NAT 穿透能力，包括：
//   - AutoNAT: NAT 类型检测和可达性判断
//   - STUN: 外部地址获取
//   - UPnP/NAT-PMP: 自动端口映射 (v1.1+)
//   - Hole Punching: UDP 打洞 (v1.1+)
//
// # 快速开始
//
// 创建和启动 NAT 服务：
//
//	config := nat.DefaultConfig()
//	service, err := nat.NewService(config, swarm, eventbus)
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	ctx := context.Background()
//	if err := service.Start(ctx); err != nil {
//	    log.Fatal(err)
//	}
//	defer service.Stop()
//
//	// 查询可达性状态
//	reachability := service.Reachability()
//	fmt.Println("Reachability:", reachability)
//
// # NAT 类型和可达性
//
// 可达性状态：
//   - ReachabilityUnknown: 初始状态，尚未探测
//   - ReachabilityPublic: 公网可达，无需 NAT 穿透
//   - ReachabilityPrivate: NAT 后，需要穿透技术
//
// NAT 类型（参考）：
//   - Full Cone NAT: 最容易穿透
//   - Restricted Cone NAT: 需要先发包
//   - Port Restricted Cone NAT: 端口限制
//   - Symmetric NAT: 最难穿透，通常需要中继
//
// # AutoNAT 检测流程
//
// 1. 定期探测（默认 15 秒间隔）
// 2. 请求远程节点拨回我们的地址
// 3. 根据成功/失败更新置信度
// 4. 达到阈值（默认 3 次）后确定状态
//
// 状态转换：
//
//	Unknown → (3次成功) → Public
//	Unknown → (3次失败) → Private
//	Public  → (连续失败) → Private
//	Private → (连续成功) → Public
//
// # 配置选项
//
// 核心配置：
//
//	config := &nat.Config{
//	    EnableAutoNAT:       true,               // 启用 AutoNAT 检测
//	    EnableUPnP:          true,               // 启用 UPnP 映射
//	    EnableNATPMP:        true,               // 启用 NAT-PMP 映射
//	    EnableHolePunch:     true,               // 启用打洞
//	    STUNServers:         []string{...},      // STUN 服务器列表
//	    ProbeInterval:       15 * time.Second,   // 探测间隔
//	    ProbeTimeout:        10 * time.Second,   // 探测超时
//	    ConfidenceThreshold: 3,                  // 置信度阈值
//	}
//
// 使用函数式选项：
//
//	config := nat.DefaultConfig()
//	config.ApplyOptions(
//	    nat.WithAutoNAT(true),
//	    nat.WithProbeInterval(30 * time.Second),
//	    nat.WithSTUNServers([]string{"stun.example.com:3478"}),
//	)
//
// # 性能特性
//
// 资源消耗：
//   - Goroutines: 1-3 个（AutoNAT 探测、STUN 刷新、端口映射续期）
//   - 内存: < 1MB
//   - 网络: 探测流量 < 1KB/探测，间隔可配置
//
// 并发安全：
//   - 所有公共方法都是线程安全的
//   - 使用 sync.RWMutex 保护共享状态
//   - 使用 atomic.Value 存储可达性状态
//
// # v1.0 实现范围
//
// ✅ 已完整实现：
//   - AutoNAT 客户端（检测可达性）
//   - STUN 客户端（pion/stun v0.6.1，真实实现）
//   - UPnP 端口映射（huin/goupnp v1.3.0，真实实现）
//   - NAT-PMP 端口映射（jackpal/go-nat-pmp v1.0.2，真实实现）
//   - Service 生命周期管理
//   - 配置和错误处理
//   - 端口映射自动续期
//   - 完整测试框架（单元测试 + 集成测试）
//
// ⬜ 技术债（需后续组件）：
//   - TD-001: Hole Punching 完整实现（需 relay 模块）
//   - TD-002: AutoNAT 服务端（v1.1 规划）
//   - TD-003: 复杂 NAT 穿透策略（依赖 TD-001）
//
// 详见：design/03_architecture/L6_domains/core_nat/TECHNICAL_DEBT.md
//
// # 已知限制
//
// v1.0 限制：
//   - AutoNAT 依赖网络中有提供服务的节点
//   - STUN 依赖公共服务器可用性
//   - Hole Punching 需要 relay 模块支持（v1.1）
//   - 不支持对称 NAT 的高级穿透（v1.1）
//
// 安全考虑：
//   - 限流：防止被滥用探测（v1.1 服务端实现）
//   - 地址验证：拒绝拨号私有地址
//   - 超时控制：防止资源耗尽
//
// # 依赖关系
//
// 内部模块依赖：
//   - internal/core/swarm: 获取连接和地址信息（可选注入）
//   - internal/core/eventbus: 发布可达性变化事件（可选注入）
//
// 外部库（已集成）：
//   - github.com/pion/stun v0.6.1: STUN 协议实现
//   - github.com/huin/goupnp v1.3.0: UPnP 端口映射
//   - github.com/jackpal/go-nat-pmp v1.0.2: NAT-PMP 协议
//   - github.com/jackpal/gateway v1.0.15: 网关发现
//
// # 错误处理
//
// 常见错误：
//   - ErrServiceClosed: 服务已关闭
//   - ErrAlreadyStarted: 服务已启动
//   - ErrInvalidConfig: 配置无效
//   - ErrNoPeers: 没有可用探测节点
//   - ErrHolePunchFailed: 打洞失败
//   - ErrNoExternalAddr: 无法获取外部地址
//
// 错误类型：
//   - DialError: 拨号错误（聚合多个错误）
//   - MappingError: 端口映射错误
//   - ProbeError: 探测错误
//
// # 架构设计
//
// 模块结构：
//
//	Service (NAT 服务主入口)
//	├── AutoNAT (NAT 检测器)
//	│   └── runProbeLoop() - 探测循环
//	├── STUNClient (STUN 客户端, v1.1)
//	│   └── GetExternalAddr() - 获取外部地址
//	├── UPnPMapper (UPnP 映射器, v1.1)
//	│   └── MapPort() - 端口映射
//	├── NATPMPMapper (NAT-PMP 映射器, v1.1)
//	│   └── MapPort() - 端口映射
//	└── HolePuncher (打洞器, v1.1)
//	    └── DirectConnect() - 直连尝试
//
// 生命周期：
//  1. NewService() - 创建服务和子组件
//  2. Start() - 启动探测循环和后台任务
//  3. (运行中) - 定期探测、更新状态、发布事件
//  4. Stop() - 停止所有 goroutine、清理资源
//
// # 示例
//
// 监听可达性变化：
//
//	// 定期检查状态
//	ticker := time.NewTicker(30 * time.Second)
//	defer ticker.Stop()
//
//	for range ticker.C {
//	    status := service.Reachability()
//	    switch status {
//	    case nat.ReachabilityPublic:
//	        log.Info("Public reachable")
//	    case nat.ReachabilityPrivate:
//	        log.Info("Behind NAT")
//	    case nat.ReachabilityUnknown:
//	        log.Info("Status unknown")
//	    }
//	}
//
// 获取外部地址：
//
//	addrs := service.ExternalAddrs()
//	for _, addr := range addrs {
//	    fmt.Println("External address:", addr)
//	}
//
// # 测试
//
// 单元测试：
//
//	go test -v -cover ./internal/core/nat
//	go test -v -cover ./internal/core/nat/stun
//	go test -v -cover ./internal/core/nat/holepunch
//
// 竞态检测：
//
//	go test -race ./internal/core/nat/...
//
// 基准测试：
//
//	go test -bench=. ./internal/core/nat
//
// # 协议标准
//
//   - RFC 5389: STUN (Session Traversal Utilities for NAT)
//   - RFC 5626: Managing Client-Initiated Connections in SIP
//   - RFC 6886: NAT-PMP (NAT Port Mapping Protocol)
//
// 架构层：Core Layer
package nat

import (
	"github.com/dep2p/go-dep2p/pkg/lib/log"
)

var logger = log.Logger("core/nat")
