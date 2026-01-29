package addressbook_test

import (
	"testing"
	"time"

	"github.com/dep2p/go-dep2p/pkg/lib/proto/addressbook"
	"google.golang.org/protobuf/proto"
)

func TestMemberEntry(t *testing.T) {
	entry := &addressbook.MemberEntry{
		NodeId:       []byte("12D3KooWTestNodeId"),
		Addrs:        [][]byte{[]byte("/ip4/192.168.1.1/tcp/4001")},
		NatType:      addressbook.NATType_NAT_TYPE_FULL_CONE,
		Capabilities: []string{"relay", "dht"},
		Online:       true,
		LastSeen:     time.Now().Unix(),
		LastUpdate:   time.Now().Unix(),
	}

	data, err := proto.Marshal(entry)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var decoded addressbook.MemberEntry
	err = proto.Unmarshal(data, &decoded)
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if string(decoded.NodeId) != "12D3KooWTestNodeId" {
		t.Errorf("NodeId = %s, want 12D3KooWTestNodeId", decoded.NodeId)
	}

	if decoded.NatType != addressbook.NATType_NAT_TYPE_FULL_CONE {
		t.Errorf("NatType = %v, want NAT_TYPE_FULL_CONE", decoded.NatType)
	}

	if !decoded.Online {
		t.Error("Online should be true")
	}
}

func TestAddressRegister(t *testing.T) {
	register := &addressbook.AddressRegister{
		NodeId:       []byte("12D3KooWTestNodeId"),
		Addrs:        [][]byte{[]byte("/ip4/1.2.3.4/tcp/4001")},
		NatType:      addressbook.NATType_NAT_TYPE_NONE,
		Capabilities: []string{"relay"},
		Signature:    []byte("test-signature"),
		Timestamp:    time.Now().Unix(),
	}

	data, err := proto.Marshal(register)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var decoded addressbook.AddressRegister
	err = proto.Unmarshal(data, &decoded)
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if !proto.Equal(register, &decoded) {
		t.Error("Round trip failed")
	}
}

func TestAddressRegisterResponse(t *testing.T) {
	response := &addressbook.AddressRegisterResponse{
		Success: true,
		Error:   "",
		Ttl:     300,
	}

	data, err := proto.Marshal(response)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var decoded addressbook.AddressRegisterResponse
	err = proto.Unmarshal(data, &decoded)
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if !decoded.Success {
		t.Error("Success should be true")
	}

	if decoded.Ttl != 300 {
		t.Errorf("Ttl = %d, want 300", decoded.Ttl)
	}
}

func TestAddressQuery(t *testing.T) {
	query := &addressbook.AddressQuery{
		TargetNodeId: []byte("target-node-id"),
		RequestorId:  []byte("requestor-node-id"),
	}

	data, err := proto.Marshal(query)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var decoded addressbook.AddressQuery
	err = proto.Unmarshal(data, &decoded)
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if string(decoded.TargetNodeId) != "target-node-id" {
		t.Errorf("TargetNodeId = %s, want target-node-id", decoded.TargetNodeId)
	}
}

func TestAddressResponse(t *testing.T) {
	response := &addressbook.AddressResponse{
		Found: true,
		Entry: &addressbook.MemberEntry{
			NodeId:  []byte("found-node-id"),
			Addrs:   [][]byte{[]byte("/ip4/10.0.0.1/tcp/4001")},
			NatType: addressbook.NATType_NAT_TYPE_RESTRICTED,
			Online:  true,
		},
		Error: "",
	}

	data, err := proto.Marshal(response)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var decoded addressbook.AddressResponse
	err = proto.Unmarshal(data, &decoded)
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if !decoded.Found {
		t.Error("Found should be true")
	}

	if decoded.Entry == nil {
		t.Fatal("Entry is nil")
	}

	if string(decoded.Entry.NodeId) != "found-node-id" {
		t.Errorf("Entry.NodeId = %s, want found-node-id", decoded.Entry.NodeId)
	}
}

func TestAddressUpdate(t *testing.T) {
	update := &addressbook.AddressUpdate{
		NodeId:     []byte("updated-node-id"),
		NewAddrs:   [][]byte{[]byte("/ip4/2.3.4.5/tcp/4001")},
		OldAddrs:   [][]byte{[]byte("/ip4/1.2.3.4/tcp/4001")},
		UpdateType: addressbook.UpdateType_UPDATE_TYPE_REPLACE,
		Timestamp:  time.Now().Unix(),
		Signature:  []byte("relay-signature"),
	}

	data, err := proto.Marshal(update)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var decoded addressbook.AddressUpdate
	err = proto.Unmarshal(data, &decoded)
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if decoded.UpdateType != addressbook.UpdateType_UPDATE_TYPE_REPLACE {
		t.Errorf("UpdateType = %v, want UPDATE_TYPE_REPLACE", decoded.UpdateType)
	}
}

