package rcmgr

import (
	"fmt"
	"math"
	"math/big"
	"strings"
	"sync"

	"github.com/dep2p/go-dep2p/core/network"
)

// resources 跟踪当前资源消耗状态
type resources struct {
	// limit 定义资源限制
	limit Limit

	// nconnsIn 入站连接数
	// nconnsOut 出站连接数
	nconnsIn, nconnsOut int
	// nstreamsIn 入站流数
	// nstreamsOut 出站流数
	nstreamsIn, nstreamsOut int
	// nfd 文件描述符数量
	nfd int

	// memory 内存使用量(字节)
	memory int64
}

// resourceScope 可以是一个 DAG(有向无环图),其中下游节点不允许比上游节点存活更久(即在下游节点之前不能调用上游节点的 Done),并使用线性化的父集来记录资源。
// resourceScope 也可以是一个 span scope,它有一个特定的所有者;span scopes 创建一个以所有者为根的树(所有者可以是 DAG scope),并且可以比它们的父节点存活更久
// -- 这一点很重要,因为 span scopes 是内存管理的主要*用户*接口,用户可能会在系统在某个后台 goroutine 中关闭 span 树的根之后调用 span scope 中的 Done。
// 如果我们不做这种区分,在这种情况下就会出现双重释放的问题。
type resourceScope struct {
	sync.Mutex
	// done 表示该 scope 是否已完成
	done bool
	// refCnt 引用计数
	refCnt int

	// spanID span 的唯一标识符
	spanID int

	// rc 资源消耗状态
	rc resources
	// owner 在 span scopes 中设置,定义树的所有者
	owner *resourceScope
	// edges 在 DAG scopes 中设置,是线性化的父集
	edges []*resourceScope

	// name 用于调试目的的名称
	name string
	// trace 用于调试跟踪
	trace *trace
	// metrics 用于指标收集
	metrics *metrics
}

// 确保 resourceScope 实现了 network.ResourceScope 接口
var _ network.ResourceScope = (*resourceScope)(nil)

// 确保 resourceScope 实现了 network.ResourceScopeSpan 接口
var _ network.ResourceScopeSpan = (*resourceScope)(nil)

// newResourceScope 创建一个新的资源作用域
// 参数:
//   - limit: 资源限制
//   - edges: 父作用域列表
//   - name: 作用域名称
//   - trace: 跟踪对象
//   - metrics: 指标对象
//
// 返回:
//   - *resourceScope: 新创建的资源作用域
func newResourceScope(limit Limit, edges []*resourceScope, name string, trace *trace, metrics *metrics) *resourceScope {
	// 增加所有父作用域的引用计数
	for _, e := range edges {
		e.IncRef()
	}
	// 创建新的资源作用域
	r := &resourceScope{
		rc:      resources{limit: limit}, // 设置资源限制
		edges:   edges,                   // 设置父作用域
		name:    name,                    // 设置名称
		trace:   trace,                   // 设置跟踪对象
		metrics: metrics,                 // 设置指标对象
	}
	// 记录作用域创建事件
	r.trace.CreateScope(name, limit)
	return r
}

// newResourceScopeSpan 创建一个新的资源作用域 span
// 参数:
//   - owner: 所有者作用域
//   - id: span 标识符
//
// 返回:
//   - *resourceScope: 新创建的资源作用域 span
func newResourceScopeSpan(owner *resourceScope, id int) *resourceScope {
	// 创建新的资源作用域 span
	r := &resourceScope{
		rc:      resources{limit: owner.rc.limit},          // 继承所有者的资源限制
		owner:   owner,                                     // 设置所有者
		name:    fmt.Sprintf("%s.span-%d", owner.name, id), // 生成 span 名称
		trace:   owner.trace,                               // 继承所有者的跟踪对象
		metrics: owner.metrics,                             // 继承所有者的指标对象
	}
	// 记录作用域创建事件
	r.trace.CreateScope(r.name, r.rc.limit)
	return r
}

// IsSpan 检查给定名称是否为 span 作用域
// 参数:
//   - name: 要检查的名称
//
// 返回:
//   - bool: 如果是 span 作用域则返回 true
func IsSpan(name string) bool {
	return strings.Contains(name, ".span-")
}

// addInt64WithOverflow 执行 int64 加法并检查溢出
// 参数:
//   - a: 第一个操作数
//   - b: 第二个操作数
//
// 返回:
//   - c: 计算结果
//   - ok: 如果没有溢出则为 true
func addInt64WithOverflow(a int64, b int64) (c int64, ok bool) {
	c = a + b
	return c, (c > a) == (b > 0)
}

// mulInt64WithOverflow 执行 int64 乘法并检查溢出
// 参数:
//   - a: 第一个操作数
//   - b: 第二个操作数
//
// 返回:
//   - c: 计算结果
//   - ok: 如果没有溢出则为 true
func mulInt64WithOverflow(a, b int64) (c int64, ok bool) {
	const mostPositive = 1<<63 - 1           // int64 最大正数
	const mostNegative = -(mostPositive + 1) // int64 最小负数
	c = a * b
	// 处理特殊情况
	if a == 0 || b == 0 || a == 1 || b == 1 {
		return c, true
	}
	if a == mostNegative || b == mostNegative {
		return c, false
	}
	return c, c/b == a
}

