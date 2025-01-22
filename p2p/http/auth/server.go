package httppeeridauth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"hash"
	"net/http"
	"sync"
	"time"

	"github.com/dep2p/go-dep2p/core/crypto"
	"github.com/dep2p/go-dep2p/core/peer"
	"github.com/dep2p/go-dep2p/p2p/http/auth/internal/handshake"
)

// hmacPool 是一个 HMAC 哈希函数的对象池
type hmacPool struct {
	p sync.Pool
}

// newHmacPool 创建一个新的 HMAC 对象池
// 参数:
//   - key: 用于创建 HMAC 的密钥
//
// 返回:
//   - *hmacPool: 新创建的 HMAC 对象池
func newHmacPool(key []byte) *hmacPool {
	return &hmacPool{
		p: sync.Pool{
			New: func() any {
				return hmac.New(sha256.New, key)
			},
		},
	}
}

// Get 从对象池中获取一个 HMAC 哈希函数
// 返回:
//   - hash.Hash: 重置后的 HMAC 哈希函数
func (p *hmacPool) Get() hash.Hash {
	h := p.p.Get().(hash.Hash)
	h.Reset()
	return h
}

// Put 将 HMAC 哈希函数放回对象池
// 参数:
//   - h: 要放回的 HMAC 哈希函数
func (p *hmacPool) Put(h hash.Hash) {
	p.p.Put(h)
}

// ServerPeerIDAuth 是处理对等节点身份认证的服务器结构体
type ServerPeerIDAuth struct {
	// PrivKey 是服务器的私钥
	PrivKey crypto.PrivKey
	// TokenTTL 是令牌的有效期
	TokenTTL time.Duration
	// Next 是认证成功后要执行的处理函数
	Next func(peer peer.ID, w http.ResponseWriter, r *http.Request)
	// NoTLS 允许服务器接受没有 TLS ServerName 的请求
	// 当其他组件终止 TLS 连接时使用
	NoTLS bool
	// ValidHostnameFn 在 NoTLS 为 true 时必需
	// 服务器只接受 Host 头返回 true 的请求
	ValidHostnameFn func(hostname string) bool

	// HmacKey 是用于 HMAC 计算的密钥
	HmacKey []byte
	// initHmac 确保 HMAC 只初始化一次
	initHmac sync.Once
	// hmacPool 是 HMAC 哈希函数的对象池
	hmacPool *hmacPool
}

// ServeHTTP 实现了 http.Handler 接口
// 使用 dep2p 对等节点身份认证方案验证请求
// 如果设置了 Next 处理函数，将在认证成功后调用它
// 参数:
//   - w: HTTP 响应写入器
//   - r: HTTP 请求
func (a *ServerPeerIDAuth) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// 初始化 HMAC 密钥和对象池
	a.initHmac.Do(func() {
		if a.HmacKey == nil {
			key := make([]byte, 32)
			_, err := rand.Read(key)
			if err != nil {
				panic(err)
			}
			a.HmacKey = key
		}
		a.hmacPool = newHmacPool(a.HmacKey)
	})

	// 获取主机名
	hostname := r.Host
	if a.NoTLS {
		if a.ValidHostnameFn == nil {
			log.Debugf("未设置 ValidHostnameFn，NoTLS 模式下必需")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if !a.ValidHostnameFn(hostname) {
			log.Debugf("主机 %s 的未授权请求：ValidHostnameFn 返回 false", hostname)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
	} else {
		if r.TLS == nil {
			log.Warn("无 TLS 连接，且 NoTLS 为 false")
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if hostname != r.TLS.ServerName {
			log.Debugf("主机 %s 的未授权请求：主机名不匹配。期望 %s", hostname, r.TLS.ServerName)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if a.ValidHostnameFn != nil && !a.ValidHostnameFn(hostname) {
			log.Debugf("主机 %s 的未授权请求：ValidHostnameFn 返回 false", hostname)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
	}

	// 从对象池获取 HMAC
	hmac := a.hmacPool.Get()
	defer a.hmacPool.Put(hmac)

	// 创建握手服务器
	hs := handshake.PeerIDAuthHandshakeServer{
		Hostname: hostname,
		PrivKey:  a.PrivKey,
		TokenTTL: a.TokenTTL,
		Hmac:     hmac,
	}

	// 解析认证头
	err := hs.ParseHeaderVal([]byte(r.Header.Get("Authorization")))
	if err != nil {
		log.Debugf("解析头部失败：%v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// 执行握手
	err = hs.Run()
	if err != nil {
		switch {
		case errors.Is(err, handshake.ErrInvalidHMAC),
			errors.Is(err, handshake.ErrExpiredChallenge),
			errors.Is(err, handshake.ErrExpiredToken):

			hmac.Reset()
			hs := handshake.PeerIDAuthHandshakeServer{
				Hostname: hostname,
				PrivKey:  a.PrivKey,
				TokenTTL: a.TokenTTL,
				Hmac:     hmac,
			}
			hs.Run()
			hs.SetHeader(w.Header())
			w.WriteHeader(http.StatusUnauthorized)

			return
		}

		log.Debugf("握手失败：%v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	hs.SetHeader(w.Header())

	// 获取对等节点 ID
	peer, err := hs.PeerID()
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	// 调用下一个处理函数或返回成功
	if a.Next == nil {
		w.WriteHeader(http.StatusOK)
		return
	}
	a.Next(peer, w, r)
}
