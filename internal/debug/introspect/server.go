package introspect

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/pprof"
	"runtime"
	"sync"
	"time"

	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/lib/log"
)

var logger = log.Logger("core/introspect")

// DefaultAddr 默认监听地址
const DefaultAddr = "127.0.0.1:6060"

// ============================================================================
//                              配置
// ============================================================================

// Config 服务配置
type Config struct {
	// Addr 监听地址，默认 "127.0.0.1:6060"
	Addr string

	// Host 可选的 Host 组件
	Host pkgif.Host

	// ConnManager 可选的连接管理器
	ConnManager pkgif.ConnManager

	// BandwidthReporter 可选的带宽报告器
	BandwidthReporter BandwidthReporter

	// CustomHandlers 自定义处理器
	CustomHandlers map[string]http.HandlerFunc
}

// BandwidthReporter 带宽报告接口
type BandwidthReporter interface {
	GetBandwidthForPeer(peer string) (in, out int64)
	GetBandwidthTotals() (in, out int64)
}

// ============================================================================
//                              Server
// ============================================================================

// Server 本地自省 HTTP 服务
type Server struct {
	config Config

	// HTTP 服务器
	server   *http.Server
	listener net.Listener

	// 状态
	running   bool
	startTime time.Time

	mu sync.Mutex
}

// New 创建自省服务
func New(cfg Config) *Server {
	if cfg.Addr == "" {
		cfg.Addr = DefaultAddr
	}

	return &Server{
		config: cfg,
	}
}

// Start 启动服务
func (s *Server) Start(_ context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		return nil
	}

	// 创建路由
	mux := http.NewServeMux()

	// 自省端点
	mux.HandleFunc("/debug/introspect", s.handleIntrospect)
	mux.HandleFunc("/debug/introspect/node", s.handleNode)
	mux.HandleFunc("/debug/introspect/connections", s.handleConnections)
	mux.HandleFunc("/debug/introspect/peers", s.handlePeers)
	mux.HandleFunc("/debug/introspect/bandwidth", s.handleBandwidth)
	mux.HandleFunc("/debug/introspect/runtime", s.handleRuntime)

	// pprof 端点
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)

	// 健康检查
	mux.HandleFunc("/health", s.handleHealth)

	// 自定义处理器
	for path, handler := range s.config.CustomHandlers {
		mux.HandleFunc(path, handler)
	}

	// 创建监听器
	listener, err := net.Listen("tcp", s.config.Addr)
	if err != nil {
		return err
	}
	s.listener = listener

	// 创建 HTTP 服务器
	s.server = &http.Server{
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	// 启动服务
	go func() {
		if err := s.server.Serve(listener); err != nil && err != http.ErrServerClosed {
			logger.Error("自省服务异常退出", "error", err)
		}
	}()

	s.running = true
	s.startTime = time.Now()
	logger.Info("自省服务已启动", "addr", s.config.Addr)
	return nil
}

// Stop 停止服务
func (s *Server) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := s.server.Shutdown(ctx); err != nil {
		logger.Error("关闭自省服务失败", "error", err)
		return err
	}

	s.running = false
	logger.Info("自省服务已停止")
	return nil
}

// Addr 返回实际监听地址
func (s *Server) Addr() string {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.listener != nil {
		return s.listener.Addr().String()
	}
	return s.config.Addr
}

// ============================================================================
//                              响应结构
// ============================================================================

// IntrospectResponse 完整诊断响应
type IntrospectResponse struct {
	Timestamp   time.Time       `json:"timestamp"`
	Uptime      string          `json:"uptime"`
	Node        *NodeInfo       `json:"node,omitempty"`
	Connections *ConnectionInfo `json:"connections,omitempty"`
	Bandwidth   *BandwidthInfo  `json:"bandwidth,omitempty"`
	Runtime     *RuntimeInfo    `json:"runtime,omitempty"`
}

// NodeInfo 节点信息
type NodeInfo struct {
	ID        string   `json:"id"`
	Addresses []string `json:"addresses"`
	Protocols []string `json:"protocols,omitempty"`
}

// ConnectionInfo 连接信息
type ConnectionInfo struct {
	Total    int        `json:"total"`
	Inbound  int        `json:"inbound"`
	Outbound int        `json:"outbound"`
	Peers    []PeerInfo `json:"peers,omitempty"`
}

// PeerInfo 节点信息
type PeerInfo struct {
	ID        string   `json:"id"`
	Addresses []string `json:"addresses"`
	Direction string   `json:"direction"`
	Latency   string   `json:"latency,omitempty"`
}

// BandwidthInfo 带宽信息
type BandwidthInfo struct {
	TotalIn  int64 `json:"total_in"`
	TotalOut int64 `json:"total_out"`
}

// RuntimeInfo 运行时信息
type RuntimeInfo struct {
	GoVersion    string `json:"go_version"`
	NumGoroutine int    `json:"num_goroutine"`
	NumCPU       int    `json:"num_cpu"`
	MemAlloc     uint64 `json:"mem_alloc"`
	MemSys       uint64 `json:"mem_sys"`
	NumGC        uint32 `json:"num_gc"`
}

