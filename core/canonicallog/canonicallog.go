package canonicallog

import (
	"fmt"
	"math/rand"
	"net"
	"strings"

	"github.com/dep2p/go-dep2p/core/peer"

	"github.com/dep2p/go-dep2p/multiformats/multiaddr"
	manet "github.com/dep2p/go-dep2p/multiformats/multiaddr/net"
	logging "github.com/dep2p/log"
)

// log 是一个带有跳过级别的日志记录器实例
// 使用 logging.WithSkip() 创建,跳过一个调用栈帧
var log = logging.WithSkip(logging.Logger("canonical-log"), 1)

// LogMisbehavingPeer 记录一个行为不当的对等节点的信息
// 协议应使用此函数来标识行为不当的对等节点,以便最终用户可以在不同协议和dep2p中轻松识别这些节点
//
// 参数:
//   - p: 对等节点ID
//   - peerAddr: 对等节点的多地址
//   - component: 组件名称
//   - err: 错误信息
//   - msg: 附加消息
func LogMisbehavingPeer(p peer.ID, peerAddr multiaddr.Multiaddr, component string, err error, msg string) {
	// 使用警告级别记录行为不当的对等节点信息
	log.Warnf("异常节点行为: 节点=%s 地址=%s 组件=%s 错误=%q 消息=%q", p, peerAddr.String(), component, err, msg)
}

// LogMisbehavingPeerNetAddr 记录一个行为不当的对等节点的信息,使用net.Addr作为地址
// 这是LogMisbehavingPeer的一个变体,接受标准net.Addr而不是multiaddr.Multiaddr
//
// 参数:
//   - p: 对等节点ID
//   - peerAddr: 对等节点的网络地址
//   - component: 组件名称
//   - originalErr: 原始错误信息
//   - msg: 附加消息
func LogMisbehavingPeerNetAddr(p peer.ID, peerAddr net.Addr, component string, originalErr error, msg string) {
	// 尝试将net.Addr转换为multiaddr
	ma, err := manet.FromNetAddr(peerAddr)
	if err != nil {
		// 如果转换失败,使用原始网络地址记录
		log.Warnf("异常节点行为: 节点=%s 网络地址=%s 组件=%s 错误=%q 消息=%q", p, peerAddr.String(), component, originalErr, msg)
		return
	}

	// 使用转换后的multiaddr调用标准的LogMisbehavingPeer
	LogMisbehavingPeer(p, ma, component, originalErr, msg)
}

// LogPeerStatus 记录对等节点的有用信息,带有采样率控制
// 此函数用于记录那些单独看似正常但大量出现可能异常的事件
//
// 参数:
//   - sampleRate: 采样率,只有1/sampleRate的消息会被记录
//   - p: 对等节点ID
//   - peerAddr: 对等节点的多地址
//   - keyVals: 键值对列表,用于记录额外信息
func LogPeerStatus(sampleRate int, p peer.ID, peerAddr multiaddr.Multiaddr, keyVals ...string) {
	// 随机采样,只有当随机数为0时才记录
	if rand.Intn(sampleRate) == 0 {
		// 构建键值对字符串
		keyValsStr := strings.Builder{}
		for i, kOrV := range keyVals {
			if i%2 == 0 {
				// 偶数索引为键
				fmt.Fprintf(&keyValsStr, " %v=", kOrV)
			} else {
				// 奇数索引为值
				fmt.Fprintf(&keyValsStr, "%q", kOrV)
			}
		}
		// 记录对等节点状态信息
		log.Infof("节点状态: 节点=%s 地址=%s 采样率=%v%s", p, peerAddr.String(), sampleRate, keyValsStr.String())
	}
}
