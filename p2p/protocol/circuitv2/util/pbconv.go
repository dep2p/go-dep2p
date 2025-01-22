package util

import (
	"errors"

	"github.com/dep2p/go-dep2p/core/peer"
	pbv2 "github.com/dep2p/go-dep2p/p2p/protocol/circuitv2/pb"

	ma "github.com/dep2p/go-dep2p/multiformats/multiaddr"
)

// PeerToPeerInfoV2 将 protobuf 格式的 Peer 转换为 AddrInfo
// 参数:
//   - p: *pbv2.Peer protobuf 格式的 Peer 对象
//
// 返回值:
//   - peer.AddrInfo 转换后的 AddrInfo 对象
//   - error 转换过程中的错误
func PeerToPeerInfoV2(p *pbv2.Peer) (peer.AddrInfo, error) {
	// 检查输入参数是否为空
	if p == nil {
		log.Debugf("peer 对象为空")
		return peer.AddrInfo{}, errors.New("peer 对象为空")
	}

	// 从字节数组解析出 peer ID
	id, err := peer.IDFromBytes(p.Id)
	if err != nil {
		log.Debugf("从字节数组解析出 peer ID 失败: %v", err)
		return peer.AddrInfo{}, err
	}

	// 初始化地址数组
	addrs := make([]ma.Multiaddr, 0, len(p.Addrs))

	// 遍历并解析所有地址
	for _, addrBytes := range p.Addrs {
		a, err := ma.NewMultiaddrBytes(addrBytes)
		if err == nil {
			addrs = append(addrs, a)
		}
	}

	// 返回组装好的 AddrInfo
	return peer.AddrInfo{ID: id, Addrs: addrs}, nil
}

// PeerInfoToPeerV2 将 AddrInfo 转换为 protobuf 格式的 Peer
// 参数:
//   - pi: peer.AddrInfo 要转换的 AddrInfo 对象
//
// 返回值:
//   - *pbv2.Peer 转换后的 protobuf 格式 Peer 对象
func PeerInfoToPeerV2(pi peer.AddrInfo) *pbv2.Peer {
	// 初始化地址字节数组
	addrs := make([][]byte, 0, len(pi.Addrs))
	// 遍历并转换所有地址为字节数组
	for _, addr := range pi.Addrs {
		addrs = append(addrs, addr.Bytes())
	}

	// 返回组装好的 protobuf Peer
	return &pbv2.Peer{
		Id:    []byte(pi.ID),
		Addrs: addrs,
	}
}