// checkMemory 检查内存分配请求是否超出限制
// 参数:
//   - rsvp: 请求分配的内存大小
//   - prio: 优先级
//
// 返回:
//   - error: 如果超出限制则返回错误
func (rc *resources) checkMemory(rsvp int64, prio uint8) error {
	// 检查是否为负数请求
	if rsvp < 0 {
		log.Debugf("不能预留负数内存. rsvp=%v", rsvp)
		return fmt.Errorf("不能预留负数内存. rsvp=%v", rsvp)
	}

	limit := rc.limit.GetMemoryLimit()
	// 处理无限制情况
	if limit == math.MaxInt64 {
		return nil
	}

	// 计算新的内存使用量
	newmem, addOk := addInt64WithOverflow(rc.memory, rsvp)

	// 计算阈值
	threshold, mulOk := mulInt64WithOverflow(1+int64(prio), limit)
	if !mulOk {
		// 处理乘法溢出情况
		thresholdBig := big.NewInt(limit)
		thresholdBig.Mul(thresholdBig, big.NewInt(1+int64(prio)))
		thresholdBig.Rsh(thresholdBig, 8) // 除以 256
		threshold = thresholdBig.Int64()
	} else {
		threshold = threshold / 256
	}

	// 检查是否超出限制
	if !addOk || newmem > threshold {
		return &ErrMemoryLimitExceeded{
			current:   rc.memory,
			attempted: rsvp,
			limit:     limit,
			priority:  prio,
			err:       network.ErrResourceLimitExceeded,
		}
	}
	return nil
}

// reserveMemory 预留内存
// 参数:
//   - size: 要预留的内存大小
//   - prio: 优先级
//
// 返回:
//   - error: 如果预留失败则返回错误
func (rc *resources) reserveMemory(size int64, prio uint8) error {
	// 检查内存限制
	if err := rc.checkMemory(size, prio); err != nil {
		log.Debugf("内存预留失败: %v", err)
		return err
	}

	// 增加内存使用量
	rc.memory += size
	return nil
}

// releaseMemory 释放内存
// 参数:
//   - size: 要释放的内存大小
func (rc *resources) releaseMemory(size int64) {
	rc.memory -= size

	// 检查是否出现负数(这是一个 bug)
	if rc.memory < 0 {
		log.Warn("BUG: 释放了过多内存")
		rc.memory = 0
	}
}

// addStream 添加一个流
// 参数:
//   - dir: 流的方向(入站/出站)
//
// 返回:
//   - error: 如果添加失败则返回错误
func (rc *resources) addStream(dir network.Direction) error {
	if dir == network.DirInbound {
		return rc.addStreams(1, 0)
	}
	return rc.addStreams(0, 1)
}

// addStreams 添加多个流
// 参数:
//   - incount: 要添加的入站流数量
//   - outcount: 要添加的出站流数量
//
// 返回:
//   - error: 如果添加失败则返回错误
func (rc *resources) addStreams(incount, outcount int) error {
	// 检查入站流限制
	if incount > 0 {
		limit := rc.limit.GetStreamLimit(network.DirInbound)
		if rc.nstreamsIn+incount > limit {
			return &ErrStreamOrConnLimitExceeded{
				current:   rc.nstreamsIn,
				attempted: incount,
				limit:     limit,
				err:       fmt.Errorf("无法预留入站流: %w", network.ErrResourceLimitExceeded),
			}
		}
	}
	// 检查出站流限制
	if outcount > 0 {
		limit := rc.limit.GetStreamLimit(network.DirOutbound)
		if rc.nstreamsOut+outcount > limit {
			return &ErrStreamOrConnLimitExceeded{
				current:   rc.nstreamsOut,
				attempted: outcount,
				limit:     limit,
				err:       fmt.Errorf("无法预留出站流: %w", network.ErrResourceLimitExceeded),
			}
		}
	}

	// 检查总流限制
	if limit := rc.limit.GetStreamTotalLimit(); rc.nstreamsIn+incount+rc.nstreamsOut+outcount > limit {
		return &ErrStreamOrConnLimitExceeded{
			current:   rc.nstreamsIn + rc.nstreamsOut,
			attempted: incount + outcount,
			limit:     limit,
			err:       fmt.Errorf("无法预留流: %w", network.ErrResourceLimitExceeded),
		}
	}

	// 更新流计数
	rc.nstreamsIn += incount
	rc.nstreamsOut += outcount
	return nil
}

// removeStream 移除一个流
// 参数:
//   - dir: 流的方向(入站/出站)
func (rc *resources) removeStream(dir network.Direction) {
	// 根据方向移除流
	if dir == network.DirInbound {
		rc.removeStreams(1, 0)
	} else {
		rc.removeStreams(0, 1)
	}
}

