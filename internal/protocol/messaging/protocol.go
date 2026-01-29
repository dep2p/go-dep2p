// Package messaging 实现点对点消息传递协议
package messaging

import (
	"fmt"
	"strings"

	"github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/protocol"
)

// buildProtocolID 构造协议 ID
//
// 生成格式: /dep2p/app/<realmID>/<protocol>/1.0.0
func buildProtocolID(realmID, protocolName string) interfaces.ProtocolID {
	return interfaces.ProtocolID(protocol.BuildAppProtocol(realmID, protocolName, protocol.Version10))
}

// validateProtocol 验证协议格式
//
// 协议格式要求:
// - 不能为空
// - 不能包含空格
// - 建议使用小写字母和下划线
func validateProtocol(protocol string) error {
	if protocol == "" {
		return fmt.Errorf("%w: protocol is empty", ErrInvalidProtocol)
	}

	if strings.Contains(protocol, " ") {
		return fmt.Errorf("%w: protocol contains whitespace", ErrInvalidProtocol)
	}

	return nil
}
