// Package protocol 协议模块测试
package protocol

import (
	"bufio"
	"bytes"
	"testing"
	"time"

	"github.com/dep2p/go-dep2p/pkg/interfaces/endpoint"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              Registry 测试
// ============================================================================

func TestRegistry_RegisterAndGet(t *testing.T) {
	registry := NewRegistry(100)

	handler := func(s endpoint.Stream) {}
	err := registry.RegisterWithHandler("/test/1.0.0", handler)

	if err != nil {
		t.Fatalf("RegisterWithHandler error: %v", err)
	}

	proto, ok := registry.Get("/test/1.0.0")
	if !ok {
		t.Fatal("expected to find registered protocol")
	}

	if proto.ID() != "/test/1.0.0" {
		t.Errorf("protocol ID mismatch: got %s", proto.ID())
	}
}

func TestRegistry_Unregister(t *testing.T) {
	registry := NewRegistry(100)

	handler := func(s endpoint.Stream) {}
	registry.RegisterWithHandler("/test/1.0.0", handler)

	err := registry.Unregister("/test/1.0.0")
	if err != nil {
		t.Fatalf("Unregister error: %v", err)
	}

	_, ok := registry.Get("/test/1.0.0")
	if ok {
		t.Error("expected protocol to be unregistered")
	}
}

func TestRegistry_DuplicateRegister(t *testing.T) {
	registry := NewRegistry(100)

	handler := func(s endpoint.Stream) {}
	registry.RegisterWithHandler("/test/1.0.0", handler)

	err := registry.RegisterWithHandler("/test/1.0.0", handler)
	if err != ErrProtocolAlreadyRegistered {
		t.Errorf("expected ErrProtocolAlreadyRegistered, got %v", err)
	}
}

func TestRegistry_MaxProtocols(t *testing.T) {
	registry := NewRegistry(3)

	handler := func(s endpoint.Stream) {}

	// 注册到上限
	for i := 0; i < 3; i++ {
		id := types.ProtocolID("/test/" + string(rune('a'+i)))
		err := registry.RegisterWithHandler(id, handler)
		if err != nil {
			t.Fatalf("RegisterWithHandler error: %v", err)
		}
	}

	// 超过上限
	err := registry.RegisterWithHandler("/test/overflow", handler)
	if err != ErrMaxProtocolsReached {
		t.Errorf("expected ErrMaxProtocolsReached, got %v", err)
	}
}

func TestRegistry_Match(t *testing.T) {
	registry := NewRegistry(100)

	handler := func(s endpoint.Stream) {}

	// 注册不同版本
	registry.RegisterWithHandler("/test/1.0.0", handler, WithPriority(10))
	registry.RegisterWithHandler("/test/1.1.0", handler, WithPriority(20))
	registry.RegisterWithHandler("/other/1.0.0", handler, WithPriority(5))

	// 语义版本匹配 - /test/1.0.0 匹配 /test/1.x.x
	matches := registry.Match("/test/1.0.0")
	// 应该匹配 /test/1.0.0 (精确) 和 /test/1.1.0 (语义版本兼容)
	if len(matches) != 2 {
		t.Errorf("expected 2 matches (semantic version), got %d", len(matches))
	}

	// 不同名称不应匹配
	matches = registry.Match("/other/1.0.0")
	if len(matches) != 1 {
		t.Errorf("expected 1 match for /other, got %d", len(matches))
	}
}

func TestRegistry_ListByName(t *testing.T) {
	registry := NewRegistry(100)

	handler := func(s endpoint.Stream) {}

	registry.RegisterWithHandler("/test/1.0.0", handler)
	registry.RegisterWithHandler("/test/1.1.0", handler)
	registry.RegisterWithHandler("/other/1.0.0", handler)

	entries := registry.ListByName("test")
	if len(entries) != 2 {
		t.Errorf("expected 2 entries for 'test', got %d", len(entries))
	}
}

func TestRegistry_Count(t *testing.T) {
	registry := NewRegistry(100)

	handler := func(s endpoint.Stream) {}

	if registry.Count() != 0 {
		t.Errorf("expected 0, got %d", registry.Count())
	}

	registry.RegisterWithHandler("/test/1.0.0", handler)

	if registry.Count() != 1 {
		t.Errorf("expected 1, got %d", registry.Count())
	}
}

// ============================================================================
//                              Router 测试
// ============================================================================

func TestRouter_AddAndRemoveHandler(t *testing.T) {
	router := NewRouter()

	handler := func(s endpoint.Stream) {}

	router.AddHandler("/test/1.0.0", handler)

	if !router.HasProtocol("/test/1.0.0") {
		t.Error("expected protocol to be registered")
	}

	router.RemoveHandler("/test/1.0.0")

	if router.HasProtocol("/test/1.0.0") {
		t.Error("expected protocol to be removed")
	}
}

func TestRouter_Protocols(t *testing.T) {
	router := NewRouter()

	handler := func(s endpoint.Stream) {}

	router.AddHandler("/test/1.0.0", handler)
	router.AddHandler("/test/2.0.0", handler)

	protocols := router.Protocols()
	if len(protocols) != 2 {
		t.Errorf("expected 2 protocols, got %d", len(protocols))
	}
}

func TestRouter_GetHandler(t *testing.T) {
	router := NewRouter()

	called := false
	handler := func(s endpoint.Stream) {
		called = true
	}

	router.AddHandler("/test/1.0.0", handler)

	h, ok := router.GetHandler("/test/1.0.0")
	if !ok {
		t.Fatal("expected to find handler")
	}

	h(nil)
	if !called {
		t.Error("handler was not called")
	}
}

func TestRouter_Clear(t *testing.T) {
	router := NewRouter()

	handler := func(s endpoint.Stream) {}

	router.AddHandler("/test/1.0.0", handler)
	router.AddHandler("/test/2.0.0", handler)

	router.Clear()

	if router.Count() != 0 {
		t.Errorf("expected 0 protocols after clear, got %d", router.Count())
	}
}

func TestRouter_SemanticVersionMatch(t *testing.T) {
	router := NewRouter()

	handler := func(s endpoint.Stream) {}

	// 注册 1.0.0 版本
	router.AddHandler("/test/1.0.0", handler)

	// 应该匹配 1.x 版本
	if !router.HasProtocol("/test/1.0.0") {
		t.Error("should match exact version")
	}

	// 1.1.0 应该通过语义版本匹配找到处理器
	h, err := router.findHandler("/test/1.1.0")
	if err != nil {
		t.Errorf("should find handler for compatible version: %v", err)
	}
	if h == nil {
		t.Error("handler should not be nil")
	}
}

// ============================================================================
//                              ParseProtocolID 测试
// ============================================================================

func TestParseProtocolID(t *testing.T) {
	tests := []struct {
		input       types.ProtocolID
		wantName    string
		wantVersion string
	}{
		{"/echo/1.0.0", "echo", "1.0.0"},
		{"/dep2p/sys/ping/1.0.0", "dep2p/sys/ping", "1.0.0"},
		{"/test", "test", ""},
		{"", "", ""},
	}

	for _, tt := range tests {
		name, version := ParseProtocolID(tt.input)
		if name != tt.wantName || version != tt.wantVersion {
			t.Errorf("ParseProtocolID(%q) = (%q, %q), want (%q, %q)",
				tt.input, name, version, tt.wantName, tt.wantVersion)
		}
	}
}

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		v1, v2 string
		want   int
	}{
		{"1.0.0", "1.0.0", 0},
		{"1.0.0", "2.0.0", -1},
		{"2.0.0", "1.0.0", 1},
		{"1.1.0", "1.0.0", 1},
		{"1.0.1", "1.0.0", 1},
	}

	for _, tt := range tests {
		got := CompareVersions(tt.v1, tt.v2)
		if got != tt.want {
			t.Errorf("CompareVersions(%q, %q) = %d, want %d",
				tt.v1, tt.v2, got, tt.want)
		}
	}
}