// removeStreams 移除多个流
// 参数:
//   - incount: 要移除的入站流数量
//   - outcount: 要移除的出站流数量
func (rc *resources) removeStreams(incount, outcount int) {
	// 减少流计数
	rc.nstreamsIn -= incount
	rc.nstreamsOut -= outcount

	// 检查是否出现负数(这是一个 bug)
	if rc.nstreamsIn < 0 {
		log.Warn("BUG: 释放了过多入站流")
		rc.nstreamsIn = 0
	}
	if rc.nstreamsOut < 0 {
		log.Warn("BUG: 释放了过多出站流")
		rc.nstreamsOut = 0
	}
}

// addConn 添加一个连接
// 参数:
//   - dir: 连接的方向(入站/出站)
//   - usefd: 是否使用文件描述符
//
// 返回:
//   - error: 如果添加失败则返回错误
func (rc *resources) addConn(dir network.Direction, usefd bool) error {
	// 确定文件描述符使用数量
	var fd int
	if usefd {
		fd = 1
	}

	// 根据方向添加连接
	if dir == network.DirInbound {
		return rc.addConns(1, 0, fd)
	}

	return rc.addConns(0, 1, fd)
}

// addConns 添加多个连接
// 参数:
//   - incount: 要添加的入站连接数量
//   - outcount: 要添加的出站连接数量
//   - fdcount: 要使用的文件描述符数量
//
// 返回:
//   - error: 如果添加失败则返回错误
func (rc *resources) addConns(incount, outcount, fdcount int) error {
	// 检查入站连接限制
	if incount > 0 {
		limit := rc.limit.GetConnLimit(network.DirInbound)
		if rc.nconnsIn+incount > limit {
			return &ErrStreamOrConnLimitExceeded{
				current:   rc.nconnsIn,
				attempted: incount,
				limit:     limit,
				err:       fmt.Errorf("无法预留入站连接: %w", network.ErrResourceLimitExceeded),
			}
		}
	}
	// 检查出站连接限制
	if outcount > 0 {
		limit := rc.limit.GetConnLimit(network.DirOutbound)
		if rc.nconnsOut+outcount > limit {
			return &ErrStreamOrConnLimitExceeded{
				current:   rc.nconnsOut,
				attempted: outcount,
				limit:     limit,
				err:       fmt.Errorf("无法预留出站连接: %w", network.ErrResourceLimitExceeded),
			}
		}
	}

	// 检查总连接限制
	if connLimit := rc.limit.GetConnTotalLimit(); rc.nconnsIn+incount+rc.nconnsOut+outcount > connLimit {
		return &ErrStreamOrConnLimitExceeded{
			current:   rc.nconnsIn + rc.nconnsOut,
			attempted: incount + outcount,
			limit:     connLimit,
			err:       fmt.Errorf("无法预留连接: %w", network.ErrResourceLimitExceeded),
		}
	}
	// 检查文件描述符限制
	if fdcount > 0 {
		limit := rc.limit.GetFDLimit()
		if rc.nfd+fdcount > limit {
			return &ErrStreamOrConnLimitExceeded{
				current:   rc.nfd,
				attempted: fdcount,
				limit:     limit,
				err:       fmt.Errorf("无法预留文件描述符: %w", network.ErrResourceLimitExceeded),
			}
		}
	}

	// 更新连接和文件描述符计数
	rc.nconnsIn += incount
	rc.nconnsOut += outcount
	rc.nfd += fdcount
	return nil
}

// removeConn 移除一个连接
// 参数:
//   - dir: 连接的方向(入站/出站)
//   - usefd: 是否释放文件描述符
func (rc *resources) removeConn(dir network.Direction, usefd bool) {
	// 确定文件描述符释放数量
	var fd int
	if usefd {
		fd = 1
	}

	// 根据方向移除连接
	if dir == network.DirInbound {
		rc.removeConns(1, 0, fd)
	} else {
		rc.removeConns(0, 1, fd)
	}
}

// removeConns 移除多个连接
// 参数:
//   - incount: 要移除的入站连接数量
//   - outcount: 要移除的出站连接数量
//   - fdcount: 要释放的文件描述符数量
func (rc *resources) removeConns(incount, outcount, fdcount int) {
	// 减少连接和文件描述符计数
	rc.nconnsIn -= incount
	rc.nconnsOut -= outcount
	rc.nfd -= fdcount

	// 检查是否出现负数(这是一个 bug)
	if rc.nconnsIn < 0 {
		log.Warn("BUG: 释放了过多入站连接")
		rc.nconnsIn = 0
	}
	if rc.nconnsOut < 0 {
		log.Warn("BUG: 释放了过多出站连接")
		rc.nconnsOut = 0
	}
	if rc.nfd < 0 {
		log.Warn("BUG: 释放了过多文件描述符")
		rc.nfd = 0
	}
}

// stat 获取资源作用域的统计信息
// 返回值:
//   - network.ScopeStat: 作用域统计信息
func (rc *resources) stat() network.ScopeStat {
	return network.ScopeStat{
		Memory:             rc.memory,      // 内存使用量
		NumStreamsInbound:  rc.nstreamsIn,  // 入站流数量
		NumStreamsOutbound: rc.nstreamsOut, // 出站流数量
		NumConnsInbound:    rc.nconnsIn,    // 入站连接数量
		NumConnsOutbound:   rc.nconnsOut,   // 出站连接数量
		NumFD:              rc.nfd,         // 文件描述符数量
	}
}

