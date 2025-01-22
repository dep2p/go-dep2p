package rcmgr

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/dep2p/go-dep2p/core/network"
)

// trace 用于跟踪资源管理器的事件
type trace struct {
	path string // 跟踪文件路径

	ctx    context.Context // 上下文
	cancel func()          // 取消函数
	wg     sync.WaitGroup  // 等待组,用于等待后台写入完成

	mx            sync.Mutex      // 互斥锁,保护以下字段
	done          bool            // 是否已完成
	pendingWrites []interface{}   // 待写入的事件列表
	reporters     []TraceReporter // 事件报告器列表
}

// TraceReporter 定义事件报告器接口
type TraceReporter interface {
	// ConsumeEvent 消费一个跟踪事件
	// 参数:
	//   - evt TraceEvt 要消费的事件
	// 注意:此方法会同步调用,实现时应快速处理事件
	ConsumeEvent(evt TraceEvt)
}

// WithTrace 创建一个带跟踪功能的选项
// 参数:
//   - path string 跟踪文件路径
//
// 返回值:
//   - Option 资源管理器选项
func WithTrace(path string) Option {
	return func(r *resourceManager) error {
		if r.trace == nil {
			r.trace = &trace{path: path}
		} else {
			r.trace.path = path
		}
		return nil
	}
}

// WithTraceReporter 创建一个带事件报告器的选项
// 参数:
//   - reporter TraceReporter 事件报告器
//
// 返回值:
//   - Option 资源管理器选项
func WithTraceReporter(reporter TraceReporter) Option {
	return func(r *resourceManager) error {
		if r.trace == nil {
			r.trace = &trace{}
		}
		r.trace.reporters = append(r.trace.reporters, reporter)
		return nil
	}
}

// TraceEvtTyp 定义跟踪事件类型
type TraceEvtTyp string

// 定义所有跟踪事件类型常量
const (
	TraceStartEvt              TraceEvtTyp = "start"                // 开始事件
	TraceCreateScopeEvt        TraceEvtTyp = "create_scope"         // 创建作用域事件
	TraceDestroyScopeEvt       TraceEvtTyp = "destroy_scope"        // 销毁作用域事件
	TraceReserveMemoryEvt      TraceEvtTyp = "reserve_memory"       // 预留内存事件
	TraceBlockReserveMemoryEvt TraceEvtTyp = "block_reserve_memory" // 阻塞预留内存事件
	TraceReleaseMemoryEvt      TraceEvtTyp = "release_memory"       // 释放内存事件
	TraceAddStreamEvt          TraceEvtTyp = "add_stream"           // 添加流事件
	TraceBlockAddStreamEvt     TraceEvtTyp = "block_add_stream"     // 阻塞添加流事件
	TraceRemoveStreamEvt       TraceEvtTyp = "remove_stream"        // 移除流事件
	TraceAddConnEvt            TraceEvtTyp = "add_conn"             // 添加连接事件
	TraceBlockAddConnEvt       TraceEvtTyp = "block_add_conn"       // 阻塞添加连接事件
	TraceRemoveConnEvt         TraceEvtTyp = "remove_conn"          // 移除连接事件
)

// scopeClass 定义作用域类型
type scopeClass struct {
	name string // 作用域名称
}

