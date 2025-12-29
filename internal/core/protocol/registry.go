// Package protocol 提供协议注册表实现
package protocol

import (
	"errors"
	"sort"
	"strings"
	"sync"

	"github.com/dep2p/go-dep2p/pkg/interfaces/endpoint"
	protocolif "github.com/dep2p/go-dep2p/pkg/interfaces/protocol"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              错误定义
// ============================================================================

// 协议注册表相关错误
var (
	// ErrProtocolAlreadyRegistered 协议已注册
	ErrProtocolAlreadyRegistered = errors.New("protocol already registered")
	ErrProtocolNotFound          = errors.New("protocol not found")
	ErrInvalidProtocolID         = errors.New("invalid protocol ID")
	ErrMaxProtocolsReached       = errors.New("maximum protocols reached")
)

// ============================================================================
//                              ProtocolEntry 实现
// ============================================================================

// ProtocolEntry 协议条目
type ProtocolEntry struct {
	id      types.ProtocolID
	handler endpoint.ProtocolHandler

	// 元信息
	name        string
	version     string
	description string
	priority    int

	// 匹配函数（可选）
	matcher protocolif.MatchFunc
}

// NewProtocolEntry 创建协议条目
func NewProtocolEntry(id types.ProtocolID, handler endpoint.ProtocolHandler) *ProtocolEntry {
	name, version := ParseProtocolID(id)
	return &ProtocolEntry{
		id:       id,
		handler:  handler,
		name:     name,
		version:  version,
		priority: 0,
	}
}

// ID 返回协议 ID
func (p *ProtocolEntry) ID() types.ProtocolID {
	return p.id
}

// Handle 处理流
func (p *ProtocolEntry) Handle(stream endpoint.Stream) error {
	if p.handler != nil {
		p.handler(stream)
	}
	return nil
}

// Name 返回协议名称
func (p *ProtocolEntry) Name() string {
	return p.name
}

// Version 返回协议版本
func (p *ProtocolEntry) Version() string {
	return p.version
}

// Description 返回协议描述
func (p *ProtocolEntry) Description() string {
	return p.description
}

// Priority 返回协议优先级
func (p *ProtocolEntry) Priority() int {
	return p.priority
}

// SetDescription 设置协议描述
func (p *ProtocolEntry) SetDescription(desc string) {
	p.description = desc
}

// SetPriority 设置协议优先级
func (p *ProtocolEntry) SetPriority(priority int) {
	p.priority = priority
}

// SetMatcher 设置匹配函数
func (p *ProtocolEntry) SetMatcher(matcher protocolif.MatchFunc) {
	p.matcher = matcher
}

// Matches 检查是否匹配指定协议 ID
func (p *ProtocolEntry) Matches(protocolID types.ProtocolID) bool {
	// 精确匹配
	if p.id == protocolID {
		return true
	}

	// 自定义匹配函数
	if p.matcher != nil && p.matcher(protocolID) {
		return true
	}

	// 语义版本匹配
	return p.matchSemantic(protocolID)
}

// matchSemantic 语义版本匹配
func (p *ProtocolEntry) matchSemantic(protocolID types.ProtocolID) bool {
	reqName, reqVersion := ParseProtocolID(protocolID)

	// 名称必须匹配
	if reqName != p.name {
		return false
	}

	// 比较主版本号
	reqMajor := getMajorVersion(reqVersion)
	myMajor := getMajorVersion(p.version)

	return reqMajor == myMajor
}

// ============================================================================
//                              Registry 实现
// ============================================================================

// Registry 协议注册表
type Registry struct {
	protocols    map[types.ProtocolID]*ProtocolEntry
	byName       map[string][]*ProtocolEntry // 按名称索引
	maxProtocols int
	mu           sync.RWMutex
}

// NewRegistry 创建协议注册表
func NewRegistry(maxProtocols int) *Registry {
	if maxProtocols <= 0 {
		maxProtocols = 100
	}

	return &Registry{
		protocols:    make(map[types.ProtocolID]*ProtocolEntry),
		byName:       make(map[string][]*ProtocolEntry),
		maxProtocols: maxProtocols,
	}
}

// 确保实现接口
var _ protocolif.Registry = (*Registry)(nil)

// Register 注册协议
func (r *Registry) Register(protocol protocolif.Protocol) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	id := protocol.ID()

	// 验证协议 ID
	if id == "" {
		return ErrInvalidProtocolID
	}

	// 检查是否已注册
	if _, exists := r.protocols[id]; exists {
		return ErrProtocolAlreadyRegistered
	}

	// 检查数量限制
	if len(r.protocols) >= r.maxProtocols {
		return ErrMaxProtocolsReached
	}

	// 创建条目
	entry := &ProtocolEntry{
		id: id,
		handler: func(s endpoint.Stream) {
			protocol.Handle(s)
		},
	}
	entry.name, entry.version = ParseProtocolID(id)

	// 存储
	r.protocols[id] = entry

	// 按名称索引
	r.byName[entry.name] = append(r.byName[entry.name], entry)

	log.Debug("注册协议",
		"protocol", string(id),
		"name", entry.name,
		"version", entry.version)

	return nil
}

