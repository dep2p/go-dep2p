package handshake

import (
	"crypto/hmac"
	"encoding/base64"
	"encoding/json"
	"errors"
	"hash"
	"io"
	"net/http"
	"time"

	"github.com/dep2p/go-dep2p/core/crypto"
	"github.com/dep2p/go-dep2p/core/peer"
)

var (
	// 挑战已过期
	ErrExpiredChallenge = errors.New("挑战已过期")
	// 令牌已过期
	ErrExpiredToken = errors.New("令牌已过期")
	// HMAC 无效
	ErrInvalidHMAC = errors.New("HMAC 无效")
)

// 挑战有效期为5分钟
const challengeTTL = 5 * time.Minute

// peerIDAuthServerState 表示服务器状态的枚举类型
type peerIDAuthServerState int

const (
	// 服务器发起的状态
	// 向客户端发送挑战
	peerIDAuthServerStateChallengeClient peerIDAuthServerState = iota
	// 验证客户端的挑战响应
	peerIDAuthServerStateVerifyChallenge
	// 验证 Bearer 令牌
	peerIDAuthServerStateVerifyBearer

	// 客户端发起的状态
	// 签名挑战
	peerIDAuthServerStateSignChallenge
)

// opaqueState 表示不透明状态的结构体
type opaqueState struct {
	// 是否为令牌
	IsToken bool `json:"is-token,omitempty"`
	// 客户端公钥
	ClientPublicKey []byte `json:"client-public-key,omitempty"`
	// 对等节点 ID
	PeerID peer.ID `json:"peer-id,omitempty"`
	// 客户端挑战
	ChallengeClient string `json:"challenge-client,omitempty"`
	// 主机名
	Hostname string `json:"hostname"`
	// 创建时间
	CreatedTime time.Time `json:"created-time"`
}

// Marshal 将状态序列化并追加到字节切片中
// 参数:
//   - hmac: HMAC 哈希实例
//   - b: 目标字节切片
//
// 返回:
//   - []byte: 序列化后的字节切片
//   - error: 错误信息
func (o *opaqueState) Marshal(hmac hash.Hash, b []byte) ([]byte, error) {
	// 重置 HMAC
	hmac.Reset()
	// 序列化字段
	fieldsMarshalled, err := json.Marshal(o)
	if err != nil {
		log.Debugf("序列化字段失败: %v", err)
		return b, err
	}
	// 计算 HMAC
	_, err = hmac.Write(fieldsMarshalled)
	if err != nil {
		log.Debugf("计算 HMAC 失败: %v", err)
		return b, err
	}
	// 将 HMAC 和序列化的字段追加到结果中
	b = hmac.Sum(b)
	b = append(b, fieldsMarshalled...)
	return b, nil
}

// Unmarshal 从字节切片中反序列化状态
// 参数:
//   - hmacImpl: HMAC 哈希实例
//   - d: 包含序列化数据的字节切片
//
// 返回:
//   - error: 错误信息
func (o *opaqueState) Unmarshal(hmacImpl hash.Hash, d []byte) error {
	// 重置 HMAC
	hmacImpl.Reset()
	// 检查数据长度是否足够
	if len(d) < hmacImpl.Size() {
		log.Debugf("HMAC 值长度不足")
		return ErrInvalidHMAC
	}
	// 分离 HMAC 值和字段数据
	hmacVal := d[:hmacImpl.Size()]
	fields := d[hmacImpl.Size():]
	// 计算 HMAC
	_, err := hmacImpl.Write(fields)
	if err != nil {
		log.Debugf("计算 HMAC 失败: %v", err)
		return ErrInvalidHMAC
	}
	// 验证 HMAC
	expectedHmac := hmacImpl.Sum(nil)
	if !hmac.Equal(hmacVal, expectedHmac) {
		log.Debugf("HMAC 验证失败")
		return ErrInvalidHMAC
	}

	// 反序列化字段
	err = json.Unmarshal(fields, &o)
	if err != nil {
		log.Debugf("反序列化字段失败: %v", err)
		return err
	}
	return nil
}

// PeerIDAuthHandshakeServer 表示对等节点认证握手服务器
type PeerIDAuthHandshakeServer struct {
	// 主机名
	Hostname string
	// 私钥
	PrivKey crypto.PrivKey
	// 令牌有效期
	TokenTTL time.Duration
	// 用于验证不透明数据和令牌的 HMAC
	Hmac hash.Hash

	// 是否已运行
	ran bool
	// 临时缓冲区
	buf [1024]byte

	// 当前状态
	state peerIDAuthServerState
	// 参数
	p params
	// 头部构建器
	hb headerBuilder

	// 不透明状态
	opaque opaqueState
}