// resourceScope 实现

// wrapError 包装错误信息,添加作用域名称
// 参数:
//   - err: error - 原始错误
//
// 返回值:
//   - error: 包装后的错误
func (s *resourceScope) wrapError(err error) error {
	return fmt.Errorf("%s: %w", s.name, err)
}

// ReserveMemory 预留内存
// 参数:
//   - size: int - 要预留的内存大小
//   - prio: uint8 - 优先级
//
// 返回值:
//   - error: 如果预留失败则返回错误
func (s *resourceScope) ReserveMemory(size int, prio uint8) error {
	s.Lock()         // 加锁保护并发访问
	defer s.Unlock() // 函数返回时解锁

	if s.done { // 如果作用域已关闭
		return s.wrapError(network.ErrResourceScopeClosed)
	}

	// 尝试预留内存
	if err := s.rc.reserveMemory(int64(size), prio); err != nil {
		log.Debugw("内存预留被阻塞", logValuesMemoryLimit(s.name, "", s.rc.stat(), err)...)
		s.trace.BlockReserveMemory(s.name, prio, int64(size), s.rc.memory)
		s.metrics.BlockMemory(size)
		return s.wrapError(err)
	}

	// 为边缘节点预留内存
	if err := s.reserveMemoryForEdges(size, prio); err != nil {
		s.rc.releaseMemory(int64(size))
		s.metrics.BlockMemory(size)
		return s.wrapError(err)
	}

	// 记录内存预留成功
	s.trace.ReserveMemory(s.name, prio, int64(size), s.rc.memory)
	s.metrics.AllowMemory(size)
	return nil
}

// reserveMemoryForEdges 为边缘节点预留内存
// 参数:
//   - size: int - 要预留的内存大小
//   - prio: uint8 - 优先级
//
// 返回值:
//   - error: 如果预留失败则返回错误
func (s *resourceScope) reserveMemoryForEdges(size int, prio uint8) error {
	if s.owner != nil { // 如果有所有者
		return s.owner.ReserveMemory(size, prio)
	}

	var reserved int
	var err error
	// 为每个边缘节点预留内存
	for _, e := range s.edges {
		var stat network.ScopeStat
		stat, err = e.ReserveMemoryForChild(int64(size), prio)
		if err != nil {
			log.Debugw("边缘节点内存预留被阻塞", logValuesMemoryLimit(s.name, e.name, stat, err)...)
			break
		}
		reserved++
	}

	if err != nil { // 如果预留失败
		// 撤销已完成的内存预留
		for _, e := range s.edges[:reserved] {
			e.ReleaseMemoryForChild(int64(size))
		}
	}

	return err
}

// releaseMemoryForEdges 为边缘节点释放内存
// 参数:
//   - size: int - 要释放的内存大小
func (s *resourceScope) releaseMemoryForEdges(size int) {
	if s.owner != nil { // 如果有所有者
		s.owner.ReleaseMemory(size)
		return
	}

	// 为每个边缘节点释放内存
	for _, e := range s.edges {
		e.ReleaseMemoryForChild(int64(size))
	}
}

// ReserveMemoryForChild 为子节点预留内存
// 参数:
//   - size: int64 - 要预留的内存大小
//   - prio: uint8 - 优先级
//
// 返回值:
//   - network.ScopeStat: 作用域统计信息
//   - error: 如果预留失败则返回错误
func (s *resourceScope) ReserveMemoryForChild(size int64, prio uint8) (network.ScopeStat, error) {
	s.Lock()         // 加锁保护并发访问
	defer s.Unlock() // 函数返回时解锁

	if s.done { // 如果作用域已关闭
		return s.rc.stat(), s.wrapError(network.ErrResourceScopeClosed)
	}

	// 尝试预留内存
	if err := s.rc.reserveMemory(size, prio); err != nil {
		s.trace.BlockReserveMemory(s.name, prio, size, s.rc.memory)
		return s.rc.stat(), s.wrapError(err)
	}

	s.trace.ReserveMemory(s.name, prio, size, s.rc.memory)
	return network.ScopeStat{}, nil
}

// ReleaseMemory 释放内存
// 参数:
//   - size: int - 要释放的内存大小
func (s *resourceScope) ReleaseMemory(size int) {
	s.Lock()         // 加锁保护并发访问
	defer s.Unlock() // 函数返回时解锁

	if s.done { // 如果作用域已关闭
		return
	}

	s.rc.releaseMemory(int64(size))                         // 释放内存
	s.releaseMemoryForEdges(size)                           // 为边缘节点释放内存
	s.trace.ReleaseMemory(s.name, int64(size), s.rc.memory) // 记录内存释放
}

