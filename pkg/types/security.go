package types

// ============================================================================
//                              ConnectionState - 安全连接状态
// ============================================================================

// ConnectionState 安全连接状态
// 描述安全握手后的连接状态信息
type ConnectionState struct {
	// Protocol 使用的安全协议（如 "tls", "noise"）
	Protocol string

	// Version 协议版本（如 "1.3" for TLS 1.3）
	Version string

	// CipherSuite 加密套件名称
	CipherSuite string

	// PeerCertificates 对端证书链（DER 编码）
	PeerCertificates [][]byte

	// DidResume 是否是恢复的会话
	DidResume bool
}

// HasCertificates 检查是否有对端证书
func (cs ConnectionState) HasCertificates() bool {
	return len(cs.PeerCertificates) > 0
}

