// HTTP semantics with dep2p. Can use a dep2p stream transport or stock HTTP
// transports. This API is experimental and will likely change soon. Implements [dep2p spec #508](https://github.com/dep2p/specs/pull/508).
package dep2phttp

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	host "github.com/dep2p/go-dep2p/core/host"
	"github.com/dep2p/go-dep2p/core/network"
	"github.com/dep2p/go-dep2p/core/peer"
	"github.com/dep2p/go-dep2p/core/peerstore"
	"github.com/dep2p/go-dep2p/core/protocol"
	ma "github.com/dep2p/go-dep2p/multiformats/multiaddr"
	gostream "github.com/dep2p/go-dep2p/p2p/net/gostream"
	logging "github.com/dep2p/log"
	lru "github.com/hashicorp/golang-lru/v2"
)

// 日志记录器
var log = logging.Logger("dep2phttp")

// 默认的 well-known 请求超时时间
var WellKnownRequestTimeout = 30 * time.Second

// 用于多流选择的协议ID
const ProtocolIDForMultistreamSelect = "/http/1.1"

// well-known 协议路径
const WellKnownProtocols = "/.well-known/dep2p/protocols"

// LegacyWellKnownProtocols 指向早期 dep2p+http 规范草案中使用的 well-known 资源。
// 一些用户已经部署了这个版本,需要向后兼容。
// 希望将来可以逐步淘汰。上下文: https://github.com/dep2p/pull/2797
const LegacyWellKnownProtocols = "/.well-known/dep2p"

// 对等节点元数据大小限制为 8KB
const peerMetadataLimit = 8 << 10

// LRU 缓存中保存的不同对等节点元数据数量
const peerMetadataLRUSize = 256

// ProtocolMeta 是协议的元数据
type ProtocolMeta struct {
	// Path 定义了此协议使用的 HTTP 路径前缀
	Path string `json:"path"`
}

// PeerMeta 是协议ID到协议元数据的映射
type PeerMeta map[protocol.ID]ProtocolMeta

// WellKnownHandler 是一个处理 well-known 资源的 http.Handler
type WellKnownHandler struct {
	// 用于保护 wellKnownMapping 的互斥锁
	wellknownMapMu sync.Mutex
	// 协议映射
	wellKnownMapping PeerMeta
	// 缓存的 JSON 响应
	wellKnownCache []byte
}

// streamHostListen 返回一个监听 HTTP/1.1 消息的 dep2p 流的 net.Listener
// 参数:
//   - streamHost: dep2p 主机
//
// 返回:
//   - net.Listener: 网络监听器
//   - error: 错误信息
func streamHostListen(streamHost host.Host) (net.Listener, error) {
	return gostream.Listen(streamHost, ProtocolIDForMultistreamSelect, gostream.IgnoreEOF())
}

// ServeHTTP 实现 http.Handler 接口,处理 well-known 请求
// 参数:
//   - w: HTTP 响应写入器
//   - r: HTTP 请求
func (h *WellKnownHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// 检查请求是否接受 JSON
	accepts := r.Header.Get("Accept")
	if accepts != "" && !(strings.Contains(accepts, "application/json") || strings.Contains(accepts, "*/*")) {
		http.Error(w, "仅支持 application/json", http.StatusNotAcceptable)
		return
	}

	// 仅支持 GET 请求
	if r.Method != http.MethodGet {
		http.Error(w, "仅支持 GET 请求", http.StatusMethodNotAllowed)
		return
	}

	// 返回包含 well-known 协议的 JSON 对象
	h.wellknownMapMu.Lock()
	mapping := h.wellKnownCache
	var err error
	if mapping == nil {
		mapping, err = json.Marshal(h.wellKnownMapping)
		if err == nil {
			h.wellKnownCache = mapping
		}
	}
	h.wellknownMapMu.Unlock()
	if err != nil {
		http.Error(w, "序列化错误", http.StatusInternalServerError)
		return
	}
	w.Header().Add("Content-Type", "application/json")
	w.Header().Add("Content-Length", strconv.Itoa(len(mapping)))
	w.Write(mapping)
}

// AddProtocolMeta 添加协议元数据
// 参数:
//   - p: 协议ID
//   - protocolMeta: 协议元数据
func (h *WellKnownHandler) AddProtocolMeta(p protocol.ID, protocolMeta ProtocolMeta) {
	h.wellknownMapMu.Lock()
	if h.wellKnownMapping == nil {
		h.wellKnownMapping = make(map[protocol.ID]ProtocolMeta)
	}
	h.wellKnownMapping[p] = protocolMeta
	h.wellKnownCache = nil
	h.wellknownMapMu.Unlock()
}

// RemoveProtocolMeta 移除协议元数据
// 参数:
//   - p: 要移除的协议ID
func (h *WellKnownHandler) RemoveProtocolMeta(p protocol.ID) {
	h.wellknownMapMu.Lock()
	if h.wellKnownMapping != nil {
		delete(h.wellKnownMapping, p)
	}
	h.wellKnownCache = nil
	h.wellknownMapMu.Unlock()
}

// Host 是一个用于 HTTP 语义的请求/响应的 dep2p 主机。
// 这与面向流的主机(如核心 host.Host 接口)不同。
// 其零值(&Host{})可用。不要按值复制。
// 参见示例了解用法。
//
// 警告:这是实验性的。API 可能会发生变化。
type Host struct {
	// StreamHost 是用于在 dep2p 流上进行 HTTP 通信的基于流的 dep2p 主机,可以为 nil
	StreamHost host.Host
	// ListenAddrs 是要监听的请求地址。多地址必须是有效的 HTTP(s) 多地址。
	// 仅支持 HTTP 传输的多地址(必须以 /http 或 /https 结尾)。
	ListenAddrs []ma.Multiaddr
	// TLSConfig 是服务器使用的 TLS 配置
	TLSConfig *tls.Config
	// InsecureAllowHTTP 表示是否允许服务器通过 TCP 提供未加密的 HTTP 请求
	InsecureAllowHTTP bool
	// ServeMux 是服务器用于处理请求的 http.ServeMux。如果为 nil,将创建一个新的 serve mux。
	// 用户可以手动向此 mux 添加处理程序,而不是使用 `SetHTTPHandler`,但如果这样做,他们也应该更新 WellKnownHandler 的协议映射。
	ServeMux           *http.ServeMux
	initializeServeMux sync.Once

	// DefaultClientRoundTripper 是客户端在通过 HTTP 传输发出请求时使用的默认 http.RoundTripper。
	// 这必须是 `*http.Transport` 类型,以便可以克隆传输并配置 `TLSClientConfig` 字段。
	// 如果未设置,将在首次使用时创建一个新的 `http.Transport`。
	DefaultClientRoundTripper *http.Transport

	// WellKnownHandler 是 well-known 资源的 http 处理程序。
	// 它负责与其他节点共享此节点的协议元数据。
	// 用户只有在使用预先存在路由设置自己的 ServeMux 时才需要关心这个。
	// 默认情况下,当用户调用 `SetHTTPHandler` 或 `SetHTTPHandlerAtPath` 时,新协议会被添加到这里。
	WellKnownHandler WellKnownHandler

	// EnableCompatibilityWithLegacyWellKnownEndpoint 允许与旧版本的规范兼容,该规范将 well-known 资源定义为: .well-known/dep2p。
	// 对于服务器,这意味着在旧路径和当前路径上都托管 well-known 资源。
	// 对于客户端,这意味着并行发出两个请求并选择第一个成功的请求。
	//
	// 长期来看,一旦足够多的用户升级到较新的 go-dep2p 版本,我们就可以删除所有这些代码,这应该被弃用。
	EnableCompatibilityWithLegacyWellKnownEndpoint bool

	// peerMetadata 是对等节点 well-known 协议映射的 LRU 缓存
	peerMetadata *lru.Cache[peer.ID, PeerMeta]
	// createHTTPTransport 用于以线程安全的方式延迟创建 httpTransport
	createHTTPTransport sync.Once
	// createDefaultClientRoundTripper 用于以线程安全的方式延迟创建默认客户端 round tripper
	createDefaultClientRoundTripper sync.Once
	httpTransport                   *httpTransport
}