// 无效头部错误
var errInvalidHeader = errors.New("无效的头部")

// Reset 重置服务器状态
func (h *PeerIDAuthHandshakeServer) Reset() {
	// 重置 HMAC
	h.Hmac.Reset()
	// 重置运行标志
	h.ran = false
	// 清空缓冲区
	clear(h.buf[:])
	// 重置状态
	h.state = 0
	// 重置参数
	h.p = params{}
	// 清空头部构建器
	h.hb.clear()
	// 重置不透明状态
	h.opaque = opaqueState{}
}

// ParseHeaderVal 解析头部值
// 参数:
//   - headerVal: 头部值字节切片
//
// 返回:
//   - error: 错误信息
func (h *PeerIDAuthHandshakeServer) ParseHeaderVal(headerVal []byte) error {
	// 检查头部值是否为空
	if len(headerVal) == 0 {
		// 初始状态，无需解析
		return nil
	}
	// 解析对等节点认证方案参数
	err := h.p.parsePeerIDAuthSchemeParams(headerVal)
	if err != nil {
		log.Debugf("解析对等节点认证方案参数失败: %v", err)
		return err
	}
	// 根据参数设置状态
	switch {
	case h.p.sigB64 != nil && h.p.opaqueB64 != nil:
		h.state = peerIDAuthServerStateVerifyChallenge
	case h.p.bearerTokenB64 != nil:
		h.state = peerIDAuthServerStateVerifyBearer
	case h.p.challengeServer != nil && h.p.publicKeyB64 != nil:
		h.state = peerIDAuthServerStateSignChallenge
	default:
		log.Debugf("无效的头部")
		return errInvalidHeader
	}
	return nil
}

