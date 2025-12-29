package noise

import (
	"bytes"
	"crypto/sha256"
	"net"
	"sync"
	"testing"
	"time"

	securityif "github.com/dep2p/go-dep2p/pkg/interfaces/security"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              握手测试
// ============================================================================

func TestHandshaker_SelectCipherSuite(t *testing.T) {
	tests := []struct {
		name    string
		config  *securityif.NoiseConfig
		wantErr bool
	}{
		{
			name:    "默认配置",
			config:  securityif.DefaultNoiseConfig(),
			wantErr: false,
		},
		{
			name: "ChaCha20-Poly1305",
			config: &securityif.NoiseConfig{
				DHCurve:      "25519",
				CipherSuite:  "ChaChaPoly",
				HashFunction: "SHA256",
			},
			wantErr: false,
		},
		{
			name: "AES-GCM",
			config: &securityif.NoiseConfig{
				DHCurve:      "25519",
				CipherSuite:  "AESGCM",
				HashFunction: "SHA256",
			},
			wantErr: false,
		},
		{
			name: "BLAKE2b",
			config: &securityif.NoiseConfig{
				DHCurve:      "25519",
				CipherSuite:  "ChaChaPoly",
				HashFunction: "BLAKE2b",
			},
			wantErr: false,
		},
		{
			name: "BLAKE2s",
			config: &securityif.NoiseConfig{
				DHCurve:      "25519",
				CipherSuite:  "ChaChaPoly",
				HashFunction: "BLAKE2s",
			},
			wantErr: false,
		},
		{
			name: "无效的加密套件",
			config: &securityif.NoiseConfig{
				CipherSuite: "InvalidCipher",
			},
			wantErr: true,
		},
		{
			name: "无效的哈希函数",
			config: &securityif.NoiseConfig{
				HashFunction: "InvalidHash",
			},
			wantErr: true,
		},
	}


	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewHandshaker(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewHandshaker() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestHandshaker_XX_Handshake(t *testing.T) {
	config := securityif.DefaultNoiseConfig()
	config.HandshakePattern = "XX"

	// 创建两个握手器
	initiator, err := NewHandshaker(config)
	if err != nil {
		t.Fatalf("创建发起者握手器失败: %v", err)
	}

	responder, err := NewHandshaker(config)
	if err != nil {
		t.Fatalf("创建响应者握手器失败: %v", err)
	}

	// 创建管道模拟连接
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	var wg sync.WaitGroup
	var initiatorResult, responderResult *HandshakeResult
	var initiatorErr, responderErr error
	deadline := time.Now().Add(5 * time.Second)

	// 发起者
	wg.Add(1)
	go func() {
		defer wg.Done()
		initiatorResult, initiatorErr = initiator.HandshakeAsInitiator(client, types.EmptyNodeID, deadline)
	}()

	// 响应者
	wg.Add(1)
	go func() {
		defer wg.Done()
		responderResult, responderErr = responder.HandshakeAsResponder(server, deadline)
	}()

	wg.Wait()

	if initiatorErr != nil {
		t.Fatalf("发起者握手失败: %v", initiatorErr)
	}
	if responderErr != nil {
		t.Fatalf("响应者握手失败: %v", responderErr)
	}

	// 验证双方看到对方的 Noise 公钥
	if !bytes.Equal(initiatorResult.NoiseRemotePubKey, responder.LocalPublicKey()) {
		t.Error("发起者看到的远程公钥不正确")
	}
	if !bytes.Equal(responderResult.NoiseRemotePubKey, initiator.LocalPublicKey()) {
		t.Error("响应者看到的远程公钥不正确")
	}
}

func TestHandshaker_LocalKeys(t *testing.T) {
	config := securityif.DefaultNoiseConfig()

	h, err := NewHandshaker(config)
	if err != nil {
		t.Fatalf("创建握手器失败: %v", err)
	}

	pubKey := h.LocalPublicKey()
	privKey := h.LocalPrivateKey()

	if len(pubKey) != NoiseKeySize {
		t.Errorf("公钥长度不正确: %d", len(pubKey))
	}
	if len(privKey) != NoiseKeySize {
		t.Errorf("私钥长度不正确: %d", len(privKey))
	}
}

func TestHandshaker_CustomKeypair(t *testing.T) {

	// 使用自定义密钥对
	customKeypair := &securityif.NoiseKeypair{
		PublicKey:  make([]byte, NoiseKeySize),
		PrivateKey: make([]byte, NoiseKeySize),
	}
	// 填充一些数据
	for i := range customKeypair.PublicKey {
		customKeypair.PublicKey[i] = byte(i)
		customKeypair.PrivateKey[i] = byte(i + 100)
	}

	config := securityif.DefaultNoiseConfig()
	config.StaticKeypair = customKeypair

	h, err := NewHandshaker(config)
	if err != nil {
		t.Fatalf("创建握手器失败: %v", err)
	}

	if !bytes.Equal(h.LocalPublicKey(), customKeypair.PublicKey) {
		t.Error("自定义公钥未被使用")
	}
}

func TestHandshaker_InvalidPattern(t *testing.T) {
	config := securityif.DefaultNoiseConfig()
	config.HandshakePattern = "InvalidPattern"

	h, err := NewHandshaker(config)
	if err != nil {
		t.Fatalf("创建握手器失败: %v", err)
	}

	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	deadline := time.Now().Add(1 * time.Second)
	_, err = h.HandshakeAsInitiator(client, types.EmptyNodeID, deadline)
	if err == nil {
		t.Error("应该返回无效握手模式错误")
	}
}

func TestHandshaker_RemoteIdentityMismatch(t *testing.T) {
	config := securityif.DefaultNoiseConfig()

	initiator, _ := NewHandshaker(config)
	responder, _ := NewHandshaker(config)

	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	// 创建一个假的期望 NodeID
	var expectedID types.NodeID
	for i := range expectedID {
		expectedID[i] = 0xFF // 填充不可能匹配的值
	}

	var wg sync.WaitGroup
	var initiatorErr error
	deadline := time.Now().Add(5 * time.Second)

	wg.Add(1)
	go func() {
		defer wg.Done()
		_, initiatorErr = initiator.HandshakeAsInitiator(client, expectedID, deadline)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		responder.HandshakeAsResponder(server, deadline)
	}()

	wg.Wait()

	if initiatorErr == nil {
		t.Error("应该返回身份不匹配错误")
	}
}

// ============================================================================
//                              配置测试
// ============================================================================

func TestDefaultNoiseConfig(t *testing.T) {
	config := securityif.DefaultNoiseConfig()

	if config.HandshakePattern != "XX" {
		t.Errorf("默认握手模式不正确: %s", config.HandshakePattern)
	}
	if config.DHCurve != "25519" {
		t.Errorf("默认 DH 曲线不正确: %s", config.DHCurve)
	}
	if config.CipherSuite != "ChaChaPoly" {
		t.Errorf("默认加密套件不正确: %s", config.CipherSuite)
	}
	if config.HashFunction != "SHA256" {
		t.Errorf("默认哈希函数不正确: %s", config.HashFunction)
	}
	if config.MaxMessageSize != 65535 {
		t.Errorf("默认最大消息大小不正确: %d", config.MaxMessageSize)
	}
}

func TestDefaultConfig_IncludesNoise(t *testing.T) {
	config := securityif.DefaultConfig()

	if config.NoiseConfig == nil {
		t.Error("默认配置应该包含 NoiseConfig")
	}
}

// ============================================================================
//                              工具函数测试
// ============================================================================

func TestNodeIDFromNoiseKey(t *testing.T) {
	key := make([]byte, NoiseKeySize)
	for i := range key {
		key[i] = byte(i)
	}

	nodeID := NodeIDFromNoiseKey(key)

	// 验证 NodeID 是公钥的 SHA-256 哈希
	// 计算预期的哈希值
	expectedHash := sha256.Sum256(key)

	for i := 0; i < 32; i++ {
		if nodeID[i] != expectedHash[i] {
			t.Errorf("NodeID[%d] = %x, want %x", i, nodeID[i], expectedHash[i])
		}
	}
}

func TestNodeIDFromNoiseKey_ConsistentWithIdentity(t *testing.T) {
	// 验证 Noise NodeID 派生与 identity 模块一致
	key := make([]byte, NoiseKeySize)
	for i := range key {
		key[i] = byte(i * 2)
	}

	nodeID1 := NodeIDFromNoiseKey(key)
	nodeID2 := NodeIDFromNoiseKey(key)

	// 相同输入应产生相同输出
	if !nodeID1.Equal(nodeID2) {
		t.Error("相同公钥应产生相同 NodeID")
	}

	// 不同输入应产生不同输出
	key2 := make([]byte, NoiseKeySize)
	for i := range key2 {
		key2[i] = byte(i * 3)
	}
	nodeID3 := NodeIDFromNoiseKey(key2)

	if nodeID1.Equal(nodeID3) {
		t.Error("不同公钥应产生不同 NodeID")
	}
}

func TestNodeIDFromNoiseKey_InvalidLength(t *testing.T) {
	key := make([]byte, 16) // 太短

	nodeID := NodeIDFromNoiseKey(key)

	if !nodeID.IsEmpty() {
		t.Error("无效长度的密钥应该返回空 NodeID")
	}
}

func TestNoiseKeyFromNodeID_Deprecated(t *testing.T) {
	var nodeID types.NodeID
	for i := range nodeID {
		nodeID[i] = byte(i)
	}

	// NoiseKeyFromNodeID 现在返回 nil，因为 NodeID 是哈希值，无法逆向恢复公钥
	key := NoiseKeyFromNodeID(nodeID)

	if key != nil {
		t.Error("NoiseKeyFromNodeID 应返回 nil，因为无法从哈希恢复公钥")
	}
}

func TestNoiseKeyFromNodeID_Legacy(t *testing.T) {
	// 这个测试验证旧行为已被废弃
	// NoiseKeyFromNodeID 现在返回 nil
	var nodeID types.NodeID
	for i := range nodeID {
		nodeID[i] = byte(i)
	}

	key := NoiseKeyFromNodeID(nodeID)

	// 新行为: 返回 nil
	if key != nil {
		t.Error("NoiseKeyFromNodeID 应返回 nil (已废弃)")
	}
}

// ============================================================================
//                              消息协议测试
// ============================================================================

func TestHandshaker_WriteReadMessage(t *testing.T) {
	config := securityif.DefaultNoiseConfig()

	h, _ := NewHandshaker(config)

	// 使用管道测试消息读写
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	testMsg := []byte("test handshake message")
	deadline := time.Now().Add(1 * time.Second)

	var wg sync.WaitGroup
	var writeErr, readErr error
	var receivedMsg []byte

	wg.Add(1)
	go func() {
		defer wg.Done()
		writeErr = h.writeMessage(client, testMsg, deadline)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		receivedMsg, readErr = h.readMessage(server, deadline)
	}()

	wg.Wait()

	if writeErr != nil {
		t.Errorf("写消息失败: %v", writeErr)
	}
	if readErr != nil {
		t.Errorf("读消息失败: %v", readErr)
	}
	if !bytes.Equal(receivedMsg, testMsg) {
		t.Errorf("消息不匹配: got %v, want %v", receivedMsg, testMsg)
	}
}

func TestHandshaker_MessageTooLarge(t *testing.T) {
	config := securityif.DefaultNoiseConfig()

	h, _ := NewHandshaker(config)

	// 创建超大消息
	largeMsg := make([]byte, MaxHandshakeMessageSize+1)

	client, _ := net.Pipe()
	defer client.Close()

	deadline := time.Now().Add(1 * time.Second)
	err := h.writeMessage(client, largeMsg, deadline)

	if err != ErrMessageTooLarge {
		t.Errorf("应该返回 ErrMessageTooLarge, 得到: %v", err)
	}
}

// ============================================================================
//                              加密通信测试（使用握手结果）
// ============================================================================

func TestEncryptedCommunication(t *testing.T) {
	config := securityif.DefaultNoiseConfig()

	initiator, _ := NewHandshaker(config)
	responder, _ := NewHandshaker(config)

	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	var wg sync.WaitGroup
	var clientResult, serverResult *HandshakeResult
	deadline := time.Now().Add(5 * time.Second)

	wg.Add(2)
	go func() {
		defer wg.Done()
		clientResult, _ = initiator.HandshakeAsInitiator(client, types.EmptyNodeID, deadline)
	}()
	go func() {
		defer wg.Done()
		serverResult, _ = responder.HandshakeAsResponder(server, deadline)
	}()
	wg.Wait()

	if clientResult == nil || serverResult == nil {
		t.Fatal("握手失败")
	}

	// 验证加密器已建立
	if clientResult.SendCipher == nil || clientResult.RecvCipher == nil {
		t.Error("客户端加密器未建立")
	}
	if serverResult.SendCipher == nil || serverResult.RecvCipher == nil {
		t.Error("服务端加密器未建立")
	}

	// 测试加密通信
	testData := []byte("Hello, encrypted world!")

	// 使用客户端发送加密器加密
	ciphertext, err := clientResult.SendCipher.Encrypt(nil, nil, testData)
	if err != nil {
		t.Fatalf("加密失败: %v", err)
	}

	// 使用服务端接收解密器解密
	plaintext, err := serverResult.RecvCipher.Decrypt(nil, nil, ciphertext)
	if err != nil {
		t.Fatalf("解密失败: %v", err)
	}

	if !bytes.Equal(plaintext, testData) {
		t.Errorf("解密数据不匹配: got %s, want %s", plaintext, testData)
	}
}

// ============================================================================
//                              零长度消息测试
// ============================================================================

func TestHandshaker_ReadMessage_ZeroLength(t *testing.T) {
	config := securityif.DefaultNoiseConfig()

	h, err := NewHandshaker(config)
	if err != nil {
		t.Fatalf("创建握手器失败: %v", err)
	}

	// 创建一个发送零长度消息的连接
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	deadline := time.Now().Add(1 * time.Second)

	// 发送零长度消息
	go func() {
		var lenBuf [2]byte
		// 长度前缀为 0
		client.Write(lenBuf[:])
	}()

	// 读取应该返回错误
	_, err = h.readMessage(server, deadline)
	if err == nil {
		t.Error("读取零长度消息应返回错误")
	}
}

// ============================================================================
//                              Identity 绑定测试
// ============================================================================

// 注意：noisePublicKey 类型已移除，因为 Noise 连接现在使用 identity 公钥。
// 相关测试在 identity_binding_test.go 中。

// ============================================================================
//                              SecureConn 测试
// ============================================================================

func TestSecureConn_MinCiphertextSize(t *testing.T) {
	// 验证最小密文大小常量
	if MinCiphertextSize != 16 {
		t.Errorf("MinCiphertextSize 应为 16 (AEAD tag)，实际为: %d", MinCiphertextSize)
	}
}
