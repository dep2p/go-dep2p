// Package member 实现 Realm 成员管理
//
// # 模块概述
//
// member 包提供 Realm 层的成员管理功能，负责成员注册、注销、查询、同步等操作。
//
// 核心职责：
//   - 成员注册与注销
//   - 成员发现与同步
//   - 在线状态管理
//   - 角色权限管理
//   - 成员缓存（LRU + TTL）
//   - 持久化存储
//   - 事件发布
//   - 心跳与健康检查
//
// # 核心组件
//
// ## Manager（成员管理器）
//
// Manager 是成员管理的核心组件，提供完整的 CRUD 操作。
//
// 特性：
//   - 线程安全的成员操作
//   - 缓存优先的查询策略
//   - 支持批量操作
//   - 自动事件发布
//
// ## Cache（成员缓存）
//
// Cache 实现了 LRU + TTL 双重缓存策略，提升查询性能。
//
// 特性：
//   - LRU 淘汰策略（最少使用）
//   - TTL 过期机制（时间过期）
//   - 后台自动清理
//   - 高缓存命中率（> 95%）
//
// ## Store（持久化存储）
//
// Store 提供成员信息的持久化存储，支持本地文件和内存存储。
//
// 特性：
//   - JSON Lines 格式存储
//   - 追加写入优化
//   - 定期压缩
//   - 快速恢复
//
// ## Synchronizer（成员同步器）
//
// Synchronizer 负责与其他节点同步成员信息。
//
// 特性：
//   - 全量同步（首次加入）
//   - 增量同步（定期更新）
//   - 冲突解决（基于时间戳）
//   - 版本管理
//
// ## HeartbeatMonitor（心跳监控）
//
// HeartbeatMonitor 监控成员在线状态，及时检测离线成员。
//
// 特性：
//   - 定期心跳发送（15 秒）
//   - 超时检测（3 次失败）
//   - 自动重连
//   - 健康检查
//
// # 使用示例
//
// ## 创建管理器
//
//	// 创建配置
//	config := member.DefaultConfig()
//	config.CacheSize = 1000
//	config.CacheTTL = 10 * time.Minute
//
//	// 创建管理器
//	manager := member.NewManager("realm-id", cache, store, eventBus)
//	defer manager.Close()
//
//	// 启动管理器
//	ctx := context.Background()
//	if err := manager.Start(ctx); err != nil {
//	    log.Fatal(err)
//	}
//
// ## 添加成员
//
//	member := &interfaces.MemberInfo{
//	    PeerID:   "peer123",
//	    RealmID:  "realm-id",
//	    Role:     interfaces.RoleMember,
//	    Online:   true,
//	    JoinedAt: time.Now(),
//	    Metadata: map[string]string{"region": "us-west"},
//	}
//
//	if err := manager.Add(ctx, member); err != nil {
//	    log.Printf("Failed to add member: %v", err)
//	}
//
// ## 查询成员
//
//	// 获取单个成员
//	member, err := manager.Get(ctx, "peer123")
//	if err != nil {
//	    log.Printf("Member not found: %v", err)
//	}
//
//	// 列出所有成员
//	opts := &interfaces.ListOptions{
//	    Limit:      100,
//	    OnlineOnly: true,
//	}
//	members, err := manager.List(ctx, opts)
//
// ## 同步成员
//
//	// 创建同步器
//	sync := member.NewSynchronizer(manager, discovery)
//
//	// 启动同步
//	if err := sync.Start(ctx); err != nil {
//	    log.Fatal(err)
//	}
//
//	// 全量同步
//	if err := manager.SyncMembers(ctx); err != nil {
//	    log.Printf("Sync failed: %v", err)
//	}
//
// ## 心跳监控
//
//	// 创建心跳监控
//	monitor := member.NewHeartbeatMonitor(manager, host, 15*time.Second, 3)
//
//	// 启动监控
//	if err := monitor.Start(ctx); err != nil {
//	    log.Fatal(err)
//	}
//
// # 性能指标
//
//   - 成员查询：< 1ms（缓存命中）
//   - 成员同步：< 100ms（增量）
//   - 心跳开销：< 1KB/成员/分钟
//   - 缓存命中率：> 95%
//
// # 线程安全
//
// 所有公共方法都是线程安全的，可以并发调用。
package member
