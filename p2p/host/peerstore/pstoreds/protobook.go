package pstoreds

import (
	"errors"
	"fmt"
	"sync"

	"github.com/dep2p/go-dep2p/core/peer"
	pstore "github.com/dep2p/go-dep2p/core/peerstore"
	"github.com/dep2p/go-dep2p/core/protocol"
)

// protoSegment 协议段结构体,用于并发控制
type protoSegment struct {
	sync.RWMutex // 读写锁
}

// protoSegments 协议段数组,大小为256
type protoSegments [256]*protoSegment

// get 根据peer ID获取对应的协议段
// 参数:
//   - p: peer ID
//
// 返回:
//   - *protoSegment: 对应的协议段
func (s *protoSegments) get(p peer.ID) *protoSegment {
	return s[p[len(p)-1]] // 使用peer ID的最后一个字节作为索引
}

// errTooManyProtocols 协议数量超出限制的错误
var errTooManyProtocols = errors.New("协议数量超出限制")

// ProtoBookOption 协议簿配置选项函数类型
type ProtoBookOption func(*dsProtoBook) error

// WithMaxProtocols 设置最大协议数量的选项
// 参数:
//   - num: 最大协议数量
//
// 返回:
//   - ProtoBookOption: 配置选项函数
func WithMaxProtocols(num int) ProtoBookOption {
	return func(pb *dsProtoBook) error {
		pb.maxProtos = num
		return nil
	}
}

// dsProtoBook 数据存储协议簿结构体
type dsProtoBook struct {
	segments  protoSegments       // 协议段数组
	meta      pstore.PeerMetadata // peer元数据接口
	maxProtos int                 // 最大协议数量
}

// 确保dsProtoBook实现了ProtoBook接口
var _ pstore.ProtoBook = (*dsProtoBook)(nil)

// NewProtoBook 创建新的协议簿
// 参数:
//   - meta: peer元数据接口
//   - opts: 配置选项
//
// 返回:
//   - *dsProtoBook: 新创建的协议簿
//   - error: 错误信息
func NewProtoBook(meta pstore.PeerMetadata, opts ...ProtoBookOption) (*dsProtoBook, error) {
	pb := &dsProtoBook{
		meta: meta,
		segments: func() (ret protoSegments) {
			for i := range ret {
				ret[i] = &protoSegment{} // 初始化所有协议段
			}
			return ret
		}(),
		maxProtos: 128, // 默认最大协议数量为128
	}

	// 应用配置选项
	for _, opt := range opts {
		if err := opt(pb); err != nil {
			log.Debugf("应用协议簿选项失败: %v", err)
			return nil, err
		}
	}
	return pb, nil
}

// SetProtocols 设置peer支持的协议列表
// 参数:
//   - p: peer ID
//   - protos: 协议ID列表
//
// 返回:
//   - error: 错误信息
func (pb *dsProtoBook) SetProtocols(p peer.ID, protos ...protocol.ID) error {
	if len(protos) > pb.maxProtos {
		return errTooManyProtocols
	}

	// 创建协议映射
	protomap := make(map[protocol.ID]struct{}, len(protos))
	for _, proto := range protos {
		protomap[proto] = struct{}{}
	}

	s := pb.segments.get(p)
	s.Lock()
	defer s.Unlock()

	return pb.meta.Put(p, "protocols", protomap)
}

// AddProtocols 为peer添加协议
// 参数:
//   - p: peer ID
//   - protos: 要添加的协议ID列表
//
// 返回:
//   - error: 错误信息
func (pb *dsProtoBook) AddProtocols(p peer.ID, protos ...protocol.ID) error {
	s := pb.segments.get(p)
	s.Lock()
	defer s.Unlock()

	pmap, err := pb.getProtocolMap(p)
	if err != nil {
		log.Debugf("获取协议映射失败: %v", err)
		return err
	}
	if len(pmap)+len(protos) > pb.maxProtos {
		log.Debugf("协议数量超出限制: %v", errTooManyProtocols)
		return errTooManyProtocols
	}

	// 添加新协议
	for _, proto := range protos {
		pmap[proto] = struct{}{}
	}

	return pb.meta.Put(p, "protocols", pmap)
}