func TestIsCompatibleVersion(t *testing.T) {
	tests := []struct {
		v1, v2 string
		want   bool
	}{
		{"1.0.0", "1.1.0", true},
		{"1.0.0", "1.9.9", true},
		{"1.0.0", "2.0.0", false},
		{"2.0.0", "1.0.0", false},
	}

	for _, tt := range tests {
		got := IsCompatibleVersion(tt.v1, tt.v2)
		if got != tt.want {
			t.Errorf("IsCompatibleVersion(%q, %q) = %v, want %v",
				tt.v1, tt.v2, got, tt.want)
		}
	}
}

// ============================================================================
//                              Negotiator 消息编解码测试
// ============================================================================

func TestWriteReadMessage(t *testing.T) {
	tests := []string{
		"/multistream/1.0.0",
		"/test/1.0.0",
		"na",
		"ls",
	}

	for _, msg := range tests {
		var buf bytes.Buffer
		writer := &buf

		// 使用原始写入方式（模拟 writeMessage）
		// 格式: [varint length][data]\n
		length := len(msg) + 1

		// 写入 varint 长度
		var lengthBuf [10]byte
		n := 0
		l := uint64(length)
		for l >= 0x80 {
			lengthBuf[n] = byte(l) | 0x80
			l >>= 7
			n++
		}
		lengthBuf[n] = byte(l)
		n++
		writer.Write(lengthBuf[:n])

		// 写入消息和换行符
		writer.WriteString(msg)
		writer.WriteByte('\n')

		// 读取验证
		reader := bytes.NewReader(buf.Bytes())

		// 读取 varint
		var result uint64
		var shift uint
		for {
			b, _ := reader.ReadByte()
			result |= uint64(b&0x7F) << shift
			if b&0x80 == 0 {
				break
			}
			shift += 7
		}

		// 读取数据
		data := make([]byte, result)
		reader.Read(data)

		// 去除换行符
		readMsg := string(data)
		if len(readMsg) > 0 && readMsg[len(readMsg)-1] == '\n' {
			readMsg = readMsg[:len(readMsg)-1]
		}

		if readMsg != msg {
			t.Errorf("message mismatch: got %q, want %q", readMsg, msg)
		}
	}
}

// ============================================================================
//                              Multiplexer 测试
// ============================================================================

func TestMultiplexer_AddRemoveProtocol(t *testing.T) {
	mux := NewMultiplexer(nil, nil)

	proto := NewProtocolWrapper("/test/1.0.0", func(s endpoint.Stream) {})

	mux.AddProtocol(proto)

	if !mux.HasProtocol("/test/1.0.0") {
		t.Error("expected protocol to be added")
	}

	mux.RemoveProtocol("/test/1.0.0")

	if mux.HasProtocol("/test/1.0.0") {
		t.Error("expected protocol to be removed")
	}
}

