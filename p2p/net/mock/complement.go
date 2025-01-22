package mocknet

import (
	"github.com/dep2p/go-dep2p/core/network"
)

// StreamComplement 返回给定流的另一端
// 参数:
//   - s: network.Stream 网络流对象
//
// 返回值:
//   - network.Stream 对端的网络流对象
//
// 注意:
//   - 当传入非 mocknet 流时会发生 panic
func StreamComplement(s network.Stream) network.Stream {
	// 将传入的流对象转换为内部 stream 类型并返回其对端流
	return s.(*stream).rstream
}

// ConnComplement 返回给定连接的另一端
// 参数:
//   - c: network.Conn 网络连接对象
//
// 返回值:
//   - network.Conn 对端的网络连接对象
//
// 注意:
//   - 当传入非 mocknet 连接时会发生 panic
func ConnComplement(c network.Conn) network.Conn {
	// 将传入的连接对象转换为内部 conn 类型并返回其对端连接
	return c.(*conn).rconn
}
