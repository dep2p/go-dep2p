// Package liveness 实现存活检测服务
package liveness

import (
	"encoding/json"
	"io"
	"time"

	"github.com/google/uuid"
)

// PingRequest Ping请求
type PingRequest struct {
	// ID 请求ID
	ID string `json:"id"`

	// Timestamp 时间戳
	Timestamp int64 `json:"timestamp"`
}

// PongResponse Pong响应
type PongResponse struct {
	// ID 请求ID
	ID string `json:"id"`

	// Timestamp 响应时间戳
	Timestamp int64 `json:"timestamp"`
}

// NewPingRequest 创建Ping请求
func NewPingRequest() *PingRequest {
	return &PingRequest{
		ID:        uuid.New().String(),
		Timestamp: time.Now().UnixNano(),
	}
}

// NewPongResponse 创建Pong响应
func NewPongResponse(pingID string) *PongResponse {
	return &PongResponse{
		ID:        pingID,
		Timestamp: time.Now().UnixNano(),
	}
}

// encodePing 编码Ping请求
func encodePing(ping *PingRequest) ([]byte, error) {
	return json.Marshal(ping)
}

// decodePing 解码Ping请求
func decodePing(data []byte) (*PingRequest, error) {
	var ping PingRequest
	if err := json.Unmarshal(data, &ping); err != nil {
		return nil, err
	}
	return &ping, nil
}

// encodePong 编码Pong响应
func encodePong(pong *PongResponse) ([]byte, error) {
	return json.Marshal(pong)
}

// decodePong 解码Pong响应
func decodePong(data []byte) (*PongResponse, error) {
	var pong PongResponse
	if err := json.Unmarshal(data, &pong); err != nil {
		return nil, err
	}
	return &pong, nil
}

// sendMessage 发送消息到流
func sendMessage(stream io.Writer, data []byte) error {
	// 先发送长度（4字节）
	length := uint32(len(data))
	lengthBytes := make([]byte, 4)
	lengthBytes[0] = byte(length >> 24)
	lengthBytes[1] = byte(length >> 16)
	lengthBytes[2] = byte(length >> 8)
	lengthBytes[3] = byte(length)

	if _, err := stream.Write(lengthBytes); err != nil {
		return err
	}

	// 发送数据
	if _, err := stream.Write(data); err != nil {
		return err
	}

	return nil
}

// receiveMessage 从流接收消息
func receiveMessage(stream io.Reader) ([]byte, error) {
	// 读取长度（4字节）
	lengthBytes := make([]byte, 4)
	if _, err := io.ReadFull(stream, lengthBytes); err != nil {
		return nil, err
	}

	length := uint32(lengthBytes[0])<<24 |
		uint32(lengthBytes[1])<<16 |
		uint32(lengthBytes[2])<<8 |
		uint32(lengthBytes[3])

	// 读取数据
	data := make([]byte, length)
	if _, err := io.ReadFull(stream, data); err != nil {
		return nil, err
	}

	return data, nil
}
