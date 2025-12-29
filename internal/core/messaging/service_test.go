// Package messaging 消息服务测试
package messaging

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	messagingif "github.com/dep2p/go-dep2p/pkg/interfaces/messaging"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              MessagingService 创建测试
// ============================================================================

func TestNewMessagingService(t *testing.T) {
	t.Run("默认配置创建", func(t *testing.T) {
		config := messagingif.DefaultConfig()

		svc := NewMessagingService(config, nil, nil)

		require.NotNil(t, svc)
		assert.Equal(t, config, svc.config)
		assert.NotNil(t, svc.pendingRequests)
		assert.NotNil(t, svc.requestHandlers)
		assert.NotNil(t, svc.queryHandlers)
		assert.NotNil(t, svc.subscriptions)
		assert.NotNil(t, svc.seenMessages)
	})

	t.Run("使用默认 Logger", func(t *testing.T) {
		config := messagingif.DefaultConfig()

		svc := NewMessagingService(config, nil, nil)

		require.NotNil(t, svc)
	})

	t.Run("无 Endpoint 创建", func(t *testing.T) {
		config := messagingif.DefaultConfig()

		svc := NewMessagingService(config, nil, nil)

		require.NotNil(t, svc)
		assert.Nil(t, svc.endpoint)
		assert.Nil(t, svc.gossipRouter) // 无 endpoint，不创建 gossip router
	})

	t.Run("洪泛模式不创建 GossipRouter", func(t *testing.T) {
		config := messagingif.DefaultConfig()
		config.FloodPublish = true

		svc := NewMessagingService(config, nil, nil)

		assert.Nil(t, svc.gossipRouter)
	})
}

func TestMessagingService_StartStop(t *testing.T) {
	t.Run("无 Endpoint 启动停止", func(t *testing.T) {
		config := messagingif.DefaultConfig()
		config.FloodPublish = true

		svc := NewMessagingService(config, nil, nil)

		// 启动
		err := svc.Start(context.Background())
		require.NoError(t, err)
		assert.Equal(t, int32(1), atomic.LoadInt32(&svc.running))

		// 停止
		err = svc.Stop()
		require.NoError(t, err)
		assert.Equal(t, int32(0), atomic.LoadInt32(&svc.running))
		assert.Equal(t, int32(1), atomic.LoadInt32(&svc.closed))
	})

	t.Run("重复启动", func(t *testing.T) {
		config := messagingif.DefaultConfig()
		config.FloodPublish = true

		svc := NewMessagingService(config, nil, nil)

		err1 := svc.Start(context.Background())
		err2 := svc.Start(context.Background())

		assert.NoError(t, err1)
		assert.NoError(t, err2)
		assert.Equal(t, int32(1), atomic.LoadInt32(&svc.running))

		svc.Stop()
	})

	t.Run("重复停止", func(t *testing.T) {
		config := messagingif.DefaultConfig()
		config.FloodPublish = true

		svc := NewMessagingService(config, nil, nil)
		svc.Start(context.Background())

		err1 := svc.Stop()
		err2 := svc.Stop()

		assert.NoError(t, err1)
		assert.NoError(t, err2)
	})

	t.Run("停止取消待处理请求", func(t *testing.T) {
		config := messagingif.DefaultConfig()
		svc := NewMessagingService(config, nil, nil)
		svc.Start(context.Background())

		// 添加待处理请求
		respCh := make(chan *types.Response, 1)
		svc.pendingMu.Lock()
		svc.pendingRequests[1] = &pendingRequest{
			id:       1,
			respCh:   respCh,
			deadline: time.Now().Add(time.Hour),
		}
		svc.pendingMu.Unlock()

		// 停止应该关闭通道
		svc.Stop()

		// 验证通道已关闭
		select {
		case _, ok := <-respCh:
			assert.False(t, ok, "通道应该被关闭")
		default:
			t.Fatal("通道应该可读（因为已关闭）")
		}
	})
}

// ============================================================================
//                              处理器注册测试
// ============================================================================

