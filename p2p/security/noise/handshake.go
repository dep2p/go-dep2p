package noise

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"os"
	"runtime/debug"
	"time"

	"github.com/dep2p/go-dep2p/core/crypto"
	"github.com/dep2p/go-dep2p/core/peer"
	"github.com/dep2p/go-dep2p/core/sec"
	"github.com/dep2p/go-dep2p/p2p/security/noise/pb"

	pool "github.com/dep2p/go-dep2p/p2plib/buffer/pool"
	"github.com/flynn/noise"
	"google.golang.org/protobuf/proto"
)

// payloadSigPrefix 在使用 dep2p 身份密钥签名之前添加到 Noise 静态密钥前面的前缀
const payloadSigPrefix = "noise-dep2p-static-key:"

// 所有 noise 会话共享一个固定的密码套件
var cipherSuite = noise.NewCipherSuite(noise.DH25519, noise.CipherChaChaPoly, noise.HashSHA256)

// runHandshake 与远程对等节点交换握手消息以建立 noise-dep2p 会话
// 参数:
//   - ctx: context.Context 上下文对象
//
// 返回值:
//   - error 握手过程中的错误
//
// 注意:
//   - 此方法会阻塞直到握手完成或失败
func (s *secureSession) runHandshake(ctx context.Context) (err error) {
	// 使用 defer 捕获可能的 panic
	defer func() {
		if rerr := recover(); rerr != nil {
			fmt.Fprintf(os.Stderr, "捕获到 panic: %s\n%s\n", rerr, debug.Stack())
			err = fmt.Errorf("noise 握手过程中发生 panic: %s", rerr)
			log.Debugf("noise 握手过程中发生 panic: %s", rerr)
		}
	}()

	// 生成静态密钥对
	kp, err := noise.DH25519.GenerateKeypair(rand.Reader)
	if err != nil {
		log.Debugf("生成静态密钥对时出错: %s", err)
		return err
	}

	// 配置 Noise 握手参数
	cfg := noise.Config{
		CipherSuite:   cipherSuite,
		Pattern:       noise.HandshakeXX,
		Initiator:     s.initiator,
		StaticKeypair: kp,
		Prologue:      s.prologue,
	}

	// 初始化握手状态
	hs, err := noise.NewHandshakeState(cfg)
	if err != nil {
		log.Debugf("初始化握手状态时出错: %s", err)
		return err
	}

	// 如果提供了截止时间,设置完成握手的期限
	// 握手完成后清除期限
	if deadline, ok := ctx.Deadline(); ok {
		if err := s.SetDeadline(deadline); err == nil {
			defer s.SetDeadline(time.Time{})
		}
	}

	// 可以重复使用此缓冲区来存储所有握手消息
	hbuf := pool.Get(2 << 10)
	defer pool.Put(hbuf)

	if s.initiator {
		// 阶段 0 //
		// 握手消息长度 = DH 临时密钥的长度
		if err := s.sendHandshakeMessage(hs, nil, hbuf); err != nil {
			log.Debugf("发送握手消息时出错: %s", err)
			return err
		}

		// 阶段 1 //
		plaintext, err := s.readHandshakeMessage(hs)
		if err != nil {
			log.Debugf("读取握手消息时出错: %s", err)
			return err
		}
		rcvdEd, err := s.handleRemoteHandshakePayload(plaintext, hs.PeerStatic())
		if err != nil {
			log.Debugf("处理远程握手负载时出错: %s", err)
			return err
		}
		if s.initiatorEarlyDataHandler != nil {
			if err := s.initiatorEarlyDataHandler.Received(ctx, s.insecureConn, rcvdEd); err != nil {
				log.Debugf("处理初始化握手负载时出错: %s", err)
				return err
			}
		}

		// 阶段 2 //
		// 握手消息长度 = DHT 静态密钥长度 + MAC(静态密钥已加密) + 负载长度 + MAC(负载已加密)
		var ed *pb.NoiseExtensions
		if s.initiatorEarlyDataHandler != nil {
			ed = s.initiatorEarlyDataHandler.Send(ctx, s.insecureConn, s.remoteID)
		}
		payload, err := s.generateHandshakePayload(kp, ed)
		if err != nil {
			log.Debugf("生成握手负载时出错: %s", err)
			return err
		}
		if err := s.sendHandshakeMessage(hs, payload, hbuf); err != nil {
			log.Debugf("发送握手消息时出错: %s", err)
			return err
		}
		return nil
	} else {
		// 阶段 0 //
		if _, err := s.readHandshakeMessage(hs); err != nil {
			log.Debugf("读取握手消息时出错: %s", err)
			return err
		}

		// 阶段 1 //
		// 握手消息长度 = DH 临时密钥长度 + DHT 静态密钥长度 + MAC(静态密钥已加密) + 负载长度 + MAC(负载已加密)
		var ed *pb.NoiseExtensions
		if s.responderEarlyDataHandler != nil {
			ed = s.responderEarlyDataHandler.Send(ctx, s.insecureConn, s.remoteID)
		}
		payload, err := s.generateHandshakePayload(kp, ed)
		if err != nil {
			log.Debugf("生成握手负载时出错: %s", err)
			return err
		}
		if err := s.sendHandshakeMessage(hs, payload, hbuf); err != nil {
			log.Debugf("发送握手消息时出错: %s", err)
			return err
		}

		// 阶段 2 //
		plaintext, err := s.readHandshakeMessage(hs)
		if err != nil {
			log.Debugf("读取握手消息时出错: %s", err)
			return err
		}
		rcvdEd, err := s.handleRemoteHandshakePayload(plaintext, hs.PeerStatic())
		if err != nil {
			log.Debugf("处理远程握手负载时出错: %s", err)
			return err
		}
		if s.responderEarlyDataHandler != nil {
			if err := s.responderEarlyDataHandler.Received(ctx, s.insecureConn, rcvdEd); err != nil {
				log.Debugf("处理响应握手负载时出错: %s", err)
				return err
			}
		}
		return nil
	}
}

