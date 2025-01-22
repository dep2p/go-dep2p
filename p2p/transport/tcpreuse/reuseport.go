package tcpreuse

import (
	"os"
	"strings"

	"github.com/dep2p/go-dep2p/p2plib/reuseport"
)

// envReuseport 是用于关闭端口重用的环境变量名称
// 默认为 true
const envReuseport = "LIBP2P_TCP_REUSEPORT"

// EnvReuseportVal 存储 envReuseport 的值,默认为 true
var EnvReuseportVal = true

// init 初始化函数
// 功能:
//   - 从环境变量中读取端口重用配置并设置 EnvReuseportVal
func init() {
	// 获取环境变量值并转为小写
	v := strings.ToLower(os.Getenv(envReuseport))
	// 如果值为 false/f/0,则禁用端口重用
	if v == "false" || v == "f" || v == "0" {
		EnvReuseportVal = false
		// log.Infof("端口重用已禁用 (LIBP2P_TCP_REUSEPORT=%s)", v)
	}
}

// ReuseportIsAvailable 返回端口重用功能是否可用
// 返回值:
//   - bool 端口重用是否可用
//
// 说明:
//   - 这里允许我们选择性地开启或关闭端口重用
//   - 目前通过环境变量控制,满足当前需求:
//     LIBP2P_TCP_REUSEPORT=false ipfs daemon
//   - 如果这成为一个受欢迎的功能,我们可以将其添加到配置中
//   - 最终,端口重用是一个权宜之计
func ReuseportIsAvailable() bool {
	return EnvReuseportVal && reuseport.Available()
}
