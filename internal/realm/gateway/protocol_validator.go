package gateway

import (
	"fmt"
	"strings"

	"github.com/dep2p/go-dep2p/internal/realm/interfaces"
	"github.com/dep2p/go-dep2p/pkg/protocol"
)

// ============================================================================
//                              协议验证器
// ============================================================================

// ProtocolValidator 协议验证器
type ProtocolValidator struct {
	// 允许的协议前缀
	allowedPrefixes []string
}

// NewProtocolValidator 创建协议验证器
func NewProtocolValidator() *ProtocolValidator {
	return &ProtocolValidator{
		allowedPrefixes: []string{
			protocol.PrefixRealm + "/",
			protocol.PrefixApp + "/",
		},
	}
}

// ============================================================================
//                              协议验证
// ============================================================================

// ValidateProtocol 验证协议
func (pv *ProtocolValidator) ValidateProtocol(proto, realmID string) error {
	// 1. 检查是否是 Realm 协议
	if !pv.IsRealmProtocol(proto) {
		return ErrInvalidProtocol
	}

	// 2. 提取 RealmID
	extracted, err := pv.ExtractRealmID(proto)
	if err != nil {
		return err
	}

	// 3. 检查 RealmID 匹配
	if extracted != realmID {
		return ErrRealmMismatch
	}

	return nil
}

// IsRealmProtocol 判断是否 Realm 协议
func (pv *ProtocolValidator) IsRealmProtocol(proto string) bool {
	for _, prefix := range pv.allowedPrefixes {
		if strings.HasPrefix(proto, prefix) {
			return true
		}
	}

	// 系统协议（/dep2p/sys/*）由节点级 Relay 处理
	if strings.HasPrefix(proto, protocol.PrefixSys+"/") {
		return false
	}

	return false
}

// ExtractRealmID 提取 RealmID
func (pv *ProtocolValidator) ExtractRealmID(proto string) (string, error) {
	// 尝试从 /dep2p/realm/<realmID>/ 提取
	if strings.HasPrefix(proto, protocol.PrefixRealm+"/") {
		parts := strings.Split(proto, "/")
		if len(parts) >= 4 {
			return parts[3], nil
		}
	}

	// 尝试从 /dep2p/app/<realmID>/ 提取
	if strings.HasPrefix(proto, protocol.PrefixApp+"/") {
		parts := strings.Split(proto, "/")
		if len(parts) >= 4 {
			return parts[3], nil
		}
	}

	return "", fmt.Errorf("%w: cannot extract realm ID from protocol: %s", ErrInvalidProtocol, proto)
}

// 确保实现接口
var _ interfaces.ProtocolValidator = (*ProtocolValidator)(nil)
