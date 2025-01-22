//go:build gomock || generate

package basichost

// 生成 NAT 接口的 mock 实现,用于测试
//
//go:generate sh -c "go run go.uber.org/mock/mockgen -build_flags=\"-tags=gomock\" -package basichost -destination mock_nat_test.go github.com/dep2p/p2p/host/basic NAT"
type NAT nat
