// Package introspect 提供本地自省 HTTP 服务
//
// 该服务运行在本地端口，提供 JSON 格式的诊断信息，用于调试和监控。
// 默认绑定到 127.0.0.1，不暴露到网络。
//
// 端点：
//   - GET /debug/introspect      - 完整诊断报告 (JSON)
//   - GET /debug/introspect/node - 节点信息
//   - GET /debug/introspect/connections - 连接信息
//   - GET /debug/introspect/realm - Realm 信息
//   - GET /debug/introspect/relay - Relay 信息
//   - GET /debug/pprof/*         - Go pprof 端点
package introspect

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/pprof"
	"sync"
	"time"

	"github.com/dep2p/go-dep2p/internal/util/logger"
	"github.com/dep2p/go-dep2p/pkg/interfaces/endpoint"
	realmif "github.com/dep2p/go-dep2p/pkg/interfaces/realm"
	relayif "github.com/dep2p/go-dep2p/pkg/interfaces/relay"
)

var log = logger.Logger("introspect")

// DefaultAddr 默认监听地址
const DefaultAddr = "127.0.0.1:6060"

// Server 本地自省 HTTP 服务
type Server struct {
	// 依赖组件
	endpoint endpoint.Endpoint
	realm    realmif.RealmManager
	relay    relayif.RelayServer // 可选

	// 配置
	addr string

	// HTTP 服务器
	server   *http.Server
	listener net.Listener

	// 状态
	running bool
	mu      sync.Mutex
}

// Config 服务配置
type Config struct {
	// Addr 监听地址，默认 "127.0.0.1:6060"
	Addr string

	// Endpoint 必需的 Endpoint 组件
	Endpoint endpoint.Endpoint

	// Realm 可选的 Realm 管理器
	Realm realmif.RealmManager

	// Relay 可选的 Relay 服务器
	Relay relayif.RelayServer
}

// New 创建自省服务
func New(cfg Config) *Server {
	addr := cfg.Addr
	if addr == "" {
		addr = DefaultAddr
	}

	return &Server{
		endpoint: cfg.Endpoint,
		realm:    cfg.Realm,
		relay:    cfg.Relay,
		addr:     addr,
	}
}

// Start 启动服务
func (s *Server) Start(ctx context.Context) error {
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
	mux.HandleFunc("/debug/introspect/realm", s.handleRealm)
	mux.HandleFunc("/debug/introspect/relay", s.handleRelay)

	// pprof 端点
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)

	// 健康检查
	mux.HandleFunc("/health", s.handleHealth)

	// 创建监听器
	listener, err := net.Listen("tcp", s.addr)
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
			log.Error("自省服务异常退出", "error", err)
		}
	}()

	s.running = true
	log.Info("自省服务已启动", "addr", s.addr)
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
		log.Error("关闭自省服务失败", "error", err)
		return err
	}

	s.running = false
	log.Info("自省服务已停止")
	return nil
}

// Addr 返回实际监听地址
func (s *Server) Addr() string {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.listener != nil {
		return s.listener.Addr().String()
	}
	return s.addr
}

// ============================================================================
//                              HTTP 处理器
// ============================================================================

// IntrospectResponse 完整诊断响应
type IntrospectResponse struct {
	// 基础诊断报告
	endpoint.DiagnosticReport

	// 扩展信息（如果有）
	Extended *ExtendedInfo `json:"extended,omitempty"`
}

// ExtendedInfo 扩展诊断信息
type ExtendedInfo struct {
	// RelayServer 中继服务器统计（如果作为 Relay Server）
	RelayServer *RelayServerInfo `json:"relay_server,omitempty"`

	// RealmDetails Realm 详细信息
	RealmDetails *RealmDetailsInfo `json:"realm_details,omitempty"`
}

// RelayServerInfo 中继服务器信息
type RelayServerInfo struct {
	Running            bool   `json:"running"`
	ActiveReservations int    `json:"active_reservations"`
	ActiveConnections  int    `json:"active_connections"`
	TotalConnections   uint64 `json:"total_connections"`
	TotalBytesRelayed  uint64 `json:"total_bytes_relayed"`
}

// RealmDetailsInfo Realm 详细信息
type RealmDetailsInfo struct {
	RealmID string   `json:"realm_id,omitempty"`
	Members []string `json:"members,omitempty"`
	Topics  []string `json:"topics,omitempty"`
}

