package resourcemgr

import (
	"github.com/dep2p/go-dep2p/pkg/lib/multiaddr"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// mustMultiaddr 创建 Multiaddr（测试辅助）
func mustMultiaddr(s string) types.Multiaddr {
	ma, err := multiaddr.NewMultiaddr(s)
	if err != nil {
		panic(err)
	}
	return ma
}