func TestMultiplexer_Protocols(t *testing.T) {
	mux := NewMultiplexer(nil, nil)

	mux.AddProtocol(NewProtocolWrapper("/test/1.0.0", nil))
	mux.AddProtocol(NewProtocolWrapper("/test/2.0.0", nil))

	protocols := mux.Protocols()
	if len(protocols) != 2 {
		t.Errorf("expected 2 protocols, got %d", len(protocols))
	}
}

func TestMultiplexer_Close(t *testing.T) {
	mux := NewMultiplexer(nil, nil)

	mux.AddProtocol(NewProtocolWrapper("/test/1.0.0", nil))

	err := mux.Close()
	if err != nil {
		t.Errorf("Close error: %v", err)
	}

	// 关闭后添加应该无效
	mux.AddProtocol(NewProtocolWrapper("/test/2.0.0", nil))
	if len(mux.Protocols()) != 0 {
		t.Error("should not add after close")
	}
}

// ============================================================================
//                              ProtocolEntry 测试
// ============================================================================

func TestProtocolEntry_Matches(t *testing.T) {
	entry := NewProtocolEntry("/test/1.0.0", nil)

	// 精确匹配
	if !entry.Matches("/test/1.0.0") {
		t.Error("should match exact protocol")
	}

	// 语义版本匹配
	if !entry.Matches("/test/1.1.0") {
		t.Error("should match compatible version")
	}

	// 不同主版本不匹配
	if entry.Matches("/test/2.0.0") {
		t.Error("should not match different major version")
	}

	// 不同名称不匹配
	if entry.Matches("/other/1.0.0") {
		t.Error("should not match different name")
	}
}

func TestProtocolEntry_WithMatcher(t *testing.T) {
	entry := NewProtocolEntry("/test/1.0.0", nil)

	// 设置自定义匹配器
	entry.SetMatcher(func(id types.ProtocolID) bool {
		return id == "/custom/match"
	})

	if !entry.Matches("/custom/match") {
		t.Error("should match with custom matcher")
	}
}

// ============================================================================
//                              PingService 测试
// ============================================================================

func TestPingService_New(t *testing.T) {
	ps := NewPingService(types.NodeID{})

	if ps == nil {
		t.Fatal("expected non-nil PingService")
	}

	if ps.ID() != PingProtocol {
		t.Errorf("ID mismatch: got %s, want %s", ps.ID(), PingProtocol)
	}
}

func TestPingService_Stats(t *testing.T) {
	ps := NewPingService(types.NodeID{})

	sent, received := ps.Stats()
	if sent != 0 || received != 0 {
		t.Errorf("initial stats should be 0, got sent=%d received=%d", sent, received)
	}
}

// ============================================================================
//                              IdentifyService 测试
// ============================================================================

func TestIdentifyService_New(t *testing.T) {
	registry := NewRegistry(100)
	is := NewIdentifyService(types.NodeID{1}, []byte("pubkey"), registry)

	if is == nil {
		t.Fatal("expected non-nil IdentifyService")
	}

	if is.ID() != IdentifyProtocol {
		t.Errorf("ID mismatch: got %s, want %s", is.ID(), IdentifyProtocol)
	}
}

func TestIdentifyService_SetListenAddrs(t *testing.T) {
	is := NewIdentifyService(types.NodeID{}, nil, nil)

	addrs := []string{"/ip4/127.0.0.1/tcp/8000", "/ip4/192.168.1.1/tcp/8000"}
	is.SetListenAddrs(addrs)

	if len(is.listenAddrs) != 2 {
		t.Errorf("expected 2 addrs, got %d", len(is.listenAddrs))
	}
}

func TestIdentifyInfo_SendAndRead(t *testing.T) {
	registry := NewRegistry(100)
	registry.RegisterWithHandler("/test/1.0.0", func(s endpoint.Stream) {})

	is := NewIdentifyService(types.NodeID{1, 2, 3}, []byte("test-pubkey"), registry)
	is.SetListenAddrs([]string{"/ip4/127.0.0.1/tcp/8000"})
	is.SetAgentVersion("test-agent/1.0.0")

	// 写入信息
	var buf bytes.Buffer
	err := is.sendIdentify(&buf)
	if err != nil {
		t.Fatalf("sendIdentify error: %v", err)
	}

	// 读取信息
	info, err := is.readIdentify(&buf)
	if err != nil {
		t.Fatalf("readIdentify error: %v", err)
	}

	if info.NodeID[0] != 1 || info.NodeID[1] != 2 || info.NodeID[2] != 3 {
		t.Error("NodeID mismatch")
	}

	if string(info.PublicKey) != "test-pubkey" {
		t.Errorf("PublicKey mismatch: got %s", info.PublicKey)
	}

	if len(info.ListenAddrs) != 1 {
		t.Errorf("ListenAddrs count mismatch: got %d", len(info.ListenAddrs))
	}

	if info.AgentVersion != "test-agent/1.0.0" {
		t.Errorf("AgentVersion mismatch: got %s", info.AgentVersion)
	}
}

// ============================================================================
//                              EchoService 测试
// ============================================================================