// MarshalJSON 实现作用域类型的JSON序列化
// 返回值:
//   - []byte 序列化后的JSON数据
//   - error 序列化错误
func (s scopeClass) MarshalJSON() ([]byte, error) {
	name := s.name
	var span string
	// 提取span信息
	if idx := strings.Index(name, "span:"); idx > -1 {
		name = name[:idx-1]
		span = name[idx+5:]
	}
	// 系统和临时作用域
	if name == "system" || name == "transient" || name == "allowlistedSystem" || name == "allowlistedTransient" {
		return json.Marshal(struct {
			Class string
			Span  string `json:",omitempty"`
		}{
			Class: name,
			Span:  span,
		})
	}
	// 连接作用域
	if strings.HasPrefix(name, "conn-") {
		return json.Marshal(struct {
			Class string
			Conn  string
			Span  string `json:",omitempty"`
		}{
			Class: "conn",
			Conn:  name[5:],
			Span:  span,
		})
	}
	// 流作用域
	if strings.HasPrefix(name, "stream-") {
		return json.Marshal(struct {
			Class  string
			Stream string
			Span   string `json:",omitempty"`
		}{
			Class:  "stream",
			Stream: name[7:],
			Span:   span,
		})
	}
	// 对等节点作用域
	if strings.HasPrefix(name, "peer:") {
		return json.Marshal(struct {
			Class string
			Peer  string
			Span  string `json:",omitempty"`
		}{
			Class: "peer",
			Peer:  name[5:],
			Span:  span,
		})
	}

	// 服务作用域
	if strings.HasPrefix(name, "service:") {
		if idx := strings.Index(name, "peer:"); idx > 0 { // 对等节点-服务作用域
			return json.Marshal(struct {
				Class   string
				Service string
				Peer    string
				Span    string `json:",omitempty"`
			}{
				Class:   "service-peer",
				Service: name[8 : idx-1],
				Peer:    name[idx+5:],
				Span:    span,
			})
		} else { // 服务作用域
			return json.Marshal(struct {
				Class   string
				Service string
				Span    string `json:",omitempty"`
			}{
				Class:   "service",
				Service: name[8:],
				Span:    span,
			})
		}
	}

	// 协议作用域
	if strings.HasPrefix(name, "protocol:") {
		if idx := strings.Index(name, "peer:"); idx > -1 { // 对等节点-协议作用域
			return json.Marshal(struct {
				Class    string
				Protocol string
				Peer     string
				Span     string `json:",omitempty"`
			}{
				Class:    "protocol-peer",
				Protocol: name[9 : idx-1],
				Peer:     name[idx+5:],
				Span:     span,
			})
		} else { // 协议作用域
			return json.Marshal(struct {
				Class    string
				Protocol string
				Span     string `json:",omitempty"`
			}{
				Class:    "protocol",
				Protocol: name[9:],
				Span:     span,
			})
		}
	}

	return nil, fmt.Errorf("无法识别的作用域: %s", name)
}

// TraceEvt 定义了跟踪事件的结构体
type TraceEvt struct {
	Time string      // 事件发生的时间
	Type TraceEvtTyp // 事件类型

	Scope *scopeClass `json:",omitempty"` // 作用域类别,可选
	Name  string      `json:",omitempty"` // 作用域名称,可选

	Limit interface{} `json:",omitempty"` // 限制条件,可选

	Priority uint8 `json:",omitempty"` // 优先级,可选

	Delta    int64 `json:",omitempty"` // 变化量,可选
	DeltaIn  int   `json:",omitempty"` // 入站变化量,可选
	DeltaOut int   `json:",omitempty"` // 出站变化量,可选

	Memory int64 `json:",omitempty"` // 内存使用量,可选

	StreamsIn  int `json:",omitempty"` // 入站流数量,可选
	StreamsOut int `json:",omitempty"` // 出站流数量,可选

	ConnsIn  int `json:",omitempty"` // 入站连接数,可选
	ConnsOut int `json:",omitempty"` // 出站连接数,可选

	FD int `json:",omitempty"` // 文件描述符数量,可选
}

// push 将事件推送到跟踪器中
// 参数:
//   - evt TraceEvt 要推送的事件
func (t *trace) push(evt TraceEvt) {
	t.mx.Lock()         // 加锁保护并发访问
	defer t.mx.Unlock() // 函数返回时解锁

	if t.done { // 如果跟踪器已关闭,直接返回
		return
	}
	evt.Time = time.Now().Format(time.RFC3339Nano) // 设置事件发生时间
	if evt.Name != "" {                            // 如果事件有名称,设置作用域
		evt.Scope = &scopeClass{name: evt.Name}
	}

	// 通知所有事件报告器
	for _, reporter := range t.reporters {
		reporter.ConsumeEvent(evt)
	}

	// 如果设置了输出路径,将事件加入待写入队列
	if t.path != "" {
		t.pendingWrites = append(t.pendingWrites, evt)
	}
}

