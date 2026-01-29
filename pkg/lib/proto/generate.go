// Package proto 包含 DeP2P 的 Protobuf 定义
//
// 使用以下命令生成 Go 代码：
//   go generate ./...
//
//go:generate protoc --go_out=. --go_opt=paths=source_relative common/common.proto
//go:generate protoc --go_out=. --go_opt=paths=source_relative peer/peer.proto
//go:generate protoc --go_out=. --go_opt=paths=source_relative key/key.proto
//go:generate protoc --go_out=. --go_opt=paths=source_relative identify/identify.proto
//go:generate protoc --go_out=. --go_opt=paths=source_relative autonat/autonat.proto
//go:generate protoc --go_out=. --go_opt=paths=source_relative holepunch/holepunch.proto
//go:generate protoc --go_out=. --go_opt=paths=source_relative relay/relay.proto
//go:generate protoc --go_out=. --go_opt=paths=source_relative relay/voucher.proto
//go:generate protoc --go_out=. --go_opt=paths=source_relative dht/dht.proto
//go:generate protoc --go_out=. --go_opt=paths=source_relative rendezvous/rendezvous.proto
//go:generate protoc --go_out=. --go_opt=paths=source_relative realm/realm.proto
//go:generate protoc --go_out=. --go_opt=paths=source_relative gossipsub/gossipsub.proto
//go:generate protoc --go_out=. --go_opt=paths=source_relative messaging/messaging.proto
//go:generate protoc --go_out=. --go_opt=paths=source_relative addressbook/addressbook.proto
package proto
