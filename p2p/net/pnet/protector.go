package pnet

import (
	"errors"
	"net"

	ipnet "github.com/dep2p/go-dep2p/core/pnet"
	logging "github.com/dep2p/log"
)

var log = logging.Logger("net-pnet")

// NewProtectedConn 创建一个新的受保护连接
// 参数:
//   - psk: ipnet.PSK 预共享密钥
//   - conn: net.Conn 原始网络连接
//
// 返回值:
//   - net.Conn: 受保护的网络连接
//   - error: 错误信息
func NewProtectedConn(psk ipnet.PSK, conn net.Conn) (net.Conn, error) {
	// 检查 PSK 长度是否为 32 字节
	if len(psk) != 32 {
		log.Debugf("预期 PSK 长度为 32 字节")
		return nil, errors.New("预期 PSK 长度为 32 字节")
	}

	// 将 PSK 复制到固定大小的数组中
	var p [32]byte
	copy(p[:], psk)

	// 使用 PSK 创建新的受保护连接
	return newPSKConn(&p, conn)
}
