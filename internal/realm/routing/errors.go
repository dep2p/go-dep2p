package routing

import "errors"

var (
	// ErrRouteNotFound 路由未找到
	ErrRouteNotFound = errors.New("routing: route not found")

	// ErrNodeNotFound 节点未找到
	ErrNodeNotFound = errors.New("routing: node not found")

	// ErrPathNotFound 路径未找到
	ErrPathNotFound = errors.New("routing: path not found")

	// ErrInvalidNode 无效的节点
	ErrInvalidNode = errors.New("routing: invalid node")

	// ErrInvalidRoute 无效的路由
	ErrInvalidRoute = errors.New("routing: invalid route")

	// ErrInvalidPath 无效的路径
	ErrInvalidPath = errors.New("routing: invalid path")

	// ErrInvalidConfig 无效的配置
	ErrInvalidConfig = errors.New("routing: invalid config")

	// ErrTableFull 路由表已满
	ErrTableFull = errors.New("routing: routing table is full")

	// ErrNoViablePath 无可行路径
	ErrNoViablePath = errors.New("routing: no viable path")

	// ErrMaxHopsExceeded 超过最大跳数
	ErrMaxHopsExceeded = errors.New("routing: max hops exceeded")

	// ErrNodeOverloaded 节点过载
	ErrNodeOverloaded = errors.New("routing: node is overloaded")

	// ErrTimeout 超时
	ErrTimeout = errors.New("routing: timeout")

	// ErrRouterClosed 路由器已关闭
	ErrRouterClosed = errors.New("routing: router is closed")

	// ErrNotStarted 未启动
	ErrNotStarted = errors.New("routing: not started")

	// ErrAlreadyStarted 已启动
	ErrAlreadyStarted = errors.New("routing: already started")

	// ErrNoDHT DHT 不可用
	ErrNoDHT = errors.New("routing: DHT is not available")
)
