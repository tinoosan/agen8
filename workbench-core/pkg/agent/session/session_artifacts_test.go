package session

import (
	"reflect"
	"testing"
)

func TestDedupeArtifactPaths(t *testing.T) {
	in := []string{
		" /workspace/report.md ",
		"",
		"   ",
		"/workspace/report.md",
		"/workspace/data/output.json",
		"/workspace/data/output.json",
		"/workspace/summary.txt",
	}

	got := dedupeArtifactPaths(in)
	want := []string{
		"/workspace/report.md",
		"/workspace/data/output.json",
		"/workspace/summary.txt",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("dedupeArtifactPaths mismatch\nwant: %#v\ngot:  %#v", want, got)
	}
}
