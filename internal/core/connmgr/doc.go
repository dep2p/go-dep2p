// Package connmgr 实现连接管理器
//
// # 核心功能
//
// 1. 水位控制 - 自动回收多余连接
//   - LowWater: 目标连接数（如 100）
//   - HighWater: 触发回收阈值（如 400）
//   - 当连接数超过 HighWater 时，回收至 LowWater
//
// 2. 连接保护 - 保护关键连接不被回收
//   - 使用标签（tag）标记重要连接
//   - 受保护的连接不会被自动回收
//   - 支持多个保护标签
//
// 3. 优先级管理 - 基于标签的优先级
//   - 每个标签有权重值
//   - 总分 = 所有标签权重之和
//   - 回收时优先关闭低分连接
//
// 4. 连接门控 - 拦截和过滤连接
//   - InterceptPeerDial: 拨号前拦截
//   - InterceptAccept: 接受前拦截
//   - InterceptSecured: 握手后拦截
//   - InterceptUpgraded: 升级后拦截
//
// # 快速开始
//
// 创建连接管理器：
//
//	cfg := connmgr.Config{
//	    LowWater:  100,
//	    HighWater: 400,
//	    GracePeriod: 20 * time.Second,
//	}
//	mgr, err := connmgr.New(cfg)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer mgr.Close()
//
// 使用标签管理优先级：
//
//	// 添加标签
//	mgr.TagPeer("peer-1", "bootstrap", 50)
//	mgr.TagPeer("peer-1", "relay", 50)
//
//	// 移除标签
//	mgr.UntagPeer("peer-1", "relay")
//
//	// 更新标签
//	mgr.UpsertTag("peer-1", "score", func(old int) int {
//	    return old + 10
//	})
//
// 保护重要连接：
//
//	// 保护连接
//	mgr.Protect("peer-1", "important")
//
//	// 取消保护
//	hasMore := mgr.Unprotect("peer-1", "important")
//
//	// 检查保护状态
//	if mgr.IsProtected("peer-1", "important") {
//	    // 连接受保护
//	}
//
// 手动触发回收：
//
//	ctx := context.Background()
//	mgr.TrimOpenConns(ctx)
//
// # 连接门控
//
// 创建门控器：
//
//	gater := connmgr.NewGater()
//
// 阻止节点：
//
//	gater.BlockPeer("bad-peer")
//
//	// 拨号会被拦截
//	if !gater.InterceptPeerDial("bad-peer") {
//	    // 拨号被拒绝
//	}
//
// 解除阻止：
//
//	gater.UnblockPeer("bad-peer")
//
// # 架构
//
// connmgr 依赖：
//   - internal/core/peerstore: 获取节点信息
//   - internal/core/eventbus: 发布连接事件
//
// connmgr 被依赖：
//   - internal/core/swarm: 使用 connmgr 管理连接
//   - internal/core/host: 集成 connmgr
//
// # 优先级计算
//
// 节点优先级 = 所有标签权重之和
//
// 示例：
//   - bootstrap 节点: +50
//   - relay 节点: +50
//   - realm 成员: +100
//   - 出站连接: +10
//   - 有活跃流: +20
//
// # 常用标签
//
//   - "bootstrap": 引导节点（权重 50）
//   - "relay": 中继节点（权重 50）
//   - "realm-member": Realm 成员（权重 100）
//   - "dht-routing": DHT 路由表节点（权重 30）
//
// # 注意事项
//
// 1. 线程安全: 所有方法都是并发安全的
// 2. 保护优先: 受保护的连接永远不会被回收
// 3. 异步回收: TrimOpenConns 可以在后台执行
// 4. 上下文取消: 回收支持通过 context 取消
//
// # 性能考虑
//
// 1. 标签操作: O(1) 时间复杂度
// 2. 回收操作: O(n log n) 时间复杂度（排序）
// 3. 保护检查: O(1) 时间复杂度
//
// # 未来扩展
//
// 1. 衰减标签: 标签权重随时间衰减
// 2. 分段锁: 减少锁竞争（连接数 > 10000 时）
// 3. 内存监控: 低内存时强制回收
// 4. 后台定时回收: 定期检查并回收
package connmgr
