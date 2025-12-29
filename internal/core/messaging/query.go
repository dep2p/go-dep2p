// Package messaging 提供消息服务模块的实现
package messaging

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"sync/atomic"
	"time"

	"github.com/dep2p/go-dep2p/pkg/interfaces/endpoint"
	messagingif "github.com/dep2p/go-dep2p/pkg/interfaces/messaging"
	"github.com/dep2p/go-dep2p/pkg/protocolids"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              查询协议常量
// ============================================================================

// 引用 pkg/protocolids 唯一真源
var (
	// ProtocolQueryResponse 查询响应协议 (v1.1 scope: app)
	ProtocolQueryResponse = protocolids.AppMessagingQueryResponse
)

// ============================================================================
//                              查询模式 - 直接连接
// ============================================================================

// Query 发布查询并等待一个响应
func (s *MessagingService) Query(ctx context.Context, topic string, query []byte) ([]byte, error) {
	responses, err := s.QueryAll(ctx, topic, query, messagingif.QueryOptions{
		MaxResponses: 1,
		Timeout:      5 * time.Second,
		MinResponses: 1,
	})
	if err != nil {
		return nil, err
	}

	if len(responses) == 0 {
		return nil, ErrTimeout
	}

	return responses[0].Data, nil
}

// QueryAll 发布查询并等待多个响应
func (s *MessagingService) QueryAll(ctx context.Context, topic string, query []byte, opts messagingif.QueryOptions) ([]types.QueryResponse, error) {
	if atomic.LoadInt32(&s.closed) == 1 {
		return nil, ErrServiceClosed
	}

	if opts.MaxResponses == 0 {
		opts = messagingif.DefaultQueryOptions()
	}

	// 创建响应通道
	respCh := make(chan types.QueryResponse, opts.MaxResponses)
	queryID := atomic.AddUint64(&s.requestID, 1)

	// 设置超时
	deadline := time.Now().Add(opts.Timeout)
	ctx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()

	// 向所有连接的节点发送查询
	if s.endpoint != nil {
		for _, conn := range s.endpoint.Connections() {
			go s.queryToConn(ctx, conn, topic, queryID, query, respCh)
		}
	}

	// 收集响应
	responses := make([]types.QueryResponse, 0, opts.MaxResponses)
	for {
		select {
		case <-ctx.Done():
			if len(responses) >= opts.MinResponses {
				return responses, nil
			}
			return responses, ErrTimeout

		case resp := <-respCh:
			responses = append(responses, resp)
			if len(responses) >= opts.MaxResponses {
				return responses, nil
			}
		}
	}
}

// queryToConn 向连接发送查询
func (s *MessagingService) queryToConn(ctx context.Context, conn endpoint.Connection, topic string, queryID uint64, query []byte, respCh chan<- types.QueryResponse) {
	start := time.Now()

	stream, err := conn.OpenStream(ctx, ProtocolQuery)
	if err != nil {
		return
	}
	defer func() { _ = stream.Close() }()

	// 写入主题
	if err := writeString(stream, topic); err != nil {
		return
	}

	// 写入查询 ID
	if err := writeUint64(stream, queryID); err != nil {
		return
	}

	// 写入查询数据
	if err := writeBytes(stream, query); err != nil {
		return
	}

	// 读取响应
	data, err := readBytes(stream)
	if err != nil {
		return
	}

	// 发送响应
	select {
	case respCh <- types.QueryResponse{
		From:    conn.RemoteID(),
		Data:    data,
		Latency: time.Since(start),
	}:
	default:
	}
}

// SetQueryHandler 设置查询处理器
func (s *MessagingService) SetQueryHandler(topic string, handler messagingif.QueryHandler) {
	s.handlerMu.Lock()
	defer s.handlerMu.Unlock()
	s.queryHandlers[topic] = handler
}

// handleQueryStream 处理查询流
func (s *MessagingService) handleQueryStream(stream endpoint.Stream) {
	defer func() { _ = stream.Close() }()

	conn := stream.Connection()
	if conn == nil {
		return
	}

	// 读取主题
	topic, err := readString(stream)
	if err != nil {
		return
	}

	// 读取查询 ID
	_, err = readUint64(stream)
	if err != nil {
		return
	}

	// 读取查询数据
	query, err := readBytes(stream)
	if err != nil {
		return
	}

	// 查找处理器
	s.handlerMu.RLock()
	handler := s.queryHandlers[topic]
	s.handlerMu.RUnlock()

	if handler != nil {
		if data, shouldRespond := handler(query, conn.RemoteID()); shouldRespond {
			_ = writeBytes(stream, data) // 发送失败在流关闭时会处理
		}
	}
}