// GetProtocols 获取peer支持的所有协议
// 参数:
//   - p: peer ID
//
// 返回:
//   - []protocol.ID: 协议ID列表
//   - error: 错误信息
func (pb *dsProtoBook) GetProtocols(p peer.ID) ([]protocol.ID, error) {
	s := pb.segments.get(p)
	s.RLock()
	defer s.RUnlock()

	pmap, err := pb.getProtocolMap(p)
	if err != nil {
		log.Debugf("获取协议映射失败: %v", err)
		return nil, err
	}

	// 将map转换为切片
	res := make([]protocol.ID, 0, len(pmap))
	for proto := range pmap {
		res = append(res, proto)
	}

	return res, nil
}

// SupportsProtocols 检查peer支持的协议
// 参数:
//   - p: peer ID
//   - protos: 要检查的协议ID列表
//
// 返回:
//   - []protocol.ID: 支持的协议ID列表
//   - error: 错误信息
func (pb *dsProtoBook) SupportsProtocols(p peer.ID, protos ...protocol.ID) ([]protocol.ID, error) {
	s := pb.segments.get(p)
	s.RLock()
	defer s.RUnlock()

	pmap, err := pb.getProtocolMap(p)
	if err != nil {
		log.Debugf("获取协议映射失败: %v", err)
		return nil, err
	}

	// 检查每个协议是否支持
	res := make([]protocol.ID, 0, len(protos))
	for _, proto := range protos {
		if _, ok := pmap[proto]; ok {
			res = append(res, proto)
		}
	}

	return res, nil
}

// FirstSupportedProtocol 获取peer支持的第一个协议
// 参数:
//   - p: peer ID
//   - protos: 要检查的协议ID列表
//
// 返回:
//   - protocol.ID: 支持的第一个协议ID
//   - error: 错误信息
func (pb *dsProtoBook) FirstSupportedProtocol(p peer.ID, protos ...protocol.ID) (protocol.ID, error) {
	s := pb.segments.get(p)
	s.RLock()
	defer s.RUnlock()

	pmap, err := pb.getProtocolMap(p)
	if err != nil {
		log.Debugf("获取协议映射失败: %v", err)
		return "", err
	}
	// 返回第一个支持的协议
	for _, proto := range protos {
		if _, ok := pmap[proto]; ok {
			return proto, nil
		}
	}

	return "", nil
}

// RemoveProtocols 移除peer的协议
// 参数:
//   - p: peer ID
//   - protos: 要移除的协议ID列表
//
// 返回:
//   - error: 错误信息
func (pb *dsProtoBook) RemoveProtocols(p peer.ID, protos ...protocol.ID) error {
	s := pb.segments.get(p)
	s.Lock()
	defer s.Unlock()

	pmap, err := pb.getProtocolMap(p)
	if err != nil {
		log.Debugf("获取协议映射失败: %v", err)
		return err
	}

	if len(pmap) == 0 {
		// 没有协议需要移除
		return nil
	}

	// 移除指定的协议
	for _, proto := range protos {
		delete(pmap, proto)
	}

	return pb.meta.Put(p, "protocols", pmap)
}

// getProtocolMap 获取peer的协议映射
// 参数:
//   - p: peer ID
//
// 返回:
//   - map[protocol.ID]struct{}: 协议映射
//   - error: 错误信息
func (pb *dsProtoBook) getProtocolMap(p peer.ID) (map[protocol.ID]struct{}, error) {
	iprotomap, err := pb.meta.Get(p, "protocols")
	switch err {
	default:
		return nil, err
	case pstore.ErrNotFound:
		return make(map[protocol.ID]struct{}), nil
	case nil:
		cast, ok := iprotomap.(map[protocol.ID]struct{})
		if !ok {
			log.Debugf("存储的协议集不是map")
			return nil, fmt.Errorf("存储的协议集不是map")
		}

		return cast, nil
	}
}

// RemovePeer 移除peer的所有信息
// 参数:
//   - p: peer ID
func (pb *dsProtoBook) RemovePeer(p peer.ID) {
	pb.meta.RemovePeer(p)
}
