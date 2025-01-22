package event

// RawJSON 是一个包含原始 JSON 字符串的类型
type RawJSON string

// GenericDHTEvent 是一个通过携带原始 JSON 来封装实际 DHT 事件的类型
//
// 上下文: DHT 事件系统目前比较特殊且有点混乱,所以在我们统一/清理它之前,这个事件用于过渡。
// 它应该只用于信息展示目的。
//
// 实验性: 当 DHT 事件类型被提升到核心层,且 DHT 事件系统与事件总线统一后,这个类型可能会被移除。
type GenericDHTEvent struct {
	// Type 是发生的 DHT 事件的类型
	Type string

	// Raw 是事件载荷的原始 JSON 表示
	Raw RawJSON
}