func TestMessagingService_Handlers(t *testing.T) {
	t.Run("设置请求处理器", func(t *testing.T) {
		config := messagingif.DefaultConfig()
		svc := NewMessagingService(config, nil, nil)

		protocol := types.ProtocolID("/test/request/1.0.0")
		called := false
		handler := func(req *types.Request) *types.Response {
			called = true
			return &types.Response{Data: []byte("response")}
		}

		svc.SetRequestHandler(protocol, handler)

		svc.handlerMu.RLock()
		h := svc.requestHandlers[protocol]
		svc.handlerMu.RUnlock()

		require.NotNil(t, h)

		// 调用处理器验证
		resp := h(&types.Request{Data: []byte("test")})
		assert.True(t, called)
		assert.Equal(t, []byte("response"), resp.Data)
	})

	t.Run("覆盖请求处理器", func(t *testing.T) {
		config := messagingif.DefaultConfig()
		svc := NewMessagingService(config, nil, nil)

		protocol := types.ProtocolID("/test/request/1.0.0")

		// 第一个处理器
		svc.SetRequestHandler(protocol, func(req *types.Request) *types.Response {
			return &types.Response{Data: []byte("first")}
		})

		// 覆盖
		svc.SetRequestHandler(protocol, func(req *types.Request) *types.Response {
			return &types.Response{Data: []byte("second")}
		})

		svc.handlerMu.RLock()
		h := svc.requestHandlers[protocol]
		svc.handlerMu.RUnlock()

		resp := h(&types.Request{})
		assert.Equal(t, []byte("second"), resp.Data, "应该使用新处理器")
	})

	t.Run("设置通知处理器", func(t *testing.T) {
		config := messagingif.DefaultConfig()
		svc := NewMessagingService(config, nil, nil)

		protocol := types.ProtocolID("/test/notify/1.0.0")
		receivedData := []byte{}
		receivedFrom := types.NodeID{}

		handler := func(data []byte, from types.NodeID) {
			receivedData = data
			receivedFrom = from
		}

		svc.SetNotifyHandler(protocol, handler)

		svc.handlerMu.RLock()
		h := svc.notifyHandlers[protocol]
		svc.handlerMu.RUnlock()

		require.NotNil(t, h)

		// 调用处理器验证
		testData := []byte("notification")
		testFrom := types.NodeID{9, 8, 7}
		h(testData, testFrom)

		assert.Equal(t, testData, receivedData)
		assert.Equal(t, testFrom, receivedFrom)
	})

	t.Run("设置查询处理器", func(t *testing.T) {
		config := messagingif.DefaultConfig()
		svc := NewMessagingService(config, nil, nil)

		topic := "test-query-topic"
		handler := func(query []byte, from types.NodeID) ([]byte, bool) {
			return []byte("answer"), true
		}

		svc.SetQueryHandler(topic, handler)

		svc.handlerMu.RLock()
		h := svc.queryHandlers[topic]
		svc.handlerMu.RUnlock()

		require.NotNil(t, h)

		// 调用处理器验证
		resp, shouldRespond := h([]byte("question"), types.NodeID{1})
		assert.True(t, shouldRespond)
		assert.Equal(t, []byte("answer"), resp)
	})

	t.Run("查询处理器返回不响应", func(t *testing.T) {
		config := messagingif.DefaultConfig()
		svc := NewMessagingService(config, nil, nil)

		topic := "conditional-query"
		handler := func(query []byte, from types.NodeID) ([]byte, bool) {
			if string(query) == "valid" {
				return []byte("ok"), true
			}
			return nil, false
		}

		svc.SetQueryHandler(topic, handler)

		svc.handlerMu.RLock()
		h := svc.queryHandlers[topic]
		svc.handlerMu.RUnlock()

		// 有效查询
		resp1, ok1 := h([]byte("valid"), types.NodeID{1})
		assert.True(t, ok1)
		assert.Equal(t, []byte("ok"), resp1)

		// 无效查询
		resp2, ok2 := h([]byte("invalid"), types.NodeID{1})
		assert.False(t, ok2)
		assert.Nil(t, resp2)
	})
}

// ============================================================================
//                              去重逻辑测试
// ============================================================================

