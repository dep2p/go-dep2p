package geoip

import (
	"fmt"
	"net"
	"sync"
)

// ============================================================================
//                              类型定义
// ============================================================================

// GeoInfo 地理位置信息
type GeoInfo struct {
	// CountryCode ISO 3166-1 Alpha-2 国家代码（如 "US", "CN", "DE"）
	CountryCode string

	// CountryName 国家名称
	CountryName string

	// ContinentCode 大洲代码（如 "NA", "EU", "AS"）
	ContinentCode string

	// ContinentName 大洲名称
	ContinentName string

	// City 城市名称（可选）
	City string

	// Region 区域/省份名称（可选）
	Region string

	// Latitude 纬度
	Latitude float64

	// Longitude 经度
	Longitude float64

	// TimeZone 时区
	TimeZone string
}

// ToRegionString 生成区域字符串标识
//
// 优先使用大洲代码，如 "NA"(北美)、"EU"(欧洲)、"AS"(亚洲)
func (g *GeoInfo) ToRegionString() string {
	if g.ContinentCode != "" {
		return g.ContinentCode
	}
	if g.CountryCode != "" {
		return g.CountryCode
	}
	return "unknown"
}

// ============================================================================
//                              配置
// ============================================================================

// Config GeoIP 配置
type Config struct {
	// CacheSize 查询结果缓存大小（IP 数量）
	// 0 表示禁用缓存，默认 1000
	CacheSize int

	// Enabled 是否启用 GeoIP
	// 禁用时所有查询返回 nil
	Enabled bool
}

// DefaultConfig 返回默认配置
func DefaultConfig() Config {
	return Config{
		CacheSize: 1000,
		Enabled:   true,
	}
}

// ============================================================================
//                              Resolver 接口
// ============================================================================

// Resolver GeoIP 解析器接口
type Resolver interface {
	// Lookup 查询 IP 地址的地理位置信息
	Lookup(ip net.IP) (*GeoInfo, error)

	// LookupString 查询字符串形式的 IP 地址
	LookupString(ipStr string) (*GeoInfo, error)

	// Close 关闭解析器，释放资源
	Close() error

	// IsAvailable 检查解析器是否可用
	IsAvailable() bool
}

// ============================================================================
//                              Stub Resolver（测试用）
// ============================================================================

// StubResolver 测试用桩解析器
//
// 允许预设 IP -> GeoInfo 映射，用于单元测试
type StubResolver struct {
	mapping map[string]*GeoInfo
	enabled bool
	mu      sync.RWMutex
}

var _ Resolver = (*StubResolver)(nil)

// NewStubResolver 创建桩解析器
func NewStubResolver() *StubResolver {
	return &StubResolver{
		mapping: make(map[string]*GeoInfo),
		enabled: true,
	}
}

// SetMapping 设置 IP 到 GeoInfo 的映射
func (r *StubResolver) SetMapping(ipStr string, info *GeoInfo) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.mapping[ipStr] = info
}

// SetRegion 快捷方法：设置 IP 的区域
func (r *StubResolver) SetRegion(ipStr string, region string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.mapping[ipStr] = &GeoInfo{
		ContinentCode: region,
	}
}

