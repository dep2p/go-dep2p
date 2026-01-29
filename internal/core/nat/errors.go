package nat

import "errors"

// Sentinel errors
var (
	// ErrNoUPnPDevice UPnP 设备未找到
	ErrNoUPnPDevice = errors.New("nat: no UPnP device found")

	// ErrHolePunchFailed Hole Punching 失败
	ErrHolePunchFailed = errors.New("nat: hole punch failed")

	// ErrSTUNTimeout STUN 请求超时
	ErrSTUNTimeout = errors.New("nat: STUN request timeout")

	// ErrNoExternalAddr 无法获取外部地址
	ErrNoExternalAddr = errors.New("nat: no external address")

	// ErrNoSTUNServers 没有配置 STUN 服务器
	ErrNoSTUNServers = errors.New("nat: no STUN servers configured")

	// ErrServiceClosed NAT 服务已关闭
	ErrServiceClosed = errors.New("nat: service closed")

	// ErrAlreadyStarted NAT 服务已经启动
	ErrAlreadyStarted = errors.New("nat: service already started")

	// ErrNotStarted NAT 服务未启动
	ErrNotStarted = errors.New("nat: service not started")

	// ErrInvalidConfig 配置无效
	ErrInvalidConfig = errors.New("nat: invalid config")

	// ErrNoPeers 没有可用的节点进行探测
	ErrNoPeers = errors.New("nat: no peers available for probe")

	// ErrHolePunchActive 已有活跃的打洞尝试
	ErrHolePunchActive = errors.New("nat: hole punch already active for peer")

	// ErrNoAddresses 没有可用的地址
	ErrNoAddresses = errors.New("nat: no addresses available")

	// ErrMappingFailed 端口映射失败
	ErrMappingFailed = errors.New("nat: port mapping failed")

	// ErrNATDetectionFailed NAT 检测失败
	ErrNATDetectionFailed = errors.New("nat: NAT detection failed")

	// ErrInvalidAddress 地址无效
	ErrInvalidAddress = errors.New("nat: invalid address")

	// ErrSTUNResponseInvalid STUN 响应无效
	ErrSTUNResponseInvalid = errors.New("nat: STUN response invalid")

	// ErrNATTypeUnknown NAT 类型未知
	ErrNATTypeUnknown = errors.New("nat: NAT type unknown")
)

// DialError 拨号错误（聚合多个错误）
type DialError struct {
	Cause  error
	Errors []error
}

func (e *DialError) Error() string {
	if e.Cause != nil {
		return "nat dial error: " + e.Cause.Error()
	}
	if len(e.Errors) > 0 {
		return "nat dial error: multiple failures"
	}
	return "nat dial error: unknown"
}

// Unwrap 解包错误
func (e *DialError) Unwrap() error {
	return e.Cause
}

// MappingError 端口映射错误
type MappingError struct {
	Protocol string
	Port     int
	Cause    error
}

func (e *MappingError) Error() string {
	return "nat mapping error for " + e.Protocol + " port " + string(rune(e.Port)) + ": " + e.Cause.Error()
}

// Unwrap 解包错误
func (e *MappingError) Unwrap() error {
	return e.Cause
}

// ProbeError 探测错误
type ProbeError struct {
	PeerID string
	Cause  error
}

func (e *ProbeError) Error() string {
	return "nat probe error for peer " + e.PeerID + ": " + e.Cause.Error()
}

// Unwrap 解包错误
func (e *ProbeError) Unwrap() error {
	return e.Cause
}