// httpTransport 是 HTTP 传输的内部结构体
type httpTransport struct {
	// 监听的多地址列表
	listenAddrs []ma.Multiaddr
	// 网络监听器列表
	listeners []net.Listener
	// 用于关闭监听器的通道
	closeListeners chan struct{}
	// 等待监听器就绪的通道
	waitingForListeners chan struct{}
}

// newPeerMetadataCache 创建一个新的对等节点元数据缓存
// 返回:
//   - *lru.Cache[peer.ID, PeerMeta]: 新创建的 LRU 缓存
func newPeerMetadataCache() *lru.Cache[peer.ID, PeerMeta] {
	peerMetadata, err := lru.New[peer.ID, PeerMeta](peerMetadataLRUSize)
	if err != nil {
		// 只有在大小小于1时才会发生。我们确保不会这样做，所以这永远不会发生。
		panic(err)
	}
	return peerMetadata
}

// httpTransportInit 初始化 HTTP 传输
// 该方法确保 HTTP 传输只被初始化一次
func (h *Host) httpTransportInit() {
	h.createHTTPTransport.Do(func() {
		h.httpTransport = &httpTransport{
			closeListeners:      make(chan struct{}),
			waitingForListeners: make(chan struct{}),
		}
	})
}

// serveMuxInit 初始化 HTTP 请求多路复用器
// 该方法确保多路复用器只被初始化一次
func (h *Host) serveMuxInit() {
	h.initializeServeMux.Do(func() {
		if h.ServeMux == nil {
			h.ServeMux = http.NewServeMux()
		}
	})
}

// Addrs 返回所有监听地址
// 返回:
//   - []ma.Multiaddr: 监听地址列表
func (h *Host) Addrs() []ma.Multiaddr {
	h.httpTransportInit()
	<-h.httpTransport.waitingForListeners
	return h.httpTransport.listenAddrs
}

// PeerID 返回底层流主机的对等节点 ID，如果没有流主机则返回零值
// 返回:
//   - peer.ID: 对等节点 ID
func (h *Host) PeerID() peer.ID {
	if h.StreamHost != nil {
		return h.StreamHost.ID()
	}
	return ""
}

// ErrNoListeners 表示没有可用的监听器
var ErrNoListeners = errors.New("没有可监听的地址")

// setupListeners 设置所有监听器
// 参数:
//   - listenerErrCh: 用于传递监听器错误的通道
//
// 返回:
//   - error: 设置过程中的错误
func (h *Host) setupListeners(listenerErrCh chan error) error {
	for _, addr := range h.ListenAddrs {
		// 解析多地址
		parsedAddr, err := parseMultiaddr(addr)
		if err != nil {
			log.Debugf("解析多地址失败: %v", err)
			return err
		}
		// 解析主机地址
		ipaddr, err := net.ResolveIPAddr("ip", parsedAddr.host)
		if err != nil {
			log.Debugf("解析主机地址失败: %v", err)
			return err
		}

		host := ipaddr.String()
		// 创建 TCP 监听器
		l, err := net.Listen("tcp", host+":"+parsedAddr.port)
		if err != nil {
			log.Debugf("创建TCP监听器失败: %v", err)
			return err
		}
		h.httpTransport.listeners = append(h.httpTransport.listeners, l)

		// 获取解析后的端口
		_, port, err := net.SplitHostPort(l.Addr().String())
		if err != nil {
			log.Debugf("获取解析后的端口失败: %v", err)
			return err
		}

		var listenAddr ma.Multiaddr
		// 根据是否使用 HTTPS 和 SNI 构建监听地址
		if parsedAddr.useHTTPS && parsedAddr.sni != "" && parsedAddr.sni != host {
			listenAddr = ma.StringCast(fmt.Sprintf("/ip4/%s/tcp/%s/tls/sni/%s/http", host, port, parsedAddr.sni))
		} else {
			scheme := "http"
			if parsedAddr.useHTTPS {
				scheme = "https"
			}
			listenAddr = ma.StringCast(fmt.Sprintf("/ip4/%s/tcp/%s/%s", host, port, scheme))
		}

		// 根据是否使用 HTTPS 启动相应的服务
		if parsedAddr.useHTTPS {
			go func() {
				srv := http.Server{
					Handler:   h.ServeMux,
					TLSConfig: h.TLSConfig,
				}
				listenerErrCh <- srv.ServeTLS(l, "", "")
			}()
			h.httpTransport.listenAddrs = append(h.httpTransport.listenAddrs, listenAddr)
		} else if h.InsecureAllowHTTP {
			go func() {
				listenerErrCh <- http.Serve(l, h.ServeMux)
			}()
			h.httpTransport.listenAddrs = append(h.httpTransport.listenAddrs, listenAddr)
		} else {
			// 不提供不安全的 HTTP 服务
			log.Warnf("不在 %s 上提供不安全的 HTTP 服务。请优先使用 HTTPS 端点。", listenAddr)
		}
	}
	return nil
}