func TestMessagingService_Deduplication(t *testing.T) {
	t.Run("消息去重正确", func(t *testing.T) {
		config := messagingif.DefaultConfig()
		svc := NewMessagingService(config, nil, nil)

		msgID := []byte{1, 2, 3, 4, 5, 6, 7, 8}

		// 第一次检查应该返回 false
		assert.False(t, svc.hasSeen(msgID), "新消息不应该被识别为已见")

		// 标记为已见
		svc.markSeen(msgID)

		// 第二次检查应该返回 true
		assert.True(t, svc.hasSeen(msgID), "标记后的消息应该被识别为已见")
	})

	t.Run("不同消息 ID 独立", func(t *testing.T) {
		config := messagingif.DefaultConfig()
		svc := NewMessagingService(config, nil, nil)

		msgID1 := []byte{1, 2, 3}
		msgID2 := []byte{4, 5, 6}

		svc.markSeen(msgID1)

		assert.True(t, svc.hasSeen(msgID1))
		assert.False(t, svc.hasSeen(msgID2), "不同 ID 应该独立判断")
	})

	t.Run("过期消息被清理", func(t *testing.T) {
		config := messagingif.DefaultConfig()
		config.DeDuplicateTTL = 50 * time.Millisecond
		svc := NewMessagingService(config, nil, nil)

		msgID := []byte{1, 2, 3}
		svc.markSeen(msgID)

		// 立即检查
		assert.True(t, svc.hasSeen(msgID))

		// 等待过期
		time.Sleep(60 * time.Millisecond)

		// 手动触发清理
		svc.cleanupSeenMessages()

		// 应该已被清理
		assert.False(t, svc.hasSeen(msgID), "过期消息应该被清理")
	})

	t.Run("未过期消息不清理", func(t *testing.T) {
		config := messagingif.DefaultConfig()
		config.DeDuplicateTTL = time.Hour
		svc := NewMessagingService(config, nil, nil)

		msgID := []byte{1, 2, 3}
		svc.markSeen(msgID)

		// 清理
		svc.cleanupSeenMessages()

		// 应该仍存在
		assert.True(t, svc.hasSeen(msgID), "未过期消息不应该被清理")
	})
}

// ============================================================================
//                              订阅测试
// ============================================================================

func TestMessagingService_SubscribeSimple(t *testing.T) {
	t.Run("简单订阅正常工作", func(t *testing.T) {
		config := messagingif.DefaultConfig()
		config.FloodPublish = true
		svc := NewMessagingService(config, nil, nil)
		svc.Start(context.Background())
		defer svc.Stop()

		sub, err := svc.subscribeSimple("test-topic")
		require.NoError(t, err)
		require.NotNil(t, sub)

		assert.Equal(t, "test-topic", sub.Topic())
		assert.True(t, sub.IsActive())

		// 验证订阅已添加
		svc.subMu.RLock()
		subs := svc.subscriptions["test-topic"]
		svc.subMu.RUnlock()
		assert.Len(t, subs, 1)

		// 取消订阅
		sub.Cancel()
		assert.False(t, sub.IsActive())
	})

	t.Run("多个订阅同一主题", func(t *testing.T) {
		config := messagingif.DefaultConfig()
		config.FloodPublish = true
		svc := NewMessagingService(config, nil, nil)
		svc.Start(context.Background())
		defer svc.Stop()

		sub1, _ := svc.subscribeSimple("shared-topic")
		sub2, _ := svc.subscribeSimple("shared-topic")

		assert.True(t, sub1.IsActive())
		assert.True(t, sub2.IsActive())

		// 验证两个订阅都存在
		svc.subMu.RLock()
		subs := svc.subscriptions["shared-topic"]
		svc.subMu.RUnlock()
		assert.Len(t, subs, 2)

		// 取消一个不影响另一个
		sub1.Cancel()
		assert.False(t, sub1.IsActive())
		assert.True(t, sub2.IsActive())

		// 验证只移除了一个
		svc.subMu.RLock()
		subs = svc.subscriptions["shared-topic"]
		svc.subMu.RUnlock()
		assert.Len(t, subs, 1)

		sub2.Cancel()
	})

	t.Run("服务关闭后订阅失败", func(t *testing.T) {
		config := messagingif.DefaultConfig()
		config.FloodPublish = true
		svc := NewMessagingService(config, nil, nil)
		svc.Start(context.Background())
		svc.Stop()

		_, err := svc.Subscribe(context.Background(), "test-topic")
		assert.Error(t, err)
		assert.Equal(t, ErrServiceClosed, err)
	})
}

