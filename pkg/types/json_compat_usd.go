package types

import "github.com/tinoosan/agen8/pkg/protocol/jsoncompat"

// UnmarshalJSON provides backwards-compatible decoding for historical JSON keys.
//
// Agen8 previously used "...Usd" keys (e.g. "costUsd") and now standardizes on
// "...USD" (e.g. "costUSD"). These unmarshallers accept both spellings so older
// persisted SQLite JSON and artifacts continue to load.

func (s *Session) UnmarshalJSON(data []byte) error {
	type sessionAlias Session
	var alias sessionAlias
	if err := jsoncompat.UnmarshalFloat64Aliases(data, &alias, jsoncompat.Float64Alias{
		Field:  "CostUSD",
		NewKey: "costUSD",
		OldKey: "costUsd",
	}); err != nil {
		return err
	}
	*s = Session(alias)
	return nil
}

func (r *Run) UnmarshalJSON(data []byte) error {
	type runAlias Run
	var alias runAlias
	if err := jsoncompat.UnmarshalFloat64Aliases(data, &alias, jsoncompat.Float64Alias{
		Field:  "CostUSD",
		NewKey: "costUSD",
		OldKey: "costUsd",
	}); err != nil {
		return err
	}
	*r = Run(alias)
	return nil
}

func (c *RunRuntimeConfig) UnmarshalJSON(data []byte) error {
	type cfgAlias RunRuntimeConfig
	var alias cfgAlias
	if err := jsoncompat.UnmarshalFloat64Aliases(
		data,
		&alias,
		jsoncompat.Float64Alias{
			Field:  "PriceInPerMTokensUSD",
			NewKey: "priceInPerMTokensUSD",
			OldKey: "priceInPerMTokensUsd",
		},
		jsoncompat.Float64Alias{
			Field:  "PriceOutPerMTokensUSD",
			NewKey: "priceOutPerMTokensUSD",
			OldKey: "priceOutPerMTokensUsd",
		},
	); err != nil {
		return err
	}
	*c = RunRuntimeConfig(alias)
	return nil
}

func (t *Task) UnmarshalJSON(data []byte) error {
	type taskAlias Task
	var alias taskAlias
	if err := jsoncompat.UnmarshalFloat64Aliases(data, &alias, jsoncompat.Float64Alias{
		Field:  "CostUSD",
		NewKey: "costUSD",
		OldKey: "costUsd",
	}); err != nil {
		return err
	}
	*t = Task(alias)
	return nil
}

func (tr *TaskResult) UnmarshalJSON(data []byte) error {
	type resultAlias TaskResult
	var alias resultAlias
	if err := jsoncompat.UnmarshalFloat64Aliases(data, &alias, jsoncompat.Float64Alias{
		Field:  "CostUSD",
		NewKey: "costUSD",
		OldKey: "costUsd",
	}); err != nil {
		return err
	}
	*tr = TaskResult(alias)
	return nil
}