// Serve 启动 HTTP 传输监听器。总是返回一个非空错误。
// 如果没有监听器，返回 ErrNoListeners。
// 参数：无
// 返回值：error - 如果发生错误则返回错误信息
func (h *Host) Serve() error {
	// 确保每个地址都包含 /http 组件
	for _, addr := range h.ListenAddrs {
		_, isHTTP := normalizeHTTPMultiaddr(addr)
		if !isHTTP {
			log.Debugf("地址 %s 不包含 /http 或 /https 组件", addr)
			return fmt.Errorf("地址 %s 不包含 /http 或 /https 组件", addr)
		}
	}

	// 初始化 ServeMux
	h.serveMuxInit()
	// 为已知协议注册处理器
	h.ServeMux.Handle(WellKnownProtocols, &h.WellKnownHandler)
	// 如果启用了与旧版已知端点的兼容性，则注册旧版处理器
	if h.EnableCompatibilityWithLegacyWellKnownEndpoint {
		h.ServeMux.Handle(LegacyWellKnownProtocols, &h.WellKnownHandler)
	}

	// 初始化 HTTP 传输
	h.httpTransportInit()

	// 标记是否已关闭等待监听器
	closedWaitingForListeners := false
	defer func() {
		if !closedWaitingForListeners {
			close(h.httpTransport.waitingForListeners)
		}
	}()

	// 检查是否有可用的监听器
	if len(h.ListenAddrs) == 0 && h.StreamHost == nil {
		log.Debugf("没有可用的监听器")
		return ErrNoListeners
	}

	// 初始化监听器切片
	h.httpTransport.listeners = make([]net.Listener, 0, len(h.ListenAddrs)+1) // +1 用于 stream host

	// 计算 StreamHost 地址数量
	streamHostAddrsCount := 0
	if h.StreamHost != nil {
		streamHostAddrsCount = len(h.StreamHost.Addrs())
	}
	// 初始化监听地址切片
	h.httpTransport.listenAddrs = make([]ma.Multiaddr, 0, len(h.ListenAddrs)+streamHostAddrsCount)

	// 创建错误通道
	errCh := make(chan error)

	// 如果存在 StreamHost，则设置流监听器
	if h.StreamHost != nil {
		listener, err := streamHostListen(h.StreamHost)
		if err != nil {
			log.Debugf("设置流监听器失败: %v", err)
			return err
		}
		h.httpTransport.listeners = append(h.httpTransport.listeners, listener)
		h.httpTransport.listenAddrs = append(h.httpTransport.listenAddrs, h.StreamHost.Addrs()...)

		go func() {
			errCh <- http.Serve(listener, connectionCloseHeaderMiddleware(h.ServeMux))
		}()
	}

	// 定义关闭所有监听器的函数
	closeAllListeners := func() {
		for _, l := range h.httpTransport.listeners {
			l.Close()
		}
	}

	// 设置监听器
	err := h.setupListeners(errCh)
	if err != nil {
		closeAllListeners()
		log.Debugf("设置监听器失败: %v", err)
		return err
	}

	// 关闭等待监听器通道
	close(h.httpTransport.waitingForListeners)
	closedWaitingForListeners = true

	// 检查监听器和地址是否有效
	if len(h.httpTransport.listeners) == 0 || len(h.httpTransport.listenAddrs) == 0 {
		closeAllListeners()
		log.Debugf("没有可用的监听器")
		return ErrNoListeners
	}

	// 等待错误或关闭信号
	expectedErrCount := len(h.httpTransport.listeners)
	select {
	case <-h.httpTransport.closeListeners:
		err = http.ErrServerClosed
	case err = <-errCh:
		expectedErrCount--
	}

	// 关闭所有监听器并等待所有错误
	closeAllListeners()
	for i := 0; i < expectedErrCount; i++ {
		<-errCh
	}
	close(errCh)

	return err
}

// Close 关闭 HTTP 主机
// 参数：无
// 返回值：error - 如果发生错误则返回错误信息
func (h *Host) Close() error {
	h.httpTransportInit()
	close(h.httpTransport.closeListeners)
	return nil
}

// SetHTTPHandler 为给定协议设置 HTTP 处理器。自动管理已知资源映射。
// 对处理器调用 http.StripPrefix，因此处理器不会感知其前缀路径。
// 参数：
//   - p: protocol.ID - 协议标识符
//   - handler: http.Handler - HTTP 处理器
func (h *Host) SetHTTPHandler(p protocol.ID, handler http.Handler) {
	h.SetHTTPHandlerAtPath(p, string(p), handler)
}

// SetHTTPHandlerAtPath 使用给定路径为指定协议设置 HTTP 处理器。自动管理已知资源映射。
// 对处理器调用 http.StripPrefix，因此处理器不会感知其前缀路径。
// 参数：
//   - p: protocol.ID - 协议标识符
//   - path: string - 处理器路径
//   - handler: http.Handler - HTTP 处理器
func (h *Host) SetHTTPHandlerAtPath(p protocol.ID, path string, handler http.Handler) {
	// 确保路径以斜杠结尾
	if path == "" || path[len(path)-1] != '/' {
		path += "/"
	}
	// 添加协议元数据
	h.WellKnownHandler.AddProtocolMeta(p, ProtocolMeta{Path: path})
	// 初始化 ServeMux
	h.serveMuxInit()
	// 注册处理器
	h.ServeMux.Handle(path, http.StripPrefix(strings.TrimSuffix(path, "/"), handler))
}

// PeerMetadataGetter 允许 RoundTripper 实现特定的方式来缓存对等节点的协议映射
type PeerMetadataGetter interface {
	// GetPeerMetadata 获取对等节点的元数据
	// 返回:
	//   - PeerMeta: 对等节点的元数据
	//   - error: 错误信息
	GetPeerMetadata() (PeerMeta, error)
}

// streamRoundTripper 实现了基于流的 HTTP 传输
type streamRoundTripper struct {
	// server 是目标服务器的对等节点 ID
	server peer.ID
	// skipAddAddrs 如果为 true,则不会将服务器地址添加到 peerstore
	// 仅在创建结构体时设置
	skipAddAddrs bool
	// addrsAdded 确保地址只添加一次
	addrsAdded sync.Once
	// serverAddrs 是服务器的多地址列表
	serverAddrs []ma.Multiaddr
	// h 是 dep2p 主机
	h host.Host
	// httpHost 是 HTTP 主机
	httpHost *Host
}

// streamReadCloser 包装了一个 io.ReadCloser,并在关闭时关闭底层流
// (同时也会关闭被包装的 ReadCloser)。这是必要的,因为我们有两个需要关闭的东西:
// body 和流。流不会被 body 自动关闭,这从 http.ReadResponse 接受 bufio.Reader
// 这一事实可以看出来。
type streamReadCloser struct {
	io.ReadCloser
	s network.Stream
}