// ============================================================================
//                              本地分发测试
// ============================================================================

func TestMessagingService_DeliverLocal(t *testing.T) {
	t.Run("消息分发到订阅者", func(t *testing.T) {
		config := messagingif.DefaultConfig()
		config.FloodPublish = true
		svc := NewMessagingService(config, nil, nil)
		svc.Start(context.Background())
		defer svc.Stop()

		// 订阅
		sub, err := svc.subscribeSimple("test-topic")
		require.NoError(t, err)
		defer sub.Cancel()

		// 本地分发
		msg := &types.Message{
			Topic: "test-topic",
			Data:  []byte("test data"),
			From:  types.NodeID{9, 8, 7},
		}
		svc.deliverLocal(msg)

		// 验证收到消息
		select {
		case received := <-sub.Messages():
			assert.Equal(t, "test-topic", received.Topic)
			assert.Equal(t, []byte("test data"), received.Data)
		case <-time.After(100 * time.Millisecond):
			t.Fatal("未收到消息")
		}
	})

	t.Run("不同主题消息不互相干扰", func(t *testing.T) {
		config := messagingif.DefaultConfig()
		config.FloodPublish = true
		svc := NewMessagingService(config, nil, nil)
		svc.Start(context.Background())
		defer svc.Stop()

		// 订阅 topic1
		sub1, _ := svc.subscribeSimple("topic1")
		defer sub1.Cancel()

		// 订阅 topic2
		sub2, _ := svc.subscribeSimple("topic2")
		defer sub2.Cancel()

		// 发送到 topic1
		msg := &types.Message{
			Topic: "topic1",
			Data:  []byte("for topic1"),
		}
		svc.deliverLocal(msg)

		// topic1 应该收到
		select {
		case <-sub1.Messages():
			// OK
		case <-time.After(100 * time.Millisecond):
			t.Fatal("topic1 未收到消息")
		}

		// topic2 不应该收到
		select {
		case <-sub2.Messages():
			t.Fatal("topic2 不应该收到消息")
		case <-time.After(50 * time.Millisecond):
			// OK
		}
	})

	t.Run("多个订阅者都收到消息", func(t *testing.T) {
		config := messagingif.DefaultConfig()
		config.FloodPublish = true
		svc := NewMessagingService(config, nil, nil)
		svc.Start(context.Background())
		defer svc.Stop()

		sub1, _ := svc.subscribeSimple("broadcast-topic")
		defer sub1.Cancel()
		sub2, _ := svc.subscribeSimple("broadcast-topic")
		defer sub2.Cancel()

		msg := &types.Message{
			Topic: "broadcast-topic",
			Data:  []byte("broadcast data"),
		}
		svc.deliverLocal(msg)

		// 两个订阅者都应该收到
		var wg sync.WaitGroup
		wg.Add(2)

		go func() {
			defer wg.Done()
			select {
			case <-sub1.Messages():
			case <-time.After(100 * time.Millisecond):
				t.Error("sub1 未收到消息")
			}
		}()

		go func() {
			defer wg.Done()
			select {
			case <-sub2.Messages():
			case <-time.After(100 * time.Millisecond):
				t.Error("sub2 未收到消息")
			}
		}()

		wg.Wait()
	})

	t.Run("已取消订阅不收消息", func(t *testing.T) {
		config := messagingif.DefaultConfig()
		config.FloodPublish = true
		svc := NewMessagingService(config, nil, nil)
		svc.Start(context.Background())
		defer svc.Stop()

		sub, _ := svc.subscribeSimple("cancel-topic")
		sub.Cancel()

		msg := &types.Message{
			Topic: "cancel-topic",
			Data:  []byte("should not receive"),
		}
		svc.deliverLocal(msg)

		// 不应该 panic
	})
}

