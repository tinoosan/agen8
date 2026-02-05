package protocol

import "encoding/json"

// UnmarshalJSON provides backwards-compatible decoding for historical JSON keys.
//
// Workbench previously used "...Usd" keys (e.g. "costUsd") and now standardizes on
// "...USD" (e.g. "costUSD"). This unmarshaller accepts both spellings so older
// persisted JSON and artifacts continue to load.
func (t *Thread) UnmarshalJSON(data []byte) error {
	type threadAlias Thread
	type threadWire struct {
		threadAlias

		CostUSD float64  `json:"-"`
		CostNew *float64 `json:"costUSD,omitempty"`
		CostOld *float64 `json:"costUsd,omitempty"`
	}
	var w threadWire
	if err := json.Unmarshal(data, &w); err != nil {
		return err
	}
	*t = Thread(w.threadAlias)
	switch {
	case w.CostNew != nil:
		t.CostUSD = *w.CostNew
	case w.CostOld != nil:
		t.CostUSD = *w.CostOld
	}
	return nil
}