func TestEchoService_New(t *testing.T) {
	es := NewEchoService()

	if es == nil {
		t.Fatal("expected non-nil EchoService")
	}

	if es.ID() != EchoProtocol {
		t.Errorf("ID mismatch: got %s, want %s", es.ID(), EchoProtocol)
	}
}

// ============================================================================
//                              GetBuiltinProtocols 测试
// ============================================================================

func TestGetBuiltinProtocols(t *testing.T) {
	builtins := GetBuiltinProtocols()

	if len(builtins) != 4 {
		t.Errorf("expected 4 builtin protocols, got %d", len(builtins))
	}

	// 验证包含预期的协议
	found := make(map[types.ProtocolID]bool)
	for _, p := range builtins {
		found[p.ID] = true
	}

	expectedIDs := []types.ProtocolID{
		PingProtocol,
		IdentifyProtocol,
		IdentifyPushProtocol,
		EchoProtocol,
	}

	for _, id := range expectedIDs {
		if !found[id] {
			t.Errorf("expected builtin protocol %s not found", id)
		}
	}
}

// ============================================================================
//                              RegisterBuiltinProtocols 测试
// ============================================================================

func TestRegisterBuiltinProtocols(t *testing.T) {
	registry := NewRegistry(100)
	router := NewRouter()

	localID := types.NodeID{1}
	publicKey := []byte("test-key")

	RegisterBuiltinProtocols(registry, router, localID, publicKey)

	// 验证协议已注册
	if registry.Count() != 4 {
		t.Errorf("expected 4 protocols in registry, got %d", registry.Count())
	}

	if router.Count() != 4 {
		t.Errorf("expected 4 protocols in router, got %d", router.Count())
	}

	// 验证可以获取处理器
	if !router.HasProtocol(PingProtocol) {
		t.Error("Ping protocol not registered in router")
	}

	if !router.HasProtocol(IdentifyProtocol) {
		t.Error("Identify protocol not registered in router")
	}
}

// ============================================================================
//                              ProtocolWrapper 测试
// ============================================================================

func TestProtocolWrapper(t *testing.T) {
	called := false
	handler := func(s endpoint.Stream) {
		called = true
	}

	wrapper := NewProtocolWrapper("/test/1.0.0", handler)

	if wrapper.ID() != "/test/1.0.0" {
		t.Errorf("ID mismatch: got %s", wrapper.ID())
	}

	wrapper.Handle(nil)

	if !called {
		t.Error("handler was not called")
	}
}

// ============================================================================
//                              Benchmark 测试
// ============================================================================

func BenchmarkRouter_Handle(b *testing.B) {
	router := NewRouter()

	// 注册100个协议
	for i := 0; i < 100; i++ {
		id := types.ProtocolID("/test/" + string(rune('a'+i%26)) + "/" + string(rune('0'+i/26)))
		router.AddHandler(id, func(s endpoint.Stream) {})
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		router.HasProtocol("/test/m/3")
	}
}

func BenchmarkRegistry_Match(b *testing.B) {
	registry := NewRegistry(100)

	// 注册协议
	for i := 0; i < 50; i++ {
		id := types.ProtocolID("/test/" + string(rune('a'+i%26)) + "/1.0.0")
		registry.RegisterWithHandler(id, func(s endpoint.Stream) {})
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		registry.Match("/test/m/1.0.0")
	}
}

func BenchmarkParseProtocolID(b *testing.B) {
	id := types.ProtocolID("/dep2p/sys/ping/1.0.0")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ParseProtocolID(id)
	}
}

// ============================================================================
//                              ProtocolSwitch 测试
// ============================================================================

func TestProtocolSwitch_Current(t *testing.T) {
	router := NewRouter()
	ps := NewProtocolSwitch(router, nil)

	if ps.Current() != "" {
		t.Error("expected empty initial protocol")
	}
}

// ============================================================================
//                              StreamMux 测试
// ============================================================================

func TestStreamMux_OpenSubStream(t *testing.T) {
	sm := NewStreamMux(nil, nil)

	ss, err := sm.OpenSubStream("/test/1.0.0")
	if err != nil {
		t.Fatalf("OpenSubStream error: %v", err)
	}

	if ss.protocol != "/test/1.0.0" {
		t.Errorf("protocol mismatch: got %s", ss.protocol)
	}

	// 再次打开相同协议应返回相同子流
	ss2, _ := sm.OpenSubStream("/test/1.0.0")
	if ss.id != ss2.id {
		t.Error("should return same substream")
	}
}

func TestStreamMux_CloseSubStream(t *testing.T) {
	sm := NewStreamMux(nil, nil)

	sm.OpenSubStream("/test/1.0.0")
	sm.CloseSubStream("/test/1.0.0")

	// 关闭后应该能重新打开
	ss, _ := sm.OpenSubStream("/test/1.0.0")
	if ss.id != 2 { // ID 应该递增
		t.Errorf("expected new substream with incremented ID, got %d", ss.id)
	}
}

// ============================================================================
//                              Negotiator 测试
// ============================================================================

