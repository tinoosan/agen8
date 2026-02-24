package hosttools

import "testing"

func TestInferObsidianCommand(t *testing.T) {
	tests := []struct {
		name string
		in   obsidianArgs
		want string
	}{
		{
			name: "upsert note when write fields provided",
			in: obsidianArgs{
				Title:    "Design Notes",
				NoteType: "MOC",
			},
			want: "upsert_note",
		},
		{
			name: "search when query filters provided",
			in: obsidianArgs{
				Query: "delegation",
			},
			want: "search",
		},
		{
			name: "graph default when ambiguous",
			in:   obsidianArgs{},
			want: "graph",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := inferObsidianCommand(tc.in); got != tc.want {
				t.Fatalf("inferObsidianCommand()=%q want %q", got, tc.want)
			}
		})
	}
}

func TestResolveUpsertNoteType(t *testing.T) {
	tests := []struct {
		name string
		in   obsidianArgs
		want string
	}{
		{
			name: "explicit note type respected",
			in: obsidianArgs{
				NoteType: "p",
			},
			want: "P",
		},
		{
			name: "infer journal from file path",
			in: obsidianArgs{
				File: "journals/daily-entry.md",
			},
			want: "JOURNAL",
		},
		{
			name: "infer moc from file path",
			in: obsidianArgs{
				File: "/knowledge/mocs/architecture.md",
			},
			want: "MOC",
		},
		{
			name: "infer fleeting from inbox file path",
			in: obsidianArgs{
				File: "inbox/capture.md",
			},
			want: "F",
		},
		{
			name: "fallback to fleeting when ambiguous",
			in: obsidianArgs{
				Title: "Some note",
			},
			want: "F",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := resolveUpsertNoteType(tc.in); got != tc.want {
				t.Fatalf("resolveUpsertNoteType()=%q want %q", got, tc.want)
			}
		})
	}
}
