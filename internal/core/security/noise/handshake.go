// Package noise 实现 Noise 协议安全传输
//
// 本实现遵循 libp2p-noise 规范：
// https://github.com/libp2p/specs/blob/master/noise/README.md
//
// Noise XX 握手流程：
//   -> e                                      (发起者发送临时公钥)
//   <- e, ee, s, es, payload                  (响应者发送临时公钥、静态公钥、payload)
//   -> s, se, payload                         (发起者发送静态公钥、payload)
//
// payload 包含：
//   - identity_key: Ed25519 身份公钥（protobuf 序列化）
//   - identity_sig: Sign("noise-libp2p-static-key:" + curve25519_static_pubkey)
package noise

import (
	"crypto/ed25519"
	"crypto/sha512"
	"encoding/binary"
	"fmt"
	"io"
	"net"

	"filippo.io/edwards25519"
	"github.com/flynn/noise"
	noisepb "github.com/dep2p/go-dep2p/pkg/lib/proto/noise"
	"github.com/dep2p/go-dep2p/pkg/lib/crypto"
	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// payloadSigPrefix 是签名 payload 的前缀
// 与 libp2p-noise 规范兼容
const payloadSigPrefix = "noise-libp2p-static-key:"

// ============================================================================
// Noise XX 握手实现
// ============================================================================

// performHandshake 执行 Noise XX 握手
//
// Noise XX 模式提供相互认证和前向保密。
// 通过在 payload 中包含 Ed25519 公钥和签名来实现身份验证。
//
// 参数：
//   - conn: 底层网络连接
//   - privKey: 本地私钥（Ed25519）
//   - remotePeer: 期望的远程 PeerID（用于验证，可为空）
//   - isInitiator: true = 客户端，false = 服务器
//
// 返回：
//   - *secureConn: 加密连接
//   - error: 握手失败时的错误
func performHandshake(conn net.Conn, privKey pkgif.PrivateKey, remotePeer types.PeerID, isInitiator bool) (*secureConn, error) {
	// 1. 密钥转换：Ed25519 -> Curve25519
	privKeyBytes, err := privKey.Raw()
	if err != nil {
		return nil, fmt.Errorf("get private key bytes: %w", err)
	}

	pubKeyBytes, err := privKey.PublicKey().Raw()
	if err != nil {
		return nil, fmt.Errorf("get public key bytes: %w", err)
	}

	// 转换为 Curve25519 密钥对（用于 DH 操作）
	curve25519Priv := ed25519ToCurve25519Private(privKeyBytes)
	curve25519Pub := ed25519ToCurve25519Public(pubKeyBytes)

	// 2. 创建 Noise 配置
	cs := noise.NewCipherSuite(noise.DH25519, noise.CipherChaChaPoly, noise.HashSHA256)
	staticKeypair := noise.DHKey{Private: curve25519Priv, Public: curve25519Pub}

	hs, err := noise.NewHandshakeState(noise.Config{
		CipherSuite:   cs,
		Pattern:       noise.HandshakeXX,
		Initiator:     isInitiator,
		StaticKeypair: staticKeypair,
	})
	if err != nil {
		return nil, fmt.Errorf("create handshake state: %w", err)
	}

	// 3. 生成本地 payload（包含 Ed25519 公钥和签名）
	localPayload, err := generateHandshakePayload(privKey, curve25519Pub)
	if err != nil {
		return nil, fmt.Errorf("generate handshake payload: %w", err)
	}

	// 4. 执行握手
	var sendCS, recvCS *noise.CipherState
	var remotePayload []byte

	if isInitiator {
		sendCS, recvCS, remotePayload, err = clientHandshake(conn, hs, localPayload)
	} else {
		sendCS, recvCS, remotePayload, err = serverHandshake(conn, hs, localPayload)
	}
	if err != nil {
		return nil, fmt.Errorf("handshake: %w", err)
	}

	// 5. 验证远程 payload 并提取 PeerID
	remotePubKey := hs.PeerStatic()
	if len(remotePubKey) != 32 {
		return nil, fmt.Errorf("invalid remote static key length: %d", len(remotePubKey))
	}

	actualRemotePeer, err := handleRemotePayload(remotePayload, remotePubKey)
	if err != nil {
		return nil, fmt.Errorf("handle remote payload: %w", err)
	}

	// 验证 PeerID（如果指定了期望的 PeerID）
	if remotePeer != "" && actualRemotePeer != remotePeer {
		return nil, fmt.Errorf("peer id mismatch: expected %s, got %s", remotePeer, actualRemotePeer)
	}

	// 6. 派生本地 PeerID
	localPeer, err := derivePeerIDFromEd25519(pubKeyBytes)
	if err != nil {
		return nil, fmt.Errorf("derive local peer id: %w", err)
	}

	// 7. 创建安全连接
	return &secureConn{
		Conn:       conn,
		sendCS:     sendCS,
		recvCS:     recvCS,
		localPeer:  localPeer,
		remotePeer: actualRemotePeer,
		readBuf:    nil,
	}, nil
}

// generateHandshakePayload 生成握手 payload
//
// payload 包含：
//   - identity_key: Ed25519 公钥的序列化
//   - identity_sig: Sign("noise-libp2p-static-key:" + curve25519_static_pubkey)
func generateHandshakePayload(privKey pkgif.PrivateKey, curve25519Pub []byte) ([]byte, error) {
	// 获取 Ed25519 公钥的原始字节
	pubKeyRaw, err := privKey.PublicKey().Raw()
	if err != nil {
		return nil, fmt.Errorf("get public key bytes: %w", err)
	}

	// 转换为 crypto.PublicKey 类型以便序列化
	cryptoPubKey, err := crypto.UnmarshalEd25519PublicKey(pubKeyRaw)
	if err != nil {
		return nil, fmt.Errorf("unmarshal ed25519 public key: %w", err)
	}

	// 序列化公钥
	pubKeyBytes, err := crypto.MarshalPublicKey(cryptoPubKey)
	if err != nil {
		return nil, fmt.Errorf("marshal public key: %w", err)
	}

	// 签名：Sign("noise-libp2p-static-key:" + curve25519_static_pubkey)
	toSign := append([]byte(payloadSigPrefix), curve25519Pub...)
	signature, err := privKey.Sign(toSign)
	if err != nil {
		return nil, fmt.Errorf("sign payload: %w", err)
	}

	// 创建 payload
	payload := &noisepb.NoiseHandshakePayload{
		IdentityKey: pubKeyBytes,
		IdentitySig: signature,
	}

	return payload.Marshal()
}

// handleRemotePayload 处理远程 payload
//
// 验证签名并提取 PeerID
func handleRemotePayload(payloadBytes []byte, remoteStatic []byte) (types.PeerID, error) {
	// 解析 payload
	payload := &noisepb.NoiseHandshakePayload{}
	if err := payload.Unmarshal(payloadBytes); err != nil {
		return "", fmt.Errorf("unmarshal payload: %w", err)
	}

	// 解析远程 Ed25519 公钥（使用序列化格式）
	remotePubKey, err := crypto.UnmarshalPublicKeyBytes(payload.IdentityKey)
	if err != nil {
		return "", fmt.Errorf("unmarshal remote public key: %w", err)
	}

	// 验证签名：Verify("noise-libp2p-static-key:" + curve25519_static_pubkey, signature)
	toVerify := append([]byte(payloadSigPrefix), remoteStatic...)
	valid, err := remotePubKey.Verify(toVerify, payload.IdentitySig)
	if err != nil {
		return "", fmt.Errorf("verify signature: %w", err)
	}
	if !valid {
		return "", fmt.Errorf("invalid signature: remote static key not bound to identity key")
	}

	// 从 Ed25519 公钥派生 PeerID
	peerID, err := crypto.PeerIDFromPublicKey(remotePubKey)
	if err != nil {
		return "", fmt.Errorf("derive peer id: %w", err)
	}

	return peerID, nil
}

// ============================================================================
// 握手流程
// ============================================================================

// clientHandshake 客户端握手（发起者）
//
// Noise XX 客户端流程：
//  1. -> e                              (发送临时公钥)
//  2. <- e, ee, s, es, payload          (接收响应者的静态公钥和 payload)
//  3. -> s, se, payload                 (发送本地静态公钥和 payload)
func clientHandshake(conn net.Conn, hs *noise.HandshakeState, localPayload []byte) (*noise.CipherState, *noise.CipherState, []byte, error) {
	// 轮次 1: 发送 e (空 payload)
	msg1, _, _, err := hs.WriteMessage(nil, nil)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("write message 1: %w", err)
	}
	if err := writeFrame(conn, msg1); err != nil {
		return nil, nil, nil, fmt.Errorf("send message 1: %w", err)
	}

	// 轮次 2: 接收 e, ee, s, es, payload
	msg2, err := readFrame(conn)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("receive message 2: %w", err)
	}
	remotePayload, _, _, err := hs.ReadMessage(nil, msg2)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("read message 2: %w", err)
	}

	// 轮次 3: 发送 s, se, payload (最后一轮，返回 CipherStates)
	msg3, cs1, cs2, err := hs.WriteMessage(nil, localPayload)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("write message 3: %w", err)
	}
	if err := writeFrame(conn, msg3); err != nil {
		return nil, nil, nil, fmt.Errorf("send message 3: %w", err)
	}

	// cs1 = 发送密钥，cs2 = 接收密钥（对于发起者）
	return cs1, cs2, remotePayload, nil
}