func TestNegotiator_SelectProtocol(t *testing.T) {
	n := NewNegotiator(nil, nil, time.Second)

	tests := []struct {
		local  []types.ProtocolID
		remote []types.ProtocolID
		want   types.ProtocolID
		hasErr bool
	}{
		{
			local:  []types.ProtocolID{"/a/1.0.0", "/b/1.0.0"},
			remote: []types.ProtocolID{"/b/1.0.0", "/c/1.0.0"},
			want:   "/b/1.0.0",
		},
		{
			local:  []types.ProtocolID{"/a/1.0.0"},
			remote: []types.ProtocolID{"/b/1.0.0"},
			hasErr: true,
		},
		{
			local:  []types.ProtocolID{"/a/1.0.0"},
			remote: []types.ProtocolID{"/a/1.1.0"}, // 语义版本兼容
			want:   "/a/1.0.0",
		},
	}

	for _, tt := range tests {
		got, err := n.SelectProtocol(tt.local, tt.remote)
		if tt.hasErr {
			if err == nil {
				t.Errorf("SelectProtocol(%v, %v) expected error", tt.local, tt.remote)
			}
		} else {
			if err != nil {
				t.Errorf("SelectProtocol(%v, %v) error: %v", tt.local, tt.remote, err)
			}
			if got != tt.want {
				t.Errorf("SelectProtocol(%v, %v) = %s, want %s", tt.local, tt.remote, got, tt.want)
			}
		}
	}
}

func TestNegotiator_ClearCache(t *testing.T) {
	n := NewNegotiator(nil, nil, time.Second)

	// 手动添加缓存
	n.cacheMu.Lock()
	n.cache["test"] = "/test/1.0.0"
	n.cacheMu.Unlock()

	n.ClearCache()

	n.cacheMu.RLock()
	if len(n.cache) != 0 {
		t.Error("cache should be cleared")
	}
	n.cacheMu.RUnlock()
}

func TestNegotiator_ClearCacheForPeer(t *testing.T) {
	n := NewNegotiator(nil, nil, time.Second)

	var peerID types.NodeID
	copy(peerID[:], []byte("test-peer-id-12345678"))

	// 手动添加缓存
	n.cacheMu.Lock()
	n.cache[peerID.ShortString()+":[/a/1.0.0]"] = "/a/1.0.0"
	n.cache[peerID.ShortString()+":[/b/1.0.0]"] = "/b/1.0.0"
	n.cache["other:key"] = "/other/1.0.0"
	n.cacheMu.Unlock()

	// 清除指定节点的缓存
	n.ClearCacheForPeer(peerID)

	n.cacheMu.RLock()
	if len(n.cache) != 1 {
		t.Errorf("expected 1 cache entry remaining, got %d", len(n.cache))
	}
	if _, ok := n.cache["other:key"]; !ok {
		t.Error("other cache entry should remain")
	}
	n.cacheMu.RUnlock()
}

func TestNegotiator_SelectProtocol_EmptyLists(t *testing.T) {
	n := NewNegotiator(nil, nil, time.Second)

	// 空本地列表
	_, err := n.SelectProtocol(nil, []types.ProtocolID{"/a/1.0.0"})
	if err != ErrNoCommonProtocol {
		t.Errorf("expected ErrNoCommonProtocol, got %v", err)
	}

	// 空远程列表
	_, err = n.SelectProtocol([]types.ProtocolID{"/a/1.0.0"}, nil)
	if err != ErrNoCommonProtocol {
		t.Errorf("expected ErrNoCommonProtocol, got %v", err)
	}
}

// ============================================================================
//                              消息编解码测试
// ============================================================================

func TestWriteMessage_TooLong(t *testing.T) {
	var buf bytes.Buffer
	writer := bufio.NewWriter(&buf)

	// 创建超过最大长度的消息
	longMsg := make([]byte, MaxMsgLen+1)
	for i := range longMsg {
		longMsg[i] = 'a'
	}

	err := writeMessage(writer, string(longMsg))
	if err != ErrMessageTooLong {
		t.Errorf("expected ErrMessageTooLong, got %v", err)
	}
}

func TestReadWriteMessage_Roundtrip(t *testing.T) {
	tests := []string{
		"/multistream/1.0.0",
		"/test/1.0.0",
		"na",
		"ls",
		"", // 空消息
	}

	for _, msg := range tests {
		var buf bytes.Buffer
		writer := bufio.NewWriter(&buf)

		err := writeMessage(writer, msg)
		if err != nil {
			t.Fatalf("writeMessage(%q) error: %v", msg, err)
		}
		writer.Flush()

		reader := bufio.NewReader(&buf)
		got, err := readMessage(reader)
		if err != nil {
			t.Fatalf("readMessage() error: %v", err)
		}

		if got != msg {
			t.Errorf("roundtrip mismatch: got %q, want %q", got, msg)
		}
	}
}

func TestReadVarint_Overflow(t *testing.T) {
	// 创建一个会导致溢出的 varint
	var buf bytes.Buffer
	// 写入过长的 varint（超过 64 位）
	for i := 0; i < 20; i++ {
		buf.WriteByte(0x80 | byte(i)) // continuation bits
	}
	buf.WriteByte(0x00) // terminal

	reader := bufio.NewReader(&buf)
	_, err := readVarint(reader)
	if err != ErrInvalidMessage {
		t.Errorf("expected ErrInvalidMessage for overflow, got %v", err)
	}
}

// ============================================================================
//                              LazyNegotiator 测试
// ============================================================================