// ReleaseMemoryForChild 为子节点释放内存
// 参数:
//   - size: int64 - 要释放的内存大小
func (s *resourceScope) ReleaseMemoryForChild(size int64) {
	s.Lock()         // 加锁保护并发访问
	defer s.Unlock() // 函数返回时解锁

	if s.done { // 如果作用域已关闭
		return
	}

	s.rc.releaseMemory(size)                         // 释放内存
	s.trace.ReleaseMemory(s.name, size, s.rc.memory) // 记录内存释放
}

// AddStream 添加流
// 参数:
//   - dir: network.Direction - 流的方向(入站/出站)
//
// 返回值:
//   - error: 如果添加失败则返回错误
func (s *resourceScope) AddStream(dir network.Direction) error {
	s.Lock()         // 加锁保护并发访问
	defer s.Unlock() // 函数返回时解锁

	if s.done { // 如果作用域已关闭
		return s.wrapError(network.ErrResourceScopeClosed)
	}

	// 尝试添加流
	if err := s.rc.addStream(dir); err != nil {
		log.Debugw("流添加被阻塞", logValuesStreamLimit(s.name, "", dir, s.rc.stat(), err)...)
		s.trace.BlockAddStream(s.name, dir, s.rc.nstreamsIn, s.rc.nstreamsOut)
		return s.wrapError(err)
	}

	// 为边缘节点添加流
	if err := s.addStreamForEdges(dir); err != nil {
		s.rc.removeStream(dir)
		return s.wrapError(err)
	}

	s.trace.AddStream(s.name, dir, s.rc.nstreamsIn, s.rc.nstreamsOut)
	return nil
}

// addStreamForEdges 为边缘节点添加流
// 参数:
//   - dir: network.Direction - 流的方向(入站/出站)
//
// 返回值:
//   - error: 如果添加失败则返回错误
func (s *resourceScope) addStreamForEdges(dir network.Direction) error {
	if s.owner != nil { // 如果有所有者
		return s.owner.AddStream(dir)
	}

	var err error
	var reserved int
	// 为每个边缘节点添加流
	for _, e := range s.edges {
		var stat network.ScopeStat
		stat, err = e.AddStreamForChild(dir)
		if err != nil {
			log.Debugw("边缘节点流添加被阻塞", logValuesStreamLimit(s.name, e.name, dir, stat, err)...)
			break
		}
		reserved++
	}

	if err != nil { // 如果添加失败
		// 撤销已完成的流添加
		for _, e := range s.edges[:reserved] {
			e.RemoveStreamForChild(dir)
		}
	}

	return err
}

// AddStreamForChild 为子节点添加流
// 参数:
//   - dir: network.Direction - 流的方向(入站/出站)
//
// 返回值:
//   - network.ScopeStat: 作用域统计信息
//   - error: 如果添加失败则返回错误
func (s *resourceScope) AddStreamForChild(dir network.Direction) (network.ScopeStat, error) {
	s.Lock()         // 加锁保护并发访问
	defer s.Unlock() // 函数返回时解锁

	if s.done { // 如果作用域已关闭
		return s.rc.stat(), s.wrapError(network.ErrResourceScopeClosed)
	}

	// 尝试添加流
	if err := s.rc.addStream(dir); err != nil {
		s.trace.BlockAddStream(s.name, dir, s.rc.nstreamsIn, s.rc.nstreamsOut)
		return s.rc.stat(), s.wrapError(err)
	}

	s.trace.AddStream(s.name, dir, s.rc.nstreamsIn, s.rc.nstreamsOut)
	return network.ScopeStat{}, nil
}

// RemoveStream 移除流
// 参数:
//   - dir: network.Direction - 流的方向(入站/出站)
func (s *resourceScope) RemoveStream(dir network.Direction) {
	s.Lock()         // 加锁保护并发访问
	defer s.Unlock() // 函数返回时解锁

	if s.done { // 如果作用域已关闭
		return
	}

	s.rc.removeStream(dir)                                               // 从资源计数器中移除流
	s.removeStreamForEdges(dir)                                          // 从边缘节点中移除流
	s.trace.RemoveStream(s.name, dir, s.rc.nstreamsIn, s.rc.nstreamsOut) // 记录流移除的跟踪信息
}

// removeStreamForEdges 从边缘节点移除流
// 参数:
//   - dir: network.Direction - 流的方向(入站/出站)
func (s *resourceScope) removeStreamForEdges(dir network.Direction) {
	if s.owner != nil { // 如果有所有者
		s.owner.RemoveStream(dir) // 从所有者中移除流
		return
	}

	for _, e := range s.edges { // 遍历所有边缘节点
		e.RemoveStreamForChild(dir) // 从每个边缘节点中移除子节点的流
	}
}

// RemoveStreamForChild 为子节点移除流
// 参数:
//   - dir: network.Direction - 流的方向(入站/出站)
func (s *resourceScope) RemoveStreamForChild(dir network.Direction) {
	s.Lock()         // 加锁保护并发访问
	defer s.Unlock() // 函数返回时解锁

	if s.done { // 如果作用域已关闭
		return
	}

	s.rc.removeStream(dir)                                               // 从资源计数器中移除流
	s.trace.RemoveStream(s.name, dir, s.rc.nstreamsIn, s.rc.nstreamsOut) // 记录流移除的跟踪信息
}

