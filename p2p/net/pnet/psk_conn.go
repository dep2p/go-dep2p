package pnet

import (
	"crypto/cipher"
	"crypto/rand"
	"fmt"
	"io"
	"net"

	"github.com/dep2p/go-dep2p/core/pnet"

	"github.com/davidlazar/go-crypto/salsa20"
	pool "github.com/dep2p/go-dep2p/p2plib/buffer/pool"
)

// 使用缓冲池因为用户需要他们的切片返回
// 所以我们不能在原地进行 XOR 加密
var (
	errShortNonce  = pnet.NewError("无法读取完整的随机数")
	errInsecureNil = pnet.NewError("不安全连接为空")
	errPSKNil      = pnet.NewError("预共享密钥为空")
)

// pskConn 实现了使用预共享密钥加密的网络连接
type pskConn struct {
	net.Conn           // 底层网络连接
	psk      *[32]byte // 预共享密钥

	writeS20 cipher.Stream // 写入流加密器
	readS20  cipher.Stream // 读取流加密器
}

// Read 从连接中读取加密数据并解密
// 参数:
//   - out: []byte 用于存储解密后数据的缓冲区
//
// 返回值:
//   - int: 读取的字节数
//   - error: 错误信息
func (c *pskConn) Read(out []byte) (int, error) {
	// 如果读取流加密器未初始化
	if c.readS20 == nil {
		// 创建随机数
		nonce := make([]byte, 24)
		// 从连接中读取随机数
		_, err := io.ReadFull(c.Conn, nonce)
		if err != nil {
			log.Debugf("读取随机数失败: %v", err)
			return 0, fmt.Errorf("%w: %w", errShortNonce, err)
		}
		// 使用随机数初始化读取流加密器
		c.readS20 = salsa20.New(c.psk, nonce)
	}

	// 从连接中读取加密数据
	n, err := c.Conn.Read(out)
	if err != nil {
		log.Debugf("读取数据失败: %v", err)
		return 0, err
	}
	if n > 0 {
		// 解密数据到输出缓冲区
		c.readS20.XORKeyStream(out[:n], out[:n])
	}
	return n, nil
}

// Write 加密数据并写入连接
// 参数:
//   - in: []byte 要加密和发送的数据
//
// 返回值:
//   - int: 写入的字节数
//   - error: 错误信息
func (c *pskConn) Write(in []byte) (int, error) {
	// 如果写入流加密器未初始化
	if c.writeS20 == nil {
		// 生成随机数
		nonce := make([]byte, 24)
		_, err := rand.Read(nonce)
		if err != nil {
			log.Debugf("生成随机数失败: %v", err)
			return 0, err
		}
		// 发送随机数给对端
		_, err = c.Conn.Write(nonce)
		if err != nil {
			log.Debugf("发送随机数失败: %v", err)
			return 0, err
		}

		// 使用随机数初始化写入流加密器
		c.writeS20 = salsa20.New(c.psk, nonce)
	}
	// 从缓冲池获取输出缓冲区
	out := pool.Get(len(in))
	defer pool.Put(out)

	// 加密数据
	c.writeS20.XORKeyStream(out, in)

	// 发送加密数据
	return c.Conn.Write(out)
}

// 确保 pskConn 实现了 net.Conn 接口
var _ net.Conn = (*pskConn)(nil)

// newPSKConn 创建一个新的预共享密钥加密连接
// 参数:
//   - psk: *[32]byte 预共享密钥
//   - insecure: net.Conn 不安全的底层连接
//
// 返回值:
//   - net.Conn: 加密的网络连接
//   - error: 错误信息
func newPSKConn(psk *[32]byte, insecure net.Conn) (net.Conn, error) {
	if insecure == nil {
		log.Debugf("不安全的底层连接为空")
		return nil, errInsecureNil
	}
	if psk == nil {
		log.Debugf("预共享密钥为空")
		return nil, errPSKNil
	}
	return &pskConn{
		Conn: insecure,
		psk:  psk,
	}, nil
}