// Close 实现了 io.Closer 接口
// 返回:
//   - error: 关闭时的错误信息
func (s *streamReadCloser) Close() error {
	// 关闭流
	s.s.Close()
	// 关闭读取器
	return s.ReadCloser.Close()
}

// GetPeerMetadata 实现了 PeerMetadataGetter 接口
// 返回:
//   - PeerMeta: 对等节点的元数据
//   - error: 错误信息
func (rt *streamRoundTripper) GetPeerMetadata() (PeerMeta, error) {
	// 创建上下文
	ctx := context.Background()
	// 设置超时时间
	ctx, cancel := context.WithDeadline(ctx, time.Now().Add(WellKnownRequestTimeout))
	defer cancel()
	// 获取并存储对等节点元数据
	return rt.httpHost.getAndStorePeerMetadata(ctx, rt, rt.server)
}

// RoundTrip 实现了 http.RoundTripper 接口
// 参数:
//   - r: *http.Request - HTTP 请求
//
// 返回:
//   - *http.Response: HTTP 响应
//   - error: 错误信息
func (rt *streamRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	// 如果需要,添加服务器地址
	if !rt.skipAddAddrs {
		rt.addrsAdded.Do(func() {
			if len(rt.serverAddrs) > 0 {
				// 将地址添加到 peerstore
				rt.h.Peerstore().AddAddrs(rt.server, rt.serverAddrs, peerstore.TempAddrTTL)
			}
			// 清理地址列表
			rt.serverAddrs = nil
		})
	}

	// 创建新的流
	s, err := rt.h.NewStream(r.Context(), rt.server, ProtocolIDForMultistreamSelect)
	if err != nil {
		log.Debugf("创建新流失败: %v", err)
		return nil, err
	}

	// 添加 connection: close 头以确保响应后关闭流
	r.Header.Add("connection", "close")

	// 启动 goroutine 写入请求
	go func() {
		defer s.CloseWrite()
		r.Write(s)
		if r.Body != nil {
			r.Body.Close()
		}
	}()

	// 设置读取超时
	if deadline, ok := r.Context().Deadline(); ok {
		s.SetReadDeadline(deadline)
	}

	// 读取响应
	resp, err := http.ReadResponse(bufio.NewReader(s), r)
	if err != nil {
		s.Close()
		log.Debugf("读取响应失败: %v", err)
		return nil, err
	}
	// 包装响应体
	resp.Body = &streamReadCloser{resp.Body, s}

	// 处理重定向 URL
	locUrl, err := resp.Location()
	if err == nil {
		// 检查响应中的 Location URL 是否为多地址 URI 且是相对路径
		// 如果是多地址 URI 且为相对路径,需要将其转换为绝对多地址 URI
		// 这样下一个请求就能知道如何访问正确的端点
		// 例如:
		// - 相对路径: /path/to/resource
		// - 绝对路径: multiaddr:/ip4/1.2.3.4/tcp/8080/path/to/resource
		if locUrl.Scheme == "multiaddr" && resp.Request.URL.Scheme == "multiaddr" {
			// 检查是否为相对 URI 并转换为绝对 URI
			u, err := relativeMultiaddrURIToAbs(resp.Request.URL, locUrl)
			if err == nil {
				// 这是一个相对 URI 并且我们成功将其转换为绝对 URI
				// 更新 Location 响应头为绝对多地址 URI 格式
				// 例如: 从 /path/to/resource 转换为 multiaddr:/ip4/1.2.3.4/tcp/8080/path/to/resource
				resp.Header.Set("Location", u.String())
			}
		}
	}

	return resp, nil
}

// errNotRelative 表示不是相对路径的错误
var errNotRelative = errors.New("不是相对路径")

// relativeMultiaddrURIToAbs 将相对多地址 URI 转换为绝对 URI
// 当服务器返回重定向的相对 URI 时,此函数很有用。
// 它允许重定向后的请求能够到达正确的服务器。
//
// 例如:
// - 原始 URL: multiaddr:/ip4/1.2.3.4/tcp/8080/http/foo
// - 相对 URL: /bar
// - 转换后: multiaddr:/ip4/1.2.3.4/tcp/8080/http/bar
//
// 判断是否为相对 URI 的方法:
//   - 非相对 URI 格式如 "multiaddr:/ip4/1.2.3.4/tcp/9899" 被 Go url 包解析时 url.OmitHost 为 true
//   - 相对 URI(如 /here-instead)会继承 multiaddr scheme,但 url.OmitHost 为 false,
//     并且会被格式化为类似 multiaddr://here-instead 的形式
//
// 参数:
//   - original: *url.URL - 原始 URL,包含完整的多地址信息
//   - relative: *url.URL - 相对 URL,通常只包含路径部分
//
// 返回:
//   - *url.URL: 转换后的绝对 URL,包含完整的多地址信息
//   - error: 如果输入无效或转换失败则返回错误
func relativeMultiaddrURIToAbs(original *url.URL, relative *url.URL) (*url.URL, error) {
	// 检查是否为相对 URI
	// 如果 OmitHost 为 true,说明不是相对路径,而是完整的多地址 URI
	if relative.OmitHost {
		// 不是相对路径(至少无法判断),无法处理
		log.Debugf("不是相对路径")
		return nil, errNotRelative
	}

	// 获取原始路径,优先使用 RawPath
	originalStr := original.RawPath
	if originalStr == "" {
		originalStr = original.Path
	}

	// 解析原始多地址
	originalMa, err := ma.NewMultiaddr(originalStr)
	if err != nil {
		log.Debugf("原始 URI 不是有效的多地址: %v", err)
		return nil, err
	}

	// 创建相对路径的多地址组件
	relativePathComponent, err := ma.NewComponent("http-path", relative.Path)
	if err != nil {
		log.Debugf("相对路径不是有效的 http-path: %v", err)
		return nil, err
	}

	// 移除原始多地址中的路径组件,并添加新的路径组件
	withoutPath, _ := ma.SplitFunc(originalMa, func(c ma.Component) bool {
		return c.Protocol().Code == ma.P_HTTP_PATH
	})
	withNewPath := withoutPath.Encapsulate(relativePathComponent)
	return url.Parse("multiaddr:" + withNewPath.String())
}

// roundTripperForSpecificServer 是针对特定服务器的 http.RoundTripper
// 仍然重用底层 RoundTripper 进行请求
// 底层 RoundTripper 必须是 HTTP Transport
type roundTripperForSpecificServer struct {
	http.RoundTripper
	// ownRoundtripper 表示是否拥有 RoundTripper
	ownRoundtripper bool
	// httpHost 是 HTTP 主机
	httpHost *Host
	// server 是目标服务器的对等节点 ID
	server peer.ID
	// targetServerAddr 是目标服务器地址
	targetServerAddr string
	// sni 是服务器名称指示
	sni string
	// scheme 是协议方案
	scheme string
	// cachedProtos 是缓存的协议映射
	cachedProtos PeerMeta
}

