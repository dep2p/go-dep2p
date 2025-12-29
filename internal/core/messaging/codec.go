// Package messaging 提供消息服务模块的实现
package messaging

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"

	messagingif "github.com/dep2p/go-dep2p/pkg/interfaces/messaging"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// 消息大小限制
const (
	// MaxMessageLength 最大消息长度 (10 MB)
	MaxMessageLength uint32 = 10 * 1024 * 1024
)

// ErrMessageTooLarge 消息过大错误
var ErrMessageTooLarge = errors.New("message too large")

// ============================================================================
//                              编解码辅助
// ============================================================================

// writeRequest 写入请求
func writeRequest(w io.Writer, req *types.Request) error {
	// 写入请求 ID
	if err := writeUint64(w, req.ID); err != nil {
		return err
	}
	// 写入协议
	if err := writeString(w, string(req.Protocol)); err != nil {
		return err
	}
	// 写入数据
	return writeBytes(w, req.Data)
}

// readRequest 读取请求
func readRequest(r io.Reader) (*types.Request, error) {
	req := &types.Request{}

	// 读取请求 ID
	id, err := readUint64(r)
	if err != nil {
		return nil, err
	}
	req.ID = id

	// 读取协议
	protocol, err := readString(r)
	if err != nil {
		return nil, err
	}
	req.Protocol = types.ProtocolID(protocol)

	// 读取数据
	data, err := readBytes(r)
	if err != nil {
		return nil, err
	}
	req.Data = data

	return req, nil
}

// writeResponse 写入响应
func writeResponse(w io.Writer, resp *types.Response) error {
	// 写入状态
	if err := binary.Write(w, binary.BigEndian, resp.Status); err != nil {
		return err
	}
	// 写入数据
	if err := writeBytes(w, resp.Data); err != nil {
		return err
	}
	// 写入错误
	return writeString(w, resp.Error)
}

// readResponse 读取响应
func readResponse(r io.Reader) (*types.Response, error) {
	resp := &types.Response{}

	// 读取状态
	if err := binary.Read(r, binary.BigEndian, &resp.Status); err != nil {
		return nil, err
	}

	// 读取数据
	data, err := readBytes(r)
	if err != nil {
		return nil, err
	}
	resp.Data = data

	// 读取错误
	errStr, err := readString(r)
	if err != nil {
		return nil, err
	}
	resp.Error = errStr

	return resp, nil
}

// writeMessage 写入消息
func writeMessage(w io.Writer, msg *types.Message) error {
	// 写入消息 ID
	if err := writeBytes(w, msg.ID); err != nil {
		return err
	}
	// 写入主题
	if err := writeString(w, msg.Topic); err != nil {
		return err
	}
	// 写入发送者
	if _, err := w.Write(msg.From[:]); err != nil {
		return err
	}
	// 写入数据
	return writeBytes(w, msg.Data)
}

// readMessage 读取消息
func readMessage(r io.Reader) (*types.Message, error) {
	msg := &types.Message{}

	// 读取消息 ID
	id, err := readBytes(r)
	if err != nil {
		return nil, err
	}
	msg.ID = id

	// 读取主题
	topic, err := readString(r)
	if err != nil {
		return nil, err
	}
	msg.Topic = topic

	// 读取发送者
	if _, err := io.ReadFull(r, msg.From[:]); err != nil {
		return nil, err
	}

	// 读取数据
	data, err := readBytes(r)
	if err != nil {
		return nil, err
	}
	msg.Data = data

	return msg, nil
}

// writeBytes 写入字节数组
func writeBytes(w io.Writer, data []byte) error {
	// 写入长度
	if err := binary.Write(w, binary.BigEndian, uint32(len(data))); err != nil {
		return err
	}
	// 写入数据
	if len(data) > 0 {
		if _, err := w.Write(data); err != nil {
			return err
		}
	}
	return nil
}

// readBytes 读取字节数组
func readBytes(r io.Reader) ([]byte, error) {
	var length uint32
	if err := binary.Read(r, binary.BigEndian, &length); err != nil {
		return nil, err
	}

	if length == 0 {
		return nil, nil
	}

	// 检查消息长度，防止内存耗尽攻击
	if length > MaxMessageLength {
		return nil, fmt.Errorf("%w: %d > %d", ErrMessageTooLarge, length, MaxMessageLength)
	}

	data := make([]byte, length)
	if _, err := io.ReadFull(r, data); err != nil {
		return nil, err
	}

	return data, nil
}

// writeString 写入字符串
func writeString(w io.Writer, str string) error {
	return writeBytes(w, []byte(str))
}

// readString 读取字符串
func readString(r io.Reader) (string, error) {
	data, err := readBytes(r)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// writeUint64 写入 uint64
func writeUint64(w io.Writer, v uint64) error {
	return binary.Write(w, binary.BigEndian, v)
}

// readUint64 读取 uint64
func readUint64(r io.Reader) (uint64, error) {
	var v uint64
	if err := binary.Read(r, binary.BigEndian, &v); err != nil {
		return 0, err
	}
	return v, nil
}

// msgIDToKey 将消息 ID 转换为字符串键
func msgIDToKey(msgID []byte) string {
	return fmt.Sprintf("%x", msgID)
}

// 确保 messagingif 包被使用
var _ = messagingif.StatusOK

