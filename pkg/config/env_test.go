package config

import "testing"

func TestParseBoolEnvDefault(t *testing.T) {
	const key = "AGEN8_TEST_BOOL_ENV"
	t.Setenv(key, "")
	if !ParseBoolEnvDefault(key, true) {
		t.Fatalf("expected default true when env is empty")
	}
	if ParseBoolEnvDefault(key, false) {
		t.Fatalf("expected default false when env is empty")
	}

	t.Setenv(key, " yes ")
	if !ParseBoolEnvDefault(key, false) {
		t.Fatalf("expected yes to parse as true")
	}

	t.Setenv(key, "OFF")
	if ParseBoolEnvDefault(key, true) {
		t.Fatalf("expected OFF to parse as false")
	}

	t.Setenv(key, "unknown")
	if !ParseBoolEnvDefault(key, true) {
		t.Fatalf("expected unknown to fall back to default true")
	}
	if ParseBoolEnvDefault(key, false) {
		t.Fatalf("expected unknown to fall back to default false")
	}
}