// GetPeerMetadata 实现了 PeerMetadataGetter 接口
// 返回:
//   - PeerMeta: 对等节点的元数据
//   - error: 错误信息
func (rt *roundTripperForSpecificServer) GetPeerMetadata() (PeerMeta, error) {
	// 检查是否已有缓存
	if rt.cachedProtos != nil {
		return rt.cachedProtos, nil
	}

	// 检查底层 RoundTripper 是否实现了 GetPeerMetadata
	if g, ok := rt.RoundTripper.(PeerMetadataGetter); ok {
		wk, err := g.GetPeerMetadata()
		if err == nil {
			rt.cachedProtos = wk
			return wk, nil
		}
	}

	// 创建上下文
	ctx := context.Background()
	ctx, cancel := context.WithDeadline(ctx, time.Now().Add(WellKnownRequestTimeout))
	defer cancel()
	// 获取并存储元数据
	wk, err := rt.httpHost.getAndStorePeerMetadata(ctx, rt, rt.server)
	if err == nil {
		rt.cachedProtos = wk
		return wk, nil
	}
	return wk, err
}

// RoundTrip 实现了 http.RoundTripper 接口
// 参数:
//   - r: *http.Request - HTTP 请求
//
// 返回:
//   - *http.Response: HTTP 响应
//   - error: 错误信息
func (rt *roundTripperForSpecificServer) RoundTrip(r *http.Request) (*http.Response, error) {
	// 验证请求的协议和主机
	if (r.URL.Scheme != "" && r.URL.Scheme != rt.scheme) || (r.URL.Host != "" && r.URL.Host != rt.targetServerAddr) {
		log.Debugf("此传输仅用于 %s://%s 的请求", rt.scheme, rt.targetServerAddr)
		return nil, fmt.Errorf("此传输仅用于 %s://%s 的请求", rt.scheme, rt.targetServerAddr)
	}
	// 设置请求的协议和主机
	r.URL.Scheme = rt.scheme
	r.URL.Host = rt.targetServerAddr
	r.Host = rt.sni
	return rt.RoundTripper.RoundTrip(r)
}

// CloseIdleConnections 关闭空闲连接
func (rt *roundTripperForSpecificServer) CloseIdleConnections() {
	if rt.ownRoundtripper {
		// 安全地关闭空闲连接,因为我们拥有 RoundTripper
		type closeIdler interface {
			CloseIdleConnections()
		}
		if tr, ok := rt.RoundTripper.(closeIdler); ok {
			tr.CloseIdleConnections()
		}
	}
}

// namespacedRoundTripper 是一个为所有请求添加路径前缀的 RoundTripper
// 用于将请求命名空间限定到特定协议
type namespacedRoundTripper struct {
	http.RoundTripper
	// protocolPrefix 是协议前缀
	protocolPrefix string
	// protocolPrefixRaw 是原始协议前缀
	protocolPrefixRaw string
}

// GetPeerMetadata 实现了 PeerMetadataGetter 接口
// 返回:
//   - PeerMeta: 对等节点的元数据
//   - error: 错误信息
func (rt *namespacedRoundTripper) GetPeerMetadata() (PeerMeta, error) {
	if g, ok := rt.RoundTripper.(PeerMetadataGetter); ok {
		return g.GetPeerMetadata()
	}

	return nil, fmt.Errorf("无法获取对等节点协议映射。内部 RoundTripper 未实现 GetPeerMetadata")
}

// RoundTrip 实现了 http.RoundTripper 接口
// 参数:
//   - r: HTTP 请求
//
// 返回:
//   - *http.Response: HTTP 响应
//   - error: 错误信息
func (rt *namespacedRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	// 如果请求路径不以协议前缀开头,则添加前缀
	if !strings.HasPrefix(r.URL.Path, rt.protocolPrefix) {
		r.URL.Path = rt.protocolPrefix + r.URL.Path
	}
	// 如果原始请求路径不以原始协议前缀开头,则添加前缀
	if !strings.HasPrefix(r.URL.RawPath, rt.protocolPrefixRaw) {
		r.URL.RawPath = rt.protocolPrefixRaw + r.URL.Path
	}

	// 调用内部 RoundTripper 处理请求
	return rt.RoundTripper.RoundTrip(r)
}

// NamespaceRoundTripper 返回一个限定在指定服务器上指定协议的 http.RoundTripper
// 参数:
//   - roundtripper: HTTP 请求处理器
//   - p: 协议 ID
//   - server: 对等节点 ID
//
// 返回:
//   - *namespacedRoundTripper: 命名空间限定的 RoundTripper
//   - error: 错误信息
func (h *Host) NamespaceRoundTripper(roundtripper http.RoundTripper, p protocol.ID, server peer.ID) (*namespacedRoundTripper, error) {
	// 创建带超时的上下文
	ctx := context.Background()
	ctx, cancel := context.WithDeadline(ctx, time.Now().Add(WellKnownRequestTimeout))
	defer cancel()

	// 获取并存储对等节点元数据
	protos, err := h.getAndStorePeerMetadata(ctx, roundtripper, server)
	if err != nil {
		return &namespacedRoundTripper{}, err
	}

	// 检查协议是否存在
	v, ok := protos[p]
	if !ok {
		log.Debugf("服务器 %s 不支持协议 %s", server, p)
		return &namespacedRoundTripper{}, fmt.Errorf("服务器 %s 不支持协议 %s", server, p)
	}

	// 处理路径
	path := v.Path
	if path[len(path)-1] == '/' {
		// 去除末尾斜杠,因为通常请求路径以前导斜杠开始
		path = path[:len(path)-1]
	}

	// 解析路径
	u, err := url.Parse(path)
	if err != nil {
		log.Debugf("服务器 %s 的协议 %s 的路径 %s 无效", server, p, v.Path)
		return &namespacedRoundTripper{}, fmt.Errorf("服务器 %s 的协议 %s 的路径 %s 无效", server, p, v.Path)
	}

	// 返回命名空间限定的 RoundTripper
	return &namespacedRoundTripper{
		RoundTripper:      roundtripper,
		protocolPrefix:    u.Path,
		protocolPrefixRaw: u.RawPath,
	}, nil
}