// Lookup 查询 IP
func (r *StubResolver) Lookup(ip net.IP) (*GeoInfo, error) {
	if !r.enabled {
		return nil, nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.mapping[ip.String()], nil
}

// LookupString 查询字符串 IP
func (r *StubResolver) LookupString(ipStr string) (*GeoInfo, error) {
	if !r.enabled {
		return nil, nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.mapping[ipStr], nil
}

// Close 关闭解析器
func (r *StubResolver) Close() error {
	return nil
}

// IsAvailable 检查是否可用
func (r *StubResolver) IsAvailable() bool {
	return r.enabled
}

// SetEnabled 设置是否启用
func (r *StubResolver) SetEnabled(enabled bool) {
	r.enabled = enabled
}

// Clear 清空映射
func (r *StubResolver) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.mapping = make(map[string]*GeoInfo)
}

// ============================================================================
//                              Simple Resolver（基于 IP 段）
// ============================================================================

// SimpleResolver 简单解析器
//
// 基于已知 IP 段进行区域判断，用于不需要精确 GeoIP 数据库的场景
type SimpleResolver struct {
	config Config
	cache  map[string]*GeoInfo
	mu     sync.RWMutex
}

var _ Resolver = (*SimpleResolver)(nil)

// NewSimpleResolver 创建简单解析器
func NewSimpleResolver(config Config) *SimpleResolver {
	if config.CacheSize == 0 {
		config.CacheSize = DefaultConfig().CacheSize
	}
	return &SimpleResolver{
		config: config,
		cache:  make(map[string]*GeoInfo, config.CacheSize),
	}
}

// Lookup 查询 IP
func (r *SimpleResolver) Lookup(ip net.IP) (*GeoInfo, error) {
	if !r.config.Enabled {
		return nil, nil
	}

	ipStr := ip.String()

	// 检查缓存
	r.mu.RLock()
	if info, ok := r.cache[ipStr]; ok {
		r.mu.RUnlock()
		return info, nil
	}
	r.mu.RUnlock()

	// 判断区域
	info := r.resolveIP(ip)

	// 更新缓存
	r.mu.Lock()
	if len(r.cache) < r.config.CacheSize {
		r.cache[ipStr] = info
	}
	r.mu.Unlock()

	return info, nil
}

// LookupString 查询字符串 IP
func (r *SimpleResolver) LookupString(ipStr string) (*GeoInfo, error) {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return nil, fmt.Errorf("geoip: invalid IP address: %s", ipStr)
	}
	return r.Lookup(ip)
}

// Close 关闭解析器
func (r *SimpleResolver) Close() error {
	return nil
}

// IsAvailable 检查是否可用
func (r *SimpleResolver) IsAvailable() bool {
	return r.config.Enabled
}

// ClearCache 清空缓存
func (r *SimpleResolver) ClearCache() {
	r.mu.Lock()
	r.cache = make(map[string]*GeoInfo, r.config.CacheSize)
	r.mu.Unlock()
}

// resolveIP 解析 IP 的区域
//
// 使用简化的 IP 段判断，基于 IANA 分配规则
func (r *SimpleResolver) resolveIP(ip net.IP) *GeoInfo {
	// 私有地址
	if ip.IsPrivate() || ip.IsLoopback() {
		return &GeoInfo{
			ContinentCode: "LOCAL",
			ContinentName: "Local Network",
		}
	}

	// IPv6 简化处理
	if ip.To4() == nil {
		return &GeoInfo{
			ContinentCode: "UNKNOWN",
			ContinentName: "Unknown",
		}
	}

	// 基于第一个字节粗略判断
	// 这只是简化实现，不准确
	firstByte := ip.To4()[0]

	switch {
	// 1.0.0.0 - 126.0.0.0 主要是 ARIN/APNIC
	case firstByte >= 1 && firstByte <= 126:
		if firstByte <= 55 {
			return &GeoInfo{
				ContinentCode: "NA",
				ContinentName: "North America",
			}
		}
		return &GeoInfo{
			ContinentCode: "AS",
			ContinentName: "Asia",
		}

	// 128-191 主要是欧洲和北美
	case firstByte >= 128 && firstByte <= 191:
		return &GeoInfo{
			ContinentCode: "EU",
			ContinentName: "Europe",
		}

	// 192-223 混合
	case firstByte >= 192 && firstByte <= 223:
		return &GeoInfo{
			ContinentCode: "NA",
			ContinentName: "North America",
		}

	default:
		return &GeoInfo{
			ContinentCode: "UNKNOWN",
			ContinentName: "Unknown",
		}
	}
}

// ============================================================================
//                              Region Resolver（区域映射）
// ============================================================================

