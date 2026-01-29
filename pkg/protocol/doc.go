// Package protocol 定义 DeP2P 协议标识符
//
// 本包是 DeP2P 所有协议 ID 的单一真相源 (Single Source of Truth)。
// 所有模块应从此包引用协议常量，而不是自行定义字符串。
//
// # 协议分类
//
// DeP2P 协议分为三类：
//
//   - 系统协议 (System): 基础设施协议，无需 Realm 成员资格
//     格式: /dep2p/sys/<protocol>/<version>
//     例如: /dep2p/sys/ping/1.0.0
//
//   - Relay 协议 (Relay): 系统协议的特例，用于中继功能
//     格式: /dep2p/relay/<version>/{hop,stop}
//     例如: /dep2p/relay/1.0.0/hop
//
//   - Realm 协议 (Realm): 控制协议，需要 Realm 成员资格
//     格式: /dep2p/realm/<realmID>/<protocol>/<version>
//     例如: /dep2p/realm/my-realm/auth/1.0.0
//
//   - 应用协议 (App): 业务协议，需要 Realm 成员资格
//     格式: /dep2p/app/<realmID>/<protocol>/<version>
//     例如: /dep2p/app/my-realm/messaging/1.0.0
//
// # 使用示例
//
// 系统协议使用常量直接引用：
//
//	import "github.com/dep2p/go-dep2p/pkg/protocol"
//
//	host.SetStreamHandler(protocol.Ping, pingHandler)
//	host.SetStreamHandler(protocol.RelayHop, relayHandler)
//
// Realm 协议使用构建器：
//
//	realm := protocol.NewRealmBuilder(realmID)
//	host.SetStreamHandler(realm.Auth(), authHandler)
//	host.SetStreamHandler(realm.Sync(), syncHandler)
//
// App 协议使用构建器：
//
//	app := protocol.NewAppBuilder(realmID)
//	host.SetStreamHandler(app.Messaging(), msgHandler)
//
// # 版本规范
//
// 协议版本遵循语义化版本号 (SemVer)：
//   - 主版本：不兼容的协议变更
//   - 次版本：向后兼容的功能添加
//   - 修订版：向后兼容的问题修正
//
// 当前所有协议使用版本 1.0.0。
package protocol
