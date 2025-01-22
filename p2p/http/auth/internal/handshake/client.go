package handshake

import (
	"encoding/base64"
	"errors"
	"io"
	"net/http"

	"github.com/dep2p/go-dep2p/core/crypto"
	"github.com/dep2p/go-dep2p/core/peer"
	logging "github.com/dep2p/log"
)

var log = logging.Logger("http-peer-id-auth")

// peerIDAuthClientState 表示客户端握手状态的枚举类型
type peerIDAuthClientState int

const (
	// 客户端需要签名挑战
	peerIDAuthClientStateSignChallenge peerIDAuthClientState = iota
	// 客户端需要验证挑战
	peerIDAuthClientStateVerifyChallenge
	// 客户端已获得 bearer token,握手完成
	peerIDAuthClientStateDone

	// 客户端发起的握手状态
	// 客户端发起挑战
	peerIDAuthClientInitiateChallenge
	// 客户端需要验证和签名挑战
	peerIDAuthClientStateVerifyAndSignChallenge
	// 客户端等待 bearer token
	peerIDAuthClientStateWaitingForBearer
)

// PeerIDAuthHandshakeClient 实现了对等节点认证握手的客户端
type PeerIDAuthHandshakeClient struct {
	// 主机名
	Hostname string
	// 客户端私钥
	PrivKey crypto.PrivKey

	// 服务端对等节点 ID
	serverPeerID peer.ID
	// 服务端公钥
	serverPubKey crypto.PubKey
	// 当前握手状态
	state peerIDAuthClientState
	// 参数集合
	p params
	// 头部构建器
	hb headerBuilder
	// 服务端挑战数据
	challengeServer []byte
	// 临时缓冲区
	buf [128]byte
}

// errMissingChallenge 表示缺少挑战数据的错误
var errMissingChallenge = errors.New("缺少挑战数据")

// SetInitiateChallenge 设置客户端为发起挑战状态
func (h *PeerIDAuthHandshakeClient) SetInitiateChallenge() {
	h.state = peerIDAuthClientInitiateChallenge
}

// ParseHeader 解析 HTTP 头部中的认证信息
// 参数:
//   - header: HTTP 头部
//
// 返回值:
//   - error: 解析过程中的错误
func (h *PeerIDAuthHandshakeClient) ParseHeader(header http.Header) error {
	// 如果已完成或正在发起挑战,则无需解析
	if h.state == peerIDAuthClientStateDone || h.state == peerIDAuthClientInitiateChallenge {
		return nil
	}
	// 重置参数
	h.p = params{}

	var headerVal []byte
	// 根据当前状态选择要解析的头部字段
	switch h.state {
	case peerIDAuthClientStateSignChallenge, peerIDAuthClientStateVerifyAndSignChallenge:
		headerVal = []byte(header.Get("WWW-Authenticate"))
	case peerIDAuthClientStateVerifyChallenge, peerIDAuthClientStateWaitingForBearer:
		headerVal = []byte(header.Get("Authentication-Info"))
	}

	// 检查头部值是否存在
	if len(headerVal) == 0 {
		log.Debugf("缺少认证信息")
		return errMissingChallenge
	}

	// 解析认证方案参数
	err := h.p.parsePeerIDAuthSchemeParams(headerVal)
	if err != nil {
		log.Debugf("解析认证方案参数失败: %v", err)
		return err
	}

	// 如果服务端公钥未设置且存在公钥数据,则解析并设置
	if h.serverPubKey == nil && len(h.p.publicKeyB64) > 0 {
		// 解码 base64 格式的公钥
		serverPubKeyBytes, err := base64.URLEncoding.AppendDecode(nil, h.p.publicKeyB64)
		if err != nil {
			log.Debugf("解码公钥失败: %v", err)
			return err
		}
		// 解析公钥
		h.serverPubKey, err = crypto.UnmarshalPublicKey(serverPubKeyBytes)
		if err != nil {
			log.Debugf("解析公钥失败: %v", err)
			return err
		}
		// 从公钥生成对等节点 ID
		h.serverPeerID, err = peer.IDFromPublicKey(h.serverPubKey)
		if err != nil {
			log.Debugf("从公钥生成对等节点 ID 失败: %v", err)
			return err
		}
	}

	return err
}