// NamespacedClient 返回一个限定在指定服务器上指定协议的 http.Client
// 每次调用都会创建一个新的 RoundTripper
// 如果需要创建多个命名空间客户端,建议直接创建 RoundTripper 并自行命名空间限定,然后基于命名空间 RoundTripper 创建客户端
// 参数:
//   - p: 协议 ID
//   - server: 对等节点地址信息
//   - opts: RoundTripper 选项
//
// 返回:
//   - http.Client: HTTP 客户端
//   - error: 错误信息
func (h *Host) NamespacedClient(p protocol.ID, server peer.AddrInfo, opts ...RoundTripperOption) (http.Client, error) {
	// 创建受限的 RoundTripper
	rt, err := h.NewConstrainedRoundTripper(server, opts...)
	if err != nil {
		log.Debugf("创建受限的 RoundTripper 失败: %v", err)
		return http.Client{}, err
	}

	// 创建命名空间限定的 RoundTripper
	nrt, err := h.NamespaceRoundTripper(rt, p, server.ID)
	if err != nil {
		log.Debugf("创建命名空间限定的 RoundTripper 失败: %v", err)
		return http.Client{}, err
	}

	// 返回使用命名空间限定 RoundTripper 的客户端
	return http.Client{Transport: nrt}, nil
}

// initDefaultRT 初始化默认的 RoundTripper
// 使用 sync.Once 确保只初始化一次
func (h *Host) initDefaultRT() {
	h.createDefaultClientRoundTripper.Do(func() {
		if h.DefaultClientRoundTripper == nil {
			// 尝试将默认传输转换为 http.Transport
			tr, ok := http.DefaultTransport.(*http.Transport)
			if ok {
				// 如果转换成功,使用默认传输
				h.DefaultClientRoundTripper = tr
			} else {
				// 否则创建新的传输
				h.DefaultClientRoundTripper = &http.Transport{}
			}
		}
	})
}

// RoundTrip 实现 http.RoundTripper 接口
// 允许将 Host 用作 http.Client 的传输层
// 参数:
//   - r: *http.Request - HTTP 请求对象
//
// 返回:
//   - *http.Response: HTTP 响应对象
//   - error: 错误信息
//
// 处理流程:
// 1. 根据 URL scheme 选择不同的处理方式:
//   - http/https: 使用默认传输
//   - multiaddr: 继续处理 multiaddr 方案
//   - 其他: 返回不支持的协议错误
//
// 2. 对于 multiaddr:
//   - 解析 multiaddr URL 并标准化
//   - 如果是 HTTP 请求:
//   - 构建标准 URL
//   - 使用默认或自定义传输处理请求
//   - 如果是基于流的请求:
//   - 验证 StreamHost 和对等节点 ID
//   - 将地址添加到对等节点存储
//   - 创建流传输并处理请求
func (h *Host) RoundTrip(r *http.Request) (*http.Response, error) {
	// 根据 URL scheme 选择不同的处理方式
	switch r.URL.Scheme {
	case "http", "https":
		// 对于标准 HTTP/HTTPS 请求,使用默认传输
		h.initDefaultRT()
		return h.DefaultClientRoundTripper.RoundTrip(r)
	case "multiaddr":
		// 继续处理 multiaddr 方案
		break
	default:
		return nil, fmt.Errorf("不支持的协议方案 %s", r.URL.Scheme)
	}

	// 解析 multiaddr URL
	addr, err := ma.NewMultiaddr(r.URL.String()[len("multiaddr:"):])
	if err != nil {
		return nil, err
	}
	// 标准化 HTTP multiaddr
	addr, isHTTP := normalizeHTTPMultiaddr(addr)
	// 解析 multiaddr 各个组件
	parsed, err := parseMultiaddr(addr)
	if err != nil {
		return nil, err
	}

	if isHTTP {
		// 处理 HTTP 请求
		scheme := "http"
		if parsed.useHTTPS {
			scheme = "https"
		}
		// 构建标准 URL
		u := url.URL{
			Scheme: scheme,
			Host:   parsed.host + ":" + parsed.port,
			Path:   parsed.httpPath,
		}
		r.URL = &u

		h.initDefaultRT()
		rt := h.DefaultClientRoundTripper
		if parsed.sni != parsed.host {
			// 如果 SNI 与主机名不同(例如使用 IP 地址但指定 SNI)
			// 需要创建自己的传输来支持这种情况
			// 注意:如果经常使用此代码路径,可以维护这些传输的池
			// 目前仅为完整性而提供,预计不会经常使用
			rt = rt.Clone()
			rt.TLSClientConfig.ServerName = parsed.sni
		}

		return rt.RoundTrip(r)
	}

	// 处理基于流的请求
	if h.StreamHost == nil {
		log.Debugf("无法通过流执行 HTTP。缺少 StreamHost")
		return nil, fmt.Errorf("无法通过流执行 HTTP。缺少 StreamHost")
	}

	if parsed.peer == "" {
		log.Debugf("multiaddr 中没有对等节点 ID")
		return nil, fmt.Errorf("multiaddr 中没有对等节点 ID")
	}
	// 分离 HTTP 路径组件
	withoutHTTPPath, _ := ma.SplitFunc(addr, func(c ma.Component) bool {
		return c.Protocol().Code == ma.P_HTTP_PATH
	})
	// 将地址添加到对等节点存储
	h.StreamHost.Peerstore().AddAddrs(parsed.peer, []ma.Multiaddr{withoutHTTPPath}, peerstore.TempAddrTTL)

	// 设置 URL 的 Opaque 字段为 http-path
	r.URL.Opaque = parsed.httpPath
	if r.Host == "" {
		// 如果主机未设置,则填充主机
		r.Host = parsed.host + ":" + parsed.port
	}
	// 创建流传输
	srt := streamRoundTripper{
		server:       parsed.peer,
		skipAddAddrs: true,
		httpHost:     h,
		h:            h.StreamHost,
	}
	return srt.RoundTrip(r)
}