// RegisterWithHandler 使用处理器注册协议
func (r *Registry) RegisterWithHandler(id types.ProtocolID, handler endpoint.ProtocolHandler, opts ...ProtocolOption) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// 验证
	if id == "" {
		return ErrInvalidProtocolID
	}
	if _, exists := r.protocols[id]; exists {
		return ErrProtocolAlreadyRegistered
	}
	if len(r.protocols) >= r.maxProtocols {
		return ErrMaxProtocolsReached
	}

	// 创建条目
	entry := NewProtocolEntry(id, handler)

	// 应用选项
	for _, opt := range opts {
		opt(entry)
	}

	// 存储
	r.protocols[id] = entry
	r.byName[entry.name] = append(r.byName[entry.name], entry)

	log.Debug("注册协议（带处理器）",
		"protocol", string(id))

	return nil
}

// Unregister 注销协议
func (r *Registry) Unregister(protocolID types.ProtocolID) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	entry, exists := r.protocols[protocolID]
	if !exists {
		return ErrProtocolNotFound
	}

	// 从主映射删除
	delete(r.protocols, protocolID)

	// 从名称索引删除
	if entries, ok := r.byName[entry.name]; ok {
		newEntries := make([]*ProtocolEntry, 0, len(entries))
		for _, e := range entries {
			if e.id != protocolID {
				newEntries = append(newEntries, e)
			}
		}
		if len(newEntries) == 0 {
			delete(r.byName, entry.name)
		} else {
			r.byName[entry.name] = newEntries
		}
	}

	log.Debug("注销协议",
		"protocol", string(protocolID))

	return nil
}

// Get 获取协议
func (r *Registry) Get(protocolID types.ProtocolID) (protocolif.Protocol, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	entry, exists := r.protocols[protocolID]
	if !exists {
		return nil, false
	}

	return entry, true
}

// GetHandler 获取处理器
func (r *Registry) GetHandler(protocolID types.ProtocolID) (endpoint.ProtocolHandler, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	entry, exists := r.protocols[protocolID]
	if !exists {
		return nil, false
	}

	return entry.handler, true
}

// List 列出所有协议
func (r *Registry) List() []protocolif.Protocol {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// 使用具体类型来避免类型断言
	entries := make([]*ProtocolEntry, 0, len(r.protocols))
	for _, entry := range r.protocols {
		entries = append(entries, entry)
	}

	// 按优先级排序
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].priority > entries[j].priority
	})

	// 转换为接口类型
	protocols := make([]protocolif.Protocol, len(entries))
	for i, entry := range entries {
		protocols[i] = entry
	}

	return protocols
}

// Match 匹配协议
func (r *Registry) Match(protocolID types.ProtocolID) []protocolif.Protocol {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// 使用具体类型来避免类型断言
	var entries []*ProtocolEntry

	for _, entry := range r.protocols {
		if entry.Matches(protocolID) {
			entries = append(entries, entry)
		}
	}

	// 按优先级排序
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].priority > entries[j].priority
	})

	// 转换为接口类型
	matches := make([]protocolif.Protocol, len(entries))
	for i, entry := range entries {
		matches[i] = entry
	}

	return matches
}

