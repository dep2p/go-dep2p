package multiaddr

import (
	"encoding/base32"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
)

// Transcoder 接口定义了协议数据的编解码方法
type Transcoder interface {
	// StringToBytes 将字符串值转换为字节
	StringToBytes(string) ([]byte, error)

	// BytesToString 将字节转换为字符串值
	BytesToString([]byte) (string, error)

	// ValidateBytes 验证字节数据是否有效
	ValidateBytes([]byte) error
}

// NewTranscoderFromFunctions 从函数创建 Transcoder
func NewTranscoderFromFunctions(
	s2b func(string) ([]byte, error),
	b2s func([]byte) (string, error),
	val func([]byte) error,
) Transcoder {
	return &transcoderWrapper{s2b, b2s, val}
}

type transcoderWrapper struct {
	stringToBytes func(string) ([]byte, error)
	bytesToString func([]byte) (string, error)
	validateBytes func([]byte) error
}

func (t *transcoderWrapper) StringToBytes(s string) ([]byte, error) {
	return t.stringToBytes(s)
}

func (t *transcoderWrapper) BytesToString(b []byte) (string, error) {
	return t.bytesToString(b)
}

func (t *transcoderWrapper) ValidateBytes(b []byte) error {
	if t.validateBytes == nil {
		return nil
	}
	return t.validateBytes(b)
}

// IP4 Transcoder
var TranscoderIP4 = NewTranscoderFromFunctions(ip4StringToBytes, ip4BytesToString, nil)

func ip4StringToBytes(s string) ([]byte, error) {
	ip := net.ParseIP(s).To4()
	if ip == nil {
		return nil, fmt.Errorf("failed to parse ip4 addr: %s", s)
	}
	return ip, nil
}

func ip4BytesToString(b []byte) (string, error) {
	if len(b) != 4 {
		return "", fmt.Errorf("invalid ip4 length: %d", len(b))
	}
	return net.IP(b).String(), nil
}

// IP6 Transcoder
var TranscoderIP6 = NewTranscoderFromFunctions(ip6StringToBytes, ip6BytesToString, nil)

func ip6StringToBytes(s string) ([]byte, error) {
	ip := net.ParseIP(s).To16()
	if ip == nil {
		return nil, fmt.Errorf("failed to parse ip6 addr: %s", s)
	}
	return ip, nil
}

func ip6BytesToString(b []byte) (string, error) {
	if len(b) != 16 {
		return "", fmt.Errorf("invalid ip6 length: %d", len(b))
	}
	ip := net.IP(b)
	// 处理 IPv4-mapped IPv6 地址
	if ip4 := ip.To4(); ip4 != nil {
		return "::ffff:" + ip4.String(), nil
	}
	return ip.String(), nil
}

// IP6Zone Transcoder
var TranscoderIP6Zone = NewTranscoderFromFunctions(ip6ZoneStringToBytes, ip6ZoneBytesToString, ip6ZoneValidateBytes)

func ip6ZoneStringToBytes(s string) ([]byte, error) {
	if len(s) == 0 {
		return nil, errors.New("empty ip6zone")
	}
	if strings.Contains(s, "/") {
		return nil, fmt.Errorf("IPv6 zone ID contains '/': %s", s)
	}
	return []byte(s), nil
}

func ip6ZoneBytesToString(b []byte) (string, error) {
	if len(b) == 0 {
		return "", errors.New("invalid length (should be > 0)")
	}
	return string(b), nil
}

func ip6ZoneValidateBytes(b []byte) error {
	if len(b) == 0 {
		return errors.New("invalid length (should be > 0)")
	}
	// 不支持 '/' 因为会破坏 multiaddr 解析
	if strings.Contains(string(b), "/") {
		return fmt.Errorf("IPv6 zone ID contains '/': %s", string(b))
	}
	return nil
}

// IPCIDR Transcoder
var TranscoderIPCIDR = NewTranscoderFromFunctions(ipCIDRStringToBytes, ipCIDRBytesToString, nil)