// handleIntrospect 处理完整诊断请求
func (s *Server) handleIntrospect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	response := IntrospectResponse{}

	// 获取基础诊断报告
	if s.endpoint != nil {
		response.DiagnosticReport = s.endpoint.DiagnosticReport()
	}

	// 收集扩展信息
	response.Extended = s.collectExtendedInfo()

	s.writeJSON(w, response)
}

// handleNode 处理节点信息请求
func (s *Server) handleNode(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.endpoint == nil {
		http.Error(w, "Endpoint not available", http.StatusServiceUnavailable)
		return
	}

	report := s.endpoint.DiagnosticReport()
	s.writeJSON(w, report.Node)
}

// handleConnections 处理连接信息请求
func (s *Server) handleConnections(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.endpoint == nil {
		http.Error(w, "Endpoint not available", http.StatusServiceUnavailable)
		return
	}

	report := s.endpoint.DiagnosticReport()
	s.writeJSON(w, report.Connections)
}

// handleRealm 处理 Realm 信息请求
func (s *Server) handleRealm(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	response := struct {
		endpoint.RealmDiagnostics
		Details *RealmDetailsInfo `json:"details,omitempty"`
	}{}

	if s.endpoint != nil {
		report := s.endpoint.DiagnosticReport()
		response.RealmDiagnostics = report.Realm
	}

	// 添加 Realm 详细信息
	if s.realm != nil {
		response.Details = s.collectRealmDetails()
	}

	s.writeJSON(w, response)
}

// handleRelay 处理 Relay 信息请求
func (s *Server) handleRelay(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	response := struct {
		endpoint.RelayDiagnostics
		Server *RelayServerInfo `json:"server,omitempty"`
	}{}

	if s.endpoint != nil {
		report := s.endpoint.DiagnosticReport()
		response.RelayDiagnostics = report.Relay
	}

	// 添加 Relay Server 信息
	if s.relay != nil {
		response.Server = s.collectRelayServerInfo()
	}

	s.writeJSON(w, response)
}

// handleHealth 处理健康检查请求
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	health := struct {
		Status    string    `json:"status"`
		Timestamp time.Time `json:"timestamp"`
	}{
		Status:    "ok",
		Timestamp: time.Now(),
	}

	// 检查核心组件
	if s.endpoint == nil {
		health.Status = "degraded"
	}

	s.writeJSON(w, health)
}

// ============================================================================
//                              辅助方法
// ============================================================================

// collectExtendedInfo 收集扩展信息
func (s *Server) collectExtendedInfo() *ExtendedInfo {
	info := &ExtendedInfo{}
	hasInfo := false

	// Relay Server 信息
	if s.relay != nil {
		info.RelayServer = s.collectRelayServerInfo()
		if info.RelayServer != nil {
			hasInfo = true
		}
	}

	// Realm 详细信息
	if s.realm != nil {
		info.RealmDetails = s.collectRealmDetails()
		if info.RealmDetails != nil {
			hasInfo = true
		}
	}

	if !hasInfo {
		return nil
	}
	return info
}

// collectRelayServerInfo 收集 Relay Server 信息
func (s *Server) collectRelayServerInfo() *RelayServerInfo {
	if s.relay == nil {
		return nil
	}

	stats := s.relay.Stats()
	return &RelayServerInfo{
		Running:            true,
		ActiveReservations: stats.ActiveReservations,
		ActiveConnections:  stats.ActiveConnections,
		TotalConnections:   stats.TotalConnections,
		TotalBytesRelayed:  stats.TotalBytesRelayed,
	}
}

// collectRealmDetails 收集 Realm 详细信息
func (s *Server) collectRealmDetails() *RealmDetailsInfo {
	if s.realm == nil {
		return nil
	}

	realm := s.realm.CurrentRealm()
	if realm == nil {
		return nil
	}

	details := &RealmDetailsInfo{
		RealmID: string(realm.ID()),
	}

	// 获取成员列表
	members := realm.Members()
	details.Members = make([]string, len(members))
	for i, m := range members {
		details.Members[i] = m.ShortString()
	}

	// 获取 Topics（如果 PubSub 服务可用）
	if pubsub := realm.PubSub(); pubsub != nil {
		topics := pubsub.Topics()
		details.Topics = make([]string, len(topics))
		for i, t := range topics {
			details.Topics[i] = t.Name()
		}
	}

	return details
}

// writeJSON 写入 JSON 响应
func (s *Server) writeJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Content-Type-Options", "nosniff")

	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(data); err != nil {
		log.Error("JSON 编码失败", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