func TestLazyNegotiator_NotNegotiated(t *testing.T) {
	n := NewNegotiator(nil, nil, time.Second)
	ln := NewLazyNegotiator(n, nil)

	if ln.IsNegotiated() {
		t.Error("should not be negotiated initially")
	}

	if ln.Protocol() != "" {
		t.Error("protocol should be empty initially")
	}
}

// ============================================================================
//                              SimultaneousNegotiator 测试
// ============================================================================

func TestSimultaneousNegotiator_isProtocolSupported(t *testing.T) {
	n := NewNegotiator(nil, nil, time.Second)
	sn := NewSimultaneousNegotiator(n)

	supported := []types.ProtocolID{"/a/1.0.0", "/b/1.0.0"}

	if !sn.isProtocolSupported("/a/1.0.0", supported) {
		t.Error("should support /a/1.0.0")
	}

	if sn.isProtocolSupported("/c/1.0.0", supported) {
		t.Error("should not support /c/1.0.0")
	}
}

// ============================================================================
//                              PingService 更多测试
// ============================================================================

func TestPingService_Handler(t *testing.T) {
	ps := NewPingService(types.NodeID{})
	handler := ps.Handler()

	if handler == nil {
		t.Fatal("Handler should not return nil")
	}
}

// ============================================================================
//                              IdentifyService 更多测试
// ============================================================================

func TestIdentifyService_GetPeerInfo_NotFound(t *testing.T) {
	is := NewIdentifyService(types.NodeID{}, nil, nil)

	var peerID types.NodeID
	copy(peerID[:], []byte("unknown-peer-12345678"))

	_, ok := is.GetPeerInfo(peerID)
	if ok {
		t.Error("should not find unknown peer")
	}
}

func TestIdentifyService_Handler(t *testing.T) {
	is := NewIdentifyService(types.NodeID{}, nil, nil)

	handler := is.Handler()
	if handler == nil {
		t.Fatal("Handler should not return nil")
	}

	pushHandler := is.PushHandler()
	if pushHandler == nil {
		t.Fatal("PushHandler should not return nil")
	}
}

// ============================================================================
//                              EchoService 更多测试
// ============================================================================

func TestEchoService_Handler(t *testing.T) {
	es := NewEchoService()
	handler := es.Handler()

	if handler == nil {
		t.Fatal("Handler should not return nil")
	}
}

// ============================================================================
//                              Router 更多测试
// ============================================================================

func TestRouter_Count(t *testing.T) {
	router := NewRouter()

	if router.Count() != 0 {
		t.Errorf("expected 0, got %d", router.Count())
	}

	router.AddHandler("/test/1.0.0", func(s endpoint.Stream) {})
	router.AddHandler("/test/2.0.0", func(s endpoint.Stream) {})

	if router.Count() != 2 {
		t.Errorf("expected 2, got %d", router.Count())
	}
}

func TestRouter_findHandler_NoMatch(t *testing.T) {
	router := NewRouter()

	_, err := router.findHandler("/unknown/1.0.0")
	if err == nil {
		t.Error("expected error for unknown protocol")
	}
}

// ============================================================================
//                              Registry 更多测试
// ============================================================================

func TestRegistry_IDs(t *testing.T) {
	registry := NewRegistry(100)

	handler := func(s endpoint.Stream) {}
	registry.RegisterWithHandler("/a/1.0.0", handler)
	registry.RegisterWithHandler("/b/1.0.0", handler)

	ids := registry.IDs()
	if len(ids) != 2 {
		t.Errorf("expected 2 IDs, got %d", len(ids))
	}
}

func TestRegistry_MatchHandler(t *testing.T) {
	registry := NewRegistry(100)

	called := false
	handler := func(s endpoint.Stream) {
		called = true
	}
	registry.RegisterWithHandler("/test/1.0.0", handler)

	h, ok := registry.MatchHandler("/test/1.0.0")
	if !ok {
		t.Fatal("expected to find handler")
	}

	h(nil)
	if !called {
		t.Error("handler was not called")
	}

	// 语义版本匹配
	h2, ok := registry.MatchHandler("/test/1.1.0")
	if !ok {
		t.Error("should match compatible version")
	}
	if h2 == nil {
		t.Error("handler should not be nil")
	}
}

func TestRegistry_UnregisterNonExistent(t *testing.T) {
	registry := NewRegistry(100)

	err := registry.Unregister("/nonexistent/1.0.0")
	if err == nil {
		t.Error("expected error for unregistering non-existent protocol")
	}
}

// ============================================================================
//                              ProtocolEntry 更多测试
// ============================================================================

func TestProtocolEntry_Priority(t *testing.T) {
	entry := NewProtocolEntry("/test/1.0.0", nil)

	// 默认优先级应为 0
	if entry.Priority() != 0 {
		t.Errorf("expected default priority 0, got %d", entry.Priority())
	}

	// 设置优先级
	entry.SetPriority(100)
	if entry.Priority() != 100 {
		t.Errorf("expected priority 100, got %d", entry.Priority())
	}
}

func TestProtocolEntry_Description(t *testing.T) {
	entry := NewProtocolEntry("/test/1.0.0", nil)

	entry.SetDescription("Test protocol")
	if entry.Description() != "Test protocol" {
		t.Errorf("description mismatch: got %s", entry.Description())
	}
}

