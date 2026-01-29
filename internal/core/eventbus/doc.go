// Package eventbus 实现进程内事件总线
//
// 提供类型安全的事件发布/订阅机制，支持：
//   - 多订阅者
//   - 缓冲区配置
//   - 发射器引用计数
//   - 并发安全
//   - 有状态模式（Stateful）
//
// # 快速开始
//
//	// 创建总线
//	bus := eventbus.NewBus()
//
//	// 订阅事件
//	sub, _ := bus.Subscribe(new(MyEvent))
//	defer sub.Close()
//
//	go func() {
//	    for evt := range sub.Out() {
//	        e := evt.(MyEvent)
//	        // 处理事件
//	    }
//	}()
//
//	// 发射事件
//	em, _ := bus.Emitter(new(MyEvent))
//	defer em.Close()
//	em.Emit(MyEvent{...})
//
// # Fx 模块
//
//	import "go.uber.org/fx"
//
//	app := fx.New(
//	    eventbus.Module(),
//	    fx.Invoke(func(bus pkgif.EventBus) {
//	        sub, _ := bus.Subscribe(new(MyEvent))
//	        // ...
//	    }),
//	)
//
// # 架构定位
//
// Tier: Core Layer Level 1（无依赖）
//
// 依赖关系：
//   - 依赖：pkg/interfaces
//   - 被依赖：host, swarm, discovery
//
// # 并发安全
//
// EventBus 使用 sync.RWMutex 和 atomic 保证并发安全：
//   - 订阅/取消订阅：RWMutex 保护
//   - 发射器引用计数：atomic.Int32
//   - 通道关闭：closeOnce 防止重复
//
// # 相关文档
//
//   - 设计文档：design/03_architecture/L6_domains/core_eventbus/
//   - 接口定义：pkg/interfaces/eventbus.go
package eventbus
