package multiaddr

// Protocol 描述一个 multiaddr 协议
type Protocol struct {
	// Name 协议名称（如 "ip4", "tcp"）
	Name string

	// Code 协议代码
	Code int

	// VCode 预计算的 varint 编码
	VCode []byte

	// Size 协议数据大小（位）
	// 0 表示无数据
	// -1 表示变长（length-prefixed）
	Size int

	// Path 是否为路径协议（终端协议）
	Path bool

	// Transcoder 编解码器
	Transcoder Transcoder
}

// String 返回协议名称
func (p Protocol) String() string {
	return p.Name
}

// LengthPrefixedVarSize 表示变长数据（使用 varint 前缀）
const LengthPrefixedVarSize = -1

// 协议代码常量（与 multiformats/multicodec 对齐）
// 参考：https://github.com/multiformats/multicodec/blob/master/table.csv
const (
	P_IP4         = 0x0004
	P_TCP         = 0x0006
	P_UDP         = 0x0111
	P_DCCP        = 0x0021
	P_IP6         = 0x0029
	P_IP6ZONE     = 0x002A
	P_IPCIDR      = 0x002B
	P_DNS         = 0x0035
	P_DNS4        = 0x0036
	P_DNS6        = 0x0037
	P_DNSADDR     = 0x0038
	P_SCTP        = 0x0084
	P_UTP         = 0x012E
	P_UDT         = 0x012D
	P_UNIX        = 0x0190
	P_P2P         = 0x01A5
	P_IPFS        = 0x01A5 // 向后兼容别名
	P_HTTP        = 0x01E0
	P_HTTPS       = 0x01BB
	P_TLS         = 0x01C0
	P_NOISE       = 0x01C6
	P_QUIC        = 0x01CC
	P_QUIC_V1     = 0x01CD
	P_WS          = 0x01DD
	P_WSS         = 0x01DE
	P_ONION       = 0x01BC
	P_ONION3      = 0x01BD
	P_GARLIC64    = 0x01BE
	P_GARLIC32    = 0x01BF
	P_P2P_CIRCUIT = 0x0122
	P_CIRCUIT     = 0x0122 // 别名
	P_WEBTRANSPORT    = 0x01D2
	P_WEBRTC          = 0x0118
	P_WEBRTC_DIRECT   = 0x0119
	P_P2P_WEBRTC_DIRECT = 0x0119 // 别名
)