// backgroundWriter 在后台运行的事件写入器
// 参数:
//   - out io.WriteCloser 输出写入器
func (t *trace) backgroundWriter(out io.WriteCloser) {
	defer t.wg.Done() // 退出时减少等待组计数
	defer out.Close() // 关闭输出写入器

	gzOut := gzip.NewWriter(out) // 创建gzip压缩写入器
	defer gzOut.Close()          // 关闭gzip写入器

	jsonOut := json.NewEncoder(gzOut) // 创建JSON编码器

	ticker := time.NewTicker(time.Second) // 创建定时器,每秒触发一次
	defer ticker.Stop()                   // 停止定时器

	var pend []interface{} // 待处理事件列表

	// getEvents 获取待写入的事件
	getEvents := func() {
		t.mx.Lock()
		tmp := t.pendingWrites
		t.pendingWrites = pend[:0]
		pend = tmp
		t.mx.Unlock()
	}

	for {
		select {
		case <-ticker.C: // 定时器触发
			getEvents() // 获取待写入事件

			if len(pend) == 0 { // 如果没有待写入事件,继续等待
				continue
			}

			// 写入事件
			if err := t.writeEvents(pend, jsonOut); err != nil {
				log.Warnf("写入资源管理器跟踪事件错误: %s", err)
				t.mx.Lock()
				t.done = true
				t.mx.Unlock()
				return
			}

			// 刷新缓冲区
			if err := gzOut.Flush(); err != nil {
				log.Warnf("刷新资源管理器跟踪缓冲区错误: %s", err)
				t.mx.Lock()
				t.done = true
				t.mx.Unlock()
				return
			}

		case <-t.ctx.Done(): // 上下文取消
			getEvents() // 获取最后的待写入事件

			if len(pend) == 0 { // 如果没有待写入事件,直接返回
				return
			}

			// 写入最后的事件
			if err := t.writeEvents(pend, jsonOut); err != nil {
				log.Warnf("写入资源管理器跟踪事件错误: %s", err)
				return
			}

			// 刷新最后的缓冲区
			if err := gzOut.Flush(); err != nil {
				log.Warnf("刷新资源管理器跟踪缓冲区错误: %s", err)
			}

			return
		}
	}
}

// writeEvents 写入事件到JSON编码器
// 参数:
//   - pend []interface{} 待写入的事件列表
//   - jout *json.Encoder JSON编码器
//
// 返回值:
//   - error 写入错误
func (t *trace) writeEvents(pend []interface{}, jout *json.Encoder) error {
	for _, e := range pend {
		if err := jout.Encode(e); err != nil {
			log.Debugf("写入资源管理器跟踪事件失败: %v", err)
			return err
		}
	}

	return nil
}

