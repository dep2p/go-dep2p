package multiaddr

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"strings"
)

// Multiaddr 是自描述的网络地址接口
type Multiaddr interface {
	// Bytes 返回二进制表示（不要修改返回的字节，可能是共享的）
	Bytes() []byte

	// String 返回字符串表示
	String() string

	// Equal 判断两个地址是否相等
	Equal(Multiaddr) bool

	// Protocols 返回地址包含的协议列表
	Protocols() []Protocol

	// Encapsulate 封装另一个地址
	Encapsulate(Multiaddr) Multiaddr

	// Decapsulate 解封装（移除匹配的后缀）
	Decapsulate(Multiaddr) Multiaddr

	// ValueForProtocol 获取指定协议代码的值
	ValueForProtocol(code int) (string, error)

	// ToTCPAddr 转换为 TCP 地址
	ToTCPAddr() (*net.TCPAddr, error)

	// ToUDPAddr 转换为 UDP 地址
	ToUDPAddr() (*net.UDPAddr, error)
}

// multiaddr 是 Multiaddr 接口的实现
type multiaddr struct {
	bytes []byte
}

// NewMultiaddr 从字符串创建多地址
func NewMultiaddr(s string) (Multiaddr, error) {
	b, err := stringToBytes(s)
	if err != nil {
		return nil, err
	}
	return &multiaddr{bytes: b}, nil
}

// NewMultiaddrBytes 从字节创建多地址
func NewMultiaddrBytes(b []byte) (Multiaddr, error) {
	if err := validateBytes(b); err != nil {
		return nil, err
	}
	// 复制一份避免外部修改
	buf := make([]byte, len(b))
	copy(buf, b)
	return &multiaddr{bytes: buf}, nil
}

// Cast 从字符串强制创建多地址（不验证）
// 警告：仅用于已知有效的地址
func Cast(b []byte) Multiaddr {
	return &multiaddr{bytes: b}
}

// Bytes 返回二进制表示
func (m *multiaddr) Bytes() []byte {
	return m.bytes
}

// String 返回字符串表示
func (m *multiaddr) String() string {
	s, err := bytesToString(m.bytes)
	if err != nil {
		// 这不应该发生，因为我们在构造时已经验证了
		panic(fmt.Errorf("multiaddr failed to convert to string: %w", err))
	}
	return s
}

// Equal 判断两个地址是否相等
func (m *multiaddr) Equal(other Multiaddr) bool {
	if other == nil {
		return false
	}
	return bytes.Equal(m.bytes, other.Bytes())
}

// Protocols 返回地址包含的协议列表
func (m *multiaddr) Protocols() []Protocol {
	var protocols []Protocol
	b := m.bytes

	for len(b) > 0 {
		// 读取协议代码
		code, n, err := readVarintCode(b)
		if err != nil {
			// 这不应该发生
			panic(err)
		}
		b = b[n:]

		// 获取协议
		proto := ProtocolWithCode(code)
		if proto.Code == 0 {
			panic(fmt.Errorf("unknown protocol code: %d", code))
		}
		protocols = append(protocols, proto)

		// 跳过协议数据
		if proto.Size != 0 {
			prefixLen, dataLen, err := sizeForAddr(proto, b)
			if err != nil {
				panic(err)
			}
			b = b[prefixLen+dataLen:]
		}
	}

	return protocols
}

// Encapsulate 封装另一个地址
func (m *multiaddr) Encapsulate(other Multiaddr) Multiaddr {
	if other == nil {
		return m
	}

	mb := m.bytes
	ob := other.Bytes()

	// 组合字节
	result := make([]byte, len(mb)+len(ob))
	copy(result, mb)
	copy(result[len(mb):], ob)

	return &multiaddr{bytes: result}
}

// Decapsulate 解封装（移除匹配的后缀）
func (m *multiaddr) Decapsulate(other Multiaddr) Multiaddr {
	if other == nil {
		return m
	}

	mb := m.bytes
	ob := other.Bytes()

	// 如果 other 比 m 长，无法解封装
	if len(ob) > len(mb) {
		return m
	}

	// 检查是否匹配后缀
	if bytes.Equal(mb[len(mb)-len(ob):], ob) {
		return &multiaddr{bytes: mb[:len(mb)-len(ob)]}
	}

	return m
}

// ValueForProtocol 获取指定协议代码的值
func (m *multiaddr) ValueForProtocol(code int) (string, error) {
	proto := ProtocolWithCode(code)
	if proto.Code == 0 {
		return "", fmt.Errorf("unknown protocol code: %d", code)
	}

	b := m.bytes

	for len(b) > 0 {
		// 读取协议代码
		currentCode, n, err := readVarintCode(b)
		if err != nil {
			return "", err
		}
		b = b[n:]

		currentProto := ProtocolWithCode(currentCode)
		if currentProto.Code == 0 {
			return "", fmt.Errorf("unknown protocol code: %d", currentCode)
		}

		// 如果协议无数据，继续
		if currentProto.Size == 0 {
			if currentCode == code {
				// 找到了，但无值
				return "", nil
			}
			continue
		}

		// 读取数据
		prefixLen, dataLen, err := sizeForAddr(currentProto, b)
		if err != nil {
			return "", err
		}

		valueBytes := b[prefixLen : prefixLen+dataLen]
		b = b[prefixLen+dataLen:]

		// 如果是我们要找的协议
		if currentCode == code {
			return currentProto.Transcoder.BytesToString(valueBytes)
		}
	}

	return "", fmt.Errorf("protocol %s not found in multiaddr", proto.Name)
}

// MarshalBinary 实现 encoding.BinaryMarshaler
func (m *multiaddr) MarshalBinary() ([]byte, error) {
	return m.Bytes(), nil
}

// UnmarshalBinary 实现 encoding.BinaryUnmarshaler
func (m *multiaddr) UnmarshalBinary(data []byte) error {
	ma, err := NewMultiaddrBytes(data)
	if err != nil {
		return err
	}
	*m = *(ma.(*multiaddr))
	return nil
}

// MarshalText 实现 encoding.TextMarshaler
func (m *multiaddr) MarshalText() ([]byte, error) {
	return []byte(m.String()), nil
}

// UnmarshalText 实现 encoding.TextUnmarshaler
func (m *multiaddr) UnmarshalText(data []byte) error {
	ma, err := NewMultiaddr(string(data))
	if err != nil {
		return err
	}
	*m = *(ma.(*multiaddr))
	return nil
}

// MarshalJSON 实现 json.Marshaler
func (m *multiaddr) MarshalJSON() ([]byte, error) {
	return json.Marshal(m.String())
}

// UnmarshalJSON 实现 json.Unmarshaler
func (m *multiaddr) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}

	ma, err := NewMultiaddr(s)
	if err != nil {
		return err
	}
	*m = *(ma.(*multiaddr))
	return nil
}

// splitString 是字符串分割的辅助函数（用于 protocols.go）
func splitString(s string) []string {
	parts := strings.Split(s, "/")
	result := make([]string, len(parts))
	copy(result, parts)
	return result
}