var (
	protoIP4 = Protocol{
		Name:       "ip4",
		Code:       P_IP4,
		VCode:      codeToVarint(P_IP4),
		Size:       32,
		Transcoder: TranscoderIP4,
	}

	protoTCP = Protocol{
		Name:       "tcp",
		Code:       P_TCP,
		VCode:      codeToVarint(P_TCP),
		Size:       16,
		Transcoder: TranscoderPort,
	}

	protoUDP = Protocol{
		Name:       "udp",
		Code:       P_UDP,
		VCode:      codeToVarint(P_UDP),
		Size:       16,
		Transcoder: TranscoderPort,
	}

	protoDCCP = Protocol{
		Name:       "dccp",
		Code:       P_DCCP,
		VCode:      codeToVarint(P_DCCP),
		Size:       16,
		Transcoder: TranscoderPort,
	}

	protoIP6 = Protocol{
		Name:       "ip6",
		Code:       P_IP6,
		VCode:      codeToVarint(P_IP6),
		Size:       128,
		Transcoder: TranscoderIP6,
	}

	protoIP6ZONE = Protocol{
		Name:       "ip6zone",
		Code:       P_IP6ZONE,
		VCode:      codeToVarint(P_IP6ZONE),
		Size:       LengthPrefixedVarSize,
		Transcoder: TranscoderIP6Zone,
	}

	protoIPCIDR = Protocol{
		Name:       "ipcidr",
		Code:       P_IPCIDR,
		VCode:      codeToVarint(P_IPCIDR),
		Size:       8,
		Transcoder: TranscoderIPCIDR,
	}

	protoDNS = Protocol{
		Name:       "dns",
		Code:       P_DNS,
		VCode:      codeToVarint(P_DNS),
		Size:       LengthPrefixedVarSize,
		Transcoder: TranscoderDNS,
	}

	protoDNS4 = Protocol{
		Name:       "dns4",
		Code:       P_DNS4,
		VCode:      codeToVarint(P_DNS4),
		Size:       LengthPrefixedVarSize,
		Transcoder: TranscoderDNS,
	}

	protoDNS6 = Protocol{
		Name:       "dns6",
		Code:       P_DNS6,
		VCode:      codeToVarint(P_DNS6),
		Size:       LengthPrefixedVarSize,
		Transcoder: TranscoderDNS,
	}

	protoDNSADDR = Protocol{
		Name:       "dnsaddr",
		Code:       P_DNSADDR,
		VCode:      codeToVarint(P_DNSADDR),
		Size:       LengthPrefixedVarSize,
		Transcoder: TranscoderDNS,
	}

	protoSCTP = Protocol{
		Name:       "sctp",
		Code:       P_SCTP,
		VCode:      codeToVarint(P_SCTP),
		Size:       16,
		Transcoder: TranscoderPort,
	}

	protoUTP = Protocol{
		Name:  "utp",
		Code:  P_UTP,
		VCode: codeToVarint(P_UTP),
		Size:  0,
	}

	protoUDT = Protocol{
		Name:  "udt",
		Code:  P_UDT,
		VCode: codeToVarint(P_UDT),
		Size:  0,
	}

	protoUNIX = Protocol{
		Name:       "unix",
		Code:       P_UNIX,
		VCode:      codeToVarint(P_UNIX),
		Size:       LengthPrefixedVarSize,
		Path:       true,
		Transcoder: TranscoderUnix,
	}

	protoP2P = Protocol{
		Name:       "p2p",
		Code:       P_P2P,
		VCode:      codeToVarint(P_P2P),
		Size:       LengthPrefixedVarSize,
		Transcoder: TranscoderP2P,
	}

	protoHTTP = Protocol{
		Name:  "http",
		Code:  P_HTTP,
		VCode: codeToVarint(P_HTTP),
		Size:  0,
	}

	protoHTTPS = Protocol{
		Name:  "https",
		Code:  P_HTTPS,
		VCode: codeToVarint(P_HTTPS),
		Size:  0,
	}

	protoTLS = Protocol{
		Name:  "tls",
		Code:  P_TLS,
		VCode: codeToVarint(P_TLS),
		Size:  0,
	}

	protoNOISE = Protocol{
		Name:  "noise",
		Code:  P_NOISE,
		VCode: codeToVarint(P_NOISE),
		Size:  0,
	}

	protoQUIC = Protocol{
		Name:  "quic",
		Code:  P_QUIC,
		VCode: codeToVarint(P_QUIC),
		Size:  0,
	}

	protoQUIC_V1 = Protocol{
		Name:  "quic-v1",
		Code:  P_QUIC_V1,
		VCode: codeToVarint(P_QUIC_V1),
		Size:  0,
	}

	protoWS = Protocol{
		Name:  "ws",
		Code:  P_WS,
		VCode: codeToVarint(P_WS),
		Size:  0,
	}

	protoWSS = Protocol{
		Name:  "wss",
		Code:  P_WSS,
		VCode: codeToVarint(P_WSS),
		Size:  0,
	}

	protoONION = Protocol{
		Name:       "onion",
		Code:       P_ONION,
		VCode:      codeToVarint(P_ONION),
		Size:       96,
		Transcoder: TranscoderOnion,
	}

	protoONION3 = Protocol{
		Name:       "onion3",
		Code:       P_ONION3,
		VCode:      codeToVarint(P_ONION3),
		Size:       296,
		Transcoder: TranscoderOnion3,
	}

	protoGARLIC64 = Protocol{
		Name:       "garlic64",
		Code:       P_GARLIC64,
		VCode:      codeToVarint(P_GARLIC64),
		Size:       LengthPrefixedVarSize,
		Transcoder: TranscoderGarlic64,
	}

	protoGARLIC32 = Protocol{
		Name:       "garlic32",
		Code:       P_GARLIC32,
		VCode:      codeToVarint(P_GARLIC32),
		Size:       LengthPrefixedVarSize,
		Transcoder: TranscoderGarlic32,
	}

	protoP2P_CIRCUIT = Protocol{
		Name:  "p2p-circuit",
		Code:  P_P2P_CIRCUIT,
		VCode: codeToVarint(P_P2P_CIRCUIT),
		Size:  0,
	}
)

