package skills

import (
	"reflect"
	"testing"
)

func TestRequiredPythonImportsFromCompatibility_RequiredOnly(t *testing.T) {
	got := RequiredPythonImportsFromCompatibility("Requires python3, pandas. Optional - sqlalchemy, psycopg2 for database connectivity.")
	want := []string{"pandas"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("required imports mismatch: got %v want %v", got, want)
	}
}

func TestRequiredPythonImportsFromCompatibility_FiltersNonPythonDeps(t *testing.T) {
	got := RequiredPythonImportsFromCompatibility("Requires python3, requests, curl, jq.")
	want := []string{"requests"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("required imports mismatch: got %v want %v", got, want)
	}
}

func TestRequiredPythonImportsFromEntries_DedupesAndSorts(t *testing.T) {
	entries := []SkillEntry{
		{Dir: "a", Skill: &Skill{Compatibility: "Requires python3, requests, pandas."}},
		{Dir: "b", Skill: &Skill{Compatibility: "Requires python3, pandas. Optional numpy."}},
		{Dir: "c", Skill: &Skill{Compatibility: "Requires bash and python3 only."}},
	}
	got := RequiredPythonImportsFromEntries(entries)
	want := []string{"pandas", "requests"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("entry imports mismatch: got %v want %v", got, want)
	}
}

func TestRequiredPythonImportsFromCompatibility_NoRequires(t *testing.T) {
	got := RequiredPythonImportsFromCompatibility("Optional matplotlib.")
	if len(got) != 0 {
		t.Fatalf("expected no imports, got %v", got)
	}
}

func TestRequiredPythonImportsFromCompatibility_IgnoresDescriptiveNonPythonText(t *testing.T) {
	got := RequiredPythonImportsFromCompatibility("Requires pandoc for format conversion. Optional python3, matplotlib for chart generation.")
	if len(got) != 0 {
		t.Fatalf("expected no imports, got %v", got)
	}
}

func TestRequiredPythonImportsFromCompatibility_HandlesPythonClauseWithTailText(t *testing.T) {
	got := RequiredPythonImportsFromCompatibility("Requires python3, requests for currency conversions, curl.")
	want := []string{"requests"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("required imports mismatch: got %v want %v", got, want)
	}
}
