package protocol

// Error describes a protocol-level failure (turn or item).
//
// This is embedded inside Turn/Item payloads and is distinct from JSON-RPC RPCError.
type Error struct {
	Code    int    `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
}