// ============================================================================
//                              查询模式 - Pub-Sub 广播
// ============================================================================

// PublishQuery 通过 Pub-Sub 广播查询
func (s *MessagingService) PublishQuery(ctx context.Context, topic string, query []byte, opts messagingif.QueryOptions) ([]types.QueryResponse, error) {
	if atomic.LoadInt32(&s.closed) == 1 {
		return nil, ErrServiceClosed
	}

	// 检查 endpoint 是否可用
	if s.endpoint == nil {
		return nil, ErrNoConnection
	}

	if opts.MaxResponses == 0 {
		opts = messagingif.DefaultQueryOptions()
	}

	// 生成查询 ID
	queryID, err := generateQueryID()
	if err != nil {
		return nil, err
	}

	// 创建响应收集器
	collector := newQueryResponseCollector(opts.MaxResponses)

	// 注册查询响应处理器
	s.registerQueryResponseHandler(queryID, collector)
	defer s.unregisterQueryResponseHandler(queryID)

	// 设置超时
	deadline := time.Now().Add(opts.Timeout)
	ctx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()

	// 生成消息 ID
	msgID, err := generateMessageID()
	if err != nil {
		return nil, err
	}

	// 构造查询消息
	msg := &types.Message{
		ID:        msgID,
		Topic:     topic,
		From:      s.endpoint.ID(),
		Data:      query,
		Timestamp: time.Now(),
		IsQuery:   true,
		QueryID:   queryID,
		ReplyTo:   s.endpoint.ID(),
	}

	// 编码查询消息（包含 Query 元数据）
	queryPayload := encodeQueryMessage(msg)

	// 通过 GossipSub 发布查询
	if s.gossipRouter != nil {
		if err := s.gossipRouter.Publish(ctx, topic, queryPayload); err != nil {
			log.Error("发布查询消息失败",
				"topic", topic,
				"err", err)
			return nil, err
		}
	} else {
		// 洪泛模式：向所有订阅者发送
		if err := s.floodPublish(ctx, topic, msg); err != nil {
			return nil, err
		}
	}

	log.Debug("查询已广播",
		"topic", topic,
		"queryID", queryID)

	// 收集响应
	return collector.collect(ctx, opts.MinResponses)
}

// queryResponseCollector 查询响应收集器
type queryResponseCollector struct {
	responses chan types.QueryResponse
	maxCount  int
}

// newQueryResponseCollector 创建响应收集器
func newQueryResponseCollector(maxCount int) *queryResponseCollector {
	return &queryResponseCollector{
		responses: make(chan types.QueryResponse, maxCount),
		maxCount:  maxCount,
	}
}

// addResponse 添加响应
func (c *queryResponseCollector) addResponse(resp types.QueryResponse) bool {
	select {
	case c.responses <- resp:
		return true
	default:
		return false // 已满
	}
}

// collect 收集响应
func (c *queryResponseCollector) collect(ctx context.Context, minCount int) ([]types.QueryResponse, error) {
	results := make([]types.QueryResponse, 0, c.maxCount)

	for {
		select {
		case <-ctx.Done():
			if len(results) >= minCount {
				return results, nil
			}
			return results, ErrTimeout

		case resp := <-c.responses:
			results = append(results, resp)
			if len(results) >= c.maxCount {
				return results, nil
			}
		}
	}
}

// registerQueryResponseHandler 注册查询响应处理器
func (s *MessagingService) registerQueryResponseHandler(queryID string, collector *queryResponseCollector) {
	s.queryResponseHandlersMu.Lock()
	s.queryResponseHandlers[queryID] = collector
	s.queryResponseHandlersMu.Unlock()
}

// unregisterQueryResponseHandler 取消注册查询响应处理器
func (s *MessagingService) unregisterQueryResponseHandler(queryID string) {
	s.queryResponseHandlersMu.Lock()
	delete(s.queryResponseHandlers, queryID)
	s.queryResponseHandlersMu.Unlock()
}

