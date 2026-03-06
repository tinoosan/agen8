package kit

import (
	"testing"
	"unicode/utf8"

	"github.com/mattn/go-runewidth"
)

func TestTruncateRight(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		maxLen int
		want   string
	}{
		{name: "ascii", input: "abcdef", maxLen: 4, want: "abc…"},
		{name: "cjk wide chars", input: "你好世界", maxLen: 5, want: "你好…"},
		{name: "mixed runes", input: "ab你cd", maxLen: 5, want: "ab你…"},
		{name: "empty input", input: "", maxLen: 5, want: ""},
		{name: "maxLen zero", input: "abcdef", maxLen: 0, want: ""},
		{name: "maxLen one", input: "abcdef", maxLen: 1, want: "…"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := TruncateRight(tt.input, tt.maxLen)
			if got != tt.want {
				t.Fatalf("TruncateRight(%q, %d) = %q, want %q", tt.input, tt.maxLen, got, tt.want)
			}
			if !utf8.ValidString(got) {
				t.Fatalf("result is not valid UTF-8: %q", got)
			}
			if tt.maxLen > 0 && runewidth.StringWidth(got) > tt.maxLen {
				t.Fatalf("result width %d exceeds maxLen %d", runewidth.StringWidth(got), tt.maxLen)
			}
		})
	}
}

func TestTruncateMiddle(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		maxLen int
		want   string
	}{
		{name: "ascii", input: "abcdefghij", maxLen: 7, want: "abc…hij"},
		{name: "cjk wide chars", input: "你好世界和平", maxLen: 9, want: "你好…和平"},
		{name: "mixed runes", input: "ab你cd界ef", maxLen: 7, want: "ab…ef"},
		{name: "empty input", input: "", maxLen: 5, want: ""},
		{name: "maxLen zero", input: "abcdefgh", maxLen: 0, want: ""},
		{name: "maxLen one", input: "abcdefgh", maxLen: 1, want: "…"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := TruncateMiddle(tt.input, tt.maxLen)
			if got != tt.want {
				t.Fatalf("TruncateMiddle(%q, %d) = %q, want %q", tt.input, tt.maxLen, got, tt.want)
			}
			if !utf8.ValidString(got) {
				t.Fatalf("result is not valid UTF-8: %q", got)
			}
			if tt.maxLen > 0 && runewidth.StringWidth(got) > tt.maxLen {
				t.Fatalf("result width %d exceeds maxLen %d", runewidth.StringWidth(got), tt.maxLen)
			}
		})
	}
}

func TestVerbFromKind_FSStat(t *testing.T) {
	if got := VerbFromKind("fs_stat"); got != "Stat" {
		t.Fatalf("VerbFromKind(fs_stat) = %q, want %q", got, "Stat")
	}
}

func TestVerbFromKind_FSTxn(t *testing.T) {
	if got := VerbFromKind("fs_txn"); got != "Txn" {
		t.Fatalf("VerbFromKind(fs_txn) = %q, want %q", got, "Txn")
	}
}

func TestVerbFromKind_FSArchiveCreate(t *testing.T) {
	if got := VerbFromKind("fs_archive_create"); got != "Archive" {
		t.Fatalf("VerbFromKind(fs_archive_create) = %q, want %q", got, "Archive")
	}
}

func TestVerbFromKind_FSBatchEdit(t *testing.T) {
	if got := VerbFromKind("fs_batch_edit"); got != "Batch edit" {
		t.Fatalf("VerbFromKind(fs_batch_edit) = %q, want %q", got, "Batch edit")
	}
}

func TestVerbFromKind_Pipe(t *testing.T) {
	if got := VerbFromKind("pipe"); got != "Pipe" {
		t.Fatalf("VerbFromKind(pipe) = %q, want %q", got, "Pipe")
	}
}
