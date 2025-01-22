package pnet

import "os"

// EnvKey 定义了一个环境变量名称,用于强制在 dep2p 中使用私有网络(PNet)
// 当此环境变量的值设置为 "1" 时,ForcePrivateNetwork 变量将被设置为 true
const EnvKey = "LIBP2P_FORCE_PNET"

// ForcePrivateNetwork 是一个布尔变量,用于强制在 dep2p 中使用私有网络
// 将此变量设置为 true 或将 LIBP2P_FORCE_PNET 环境变量设置为 true 将使 dep2p 要求使用私有网络保护器
// 如果未提供网络保护器且此变量设置为 true,dep2p 将拒绝连接
var ForcePrivateNetwork = false

// init 初始化函数,在包被导入时自动执行
// 功能:
//   - 检查环境变量 LIBP2P_FORCE_PNET 的值
//   - 如果环境变量值为 "1",则将 ForcePrivateNetwork 设置为 true
func init() {
	ForcePrivateNetwork = os.Getenv(EnvKey) == "1"
}
