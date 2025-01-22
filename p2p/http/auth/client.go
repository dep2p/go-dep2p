package httppeeridauth

import (
	"errors"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/dep2p/go-dep2p/core/crypto"
	"github.com/dep2p/go-dep2p/core/peer"
	"github.com/dep2p/go-dep2p/p2p/http/auth/internal/handshake"
)

// ClientPeerIDAuth 实现了对等节点身份认证的客户端
type ClientPeerIDAuth struct {
	// 客户端私钥
	PrivKey crypto.PrivKey
	// token 的有效期
	TokenTTL time.Duration
	// token 映射表
	tm tokenMap
}

// AuthenticatedDo 类似于 http.Client.Do,但会在需要时执行 dep2p 对等节点身份认证握手
// 建议传入设置了 GetBody 的 http.Request,这样在之前使用的 token 过期时可以重试请求
//
// 参数:
//   - client: HTTP 客户端
//   - req: HTTP 请求
//
// 返回:
//   - peer.ID: 对等节点 ID
//   - *http.Response: HTTP 响应
//   - error: 错误信息
func (a *ClientPeerIDAuth) AuthenticatedDo(client *http.Client, req *http.Request) (peer.ID, *http.Response, error) {
	// 获取主机名
	hostname := req.Host
	// 尝试获取已有的 token
	ti, hasToken := a.tm.get(hostname, a.TokenTTL)
	// 创建握手实例
	handshake := handshake.PeerIDAuthHandshakeClient{
		Hostname: hostname,
		PrivKey:  a.PrivKey,
	}

	if hasToken {
		// 如果有 token,尝试使用它,如果失败则回退到服务器发起的挑战
		peer, resp, err := a.doWithToken(client, req, ti)
		switch {
		case err == nil:
			return peer, resp, nil
		case errors.Is(err, errTokenRejected):
			// token 被拒绝,需要重新认证
			break
		default:
			log.Debugf("doWithToken 失败: %v", err)
			return "", nil, err
		}

		// token 不可用,需要重新认证
		// 运行服务器发起的握手
		req = req.Clone(req.Context())
		req.Body, err = req.GetBody()
		if err != nil {
			log.Debugf("GetBody 失败: %v", err)
			return "", nil, err
		}

		handshake.ParseHeader(resp.Header)
	} else {
		// 没有握手 token,所以我们发起握手
		// 如果我们的 token 被拒绝,服务器会发起握手
		handshake.SetInitiateChallenge()
	}

	// 执行握手
	serverPeerID, resp, err := a.runHandshake(client, req, clearBody(req), &handshake)
	if err != nil {
		log.Debugf("握手失败: %v", err)
		return "", nil, err
	}
	// 保存新的 token
	a.tm.set(hostname, tokenInfo{
		token:      handshake.BearerToken(),
		insertedAt: time.Now(),
		peerID:     serverPeerID,
	})
	return serverPeerID, resp, nil
}

// runHandshake 执行握手过程
//
// 参数:
//   - client: HTTP 客户端
//   - req: HTTP 请求
//   - b: 请求体元数据
//   - hs: 握手实例
//
// 返回:
//   - peer.ID: 对等节点 ID
//   - *http.Response: HTTP 响应
//   - error: 错误信息
func (a *ClientPeerIDAuth) runHandshake(client *http.Client, req *http.Request, b bodyMeta, hs *handshake.PeerIDAuthHandshakeClient) (peer.ID, *http.Response, error) {
	// 设置最大步骤数,避免握手出现无限循环
	maxSteps := 5
	var resp *http.Response

	// 运行握手
	err := hs.Run()
	if err != nil {
		log.Debugf("握手失败: %v", err)
		return "", nil, err
	}

	sentBody := false
	for !hs.HandshakeDone() || !sentBody {
		req = req.Clone(req.Context())
		hs.AddHeader(req.Header)
		if hs.ServerAuthenticated() {
			sentBody = true
			b.setBody(req)
		}

		resp, err = client.Do(req)
		if err != nil {
			log.Debugf("发送请求失败: %v", err)
			return "", nil, err
		}

		hs.ParseHeader(resp.Header)
		err = hs.Run()
		if err != nil {
			log.Debugf("握手失败: %v", err)
			resp.Body.Close()
			return "", nil, err
		}

		if maxSteps--; maxSteps == 0 {
			log.Debugf("握手步骤过多")
			return "", nil, errors.New("握手步骤过多")
		}
	}

	p, err := hs.PeerID()
	if err != nil {
		log.Debugf("获取对等节点 ID 失败: %v", err)
		resp.Body.Close()
		return "", nil, err
	}
	return p, resp, nil
}