// AddConn 添加连接
// 参数:
//   - dir: network.Direction - 连接的方向(入站/出站)
//   - usefd: bool - 是否使用文件描述符
//
// 返回值:
//   - error: 如果添加失败则返回错误
func (s *resourceScope) AddConn(dir network.Direction, usefd bool) error {
	s.Lock()         // 加锁保护并发访问
	defer s.Unlock() // 函数返回时解锁

	if s.done { // 如果作用域已关闭
		return s.wrapError(network.ErrResourceScopeClosed)
	}

	if err := s.rc.addConn(dir, usefd); err != nil { // 尝试添加连接到资源计数器
		log.Debugw("连接被阻塞", logValuesConnLimit(s.name, "", dir, usefd, s.rc.stat(), err)...)
		s.trace.BlockAddConn(s.name, dir, usefd, s.rc.nconnsIn, s.rc.nconnsOut, s.rc.nfd)
		return s.wrapError(err)
	}

	if err := s.addConnForEdges(dir, usefd); err != nil { // 尝试添加连接到边缘节点
		s.rc.removeConn(dir, usefd) // 如果失败则回滚资源计数器的更改
		return s.wrapError(err)
	}

	s.trace.AddConn(s.name, dir, usefd, s.rc.nconnsIn, s.rc.nconnsOut, s.rc.nfd) // 记录连接添加的跟踪信息
	return nil
}

// addConnForEdges 为边缘节点添加连接
// 参数:
//   - dir: network.Direction - 连接的方向(入站/出站)
//   - usefd: bool - 是否使用文件描述符
//
// 返回值:
//   - error: 如果添加失败则返回错误
func (s *resourceScope) addConnForEdges(dir network.Direction, usefd bool) error {
	if s.owner != nil { // 如果有所有者
		return s.owner.AddConn(dir, usefd) // 在所有者中添加连接
	}

	var err error
	var reserved int
	for _, e := range s.edges { // 遍历所有边缘节点
		var stat network.ScopeStat
		stat, err = e.AddConnForChild(dir, usefd) // 在每个边缘节点中为子节点添加连接
		if err != nil {
			log.Debugf("边缘节点连接添加被阻塞", logValuesConnLimit(s.name, e.name, dir, usefd, stat, err)...)
			break
		}
		reserved++
	}

	if err != nil { // 如果添加失败
		for _, e := range s.edges[:reserved] { // 撤销已完成的连接添加
			e.RemoveConnForChild(dir, usefd)
		}
	}

	return err
}

// AddConnForChild 为子节点添加连接
// 参数:
//   - dir: network.Direction - 连接的方向(入站/出站)
//   - usefd: bool - 是否使用文件描述符
//
// 返回值:
//   - network.ScopeStat: 作用域统计信息
//   - error: 如果添加失败则返回错误
func (s *resourceScope) AddConnForChild(dir network.Direction, usefd bool) (network.ScopeStat, error) {
	s.Lock()         // 加锁保护并发访问
	defer s.Unlock() // 函数返回时解锁

	if s.done { // 如果作用域已关闭
		return s.rc.stat(), s.wrapError(network.ErrResourceScopeClosed)
	}

	if err := s.rc.addConn(dir, usefd); err != nil { // 尝试添加连接到资源计数器
		s.trace.BlockAddConn(s.name, dir, usefd, s.rc.nconnsIn, s.rc.nconnsOut, s.rc.nfd)
		return s.rc.stat(), s.wrapError(err)
	}

	s.trace.AddConn(s.name, dir, usefd, s.rc.nconnsIn, s.rc.nconnsOut, s.rc.nfd) // 记录连接添加的跟踪信息
	return network.ScopeStat{}, nil
}

// RemoveConn 移除连接
// 参数:
//   - dir: network.Direction - 连接的方向(入站/出站)
//   - usefd: bool - 是否使用文件描述符
func (s *resourceScope) RemoveConn(dir network.Direction, usefd bool) {
	s.Lock()         // 加锁保护并发访问
	defer s.Unlock() // 函数返回时解锁

	if s.done { // 如果作用域已关闭
		return
	}

	s.rc.removeConn(dir, usefd)                                                     // 从资源计数器中移除连接
	s.removeConnForEdges(dir, usefd)                                                // 从边缘节点中移除连接
	s.trace.RemoveConn(s.name, dir, usefd, s.rc.nconnsIn, s.rc.nconnsOut, s.rc.nfd) // 记录连接移除的跟踪信息
}

// removeConnForEdges 从边缘节点移除连接
// 参数:
//   - dir: network.Direction - 连接的方向(入站/出站)
//   - usefd: bool - 是否使用文件描述符
func (s *resourceScope) removeConnForEdges(dir network.Direction, usefd bool) {
	if s.owner != nil { // 如果有所有者
		s.owner.RemoveConn(dir, usefd) // 从所有者中移除连接
	}

	for _, e := range s.edges { // 遍历所有边缘节点
		e.RemoveConnForChild(dir, usefd) // 从每个边缘节点中移除子节点的连接
	}
}