// ============================================================================
//                              并发测试
// ============================================================================

func TestMessagingService_Concurrency(t *testing.T) {
	t.Run("并发订阅取消", func(t *testing.T) {
		config := messagingif.DefaultConfig()
		config.FloodPublish = true
		svc := NewMessagingService(config, nil, nil)
		svc.Start(context.Background())
		defer svc.Stop()

		var wg sync.WaitGroup
		for i := 0; i < 50; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				sub, err := svc.subscribeSimple("concurrent-topic")
				if err != nil {
					return
				}
				time.Sleep(time.Duration(idx%10) * time.Millisecond)
				sub.Cancel()
			}(i)
		}
		wg.Wait()

		// 验证无 panic
	})

	t.Run("并发处理器注册", func(t *testing.T) {
		config := messagingif.DefaultConfig()
		svc := NewMessagingService(config, nil, nil)

		var wg sync.WaitGroup
		for i := 0; i < 50; i++ {
			wg.Add(2)
			go func(idx int) {
				defer wg.Done()
				protocol := types.ProtocolID("/test/" + string(rune('a'+idx)))
				svc.SetRequestHandler(protocol, func(req *types.Request) *types.Response {
					return &types.Response{}
				})
			}(i)
			go func(idx int) {
				defer wg.Done()
				topic := "topic-" + string(rune('A'+idx))
				svc.SetQueryHandler(topic, func(q []byte, from types.NodeID) ([]byte, bool) {
					return nil, false
				})
			}(i)
		}
		wg.Wait()

		// 验证注册成功
		svc.handlerMu.RLock()
		reqCount := len(svc.requestHandlers)
		queryCount := len(svc.queryHandlers)
		svc.handlerMu.RUnlock()

		assert.Equal(t, 50, reqCount)
		assert.Equal(t, 50, queryCount)
	})

	t.Run("并发去重检查", func(t *testing.T) {
		config := messagingif.DefaultConfig()
		svc := NewMessagingService(config, nil, nil)

		msgID := []byte{1, 2, 3, 4, 5}
		svc.markSeen(msgID)

		var wg sync.WaitGroup
		results := make([]bool, 100)

		for i := 0; i < 100; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				results[idx] = svc.hasSeen(msgID)
			}(i)
		}
		wg.Wait()

		// 所有结果应该为 true
		for i, result := range results {
			assert.True(t, result, "检查 %d 应该返回 true", i)
		}
	})

	t.Run("并发分发和订阅", func(t *testing.T) {
		config := messagingif.DefaultConfig()
		config.FloodPublish = true
		svc := NewMessagingService(config, nil, nil)
		svc.Start(context.Background())
		defer svc.Stop()

		var wg sync.WaitGroup

		// 并发订阅
		for i := 0; i < 20; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				sub, _ := svc.subscribeSimple("concurrent-deliver")
				defer sub.Cancel()
				time.Sleep(50 * time.Millisecond)
			}()
		}

		// 并发分发
		for i := 0; i < 30; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				msg := &types.Message{
					Topic: "concurrent-deliver",
					Data:  []byte{byte(idx)},
				}
				svc.deliverLocal(msg)
			}(i)
		}

		wg.Wait()
		// 验证无 panic
	})
}

// ============================================================================
//                              错误处理测试
// ============================================================================