// errTokenRejected 表示 token 被拒绝的错误
var errTokenRejected = errors.New("token 被拒绝")

// doWithToken 使用 token 发送请求
//
// 参数:
//   - client: HTTP 客户端
//   - req: HTTP 请求
//   - ti: token 信息
//
// 返回:
//   - peer.ID: 对等节点 ID
//   - *http.Response: HTTP 响应
//   - error: 错误信息
func (a *ClientPeerIDAuth) doWithToken(client *http.Client, req *http.Request, ti tokenInfo) (peer.ID, *http.Response, error) {
	// 使用 token 发送请求
	req.Header.Set("Authorization", ti.token)
	resp, err := client.Do(req)
	if err != nil {
		log.Debugf("发送请求失败: %v", err)
		return "", nil, err
	}
	if resp.StatusCode != http.StatusUnauthorized {
		// token 仍然有效
		return ti.peerID, resp, nil
	}
	if req.GetBody == nil {
		// 无法重试请求
		// 返回响应和错误
		log.Debugf("token 已过期。无法执行握手因为 req.GetBody 为空")
		return "", resp, errors.New("token 已过期。无法执行握手因为 req.GetBody 为空")
	}
	resp.Body.Close()

	return "", resp, errTokenRejected
}

// bodyMeta 保存请求体的元数据
type bodyMeta struct {
	// 请求体
	body io.ReadCloser
	// 内容长度
	contentLength int64
	// 获取请求体的函数
	getBody func() (io.ReadCloser, error)
}

// clearBody 清除请求体并返回元数据
//
// 参数:
//   - req: HTTP 请求
//
// 返回:
//   - bodyMeta: 请求体元数据
func clearBody(req *http.Request) bodyMeta {
	defer func() {
		req.Body = nil
		req.ContentLength = 0
		req.GetBody = nil
	}()
	return bodyMeta{body: req.Body, contentLength: req.ContentLength, getBody: req.GetBody}
}

// setBody 设置请求体
//
// 参数:
//   - req: HTTP 请求
func (b *bodyMeta) setBody(req *http.Request) {
	req.Body = b.body
	req.ContentLength = b.contentLength
	req.GetBody = b.getBody
}

// tokenInfo 保存 token 相关信息
type tokenInfo struct {
	// token 字符串
	token string
	// 插入时间
	insertedAt time.Time
	// 对等节点 ID
	peerID peer.ID
}

// tokenMap 实现了线程安全的 token 映射表
type tokenMap struct {
	// 互斥锁
	tokenMapMu sync.Mutex
	// token 映射表
	tokenMap map[string]tokenInfo
}

// get 获取指定主机名的 token 信息
//
// 参数:
//   - hostname: 主机名
//   - ttl: token 有效期
//
// 返回:
//   - tokenInfo: token 信息
//   - bool: 是否存在有效的 token
func (tm *tokenMap) get(hostname string, ttl time.Duration) (tokenInfo, bool) {
	tm.tokenMapMu.Lock()
	defer tm.tokenMapMu.Unlock()

	ti, ok := tm.tokenMap[hostname]
	if ok && ttl != 0 && time.Since(ti.insertedAt) > ttl {
		delete(tm.tokenMap, hostname)
		return tokenInfo{}, false
	}
	return ti, ok
}

// set 设置指定主机名的 token 信息
//
// 参数:
//   - hostname: 主机名
//   - ti: token 信息
func (tm *tokenMap) set(hostname string, ti tokenInfo) {
	tm.tokenMapMu.Lock()
	defer tm.tokenMapMu.Unlock()
	if tm.tokenMap == nil {
		tm.tokenMap = make(map[string]tokenInfo)
	}
	tm.tokenMap[hostname] = ti
}
