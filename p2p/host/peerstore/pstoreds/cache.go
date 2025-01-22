package pstoreds

// cache 定义缓存接口,抽象了从ARCCache访问的所有方法
// 以支持其他实现(如空操作实现)
type cache[K comparable, V any] interface {
	// Get 获取键对应的值
	// 参数:
	//   - key: K 键
	// 返回:
	//   - value: V 值
	//   - ok: bool 是否存在
	Get(key K) (value V, ok bool)

	// Add 添加键值对
	// 参数:
	//   - key: K 键
	//   - value: V 值
	Add(key K, value V)

	// Remove 移除键值对
	// 参数:
	//   - key: K 键
	Remove(key K)

	// Contains 检查键是否存在
	// 参数:
	//   - key: K 键
	// 返回:
	//   - bool 是否存在
	Contains(key K) bool

	// Peek 获取键对应的值但不更新访问状态
	// 参数:
	//   - key: K 键
	// 返回:
	//   - value: V 值
	//   - ok: bool 是否存在
	Peek(key K) (value V, ok bool)

	// Keys 获取所有键的列表
	// 返回:
	//   - []K 键列表
	Keys() []K
}

// noopCache 是一个空操作缓存实现,用于禁用缓存时
type noopCache[K comparable, V any] struct {
}

// 确保noopCache实现了cache接口
var _ cache[int, int] = (*noopCache[int, int])(nil)

// Get 获取键对应的值,始终返回零值和false
func (*noopCache[K, V]) Get(key K) (value V, ok bool) {
	// 返回类型V的零值和false
	return *new(V), false
}

// Add 添加键值对,空操作
func (*noopCache[K, V]) Add(key K, value V) {
	// 空操作
}

// Remove 移除键值对,空操作
func (*noopCache[K, V]) Remove(key K) {
	// 空操作
}

// Contains 检查键是否存在,始终返回false
func (*noopCache[K, V]) Contains(key K) bool {
	// 始终返回false
	return false
}

// Peek 获取键对应的值但不更新访问状态,始终返回零值和false
func (*noopCache[K, V]) Peek(key K) (value V, ok bool) {
	// 返回类型V的零值和false
	return *new(V), false
}

// Keys 获取所有键的列表,始终返回空列表
func (*noopCache[K, V]) Keys() (keys []K) {
	// 返回空列表
	return keys
}