// Run 运行握手服务器
// 返回:
//   - error: 错误信息
func (h *PeerIDAuthHandshakeServer) Run() error {
	// 设置运行标志
	h.ran = true
	// 根据状态执行相应操作
	switch h.state {
	case peerIDAuthServerStateSignChallenge:
		// 写入认证方案
		h.hb.writeScheme(PeerIDAuthScheme)
		// 添加客户端挑战参数
		if err := h.addChallengeClientParam(); err != nil {
			log.Debugf("添加客户端挑战参数失败: %v", err)
			return err
		}
		// 添加公钥参数
		if err := h.addPublicKeyParam(); err != nil {
			log.Debugf("添加公钥参数失败: %v", err)
			return err
		}

		// 解码客户端公钥
		publicKeyBytes, err := base64.URLEncoding.AppendDecode(nil, h.p.publicKeyB64)
		if err != nil {
			log.Debugf("解码客户端公钥失败: %v", err)
			return err
		}
		// 保存客户端公钥
		h.opaque.ClientPublicKey = publicKeyBytes
		// 添加服务器签名参数
		if err := h.addServerSigParam(publicKeyBytes); err != nil {
			log.Debugf("添加服务器签名参数失败: %v", err)
			return err
		}
		// 添加不透明参数
		if err := h.addOpaqueParam(); err != nil {
			log.Debugf("添加不透明参数失败: %v", err)
			return err
		}
	case peerIDAuthServerStateChallengeClient:
		// 写入认证方案
		h.hb.writeScheme(PeerIDAuthScheme)
		// 添加客户端挑战参数
		if err := h.addChallengeClientParam(); err != nil {
			log.Debugf("添加客户端挑战参数失败: %v", err)
			return err
		}
		// 添加公钥参数
		if err := h.addPublicKeyParam(); err != nil {
			log.Debugf("添加公钥参数失败: %v", err)
			return err
		}
		// 添加不透明参数
		if err := h.addOpaqueParam(); err != nil {
			log.Debugf("添加不透明参数失败: %v", err)
			return err
		}
	case peerIDAuthServerStateVerifyChallenge:
		// 解码不透明数据
		opaque, err := base64.URLEncoding.AppendDecode(h.buf[:0], h.p.opaqueB64)
		if err != nil {
			log.Debugf("解码不透明数据失败: %v", err)
			return err
		}
		// 反序列化不透明状态
		err = h.opaque.Unmarshal(h.Hmac, opaque)
		if err != nil {
			log.Debugf("反序列化不透明状态失败: %v", err)
			return err
		}

		// 检查挑战是否过期
		if nowFn().After(h.opaque.CreatedTime.Add(challengeTTL)) {
			log.Debugf("挑战已过期")
			return ErrExpiredChallenge
		}
		// 检查是否为令牌
		if h.opaque.IsToken {
			log.Debugf("预期挑战，但收到令牌")
			return errors.New("预期挑战，但收到令牌")
		}

		// 检查主机名是否匹配
		if h.Hostname != h.opaque.Hostname {
			log.Debugf("主机名不匹配")
			return errors.New("主机名不匹配")
		}

		var publicKeyBytes []byte
		// 判断是否为客户端发起的握手
		clientInitiatedHandshake := h.opaque.ClientPublicKey != nil

		if clientInitiatedHandshake {
			publicKeyBytes = h.opaque.ClientPublicKey
		} else {
			// 检查公钥是否存在
			if len(h.p.publicKeyB64) == 0 {
				log.Debugf("公钥缺失")
				return errors.New("公钥缺失")
			}
			var err error
			// 解码公钥
			publicKeyBytes, err = base64.URLEncoding.AppendDecode(nil, h.p.publicKeyB64)
			if err != nil {
				log.Debugf("解码公钥失败: %v", err)
				return err
			}
		}
		// 解析公钥
		pubKey, err := crypto.UnmarshalPublicKey(publicKeyBytes)
		if err != nil {
			log.Debugf("解析公钥失败: %v", err)
			return err
		}
		// 验证签名
		if err := h.verifySig(pubKey); err != nil {
			log.Debugf("验证签名失败: %v", err)
			return err
		}

		// 从公钥获取对等节点 ID
		peerID, err := peer.IDFromPublicKey(pubKey)
		if err != nil {
			log.Debugf("从公钥获取对等节点 ID 失败: %v", err)
			return err
		}

		// 创建客户端的 Bearer 令牌
		h.opaque = opaqueState{
			IsToken:     true,
			PeerID:      peerID,
			Hostname:    h.Hostname,
			CreatedTime: nowFn(),
		}

		// 写入认证方案
		h.hb.writeScheme(PeerIDAuthScheme)

		if !clientInitiatedHandshake {
			// 添加服务器签名参数
			if err := h.addServerSigParam(publicKeyBytes); err != nil {
				log.Debugf("添加服务器签名参数失败: %v", err)
				return err
			}
		}
		// 添加 Bearer 参数
		if err := h.addBearerParam(); err != nil {
			log.Debugf("添加 Bearer 参数失败: %v", err)
			return err
		}
	case peerIDAuthServerStateVerifyBearer:
		// 解码 Bearer 令牌
		bearerToken, err := base64.URLEncoding.AppendDecode(h.buf[:0], h.p.bearerTokenB64)
		if err != nil {
			log.Debugf("解码 Bearer 令牌失败: %v", err)
			return err
		}
		// 反序列化不透明状态
		err = h.opaque.Unmarshal(h.Hmac, bearerToken)
		if err != nil {
			log.Debugf("反序列化不透明状态失败: %v", err)
			return err
		}

		// 检查是否为令牌
		if !h.opaque.IsToken {
			log.Debugf("预期令牌，但收到挑战")
			return errors.New("预期令牌，但收到挑战")
		}

		// 检查令牌是否过期
		if nowFn().After(h.opaque.CreatedTime.Add(h.TokenTTL)) {
			log.Debugf("令牌已过期")
			return ErrExpiredToken
		}

		return nil
	default:
		log.Debugf("未处理的状态")
		return errors.New("未处理的状态")
	}

	return nil
}

// addChallengeClientParam 添加客户端挑战参数
// 返回:
//   - error: 错误信息
func (h *PeerIDAuthHandshakeServer) addChallengeClientParam() error {
	// 生成随机挑战
	_, err := io.ReadFull(randReader, h.buf[:challengeLen])
	if err != nil {
		log.Debugf("生成随机挑战失败: %v", err)
		return err
	}
	// 编码挑战
	encodedChallenge := base64.URLEncoding.AppendEncode(h.buf[challengeLen:challengeLen], h.buf[:challengeLen])
	// 保存挑战和状态信息
	h.opaque.ChallengeClient = string(encodedChallenge)
	h.opaque.Hostname = h.Hostname
	h.opaque.CreatedTime = nowFn()
	// 写入挑战参数
	h.hb.writeParam("challenge-client", encodedChallenge)
	return nil
}

// addOpaqueParam 添加不透明参数
// 返回:
//   - error: 错误信息
func (h *PeerIDAuthHandshakeServer) addOpaqueParam() error {
	// 序列化不透明状态
	opaqueVal, err := h.opaque.Marshal(h.Hmac, h.buf[:0])
	if err != nil {
		log.Debugf("序列化不透明状态失败: %v", err)
		return err
	}
	// 写入不透明参数
	h.hb.writeParamB64(h.buf[len(opaqueVal):], "opaque", opaqueVal)
	return nil
}

