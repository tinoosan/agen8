package membertype

import "fmt"

// registry holds all registered member types, keyed by name.
var registry = map[MemberTypeName]MemberType{}

// Register adds a member type to the global registry.
// Panics if a type with the same name is already registered.
func Register(t MemberType) {
	name := t.Name()
	if _, exists := registry[name]; exists {
		panic(fmt.Sprintf("membertype: duplicate registration for %q", name))
	}
	registry[name] = t
}

// Lookup returns the member type for the given name.
// Returns an error if the name is not registered.
func Lookup(name MemberTypeName) (MemberType, error) {
	t, ok := registry[name]
	if !ok {
		return nil, fmt.Errorf("membertype: unknown member type %q", name)
	}
	return t, nil
}

// All returns a copy of all registered member types.
func All() map[MemberTypeName]MemberType {
	out := make(map[MemberTypeName]MemberType, len(registry))
	for k, v := range registry {
		out[k] = v
	}
	return out
}

func init() {
	Register(&CoordinatorType{})
	Register(&LoneCoordinatorType{})
	Register(&WorkerType{})
	Register(&ReviewerType{})
}
