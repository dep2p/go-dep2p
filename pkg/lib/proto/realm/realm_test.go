package realm_test

import (
	"testing"

	"github.com/dep2p/go-dep2p/pkg/lib/proto/realm"
	"google.golang.org/protobuf/proto"
)

func TestAuthRequest(t *testing.T) {
	req := &realm.AuthRequest{
		RealmId:           []byte("test-realm-id"),
		PeerId:            []byte("test-peer-id"),
		ChallengeResponse: []byte("challenge-response"),
		Timestamp:         1234567890,
		Nonce:             []byte("random-nonce"),
	}

	data, err := proto.Marshal(req)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var decoded realm.AuthRequest
	err = proto.Unmarshal(data, &decoded)
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if !proto.Equal(req, &decoded) {
		t.Error("Round trip failed")
	}
}

func TestAuthResponse(t *testing.T) {
	resp := &realm.AuthResponse{
		Status:      realm.AuthResponse_OK,
		StatusText:  "Success",
		MemberToken: []byte("token"),
		Expiration:  9999999999,
	}

	data, err := proto.Marshal(resp)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var decoded realm.AuthResponse
	err = proto.Unmarshal(data, &decoded)
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if decoded.Status != realm.AuthResponse_OK {
		t.Errorf("Status = %v, want OK", decoded.Status)
	}
}

func TestMemberList(t *testing.T) {
	list := &realm.MemberList{
		RealmId: []byte("realm-id"),
		Members: []*realm.MemberInfo{
			{
				PeerId:   []byte("peer1"),
				Addrs:    [][]byte{[]byte("/ip4/1.2.3.4/tcp/4001")},
				JoinedAt: 1234567890,
				Metadata: map[string]string{"role": "validator"},
			},
		},
		Version: 1,
	}

	data, err := proto.Marshal(list)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var decoded realm.MemberList
	err = proto.Unmarshal(data, &decoded)
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if len(decoded.Members) != 1 {
		t.Errorf("Members count = %d, want 1", len(decoded.Members))
	}
}
