package noise

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/dep2p/go-dep2p/internal/core/identity"
	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// SecureConn 基础功能测试
// ============================================================================

// TestSecureConn_ReadWrite 测试基本的加密读写功能
func TestSecureConn_ReadWrite(t *testing.T) {
	clientConn, serverConn := setupSecureConnPair(t)
	defer clientConn.Close()
	defer serverConn.Close()

	testData := []byte("Hello, Noise Protocol!")

	// Client 写入数据
	n, err := clientConn.Write(testData)
	require.NoError(t, err)
	assert.Equal(t, len(testData), n)

	// Server 读取数据
	recvBuf := make([]byte, len(testData))
	n, err = serverConn.Read(recvBuf)
	require.NoError(t, err)
	assert.Equal(t, len(testData), n)
	assert.Equal(t, testData, recvBuf)

	t.Log("✅ 加密读写功能正常")
}

// TestSecureConn_ReadWrite_Bidirectional 测试双向通信
func TestSecureConn_ReadWrite_Bidirectional(t *testing.T) {
	clientConn, serverConn := setupSecureConnPair(t)
	defer clientConn.Close()
	defer serverConn.Close()

	// Client → Server
	clientMsg := []byte("From Client")
	_, err := clientConn.Write(clientMsg)
	require.NoError(t, err)

	serverRecvBuf := make([]byte, len(clientMsg))
	_, err = serverConn.Read(serverRecvBuf)
	require.NoError(t, err)
	assert.Equal(t, clientMsg, serverRecvBuf)

	// Server → Client
	serverMsg := []byte("From Server")
	_, err = serverConn.Write(serverMsg)
	require.NoError(t, err)

	clientRecvBuf := make([]byte, len(serverMsg))
	_, err = clientConn.Read(clientRecvBuf)
	require.NoError(t, err)
	assert.Equal(t, serverMsg, clientRecvBuf)

	t.Log("✅ 双向加密通信正常")
}

// TestSecureConn_ReadWrite_MultipleMessages 测试多次读写
func TestSecureConn_ReadWrite_MultipleMessages(t *testing.T) {
	clientConn, serverConn := setupSecureConnPair(t)
	defer clientConn.Close()
	defer serverConn.Close()

	messages := [][]byte{
		[]byte("Message 1"),
		[]byte("Message 2"),
		[]byte("Message 3"),
	}

	// 发送多条消息
	for i, msg := range messages {
		_, err := clientConn.Write(msg)
		require.NoError(t, err, "Failed to send message %d", i+1)
	}

	// 接收多条消息
	for i, expectedMsg := range messages {
		recvBuf := make([]byte, len(expectedMsg))
		n, err := serverConn.Read(recvBuf)
		require.NoError(t, err, "Failed to receive message %d", i+1)
		assert.Equal(t, len(expectedMsg), n)
		assert.Equal(t, expectedMsg, recvBuf)
	}

	t.Log("✅ 多次读写功能正常")
}

// ============================================================================
// 边界条件测试
// ============================================================================

// TestSecureConn_ReadWrite_EmptyData 测试空数据
func TestSecureConn_ReadWrite_EmptyData(t *testing.T) {
	clientConn, serverConn := setupSecureConnPair(t)
	defer clientConn.Close()
	defer serverConn.Close()

	// 写入空数据
	emptyData := []byte{}
	n, err := clientConn.Write(emptyData)
	require.NoError(t, err)
	assert.Equal(t, 0, n)

	// 读取空数据（应该能正常处理）
	recvBuf := make([]byte, 100)
	n, err = serverConn.Read(recvBuf)
	require.NoError(t, err)
	assert.Equal(t, 0, n)

	t.Log("✅ 空数据处理正常")
}

// TestSecureConn_ReadWrite_LargeData 测试大数据块
func TestSecureConn_ReadWrite_LargeData(t *testing.T) {
	clientConn, serverConn := setupSecureConnPair(t)
	defer clientConn.Close()
	defer serverConn.Close()

	// 创建 10KB 数据
	largeData := make([]byte, 10*1024)
	for i := range largeData {
		largeData[i] = byte(i % 256)
	}

	// 写入大数据
	_, err := clientConn.Write(largeData)
	require.NoError(t, err)

	// 读取大数据（可能需要多次读取）
	recvBuf := make([]byte, len(largeData))
	totalRead := 0
	for totalRead < len(largeData) {
		n, err := serverConn.Read(recvBuf[totalRead:])
		require.NoError(t, err)
		totalRead += n
	}

	assert.Equal(t, largeData, recvBuf)
	t.Log("✅ 大数据块传输正常")
}

// TestSecureConn_Read_BufferManagement 测试读缓冲区管理
// 当读取的数据小于消息长度时，应该缓存剩余数据
func TestSecureConn_Read_BufferManagement(t *testing.T) {
	clientConn, serverConn := setupSecureConnPair(t)
	defer clientConn.Close()
	defer serverConn.Close()

	// 发送一个较长的消息
	longMsg := []byte("This is a long message for buffer testing")
	_, err := clientConn.Write(longMsg)
	require.NoError(t, err)

	// 分多次小块读取
	readBuf := make([]byte, 10) // 小缓冲区
	totalRead := 0
	var receivedData []byte

	for totalRead < len(longMsg) {
		n, err := serverConn.Read(readBuf)
		require.NoError(t, err)
		receivedData = append(receivedData, readBuf[:n]...)
		totalRead += n
	}

	assert.Equal(t, longMsg, receivedData)
	t.Log("✅ 读缓冲区管理正常")
}

