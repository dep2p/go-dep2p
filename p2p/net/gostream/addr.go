package gostream

import "github.com/dep2p/go-dep2p/core/peer"

// addr 实现了 net.Addr 接口并持有一个 dep2p peer ID
type addr struct {
	// id 表示节点的唯一标识符
	id peer.ID
}

// Network 返回该地址所属的网络名称 (dep2p)
// 参数:
//   - a: addr 结构体指针
//
// 返回值:
//   - string: 网络名称,固定返回 Network 常量
func (a *addr) Network() string {
	// 返回预定义的网络名称常量
	return Network
}

// String 将该地址的 peer ID 转换为字符串形式 (B58编码)
// 参数:
//   - a: addr 结构体指针
//
// 返回值:
//   - string: B58编码的 peer ID 字符串
func (a *addr) String() string {
	// 调用 peer.ID 的 String() 方法将 ID 转换为字符串
	return a.id.String()
}