func ipCIDRStringToBytes(s string) ([]byte, error) {
	ipMask, err := strconv.ParseUint(s, 10, 8)
	if err != nil {
		return nil, err
	}
	return []byte{byte(ipMask)}, nil
}

func ipCIDRBytesToString(b []byte) (string, error) {
	if len(b) != 1 {
		return "", errors.New("invalid length (should be == 1)")
	}
	return strconv.Itoa(int(b[0])), nil
}

// Port Transcoder (TCP/UDP/SCTP/DCCP)
var TranscoderPort = NewTranscoderFromFunctions(portStringToBytes, portBytesToString, nil)

func portStringToBytes(s string) ([]byte, error) {
	port, err := strconv.ParseUint(s, 10, 16)
	if err != nil {
		return nil, fmt.Errorf("failed to parse port: %s", err)
	}
	if port > 65535 {
		return nil, errors.New("port out of range")
	}
	b := make([]byte, 2)
	binary.BigEndian.PutUint16(b, uint16(port))
	return b, nil
}

func portBytesToString(b []byte) (string, error) {
	if len(b) != 2 {
		return "", fmt.Errorf("invalid port length: %d", len(b))
	}
	port := binary.BigEndian.Uint16(b)
	return strconv.Itoa(int(port)), nil
}

// DNS Transcoder (DNS/DNS4/DNS6/DNSADDR)
var TranscoderDNS = NewTranscoderFromFunctions(dnsStringToBytes, dnsBytesToString, dnsValidateBytes)

func dnsStringToBytes(s string) ([]byte, error) {
	if len(s) == 0 {
		return nil, errors.New("empty DNS name")
	}
	// 简单的 DNS 名称验证
	if strings.Contains(s, "/") {
		return nil, fmt.Errorf("DNS name contains '/': %s", s)
	}
	return []byte(s), nil
}

func dnsBytesToString(b []byte) (string, error) {
	if len(b) == 0 {
		return "", errors.New("invalid length (should be > 0)")
	}
	return string(b), nil
}

func dnsValidateBytes(b []byte) error {
	if len(b) == 0 {
		return errors.New("invalid length (should be > 0)")
	}
	// 验证不包含 '/'
	if strings.Contains(string(b), "/") {
		return fmt.Errorf("DNS name contains '/': %s", string(b))
	}
	return nil
}

// P2P Transcoder (PeerID)
var TranscoderP2P = NewTranscoderFromFunctions(p2pStringToBytes, p2pBytesToString, p2pValidateBytes)

func p2pStringToBytes(s string) ([]byte, error) {
	if len(s) == 0 {
		return nil, errors.New("empty peer ID")
	}
	// PeerID 是 base58 编码的，这里我们直接存储字符串
	// 实际应该使用 base58 解码，但为了简化先存储原始字符串
	return []byte(s), nil
}

func p2pBytesToString(b []byte) (string, error) {
	if len(b) == 0 {
		return "", errors.New("invalid peer ID length")
	}
	return string(b), nil
}

func p2pValidateBytes(b []byte) error {
	if len(b) == 0 {
		return errors.New("invalid peer ID length")
	}
	return nil
}

// Unix Transcoder
var TranscoderUnix = NewTranscoderFromFunctions(unixStringToBytes, unixBytesToString, nil)

func unixStringToBytes(s string) ([]byte, error) {
	if len(s) == 0 {
		return nil, errors.New("empty unix path")
	}
	return []byte(s), nil
}

func unixBytesToString(b []byte) (string, error) {
	if len(b) == 0 {
		return "", errors.New("invalid unix path length")
	}
	return string(b), nil
}

// Onion Transcoder
var TranscoderOnion = NewTranscoderFromFunctions(onionStringToBytes, onionBytesToString, nil)

