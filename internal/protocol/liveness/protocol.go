// Package liveness 实现存活检测服务
package liveness

import (
	"github.com/dep2p/go-dep2p/pkg/protocol"
)

const (
	// ProtocolVersion 协议版本
	ProtocolVersion = protocol.Version10

	// PingProtocolSuffix Ping协议后缀
	PingProtocolSuffix = "ping"
)

// buildProtocolID 构建完整协议ID
//
// 格式: /dep2p/app/<realmID>/liveness/ping/1.0.0
func buildProtocolID(realmID string) string {
	return string(protocol.BuildAppProtocol(realmID, "liveness/"+PingProtocolSuffix, ProtocolVersion))
}
