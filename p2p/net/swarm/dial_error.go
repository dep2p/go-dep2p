package swarm

import (
	"fmt"
	"os"
	"strings"

	"github.com/dep2p/go-dep2p/core/peer"

	ma "github.com/dep2p/go-dep2p/multiformats/multiaddr"
)

// maxDialDialErrors 是我们记录的最大拨号错误数量
const maxDialDialErrors = 16

// DialError 是拨号时返回的错误类型
type DialError struct {
	// Peer 是目标节点的ID
	Peer peer.ID
	// DialErrors 记录每个传输层的错误
	DialErrors []TransportError
	// Cause 是导致拨号失败的根本原因
	Cause error
	// Skipped 记录由于超出最大错误数而跳过的错误数量
	Skipped int
}

// Timeout 检查错误是否为超时错误
// 返回值:
//   - bool 如果是超时错误返回true,否则返回false
func (e *DialError) Timeout() bool {
	return os.IsTimeout(e.Cause)
}

// recordErr 记录特定地址的拨号错误
// 参数:
//   - addr: ma.Multiaddr 发生错误的多地址
//   - err: error 具体的错误信息
func (e *DialError) recordErr(addr ma.Multiaddr, err error) {
	// 如果错误数量达到上限,只增加跳过计数
	if len(e.DialErrors) >= maxDialDialErrors {
		e.Skipped++
		return
	}
	// 添加新的传输错误记录
	e.DialErrors = append(e.DialErrors, TransportError{Address: addr, Cause: err})
}

// Error 实现error接口,返回格式化的错误信息
// 返回值:
//   - string 格式化后的错误信息
func (e *DialError) Error() string {
	var builder strings.Builder
	fmt.Fprintf(&builder, "拨号 %s 失败:", e.Peer)
	if e.Cause != nil {
		fmt.Fprintf(&builder, " %s", e.Cause)
	}
	for _, te := range e.DialErrors {
		fmt.Fprintf(&builder, "\n  * [%s] %s", te.Address, te.Cause)
	}
	if e.Skipped > 0 {
		fmt.Fprintf(&builder, "\n    ... 跳过 %d 个错误 ...", e.Skipped)
	}
	return builder.String()
}

// Unwrap 实现错误链,返回所有子错误
// 返回值:
//   - []error 包含所有子错误的切片
func (e *DialError) Unwrap() []error {
	if e == nil {
		return nil
	}

	errs := make([]error, 0, len(e.DialErrors)+1)
	if e.Cause != nil {
		errs = append(errs, e.Cause)
	}
	for i := 0; i < len(e.DialErrors); i++ {
		errs = append(errs, &e.DialErrors[i])
	}
	return errs
}

// 确保 DialError 实现了 error 接口
var _ error = (*DialError)(nil)

// TransportError 是拨号特定地址时返回的错误类型
type TransportError struct {
	// Address 是尝试连接的多地址
	Address ma.Multiaddr
	// Cause 是具体的错误原因
	Cause error
}

// Error 实现error接口,返回格式化的错误信息
// 返回值:
//   - string 格式化后的错误信息
func (e *TransportError) Error() string {
	log.Debugf("拨号 %s 失败: %s", e.Address, e.Cause)
	return fmt.Sprintf("拨号 %s 失败: %s", e.Address, e.Cause)
}

// Unwrap 返回底层错误
// 返回值:
//   - error 底层的错误对象
func (e *TransportError) Unwrap() error {
	return e.Cause
}

// 确保 TransportError 实现了 error 接口
var _ error = (*TransportError)(nil)
