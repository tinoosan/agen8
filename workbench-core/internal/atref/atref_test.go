package atref

import "testing"

func TestExtractAtRefs_Quoted(t *testing.T) {
	t.Parallel()

	got := ExtractAtRefs(`please open @"my file.md" and @‘notes one.md’ and @cmd/main.go`)
	if len(got) != 3 {
		t.Fatalf("got %d tokens, want %d (%v)", len(got), 3, got)
	}
	if got[0] != "my file.md" {
		t.Fatalf("got[0]=%q, want %q", got[0], "my file.md")
	}
	if got[1] != "notes one.md" {
		t.Fatalf("got[1]=%q, want %q", got[1], "notes one.md")
	}
	if got[2] != "cmd/main.go" {
		t.Fatalf("got[2]=%q, want %q", got[2], "cmd/main.go")
	}
}

func TestActiveAtTokenAtEnd_Unquoted(t *testing.T) {
	q, start, end, ok := ActiveAtTokenAtEnd("open @cmd/main.go")
	if !ok {
		t.Fatalf("expected ok")
	}
	if q != "cmd/main.go" {
		t.Fatalf("query=%q", q)
	}
	if start <= 0 || end != len([]rune("open @cmd/main.go")) {
		t.Fatalf("start/end=%d/%d", start, end)
	}
}

func TestActiveAtTokenAtEnd_QuotedIncomplete(t *testing.T) {
	q, _, _, ok := ActiveAtTokenAtEnd(`open @"my file`)
	if !ok {
		t.Fatalf("expected ok")
	}
	if q != "my file" {
		t.Fatalf("query=%q", q)
	}
}

func TestActiveAtTokenAtEnd_DoesNotMatchEmailLike(t *testing.T) {
	_, _, _, ok := ActiveAtTokenAtEnd("email me at foo@bar.com")
	if ok {
		t.Fatalf("expected ok=false")
	}
}

func TestFormatAtRef_QuotesWhenNeeded(t *testing.T) {
	if got := FormatAtRef("a b.txt"); got != "@'a b.txt'" {
		t.Fatalf("got %q", got)
	}
	if got := FormatAtRef("it's ok.txt"); got != `@"it's ok.txt"` {
		t.Fatalf("got %q", got)
	}
	if got := FormatAtRef("plain.txt"); got != "@plain.txt" {
		t.Fatalf("got %q", got)
	}
}