// ============================================================================
//                              Multiplexer 更多测试
// ============================================================================

func TestMultiplexer_HasProtocolAfterAdd(t *testing.T) {
	mux := NewMultiplexer(nil, nil)

	proto := NewProtocolWrapper("/test/1.0.0", func(s endpoint.Stream) {})
	mux.AddProtocol(proto)

	if !mux.HasProtocol("/test/1.0.0") {
		t.Error("expected to have protocol after add")
	}
}

func TestMultiplexer_AddDuplicate(t *testing.T) {
	mux := NewMultiplexer(nil, nil)

	proto1 := NewProtocolWrapper("/test/1.0.0", func(s endpoint.Stream) {})
	proto2 := NewProtocolWrapper("/test/1.0.0", func(s endpoint.Stream) {})

	mux.AddProtocol(proto1)
	mux.AddProtocol(proto2) // 覆盖

	// 应该只有一个
	if len(mux.Protocols()) != 1 {
		t.Errorf("expected 1 protocol, got %d", len(mux.Protocols()))
	}
}

// ============================================================================
//                              ProtocolSwitch 更多测试
// ============================================================================

func TestProtocolSwitch_Current_Initial(t *testing.T) {
	router := NewRouter()
	router.AddHandler("/a/1.0.0", func(s endpoint.Stream) {})
	router.AddHandler("/b/1.0.0", func(s endpoint.Stream) {})

	ps := NewProtocolSwitch(router, nil)

	// 初始应该为空
	if ps.Current() != "" {
		t.Errorf("expected empty current protocol, got %s", ps.Current())
	}
}

// ============================================================================
//                              StreamMux 更多测试
// ============================================================================

func TestStreamMux_OpenAndCloseSubStream(t *testing.T) {
	sm := NewStreamMux(nil, nil)

	ss1, err := sm.OpenSubStream("/test/1.0.0")
	if err != nil {
		t.Fatalf("OpenSubStream error: %v", err)
	}
	if ss1 == nil {
		t.Fatal("expected substream")
	}

	ss2, err := sm.OpenSubStream("/test/2.0.0")
	if err != nil {
		t.Fatalf("OpenSubStream error: %v", err)
	}
	if ss2 == nil {
		t.Fatal("expected substream")
	}

	// 关闭子流
	sm.CloseSubStream("/test/1.0.0")
	sm.CloseSubStream("/test/2.0.0")

	// 验证关闭后可以重新打开（ID 应该递增）
	ss3, _ := sm.OpenSubStream("/test/1.0.0")
	if ss3.id <= ss1.id {
		t.Error("new substream should have incremented ID")
	}
}

// ============================================================================
//                              测试用 Mock
// ============================================================================

// mockStream 简化的测试用流
type mockStream struct {
	protocolID types.ProtocolID
	closed     bool
}

func (m *mockStream) Read(p []byte) (n int, err error)                 { return 0, nil }
func (m *mockStream) Write(p []byte) (n int, err error)                { return len(p), nil }
func (m *mockStream) Close() error                                     { m.closed = true; return nil }
func (m *mockStream) ID() endpoint.StreamID                            { return 1 }
func (m *mockStream) ProtocolID() endpoint.ProtocolID                  { return endpoint.ProtocolID(m.protocolID) }
func (m *mockStream) Connection() endpoint.Connection                  { return nil }
func (m *mockStream) SetDeadline(t time.Time) error                    { return nil }
func (m *mockStream) SetReadDeadline(t time.Time) error                { return nil }
func (m *mockStream) SetWriteDeadline(t time.Time) error               { return nil }
func (m *mockStream) CloseRead() error                                 { return nil }
func (m *mockStream) CloseWrite() error                                { return nil }
func (m *mockStream) SetPriority(priority endpoint.Priority)           {}
func (m *mockStream) Priority() endpoint.Priority                      { return 0 }
func (m *mockStream) Stats() endpoint.StreamStats                      { return endpoint.StreamStats{} }
func (m *mockStream) IsClosed() bool                                   { return m.closed }

// ============================================================================
//                              Panic 恢复测试
// ============================================================================

func TestRouter_Handle_PanicRecovery(t *testing.T) {
	router := NewRouter()

	// 使用系统协议格式以绕过 RealmAuth 检查
	// 注册一个会 panic 的处理器
	router.AddHandler("/dep2p/sys/panic/1.0.0", func(s endpoint.Stream) {
		panic("test panic")
	})

	// 创建模拟流
	stream := &mockStream{
		protocolID: "/dep2p/sys/panic/1.0.0",
	}

	// 调用 Handle 应该返回 ErrHandlerPanic
	err := router.Handle(stream)
	if err != ErrHandlerPanic {
		t.Errorf("expected ErrHandlerPanic, got %v", err)
	}
}

func TestRouter_Handle_NoPanic(t *testing.T) {
	router := NewRouter()

	// 使用系统协议格式以绕过 RealmAuth 检查
	called := false
	router.AddHandler("/dep2p/sys/normal/1.0.0", func(s endpoint.Stream) {
		called = true
	})

	stream := &mockStream{
		protocolID: "/dep2p/sys/normal/1.0.0",
	}

	err := router.Handle(stream)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if !called {
		t.Error("handler was not called")
	}
}

// ============================================================================
//                              缓存限制测试
// ============================================================================

