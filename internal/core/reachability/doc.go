// Package reachability 提供可达性协调模块的实现
//
// # IMPL-NETWORK-RESILIENCE Phase 6.4: Reachability Coordinator
//
// 可达性协调器统一管理地址发布，实现"可达性优先"策略：
// - 先保证连得上（Relay 兜底）
// - 再争取直连更优路径
//
// # 核心组件
//
// Coordinator - 可达性协调器:
//   - 聚合来自不同来源的地址
//   - 按优先级排序
//   - 在地址变更时通知订阅者
//   - 通过 dial-back 验证地址的真实可达性
//
// DialBackService - 回拨验证服务:
//   - 作为验证方：请求协助节点回拨验证候选地址
//   - 作为协助方：响应其他节点的回拨请求
//
// WitnessService - 入站见证服务:
//   - 发送方：出站连接成功后发送 WitnessReport
//   - 接收方：验证并记录见证，达到阈值后升级 VerifiedDirect
//
// DirectAddrStore - 直连地址存储:
//   - 跨进程重启持久化
//   - 原子写入（临时文件 + rename）
//   - debounce flush（减少磁盘 I/O）
//
// # 地址优先级
//
//   PriorityVerifiedDirect > PriorityRelayGuarantee > PriorityLocalListen > PriorityUnverified
//
// # Dial-Back 验证流程
//
//	Node A (验证方)                    Node B (协助方)
//	     │  1. 建立连接（已有）             │
//	     │────────────────────────────────>│
//	     │                                 │
//	     │  2. 发送 DialBackRequest        │
//	     │    { addrs: [候选地址列表] }     │
//	     │────────────────────────────────>│
//	     │                                 │
//	     │         3. B 尝试回拨 A 的候选地址
//	     │<────────────────────────────────│
//	     │                                 │
//	     │  4. 返回 DialBackResponse       │
//	     │    { reachable: [可达地址列表] } │
//	     │<────────────────────────────────│
//
// # Witness-Threshold 验证
//
// 无外部依赖的升级路径：
// 1. Peer A 使用候选地址成功连入 Peer B
// 2. Peer A 自动发送 WitnessReport 给 Peer B
// 3. Peer B 记录见证（按 IP 前缀去重）
// 4. 当同一地址被 >= MinWitnesses 个不同 IP 前缀的 peer 见证后
// 5. 该地址自动升级为 VerifiedDirect
package reachability