// ============================================================================
// 并发安全测试
// ============================================================================

// TestSecureConn_ConcurrentRead 测试并发读取
func TestSecureConn_ConcurrentRead(t *testing.T) {
	clientConn, serverConn := setupSecureConnPair(t)
	defer clientConn.Close()
	defer serverConn.Close()

	// 发送多条消息
	messageCount := 10
	for i := 0; i < messageCount; i++ {
		msg := []byte("Message " + string(rune('0'+i)))
		_, err := clientConn.Write(msg)
		require.NoError(t, err)
	}

	// 并发读取（测试读锁）
	var wg sync.WaitGroup
	errChan := make(chan error, messageCount)

	for i := 0; i < messageCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			buf := make([]byte, 100)
			_, err := serverConn.Read(buf)
			if err != nil {
				errChan <- err
			}
		}()
	}

	wg.Wait()
	close(errChan)

	// 检查是否有错误
	for err := range errChan {
		t.Errorf("Concurrent read error: %v", err)
	}

	t.Log("✅ 并发读取安全")
}

// TestSecureConn_ConcurrentWrite 测试并发写入
func TestSecureConn_ConcurrentWrite(t *testing.T) {
	clientConn, serverConn := setupSecureConnPair(t)
	defer clientConn.Close()
	defer serverConn.Close()

	messageCount := 10
	var wg sync.WaitGroup
	errChan := make(chan error, messageCount)

	// 并发写入（测试写锁）
	for i := 0; i < messageCount; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			msg := []byte("Concurrent message")
			_, err := clientConn.Write(msg)
			if err != nil {
				errChan <- err
			}
		}(i)
	}

	wg.Wait()
	close(errChan)

	// 检查是否有错误
	for err := range errChan {
		t.Errorf("Concurrent write error: %v", err)
	}

	// 读取所有消息（验证全部写入成功）
	for i := 0; i < messageCount; i++ {
		buf := make([]byte, 100)
		_, err := serverConn.Read(buf)
		require.NoError(t, err)
	}

	t.Log("✅ 并发写入安全")
}

// TestSecureConn_ConcurrentReadWrite 测试同时读写
func TestSecureConn_ConcurrentReadWrite(t *testing.T) {
	clientConn, serverConn := setupSecureConnPair(t)
	defer clientConn.Close()
	defer serverConn.Close()

	done := make(chan struct{})

	// Client 持续写入
	go func() {
		for i := 0; i < 20; i++ {
			msg := []byte("Message from client")
			clientConn.Write(msg)
			time.Sleep(10 * time.Millisecond)
		}
		close(done)
	}()

	// Server 持续读取并回写
	go func() {
		buf := make([]byte, 100)
		for {
			select {
			case <-done:
				return
			default:
				n, err := serverConn.Read(buf)
				if err == nil && n > 0 {
					serverConn.Write([]byte("ACK"))
				}
			}
		}
	}()

	// 等待完成
	<-done
	time.Sleep(100 * time.Millisecond)

	t.Log("✅ 并发读写安全")
}

// ============================================================================
// 状态查询测试
// ============================================================================

// TestSecureConn_LocalPeer 测试 LocalPeer() 方法
func TestSecureConn_LocalPeer(t *testing.T) {
	clientID, serverID := createTestIdentities(t)
	clientConn, serverConn := setupSecureConnPairWithIDs(t, clientID, serverID)
	defer clientConn.Close()
	defer serverConn.Close()

	// 验证 LocalPeer
	clientLocalPeer := clientConn.LocalPeer()
	assert.Equal(t, types.PeerID(clientID.PeerID()), clientLocalPeer)

	serverLocalPeer := serverConn.LocalPeer()
	assert.Equal(t, types.PeerID(serverID.PeerID()), serverLocalPeer)

	t.Log("✅ LocalPeer() 功能正常")
}

// TestSecureConn_RemotePeer 测试 RemotePeer() 方法
func TestSecureConn_RemotePeer(t *testing.T) {
	clientID, serverID := createTestIdentities(t)
	clientConn, serverConn := setupSecureConnPairWithIDs(t, clientID, serverID)
	defer clientConn.Close()
	defer serverConn.Close()

	// 验证 RemotePeer（client 的 remote 应该是 server）
	clientRemotePeer := clientConn.RemotePeer()
	assert.Equal(t, types.PeerID(serverID.PeerID()), clientRemotePeer)

	// 验证 RemotePeer（server 的 remote 应该是 client）
	serverRemotePeer := serverConn.RemotePeer()
	assert.Equal(t, types.PeerID(clientID.PeerID()), serverRemotePeer)

	t.Log("✅ RemotePeer() 功能正常")
}

