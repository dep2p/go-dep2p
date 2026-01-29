package dep2p

import (
	"context"
	"io"
	"time"

	"github.com/dep2p/go-dep2p/pkg/interfaces"
)

// ════════════════════════════════════════════════════════════════════════════
//                              用户 API: Streams
// ════════════════════════════════════════════════════════════════════════════

// Streams 用户级流服务 API
//
// Streams 提供双向流通信能力，支持多协议。
//
// 使用示例：
//
//	streams := realm.Streams()
//	
//	// 注册多个流处理器
//	streams.RegisterHandler("file-transfer", handleFileTransfer)
//	streams.RegisterHandler("video", handleVideoStream)
//	
//	// 打开不同协议的流
//	fileStream, _ := streams.Open(ctx, peerID, "file-transfer")
//	fileStream.Write(fileData)
//	fileStream.Close()
type Streams struct {
	internal interfaces.Streams
}

// ════════════════════════════════════════════════════════════════════════════
//                              打开流
// ════════════════════════════════════════════════════════════════════════════

// Open 打开到指定节点的流
//
// 参数：
//   - ctx: 上下文（用于超时控制）
//   - peerID: 目标节点 ID
//   - protocol: 协议标识（如 "file-transfer", "video", "tunnel"）
//
// 返回：
//   - *BiStream: 双向流对象
//   - error: 错误信息
//
// 协议 ID 组装：
//   用户传: "file-transfer"
//   实际: /dep2p/app/<realmID>/streams/file-transfer/1.0.0
//
// 示例：
//
//	stream, err := streams.Open(ctx, peerID, "file-transfer")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer stream.Close()
//	
//	// 发送文件名
//	stream.Write([]byte("document.pdf"))
//	
//	// 发送文件内容
//	file, _ := os.Open("document.pdf")
//	io.Copy(stream, file)
func (s *Streams) Open(ctx context.Context, peerID string, protocol string) (*BiStream, error) {
	internal, err := s.internal.Open(ctx, peerID, protocol)
	if err != nil {
		return nil, err
	}
	return &BiStream{internal: internal}, nil
}

// ════════════════════════════════════════════════════════════════════════════
//                              注册处理器
// ════════════════════════════════════════════════════════════════════════════

// RegisterHandler 注册流处理器
//
// 参数：
//   - protocol: 协议标识（如 "file-transfer", "video"）
//   - handler: 流处理函数
//
// 一个 Realm 可以注册多个流协议处理器，互不干扰。
//
// 示例：
//
//	streams.RegisterHandler("file-transfer", func(stream *BiStream) {
//	    defer stream.Close()
//	    
//	    // 读取文件名
//	    nameBuf := make([]byte, 256)
//	    n, _ := stream.Read(nameBuf)
//	    filename := string(nameBuf[:n])
//	    
//	    // 接收文件
//	    file, _ := os.Create("received_" + filename)
//	    defer file.Close()
//	    
//	    io.Copy(file, stream)
//	})
func (s *Streams) RegisterHandler(protocol string, handler BiStreamHandler) error {
	// 包装处理函数
	wrappedHandler := func(internal interfaces.BiStream) {
		handler(&BiStream{internal: internal})
	}
	return s.internal.RegisterHandler(protocol, wrappedHandler)
}

// UnregisterHandler 注销流处理器
//
// 参数：
//   - protocol: 协议标识
//
// 示例：
//
//	streams.UnregisterHandler("file-transfer")
func (s *Streams) UnregisterHandler(protocol string) error {
	return s.internal.UnregisterHandler(protocol)
}

// ════════════════════════════════════════════════════════════════════════════
//                              生命周期
// ════════════════════════════════════════════════════════════════════════════

// Close 关闭服务
func (s *Streams) Close() error {
	return s.internal.Close()
}

// ════════════════════════════════════════════════════════════════════════════
//                              用户 API: BiStream
// ════════════════════════════════════════════════════════════════════════════