// RemoveConnForChild 为子节点移除连接
// 参数:
//   - dir: network.Direction - 连接的方向(入站/出站)
//   - usefd: bool - 是否使用文件描述符
func (s *resourceScope) RemoveConnForChild(dir network.Direction, usefd bool) {
	s.Lock()         // 加锁保护并发访问
	defer s.Unlock() // 函数返回时解锁

	if s.done { // 如果作用域已关闭
		return
	}

	s.rc.removeConn(dir, usefd)                                                     // 从资源计数器中移除连接
	s.trace.RemoveConn(s.name, dir, usefd, s.rc.nconnsIn, s.rc.nconnsOut, s.rc.nfd) // 记录连接移除的跟踪信息
}

// ReserveForChild 为子作用域预留资源
// 参数:
//   - st: network.ScopeStat - 需要预留的资源统计信息
//
// 返回值:
//   - error 预留资源过程中的错误,如果成功则返回nil
func (s *resourceScope) ReserveForChild(st network.ScopeStat) error {
	s.Lock()         // 加锁保护并发访问
	defer s.Unlock() // 函数返回时解锁

	if s.done { // 如果作用域已关闭
		return s.wrapError(network.ErrResourceScopeClosed) // 返回作用域已关闭错误
	}

	if err := s.rc.reserveMemory(st.Memory, network.ReservationPriorityAlways); err != nil { // 尝试预留内存资源
		s.trace.BlockReserveMemory(s.name, 255, st.Memory, s.rc.memory) // 记录内存预留被阻塞的跟踪信息
		return s.wrapError(err)                                         // 返回预留内存失败的错误
	}

	if err := s.rc.addStreams(st.NumStreamsInbound, st.NumStreamsOutbound); err != nil { // 尝试添加流资源
		s.trace.BlockAddStreams(s.name, st.NumStreamsInbound, st.NumStreamsOutbound, s.rc.nstreamsIn, s.rc.nstreamsOut) // 记录添加流被阻塞的跟踪信息
		s.rc.releaseMemory(st.Memory)                                                                                   // 释放已预留的内存资源
		return s.wrapError(err)                                                                                         // 返回添加流失败的错误
	}

	if err := s.rc.addConns(st.NumConnsInbound, st.NumConnsOutbound, st.NumFD); err != nil { // 尝试添加连接资源
		s.trace.BlockAddConns(s.name, st.NumConnsInbound, st.NumConnsOutbound, st.NumFD, s.rc.nconnsIn, s.rc.nconnsOut, s.rc.nfd) // 记录添加连接被阻塞的跟踪信息

		s.rc.releaseMemory(st.Memory)                                   // 释放已预留的内存资源
		s.rc.removeStreams(st.NumStreamsInbound, st.NumStreamsOutbound) // 移除已添加的流资源
		return s.wrapError(err)                                         // 返回添加连接失败的错误
	}

	// 记录资源预留成功的跟踪信息
	s.trace.ReserveMemory(s.name, 255, st.Memory, s.rc.memory)
	s.trace.AddStreams(s.name, st.NumStreamsInbound, st.NumStreamsOutbound, s.rc.nstreamsIn, s.rc.nstreamsOut)
	s.trace.AddConns(s.name, st.NumConnsInbound, st.NumConnsOutbound, st.NumFD, s.rc.nconnsIn, s.rc.nconnsOut, s.rc.nfd)

	return nil
}

// ReleaseForChild 为子作用域释放资源
// 参数:
//   - st: network.ScopeStat - 需要释放的资源统计信息
func (s *resourceScope) ReleaseForChild(st network.ScopeStat) {
	s.Lock()         // 加锁保护并发访问
	defer s.Unlock() // 函数返回时解锁

	if s.done { // 如果作用域已关闭
		return
	}

	s.rc.releaseMemory(st.Memory)                                       // 释放内存资源
	s.rc.removeStreams(st.NumStreamsInbound, st.NumStreamsOutbound)     // 移除流资源
	s.rc.removeConns(st.NumConnsInbound, st.NumConnsOutbound, st.NumFD) // 移除连接资源

	// 记录资源释放的跟踪信息
	s.trace.ReleaseMemory(s.name, st.Memory, s.rc.memory)
	s.trace.RemoveStreams(s.name, st.NumStreamsInbound, st.NumStreamsOutbound, s.rc.nstreamsIn, s.rc.nstreamsOut)
	s.trace.RemoveConns(s.name, st.NumConnsInbound, st.NumConnsOutbound, st.NumFD, s.rc.nconnsIn, s.rc.nconnsOut, s.rc.nfd)
}