func TestMessagingService_Errors(t *testing.T) {
	t.Run("无 Endpoint 时 Request 失败", func(t *testing.T) {
		config := messagingif.DefaultConfig()
		svc := NewMessagingService(config, nil, nil)
		svc.Start(context.Background())
		defer svc.Stop()

		_, err := svc.Request(context.Background(), types.NodeID{1}, "/test", []byte("data"))
		assert.Error(t, err)
		assert.ErrorIs(t, err, ErrNoConnection)
	})

	t.Run("无 Endpoint 时 Send 失败", func(t *testing.T) {
		config := messagingif.DefaultConfig()
		svc := NewMessagingService(config, nil, nil)
		svc.Start(context.Background())
		defer svc.Stop()

		err := svc.Send(context.Background(), types.NodeID{1}, "/test", []byte("data"))
		assert.Error(t, err)
		assert.ErrorIs(t, err, ErrNoConnection)
	})

	t.Run("服务关闭后 Request 失败", func(t *testing.T) {
		config := messagingif.DefaultConfig()
		svc := NewMessagingService(config, nil, nil)
		svc.Start(context.Background())
		svc.Stop()

		_, err := svc.Request(context.Background(), types.NodeID{1}, "/test", []byte("data"))
		assert.Error(t, err)
		assert.Equal(t, ErrServiceClosed, err)
	})

	t.Run("服务关闭后 Send 失败", func(t *testing.T) {
		config := messagingif.DefaultConfig()
		svc := NewMessagingService(config, nil, nil)
		svc.Start(context.Background())
		svc.Stop()

		err := svc.Send(context.Background(), types.NodeID{1}, "/test", []byte("data"))
		assert.Error(t, err)
		assert.Equal(t, ErrServiceClosed, err)
	})

	t.Run("服务关闭后 Publish 失败", func(t *testing.T) {
		config := messagingif.DefaultConfig()
		config.FloodPublish = true
		svc := NewMessagingService(config, nil, nil)
		svc.Start(context.Background())
		svc.Stop()

		err := svc.Publish(context.Background(), "topic", []byte("data"))
		assert.Error(t, err)
		assert.Equal(t, ErrServiceClosed, err)
	})
}

// ============================================================================
//                              QueryResponseCollector 详细测试
// ============================================================================

func TestQueryResponseCollector_AddResponse(t *testing.T) {
	t.Run("正常添加响应", func(t *testing.T) {
		collector := newQueryResponseCollector(5)

		resp := types.QueryResponse{
			From: types.NodeID{1},
			Data: []byte("data"),
		}

		ok := collector.addResponse(resp)
		assert.True(t, ok)
	})

	t.Run("达到容量后拒绝", func(t *testing.T) {
		collector := newQueryResponseCollector(2)

		// 添加两个
		collector.addResponse(types.QueryResponse{From: types.NodeID{1}})
		collector.addResponse(types.QueryResponse{From: types.NodeID{2}})

		// 第三个应该失败
		ok := collector.addResponse(types.QueryResponse{From: types.NodeID{3}})
		assert.False(t, ok)
	})
}

func TestQueryResponseCollector_Collect(t *testing.T) {
	t.Run("收集足够响应", func(t *testing.T) {
		collector := newQueryResponseCollector(5)

		// 后台添加响应
		go func() {
			for i := 0; i < 3; i++ {
				time.Sleep(10 * time.Millisecond)
				collector.addResponse(types.QueryResponse{
					From: types.NodeID{byte(i)},
					Data: []byte{byte(i)},
				})
			}
		}()

		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer cancel()

		results, err := collector.collect(ctx, 2)
		assert.NoError(t, err)
		assert.GreaterOrEqual(t, len(results), 2)
	})

	t.Run("超时未收集足够响应", func(t *testing.T) {
		collector := newQueryResponseCollector(5)

		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()

		_, err := collector.collect(ctx, 3)
		assert.Error(t, err)
		assert.Equal(t, ErrTimeout, err)
	})

	t.Run("达到最大响应数停止", func(t *testing.T) {
		collector := newQueryResponseCollector(2)

		// 快速添加响应
		for i := 0; i < 5; i++ {
			collector.addResponse(types.QueryResponse{
				From: types.NodeID{byte(i)},
				Data: []byte{byte(i)},
			})
		}

		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		results, err := collector.collect(ctx, 1)
		assert.NoError(t, err)
		assert.Len(t, results, 2, "应该在达到最大响应数时停止")
	})

	t.Run("部分响应满足最小要求", func(t *testing.T) {
		collector := newQueryResponseCollector(10)

		// 添加 3 个响应
		collector.addResponse(types.QueryResponse{From: types.NodeID{1}})
		collector.addResponse(types.QueryResponse{From: types.NodeID{2}})
		collector.addResponse(types.QueryResponse{From: types.NodeID{3}})

		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()

		// 最小 2 个，实际 3 个，超时后返回
		results, err := collector.collect(ctx, 2)
		assert.NoError(t, err)
		assert.Len(t, results, 3)
	})
}

