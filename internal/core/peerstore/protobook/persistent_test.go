package protobook

import (
	"path/filepath"
	"testing"

	"github.com/dep2p/go-dep2p/internal/core/storage/engine"
	"github.com/dep2p/go-dep2p/internal/core/storage/engine/badger"
	"github.com/dep2p/go-dep2p/internal/core/storage/kv"
	"github.com/dep2p/go-dep2p/pkg/types"
)

func newTestPersistentProtoBook(t *testing.T) (*PersistentProtoBook, func()) {
	t.Helper()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	cfg := engine.DefaultConfig(dbPath)
	eng, err := badger.New(cfg)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	store := kv.New(eng, []byte("p/p/"))
	pb, err := NewPersistent(store)
	if err != nil {
		eng.Close()
		t.Fatalf("failed to create persistent protobook: %v", err)
	}

	cleanup := func() {
		eng.Close()
	}

	return pb, cleanup
}

func TestPersistentProtoBook_AddAndGet(t *testing.T) {
	pb, cleanup := newTestPersistentProtoBook(t)
	defer cleanup()

	peerID := types.PeerID("test-peer-1")
	proto1 := types.ProtocolID("/test/proto/1.0.0")
	proto2 := types.ProtocolID("/test/proto/2.0.0")

	// 添加协议
	if err := pb.AddProtocols(peerID, proto1, proto2); err != nil {
		t.Fatalf("failed to add protocols: %v", err)
	}

	// 获取协议
	protos, err := pb.GetProtocols(peerID)
	if err != nil {
		t.Fatalf("failed to get protocols: %v", err)
	}

	if len(protos) != 2 {
		t.Fatalf("expected 2 protocols, got %d", len(protos))
	}
}

func TestPersistentProtoBook_SetProtocols(t *testing.T) {
	pb, cleanup := newTestPersistentProtoBook(t)
	defer cleanup()

	peerID := types.PeerID("test-peer-1")
	proto1 := types.ProtocolID("/test/proto/1.0.0")
	proto2 := types.ProtocolID("/test/proto/2.0.0")

	// 添加协议
	if err := pb.AddProtocols(peerID, proto1); err != nil {
		t.Fatalf("failed to add protocols: %v", err)
	}

	// 设置协议（覆盖）
	if err := pb.SetProtocols(peerID, proto2); err != nil {
		t.Fatalf("failed to set protocols: %v", err)
	}

	// 验证
	protos, _ := pb.GetProtocols(peerID)
	if len(protos) != 1 {
		t.Fatalf("expected 1 protocol, got %d", len(protos))
	}

	if protos[0] != proto2 {
		t.Errorf("expected %s, got %s", proto2, protos[0])
	}
}

func TestPersistentProtoBook_RemoveProtocols(t *testing.T) {
	pb, cleanup := newTestPersistentProtoBook(t)
	defer cleanup()

	peerID := types.PeerID("test-peer-1")
	proto1 := types.ProtocolID("/test/proto/1.0.0")
	proto2 := types.ProtocolID("/test/proto/2.0.0")

	// 添加协议
	pb.AddProtocols(peerID, proto1, proto2)

	// 移除协议
	if err := pb.RemoveProtocols(peerID, proto1); err != nil {
		t.Fatalf("failed to remove protocols: %v", err)
	}

	// 验证
	protos, _ := pb.GetProtocols(peerID)
	if len(protos) != 1 {
		t.Fatalf("expected 1 protocol, got %d", len(protos))
	}

	if protos[0] != proto2 {
		t.Errorf("expected %s, got %s", proto2, protos[0])
	}
}

func TestPersistentProtoBook_SupportsProtocols(t *testing.T) {
	pb, cleanup := newTestPersistentProtoBook(t)
	defer cleanup()

	peerID := types.PeerID("test-peer-1")
	proto1 := types.ProtocolID("/test/proto/1.0.0")
	proto2 := types.ProtocolID("/test/proto/2.0.0")
	proto3 := types.ProtocolID("/test/proto/3.0.0")

	// 添加协议
	pb.AddProtocols(peerID, proto1, proto2)

	// 检查支持
	supported, err := pb.SupportsProtocols(peerID, proto1, proto3)
	if err != nil {
		t.Fatalf("failed to check protocols: %v", err)
	}

	if len(supported) != 1 {
		t.Fatalf("expected 1 supported protocol, got %d", len(supported))
	}

	if supported[0] != proto1 {
		t.Errorf("expected %s, got %s", proto1, supported[0])
	}
}

func TestPersistentProtoBook_FirstSupportedProtocol(t *testing.T) {
	pb, cleanup := newTestPersistentProtoBook(t)
	defer cleanup()

	peerID := types.PeerID("test-peer-1")
	proto1 := types.ProtocolID("/test/proto/1.0.0")
	proto2 := types.ProtocolID("/test/proto/2.0.0")

	// 添加协议
	pb.AddProtocols(peerID, proto1, proto2)

	// 获取首个支持的协议
	first, err := pb.FirstSupportedProtocol(peerID, proto2, proto1)
	if err != nil {
		t.Fatalf("failed to get first supported protocol: %v", err)
	}

	if first != proto2 {
		t.Errorf("expected %s, got %s", proto2, first)
	}
}

func TestPersistentProtoBook_Persistence(t *testing.T) {
	// 创建引擎
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "persist.db")
	cfg := engine.DefaultConfig(dbPath)
	eng, err := badger.New(cfg)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}
	defer eng.Close()

	store := kv.New(eng, []byte("p/p/"))
	peerID := types.PeerID("test-peer-1")
	proto := types.ProtocolID("/test/proto/1.0.0")

	// 创建第一个 ProtoBook 并写入数据
	pb1, err := NewPersistent(store)
	if err != nil {
		t.Fatalf("failed to create persistent protobook: %v", err)
	}

	if err := pb1.AddProtocols(peerID, proto); err != nil {
		t.Fatalf("failed to add protocols: %v", err)
	}

	// 创建第二个 ProtoBook，验证数据持久化
	pb2, err := NewPersistent(store)
	if err != nil {
		t.Fatalf("failed to create persistent protobook: %v", err)
	}

	protos, _ := pb2.GetProtocols(peerID)
	if len(protos) != 1 {
		t.Fatalf("expected 1 protocol after reload, got %d", len(protos))
	}

	if protos[0] != proto {
		t.Errorf("expected %s, got %s", proto, protos[0])
	}
}

func TestPersistentProtoBook_RemovePeer(t *testing.T) {
	pb, cleanup := newTestPersistentProtoBook(t)
	defer cleanup()

	peerID := types.PeerID("test-peer-1")
	proto := types.ProtocolID("/test/proto/1.0.0")

	// 添加协议
	pb.AddProtocols(peerID, proto)

	// 移除节点
	pb.RemovePeer(peerID)

	// 验证协议已清除
	protos, _ := pb.GetProtocols(peerID)
	if len(protos) != 0 {
		t.Fatalf("expected 0 protocols, got %d", len(protos))
	}
}