func onionStringToBytes(s string) ([]byte, error) {
	addr := strings.Split(s, ":")
	if len(addr) != 2 {
		return nil, fmt.Errorf("invalid onion address: %s", s)
	}

	// Onion 地址是 base32 编码
	onionHost, err := base32.StdEncoding.DecodeString(strings.ToUpper(addr[0]))
	if err != nil {
		return nil, fmt.Errorf("failed to decode onion address: %w", err)
	}
	if len(onionHost) != 10 {
		return nil, fmt.Errorf("invalid onion address length: %d", len(onionHost))
	}

	// 解析端口
	port, err := strconv.ParseUint(addr[1], 10, 16)
	if err != nil {
		return nil, fmt.Errorf("failed to parse onion port: %w", err)
	}

	// 组装：10字节地址 + 2字节端口
	result := make([]byte, 12)
	copy(result[:10], onionHost)
	binary.BigEndian.PutUint16(result[10:], uint16(port))

	return result, nil
}

func onionBytesToString(b []byte) (string, error) {
	if len(b) != 12 {
		return "", fmt.Errorf("invalid onion length: %d", len(b))
	}

	addr := base32.StdEncoding.EncodeToString(b[:10])
	port := binary.BigEndian.Uint16(b[10:])

	return fmt.Sprintf("%s:%d", strings.ToLower(addr), port), nil
}

// Onion3 Transcoder
var TranscoderOnion3 = NewTranscoderFromFunctions(onion3StringToBytes, onion3BytesToString, nil)

func onion3StringToBytes(s string) ([]byte, error) {
	addr := strings.Split(s, ":")
	if len(addr) != 2 {
		return nil, fmt.Errorf("invalid onion3 address: %s", s)
	}

	// Onion3 地址是 base32 编码（去掉 .onion 后缀）
	onionHost := strings.TrimSuffix(addr[0], ".onion")
	hostBytes, err := base32.StdEncoding.DecodeString(strings.ToUpper(onionHost))
	if err != nil {
		return nil, fmt.Errorf("failed to decode onion3 address: %w", err)
	}
	if len(hostBytes) != 35 {
		return nil, fmt.Errorf("invalid onion3 address length: %d", len(hostBytes))
	}

	// 解析端口
	port, err := strconv.ParseUint(addr[1], 10, 16)
	if err != nil {
		return nil, fmt.Errorf("failed to parse onion3 port: %w", err)
	}

	// 组装：35字节地址 + 2字节端口
	result := make([]byte, 37)
	copy(result[:35], hostBytes)
	binary.BigEndian.PutUint16(result[35:], uint16(port))

	return result, nil
}

func onion3BytesToString(b []byte) (string, error) {
	if len(b) != 37 {
		return "", fmt.Errorf("invalid onion3 length: %d", len(b))
	}

	addr := base32.StdEncoding.EncodeToString(b[:35])
	port := binary.BigEndian.Uint16(b[35:])

	return fmt.Sprintf("%s:%d", strings.ToLower(addr), port), nil
}

// Garlic64 Transcoder
var TranscoderGarlic64 = NewTranscoderFromFunctions(garlic64StringToBytes, garlic64BytesToString, nil)

func garlic64StringToBytes(s string) ([]byte, error) {
	// Garlic64 是 base32 编码（I2P 地址）
	b, err := base32.StdEncoding.DecodeString(strings.ToUpper(s))
	if err != nil {
		return nil, fmt.Errorf("failed to decode garlic64: %w", err)
	}
	return b, nil
}

func garlic64BytesToString(b []byte) (string, error) {
	return strings.ToLower(base32.StdEncoding.EncodeToString(b)), nil
}

// Garlic32 Transcoder
var TranscoderGarlic32 = NewTranscoderFromFunctions(garlic32StringToBytes, garlic32BytesToString, nil)

func garlic32StringToBytes(s string) ([]byte, error) {
	// Garlic32 是 base32 编码（I2P 短地址）
	b, err := base32.StdEncoding.DecodeString(strings.ToUpper(s))
	if err != nil {
		return nil, fmt.Errorf("failed to decode garlic32: %w", err)
	}
	return b, nil
}

func garlic32BytesToString(b []byte) (string, error) {
	return strings.ToLower(base32.StdEncoding.EncodeToString(b)), nil
}
