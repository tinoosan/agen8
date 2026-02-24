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