// serverHandshake 服务器握手（响应者）
//
// Noise XX 服务器流程：
//  1. <- e                              (接收临时公钥)
//  2. -> e, ee, s, es, payload          (发送本地静态公钥和 payload)
//  3. <- s, se, payload                 (接收发起者的静态公钥和 payload)
func serverHandshake(conn net.Conn, hs *noise.HandshakeState, localPayload []byte) (*noise.CipherState, *noise.CipherState, []byte, error) {
	// 轮次 1: 接收 e
	msg1, err := readFrame(conn)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("receive message 1: %w", err)
	}
	_, _, _, err = hs.ReadMessage(nil, msg1)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("read message 1: %w", err)
	}

	// 轮次 2: 发送 e, ee, s, es, payload
	msg2, _, _, err := hs.WriteMessage(nil, localPayload)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("write message 2: %w", err)
	}
	if err := writeFrame(conn, msg2); err != nil {
		return nil, nil, nil, fmt.Errorf("send message 2: %w", err)
	}

	// 轮次 3: 接收 s, se, payload (最后一轮，返回 CipherStates)
	msg3, err := readFrame(conn)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("receive message 3: %w", err)
	}
	remotePayload, cs1, cs2, err := hs.ReadMessage(nil, msg3)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("read message 3: %w", err)
	}

	// cs1 = 接收密钥，cs2 = 发送密钥（对于响应者，与发起者相反）
	return cs2, cs1, remotePayload, nil
}

