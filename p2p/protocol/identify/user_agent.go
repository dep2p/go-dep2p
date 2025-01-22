package identify

import (
	"fmt"
	"runtime/debug"
)

// init 初始化默认用户代理字符串
//
// 功能:
//   - 从编译信息中读取版本信息并设置默认用户代理
//   - 如果是开发版本,则使用git commit hash作为版本号
//
// 注意:
//   - 只有作为其他模块的依赖构建时才会有非空版本号
func init() {
	// 读取编译信息
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return
	}

	// 获取主模块版本号
	version := bi.Main.Version
	// 只有作为其他模块的依赖构建时版本号才会非空
	if version == "" {
		return
	}

	// 如果不是开发版本,直接使用版本号
	if version != "(devel)" {
		defaultUserAgent = fmt.Sprintf("%s@%s", bi.Main.Path, bi.Main.Version)
		return
	}

	// 从编译设置中获取git信息
	var revision string
	var dirty bool
	for _, bs := range bi.Settings {
		switch bs.Key {
		case "vcs.revision":
			// 获取git commit hash
			revision = bs.Value
			// 只保留前9位
			if len(revision) > 9 {
				revision = revision[:9]
			}
		case "vcs.modified":
			// 检查是否有未提交的修改
			if bs.Value == "true" {
				dirty = true
			}
		}
	}

	// 设置用户代理字符串
	defaultUserAgent = fmt.Sprintf("%s@%s", bi.Main.Path, revision)
	// 如果有未提交的修改,添加dirty标记
	if dirty {
		defaultUserAgent += "-dirty"
	}
}
