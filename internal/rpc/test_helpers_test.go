package rpc

import (
	"encoding/json"
	"testing"
)

func decodeRPCResponse(t *testing.T, raw []byte) Response {
	t.Helper()
	var resp Response
	if err := json.Unmarshal(raw, &resp); err != nil {
		t.Fatalf("decode rpc response: %v\n%s", err, string(raw))
	}
	return resp
}