// setCipherStates 设置用于保护握手后通信的初始密码状态
// 参数:
//   - cs1: *noise.CipherState 第一个密码状态
//   - cs2: *noise.CipherState 第二个密码状态
//
// 注意:
//   - 当最终握手消息被 sendHandshakeMessage 或 readHandshakeMessage 处理时调用
func (s *secureSession) setCipherStates(cs1, cs2 *noise.CipherState) {
	if s.initiator {
		s.enc = cs1
		s.dec = cs2
	} else {
		s.enc = cs2
		s.dec = cs1
	}
}

// sendHandshakeMessage 发送握手序列中的下一条消息
// 参数:
//   - hs: *noise.HandshakeState 握手状态
//   - payload: []byte 要包含在握手消息中的负载
//   - hbuf: []byte 用于存储握手消息的缓冲区
//
// 返回值:
//   - error 发送过程中的错误
//
// 注意:
//   - 如果 payload 非空,它将被包含在握手消息中
//   - 如果这是序列中的最后一条消息,将调用 setCipherStates 初始化密码状态
func (s *secureSession) sendHandshakeMessage(hs *noise.HandshakeState, payload []byte, hbuf []byte) error {
	// 前两个字节将是 noise 握手消息的长度
	bz, cs1, cs2, err := hs.WriteMessage(hbuf[:LengthPrefixLength], payload)
	if err != nil {
		log.Debugf("发送握手消息时出错: %s", err)
		return err
	}

	// bz 也将包含长度前缀,因为我们传递了 LengthPrefixLength 长度的切片给 hs.Write()
	binary.BigEndian.PutUint16(bz, uint16(len(bz)-LengthPrefixLength))

	_, err = s.writeMsgInsecure(bz)
	if err != nil {
		log.Debugf("发送握手消息时出错: %s", err)
		return err
	}

	if cs1 != nil && cs2 != nil {
		s.setCipherStates(cs1, cs2)
	}
	return nil
}

