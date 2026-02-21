package types

import "encoding/json"

// UnmarshalJSON provides backwards-compatible decoding for historical JSON keys.
//
// Agen8 previously used "...Usd" keys (e.g. "costUsd") and now standardizes on
// "...USD" (e.g. "costUSD"). These unmarshallers accept both spellings so older
// persisted SQLite JSON and artifacts continue to load.

func (s *Session) UnmarshalJSON(data []byte) error {
	type sessionAlias Session
	type sessionWire struct {
		sessionAlias

		CostUSD float64  `json:"-"`
		CostNew *float64 `json:"costUSD,omitempty"`
		CostOld *float64 `json:"costUsd,omitempty"`
	}
	var w sessionWire
	if err := json.Unmarshal(data, &w); err != nil {
		return err
	}
	*s = Session(w.sessionAlias)
	switch {
	case w.CostNew != nil:
		s.CostUSD = *w.CostNew
	case w.CostOld != nil:
		s.CostUSD = *w.CostOld
	}
	return nil
}

func (r *Run) UnmarshalJSON(data []byte) error {
	type runAlias Run
	type runWire struct {
		runAlias

		CostUSD float64  `json:"-"`
		CostNew *float64 `json:"costUSD,omitempty"`
		CostOld *float64 `json:"costUsd,omitempty"`
	}
	var w runWire
	if err := json.Unmarshal(data, &w); err != nil {
		return err
	}
	*r = Run(w.runAlias)
	switch {
	case w.CostNew != nil:
		r.CostUSD = *w.CostNew
	case w.CostOld != nil:
		r.CostUSD = *w.CostOld
	}
	return nil
}

func (c *RunRuntimeConfig) UnmarshalJSON(data []byte) error {
	type cfgAlias RunRuntimeConfig
	type cfgWire struct {
		cfgAlias

		PriceInPerMTokensUSD  float64  `json:"-"`
		PriceOutPerMTokensUSD float64  `json:"-"`
		PriceInNew            *float64 `json:"priceInPerMTokensUSD,omitempty"`
		PriceInOld            *float64 `json:"priceInPerMTokensUsd,omitempty"`
		PriceOutNew           *float64 `json:"priceOutPerMTokensUSD,omitempty"`
		PriceOutOld           *float64 `json:"priceOutPerMTokensUsd,omitempty"`
	}
	var w cfgWire
	if err := json.Unmarshal(data, &w); err != nil {
		return err
	}
	*c = RunRuntimeConfig(w.cfgAlias)
	switch {
	case w.PriceInNew != nil:
		c.PriceInPerMTokensUSD = *w.PriceInNew
	case w.PriceInOld != nil:
		c.PriceInPerMTokensUSD = *w.PriceInOld
	}
	switch {
	case w.PriceOutNew != nil:
		c.PriceOutPerMTokensUSD = *w.PriceOutNew
	case w.PriceOutOld != nil:
		c.PriceOutPerMTokensUSD = *w.PriceOutOld
	}
	return nil
}

func (t *Task) UnmarshalJSON(data []byte) error {
	type taskAlias Task
	type taskWire struct {
		taskAlias

		CostUSD float64  `json:"-"`
		CostNew *float64 `json:"costUSD,omitempty"`
		CostOld *float64 `json:"costUsd,omitempty"`
	}
	var w taskWire
	if err := json.Unmarshal(data, &w); err != nil {
		return err
	}
	*t = Task(w.taskAlias)
	switch {
	case w.CostNew != nil:
		t.CostUSD = *w.CostNew
	case w.CostOld != nil:
		t.CostUSD = *w.CostOld
	}
	return nil
}

func (tr *TaskResult) UnmarshalJSON(data []byte) error {
	type resultAlias TaskResult
	type resultWire struct {
		resultAlias

		CostUSD float64  `json:"-"`
		CostNew *float64 `json:"costUSD,omitempty"`
		CostOld *float64 `json:"costUsd,omitempty"`
	}
	var w resultWire
	if err := json.Unmarshal(data, &w); err != nil {
		return err
	}
	*tr = TaskResult(w.resultAlias)
	switch {
	case w.CostNew != nil:
		tr.CostUSD = *w.CostNew
	case w.CostOld != nil:
		tr.CostUSD = *w.CostOld
	}
	return nil
}
