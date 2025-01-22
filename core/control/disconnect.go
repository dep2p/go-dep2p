package control

// DisconnectReason 表示连接关闭的原因。
//
// 零值表示"无原因"/不适用。
//
// 这是一个实验性类型。它在未来可能会发生变化。更多信息请参考 connmgr.ConnectionGater 的 godoc。
type DisconnectReason int
