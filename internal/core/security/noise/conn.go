// Package noise 实现 Noise 协议安全传输
package noise

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"sync"

	"github.com/flynn/noise"

	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
// Secure Connection 实现
// ============================================================================

// secureConn Noise 安全连接
type secureConn struct {
	net.Conn
	
	// Noise cipher states
	sendCS *noise.CipherState
	recvCS *noise.CipherState
	
	// 节点信息
	localPeer  types.PeerID
	remotePeer types.PeerID
	
	// 读写锁
	readMu  sync.Mutex
	writeMu sync.Mutex
	
	// 缓冲区
	readBuf []byte
}

// 确保实现接口
var _ pkgif.SecureConn = (*secureConn)(nil)

// ============================================================================
// 接口实现
// ============================================================================

// Read 从连接读取数据（解密）
func (c *secureConn) Read(p []byte) (int, error) {
	c.readMu.Lock()
	defer c.readMu.Unlock()
	
	// 如果缓冲区有数据，先返回缓冲区的数据
	if len(c.readBuf) > 0 {
		n := copy(p, c.readBuf)
		c.readBuf = c.readBuf[n:]
		return n, nil
	}
	
	// 读取加密消息长度（2 字节）
	lenBuf := make([]byte, 2)
	_, err := io.ReadFull(c.Conn, lenBuf)
	if err != nil {
		return 0, err
	}
	
	msgLen := binary.BigEndian.Uint16(lenBuf)
	if msgLen == 0 {
		return 0, io.EOF
	}
	
	// 读取加密消息
	encMsg := make([]byte, msgLen)
	_, err = io.ReadFull(c.Conn, encMsg)
	if err != nil {
		return 0, err
	}
	
	// 解密消息
	plaintext, err := c.recvCS.Decrypt(nil, nil, encMsg)
	if err != nil {
		return 0, fmt.Errorf("decrypt: %w", err)
	}
	
	// 复制到输出缓冲区
	n := copy(p, plaintext)
	
	// 如果还有剩余数据，保存到缓冲区
	if n < len(plaintext) {
		c.readBuf = make([]byte, len(plaintext)-n)
		copy(c.readBuf, plaintext[n:])
	}
	
	return n, nil
}

// Write 向连接写入数据（加密）
func (c *secureConn) Write(p []byte) (int, error) {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	
	// 加密数据
	ciphertext, err := c.sendCS.Encrypt(nil, nil, p)
	if err != nil {
		return 0, fmt.Errorf("encrypt: %w", err)
	}
	
	// 写入长度（2 字节）
	lenBuf := make([]byte, 2)
	binary.BigEndian.PutUint16(lenBuf, uint16(len(ciphertext)))
	
	_, err = c.Conn.Write(lenBuf)
	if err != nil {
		return 0, err
	}
	
	// 写入加密消息
	_, err = c.Conn.Write(ciphertext)
	if err != nil {
		return 0, err
	}
	
	return len(p), nil
}

// LocalPeer 返回本地节点 ID
func (c *secureConn) LocalPeer() types.PeerID {
	return c.localPeer
}

// RemotePeer 返回远端节点 ID
func (c *secureConn) RemotePeer() types.PeerID {
	return c.remotePeer
}

// LocalPublicKey 返回本地公钥
func (c *secureConn) LocalPublicKey() []byte {
	// Noise 不直接暴露公钥，返回空
	return nil
}

// RemotePublicKey 返回远端公钥
func (c *secureConn) RemotePublicKey() []byte {
	// Noise 不直接暴露公钥，返回空
	return nil
}

// ConnState 返回连接状态
func (c *secureConn) ConnState() pkgif.SecureConnState {
	return pkgif.SecureConnState{
		Protocol:        types.ProtocolID("/noise/1.0.0"),
		LocalPeer:       c.localPeer,
		RemotePeer:      c.remotePeer,
		LocalPublicKey:  nil,
		RemotePublicKey: nil,
		Opened:          true,
	}
}
