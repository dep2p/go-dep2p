package mocknet

import (
	"fmt"
	"io"

	"github.com/dep2p/go-dep2p/core/network"
	"github.com/dep2p/go-dep2p/core/peer"
)

// printer 用于打印网络信息的辅助结构体
// 将接口分离为独立对象
type printer struct {
	w io.Writer // 输出写入器
}

// MocknetLinks 打印模拟网络的链路映射信息
// 参数:
//   - mn: Mocknet 模拟网络对象
func (p *printer) MocknetLinks(mn Mocknet) {
	// 获取所有链路信息
	links := mn.Links()

	// 打印链路映射标题
	fmt.Fprintf(p.w, "Mocknet link map:\n")
	// 遍历每个节点的链路映射
	for p1, lm := range links {
		// 打印当前节点的链接信息
		fmt.Fprintf(p.w, "\t%s linked to:\n", peer.ID(p1))
		// 遍历当前节点连接的所有对端节点
		for p2, l := range lm {
			// 打印对端节点ID和链路数量
			fmt.Fprintf(p.w, "\t\t%s (%d links)\n", peer.ID(p2), len(l))
		}
	}
	// 打印空行分隔
	fmt.Fprintf(p.w, "\n")
}

// NetworkConns 打印网络连接信息
// 参数:
//   - ni: network.Network 网络对象
func (p *printer) NetworkConns(ni network.Network) {
	// 打印本地节点ID和连接标题
	fmt.Fprintf(p.w, "%s connected to:\n", ni.LocalPeer())
	// 遍历所有连接
	for _, c := range ni.Conns() {
		// 打印每个连接的远程节点ID和地址
		fmt.Fprintf(p.w, "\t%s (addr: %s)\n", c.RemotePeer(), c.RemoteMultiaddr())
	}
	// 打印空行分隔
	fmt.Fprintf(p.w, "\n")
}
