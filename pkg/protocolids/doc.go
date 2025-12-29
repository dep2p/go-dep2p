// Package protocolids 定义 DeP2P 所有协议的唯一协议 ID 注册表。
//
// # 唯一真源原则
//
// 本包是 DeP2P 项目中协议 ID 的**唯一权威来源**。所有模块、测试、示例、CLI
// 工具在需要协议 ID 时，必须引用本包中的常量，禁止在其他位置定义字面量。
//
// # 协议命名规范
//
//   - 系统协议: /dep2p/sys/{name}/{version}
//     系统协议是 DeP2P 基础设施的一部分，无需 RealmAuth 验证即可使用。
//     例如: /dep2p/sys/ping/1.0.0, /dep2p/sys/relay/1.0.0
//
//   - 应用协议: /dep2p/app/{name}/{version}
//     应用协议是业务层协议，默认需要 RealmAuth 验证后才能使用。
//     例如: /dep2p/app/chat/1.0.0, /dep2p/app/messaging/request/1.0.0
//
//   - 测试协议: /dep2p/sys/test/{name}/{version}
//     测试专用协议，属于系统协议范畴，仅在测试环境中使用。
//     例如: /dep2p/sys/test/echo/1.0.0
//
// # 新增协议流程
//
//  1. 在本包的 sys.go 或 app.go 中定义常量
//  2. 更新 docs/01-design/architecture/protocol-registry.md
//  3. 在对应模块中引用新常量实现协议
//
// # CI 强制校验
//
// 项目 CI 通过 tests/protocolids/lint_test.go 扫描整个仓库，
// 禁止在非允许文件（pkg/protocolids/** 和 protocol-registry.md）中
// 出现 "/dep2p/" 字面量。违规将导致构建失败。
//
// # 版本升级策略
//
// 协议版本采用语义化版本号（SemVer），但仅使用 major.minor 形式：
//   - major 变更: 不兼容的协议格式变化
//   - minor 变更: 向后兼容的功能增强
//
// 参考: docs/01-design/architecture/protocol-registry.md
package protocolids

