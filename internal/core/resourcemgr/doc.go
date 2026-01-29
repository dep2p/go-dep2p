// Package resourcemgr 实现资源管理
//
// 提供层次化的资源限制和作用域管理，支持：
//   - 连接数/流数/内存/FD 限制
//   - 7 层作用域层次（System/Transient/Service/Protocol/Peer/Connection/Stream）
//   - 资源预留与释放
//   - 优先级控制（Low/Medium/High/Always）
//   - 并发安全（atomic + mutex）
//
// # 快速开始
//
//	cfg := DefaultLimitConfig()
//	rm, _ := NewResourceManager(cfg)
//	defer rm.Close()
//
//	// 打开连接
//	connScope, _ := rm.OpenConnection(pkgif.DirInbound, true, addr)
//	defer connScope.Done()
//
//	// 打开流
//	streamScope, _ := rm.OpenStream(peerID, pkgif.DirOutbound)
//	defer streamScope.Done()
//
//	// 预留内存
//	span, _ := streamScope.BeginSpan()
//	defer span.Done()
//	span.ReserveMemory(1024, pkgif.ReservationPriorityHigh)
//
// # 作用域层次
//
//	System (系统级)
//	  ├─ Transient (临时资源，握手期间)
//	  ├─ Service[name] (服务级)
//	  ├─ Protocol[id] (协议级)
//	  └─ Peer[id] (节点级)
//	       ├─ Connection (连接级)
//	       └─ Stream (流级)
//
// 资源使用向上累积到父作用域。
//
// # Fx 模块
//
//	import "go.uber.org/fx"
//
//	app := fx.New(
//	    resourcemgr.Module,
//	    fx.Invoke(func(rm pkgif.ResourceManager) {
//	        rm.ViewSystem(func(s pkgif.ResourceScope) error {
//	            stat := s.Stat()
//	            fmt.Printf("System: %+v\n", stat)
//	            return nil
//	        })
//	    }),
//	)
//
// # 架构定位
//
// Tier: Core Layer Level 1（无依赖）
//
// 依赖关系：
//   - 依赖：pkg/interfaces, pkg/types, pkg/multiaddr
//   - 被依赖：swarm, connmgr, host
//
// # 并发安全
//
// ResourceManager 使用 sync.Mutex 和 atomic 保证并发安全：
//   - 作用域管理：sync.Mutex 保护 map（冷路径）
//   - 资源计数：atomic 操作（热路径）
//   - 关闭操作：sync.Once + atomic.Bool
//
// # 资源限制
//
// 资源类型：
//   - Streams: 流数量（入站/出站）
//   - Conns: 连接数量（入站/出站）
//   - FD: 文件描述符数量
//   - Memory: 内存使用量（字节）
//
// 预留优先级：
//   - Low (101): <= 40% 利用率时预留
//   - Medium (152): <= 60% 利用率时预留
//   - High (203): <= 80% 利用率时预留
//   - Always (255): 只要有资源就预留
//
// # 相关文档
//
//   - 设计文档：design/03_architecture/L6_domains/core_resourcemgr/
//   - 接口定义：pkg/interfaces/resource.go
package resourcemgr
