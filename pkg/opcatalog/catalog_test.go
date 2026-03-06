package opcatalog

import (
	"reflect"
	"testing"
)

func TestCategory(t *testing.T) {
	tests := []struct {
		op       string
		want     string
		wantSeen bool
	}{
		{op: "fs_read", want: "Explored", wantSeen: true},
		{op: "fs_stat", want: "Explored", wantSeen: true},
		{op: "fs_write", want: "Updated", wantSeen: true},
		{op: "fs_batch_edit", want: "Updated", wantSeen: true},
		{op: "fs_txn", want: "Updated", wantSeen: true},
		{op: "fs_archive_create", want: "Updated", wantSeen: true},
		{op: "fs_archive_extract", want: "Updated", wantSeen: true},
		{op: "fs_archive_list", want: "Explored", wantSeen: true},
		{op: "shell_exec", want: "Ran", wantSeen: true},
		{op: "code_exec", want: "Ran", wantSeen: true},
		{op: "http_fetch", want: "Fetched", wantSeen: true},
		{op: "browser", want: "Browsed", wantSeen: true},
		{op: "email", want: "Sent", wantSeen: true},
		{op: "trace_run", want: "Traced", wantSeen: true},
		{op: "agent_spawn", want: "Delegated", wantSeen: true},
		{op: "task_create", want: "Created", wantSeen: true},
		{op: "obsidian", want: "Knowledge", wantSeen: true},
		{op: "unknown_op", want: "", wantSeen: false},
	}

	for _, tc := range tests {
		got, ok := Category(tc.op)
		if ok != tc.wantSeen {
			t.Fatalf("Category(%q) presence = %v, want %v", tc.op, ok, tc.wantSeen)
		}
		if got != tc.want {
			t.Fatalf("Category(%q) = %q, want %q", tc.op, got, tc.want)
		}
	}
}

func TestUsesSharedRequestTitle(t *testing.T) {
	tests := []struct {
		op   string
		want bool
	}{
		{op: "fs_read", want: true},
		{op: "fs_stat", want: true},
		{op: "shell_exec", want: true},
		{op: "code_exec", want: true},
		{op: "http_fetch", want: true},
		{op: "trace_run", want: true},
		{op: "agent_spawn", want: true},
		{op: "task_create", want: true},
		{op: "obsidian", want: true},
		{op: "fs_batch_edit", want: true},
		{op: "fs_txn", want: true},
		{op: "fs_archive_create", want: true},
		{op: "fs_archive_extract", want: true},
		{op: "fs_archive_list", want: true},
		{op: "email", want: false},
		{op: "browser", want: false},
		{op: "unknown_op", want: false},
	}

	for _, tc := range tests {
		if got := UsesSharedRequestTitle(tc.op); got != tc.want {
			t.Fatalf("UsesSharedRequestTitle(%q) = %v, want %v", tc.op, got, tc.want)
		}
	}
}

func TestKnownOpsSorted(t *testing.T) {
	want := []string{
		"agent_spawn",
		"browser",
		"code_exec",
		"email",
		"fs_append",
		"fs_archive_create",
		"fs_archive_extract",
		"fs_archive_list",
		"fs_batch_edit",
		"fs_edit",
		"fs_list",
		"fs_patch",
		"fs_read",
		"fs_search",
		"fs_stat",
		"fs_txn",
		"fs_write",
		"http_fetch",
		"obsidian",
		"shell_exec",
		"soul_update",
		"task_create",
		"task_review",
		"trace_run",
	}
	if got := KnownOps(); !reflect.DeepEqual(got, want) {
		t.Fatalf("KnownOps() = %#v, want %#v", got, want)
	}
}
