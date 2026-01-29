package common_test

import (
	"testing"

	"github.com/dep2p/go-dep2p/pkg/lib/proto/common"
	"google.golang.org/protobuf/proto"
)

func TestTimestamp(t *testing.T) {
	ts := &common.Timestamp{
		Seconds: 1234567890,
		Nanos:   123456789,
	}

	data, err := proto.Marshal(ts)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var decoded common.Timestamp
	err = proto.Unmarshal(data, &decoded)
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if !proto.Equal(ts, &decoded) {
		t.Error("Round trip failed")
	}
}

func TestError(t *testing.T) {
	err := &common.Error{
		Code:    500,
		Message: "Internal Error",
		Details: map[string]string{
			"component": "dht",
			"operation": "find_node",
		},
	}

	data, marshalErr := proto.Marshal(err)
	if marshalErr != nil {
		t.Fatalf("Marshal failed: %v", marshalErr)
	}

	var decoded common.Error
	unmarshalErr := proto.Unmarshal(data, &decoded)
	if unmarshalErr != nil {
		t.Fatalf("Unmarshal failed: %v", unmarshalErr)
	}

	if !proto.Equal(err, &decoded) {
		t.Error("Round trip failed")
	}
}

func TestResult(t *testing.T) {
	result := &common.Result{
		Success: true,
		Error:   nil,
	}

	data, err := proto.Marshal(result)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var decoded common.Result
	err = proto.Unmarshal(data, &decoded)
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if decoded.Success != true {
		t.Error("Success should be true")
	}
}

func TestVersion(t *testing.T) {
	v := &common.Version{
		Major: 1,
		Minor: 2,
		Patch: 3,
	}

	data, err := proto.Marshal(v)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var decoded common.Version
	err = proto.Unmarshal(data, &decoded)
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if !proto.Equal(v, &decoded) {
		t.Error("Round trip failed")
	}
}
