package app

import (
	"reflect"
	"testing"
)

func TestResolveCodeExecRequiredImports_ConfigOnly(t *testing.T) {
	got := resolveCodeExecRequiredImports([]string{"numpy", "requests", " numpy "})
	want := []string{"numpy", "requests"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("resolved imports mismatch: got %v want %v", got, want)
	}
}

func TestParseMissingPythonModules(t *testing.T) {
	err := errString("code_exec preflight: missing python module(s): pandas, requests")
	got := parseMissingPythonModules(err)
	want := []string{"pandas", "requests"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("missing modules mismatch: got %v want %v", got, want)
	}
}

func TestParseMissingPythonModules_NoMarker(t *testing.T) {
	if got := parseMissingPythonModules(errString("other error")); got != nil {
		t.Fatalf("expected nil, got %v", got)
	}
}

func TestResolveCodeExecRequiredImports_Empty(t *testing.T) {
	got := resolveCodeExecRequiredImports(nil)
	if got != nil {
		t.Fatalf("expected nil, got %v", got)
	}
}

func TestResolveCodeExecRequiredImports_Deterministic(t *testing.T) {
	got := resolveCodeExecRequiredImports([]string{"requests", "pandas"})
	want := []string{"pandas", "requests"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("resolved imports mismatch: got %v want %v", got, want)
	}
}

type errString string

func (e errString) Error() string { return string(e) }