// Run 执行握手流程的一个步骤
// 返回值:
//   - error: 执行过程中的错误
func (h *PeerIDAuthHandshakeClient) Run() error {
	// 如果握手已完成,则直接返回
	if h.state == peerIDAuthClientStateDone {
		return nil
	}

	// 清空头部构建器
	h.hb.clear()
	// 获取客户端公钥字节
	clientPubKeyBytes, err := crypto.MarshalPublicKey(h.PrivKey.GetPublic())
	if err != nil {
		log.Debugf("获取客户端公钥字节失败: %v", err)
		return err
	}

	// 根据当前状态执行相应的操作
	switch h.state {
	case peerIDAuthClientInitiateChallenge:
		// 客户端发起挑战
		h.hb.writeScheme(PeerIDAuthScheme)
		h.addChallengeServerParam()
		h.hb.writeParamB64(nil, "public-key", clientPubKeyBytes)
		h.state = peerIDAuthClientStateVerifyAndSignChallenge
		return nil
	case peerIDAuthClientStateVerifyAndSignChallenge:
		// 如果服务端拒绝客户端发起的握手
		if len(h.p.sigB64) == 0 && len(h.p.challengeClient) != 0 {
			// 切换到服务端发起的握手流程
			h.state = peerIDAuthClientStateSignChallenge
			return h.Run()
		}
		// 验证服务端签名
		if err := h.verifySig(clientPubKeyBytes); err != nil {
			return err
		}

		// 构建响应
		h.hb.writeScheme(PeerIDAuthScheme)
		h.hb.writeParam("opaque", h.p.opaqueB64)
		h.addSigParam()
		h.state = peerIDAuthClientStateWaitingForBearer
		return nil

	case peerIDAuthClientStateWaitingForBearer:
		// 等待 bearer token
		h.hb.writeScheme(PeerIDAuthScheme)
		h.hb.writeParam("bearer", h.p.bearerTokenB64)
		h.state = peerIDAuthClientStateDone
		return nil

	case peerIDAuthClientStateSignChallenge:
		// 验证挑战长度
		if len(h.p.challengeClient) < challengeLen {
			log.Debugf("挑战数据长度过短")
			return errors.New("挑战数据长度过短")
		}

		// 构建响应
		h.hb.writeScheme(PeerIDAuthScheme)
		h.hb.writeParamB64(nil, "public-key", clientPubKeyBytes)
		if err := h.addChallengeServerParam(); err != nil {
			log.Debugf("添加服务端挑战参数失败: %v", err)
			return err
		}
		if err := h.addSigParam(); err != nil {
			log.Debugf("添加客户端签名参数失败: %v", err)
			return err
		}
		h.hb.writeParam("opaque", h.p.opaqueB64)

		h.state = peerIDAuthClientStateVerifyChallenge
		return nil
	case peerIDAuthClientStateVerifyChallenge:
		// 验证服务端签名
		if err := h.verifySig(clientPubKeyBytes); err != nil {
			log.Debugf("验证服务端签名失败: %v", err)
			return err
		}

		// 构建最终响应
		h.hb.writeScheme(PeerIDAuthScheme)
		h.hb.writeParam("bearer", h.p.bearerTokenB64)
		h.state = peerIDAuthClientStateDone

		return nil
	}

	return errors.New("未处理的状态")
}

// addChallengeServerParam 生成并添加服务端挑战参数
// 返回值:
//   - error: 生成过程中的错误
func (h *PeerIDAuthHandshakeClient) addChallengeServerParam() error {
	// 生成随机挑战数据
	_, err := io.ReadFull(randReader, h.buf[:challengeLen])
	if err != nil {
		log.Debugf("生成随机挑战数据失败: %v", err)
		return err
	}
	// 编码为 base64 格式
	h.challengeServer = base64.URLEncoding.AppendEncode(nil, h.buf[:challengeLen])
	// 清空缓冲区
	clear(h.buf[:challengeLen])
	// 添加到头部
	h.hb.writeParam("challenge-server", h.challengeServer)
	return nil
}

