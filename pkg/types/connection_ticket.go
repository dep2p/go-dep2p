package types

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// ============================================================================
//                          ConnectionTicket
// ============================================================================

// ConnectionTicket 连接票据
//
// 用户友好的连接信息格式，便于分享（聊天/二维码）。
//
// 设计理念：
//   - NodeID 是唯一必需字段（身份优先）
//   - AddressHints 是可选提示（加速连接）
//   - 中继信息由系统自动发现，不包含在票据中
type ConnectionTicket struct {
	// NodeID 节点身份（必需）
	NodeID string `json:"node_id"`

	// AddressHints 地址提示（可选）
	//
	// 用于加速连接，可以为空（系统通过 DHT 发现）。
	AddressHints []string `json:"address_hints,omitempty"`

	// RealmID 所属 Realm（可选）
	//
	// 用于 Realm 内发现优化。
	RealmID string `json:"realm_id,omitempty"`

	// Timestamp 生成时间（可选）
	//
	// 用于过期检查。
	Timestamp int64 `json:"timestamp,omitempty"`
}

// NewConnectionTicket 创建连接票据
func NewConnectionTicket(nodeID string, addressHints []string) *ConnectionTicket {
	return &ConnectionTicket{
		NodeID:       nodeID,
		AddressHints: addressHints,
		Timestamp:    time.Now().Unix(),
	}
}

// Encode 编码为字符串
//
// 格式：dep2p://base64url(json(ticket))
func (t *ConnectionTicket) Encode() (string, error) {
	// 序列化为 JSON
	data, err := json.Marshal(t)
	if err != nil {
		return "", fmt.Errorf("marshal ticket: %w", err)
	}

	// Base64 URL 编码（去除 padding）
	encoded := base64.RawURLEncoding.EncodeToString(data)

	return "dep2p://" + encoded, nil
}

// DecodeConnectionTicket 从字符串解码连接票据
//
// 安全检查：
//   - 前缀验证
//   - Base64 解码
//   - JSON 解析
//   - NodeID 格式验证（非空、长度、Base58）
//   - 地址格式基本检查
func DecodeConnectionTicket(s string) (*ConnectionTicket, error) {
	// 预处理：去除空格
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, fmt.Errorf("invalid ticket: empty string")
	}

	// 检查前缀
	if !strings.HasPrefix(s, "dep2p://") {
		return nil, fmt.Errorf("invalid ticket format: missing dep2p:// prefix")
	}

	// 移除前缀
	encoded := strings.TrimPrefix(s, "dep2p://")
	if encoded == "" {
		return nil, fmt.Errorf("invalid ticket: empty payload")
	}

	// 长度检查（防止超长攻击，Base64 编码的合理票据不应超过 2KB）
	if len(encoded) > 2048 {
		return nil, fmt.Errorf("invalid ticket: payload too long (%d > 2048)", len(encoded))
	}

	// Base64 URL 解码
	data, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("decode ticket: %w", err)
	}

	// 解析 JSON
	var ticket ConnectionTicket
	if err := json.Unmarshal(data, &ticket); err != nil {
		return nil, fmt.Errorf("unmarshal ticket: %w", err)
	}

	// 验证 NodeID：非空
	if ticket.NodeID == "" {
		return nil, fmt.Errorf("invalid ticket: missing node_id")
	}

	// 验证 NodeID：长度检查（Base58 编码的 32 字节哈希约 43-44 字符）
	if len(ticket.NodeID) < 20 || len(ticket.NodeID) > 100 {
		return nil, fmt.Errorf("invalid ticket: node_id length invalid (%d)", len(ticket.NodeID))
	}

	// 验证 NodeID：Base58 格式
	peerID := PeerID(ticket.NodeID)
	if err := peerID.Validate(); err != nil {
		return nil, fmt.Errorf("invalid ticket: node_id format invalid: %w", err)
	}

	// 过滤无效地址（可选字段，但如果提供了就要检查）
	if len(ticket.AddressHints) > 0 {
		validAddrs := make([]string, 0, len(ticket.AddressHints))
		for _, addr := range ticket.AddressHints {
			// 基本格式检查
			addr = strings.TrimSpace(addr)
			if addr == "" {
				continue
			}
			// 长度检查
			if len(addr) > 500 {
				continue
			}
			// 必须以 / 开头
			if !strings.HasPrefix(addr, "/") {
				continue
			}
			// 禁止危险字符
			if strings.ContainsAny(addr, ";|&$`\n\r\\") {
				continue
			}
			validAddrs = append(validAddrs, addr)
		}
		ticket.AddressHints = validAddrs
	}

	return &ticket, nil
}

// IsExpired 检查票据是否过期
func (t *ConnectionTicket) IsExpired(maxAge time.Duration) bool {
	if t.Timestamp == 0 {
		return false // 无时间戳，不过期
	}

	age := time.Since(time.Unix(t.Timestamp, 0))
	return age > maxAge
}

// ============================================================================
//                          BootstrapCandidate
// ============================================================================

// BootstrapCandidate 引导候选
//
// 与 ShareableAddrs 正交：
//   - ShareableAddrs: 严格验证，可入 DHT
//   - BootstrapCandidate: 旁路，包含所有候选（直连+中继）
type BootstrapCandidate struct {
	// NodeID 节点 ID
	NodeID string `json:"node_id"`

	// Addrs 候选地址列表
	Addrs []string `json:"addrs"`

	// Type 候选类型
	Type BootstrapCandidateType `json:"type"`
}

// BootstrapCandidateType 候选类型
type BootstrapCandidateType int

const (
	// BootstrapCandidateUnknown 未知类型
	BootstrapCandidateUnknown BootstrapCandidateType = iota

	// BootstrapCandidateDirect 直连候选
	BootstrapCandidateDirect

	// BootstrapCandidateRelay 中继候选
	BootstrapCandidateRelay
)

func (t BootstrapCandidateType) String() string {
	switch t {
	case BootstrapCandidateDirect:
		return "Direct"
	case BootstrapCandidateRelay:
		return "Relay"
	default:
		return "Unknown"
	}
}