// ============================================================================
//                              SubscriptionHandle 测试
// ============================================================================

func TestSubscriptionHandle(t *testing.T) {
	t.Run("Topic 返回正确主题", func(t *testing.T) {
		config := messagingif.DefaultConfig()
		svc := NewMessagingService(config, nil, nil)

		sub, _ := svc.subscribeSimple("my-topic")

		assert.Equal(t, "my-topic", sub.Topic())
		sub.Cancel()
	})

	t.Run("Messages 返回通道", func(t *testing.T) {
		config := messagingif.DefaultConfig()
		svc := NewMessagingService(config, nil, nil)

		sub, _ := svc.subscribeSimple("channel-topic")
		ch := sub.Messages()

		assert.NotNil(t, ch)
		sub.Cancel()
	})

	t.Run("Cancel 关闭通道", func(t *testing.T) {
		config := messagingif.DefaultConfig()
		svc := NewMessagingService(config, nil, nil)

		sub, _ := svc.subscribeSimple("cancel-test")
		ch := sub.Messages()

		sub.Cancel()

		// 通道应该关闭
		select {
		case _, ok := <-ch:
			assert.False(t, ok, "通道应该关闭")
		default:
			// 可能还没关闭，等一下
			time.Sleep(10 * time.Millisecond)
			select {
			case _, ok := <-ch:
				assert.False(t, ok, "通道应该关闭")
			default:
				t.Fatal("通道应该可读（因为已关闭）")
			}
		}
	})

	t.Run("重复 Cancel 安全", func(t *testing.T) {
		config := messagingif.DefaultConfig()
		svc := NewMessagingService(config, nil, nil)

		sub, _ := svc.subscribeSimple("double-cancel")

		sub.Cancel()
		sub.Cancel() // 不应该 panic

		assert.False(t, sub.IsActive())
	})
}

// ============================================================================
//                              编解码工具函数测试
// ============================================================================

func TestMsgIDToKey(t *testing.T) {
	t.Run("相同 ID 相同 key", func(t *testing.T) {
		id := []byte{1, 2, 3, 4, 5}
		key1 := msgIDToKey(id)
		key2 := msgIDToKey(id)
		assert.Equal(t, key1, key2)
	})

	t.Run("不同 ID 不同 key", func(t *testing.T) {
		id1 := []byte{1, 2, 3}
		id2 := []byte{4, 5, 6}
		key1 := msgIDToKey(id1)
		key2 := msgIDToKey(id2)
		assert.NotEqual(t, key1, key2)
	})
}

// ============================================================================
//                              handleQueryResponse 实例方法测试
// ============================================================================

func TestHandleQueryResponse(t *testing.T) {
	t.Run("有收集器时响应被添加", func(t *testing.T) {
		config := messagingif.DefaultConfig()
		svc := NewMessagingService(config, nil, nil)

		collector := newQueryResponseCollector(5)
		queryID := "test-query-id"

		// 注册收集器
		svc.registerQueryResponseHandler(queryID, collector)
		defer svc.unregisterQueryResponseHandler(queryID)

		// 调用处理
		svc.handleQueryResponse(queryID, types.NodeID{1}, []byte("data"), 100*time.Millisecond)

		// 验证响应被添加
		select {
		case resp := <-collector.responses:
			assert.Equal(t, types.NodeID{1}, resp.From)
			assert.Equal(t, []byte("data"), resp.Data)
			assert.Equal(t, 100*time.Millisecond, resp.Latency)
		default:
			t.Fatal("响应未被添加")
		}
	})

	t.Run("无收集器时静默忽略", func(t *testing.T) {
		config := messagingif.DefaultConfig()
		svc := NewMessagingService(config, nil, nil)

		// 调用不存在的 queryID
		svc.handleQueryResponse("non-existent", types.NodeID{1}, []byte("data"), 0)
		// 不应该 panic
	})
}

