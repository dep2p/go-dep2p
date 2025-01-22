//go:build tools

package dep2p

import (
	_ "github.com/golang/mock/mockgen"
	_ "github.com/quic-go/quic-go/http3"
	_ "go.uber.org/mock/mockgen"
	_ "golang.org/x/tools/cmd/goimports"
	_ "google.golang.org/protobuf/cmd/protoc-gen-go"
)
