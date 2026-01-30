package dep2p

// ════════════════════════════════════════════════════════════════════════════
//                              版本信息
// ════════════════════════════════════════════════════════════════════════════

// Version 当前版本
// 更新此版本号时，请同步更新 version.json
const Version = "v0.2.0-beta.1"

// BuildInfo 构建信息（通过 ldflags 注入）
var (
	// GitCommit Git 提交哈希
	GitCommit string

	// BuildDate 构建日期
	BuildDate string

	// GoVersion Go 版本
	GoVersion string
)

// VersionInfo 返回完整版本信息字符串
func VersionInfo() string {
	info := "DeP2P " + Version
	if GitCommit != "" {
		info += " (" + GitCommit[:min(8, len(GitCommit))] + ")"
	}
	if BuildDate != "" {
		info += " built " + BuildDate
	}
	return info
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// ════════════════════════════════════════════════════════════════════════════
//                              类型别名
// ════════════════════════════════════════════════════════════════════════════

// Endpoint 是 Node 的类型别名
//
// 为兼容旧 API 提供，新代码应直接使用 *Node。
// Endpoint 提供网络端点功能：
//   - ID() 获取节点 ID
//   - ListenAddrs() 获取监听地址
//   - ConnectionCount() 获取连接数
//   - Close() 关闭端点
type Endpoint = *Node