// addServerSigParam 添加服务器签名参数
// 参数:
//   - clientPublicKeyBytes: 客户端公钥字节
//
// 返回:
//   - error: 错误信息
func (h *PeerIDAuthHandshakeServer) addServerSigParam(clientPublicKeyBytes []byte) error {
	// 检查挑战长度
	if len(h.p.challengeServer) < challengeLen {
		log.Debugf("挑战太短")
		return errors.New("挑战太短")
	}
	// 生成服务器签名
	serverSig, err := sign(h.PrivKey, PeerIDAuthScheme, []sigParam{
		{"challenge-server", h.p.challengeServer},
		{"client-public-key", clientPublicKeyBytes},
		{"hostname", []byte(h.Hostname)},
	})
	if err != nil {
		log.Debugf("生成服务器签名失败: %v", err)
		return err
	}
	// 写入签名参数
	h.hb.writeParamB64(h.buf[:], "sig", serverSig)
	return nil
}

// addBearerParam 添加 Bearer 参数
// 返回:
//   - error: 错误信息
func (h *PeerIDAuthHandshakeServer) addBearerParam() error {
	// 序列化 Bearer 令牌
	bearerToken, err := h.opaque.Marshal(h.Hmac, h.buf[:0])
	if err != nil {
		log.Debugf("序列化 Bearer 令牌失败: %v", err)
		return err
	}
	// 写入 Bearer 参数
	h.hb.writeParamB64(h.buf[len(bearerToken):], "bearer", bearerToken)
	return nil
}

// addPublicKeyParam 添加公钥参数
// 返回:
//   - error: 错误信息
func (h *PeerIDAuthHandshakeServer) addPublicKeyParam() error {
	// 获取服务器公钥
	serverPubKey := h.PrivKey.GetPublic()
	// 序列化公钥
	pubKeyBytes, err := crypto.MarshalPublicKey(serverPubKey)
	if err != nil {
		log.Debugf("序列化公钥失败: %v", err)
		return err
	}
	// 写入公钥参数
	h.hb.writeParamB64(h.buf[:], "public-key", pubKeyBytes)
	return nil
}

// verifySig 验证签名
// 参数:
//   - clientPubKey: 客户端公钥
//
// 返回:
//   - error: 错误信息
func (h *PeerIDAuthHandshakeServer) verifySig(clientPubKey crypto.PubKey) error {
	// 获取服务器公钥
	serverPubKey := h.PrivKey.GetPublic()
	// 序列化服务器公钥
	serverPubKeyBytes, err := crypto.MarshalPublicKey(serverPubKey)
	if err != nil {
		log.Debugf("序列化服务器公钥失败: %v", err)
		return err
	}
	// 解码签名
	sig, err := base64.URLEncoding.AppendDecode(h.buf[:0], h.p.sigB64)
	if err != nil {
		log.Debugf("解码签名失败: %v", err)
		return err
	}
	// 验证签名
	err = verifySig(clientPubKey, PeerIDAuthScheme, []sigParam{
		{k: "challenge-client", v: []byte(h.opaque.ChallengeClient)},
		{k: "server-public-key", v: serverPubKeyBytes},
		{k: "hostname", v: []byte(h.Hostname)},
	}, sig)
	if err != nil {
		log.Debugf("验证签名失败: %v", err)
		return err
	}
	return nil
}

// PeerID 返回已认证客户端的对等节点 ID
// 返回:
//   - peer.ID: 对等节点 ID
//   - error: 错误信息
func (h *PeerIDAuthHandshakeServer) PeerID() (peer.ID, error) {
	// 检查是否已运行
	if !h.ran {
		log.Debugf("未运行")
		return "", errNotRan
	}
	// 检查状态是否正确
	switch h.state {
	case peerIDAuthServerStateVerifyChallenge:
	case peerIDAuthServerStateVerifyBearer:
	default:
		log.Debugf("状态不正确")
		return "", errors.New("状态不正确")
	}
	// 检查对等节点 ID 是否已设置
	if h.opaque.PeerID == "" {
		log.Debugf("对等节点 ID 未设置")
		return "", errors.New("对等节点 ID 未设置")
	}
	return h.opaque.PeerID, nil
}

// SetHeader 设置 HTTP 头部
// 参数:
//   - hdr: HTTP 头部
func (h *PeerIDAuthHandshakeServer) SetHeader(hdr http.Header) {
	// 检查是否已运行
	if !h.ran {
		return
	}
	// 清理头部构建器
	defer h.hb.clear()
	// 根据状态设置不同的头部
	switch h.state {
	case peerIDAuthServerStateChallengeClient, peerIDAuthServerStateSignChallenge:
		hdr.Set("WWW-Authenticate", h.hb.b.String())
	case peerIDAuthServerStateVerifyChallenge:
		hdr.Set("Authentication-Info", h.hb.b.String())
	case peerIDAuthServerStateVerifyBearer:
		// 完整性考虑，无需操作
	}
}