// TestSecureConn_LocalPublicKey 测试 LocalPublicKey() 方法
func TestSecureConn_LocalPublicKey(t *testing.T) {
	clientConn, serverConn := setupSecureConnPair(t)
	defer clientConn.Close()
	defer serverConn.Close()

	// Noise 实现中，LocalPublicKey 返回 nil（不暴露原始公钥）
	clientPubKey := clientConn.LocalPublicKey()
	assert.Nil(t, clientPubKey, "Noise should not expose raw public key")

	serverPubKey := serverConn.LocalPublicKey()
	assert.Nil(t, serverPubKey, "Noise should not expose raw public key")

	t.Log("✅ LocalPublicKey() 行为符合预期（返回nil）")
}

// TestSecureConn_RemotePublicKey 测试 RemotePublicKey() 方法
func TestSecureConn_RemotePublicKey(t *testing.T) {
	clientConn, serverConn := setupSecureConnPair(t)
	defer clientConn.Close()
	defer serverConn.Close()

	// Noise 实现中，RemotePublicKey 返回 nil（不暴露原始公钥）
	clientRemotePubKey := clientConn.RemotePublicKey()
	assert.Nil(t, clientRemotePubKey, "Noise should not expose remote public key")

	serverRemotePubKey := serverConn.RemotePublicKey()
	assert.Nil(t, serverRemotePubKey, "Noise should not expose remote public key")

	t.Log("✅ RemotePublicKey() 行为符合预期（返回nil）")
}

// TestSecureConn_ConnState 测试 ConnState() 方法
func TestSecureConn_ConnState(t *testing.T) {
	clientID, serverID := createTestIdentities(t)
	clientConn, serverConn := setupSecureConnPairWithIDs(t, clientID, serverID)
	defer clientConn.Close()
	defer serverConn.Close()

	// 验证 Client 连接状态
	clientState := clientConn.ConnState()
	assert.Equal(t, types.ProtocolID("/noise/1.0.0"), clientState.Protocol)
	assert.Equal(t, types.PeerID(clientID.PeerID()), clientState.LocalPeer)
	assert.Equal(t, types.PeerID(serverID.PeerID()), clientState.RemotePeer)
	assert.True(t, clientState.Opened)
	assert.Nil(t, clientState.LocalPublicKey)
	assert.Nil(t, clientState.RemotePublicKey)

	// 验证 Server 连接状态
	serverState := serverConn.ConnState()
	assert.Equal(t, types.ProtocolID("/noise/1.0.0"), serverState.Protocol)
	assert.Equal(t, types.PeerID(serverID.PeerID()), serverState.LocalPeer)
	assert.Equal(t, types.PeerID(clientID.PeerID()), serverState.RemotePeer)
	assert.True(t, serverState.Opened)
	assert.Nil(t, serverState.LocalPublicKey)
	assert.Nil(t, serverState.RemotePublicKey)

	t.Log("✅ ConnState() 功能正常")
}

// ============================================================================
// 测试辅助函数
// ============================================================================

// setupSecureConnPair 创建一对已完成握手的安全连接
func setupSecureConnPair(t *testing.T) (clientConn, serverConn pkgif.SecureConn) {
	clientID, serverID := createTestIdentities(t)
	return setupSecureConnPairWithIDs(t, clientID, serverID)
}

// setupSecureConnPairWithIDs 使用指定的身份创建安全连接对
func setupSecureConnPairWithIDs(t *testing.T, clientID, serverID *identity.Identity) (clientConn, serverConn pkgif.SecureConn) {
	// 创建 Transport
	clientTransport, err := New(clientID)
	require.NoError(t, err)
	serverTransport, err := New(serverID)
	require.NoError(t, err)

	// 创建测试连接
	rawClientConn, rawServerConn := createTestPipe()

	// 获取 PeerID
	serverPeerID := types.PeerID(serverID.PeerID())
	clientPeerID := types.PeerID(clientID.PeerID())

	// 并发握手
	var clientSecConn, serverSecConn pkgif.SecureConn
	var clientErr, serverErr error
	done := make(chan struct{}, 2)

	ctx := context.Background()

	// Client 端握手
	go func() {
		clientSecConn, clientErr = clientTransport.SecureOutbound(ctx, rawClientConn, serverPeerID)
		done <- struct{}{}
	}()

	// Server 端握手
	go func() {
		serverSecConn, serverErr = serverTransport.SecureInbound(ctx, rawServerConn, clientPeerID)
		done <- struct{}{}
	}()

	// 等待握手完成
	<-done
	<-done

	require.NoError(t, clientErr)
	require.NoError(t, serverErr)
	require.NotNil(t, clientSecConn)
	require.NotNil(t, serverSecConn)

	return clientSecConn, serverSecConn
}

// createTestIdentities 创建测试用的两个身份
func createTestIdentities(t *testing.T) (clientID, serverID *identity.Identity) {
	clientID, err := identity.Generate()
	require.NoError(t, err)

	serverID, err = identity.Generate()
	require.NoError(t, err)

	return clientID, serverID
}
