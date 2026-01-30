package dep2p

import (
	"strings"

	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// ════════════════════════════════════════════════════════════════════════════
//                              内部辅助函数
// ════════════════════════════════════════════════════════════════════════════
//
// 本文件包含不导出的内部辅助函数，供 Node 及其扩展方法使用。
// 这些函数不构成公开 API 的一部分。

// deriveRealmID 从 realmKey 派生 RealmID
//
// 使用标准的 HKDF-SHA256 派生算法（与 internal/realm/auth 保持一致）。
// 格式：64 字符十六进制字符串
func deriveRealmID(realmKey []byte) string {
	if len(realmKey) == 0 {
		return ""
	}

	// 使用 types.RealmIDFromPSK 标准实现（HKDF-SHA256）
	psk := types.PSK(realmKey)
	realmID := psk.DeriveRealmID()
	return string(realmID)
}

// isPublicAddr 检查地址是否为公网地址
//
// 过滤掉私网、回环、link-local 地址。
func isPublicAddr(addr string) bool {
	// 回环地址
	if strings.Contains(addr, "/ip4/127.") || strings.Contains(addr, "/ip6/::1") {
		return false
	}

	// 私网地址（RFC 1918）
	if strings.Contains(addr, "/ip4/10.") ||
		strings.Contains(addr, "/ip4/172.16.") ||
		strings.Contains(addr, "/ip4/172.17.") ||
		strings.Contains(addr, "/ip4/172.18.") ||
		strings.Contains(addr, "/ip4/172.19.") ||
		strings.Contains(addr, "/ip4/172.20.") ||
		strings.Contains(addr, "/ip4/172.21.") ||
		strings.Contains(addr, "/ip4/172.22.") ||
		strings.Contains(addr, "/ip4/172.23.") ||
		strings.Contains(addr, "/ip4/172.24.") ||
		strings.Contains(addr, "/ip4/172.25.") ||
		strings.Contains(addr, "/ip4/172.26.") ||
		strings.Contains(addr, "/ip4/172.27.") ||
		strings.Contains(addr, "/ip4/172.28.") ||
		strings.Contains(addr, "/ip4/172.29.") ||
		strings.Contains(addr, "/ip4/172.30.") ||
		strings.Contains(addr, "/ip4/172.31.") ||
		strings.Contains(addr, "/ip4/192.168.") {
		return false
	}

	// Link-local 地址
	if strings.Contains(addr, "/ip4/169.254.") || strings.Contains(addr, "/ip6/fe80:") {
		return false
	}

	return true
}

// buildCircuitAddr 构建中继电路地址
//
// 格式：{relay-addr}/p2p-circuit/p2p/{local-id}
//
// 参数：
//   - relayAddr: Relay 节点的完整地址（含 /p2p/{relay-id}）
//   - localID: 本地节点 ID
//
// 返回：
//   - 完整的电路地址，或空字符串（如果构建失败）
func buildCircuitAddr(relayAddr string, localID string) string {
	if relayAddr == "" || localID == "" {
		return ""
	}

	// 验证 relayAddr 包含 /p2p/ 组件
	if !strings.Contains(relayAddr, "/p2p/") {
		return ""
	}

	// 构建电路地址：{relay-addr}/p2p-circuit/p2p/{local-id}
	return relayAddr + "/p2p-circuit/p2p/" + localID
}

// containsAddr 检查地址列表是否包含指定地址
func containsAddr(addrs []string, target string) bool {
	for _, addr := range addrs {
		if addr == target {
			return true
		}
	}
	return false
}

// directionToString 将 Direction 枚举转换为字符串
func directionToString(dir pkgif.Direction) string {
	switch dir {
	case pkgif.DirInbound:
		return "inbound"
	case pkgif.DirOutbound:
		return "outbound"
	default:
		return "unknown"
	}
}
