// Package streams 实现流协议
package streams

import (
	"strings"

	"github.com/dep2p/go-dep2p/pkg/protocol"
)

const (
	// ProtocolVersion 协议版本
	ProtocolVersion = protocol.Version10
)

// buildProtocolID 构建完整协议ID
//
// 格式: /dep2p/app/<realmID>/streams/<protocol>/1.0.0
func buildProtocolID(realmID, protocolName string) string {
	return string(protocol.BuildAppProtocol(realmID, "streams/"+protocolName, ProtocolVersion))
}

// validateProtocol 验证协议名称
func validateProtocol(protocol string) error {
	if protocol == "" {
		return ErrEmptyProtocol
	}

	// 协议名称不能包含 '/'
	if strings.Contains(protocol, "/") {
		return ErrInvalidProtocol
	}

	return nil
}
