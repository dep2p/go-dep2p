package config

import (
	"fmt"
	"strings"
)

// ValidationError 配置校验错误
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("配置错误 [%s]: %s", e.Field, e.Message)
}

// ValidationErrors 多个配置校验错误
type ValidationErrors []ValidationError

func (e ValidationErrors) Error() string {
	if len(e) == 0 {
		return ""
	}

	var msgs []string
	for _, err := range e {
		msgs = append(msgs, err.Error())
	}
	return strings.Join(msgs, "; ")
}

// HasErrors 是否有错误
func (e ValidationErrors) HasErrors() bool {
	return len(e) > 0
}

// Validator 配置校验器
type Validator struct {
	errors ValidationErrors
}

// NewValidator 创建校验器
func NewValidator() *Validator {
	return &Validator{
		errors: make(ValidationErrors, 0),
	}
}

// addError 添加错误
func (v *Validator) addError(field, message string) {
	v.errors = append(v.errors, ValidationError{
		Field:   field,
		Message: message,
	})
}

// Errors 返回所有错误
func (v *Validator) Errors() ValidationErrors {
	return v.errors
}

// Validate 校验配置
func Validate(config *Config) error {
	v := NewValidator()

	// 校验监听地址
	v.validateListenAddrs(config.ListenAddrs)

	// 校验连接管理配置
	v.validateConnectionManager(&config.ConnectionManager)

	// 校验传输配置
	v.validateTransport(&config.Transport)

	// 校验发现配置
	v.validateDiscovery(&config.Discovery)

	// 校验中继配置
	v.validateRelay(&config.Relay)

	// 校验消息配置
	v.validateMessaging(&config.Messaging)

	if v.errors.HasErrors() {
		return v.errors
	}
	return nil
}

// validateListenAddrs 校验监听地址
func (v *Validator) validateListenAddrs(addrs []string) {
	for i, addr := range addrs {
		if addr == "" {
			v.addError(
				fmt.Sprintf("listen_addrs[%d]", i),
				"地址不能为空",
			)
			continue
		}

		// 简单检查格式
		if !strings.HasPrefix(addr, "/") {
			v.addError(
				fmt.Sprintf("listen_addrs[%d]", i),
				"地址必须以 / 开头 (Multiaddr 格式)",
			)
		}
	}
}

// validateConnectionManager 校验连接管理配置
func (v *Validator) validateConnectionManager(cfg *ConnectionManagerConfig) {
	if cfg.LowWater < 0 {
		v.addError("connection_manager.low_water", "不能为负数")
	}

	if cfg.HighWater < 0 {
		v.addError("connection_manager.high_water", "不能为负数")
	}

	if cfg.LowWater > cfg.HighWater {
		v.addError(
			"connection_manager.low_water",
			"低水位线不能大于高水位线",
		)
	}

	if cfg.EmergencyWater > 0 && cfg.EmergencyWater < cfg.HighWater {
		v.addError(
			"connection_manager.emergency_water",
			"紧急水位线不能小于高水位线",
		)
	}

	if cfg.GracePeriod < 0 {
		v.addError("connection_manager.grace_period", "不能为负数")
	}

	if cfg.IdleTimeout < 0 {
		v.addError("connection_manager.idle_timeout", "不能为负数")
	}
}

// validateTransport 校验传输配置
func (v *Validator) validateTransport(cfg *TransportConfig) {
	if cfg.MaxConnections < 0 {
		v.addError("transport.max_connections", "不能为负数")
	}

	if cfg.MaxStreamsPerConn < 0 {
		v.addError("transport.max_streams_per_conn", "不能为负数")
	}

	if cfg.DialTimeout < 0 {
		v.addError("transport.dial_timeout", "不能为负数")
	}

	if cfg.HandshakeTimeout < 0 {
		v.addError("transport.handshake_timeout", "不能为负数")
	}

	if cfg.IdleTimeout < 0 {
		v.addError("transport.idle_timeout", "不能为负数")
	}

	// QUIC 配置
	if cfg.QUIC.MaxIdleTimeout < 0 {
		v.addError("transport.quic.max_idle_timeout", "不能为负数")
	}

	if cfg.QUIC.MaxIncomingStreams < 0 {
		v.addError("transport.quic.max_incoming_streams", "不能为负数")
	}

	if cfg.QUIC.KeepAlivePeriod < 0 {
		v.addError("transport.quic.keep_alive_period", "不能为负数")
	}
}

// validateDiscovery 校验发现配置
func (v *Validator) validateDiscovery(cfg *DiscoveryConfig) {
	if cfg.RefreshInterval < 0 {
		v.addError("discovery.refresh_interval", "不能为负数")
	}

	if cfg.DHT.BucketSize < 1 {
		v.addError("discovery.dht.bucket_size", "必须大于 0")
	}

	if cfg.DHT.Concurrency < 1 {
		v.addError("discovery.dht.concurrency", "必须大于 0")
	}

	// 验证引导节点格式
	for i, peer := range cfg.BootstrapPeers {
		if peer == "" {
			v.addError(
				fmt.Sprintf("discovery.bootstrap_peers[%d]", i),
				"节点地址不能为空",
			)
			continue
		}

		if !strings.HasPrefix(peer, "/") {
			v.addError(
				fmt.Sprintf("discovery.bootstrap_peers[%d]", i),
				"节点地址必须以 / 开头 (Multiaddr 格式)",
			)
		}
	}
}

// validateRelay 校验中继配置
func (v *Validator) validateRelay(cfg *RelayConfig) {
	if cfg.MaxReservations < 0 {
		v.addError("relay.max_reservations", "不能为负数")
	}

	if cfg.MaxCircuits < 0 {
		v.addError("relay.max_circuits", "不能为负数")
	}

	if cfg.MaxCircuitsPerPeer < 0 {
		v.addError("relay.max_circuits_per_peer", "不能为负数")
	}

	if cfg.ReservationTTL < 0 {
		v.addError("relay.reservation_ttl", "不能为负数")
	}
}

// validateMessaging 校验消息配置
func (v *Validator) validateMessaging(cfg *MessagingConfig) {
	if cfg.RequestTimeout < 0 {
		v.addError("messaging.request_timeout", "不能为负数")
	}

	if cfg.MaxMessageSize < 0 {
		v.addError("messaging.max_message_size", "不能为负数")
	}

	if cfg.PubSub.MessageCacheSize < 0 {
		v.addError("messaging.pubsub.message_cache_size", "不能为负数")
	}

	if cfg.PubSub.MessageCacheTTL < 0 {
		v.addError("messaging.pubsub.message_cache_ttl", "不能为负数")
	}
}

