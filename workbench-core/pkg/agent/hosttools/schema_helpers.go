package hosttools

var (
	intOrNull         = []any{"integer", "null"}
	stringOrNull      = []any{"string", "null"}
	boolOrNull        = []any{"boolean", "null"}
	stringArrayOrNull = map[string]any{
		"type":  []any{"array", "null"},
		"items": map[string]any{"type": "string"},
	}
)
