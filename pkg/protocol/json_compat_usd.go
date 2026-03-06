package protocol

import "github.com/tinoosan/agen8/pkg/protocol/jsoncompat"

// UnmarshalJSON provides backwards-compatible decoding for historical JSON keys.
//
// Agen8 previously used "...Usd" keys (e.g. "costUsd") and now standardizes on
// "...USD" (e.g. "costUSD"). This unmarshaller accepts both spellings so older
// persisted JSON and artifacts continue to load.
func (t *Thread) UnmarshalJSON(data []byte) error {
	type threadAlias Thread
	var alias threadAlias
	if err := jsoncompat.UnmarshalFloat64Aliases(data, &alias, jsoncompat.Float64Alias{
		Field:  "CostUSD",
		NewKey: "costUSD",
		OldKey: "costUsd",
	}); err != nil {
		return err
	}
	*t = Thread(alias)
	return nil
}
