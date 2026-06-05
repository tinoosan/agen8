package mcp

import (
	"reflect"
	"testing"
)

func TestBuildToolDefsRetainedMCPFirstSurface(t *testing.T) {
	defs, err := buildToolDefs()
	if err != nil {
		t.Fatalf("buildToolDefs: %v", err)
	}
	got := make([]string, 0, len(defs))
	for _, def := range defs {
		got = append(got, def.name())
	}
	want := []string{
		"decision",
		"graph_query",
		"http",
		"mission",
		"project",
		"task",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("tools=%v want %v", got, want)
	}
}