// Start 启动跟踪器
// 参数:
//   - limits Limiter 资源限制器
//
// 返回值:
//   - error 启动错误
func (t *trace) Start(limits Limiter) error {
	if t == nil { // 如果跟踪器为空,直接返回
		return nil
	}

	// 创建上下文和取消函数
	t.ctx, t.cancel = context.WithCancel(context.Background())

	// 如果设置了输出路径,创建输出文件
	if t.path != "" {
		out, err := os.OpenFile(t.path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
		if err != nil {
			return nil
		}

		t.wg.Add(1)                // 增加等待组计数
		go t.backgroundWriter(out) // 启动后台写入器
	}

	// 推送启动事件
	t.push(TraceEvt{
		Type:  TraceStartEvt,
		Limit: limits,
	})

	return nil
}

// Close 关闭跟踪器
// 返回值:
//   - error 关闭错误
func (t *trace) Close() error {
	if t == nil { // 如果跟踪器为空,直接返回
		return nil
	}

	t.mx.Lock() // 加锁保护并发访问

	if t.done { // 如果已经关闭,解锁并返回
		t.mx.Unlock()
		return nil
	}

	t.cancel()    // 取消上下文
	t.done = true // 标记为已关闭
	t.mx.Unlock() // 解锁

	t.wg.Wait() // 等待所有后台任务完成
	return nil
}

// CreateScope 创建作用域
// 参数:
//   - scope string 作用域名称
//   - limit Limit 作用域限制
func (t *trace) CreateScope(scope string, limit Limit) {
	if t == nil { // 如果跟踪器为空,直接返回
		return
	}

	// 推送创建作用域事件
	t.push(TraceEvt{
		Type:  TraceCreateScopeEvt,
		Name:  scope,
		Limit: limit,
	})
}

// DestroyScope 销毁作用域
// 参数:
//   - scope string 作用域名称
func (t *trace) DestroyScope(scope string) {
	if t == nil { // 如果跟踪器为空,直接返回
		return
	}

	// 推送销毁作用域事件
	t.push(TraceEvt{
		Type: TraceDestroyScopeEvt,
		Name: scope,
	})
}

// ReserveMemory 预留内存
// 参数:
//   - scope string 作用域名称
//   - prio uint8 优先级
//   - size int64 预留大小
//   - mem int64 当前内存使用量
func (t *trace) ReserveMemory(scope string, prio uint8, size, mem int64) {
	if t == nil { // 如果跟踪器为空,直接返回
		return
	}

	if size == 0 { // 如果预留大小为0,直接返回
		return
	}

	// 推送预留内存事件
	t.push(TraceEvt{
		Type:     TraceReserveMemoryEvt,
		Name:     scope,
		Priority: prio,
		Delta:    size,
		Memory:   mem,
	})
}

// BlockReserveMemory 阻塞预留内存
// 参数:
//   - scope string 作用域名称
//   - prio uint8 优先级
//   - size int64 预留大小
//   - mem int64 当前内存使用量
func (t *trace) BlockReserveMemory(scope string, prio uint8, size, mem int64) {
	if t == nil { // 如果跟踪器为空,直接返回
		return
	}

	if size == 0 { // 如果预留大小为0,直接返回
		return
	}

	// 推送阻塞预留内存事件
	t.push(TraceEvt{
		Type:     TraceBlockReserveMemoryEvt,
		Name:     scope,
		Priority: prio,
		Delta:    size,
		Memory:   mem,
	})
}

// ReleaseMemory 释放内存
// 参数:
//   - scope string 作用域名称
//   - size int64 释放大小
//   - mem int64 当前内存使用量
func (t *trace) ReleaseMemory(scope string, size, mem int64) {
	if t == nil { // 如果跟踪器为空,直接返回
		return
	}

	if size == 0 { // 如果释放大小为0,直接返回
		return
	}

	// 推送释放内存事件
	t.push(TraceEvt{
		Type:   TraceReleaseMemoryEvt,
		Name:   scope,
		Delta:  -size,
		Memory: mem,
	})
}

// AddStream 添加流
// 参数:
//   - scope string 作用域名称
//   - dir network.Direction 流方向
//   - nstreamsIn int 入站流数量
//   - nstreamsOut int 出站流数量
func (t *trace) AddStream(scope string, dir network.Direction, nstreamsIn, nstreamsOut int) {
	if t == nil { // 如果跟踪器为空,直接返回
		return
	}

	var deltaIn, deltaOut int      // 定义入站和出站流的变化量
	if dir == network.DirInbound { // 如果是入站流
		deltaIn = 1
	} else { // 如果是出站流
		deltaOut = 1
	}

	// 推送添加流事件
	t.push(TraceEvt{
		Type:       TraceAddStreamEvt,
		Name:       scope,
		DeltaIn:    deltaIn,
		DeltaOut:   deltaOut,
		StreamsIn:  nstreamsIn,
		StreamsOut: nstreamsOut,
	})
}

// BlockAddStream 阻塞添加流
// 参数:
//   - scope string 作用域名称
//   - dir network.Direction 流方向
//   - nstreamsIn int 入站流数量
//   - nstreamsOut int 出站流数量
func (t *trace) BlockAddStream(scope string, dir network.Direction, nstreamsIn, nstreamsOut int) {
	if t == nil { // 如果跟踪器为空,直接返回
		return
	}

	var deltaIn, deltaOut int      // 定义入站和出站流的变化量
	if dir == network.DirInbound { // 如果是入站流
		deltaIn = 1
	} else { // 如果是出站流
		deltaOut = 1
	}

	// 推送阻塞添加流事件
	t.push(TraceEvt{
		Type:       TraceBlockAddStreamEvt,
		Name:       scope,
		DeltaIn:    deltaIn,
		DeltaOut:   deltaOut,
		StreamsIn:  nstreamsIn,
		StreamsOut: nstreamsOut,
	})
}

// RemoveStream 移除流
// 参数:
//   - scope string 作用域名称
//   - dir network.Direction 流方向
//   - nstreamsIn int 入站流数量
//   - nstreamsOut int 出站流数量
func (t *trace) RemoveStream(scope string, dir network.Direction, nstreamsIn, nstreamsOut int) {
	if t == nil { // 如果跟踪器为空,直接返回
		return
	}

	var deltaIn, deltaOut int      // 定义入站和出站流的变化量
	if dir == network.DirInbound { // 如果是入站流
		deltaIn = -1
	} else { // 如果是出站流
		deltaOut = -1
	}

	// 推送移除流事件
	t.push(TraceEvt{
		Type:       TraceRemoveStreamEvt,
		Name:       scope,
		DeltaIn:    deltaIn,
		DeltaOut:   deltaOut,
		StreamsIn:  nstreamsIn,
		StreamsOut: nstreamsOut,
	})
}

// AddStreams 批量添加流
// 参数:
//   - scope string 作用域名称
//   - deltaIn int 入站流变化量
//   - deltaOut int 出站流变化量
//   - nstreamsIn int 入站流数量
//   - nstreamsOut int 出站流数量
func (t *trace) AddStreams(scope string, deltaIn, deltaOut, nstreamsIn, nstreamsOut int) {
	if t == nil { // 如果跟踪器为空,直接返回
		return
	}

	if deltaIn == 0 && deltaOut == 0 { // 如果入站和出站流变化量都为0,直接返回
		return
	}

	// 推送批量添加流事件
	t.push(TraceEvt{
		Type:       TraceAddStreamEvt,
		Name:       scope,
		DeltaIn:    deltaIn,
		DeltaOut:   deltaOut,
		StreamsIn:  nstreamsIn,
		StreamsOut: nstreamsOut,
	})
}

// BlockAddStreams 阻塞批量添加流
// 参数:
//   - scope string 作用域名称
//   - deltaIn int 入站流变化量
//   - deltaOut int 出站流变化量
//   - nstreamsIn int 入站流数量
//   - nstreamsOut int 出站流数量
func (t *trace) BlockAddStreams(scope string, deltaIn, deltaOut, nstreamsIn, nstreamsOut int) {
	if t == nil { // 如果跟踪器为空,直接返回
		return
	}

	if deltaIn == 0 && deltaOut == 0 { // 如果入站和出站流变化量都为0,直接返回
		return
	}

	// 推送阻塞批量添加流事件
	t.push(TraceEvt{
		Type:       TraceBlockAddStreamEvt,
		Name:       scope,
		DeltaIn:    deltaIn,
		DeltaOut:   deltaOut,
		StreamsIn:  nstreamsIn,
		StreamsOut: nstreamsOut,
	})
}

// RemoveStreams 批量移除流
// 参数:
//   - scope string 作用域名称
//   - deltaIn int 入站流变化量
//   - deltaOut int 出站流变化量
//   - nstreamsIn int 入站流数量
//   - nstreamsOut int 出站流数量
func (t *trace) RemoveStreams(scope string, deltaIn, deltaOut, nstreamsIn, nstreamsOut int) {
	if t == nil { // 如果跟踪器为空,直接返回
		return
	}

	if deltaIn == 0 && deltaOut == 0 { // 如果入站和出站流变化量都为0,直接返回
		return
	}

	// 推送批量移除流事件
	t.push(TraceEvt{
		Type:       TraceRemoveStreamEvt,
		Name:       scope,
		DeltaIn:    -deltaIn,
		DeltaOut:   -deltaOut,
		StreamsIn:  nstreamsIn,
		StreamsOut: nstreamsOut,
	})
}

// AddConn 添加连接
// 参数:
//   - scope string 作用域名称
//   - dir network.Direction 连接方向
//   - usefd bool 是否使用文件描述符
//   - nconnsIn int 入站连接数量
//   - nconnsOut int 出站连接数量
//   - nfd int 文件描述符数量
func (t *trace) AddConn(scope string, dir network.Direction, usefd bool, nconnsIn, nconnsOut, nfd int) {
	if t == nil { // 如果跟踪器为空,直接返回
		return
	}

	var deltaIn, deltaOut, deltafd int // 定义入站、出站连接和文件描述符的变化量
	if dir == network.DirInbound {     // 如果是入站连接
		deltaIn = 1
	} else { // 如果是出站连接
		deltaOut = 1
	}
	if usefd { // 如果使用文件描述符
		deltafd = 1
	}

	// 推送添加连接事件
	t.push(TraceEvt{
		Type:     TraceAddConnEvt,
		Name:     scope,
		DeltaIn:  deltaIn,
		DeltaOut: deltaOut,
		Delta:    int64(deltafd),
		ConnsIn:  nconnsIn,
		ConnsOut: nconnsOut,
		FD:       nfd,
	})
}

// BlockAddConn 阻塞添加连接
// 参数:
//   - scope string 作用域名称
//   - dir network.Direction 连接方向
//   - usefd bool 是否使用文件描述符
//   - nconnsIn int 入站连接数量
//   - nconnsOut int 出站连接数量
//   - nfd int 文件描述符数量
func (t *trace) BlockAddConn(scope string, dir network.Direction, usefd bool, nconnsIn, nconnsOut, nfd int) {
	if t == nil { // 如果跟踪器为空,直接返回
		return
	}

	var deltaIn, deltaOut, deltafd int // 定义入站、出站连接和文件描述符的变化量
	if dir == network.DirInbound {     // 如果是入站连接
		deltaIn = 1
	} else { // 如果是出站连接
		deltaOut = 1
	}
	if usefd { // 如果使用文件描述符
		deltafd = 1
	}

	// 推送阻塞添加连接事件
	t.push(TraceEvt{
		Type:     TraceBlockAddConnEvt,
		Name:     scope,
		DeltaIn:  deltaIn,
		DeltaOut: deltaOut,
		Delta:    int64(deltafd),
		ConnsIn:  nconnsIn,
		ConnsOut: nconnsOut,
		FD:       nfd,
	})
}

// RemoveConn 移除连接
// 参数:
//   - scope string 作用域名称
//   - dir network.Direction 连接方向
//   - usefd bool 是否使用文件描述符
//   - nconnsIn int 入站连接数量
//   - nconnsOut int 出站连接数量
//   - nfd int 文件描述符数量
func (t *trace) RemoveConn(scope string, dir network.Direction, usefd bool, nconnsIn, nconnsOut, nfd int) {
	if t == nil { // 如果跟踪器为空,直接返回
		return
	}

	var deltaIn, deltaOut, deltafd int // 定义入站、出站连接和文件描述符的变化量
	if dir == network.DirInbound {     // 如果是入站连接
		deltaIn = -1
	} else { // 如果是出站连接
		deltaOut = -1
	}
	if usefd { // 如果使用文件描述符
		deltafd = -1
	}

	// 推送移除连接事件
	t.push(TraceEvt{
		Type:     TraceRemoveConnEvt,
		Name:     scope,
		DeltaIn:  deltaIn,
		DeltaOut: deltaOut,
		Delta:    int64(deltafd),
		ConnsIn:  nconnsIn,
		ConnsOut: nconnsOut,
		FD:       nfd,
	})
}

// AddConns 批量添加连接
// 参数:
//   - scope string 作用域名称
//   - deltaIn int 入站连接变化量
//   - deltaOut int 出站连接变化量
//   - deltafd int 文件描述符变化量
//   - nconnsIn int 入站连接数量
//   - nconnsOut int 出站连接数量
//   - nfd int 文件描述符数量
func (t *trace) AddConns(scope string, deltaIn, deltaOut, deltafd, nconnsIn, nconnsOut, nfd int) {
	if t == nil { // 如果跟踪器为空,直接返回
		return
	}

	if deltaIn == 0 && deltaOut == 0 && deltafd == 0 { // 如果所有变化量都为0,直接返回
		return
	}

	// 推送批量添加连接事件
	t.push(TraceEvt{
		Type:     TraceAddConnEvt,
		Name:     scope,
		DeltaIn:  deltaIn,
		DeltaOut: deltaOut,
		Delta:    int64(deltafd),
		ConnsIn:  nconnsIn,
		ConnsOut: nconnsOut,
		FD:       nfd,
	})
}

// BlockAddConns 阻塞批量添加连接
// 参数:
//   - scope string 作用域名称
//   - deltaIn int 入站连接变化量
//   - deltaOut int 出站连接变化量
//   - deltafd int 文件描述符变化量
//   - nconnsIn int 入站连接数量
//   - nconnsOut int 出站连接数量
//   - nfd int 文件描述符数量
func (t *trace) BlockAddConns(scope string, deltaIn, deltaOut, deltafd, nconnsIn, nconnsOut, nfd int) {
	if t == nil { // 如果跟踪器为空,直接返回
		return
	}

	if deltaIn == 0 && deltaOut == 0 && deltafd == 0 { // 如果所有变化量都为0,直接返回
		return
	}

	// 推送阻塞批量添加连接事件
	t.push(TraceEvt{
		Type:     TraceBlockAddConnEvt,
		Name:     scope,
		DeltaIn:  deltaIn,
		DeltaOut: deltaOut,
		Delta:    int64(deltafd),
		ConnsIn:  nconnsIn,
		ConnsOut: nconnsOut,
		FD:       nfd,
	})
}

// RemoveConns 批量移除连接
// 参数:
//   - scope string 作用域名称
//   - deltaIn int 入站连接变化量
//   - deltaOut int 出站连接变化量
//   - deltafd int 文件描述符变化量
//   - nconnsIn int 入站连接数量
//   - nconnsOut int 出站连接数量
//   - nfd int 文件描述符数量
func (t *trace) RemoveConns(scope string, deltaIn, deltaOut, deltafd, nconnsIn, nconnsOut, nfd int) {
	if t == nil { // 如果跟踪器为空,直接返回
		return
	}

	if deltaIn == 0 && deltaOut == 0 && deltafd == 0 { // 如果所有变化量都为0,直接返回
		return
	}

	// 推送批量移除连接事件
	t.push(TraceEvt{
		Type:     TraceRemoveConnEvt,
		Name:     scope,
		DeltaIn:  -deltaIn,
		DeltaOut: -deltaOut,
		Delta:    -int64(deltafd),
		ConnsIn:  nconnsIn,
		ConnsOut: nconnsOut,
		FD:       nfd,
	})
}
