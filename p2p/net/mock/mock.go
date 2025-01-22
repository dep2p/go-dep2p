package mocknet

import (
	logging "github.com/dep2p/log"
)

// 日志记录器
var log = logging.Logger("net-mocknet")

// WithNPeers 构造一个包含N个节点的模拟网络
// 参数:
//   - n: int 节点数量
//
// 返回值:
//   - Mocknet: 创建的模拟网络
//   - error: 错误信息
func WithNPeers(n int) (Mocknet, error) {
	// 创建新的模拟网络
	m := New()
	// 生成n个节点
	for i := 0; i < n; i++ {
		if _, err := m.GenPeer(); err != nil {
			log.Debugf("生成节点失败: %v", err)
			return nil, err
		}
	}
	return m, nil
}

// FullMeshLinked 构造一个全连接的模拟网络
// 这意味着所有节点都**可以**相互连接
// (注意这并不意味着它们已经连接,你可以使用m.ConnectAll()来连接它们)
// 参数:
//   - n: int 节点数量
//
// 返回值:
//   - Mocknet: 创建的模拟网络
//   - error: 错误信息
func FullMeshLinked(n int) (Mocknet, error) {
	// 创建包含n个节点的模拟网络
	m, err := WithNPeers(n)
	if err != nil {
		log.Debugf("创建节点失败: %v", err)
		return nil, err
	}

	// 在所有节点间建立链路
	if err := m.LinkAll(); err != nil {
		log.Debugf("建立链路失败: %v", err)
		return nil, err
	}
	return m, nil
}

// FullMeshConnected 构造一个全连接的模拟网络
// 这意味着所有节点都已经拨号并准备好相互通信
// 参数:
//   - n: int 节点数量
//
// 返回值:
//   - Mocknet: 创建的模拟网络
//   - error: 错误信息
func FullMeshConnected(n int) (Mocknet, error) {
	// 创建全连接的链路网络
	m, err := FullMeshLinked(n)
	if err != nil {
		log.Debugf("创建全连接链路网络失败: %v", err)
		return nil, err
	}

	// 连接所有节点(除了自身)
	if err := m.ConnectAllButSelf(); err != nil {
		log.Debugf("连接所有节点失败: %v", err)
		return nil, err
	}
	return m, nil
}