// ============================================================================
//                              新增测试 - 修复验证
// ============================================================================

func TestMessagingService_PublishFlood_NilEndpoint(t *testing.T) {
	config := messagingif.DefaultConfig()
	config.FloodPublish = true
	svc := NewMessagingService(config, nil, nil)
	svc.Start(context.Background())
	defer svc.Stop()

	// publishFlood 在 endpoint 为 nil 时应返回错误而不是 panic
	err := svc.publishFlood(context.Background(), "test-topic", []byte("data"))
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrNoConnection)
}

func TestMessagingService_CleanupLoop_ZeroTTL(t *testing.T) {
	config := messagingif.DefaultConfig()
	config.DeDuplicateTTL = 0 // 零 TTL
	config.FloodPublish = true

	svc := NewMessagingService(config, nil, nil)

	// 启动不应该 panic（应该使用最小间隔）
	err := svc.Start(context.Background())
	assert.NoError(t, err)

	// 等待一会确保 cleanupLoop 运行
	time.Sleep(100 * time.Millisecond)

	err = svc.Stop()
	assert.NoError(t, err)
}

func TestMessagingService_GossipSubscriptionHandle_MessagesOnce(t *testing.T) {
	// 测试 gossipSubscriptionHandle.Messages() 只初始化一次 goroutine
	handle := &gossipSubscriptionHandle{
		topic:  "test",
		active: 1,
		// 这里不设置 messages，因为我们只是测试 initOnce 行为
	}

	// 多次调用 Messages() 应该返回同一个通道
	ch1 := handle.Messages()
	ch2 := handle.Messages()
	ch3 := handle.Messages()

	assert.Equal(t, ch1, ch2, "多次调用应返回同一个通道")
	assert.Equal(t, ch2, ch3, "多次调用应返回同一个通道")
}

func TestMessagingService_PublishQuery_NilEndpoint(t *testing.T) {
	config := messagingif.DefaultConfig()
	config.FloodPublish = true
	svc := NewMessagingService(config, nil, nil)
	svc.Start(context.Background())
	defer svc.Stop()

	// PublishQuery 在 endpoint 为 nil 时应返回错误
	_, err := svc.PublishQuery(context.Background(), "test-topic", []byte("query"), messagingif.DefaultQueryOptions())
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrNoConnection)
}

// ============================================================================
//                              TopicPeers/MeshPeers 测试
// ============================================================================

func TestMessagingService_TopicPeers(t *testing.T) {
	t.Run("无 GossipRouter 时返回 nil", func(t *testing.T) {
		config := messagingif.DefaultConfig()
		config.FloodPublish = true // 洪泛模式不创建 GossipRouter
		svc := NewMessagingService(config, nil, nil)

		peers := svc.TopicPeers("test-topic")
		assert.Nil(t, peers, "无 GossipRouter 时应返回 nil")
	})

	t.Run("无 Endpoint 时返回 nil", func(t *testing.T) {
		config := messagingif.DefaultConfig()
		// 无 endpoint 也不会创建 GossipRouter
		svc := NewMessagingService(config, nil, nil)

		peers := svc.TopicPeers("test-topic")
		assert.Nil(t, peers)
	})
}

func TestMessagingService_MeshPeers(t *testing.T) {
	t.Run("无 GossipRouter 时返回 nil", func(t *testing.T) {
		config := messagingif.DefaultConfig()
		config.FloodPublish = true // 洪泛模式不创建 GossipRouter
		svc := NewMessagingService(config, nil, nil)

		peers := svc.MeshPeers("test-topic")
		assert.Nil(t, peers, "无 GossipRouter 时应返回 nil")
	})

	t.Run("无 Endpoint 时返回 nil", func(t *testing.T) {
		config := messagingif.DefaultConfig()
		// 无 endpoint 也不会创建 GossipRouter
		svc := NewMessagingService(config, nil, nil)

		peers := svc.MeshPeers("test-topic")
		assert.Nil(t, peers)
	})
}
