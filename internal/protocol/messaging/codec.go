// Package messaging 实现点对点消息传递协议
package messaging

import (
	"fmt"
	"io"
	"time"

	"github.com/dep2p/go-dep2p/pkg/interfaces"
	pb "github.com/dep2p/go-dep2p/pkg/lib/proto/messaging"
	"google.golang.org/protobuf/proto"
)

// Codec 消息编解码器
type Codec struct{}

// NewCodec 创建编解码器
func NewCodec() *Codec {
	return &Codec{}
}

// EncodeRequest 编码请求为字节流
func (c *Codec) EncodeRequest(req *interfaces.Request) ([]byte, error) {
	if req == nil {
		return nil, fmt.Errorf("%w: request is nil", ErrInvalidMessage)
	}

	// 转换为 protobuf Message
	msg := &pb.Message{
		Id:        []byte(req.ID),
		From:      []byte(req.From),
		Type:      pb.MessageType_DIRECT,
		Priority:  pb.Priority_NORMAL,
		Payload:   req.Data,
		Timestamp: uint64(req.Timestamp.Unix()),
		Metadata:  convertMetadataToProto(req.Metadata),
	}

	// 序列化
	data, err := proto.Marshal(msg)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	return data, nil
}

// DecodeRequest 解码字节流为请求
func (c *Codec) DecodeRequest(data []byte) (*interfaces.Request, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("%w: empty data", ErrInvalidMessage)
	}

	// 反序列化
	msg := &pb.Message{}
	if err := proto.Unmarshal(data, msg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal request: %w", err)
	}

	// 转换为 interfaces.Request
	req := &interfaces.Request{
		ID:        string(msg.Id),
		From:      string(msg.From),
		Data:      msg.Payload,
		Timestamp: time.Unix(int64(msg.Timestamp), 0),
		Metadata:  convertMetadataFromProto(msg.Metadata),
	}

	return req, nil
}

// EncodeResponse 编码响应为字节流
func (c *Codec) EncodeResponse(resp *interfaces.Response) ([]byte, error) {
	if resp == nil {
		return nil, fmt.Errorf("%w: response is nil", ErrInvalidMessage)
	}

	// 转换为 protobuf Message
	msg := &pb.Message{
		Id:        []byte(resp.ID),
		From:      []byte(resp.From),
		Type:      pb.MessageType_DIRECT,
		Priority:  pb.Priority_NORMAL,
		Payload:   resp.Data,
		Timestamp: uint64(resp.Timestamp.Unix()),
		Metadata:  convertMetadataToProto(resp.Metadata),
	}

	// 如果有错误,将错误信息放入 metadata
	if resp.Error != nil {
		if msg.Metadata == nil {
			msg.Metadata = make(map[string][]byte)
		}
		msg.Metadata["error"] = []byte(resp.Error.Error())
	}

	// 延迟信息也放入 metadata
	if resp.Latency > 0 {
		if msg.Metadata == nil {
			msg.Metadata = make(map[string][]byte)
		}
		msg.Metadata["latency"] = []byte(fmt.Sprintf("%d", resp.Latency.Nanoseconds()))
	}

	// 序列化
	data, err := proto.Marshal(msg)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal response: %w", err)
	}

	return data, nil
}

// DecodeResponse 解码字节流为响应
func (c *Codec) DecodeResponse(data []byte) (*interfaces.Response, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("%w: empty data", ErrInvalidMessage)
	}

	// 反序列化
	msg := &pb.Message{}
	if err := proto.Unmarshal(data, msg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	// 转换为 interfaces.Response
	resp := &interfaces.Response{
		ID:        string(msg.Id),
		From:      string(msg.From),
		Data:      msg.Payload,
		Timestamp: time.Unix(int64(msg.Timestamp), 0),
		Metadata:  convertMetadataFromProto(msg.Metadata),
	}

	// 提取错误信息
	if errMsg, exists := msg.Metadata["error"]; exists {
		resp.Error = fmt.Errorf("%s", string(errMsg))
	}

	// 提取延迟信息
	if latencyBytes, exists := msg.Metadata["latency"]; exists {
		var latencyNs int64
		if _, err := fmt.Sscanf(string(latencyBytes), "%d", &latencyNs); err == nil {
			resp.Latency = time.Duration(latencyNs)
		}
	}

	return resp, nil
}

