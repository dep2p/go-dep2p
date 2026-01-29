package multiaddr

import (
	"bytes"
	"fmt"
	"strings"
)

// stringToBytes 将多地址字符串转换为二进制格式
func stringToBytes(s string) ([]byte, error) {
	// 去除尾部斜杠
	s = strings.TrimRight(s, "/")

	if len(s) == 0 {
		return nil, fmt.Errorf("empty multiaddr")
	}

	if !strings.HasPrefix(s, "/") {
		return nil, fmt.Errorf("multiaddr must begin with /")
	}

	var buf bytes.Buffer
	parts := strings.Split(s, "/")

	// 跳过第一个空元素
	parts = parts[1:]

	if len(parts) == 0 {
		return nil, fmt.Errorf("empty multiaddr")
	}

	// 解析每个协议及其值
	for len(parts) > 0 {
		name := parts[0]
		proto := ProtocolWithName(name)
		if proto.Code == 0 {
			return nil, fmt.Errorf("unknown protocol: %s", name)
		}

		// 写入协议代码（varint）
		buf.Write(proto.VCode)
		parts = parts[1:]

		// 如果协议无数据，继续下一个
		if proto.Size == 0 {
			continue
		}

		// 协议需要值
		if len(parts) < 1 {
			return nil, fmt.Errorf("protocol %s requires a value", name)
		}

		// 如果是路径协议，消费剩余所有部分
		if proto.Path {
			parts = []string{"/" + strings.Join(parts, "/")}
		}

		// 使用 transcoder 转换值
		value := parts[0]
		valueBytes, err := proto.Transcoder.StringToBytes(value)
		if err != nil {
			return nil, fmt.Errorf("failed to convert value for protocol %s: %w", name, err)
		}

		// 如果是变长协议，写入长度前缀
		if proto.Size == LengthPrefixedVarSize {
			buf.Write(uvarintEncode(uint64(len(valueBytes))))
		}

		// 写入值
		buf.Write(valueBytes)
		parts = parts[1:]
	}

	return buf.Bytes(), nil
}

// bytesToString 将二进制格式的多地址转换为字符串
func bytesToString(b []byte) (string, error) {
	if len(b) == 0 {
		return "", fmt.Errorf("empty multiaddr bytes")
	}

	var sb strings.Builder

	for len(b) > 0 {
		// 读取协议代码
		code, n, err := readVarintCode(b)
		if err != nil {
			return "", fmt.Errorf("failed to read protocol code: %w", err)
		}
		b = b[n:]

		// 获取协议
		proto := ProtocolWithCode(code)
		if proto.Code == 0 {
			return "", fmt.Errorf("unknown protocol code: %d", code)
		}

		// 写入协议名称
		sb.WriteString("/")
		sb.WriteString(proto.Name)

		// 如果协议无数据，继续
		if proto.Size == 0 {
			continue
		}

		// 读取数据大小
		var size int
		if proto.Size == LengthPrefixedVarSize {
			// 变长：读取长度前缀
			length, bytesRead, err := uvarintDecode(b)
			if err != nil {
				return "", fmt.Errorf("failed to read length for protocol %s: %w", proto.Name, err)
			}
			b = b[bytesRead:]
			size = int(length)
		} else {
			// 固定长度（位转字节）
			size = proto.Size / 8
		}

		// 验证数据长度
		if len(b) < size {
			return "", fmt.Errorf("insufficient data for protocol %s: need %d, have %d", proto.Name, size, len(b))
		}

		// 读取数据
		valueBytes := b[:size]
		b = b[size:]

		// 验证数据
		if err := proto.Transcoder.ValidateBytes(valueBytes); err != nil {
			return "", fmt.Errorf("invalid data for protocol %s: %w", proto.Name, err)
		}

		// 转换为字符串
		valueStr, err := proto.Transcoder.BytesToString(valueBytes)
		if err != nil {
			return "", fmt.Errorf("failed to convert bytes for protocol %s: %w", proto.Name, err)
		}

		// 写入值
		sb.WriteString("/")
		sb.WriteString(valueStr)
	}

	return sb.String(), nil
}

// validateBytes 验证二进制多地址的格式
func validateBytes(b []byte) error {
	if len(b) == 0 {
		return fmt.Errorf("empty multiaddr")
	}

	for len(b) > 0 {
		// 读取协议代码
		code, n, err := readVarintCode(b)
		if err != nil {
			return fmt.Errorf("invalid protocol code: %w", err)
		}
		b = b[n:]

		// 获取协议
		proto := ProtocolWithCode(code)
		if proto.Code == 0 {
			return fmt.Errorf("unknown protocol code: %d", code)
		}

		// 如果协议无数据，继续
		if proto.Size == 0 {
			continue
		}

		// 确定数据大小
		var size int
		if proto.Size == LengthPrefixedVarSize {
			// 变长：读取长度前缀
			length, bytesRead, err := uvarintDecode(b)
			if err != nil {
				return fmt.Errorf("failed to read length for protocol %s: %w", proto.Name, err)
			}
			b = b[bytesRead:]
			size = int(length)
		} else {
			// 固定长度
			size = proto.Size / 8
		}

		// 验证数据长度
		if len(b) < size {
			return fmt.Errorf("insufficient data for protocol %s: need %d, have %d", proto.Name, size, len(b))
		}

		// 验证数据
		valueBytes := b[:size]
		if err := proto.Transcoder.ValidateBytes(valueBytes); err != nil {
			return fmt.Errorf("invalid data for protocol %s: %w", proto.Name, err)
		}

		b = b[size:]
	}

	return nil
}

// sizeForAddr 计算协议数据部分的大小
// 返回：(length_prefix_bytes, data_bytes, error)
func sizeForAddr(proto Protocol, b []byte) (int, int, error) {
	if proto.Size == 0 {
		return 0, 0, nil
	}

	if proto.Size == LengthPrefixedVarSize {
		// 读取长度前缀
		length, n, err := uvarintDecode(b)
		if err != nil {
			return 0, 0, err
		}
		return n, int(length), nil
	}

	// 固定大小（位转字节）
	return 0, proto.Size / 8, nil
}