// BiStream 用户级双向流 API
//
// BiStream 实现 io.Reader, io.Writer, io.Closer 接口，
// 提供原始的字节流通信能力。
//
// 使用示例：
//
//	stream, _ := streams.Open(ctx, peerID, "file-transfer")
//	defer stream.Close()
//	
//	// 写入数据
//	stream.Write([]byte("hello"))
//	
//	// 读取数据
//	buf := make([]byte, 1024)
//	n, _ := stream.Read(buf)
//	
//	// 也可以使用标准库的 io 工具
//	io.Copy(stream, file)
type BiStream struct {
	internal interfaces.BiStream
}

// ════════════════════════════════════════════════════════════════════════════
//                              io.Reader
// ════════════════════════════════════════════════════════════════════════════

// Read 从流读取数据
//
// 实现 io.Reader 接口。
func (b *BiStream) Read(p []byte) (int, error) {
	return b.internal.Read(p)
}

// ════════════════════════════════════════════════════════════════════════════
//                              io.Writer
// ════════════════════════════════════════════════════════════════════════════

// Write 向流写入数据
//
// 实现 io.Writer 接口。
func (b *BiStream) Write(p []byte) (int, error) {
	return b.internal.Write(p)
}

// ════════════════════════════════════════════════════════════════════════════
//                              io.Closer
// ════════════════════════════════════════════════════════════════════════════

// Close 关闭流
//
// 实现 io.Closer 接口。
// 优雅关闭，双方都知道流已结束。
func (b *BiStream) Close() error {
	return b.internal.Close()
}

// ════════════════════════════════════════════════════════════════════════════
//                              流信息
// ════════════════════════════════════════════════════════════════════════════

// Protocol 返回流使用的协议
func (b *BiStream) Protocol() string {
	return b.internal.Protocol()
}

// RemotePeer 返回远端节点 ID
func (b *BiStream) RemotePeer() string {
	return b.internal.RemotePeer()
}

// ════════════════════════════════════════════════════════════════════════════
//                              流控制
// ════════════════════════════════════════════════════════════════════════════

// Reset 重置流（异常关闭）
//
// 强制关闭流，不等待双方确认。
// 通常用于错误处理。
func (b *BiStream) Reset() error {
	return b.internal.Reset()
}

// CloseRead 关闭读端
//
// 关闭后无法继续读取，但仍可写入。
func (b *BiStream) CloseRead() error {
	return b.internal.CloseRead()
}

// CloseWrite 关闭写端
//
// 关闭后无法继续写入，但仍可读取。
// 通常用于告知对方"我已发送完毕"。
func (b *BiStream) CloseWrite() error {
	return b.internal.CloseWrite()
}

// ════════════════════════════════════════════════════════════════════════════
//                              超时设置
// ════════════════════════════════════════════════════════════════════════════

// SetDeadline 设置读写超时
//
// 设置读和写操作的截止时间。
// 超时后，Read 和 Write 会返回错误。
// 传入零值 time.Time{} 表示不超时。
//
// 重要：对于长时间运行的流，建议设置超时以避免 goroutine 泄漏。
//
// 示例：
//
//	stream, _ := streams.Open(ctx, peerID, "file-transfer")
//	defer stream.Close()
//	
//	// 设置 30 秒超时
//	stream.SetDeadline(time.Now().Add(30 * time.Second))
//	
//	// 读写操作会在超时后返回错误
//	n, err := stream.Read(buf)
//	if err != nil {
//	    // 可能是超时错误
//	}
func (b *BiStream) SetDeadline(t time.Time) error {
	return b.internal.SetDeadline(t)
}

// SetReadDeadline 设置读超时
//
// 仅影响读操作的超时。
func (b *BiStream) SetReadDeadline(t time.Time) error {
	return b.internal.SetReadDeadline(t)
}

// SetWriteDeadline 设置写超时
//
// 仅影响写操作的超时。
func (b *BiStream) SetWriteDeadline(t time.Time) error {
	return b.internal.SetWriteDeadline(t)
}

// ════════════════════════════════════════════════════════════════════════════
//                              类型定义
// ════════════════════════════════════════════════════════════════════════════

// BiStreamHandler 双向流处理函数类型
type BiStreamHandler func(stream *BiStream)

// 确保 BiStream 实现标准接口
var _ io.Reader = (*BiStream)(nil)
var _ io.Writer = (*BiStream)(nil)
var _ io.Closer = (*BiStream)(nil)
