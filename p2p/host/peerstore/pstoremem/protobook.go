package pstoremem

import (
	"errors"
	"sync"

	"github.com/dep2p/go-dep2p/core/peer"
	pstore "github.com/dep2p/go-dep2p/core/peerstore"
	"github.com/dep2p/go-dep2p/core/protocol"
)

// protoSegment 协议段结构
type protoSegment struct {
	sync.RWMutex                                      // 读写锁,用于并发控制
	protocols    map[peer.ID]map[protocol.ID]struct{} // 存储节点支持的协议映射
}

// protoSegments 协议段数组,用于分片存储
type protoSegments [256]*protoSegment

// get 根据对等节点ID获取对应的协议段
// 参数:
//   - p: 对等节点ID
//
// 返回:
//   - *protoSegment: 协议段
func (s *protoSegments) get(p peer.ID) *protoSegment {
	return s[p[len(p)-1]] // 使用ID的最后一个字节作为索引
}

// errTooManyProtocols 协议数量超出限制错误
var errTooManyProtocols = errors.New("协议数量超出限制")

// memoryProtoBook 内存协议簿实现
type memoryProtoBook struct {
	segments  protoSegments // 协议段数组
	maxProtos int           // 最大协议数量限制
}

// 确保 memoryProtoBook 实现了 ProtoBook 接口
var _ pstore.ProtoBook = (*memoryProtoBook)(nil)

// ProtoBookOption 定义协议簿选项函数类型
type ProtoBookOption func(book *memoryProtoBook) error

// WithMaxProtocols 设置最大协议数量的选项
// 参数:
//   - num: 最大协议数量
//
// 返回:
//   - ProtoBookOption: 协议簿选项函数
func WithMaxProtocols(num int) ProtoBookOption {
	return func(pb *memoryProtoBook) error {
		pb.maxProtos = num // 设置最大协议数量
		return nil
	}
}

// NewProtoBook 创建新的内存协议簿
// 参数:
//   - opts: 可选的配置选项
//
// 返回:
//   - *memoryProtoBook: 内存协议簿实例
//   - error: 错误信息
func NewProtoBook(opts ...ProtoBookOption) (*memoryProtoBook, error) {
	pb := &memoryProtoBook{
		segments: func() (ret protoSegments) { // 初始化协议段数组
			for i := range ret { // 遍历所有段
				ret[i] = &protoSegment{ // 创建新的协议段
					protocols: make(map[peer.ID]map[protocol.ID]struct{}), // 初始化协议映射
				}
			}
			return ret
		}(),
		maxProtos: 128, // 设置默认最大协议数量
	}

	for _, opt := range opts { // 应用选项
		if err := opt(pb); err != nil { // 如果选项应用失败
			log.Debugf("应用协议簿选项失败: %v", err)
			return nil, err // 返回错误
		}
	}
	return pb, nil
}

// SetProtocols 设置对等节点支持的协议列表
// 参数:
//   - p: 对等节点ID
//   - protos: 协议ID列表
//
// 返回:
//   - error: 错误信息
func (pb *memoryProtoBook) SetProtocols(p peer.ID, protos ...protocol.ID) error {
	if len(protos) > pb.maxProtos { // 检查协议数量是否超出限制
		return errTooManyProtocols
	}

	newprotos := make(map[protocol.ID]struct{}, len(protos)) // 创建新的协议映射
	for _, proto := range protos {                           // 遍历协议列表
		newprotos[proto] = struct{}{} // 添加协议
	}

	s := pb.segments.get(p)    // 获取协议段
	s.Lock()                   // 加写锁
	s.protocols[p] = newprotos // 设置新的协议映射
	s.Unlock()                 // 解锁

	return nil
}

