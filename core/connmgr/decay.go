package connmgr

import (
	"io"
	"time"

	"github.com/dep2p/go-dep2p/core/peer"
)

// Decayer 由支持衰减标签的连接管理器实现。
// 衰减标签是一个其值会随时间自动衰减的标签。
//
// 实际的衰减行为封装在用户提供的衰减函数(DecayFn)中。
// 该函数在每个时间间隔(由interval参数确定)被调用,并返回标签的新值,或者标签是否应该被删除。
//
// 我们不直接设置衰减标签的值。
// 相反,我们通过增量来"提升"衰减标签。
// 这会调用BumpFn函数,传入旧值和增量,来确定新值。
//
// 这种可插拔的设计提供了很大的灵活性和多功能性。
// 容易实现的行为包括:
//
//   - 每个时间间隔将标签值减1,或减半。
//   - 每次提升值时,将其与当前值相加。
//   - 每次提升时指数级提高分数。
//   - 将输入分数相加,但保持在最小值和最大值范围内。
//
// 本包提供了常用的DecayFns和BumpFns。
type Decayer interface {
	io.Closer

	// RegisterDecayingTag 创建并注册一个新的衰减标签,当且仅当具有提供名称的标签不存在时。否则,返回错误。
	//
	// 调用者提供刷新标签的时间间隔,以及衰减函数和提升函数。
	// 有关更多信息,请参阅DecayFn和BumpFn的godoc。
	RegisterDecayingTag(name string, interval time.Duration, decayFn DecayFn, bumpFn BumpFn) (DecayingTag, error)
}

// DecayFn 对对等节点的分数应用衰减。
// 实现必须在注册标签时提供的时间间隔调用DecayFn。
//
// 它接收衰减值的副本,并返回应用衰减后的分数,以及一个标志以指示是否应删除该标签。
type DecayFn func(value DecayingValue) (after int, rm bool)

// BumpFn 将增量应用到现有分数上,并返回新分数。
//
// 非平凡的提升函数包括指数提升、移动平均、上限等。
type BumpFn func(value DecayingValue, delta int) (after int)

// DecayingTag 表示一个衰减标签。
// 该标签是一个长期存在的通用对象,用于操作对等节点的标签值。
type DecayingTag interface {
	// Name 返回标签的名称。
	Name() string

	// Interval 是此标签将滴答的有效时间间隔。
	// 在注册时,根据衰减器的分辨率,所需的时间间隔可能会被覆盖,此方法允许您获取有效的时间间隔。
	Interval() time.Duration

	// Bump 对标签值应用增量,调用其提升函数。
	// 提升将异步应用,非nil错误表示排队时出现故障。
	Bump(peer peer.ID, delta int) error

	// Remove 从对等节点中移除衰减标签。
	// 移除将异步应用,非nil错误表示排队时出现故障。
	Remove(peer peer.ID) error

	// Close 关闭衰减标签。
	// Decayer将停止跟踪此标签,并且连接管理器中持有此标签的所有对等节点的状态将被更新。
	//
	// 删除是异步执行的。
	//
	// 一旦删除,标签就不应再使用,后续调用Bump/Remove将返回错误。
	//
	// 重复调用Remove不会返回错误,但如果无法将第一个实际移除排队(例如,当系统积压时),则会返回错误。
	Close() error
}

// DecayingValue 表示衰减标签的值。
type DecayingValue struct {
	// Tag 指向此值所属的标签。
	Tag DecayingTag

	// Peer 是与此值关联的对等节点ID。
	Peer peer.ID

	// Added 是首次为标签和对等节点添加此值的时间戳。
	Added time.Time

	// LastVisit 是最后一次访问的时间戳。
	LastVisit time.Time

	// Value 是标签的当前值。
	Value int
}