// ReleaseResources 释放资源并通知所有者或边缘节点
// 参数:
//   - st: network.ScopeStat - 需要释放的资源统计信息
func (s *resourceScope) ReleaseResources(st network.ScopeStat) {
	s.Lock()         // 加锁保护并发访问
	defer s.Unlock() // 函数返回时解锁

	if s.done { // 如果作用域已关闭
		return
	}

	s.rc.releaseMemory(st.Memory)                                       // 释放内存资源
	s.rc.removeStreams(st.NumStreamsInbound, st.NumStreamsOutbound)     // 移除流资源
	s.rc.removeConns(st.NumConnsInbound, st.NumConnsOutbound, st.NumFD) // 移除连接资源

	if s.owner != nil { // 如果有所有者
		s.owner.ReleaseResources(st) // 通知所有者释放资源
	} else { // 如果没有所有者
		for _, e := range s.edges { // 遍历所有边缘节点
			e.ReleaseForChild(st) // 通知每个边缘节点释放子节点的资源
		}
	}

	// 记录资源释放的跟踪信息
	s.trace.ReleaseMemory(s.name, st.Memory, s.rc.memory)
	s.trace.RemoveStreams(s.name, st.NumStreamsInbound, st.NumStreamsOutbound, s.rc.nstreamsIn, s.rc.nstreamsOut)
	s.trace.RemoveConns(s.name, st.NumConnsInbound, st.NumConnsOutbound, st.NumFD, s.rc.nconnsIn, s.rc.nconnsOut, s.rc.nfd)
}

// nextSpanID 生成下一个跨度ID
// 返回值:
//   - int - 新生成的跨度ID
func (s *resourceScope) nextSpanID() int {
	s.spanID++ // 递增跨度ID计数器
	return s.spanID
}

// BeginSpan 开始一个新的资源作用域跨度
// 返回值:
//   - network.ResourceScopeSpan - 新创建的资源作用域跨度
//   - error 创建过程中的错误,如果成功则返回nil
func (s *resourceScope) BeginSpan() (network.ResourceScopeSpan, error) {
	s.Lock()         // 加锁保护并发访问
	defer s.Unlock() // 函数返回时解锁

	if s.done { // 如果作用域已关闭
		return nil, s.wrapError(network.ErrResourceScopeClosed) // 返回作用域已关闭错误
	}

	s.refCnt++                                          // 增加引用计数
	return newResourceScopeSpan(s, s.nextSpanID()), nil // 创建并返回新的资源作用域跨度
}

// Done 标记资源作用域为已完成状态
func (s *resourceScope) Done() {
	s.Lock()         // 加锁保护并发访问
	defer s.Unlock() // 函数返回时解锁

	s.doneUnlocked() // 执行未加锁的完成操作
}

// doneUnlocked 在已加锁的情况下执行完成操作
func (s *resourceScope) doneUnlocked() {
	if s.done { // 如果作用域已关闭
		return
	}
	stat := s.rc.stat() // 获取当前资源统计信息
	if s.owner != nil { // 如果有所有者
		s.owner.ReleaseResources(stat) // 通知所有者释放资源
		s.owner.DecRef()               // 减少所有者的引用计数
	} else { // 如果没有所有者
		for _, e := range s.edges { // 遍历所有边缘节点
			e.ReleaseForChild(stat) // 通知每个边缘节点释放子节点的资源
			e.DecRef()              // 减少边缘节点的引用计数
		}
	}

	// 重置所有资源计数
	s.rc.nstreamsIn = 0
	s.rc.nstreamsOut = 0
	s.rc.nconnsIn = 0
	s.rc.nconnsOut = 0
	s.rc.nfd = 0
	s.rc.memory = 0

	s.done = true // 标记作用域为已完成

	s.trace.DestroyScope(s.name) // 记录作用域销毁的跟踪信息
}

// Stat 获取资源作用域的统计信息
// 返回值:
//   - network.ScopeStat - 当前资源统计信息
func (s *resourceScope) Stat() network.ScopeStat {
	s.Lock()         // 加锁保护并发访问
	defer s.Unlock() // 函数返回时解锁

	return s.rc.stat() // 返回资源计数器的统计信息
}

// IncRef 增加资源作用域的引用计数
func (s *resourceScope) IncRef() {
	s.Lock()         // 加锁保护并发访问
	defer s.Unlock() // 函数返回时解锁

	s.refCnt++ // 增加引用计数
}

// DecRef 减少资源作用域的引用计数
func (s *resourceScope) DecRef() {
	s.Lock()         // 加锁保护并发访问
	defer s.Unlock() // 函数返回时解锁

	s.refCnt-- // 减少引用计数
}

// IsUnused 检查资源作用域是否未被使用
// 返回值:
//   - bool - 如果作用域未被使用则返回true,否则返回false
func (s *resourceScope) IsUnused() bool {
	s.Lock()         // 加锁保护并发访问
	defer s.Unlock() // 函数返回时解锁

	if s.done { // 如果作用域已关闭
		return true
	}

	if s.refCnt > 0 { // 如果引用计数大于0
		return false
	}

	st := s.rc.stat()                   // 获取当前资源统计信息
	return st.NumStreamsInbound == 0 && // 检查是否所有资源计数都为0
		st.NumStreamsOutbound == 0 &&
		st.NumConnsInbound == 0 &&
		st.NumConnsOutbound == 0 &&
		st.NumFD == 0
}