// protocols 协议注册表（按代码索引）
var protocols = map[int]Protocol{
	P_IP4:         protoIP4,
	P_TCP:         protoTCP,
	P_UDP:         protoUDP,
	P_DCCP:        protoDCCP,
	P_IP6:         protoIP6,
	P_IP6ZONE:     protoIP6ZONE,
	P_IPCIDR:      protoIPCIDR,
	P_DNS:         protoDNS,
	P_DNS4:        protoDNS4,
	P_DNS6:        protoDNS6,
	P_DNSADDR:     protoDNSADDR,
	P_SCTP:        protoSCTP,
	P_UTP:         protoUTP,
	P_UDT:         protoUDT,
	P_UNIX:        protoUNIX,
	P_P2P:         protoP2P,
	P_HTTP:        protoHTTP,
	P_HTTPS:       protoHTTPS,
	P_TLS:         protoTLS,
	P_NOISE:       protoNOISE,
	P_QUIC:        protoQUIC,
	P_QUIC_V1:     protoQUIC_V1,
	P_WS:          protoWS,
	P_WSS:         protoWSS,
	P_ONION:       protoONION,
	P_ONION3:      protoONION3,
	P_GARLIC64:    protoGARLIC64,
	P_GARLIC32:    protoGARLIC32,
	P_P2P_CIRCUIT: protoP2P_CIRCUIT,
}

// protocolsByName 协议注册表（按名称索引）
var protocolsByName = map[string]Protocol{
	"ip4":         protoIP4,
	"tcp":         protoTCP,
	"udp":         protoUDP,
	"dccp":        protoDCCP,
	"ip6":         protoIP6,
	"ip6zone":     protoIP6ZONE,
	"ipcidr":      protoIPCIDR,
	"dns":         protoDNS,
	"dns4":        protoDNS4,
	"dns6":        protoDNS6,
	"dnsaddr":     protoDNSADDR,
	"sctp":        protoSCTP,
	"utp":         protoUTP,
	"udt":         protoUDT,
	"unix":        protoUNIX,
	"p2p":         protoP2P,
	"ipfs":        protoP2P, // 别名
	"http":        protoHTTP,
	"https":       protoHTTPS,
	"tls":         protoTLS,
	"noise":       protoNOISE,
	"quic":        protoQUIC,
	"quic-v1":     protoQUIC_V1,
	"ws":          protoWS,
	"wss":         protoWSS,
	"onion":       protoONION,
	"onion3":      protoONION3,
	"garlic64":    protoGARLIC64,
	"garlic32":    protoGARLIC32,
	"p2p-circuit": protoP2P_CIRCUIT,
}

// ProtocolWithCode 根据协议代码获取协议
// 如果协议不存在，返回零值协议（Code = 0）
func ProtocolWithCode(code int) Protocol {
	if proto, ok := protocols[code]; ok {
		return proto
	}
	return Protocol{}
}

// ProtocolWithName 根据协议名称获取协议
// 如果协议不存在，返回零值协议（Code = 0）
func ProtocolWithName(name string) Protocol {
	if proto, ok := protocolsByName[name]; ok {
		return proto
	}
	return Protocol{}
}

// ProtocolsWithString 返回多地址字符串中的所有协议名称
func ProtocolsWithString(s string) ([]string, error) {
	ps := []string{}
	parts := splitString(s)

	if len(parts) == 0 {
		return nil, nil
	}

	// 跳过第一个空字符串
	for i := 1; i < len(parts); i += 2 {
		proto := ProtocolWithName(parts[i])
		if proto.Code == 0 {
			return nil, ErrInvalidProtocol
		}
		ps = append(ps, proto.Name)
	}

	return ps, nil
}