// WriteRequest 将请求写入流
func (c *Codec) WriteRequest(w io.Writer, req *interfaces.Request) error {
	data, err := c.EncodeRequest(req)
	if err != nil {
		return err
	}

	// 写入长度前缀 (varint)
	if err := writeVarint(w, uint64(len(data))); err != nil {
		return fmt.Errorf("failed to write length: %w", err)
	}

	// 写入数据
	if _, err := w.Write(data); err != nil {
		return fmt.Errorf("failed to write data: %w", err)
	}

	return nil
}

// ReadRequest 从流中读取请求
func (c *Codec) ReadRequest(r io.Reader) (*interfaces.Request, error) {
	// 读取长度前缀
	length, err := readVarint(r)
	if err != nil {
		return nil, fmt.Errorf("failed to read length: %w", err)
	}

	// 读取数据
	data := make([]byte, length)
	if _, err := io.ReadFull(r, data); err != nil {
		return nil, fmt.Errorf("failed to read data: %w", err)
	}

	return c.DecodeRequest(data)
}

// WriteResponse 将响应写入流
func (c *Codec) WriteResponse(w io.Writer, resp *interfaces.Response) error {
	data, err := c.EncodeResponse(resp)
	if err != nil {
		return err
	}

	// 写入长度前缀
	if err := writeVarint(w, uint64(len(data))); err != nil {
		return fmt.Errorf("failed to write length: %w", err)
	}

	// 写入数据
	if _, err := w.Write(data); err != nil {
		return fmt.Errorf("failed to write data: %w", err)
	}

	return nil
}

// ReadResponse 从流中读取响应
func (c *Codec) ReadResponse(r io.Reader) (*interfaces.Response, error) {
	// 读取长度前缀
	length, err := readVarint(r)
	if err != nil {
		return nil, fmt.Errorf("failed to read length: %w", err)
	}

	// 读取数据
	data := make([]byte, length)
	if _, err := io.ReadFull(r, data); err != nil {
		return nil, fmt.Errorf("failed to read data: %w", err)
	}

	return c.DecodeResponse(data)
}

// convertMetadataToProto 转换 metadata 到 protobuf 格式
func convertMetadataToProto(metadata map[string]string) map[string][]byte {
	if metadata == nil {
		return nil
	}

	result := make(map[string][]byte, len(metadata))
	for k, v := range metadata {
		result[k] = []byte(v)
	}
	return result
}

// convertMetadataFromProto 从 protobuf 格式转换 metadata
func convertMetadataFromProto(metadata map[string][]byte) map[string]string {
	if metadata == nil {
		return nil
	}

	result := make(map[string]string, len(metadata))
	for k, v := range metadata {
		// 跳过内部使用的 key
		if k == "error" || k == "latency" {
			continue
		}
		result[k] = string(v)
	}
	return result
}

// writeVarint 写入可变长度整数
func writeVarint(w io.Writer, v uint64) error {
	buf := make([]byte, 10)
	n := putUvarint(buf, v)
	_, err := w.Write(buf[:n])
	return err
}

// readVarint 读取可变长度整数
func readVarint(r io.Reader) (uint64, error) {
	var x uint64
	var s uint
	for i := 0; i < 10; i++ {
		b := make([]byte, 1)
		if _, err := io.ReadFull(r, b); err != nil {
			return 0, err
		}
		if b[0] < 0x80 {
			if i == 9 && b[0] > 1 {
				return 0, fmt.Errorf("varint overflow")
			}
			return x | uint64(b[0])<<s, nil
		}
		x |= uint64(b[0]&0x7f) << s
		s += 7
	}
	return 0, fmt.Errorf("varint too long")
}

// putUvarint 编码可变长度整数
func putUvarint(buf []byte, x uint64) int {
	i := 0
	for x >= 0x80 {
		buf[i] = byte(x) | 0x80
		x >>= 7
		i++
	}
	buf[i] = byte(x)
	return i + 1
}