// MatchHandler 匹配处理器
func (r *Registry) MatchHandler(protocolID types.ProtocolID) (endpoint.ProtocolHandler, bool) {
	matches := r.Match(protocolID)
	if len(matches) == 0 {
		return nil, false
	}
	// 安全的类型断言
	if entry, ok := matches[0].(*ProtocolEntry); ok {
		return entry.handler, true
	}
	return nil, false
}

// ListByName 按名称列出协议
func (r *Registry) ListByName(name string) []*ProtocolEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()

	entries := r.byName[name]
	result := make([]*ProtocolEntry, len(entries))
	copy(result, entries)
	return result
}

// Count 返回协议数量
func (r *Registry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.protocols)
}

// IDs 返回所有协议 ID
func (r *Registry) IDs() []types.ProtocolID {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ids := make([]types.ProtocolID, 0, len(r.protocols))
	for id := range r.protocols {
		ids = append(ids, id)
	}
	return ids
}

// Clear 清空注册表
func (r *Registry) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.protocols = make(map[types.ProtocolID]*ProtocolEntry)
	r.byName = make(map[string][]*ProtocolEntry)

	log.Debug("清空协议注册表")
}

// ============================================================================
//                              ProtocolOption
// ============================================================================

// ProtocolOption 协议选项
type ProtocolOption func(*ProtocolEntry)

// WithDescription 设置描述
func WithDescription(desc string) ProtocolOption {
	return func(p *ProtocolEntry) {
		p.description = desc
	}
}

// WithPriority 设置优先级
func WithPriority(priority int) ProtocolOption {
	return func(p *ProtocolEntry) {
		p.priority = priority
	}
}

// WithMatcher 设置匹配函数
func WithMatcher(matcher protocolif.MatchFunc) ProtocolOption {
	return func(p *ProtocolEntry) {
		p.matcher = matcher
	}
}

// ============================================================================
//                              辅助函数
// ============================================================================

// ParseProtocolID 解析协议 ID
//
// 协议 ID 格式: /name/version 或 /org/name/version
// 示例:
//   - /echo/1.0.0 -> name="echo", version="1.0.0"
//   - /dep2p/ping/1.0.0 -> name="dep2p/ping", version="1.0.0"
func ParseProtocolID(id types.ProtocolID) (name, version string) {
	s := string(id)
	s = strings.TrimPrefix(s, "/")

	parts := strings.Split(s, "/")
	if len(parts) == 0 {
		return "", ""
	}

	if len(parts) == 1 {
		return parts[0], ""
	}

	// 最后一部分是版本号
	version = parts[len(parts)-1]
	name = strings.Join(parts[:len(parts)-1], "/")

	return name, version
}

// getMajorVersion 获取主版本号
func getMajorVersion(version string) string {
	parts := strings.Split(version, ".")
	if len(parts) > 0 {
		return parts[0]
	}
	return version
}

// CompareVersions 比较版本号
//
// 返回值:
//   - < 0: v1 < v2
//   - = 0: v1 == v2
//   - > 0: v1 > v2
func CompareVersions(v1, v2 string) int {
	parts1 := strings.Split(v1, ".")
	parts2 := strings.Split(v2, ".")

	maxLen := len(parts1)
	if len(parts2) > maxLen {
		maxLen = len(parts2)
	}

	for i := 0; i < maxLen; i++ {
		var n1, n2 int
		if i < len(parts1) {
			n1 = parseVersionPart(parts1[i])
		}
		if i < len(parts2) {
			n2 = parseVersionPart(parts2[i])
		}

		if n1 < n2 {
			return -1
		}
		if n1 > n2 {
			return 1
		}
	}

	return 0
}

// parseVersionPart 解析版本号部分
func parseVersionPart(s string) int {
	var n int
	for _, c := range s {
		if c >= '0' && c <= '9' {
			n = n*10 + int(c-'0')
		} else {
			break
		}
	}
	return n
}

// IsCompatibleVersion 检查版本兼容性
//
// 主版本号相同则兼容
func IsCompatibleVersion(v1, v2 string) bool {
	return getMajorVersion(v1) == getMajorVersion(v2)
}