func TestBatchAddressQuery(t *testing.T) {
	query := &addressbook.BatchAddressQuery{
		TargetNodeIds: [][]byte{
			[]byte("node-1"),
			[]byte("node-2"),
			[]byte("node-3"),
		},
		RequestorId: []byte("requestor"),
	}

	data, err := proto.Marshal(query)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var decoded addressbook.BatchAddressQuery
	err = proto.Unmarshal(data, &decoded)
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if len(decoded.TargetNodeIds) != 3 {
		t.Errorf("TargetNodeIds length = %d, want 3", len(decoded.TargetNodeIds))
	}
}

func TestAddressBookMessage_Register(t *testing.T) {
	msg := &addressbook.AddressBookMessage{
		Type: addressbook.AddressBookMessage_REGISTER,
		Payload: &addressbook.AddressBookMessage_Register{
			Register: &addressbook.AddressRegister{
				NodeId:    []byte("test-node"),
				Addrs:     [][]byte{[]byte("/ip4/1.2.3.4/tcp/4001")},
				NatType:   addressbook.NATType_NAT_TYPE_NONE,
				Timestamp: time.Now().Unix(),
			},
		},
	}

	data, err := proto.Marshal(msg)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var decoded addressbook.AddressBookMessage
	err = proto.Unmarshal(data, &decoded)
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if decoded.Type != addressbook.AddressBookMessage_REGISTER {
		t.Errorf("Type = %v, want REGISTER", decoded.Type)
	}

	register := decoded.GetRegister()
	if register == nil {
		t.Fatal("Register payload is nil")
	}

	if string(register.NodeId) != "test-node" {
		t.Errorf("Register.NodeId = %s, want test-node", register.NodeId)
	}
}

func TestAddressBookMessage_Query(t *testing.T) {
	msg := &addressbook.AddressBookMessage{
		Type: addressbook.AddressBookMessage_QUERY,
		Payload: &addressbook.AddressBookMessage_Query{
			Query: &addressbook.AddressQuery{
				TargetNodeId: []byte("target"),
				RequestorId:  []byte("requestor"),
			},
		},
	}

	data, err := proto.Marshal(msg)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var decoded addressbook.AddressBookMessage
	err = proto.Unmarshal(data, &decoded)
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if decoded.Type != addressbook.AddressBookMessage_QUERY {
		t.Errorf("Type = %v, want QUERY", decoded.Type)
	}

	query := decoded.GetQuery()
	if query == nil {
		t.Fatal("Query payload is nil")
	}
}

func TestNATType_String(t *testing.T) {
	tests := []struct {
		natType addressbook.NATType
		want    string
	}{
		{addressbook.NATType_NAT_TYPE_UNKNOWN, "NAT_TYPE_UNKNOWN"},
		{addressbook.NATType_NAT_TYPE_NONE, "NAT_TYPE_NONE"},
		{addressbook.NATType_NAT_TYPE_FULL_CONE, "NAT_TYPE_FULL_CONE"},
		{addressbook.NATType_NAT_TYPE_RESTRICTED, "NAT_TYPE_RESTRICTED"},
		{addressbook.NATType_NAT_TYPE_PORT_RESTRICTED, "NAT_TYPE_PORT_RESTRICTED"},
		{addressbook.NATType_NAT_TYPE_SYMMETRIC, "NAT_TYPE_SYMMETRIC"},
	}

	for _, tt := range tests {
		if got := tt.natType.String(); got != tt.want {
			t.Errorf("NATType(%d).String() = %q, want %q", tt.natType, got, tt.want)
		}
	}
}

func TestUpdateType_String(t *testing.T) {
	tests := []struct {
		updateType addressbook.UpdateType
		want       string
	}{
		{addressbook.UpdateType_UPDATE_TYPE_UNKNOWN, "UPDATE_TYPE_UNKNOWN"},
		{addressbook.UpdateType_UPDATE_TYPE_ADD, "UPDATE_TYPE_ADD"},
		{addressbook.UpdateType_UPDATE_TYPE_REMOVE, "UPDATE_TYPE_REMOVE"},
		{addressbook.UpdateType_UPDATE_TYPE_REPLACE, "UPDATE_TYPE_REPLACE"},
	}

	for _, tt := range tests {
		if got := tt.updateType.String(); got != tt.want {
			t.Errorf("UpdateType(%d).String() = %q, want %q", tt.updateType, got, tt.want)
		}
	}
}