// NewConstrainedRoundTripper 返回一个可以向指定服务器发送 HTTP 请求的 http.RoundTripper
// 它可以使用 HTTP 传输或基于流的传输。允许传入空的 server.ID
// 如果服务器有多个地址,它将使用以下规则选择最佳传输(流与标准 HTTP):
//   - 如果设置了 PreferHTTPTransport,使用 HTTP 传输
//   - 如果设置了 ServerMustAuthenticatePeerID,使用流传输,因为 HTTP 传输尚不支持对等节点 ID 认证
//   - 如果已经在流传输上有连接,使用该连接
//   - 否则,如果两种传输都可用,使用 HTTP 传输
//
// 参数:
//   - server: 对等节点地址信息
//   - opts: RoundTripper 选项
//
// 返回:
//   - http.RoundTripper: HTTP 传输
//   - error: 错误信息
func (h *Host) NewConstrainedRoundTripper(server peer.AddrInfo, opts ...RoundTripperOption) (http.RoundTripper, error) {
	// 应用选项
	options := roundTripperOpts{}
	for _, o := range opts {
		options = o(options)
	}

	// 验证对等节点认证要求
	if options.serverMustAuthenticatePeerID && server.ID == "" {
		log.Debugf("要求服务器认证对等节点 ID,但未提供对等节点 ID")
		return nil, fmt.Errorf("要求服务器认证对等节点 ID,但未提供对等节点 ID")
	}

	// 分离 HTTP 和非 HTTP 地址
	httpAddrs := make([]ma.Multiaddr, 0, 1)
	nonHTTPAddrs := make([]ma.Multiaddr, 0, len(server.Addrs))

	firstAddrIsHTTP := false

	// 遍历并分类地址
	for i, addr := range server.Addrs {
		addr, isHTTP := normalizeHTTPMultiaddr(addr)
		if isHTTP {
			if i == 0 {
				firstAddrIsHTTP = true
			}
			httpAddrs = append(httpAddrs, addr)
		} else {
			nonHTTPAddrs = append(nonHTTPAddrs, addr)
		}
	}

	// 检查是否存在到此对等节点的连接
	existingStreamConn := false
	if server.ID != "" && h.StreamHost != nil {
		existingStreamConn = len(h.StreamHost.Network().ConnsToPeer(server.ID)) > 0
	}

	// 选择合适的传输方式
	if !options.serverMustAuthenticatePeerID && len(httpAddrs) > 0 && (options.preferHTTPTransport || (firstAddrIsHTTP && !existingStreamConn)) {
		// 使用 HTTP 传输
		parsed, err := parseMultiaddr(httpAddrs[0])
		if err != nil {
			log.Debugf("解析多地址失败: %v", err)
			return nil, err
		}
		scheme := "http"
		if parsed.useHTTPS {
			scheme = "https"
		}

		h.initDefaultRT()
		rt := h.DefaultClientRoundTripper
		ownRoundtripper := false
		if parsed.sni != parsed.host {
			// SNI 与主机不同时创建新的传输
			rt = rt.Clone()
			rt.TLSClientConfig.ServerName = parsed.sni
			ownRoundtripper = true
		}

		return &roundTripperForSpecificServer{
			RoundTripper:     rt,
			ownRoundtripper:  ownRoundtripper,
			httpHost:         h,
			server:           server.ID,
			targetServerAddr: parsed.host + ":" + parsed.port,
			sni:              parsed.sni,
			scheme:           scheme,
		}, nil
	}

	// 使用基于流的传输
	if h.StreamHost == nil {
		log.Debugf("无法使用 HTTP 传输(无地址或需要对等节点 ID 认证),且未提供流主机")
		return nil, fmt.Errorf("无法使用 HTTP 传输(无地址或需要对等节点 ID 认证),且未提供流主机")
	}
	if !existingStreamConn {
		if server.ID == "" {
			log.Debugf("无法使用 HTTP 传输,且未提供服务器对等节点 ID")
			return nil, fmt.Errorf("无法使用 HTTP 传输,且未提供服务器对等节点 ID")
		}
	}

	return &streamRoundTripper{h: h.StreamHost, server: server.ID, serverAddrs: nonHTTPAddrs, httpHost: h}, nil
}

// explodedMultiaddr 是解析后的多地址结构体
type explodedMultiaddr struct {
	useHTTPS bool    // 是否使用 HTTPS
	host     string  // 主机地址
	port     string  // 端口号
	sni      string  // SNI 服务器名称
	httpPath string  // HTTP 路径
	peer     peer.ID // 对等节点 ID
}

// parseMultiaddr 解析多地址
// 参数:
//   - addr: ma.Multiaddr - 要解析的多地址
//
// 返回:
//   - explodedMultiaddr: 解析后的多地址结构体
//   - error: 错误信息
func parseMultiaddr(addr ma.Multiaddr) (explodedMultiaddr, error) {
	// 初始化输出结构体
	out := explodedMultiaddr{}
	var err error
	// 遍历多地址的每个组件
	ma.ForEach(addr, func(c ma.Component) bool {
		switch c.Protocol().Code {
		case ma.P_IP4, ma.P_IP6, ma.P_DNS, ma.P_DNS4, ma.P_DNS6:
			// 解析主机地址
			out.host = c.Value()
		case ma.P_TCP, ma.P_UDP:
			// 解析端口号
			out.port = c.Value()
		case ma.P_TLS, ma.P_HTTPS:
			// 标记使用 HTTPS
			out.useHTTPS = true
		case ma.P_SNI:
			// 解析 SNI
			out.sni = c.Value()
		case ma.P_HTTP_PATH:
			// 解析 HTTP 路径
			out.httpPath, err = url.QueryUnescape(c.Value())
			if err == nil && out.httpPath[0] != '/' {
				out.httpPath = "/" + out.httpPath
			}
		case ma.P_P2P:
			// 解析对等节点 ID
			out.peer, err = peer.Decode(c.Value())
		}

		// 如果有错误则停止遍历,否则继续遍历所有组件(以防这是一个电路地址)
		return err == nil
	})

	// 如果使用 HTTPS 但未指定 SNI,则使用主机名作为 SNI
	if out.useHTTPS && out.sni == "" {
		out.sni = out.host
	}

	// 如果未指定 HTTP 路径,则使用根路径
	if out.httpPath == "" {
		out.httpPath = "/"
	}
	return out, err
}

// 创建 HTTP 和 TLS 组件
var httpComponent, _ = ma.NewComponent("http", "")
var tlsComponent, _ = ma.NewComponent("tls", "")

// normalizeHTTPMultiaddr 将 HTTPS 多地址转换为 TLS/HTTP 多地址
// 参数:
//   - addr: ma.Multiaddr - 要转换的多地址
//
// 返回:
//   - ma.Multiaddr: 转换后的多地址
//   - bool: 输入多地址是否包含 HTTP 或 HTTPS 组件
func normalizeHTTPMultiaddr(addr ma.Multiaddr) (ma.Multiaddr, bool) {
	isHTTPMultiaddr := false
	// 分割多地址
	beforeHTTPS, afterIncludingHTTPS := ma.SplitFunc(addr, func(c ma.Component) bool {
		if c.Protocol().Code == ma.P_HTTP {
			isHTTPMultiaddr = true
		}

		if c.Protocol().Code == ma.P_HTTPS {
			isHTTPMultiaddr = true
			return true
		}
		return false
	})
	if beforeHTTPS == nil || !isHTTPMultiaddr {
		return addr, false
	}

	if afterIncludingHTTPS == nil {
		// 没有 HTTPS 组件,返回原始地址
		return addr, isHTTPMultiaddr
	}

	// 分离 HTTPS 组件后的部分
	_, afterHTTPS := ma.SplitFirst(afterIncludingHTTPS)
	if afterHTTPS == nil {
		return ma.Join(beforeHTTPS, tlsComponent, httpComponent), isHTTPMultiaddr
	}

	return ma.Join(beforeHTTPS, tlsComponent, httpComponent, afterHTTPS), isHTTPMultiaddr
}

