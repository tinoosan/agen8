package jsoncompat

import (
	"encoding/json"
	"fmt"
	"reflect"
)

// Float64Alias maps a canonical JSON key to a legacy fallback key for float64 fields.
type Float64Alias struct {
	Field  string
	NewKey string
	OldKey string
}

// UnmarshalFloat64Aliases unmarshals JSON into target, then applies legacy key fallbacks
// for any float64 fields listed in aliases. When both keys are present, NewKey wins.
func UnmarshalFloat64Aliases(data []byte, target any, aliases ...Float64Alias) error {
	if err := json.Unmarshal(data, target); err != nil {
		return err
	}
	if len(aliases) == 0 {
		return nil
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	value := reflect.ValueOf(target)
	if value.Kind() != reflect.Pointer || value.IsNil() {
		return fmt.Errorf("jsoncompat: target must be a non-nil pointer")
	}
	elem := value.Elem()
	if elem.Kind() != reflect.Struct {
		return fmt.Errorf("jsoncompat: target must point to a struct")
	}

	for _, alias := range aliases {
		field := elem.FieldByName(alias.Field)
		if !field.IsValid() || !field.CanSet() || field.Kind() != reflect.Float64 {
			return fmt.Errorf("jsoncompat: target field %q must be a settable float64", alias.Field)
		}

		msg, ok := raw[alias.NewKey]
		if !ok {
			msg, ok = raw[alias.OldKey]
		}
		if !ok {
			continue
		}

		var v float64
		if err := json.Unmarshal(msg, &v); err != nil {
			return err
		}
		field.SetFloat(v)
	}
	return nil
}