// readHandshakeMessage 从不安全连接读取消息并尝试将其作为握手序列中的下一条预期消息进行处理
// 参数:
//   - hs: *noise.HandshakeState 握手状态
//
// 返回值:
//   - []byte 如果消息包含负载,则返回解密后的负载
//   - error 读取过程中的错误
//
// 注意:
//   - 如果这是序列中的最后一条消息,它将调用 setCipherStates 初始化密码状态
func (s *secureSession) readHandshakeMessage(hs *noise.HandshakeState) ([]byte, error) {
	l, err := s.readNextInsecureMsgLen()
	if err != nil {
		log.Debugf("读取握手消息时出错: %s", err)
		return nil, err
	}

	buf := pool.Get(l)
	defer pool.Put(buf)

	if err := s.readNextMsgInsecure(buf); err != nil {
		log.Debugf("读取握手消息时出错: %s", err)
		return nil, err
	}

	msg, cs1, cs2, err := hs.ReadMessage(nil, buf)
	if err != nil {
		log.Debugf("读取握手消息时出错: %s", err)
		return nil, err
	}
	if cs1 != nil && cs2 != nil {
		s.setCipherStates(cs1, cs2)
	}
	return msg, nil
}

// generateHandshakePayload 创建带有静态 noise 密钥签名的 dep2p 握手负载
// 参数:
//   - localStatic: noise.DHKey 本地静态密钥对
//   - ext: *pb.NoiseExtensions 扩展数据
//
// 返回值:
//   - []byte 序列化的握手负载
//   - error 生成过程中的错误
func (s *secureSession) generateHandshakePayload(localStatic noise.DHKey, ext *pb.NoiseExtensions) ([]byte, error) {
	// 从握手会话获取公钥,以便我们可以用 dep2p 私钥对其签名
	localKeyRaw, err := crypto.MarshalPublicKey(s.LocalPublicKey())
	if err != nil {
		log.Debugf("序列化 dep2p 身份密钥时出错: %s", err)
		return nil, err
	}

	// 准备要签名的负载并执行签名
	toSign := append([]byte(payloadSigPrefix), localStatic.Public...)
	signedPayload, err := s.localKey.Sign(toSign)
	if err != nil {
		log.Debugf("签名握手负载时出错: %s", err)
		return nil, err
	}

	// 创建负载
	payloadEnc, err := proto.Marshal(&pb.NoiseHandshakePayload{
		IdentityKey: localKeyRaw,
		IdentitySig: signedPayload,
		Extensions:  ext,
	})
	if err != nil {
		log.Debugf("序列化握手负载时出错: %s", err)
		return nil, err
	}
	return payloadEnc, nil
}

// handleRemoteHandshakePayload 解析远程对等节点发送的握手负载对象,并验证对等节点静态 Noise 密钥的签名
// 参数:
//   - payload: []byte 握手负载数据
//   - remoteStatic: []byte 远程静态密钥
//
// 返回值:
//   - *pb.NoiseExtensions 附加到负载的数据
//   - error 处理过程中的错误
func (s *secureSession) handleRemoteHandshakePayload(payload []byte, remoteStatic []byte) (*pb.NoiseExtensions, error) {
	// 解析负载
	nhp := new(pb.NoiseHandshakePayload)
	err := proto.Unmarshal(payload, nhp)
	if err != nil {
		log.Debugf("解析远程握手负载时出错: %s", err)
		return nil, err
	}

	// 解包远程对等节点的公共 dep2p 密钥
	remotePubKey, err := crypto.UnmarshalPublicKey(nhp.GetIdentityKey())
	if err != nil {
		log.Debugf("解包远程对等节点的公共 dep2p 密钥时出错: %s", err)
		return nil, err
	}
	id, err := peer.IDFromPublicKey(remotePubKey)
	if err != nil {
		log.Debugf("从公共 dep2p 密钥创建对等节点 ID 时出错: %s", err)
		return nil, err
	}

	// 如果启用了检查对等节点 ID
	if s.checkPeerID && s.remoteID != id {
		log.Debugf("对等节点 ID 不匹配: 期望 %s, 实际 %s", s.remoteID, id)
		return nil, sec.ErrPeerIDMismatch{Expected: s.remoteID, Actual: id}
	}

	// 验证负载是否由声称的远程 dep2p 密钥签名
	sig := nhp.GetIdentitySig()
	msg := append([]byte(payloadSigPrefix), remoteStatic...)
	ok, err := remotePubKey.Verify(msg, sig)
	if err != nil {
		log.Debugf("验证签名时出错: %s", err)
		return nil, err
	} else if !ok {
		log.Debugf("握手签名无效")
		return nil, fmt.Errorf("握手签名无效")
	}

	// 设置远程对等节点密钥和 ID
	s.remoteID = id
	s.remoteKey = remotePubKey
	return nhp.Extensions, nil
}