// verifySig 验证服务端签名
// 参数:
//   - clientPubKeyBytes: 客户端公钥字节
//
// 返回值:
//   - error: 验证过程中的错误
func (h *PeerIDAuthHandshakeClient) verifySig(clientPubKeyBytes []byte) error {
	if len(h.p.sigB64) == 0 {
		log.Debugf("签名未设置")
		return errors.New("签名未设置")
	}
	// 解码签名
	sig, err := base64.URLEncoding.AppendDecode(nil, h.p.sigB64)
	if err != nil {
		log.Debugf("解码签名失败: %v", err)
		return err
	}
	// 验证签名
	err = verifySig(h.serverPubKey, PeerIDAuthScheme, []sigParam{
		{"challenge-server", h.challengeServer},
		{"client-public-key", clientPubKeyBytes},
		{"hostname", []byte(h.Hostname)},
	}, sig)
	return err
}

// addSigParam 生成并添加客户端签名参数
// 返回值:
//   - error: 生成过程中的错误
func (h *PeerIDAuthHandshakeClient) addSigParam() error {
	if h.serverPubKey == nil {
		return errors.New("服务端公钥未设置")
	}
	// 获取服务端公钥字节
	serverPubKeyBytes, err := crypto.MarshalPublicKey(h.serverPubKey)
	if err != nil {
		log.Debugf("获取服务端公钥字节失败: %v", err)
		return err
	}
	// 生成签名
	clientSig, err := sign(h.PrivKey, PeerIDAuthScheme, []sigParam{
		{"challenge-client", h.p.challengeClient},
		{"server-public-key", serverPubKeyBytes},
		{"hostname", []byte(h.Hostname)},
	})
	if err != nil {
		log.Debugf("签名挑战失败: %v", err)
		return err
	}
	// 添加签名参数
	h.hb.writeParamB64(nil, "sig", clientSig)
	return nil
}

// PeerID 返回已认证的服务端对等节点 ID
// 返回值:
//   - peer.ID: 服务端对等节点 ID
//   - error: 获取过程中的错误
func (h *PeerIDAuthHandshakeClient) PeerID() (peer.ID, error) {
	switch h.state {
	case peerIDAuthClientStateDone:
	case peerIDAuthClientStateWaitingForBearer:
	default:
		log.Debugf("服务端尚未认证")
		return "", errors.New("服务端尚未认证")
	}

	if h.serverPeerID == "" {
		log.Debugf("对等节点 ID 未设置")
		return "", errors.New("对等节点 ID 未设置")
	}
	return h.serverPeerID, nil
}

// AddHeader 将认证信息添加到 HTTP 头部
// 参数:
//   - hdr: HTTP 头部
func (h *PeerIDAuthHandshakeClient) AddHeader(hdr http.Header) {
	hdr.Set("Authorization", h.hb.b.String())
}

// BearerToken 返回服务端提供的 bearer token
// 返回值:
//   - string: bearer token 字符串
func (h *PeerIDAuthHandshakeClient) BearerToken() string {
	if h.state != peerIDAuthClientStateDone {
		return ""
	}
	return h.hb.b.String()
}

// ServerAuthenticated 检查服务端是否已认证
// 返回值:
//   - bool: 服务端是否已认证
func (h *PeerIDAuthHandshakeClient) ServerAuthenticated() bool {
	switch h.state {
	case peerIDAuthClientStateDone:
	case peerIDAuthClientStateWaitingForBearer:
	default:
		return false
	}

	return h.serverPeerID != ""
}

// HandshakeDone 检查握手是否已完成
// 返回值:
//   - bool: 握手是否已完成
func (h *PeerIDAuthHandshakeClient) HandshakeDone() bool {
	return h.state == peerIDAuthClientStateDone
}