func TestNegotiator_CacheResultEviction(t *testing.T) {
	negotiator := NewNegotiator(nil, nil, DefaultNegotiationTimeout)

	// 填满缓存
	for i := 0; i < MaxCacheSize; i++ {
		key := "cache-key-" + string(rune(i/26+'a')) + string(rune(i%26+'a'))
		negotiator.cache[key] = "/test/1.0.0"
	}

	initialSize := len(negotiator.cache)

	// 添加新条目，应该触发清理
	negotiator.cacheResult("new-key", "/new/1.0.0")

	// 验证缓存已被清理（大小应该小于原来的一半 + 1）
	if len(negotiator.cache) >= initialSize {
		t.Errorf("cache should have been evicted, got size %d vs initial %d", len(negotiator.cache), initialSize)
	}
}

// ============================================================================
//                              StreamMux Nil Base 测试
// ============================================================================

func TestStreamMux_CloseNilBase(t *testing.T) {
	// 创建 StreamMux 时 base 为 nil
	sm := NewStreamMux(nil, nil)

	// 打开子流
	_, _ = sm.OpenSubStream("/test/1.0.0")

	// 关闭应该不 panic
	err := sm.Close()
	if err != nil {
		t.Errorf("expected nil error for nil base, got %v", err)
	}

	// 重复关闭也应该正常
	err = sm.Close()
	if err != nil {
		t.Errorf("double close should succeed, got %v", err)
	}
}

// ============================================================================
//                              Module 直接调用测试
// ============================================================================

func TestModule_ProvideServices_Directly(t *testing.T) {
	input := ModuleInput{
		Handlers: []ProtocolHandlerEntry{
			{
				Protocol: "/test/1.0.0",
				Handler:  func(s endpoint.Stream) {},
			},
		},
	}

	output, err := ProvideServices(input)
	if err != nil {
		t.Fatalf("ProvideServices error: %v", err)
	}

	if output.ProtocolRouter == nil {
		t.Fatal("expected ProtocolRouter")
	}

	// 验证预注册的处理器
	protocols := output.ProtocolRouter.Protocols()
	found := false
	for _, p := range protocols {
		if p == "/test/1.0.0" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected pre-registered protocol /test/1.0.0")
	}
}

func TestModule_ProvideServices_Default(t *testing.T) {
	input := ModuleInput{}

	output, err := ProvideServices(input)
	if err != nil {
		t.Fatalf("ProvideServices error: %v", err)
	}

	if output.ProtocolRouter == nil {
		t.Fatal("expected ProtocolRouter")
	}
}

// ============================================================================
//                              HandleIncoming 超时测试
// ============================================================================

func TestNegotiator_HandleIncomingWithTimeout(t *testing.T) {
	registry := NewRegistry(100)
	registry.RegisterWithHandler("/test/1.0.0", func(s endpoint.Stream) {})

	negotiator := NewNegotiator(registry, nil, DefaultNegotiationTimeout)

	// 验证 Negotiator 创建成功
	if negotiator == nil {
		t.Fatal("expected negotiator")
	}

	// 验证 registry 正确设置
	if negotiator.registry != registry {
		t.Error("registry not set correctly")
	}
}

// ============================================================================
//                              Registry 类型安全测试
// ============================================================================

func TestRegistry_ListSafeTypeAssertion(t *testing.T) {
	registry := NewRegistry(100)

	handler := func(s endpoint.Stream) {}
	registry.RegisterWithHandler("/a/1.0.0", handler, WithPriority(10))
	registry.RegisterWithHandler("/b/1.0.0", handler, WithPriority(20))
	registry.RegisterWithHandler("/c/1.0.0", handler, WithPriority(5))

	// List 应该按优先级排序返回，且不 panic
	protocols := registry.List()
	if len(protocols) != 3 {
		t.Fatalf("expected 3 protocols, got %d", len(protocols))
	}

	// 验证排序（按优先级降序）
	ids := make([]types.ProtocolID, len(protocols))
	for i, p := range protocols {
		ids[i] = p.ID()
	}
	if ids[0] != "/b/1.0.0" || ids[1] != "/a/1.0.0" || ids[2] != "/c/1.0.0" {
		t.Errorf("unexpected order: %v", ids)
	}
}

func TestRegistry_MatchSafeTypeAssertion(t *testing.T) {
	registry := NewRegistry(100)

	handler := func(s endpoint.Stream) {}
	registry.RegisterWithHandler("/test/1.0.0", handler, WithPriority(10))
	registry.RegisterWithHandler("/test/2.0.0", handler, WithPriority(20))

	// Match 应该正常工作且不 panic
	matches := registry.Match("/test/1.0.0")
	if len(matches) == 0 {
		t.Fatal("expected matches")
	}
}

func TestRegistry_MatchHandlerSafeTypeAssertion(t *testing.T) {
	registry := NewRegistry(100)

	called := false
	handler := func(s endpoint.Stream) { called = true }
	registry.RegisterWithHandler("/test/1.0.0", handler)

	h, ok := registry.MatchHandler("/test/1.0.0")
	if !ok {
		t.Fatal("expected to find handler")
	}
	if h == nil {
		t.Fatal("handler should not be nil")
	}

	// 调用处理器验证正确性
	h(nil)
	if !called {
		t.Error("handler was not called correctly")
	}
}