// getAndStorePeerMetadata 在已知映射中查找协议路径并返回
// 仅当提供服务器 ID 时才存储对等节点的协议映射
// 参数:
//   - ctx: context.Context - 上下文
//   - roundtripper: http.RoundTripper - HTTP 传输器
//   - server: peer.ID - 服务器 ID
//
// 返回:
//   - PeerMeta: 对等节点元数据
//   - error: 错误信息
func (h *Host) getAndStorePeerMetadata(ctx context.Context, roundtripper http.RoundTripper, server peer.ID) (PeerMeta, error) {
	// 初始化元数据缓存
	if h.peerMetadata == nil {
		h.peerMetadata = newPeerMetadataCache()
	}
	// 检查缓存中是否存在
	if meta, ok := h.peerMetadata.Get(server); server != "" && ok {
		return meta, nil
	}

	var meta PeerMeta
	var err error
	// 处理旧版兼容性
	if h.EnableCompatibilityWithLegacyWellKnownEndpoint {
		type metaAndErr struct {
			m   PeerMeta
			err error
		}
		// 创建响应通道
		legacyRespCh := make(chan metaAndErr, 1)
		wellKnownRespCh := make(chan metaAndErr, 1)
		ctx, cancel := context.WithCancel(ctx)
		// 并行请求旧版和新版端点
		go func() {
			meta, err := requestPeerMeta(ctx, roundtripper, LegacyWellKnownProtocols)
			legacyRespCh <- metaAndErr{meta, err}
		}()
		go func() {
			meta, err := requestPeerMeta(ctx, roundtripper, WellKnownProtocols)
			wellKnownRespCh <- metaAndErr{meta, err}
		}()
		// 选择第一个成功的响应
		select {
		case resp := <-legacyRespCh:
			if resp.err != nil {
				resp = <-wellKnownRespCh
			}
			meta, err = resp.m, resp.err
		case resp := <-wellKnownRespCh:
			if resp.err != nil {
				legacyResp := <-legacyRespCh
				if legacyResp.err != nil {
					// 如果两个端点都出错,返回新版端点的错误
					meta, err = resp.m, resp.err
				} else {
					meta, err = legacyResp.m, legacyResp.err
				}
			} else {
				meta, err = resp.m, resp.err
			}
		}
		cancel()
	} else {
		meta, err = requestPeerMeta(ctx, roundtripper, WellKnownProtocols)
	}
	if err != nil {
		log.Debugf("请求对等节点元数据失败: %v", err)
		return nil, err
	}

	// 存储元数据
	if server != "" {
		h.peerMetadata.Add(server, meta)
	}

	return meta, nil
}

// requestPeerMeta 请求对等节点元数据
// 参数:
//   - ctx: context.Context - 上下文
//   - roundtripper: http.RoundTripper - HTTP 传输器
//   - wellKnownResource: string - 已知资源路径
//
// 返回:
//   - PeerMeta: 对等节点元数据
//   - error: 错误信息
func requestPeerMeta(ctx context.Context, roundtripper http.RoundTripper, wellKnownResource string) (PeerMeta, error) {
	// 创建请求
	req, err := http.NewRequestWithContext(ctx, "GET", wellKnownResource, nil)
	if err != nil {
		log.Debugf("创建请求失败: %v", err)
		return nil, err
	}
	req.Header.Set("Accept", "application/json")

	// 发送请求
	client := http.Client{Transport: roundtripper}
	resp, err := client.Do(req)
	if err != nil {
		log.Debugf("发送请求失败: %v", err)
		return nil, err
	}
	defer resp.Body.Close()

	// 检查状态码
	if resp.StatusCode != http.StatusOK {
		log.Debugf("意外的状态码: %d", resp.StatusCode)
		return nil, fmt.Errorf("意外的状态码: %d", resp.StatusCode)
	}

	// 解析响应
	meta := PeerMeta{}
	err = json.NewDecoder(&io.LimitedReader{
		R: resp.Body,
		N: peerMetadataLimit,
	}).Decode(&meta)
	if err != nil {
		log.Debugf("解析响应失败: %v", err)
		return nil, err
	}

	return meta, nil
}

// SetPeerMetadata 将对等节点的协议元数据添加到 HTTP 主机
// 当您有对等节点协议映射的带外知识时很有用
// 参数:
//   - server: peer.ID - 服务器 ID
//   - meta: PeerMeta - 元数据
func (h *Host) SetPeerMetadata(server peer.ID, meta PeerMeta) {
	if h.peerMetadata == nil {
		h.peerMetadata = newPeerMetadataCache()
	}
	h.peerMetadata.Add(server, meta)
}

// AddPeerMetadata 合并给定对等节点的协议元数据到 HTTP 主机
// 当您有对等节点协议映射的带外知识时很有用
// 参数:
//   - server: peer.ID - 服务器 ID
//   - meta: PeerMeta - 元数据
func (h *Host) AddPeerMetadata(server peer.ID, meta PeerMeta) {
	if h.peerMetadata == nil {
		h.peerMetadata = newPeerMetadataCache()
	}
	origMeta, ok := h.peerMetadata.Get(server)
	if !ok {
		h.peerMetadata.Add(server, meta)
		return
	}
	for proto, m := range meta {
		origMeta[proto] = m
	}
	h.peerMetadata.Add(server, origMeta)
}

// GetPeerMetadata 从 HTTP 主机获取对等节点的缓存协议元数据
// 参数:
//   - server: peer.ID - 服务器 ID
//
// 返回:
//   - PeerMeta: 对等节点元数据
//   - bool: 是否找到元数据
func (h *Host) GetPeerMetadata(server peer.ID) (PeerMeta, bool) {
	if h.peerMetadata == nil {
		return nil, false
	}
	return h.peerMetadata.Get(server)
}

// RemovePeerMetadata 从 HTTP 主机移除对等节点的协议元数据
// 参数:
//   - server: peer.ID - 服务器 ID
func (h *Host) RemovePeerMetadata(server peer.ID) {
	if h.peerMetadata == nil {
		return
	}
	h.peerMetadata.Remove(server)
}

// connectionCloseHeaderMiddleware 设置连接关闭头的中间件
// 参数:
//   - next: http.Handler - 下一个处理器
//
// 返回:
//   - http.Handler: 包装后的处理器
func connectionCloseHeaderMiddleware(next http.Handler) http.Handler {
	// 设置 connection: close。最好不要重用 HTTP 的流
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Connection", "close")
		next.ServeHTTP(w, r)
	})
}
