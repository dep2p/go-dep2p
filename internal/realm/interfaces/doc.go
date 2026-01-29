// Package interfaces 定义 Realm 层内部接口
//
// 本包提供 Realm 层各子模块之间的内部接口定义，
// 用于解耦模块依赖，支持依赖注入和测试。
//
// # 接口分类
//
// ## 核心接口
//
//   - Realm: Realm 核心抽象
//   - Member: 成员管理接口
//   - Auth: 认证接口
//
// ## 网络接口
//
//   - Gateway: 域网关接口
//   - Routing: 路由接口
//   - AddressBook: 地址簿接口
//
// # 与 pkg/interfaces 的关系
//
// 本包定义的是 Realm 层内部使用的接口，
// 而 pkg/interfaces 定义的是对外公开的接口。
//
// 内部接口可能包含更多实现细节，
// 公开接口则更加稳定和通用。
//
// # 使用示例
//
//	// 在子模块中依赖接口而非具体实现
//	type Service struct {
//	    memberMgr interfaces.MemberManager
//	    auth      interfaces.Authenticator
//	}
//
//	func NewService(mm interfaces.MemberManager, auth interfaces.Authenticator) *Service {
//	    return &Service{
//	        memberMgr: mm,
//	        auth:      auth,
//	    }
//	}
package interfaces
