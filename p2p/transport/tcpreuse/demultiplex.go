package tcpreuse

import (
	"errors"
	"fmt"
	"time"

	manet "github.com/dep2p/go-dep2p/multiformats/multiaddr/net"
	"github.com/dep2p/go-dep2p/p2p/transport/tcpreuse/internal/sampledconn"
)

// 这是在握手后读取第一个数据包的前3个字节。
// 在 TCP Transport 中设置为默认的 TCP 连接超时时间。
//
// 定义为变量以便在测试中修改。
var identifyConnTimeout = 5 * time.Second

// DemultiplexedConnType 表示多路复用连接的类型
type DemultiplexedConnType int

const (
	// DemultiplexedConnType_Unknown 未知连接类型
	DemultiplexedConnType_Unknown DemultiplexedConnType = iota
	// DemultiplexedConnType_MultistreamSelect 多流选择协议连接
	DemultiplexedConnType_MultistreamSelect
	// DemultiplexedConnType_HTTP HTTP协议连接
	DemultiplexedConnType_HTTP
	// DemultiplexedConnType_TLS TLS协议连接
	DemultiplexedConnType_TLS
)

// String 返回连接类型的字符串表示
// 返回值:
//   - string 连接类型的描述字符串
func (t DemultiplexedConnType) String() string {
	switch t {
	case DemultiplexedConnType_MultistreamSelect:
		return "MultistreamSelect"
	case DemultiplexedConnType_HTTP:
		return "HTTP"
	case DemultiplexedConnType_TLS:
		return "TLS"
	default:
		return fmt.Sprintf("Unknown(%d)", int(t))
	}
}

// IsKnown 检查连接类型是否为已知类型
// 返回值:
//   - bool 如果是已知类型返回true，否则返回false
func (t DemultiplexedConnType) IsKnown() bool {
	return t >= 1 || t <= 3
}

// identifyConnType 通过查看前几个字节来识别连接类型
// 参数:
//   - c: manet.Conn 需要识别类型的网络连接
//
// 返回值:
//   - DemultiplexedConnType 识别出的连接类型
//   - manet.Conn 处理后的网络连接
//   - error 可能发生的错误
//
// 注意:
//   - 调用者在此函数返回后不能使用传入的连接
//   - 如果返回错误，连接将被关闭
func identifyConnType(c manet.Conn) (DemultiplexedConnType, manet.Conn, error) {
	// 设置读取超时时间
	if err := c.SetReadDeadline(time.Now().Add(identifyConnTimeout)); err != nil {
		log.Debugf("设置读取超时时间时出错: %s", err)
		closeErr := c.Close()
		return 0, nil, errors.Join(err, closeErr)
	}

	// 读取前几个字节用于识别
	s, peekedConn, err := sampledconn.PeekBytes(c)
	if err != nil {
		log.Debugf("读取前几个字节用于识别时出错: %s", err)
		closeErr := c.Close()
		return 0, nil, errors.Join(err, closeErr)
	}

	// 清除读取超时时间
	if err := peekedConn.SetReadDeadline(time.Time{}); err != nil {
		log.Debugf("清除读取超时时间时出错: %s", err)
		closeErr := peekedConn.Close()
		return 0, nil, errors.Join(err, closeErr)
	}

	// 依次判断各种协议类型
	if IsMultistreamSelect(s) {
		return DemultiplexedConnType_MultistreamSelect, peekedConn, nil
	}
	if IsTLS(s) {
		return DemultiplexedConnType_TLS, peekedConn, nil
	}
	if IsHTTP(s) {
		return DemultiplexedConnType_HTTP, peekedConn, nil
	}
	return DemultiplexedConnType_Unknown, peekedConn, nil
}

// Prefix 定义前缀类型为3字节数组
// 匹配器在这里实现而不是在传输中实现，这样我们可以更容易地一起模糊测试它们
type Prefix = [3]byte

// IsMultistreamSelect 检查是否为多流选择协议
// 参数:
//   - s: Prefix 需要检查的3字节前缀
//
// 返回值:
//   - bool 如果是多流选择协议返回true，否则返回false
func IsMultistreamSelect(s Prefix) bool {
	return string(s[:]) == "\x13/m"
}

// IsHTTP 检查是否为HTTP协议
// 参数:
//   - s: Prefix 需要检查的3字节前缀
//
// 返回值:
//   - bool 如果是HTTP协议返回true，否则返回false
func IsHTTP(s Prefix) bool {
	switch string(s[:]) {
	case "GET", "HEA", "POS", "PUT", "DEL", "CON", "OPT", "TRA", "PAT":
		return true
	default:
		return false
	}
}

// IsTLS 检查是否为TLS协议
// 参数:
//   - s: Prefix 需要检查的3字节前缀
//
// 返回值:
//   - bool 如果是TLS协议返回true，否则返回false
func IsTLS(s Prefix) bool {
	switch string(s[:]) {
	case "\x16\x03\x01", "\x16\x03\x02", "\x16\x03\x03":
		return true
	default:
		return false
	}
}
