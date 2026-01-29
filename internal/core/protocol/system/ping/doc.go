// Package ping 实现存活检测协议
//
// ping 协议用于检测节点是否存活，测量往返延迟（RTT）。
//
// # 协议 ID
//
//   /dep2p/sys/ping/1.0.0
//
// # 消息格式
//
// 请求和响应都是 32 字节的随机数据，响应必须与请求相同。
//
// # 使用示例
//
//	rtt, err := ping.Ping(ctx, host, peerID)
//	if err != nil {
//	    log.Printf("节点不可达: %v", err)
//	} else {
//	    log.Printf("RTT: %v", rtt)
//	}
package ping