// ============================================================================
// 密钥转换（标准实现）
// ============================================================================

// ed25519ToCurve25519Private 将 Ed25519 私钥转换为 Curve25519 私钥
//
// 标准转换方法（RFC 7748, RFC 8032）：
//  1. 对私钥种子进行 SHA-512 哈希
//  2. 取哈希前 32 字节
//  3. 进行 "clamping"（清理低 3 位和高 2 位）
func ed25519ToCurve25519Private(edPriv []byte) []byte {
	var seed []byte

	switch len(edPriv) {
	case ed25519.PrivateKeySize: // 64 字节：标准私钥格式
		seed = edPriv[:32]
	case 32: // 32 字节：种子格式
		seed = edPriv
	default:
		return make([]byte, 32)
	}

	// SHA-512 哈希种子
	h := sha512.Sum512(seed)

	// Clamping（RFC 7748）
	h[0] &= 248  // 清除低 3 位
	h[31] &= 127 // 清除最高位
	h[31] |= 64  // 设置次高位

	return h[:32]
}

// ed25519ToCurve25519Public 将 Ed25519 公钥转换为 Curve25519 公钥
//
// 使用 Edwards -> Montgomery 转换公式：
//   u = (1 + y) / (1 - y)  (mod p)
func ed25519ToCurve25519Public(edPub []byte) []byte {
	if len(edPub) != ed25519.PublicKeySize {
		return make([]byte, 32)
	}

	// 使用 filippo.io/edwards25519 进行标准转换
	point, err := new(edwards25519.Point).SetBytes(edPub)
	if err != nil {
		return make([]byte, 32)
	}

	// 转换为 Montgomery 形式（Curve25519）
	return point.BytesMontgomery()
}

// ============================================================================
// 辅助函数
// ============================================================================

// writeFrame 写入帧（2 字节长度 + 数据）
func writeFrame(w io.Writer, data []byte) error {
	lenBuf := make([]byte, 2)
	binary.BigEndian.PutUint16(lenBuf, uint16(len(data)))

	if _, err := w.Write(lenBuf); err != nil {
		return err
	}

	if _, err := w.Write(data); err != nil {
		return err
	}

	return nil
}

// readFrame 读取帧（2 字节长度 + 数据）
func readFrame(r io.Reader) ([]byte, error) {
	lenBuf := make([]byte, 2)
	if _, err := io.ReadFull(r, lenBuf); err != nil {
		return nil, err
	}

	length := binary.BigEndian.Uint16(lenBuf)
	if length == 0 {
		return nil, nil
	}

	data := make([]byte, length)
	if _, err := io.ReadFull(r, data); err != nil {
		return nil, err
	}

	return data, nil
}

// derivePeerIDFromEd25519 从 Ed25519 公钥派生 PeerID
func derivePeerIDFromEd25519(pubKeyBytes []byte) (types.PeerID, error) {
	if len(pubKeyBytes) != ed25519.PublicKeySize {
		return "", fmt.Errorf("invalid ed25519 public key length: %d", len(pubKeyBytes))
	}

	pubKey, err := crypto.UnmarshalEd25519PublicKey(pubKeyBytes)
	if err != nil {
		return "", fmt.Errorf("unmarshal ed25519 public key: %w", err)
	}

	peerID, err := crypto.PeerIDFromPublicKey(pubKey)
	if err != nil {
		return "", fmt.Errorf("derive peer id: %w", err)
	}

	return peerID, nil
}
