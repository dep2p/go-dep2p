package pstoremem

import (
	"bytes"

	ma "github.com/dep2p/go-dep2p/multiformats/multiaddr"
	mafmt "github.com/dep2p/go-dep2p/multiformats/multiaddr/fmt"
	manet "github.com/dep2p/go-dep2p/multiformats/multiaddr/net"
)

// isFDCostlyTransport 检查地址是否使用消耗文件描述符的传输协议
// 参数:
//   - a: 多地址
//
// 返回:
//   - bool: 如果是TCP等消耗文件描述符的传输协议返回true
func isFDCostlyTransport(a ma.Multiaddr) bool {
	return mafmt.TCP.Matches(a) // 检查是否为TCP协议
}

// addrList 多地址列表类型
type addrList []ma.Multiaddr

// Len 返回地址列表长度
// 返回:
//   - int: 列表长度
func (al addrList) Len() int { return len(al) }

// Swap 交换列表中两个地址的位置
// 参数:
//   - i: 第一个位置索引
//   - j: 第二个位置索引
func (al addrList) Swap(i, j int) { al[i], al[j] = al[j], al[i] }

// Less 比较两个地址的优先级
// 参数:
//   - i: 第一个地址索引
//   - j: 第二个地址索引
//
// 返回:
//   - bool: 如果地址i应排在地址j前面则返回true
func (al addrList) Less(i, j int) bool {
	a := al[i] // 获取第一个地址
	b := al[j] // 获取第二个地址

	lba := manet.IsIPLoopback(a) // 检查a是否为本地回环地址
	lbb := manet.IsIPLoopback(b) // 检查b是否为本地回环地址
	if lba && !lbb {             // 如果a是本地回环而b不是
		return true // 优先选择本地回环地址
	}

	fda := isFDCostlyTransport(a) // 检查a是否消耗文件描述符
	fdb := isFDCostlyTransport(b) // 检查b是否消耗文件描述符
	if !fda {                     // 如果a不消耗文件描述符
		return fdb // 优先选择不消耗文件描述符的地址
	}

	if !fdb { // 如果b不消耗文件描述符
		return false // b优先级更高
	}

	if lbb { // 如果b是本地回环且都消耗文件描述符
		return false // b优先级更高
	}

	return bytes.Compare(a.Bytes(), b.Bytes()) > 0 // 其他情况按字节序比较
}
