// Package pnet 提供 dep2p 中私有网络的接口。
package pnet

// PSK 使私有网络实现在 dep2p 中透明化。
// 它用于确保对等点只能与使用相同 PSK 的其他对等点建立连接。
type PSK []byte