// HealthResponse 健康检查响应
type HealthResponse struct {
	Status    string    `json:"status"`
	Timestamp time.Time `json:"timestamp"`
	Uptime    string    `json:"uptime,omitempty"`
}

// ============================================================================
//                              HTTP 处理器
// ============================================================================

// handleIntrospect 处理完整诊断请求
func (s *Server) handleIntrospect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	response := IntrospectResponse{
		Timestamp: time.Now(),
		Uptime:    time.Since(s.startTime).String(),
	}

	// 收集节点信息
	response.Node = s.collectNodeInfo()

	// 收集连接信息
	response.Connections = s.collectConnectionInfo()

	// 收集带宽信息
	response.Bandwidth = s.collectBandwidthInfo()

	// 收集运行时信息
	response.Runtime = s.collectRuntimeInfo()

	s.writeJSON(w, response)
}

// handleNode 处理节点信息请求
func (s *Server) handleNode(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	info := s.collectNodeInfo()
	if info == nil {
		http.Error(w, "Node info not available", http.StatusServiceUnavailable)
		return
	}

	s.writeJSON(w, info)
}

// handleConnections 处理连接信息请求
func (s *Server) handleConnections(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	info := s.collectConnectionInfo()
	if info == nil {
		http.Error(w, "Connection info not available", http.StatusServiceUnavailable)
		return
	}

	s.writeJSON(w, info)
}

// handlePeers 处理节点列表请求
func (s *Server) handlePeers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	connInfo := s.collectConnectionInfo()
	if connInfo == nil {
		http.Error(w, "Peer info not available", http.StatusServiceUnavailable)
		return
	}

	s.writeJSON(w, connInfo.Peers)
}

// handleBandwidth 处理带宽统计请求
func (s *Server) handleBandwidth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	info := s.collectBandwidthInfo()
	if info == nil {
		info = &BandwidthInfo{} // 返回空数据而不是错误
	}

	s.writeJSON(w, info)
}

// handleRuntime 处理运行时信息请求
func (s *Server) handleRuntime(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	info := s.collectRuntimeInfo()
	s.writeJSON(w, info)
}

// handleHealth 处理健康检查请求
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	health := HealthResponse{
		Status:    "ok",
		Timestamp: time.Now(),
		Uptime:    time.Since(s.startTime).String(),
	}

	// 检查核心组件
	if s.config.Host == nil {
		health.Status = "degraded"
	}

	s.writeJSON(w, health)
}

// ============================================================================
//                              数据收集
// ============================================================================

// collectNodeInfo 收集节点信息
func (s *Server) collectNodeInfo() *NodeInfo {
	if s.config.Host == nil {
		return nil
	}

	info := &NodeInfo{
		ID: s.config.Host.ID(),
	}

	// 获取地址（已经是 []string）
	info.Addresses = s.config.Host.Addrs()

	return info
}

// collectConnectionInfo 收集连接信息
func (s *Server) collectConnectionInfo() *ConnectionInfo {
	if s.config.Host == nil {
		return nil
	}

	info := &ConnectionInfo{
		Peers: make([]PeerInfo, 0),
	}

	// 从 Peerstore 获取所有节点
	if peerstore := s.config.Host.Peerstore(); peerstore != nil {
		peers := peerstore.Peers()
		info.Total = len(peers)

		for _, peerID := range peers {
			addrs := peerstore.Addrs(peerID)
			addrStrs := make([]string, len(addrs))
			for i, addr := range addrs {
				addrStrs[i] = addr.String()
			}

			peerInfo := PeerInfo{
				ID:        string(peerID),
				Addresses: addrStrs,
				Direction: "unknown", // 无法从 Peerstore 确定方向
			}
			info.Peers = append(info.Peers, peerInfo)
		}
	}

	// 注：如果需要更详细的连接信息，可以从 ConnManager 获取
	// 这里简化处理，只用 Peerstore 的数据

	return info
}

// collectBandwidthInfo 收集带宽信息
func (s *Server) collectBandwidthInfo() *BandwidthInfo {
	if s.config.BandwidthReporter == nil {
		return nil
	}

	in, out := s.config.BandwidthReporter.GetBandwidthTotals()
	return &BandwidthInfo{
		TotalIn:  in,
		TotalOut: out,
	}
}

// collectRuntimeInfo 收集运行时信息
func (s *Server) collectRuntimeInfo() *RuntimeInfo {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	return &RuntimeInfo{
		GoVersion:    runtime.Version(),
		NumGoroutine: runtime.NumGoroutine(),
		NumCPU:       runtime.NumCPU(),
		MemAlloc:     memStats.Alloc,
		MemSys:       memStats.Sys,
		NumGC:        memStats.NumGC,
	}
}

// ============================================================================
//                              辅助方法
// ============================================================================

// writeJSON 写入 JSON 响应
func (s *Server) writeJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Content-Type-Options", "nosniff")

	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(data); err != nil {
		logger.Error("JSON 编码失败", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}
