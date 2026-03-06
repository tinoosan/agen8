package opmeta

import "testing"

func TestFormatRequestTitle_SharedOps(t *testing.T) {
	tests := []struct {
		name string
		data map[string]string
		want string
	}{
		{
			name: "stat with path",
			data: map[string]string{"op": "fs_stat", "path": "/workspace/a.txt"},
			want: "Stat /workspace/a.txt",
		},
		{
			name: "search with path and query",
			data: map[string]string{"op": "fs_search", "path": "/workspace", "query": "todo"},
			want: `Search /workspace for "todo"`,
		},
		{
			name: "search with path and pattern",
			data: map[string]string{"op": "fs_search", "path": "/workspace", "pattern": "TODO\\([a-z]+\\)"},
			want: `Search /workspace for "TODO\\([a-z]+\\)"`,
		},
		{
			name: "txn with step count",
			data: map[string]string{"op": "fs_txn", "steps": "3"},
			want: "Txn 3 steps",
		},
		{
			name: "archive create",
			data: map[string]string{"op": "fs_archive_create", "path": "/workspace/journals", "destination": "/workspace/journals.tar.gz"},
			want: "Archive /workspace/journals -> /workspace/journals.tar.gz",
		},
		{
			name: "archive list",
			data: map[string]string{"op": "fs_archive_list", "path": "/workspace/journals.tar.gz"},
			want: "List archive /workspace/journals.tar.gz",
		},
		{
			name: "shell uses argvPreview",
			data: map[string]string{"op": "shell_exec", "argvPreview": "rg -n test"},
			want: "rg -n test",
		},
		{
			name: "http with body preview",
			data: map[string]string{"op": "http_fetch", "method": "post", "url": "https://example.com", "body": "{\n\"x\":1\n}"},
			want: "POST https://example.com body: { \"x\":1 }",
		},
		{
			name: "code exec",
			data: map[string]string{"op": "code_exec", "language": "python"},
			want: "Run python code",
		},
		{
			name: "trace action and key",
			data: map[string]string{"op": "trace_run", "traceAction": "set", "traceKey": "alpha"},
			want: "trace.set alpha",
		},
		{
			name: "agent spawn with model and depth",
			data: map[string]string{
				"op":           "agent_spawn",
				"goal":         "Review this module and summarize risks",
				"model":        "gpt-5-mini",
				"currentDepth": "1",
				"maxDepth":     "3",
			},
			want: "Spawn child agent: Review this module and summarize risks (model=gpt-5-mini, depth=1/3)",
		},
		{
			name: "default op+path",
			data: map[string]string{"op": "custom", "path": "/tmp/file"},
			want: "custom /tmp/file",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := FormatRequestTitle(tc.data); got != tc.want {
				t.Fatalf("FormatRequestTitle()=%q want %q", got, tc.want)
			}
		})
	}
}

func TestShouldHideRoutingNoiseOp(t *testing.T) {
	tests := []struct {
		op   string
		path string
		want bool
	}{
		{op: "fs_list", path: "/workspace/deliverables/a.txt", want: true},
		{op: "fs_stat", path: "/workspace/quarantine/a.txt", want: true},
		{op: "fs_read", path: "/workspace/quarantine/a.txt", want: true},
		{op: "fs_read", path: "/workspace/src/main.go", want: false},
		{op: "shell_exec", path: "/workspace/deliverables/a.txt", want: false},
	}
	for _, tc := range tests {
		if got := ShouldHideRoutingNoiseOp(tc.op, tc.path); got != tc.want {
			t.Fatalf("ShouldHideRoutingNoiseOp(%q,%q)=%v want %v", tc.op, tc.path, got, tc.want)
		}
	}
}