// handleQueryResponse 处理查询响应（实例方法）
func (s *MessagingService) handleQueryResponse(queryID string, from types.NodeID, data []byte, latency time.Duration) {
	s.queryResponseHandlersMu.RLock()
	collector := s.queryResponseHandlers[queryID]
	s.queryResponseHandlersMu.RUnlock()

	if collector != nil {
		collector.addResponse(types.QueryResponse{
			From:    from,
			Data:    data,
			Latency: latency,
		})
	}
}

// generateQueryID 生成查询 ID
func generateQueryID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// generateMessageID 生成消息 ID
func generateMessageID() ([]byte, error) {
	b := make([]byte, 20)
	if _, err := rand.Read(b); err != nil {
		return nil, err
	}
	return b, nil
}

// ============================================================================
//                              查询消息编解码
// ============================================================================

// queryMessageHeader 查询消息头（用于识别查询消息）
const queryMessageHeader byte = 0x51 // 'Q'

// encodeQueryMessage 编码查询消息
//
// 格式：[Header:1][QueryIDLen:2][QueryID][ReplyToLen:2][ReplyTo][Data]
func encodeQueryMessage(msg *types.Message) []byte {
	var buf bytes.Buffer

	// 写入头标识
	buf.WriteByte(queryMessageHeader)

	// 写入 QueryID
	queryIDBytes := []byte(msg.QueryID)
	_ = binary.Write(&buf, binary.BigEndian, uint16(len(queryIDBytes))) //nolint:gosec // G115: queryID 长度由协议限制
	buf.Write(queryIDBytes)

	// 写入 ReplyTo
	replyToBytes := msg.ReplyTo[:]
	_ = binary.Write(&buf, binary.BigEndian, uint16(len(replyToBytes))) //nolint:gosec // G115: nodeID 固定大小
	buf.Write(replyToBytes)

	// 写入原始数据
	buf.Write(msg.Data)

	return buf.Bytes()
}

// decodeQueryMessage 解码查询消息
//
// 返回：queryID, replyTo, data, isQuery
func decodeQueryMessage(payload []byte) (string, types.NodeID, []byte, bool) {
	if len(payload) < 1 || payload[0] != queryMessageHeader {
		return "", types.EmptyNodeID, payload, false
	}

	buf := bytes.NewReader(payload[1:])

	// 读取 QueryID
	var queryIDLen uint16
	if err := binary.Read(buf, binary.BigEndian, &queryIDLen); err != nil {
		return "", types.EmptyNodeID, payload, false
	}

	queryIDBytes := make([]byte, queryIDLen)
	if _, err := buf.Read(queryIDBytes); err != nil {
		return "", types.EmptyNodeID, payload, false
	}

	// 读取 ReplyTo
	var replyToLen uint16
	if err := binary.Read(buf, binary.BigEndian, &replyToLen); err != nil {
		return "", types.EmptyNodeID, payload, false
	}

	replyToBytes := make([]byte, replyToLen)
	if _, err := buf.Read(replyToBytes); err != nil {
		return "", types.EmptyNodeID, payload, false
	}

	var replyTo types.NodeID
	copy(replyTo[:], replyToBytes)

	// 读取数据
	data := make([]byte, buf.Len())
	_, _ = buf.Read(data) // bytes.Buffer.Read 不会返回错误

	return string(queryIDBytes), replyTo, data, true
}

// handleQueryResponseStream 处理查询响应流
func (s *MessagingService) handleQueryResponseStream(stream endpoint.Stream) {
	defer func() { _ = stream.Close() }()

	start := time.Now()

	conn := stream.Connection()
	if conn == nil {
		return
	}

	// 读取查询 ID
	queryID, err := readString(stream)
	if err != nil {
		return
	}

	// 读取响应数据
	data, err := readBytes(stream)
	if err != nil {
		return
	}

	// 处理响应（使用实例方法）
	s.handleQueryResponse(queryID, conn.RemoteID(), data, time.Since(start))
}

// floodPublish 洪泛发布完整消息
func (s *MessagingService) floodPublish(ctx context.Context, topic string, msg *types.Message) error {
	// 标记为已见
	s.markSeen(msg.ID)

	// 本地分发
	s.deliverLocal(msg)

	// 向所有连接的节点广播
	if s.endpoint != nil {
		for _, conn := range s.endpoint.Connections() {
			go s.publishToConn(ctx, conn, msg)
		}
	}

	log.Debug("消息已发布（洪泛模式）",
		"topic", topic,
		"size", len(msg.Data))

	return nil
}

