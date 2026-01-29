package metrics

// Stats 带宽统计快照
//
// Stats 表示某个时间点的带宽指标快照。
// TotalIn 和 TotalOut 记录累计发送/接收字节数。
// RateIn 和 RateOut 记录每秒发送/接收字节数。
type Stats struct {
	TotalIn  int64   // 总入站字节
	TotalOut int64   // 总出站字节
	RateIn   float64 // 入站速率（字节/秒）
	RateOut  float64 // 出站速率（字节/秒）
}