// AddProtocols 为对等节点添加协议
// 参数:
//   - p: 对等节点ID
//   - protos: 要添加的协议ID列表
//
// 返回:
//   - error: 错误信息
func (pb *memoryProtoBook) AddProtocols(p peer.ID, protos ...protocol.ID) error {
	s := pb.segments.get(p) // 获取协议段
	s.Lock()                // 加写锁
	defer s.Unlock()        // 延迟解锁

	protomap, ok := s.protocols[p] // 获取节点的协议映射
	if !ok {                       // 如果不存在
		protomap = make(map[protocol.ID]struct{}) // 创建新的映射
		s.protocols[p] = protomap                 // 保存映射
	}
	if len(protomap)+len(protos) > pb.maxProtos { // 检查是否超出限制
		return errTooManyProtocols
	}

	for _, proto := range protos { // 遍历要添加的协议
		protomap[proto] = struct{}{} // 添加协议
	}
	return nil
}

// GetProtocols 获取对等节点支持的所有协议
// 参数:
//   - p: 对等节点ID
//
// 返回:
//   - []protocol.ID: 协议ID列表
//   - error: 错误信息
func (pb *memoryProtoBook) GetProtocols(p peer.ID) ([]protocol.ID, error) {
	s := pb.segments.get(p) // 获取协议段
	s.RLock()               // 加读锁
	defer s.RUnlock()       // 延迟解锁

	out := make([]protocol.ID, 0, len(s.protocols[p])) // 创建结果切片
	for k := range s.protocols[p] {                    // 遍历协议映射
		out = append(out, k) // 添加协议ID
	}

	return out, nil
}

// RemoveProtocols 移除对等节点的指定协议
// 参数:
//   - p: 对等节点ID
//   - protos: 要移除的协议ID列表
//
// 返回:
//   - error: 错误信息
func (pb *memoryProtoBook) RemoveProtocols(p peer.ID, protos ...protocol.ID) error {
	s := pb.segments.get(p) // 获取协议段
	s.Lock()                // 加写锁
	defer s.Unlock()        // 延迟解锁

	protomap, ok := s.protocols[p] // 获取节点的协议映射
	if !ok {                       // 如果不存在
		return nil // 无需移除
	}

	for _, proto := range protos { // 遍历要移除的协议
		delete(protomap, proto) // 删除协议
	}
	if len(protomap) == 0 { // 如果没有剩余协议
		delete(s.protocols, p) // 删除节点的协议映射
	}
	return nil
}

// SupportsProtocols 检查对等节点支持的协议
// 参数:
//   - p: 对等节点ID
//   - protos: 要检查的协议ID列表
//
// 返回:
//   - []protocol.ID: 支持的协议ID列表
//   - error: 错误信息
func (pb *memoryProtoBook) SupportsProtocols(p peer.ID, protos ...protocol.ID) ([]protocol.ID, error) {
	s := pb.segments.get(p) // 获取协议段
	s.RLock()               // 加读锁
	defer s.RUnlock()       // 延迟解锁

	out := make([]protocol.ID, 0, len(protos)) // 创建结果切片
	for _, proto := range protos {             // 遍历要检查的协议
		if _, ok := s.protocols[p][proto]; ok { // 如果支持该协议
			out = append(out, proto) // 添加到结果中
		}
	}

	return out, nil
}

// FirstSupportedProtocol 获取对等节点支持的第一个协议
// 参数:
//   - p: 对等节点ID
//   - protos: 要检查的协议ID列表
//
// 返回:
//   - protocol.ID: 支持的第一个协议ID
//   - error: 错误信息
func (pb *memoryProtoBook) FirstSupportedProtocol(p peer.ID, protos ...protocol.ID) (protocol.ID, error) {
	s := pb.segments.get(p) // 获取协议段
	s.RLock()               // 加读锁
	defer s.RUnlock()       // 延迟解锁

	for _, proto := range protos { // 遍历要检查的协议
		if _, ok := s.protocols[p][proto]; ok { // 如果支持该协议
			return proto, nil // 返回该协议
		}
	}
	return "", nil
}

// RemovePeer 移除对等节点的所有协议信息
// 参数:
//   - p: 对等节点ID
func (pb *memoryProtoBook) RemovePeer(p peer.ID) {
	s := pb.segments.get(p) // 获取协议段
	s.Lock()                // 加写锁
	delete(s.protocols, p)  // 删除节点的所有协议
	s.Unlock()              // 解锁
}
