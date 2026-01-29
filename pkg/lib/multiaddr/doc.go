// Package multiaddr 提供多地址（Multiaddr）的实现
//
// Multiaddr 是一种自描述的网络地址格式，支持多种传输协议和地址类型。
//
// # 基本用法
//
//	// 创建多地址
//	ma, err := multiaddr.NewMultiaddr("/ip4/127.0.0.1/tcp/4001")
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	// 获取字符串表示
//	fmt.Println(ma.String()) // /ip4/127.0.0.1/tcp/4001
//
//	// 获取二进制表示
//	bytes := ma.Bytes()
//
//	// 封装另一个地址
//	p2p, _ := multiaddr.NewMultiaddr("/p2p/QmYyQSo1c1Ym7orWxLYvCrM2EmxFTANf8wXmmE7DWjhx5N")
//	full := ma.Encapsulate(p2p)
//
// # 支持的协议
//
// 本包支持以下协议（与 multiformats/multicodec 完全对齐）：
//
//   - IP4/IP6: IPv4 和 IPv6 地址
//   - TCP/UDP: 传输层端口
//   - QUIC/QUIC-V1: QUIC 传输
//   - P2P: 对等节点 ID
//   - DNS/DNS4/DNS6/DNSADDR: DNS 名称
//   - WS/WSS: WebSocket
//   - TLS/NOISE: 安全传输
//   - ONION/ONION3: Tor 地址
//   - GARLIC32/GARLIC64: I2P 地址
//
// # 地址格式
//
// 字符串格式：
//
//	/ip4/127.0.0.1/tcp/4001
//	/ip6/::1/tcp/8080
//	/ip4/192.168.1.1/udp/4001/quic-v1
//	/ip4/1.2.3.4/tcp/4001/p2p/QmYyQSo1c1Ym7orWxLYvCrM2EmxFTANf8wXmmE7DWjhx5N
//	/dns/example.com/tcp/443/wss
//
// 二进制格式：
//
//	[varint:protocol_code][varint:length][data_bytes]...
//
// # 与标准网络类型转换
//
//	// 从 net.TCPAddr 创建
//	tcpAddr := &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 4001}
//	ma, err := multiaddr.FromTCPAddr(tcpAddr)
//
//	// 转换为 net.TCPAddr
//	ma, _ := multiaddr.NewMultiaddr("/ip4/127.0.0.1/tcp/4001")
//	tcpAddr, err := ma.ToTCPAddr()
//
// # 工具函数
//
//	// 分离传输地址和 P2P 组件
//	transport, peerID := multiaddr.Split(ma)
//
//	// 合并传输地址和 P2P 组件
//	full := multiaddr.Join(transport, peerID)
//
//	// 过滤地址
//	tcpAddrs := multiaddr.FilterAddrs(addrs, func(ma multiaddr.Multiaddr) bool {
//	    return multiaddr.HasProtocol(ma, multiaddr.P_TCP)
//	})
//
//	// 去重
//	unique := multiaddr.UniqueAddrs(addrs)
//
// # 二进制编码
//
// 多地址使用高效的二进制编码格式，兼容 go-multiaddr 标准实现。
// 协议代码使用 varint 编码，变长数据使用长度前缀。
//
// # 与 multiformats 对齐
//
// 所有协议代码与 multiformats/multicodec 完全对齐：
// https://github.com/multiformats/multicodec/blob/master/table.csv
package multiaddr