// RegionResolver 区域解析器
//
// 使用预定义的 CIDR 到区域映射
type RegionResolver struct {
	config   Config
	mappings []regionMapping
	cache    map[string]*GeoInfo
	mu       sync.RWMutex
}

type regionMapping struct {
	network *net.IPNet
	info    *GeoInfo
}

var _ Resolver = (*RegionResolver)(nil)

// NewRegionResolver 创建区域解析器
func NewRegionResolver(config Config) *RegionResolver {
	if config.CacheSize == 0 {
		config.CacheSize = DefaultConfig().CacheSize
	}

	r := &RegionResolver{
		config:   config,
		mappings: make([]regionMapping, 0),
		cache:    make(map[string]*GeoInfo, config.CacheSize),
	}

	// 添加一些常见的映射
	r.addDefaultMappings()

	return r
}

// AddMapping 添加 CIDR 到区域的映射
func (r *RegionResolver) AddMapping(cidr string, info *GeoInfo) error {
	_, network, err := net.ParseCIDR(cidr)
	if err != nil {
		return err
	}

	r.mu.Lock()
	r.mappings = append(r.mappings, regionMapping{
		network: network,
		info:    info,
	})
	r.mu.Unlock()

	return nil
}

// addDefaultMappings 添加默认映射
func (r *RegionResolver) addDefaultMappings() {
	// 私有地址
	r.AddMapping("10.0.0.0/8", &GeoInfo{ContinentCode: "LOCAL", ContinentName: "Local Network"})
	r.AddMapping("172.16.0.0/12", &GeoInfo{ContinentCode: "LOCAL", ContinentName: "Local Network"})
	r.AddMapping("192.168.0.0/16", &GeoInfo{ContinentCode: "LOCAL", ContinentName: "Local Network"})
	r.AddMapping("127.0.0.0/8", &GeoInfo{ContinentCode: "LOCAL", ContinentName: "Loopback"})

	// 常见 CDN/云服务商范围（示例，不完整）
	// Google
	r.AddMapping("8.8.8.0/24", &GeoInfo{ContinentCode: "NA", CountryCode: "US", CountryName: "United States"})
	// Cloudflare
	r.AddMapping("1.1.1.0/24", &GeoInfo{ContinentCode: "NA", CountryCode: "US", CountryName: "United States"})
}

// Lookup 查询 IP
func (r *RegionResolver) Lookup(ip net.IP) (*GeoInfo, error) {
	if !r.config.Enabled {
		return nil, nil
	}

	ipStr := ip.String()

	// 检查缓存
	r.mu.RLock()
	if info, ok := r.cache[ipStr]; ok {
		r.mu.RUnlock()
		return info, nil
	}
	r.mu.RUnlock()

	// 查找匹配的映射
	r.mu.RLock()
	var info *GeoInfo
	for _, m := range r.mappings {
		if m.network.Contains(ip) {
			info = m.info
			break
		}
	}
	r.mu.RUnlock()

	// 未找到时返回未知
	if info == nil {
		info = &GeoInfo{
			ContinentCode: "UNKNOWN",
			ContinentName: "Unknown",
		}
	}

	// 更新缓存
	r.mu.Lock()
	if len(r.cache) < r.config.CacheSize {
		r.cache[ipStr] = info
	}
	r.mu.Unlock()

	return info, nil
}

// LookupString 查询字符串 IP
func (r *RegionResolver) LookupString(ipStr string) (*GeoInfo, error) {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return nil, fmt.Errorf("geoip: invalid IP address: %s", ipStr)
	}
	return r.Lookup(ip)
}

// Close 关闭解析器
func (r *RegionResolver) Close() error {
	return nil
}

// IsAvailable 检查是否可用
func (r *RegionResolver) IsAvailable() bool {
	return r.config.Enabled
}

// ClearCache 清空缓存
func (r *RegionResolver) ClearCache() {
	r.mu.Lock()
	r.cache = make(map[string]*GeoInfo, r.config.CacheSize)
	r.mu.Unlock()
}
