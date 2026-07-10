package infra

import (
	"errors"
	"strings"
	"testing"
)

// TestShellSingleQuoteNeutralizesInjection is the security-critical test: every
// shell metacharacter must survive as literal data inside the quoted argument.
// If any of these escaped a real remote shell, a malicious file name would be
// remote code execution.
func TestShellSingleQuoteNeutralizesInjection(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"plain", "main.go", `'main.go'`},
		{"semicolon", "a; rm -rf /", `'a; rm -rf /'`},
		{"command-sub-dollar", "$(curl evil)", `'$(curl evil)'`},
		{"command-sub-backtick", "`curl evil`", "'`curl evil`'"},
		{"pipe", "x | sh", `'x | sh'`},
		{"single-quote-break", "a' rm -rf / '", `'a'\'' rm -rf / '\'''`},
		{"newline", "a\nrm -rf /", "'a\nrm -rf /'"},
		{"ampersand", "a && reboot", `'a && reboot'`},
		{"redirect", "a > /etc/passwd", `'a > /etc/passwd'`},
		{"empty", "", `''`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := shellSingleQuote(tc.in)
			if got != tc.want {
				t.Fatalf("shellSingleQuote(%q) = %q, want %q", tc.in, got, tc.want)
			}
			// Structural guarantee: the result is wrapped in single quotes, and
			// every internal single quote is part of the escape sequence '\''.
			if !strings.HasPrefix(got, "'") || !strings.HasSuffix(got, "'") {
				t.Fatalf("result %q is not single-quote wrapped", got)
			}
		})
	}
}

// TestShellSingleQuoteRoundTripsThroughSh proves the escaping is correct by the
// only standard that matters: feeding the quoted string to a real POSIX shell
// must echo back the exact original bytes, with no command executing.
func TestShellSingleQuoteRoundTripsThroughSh(t *testing.T) {
	for _, in := range []string{
		"main.go",
		"weird name with spaces.txt",
		"a'; touch /tmp/pwned; echo '",
		"$(touch /tmp/pwned)",
		"`touch /tmp/pwned`",
		"semi;colon&and|pipe",
	} {
		quoted := shellSingleQuote(in)
		got := runShEcho(t, quoted)
		if got != in {
			t.Fatalf("round trip of %q via %s = %q, want unchanged", in, quoted, got)
		}
	}
}

// TestGitBaselineCommandIsFixedAndQuoted documents the exact remote command
// shape so a regression that drops quoting or changes the verb is caught.
func TestGitBaselineCommandIsFixedAndQuoted(t *testing.T) {
	dir := "/srv/app/sub dir"
	name := "weird'; rm -rf /; '.go"
	command := "git -C " + shellSingleQuote(dir) + " show " + shellSingleQuote("HEAD:./"+name)

	if !strings.HasPrefix(command, "git -C '") {
		t.Fatalf("command must start with the fixed git -C verb: %q", command)
	}
	if !strings.Contains(command, " show '") {
		t.Fatalf("command must use the fixed read-only show verb: %q", command)
	}
	// The hostile name must be fully contained in a quoted argument — no bare
	// rm, no unquoted semicolon outside the quotes.
	if strings.Contains(command, "; rm -rf /; '.go") && !strings.Contains(command, `'\''; rm -rf /; '\''`) {
		t.Fatalf("hostile name leaked outside quoting: %q", command)
	}
	got := runShWordCount(t, command)
	// The whole command, run through `set -- <command>` style splitting, must
	// resolve to exactly 5 shell words: git, -C, <dir>, show, <rev>.
	if got != 5 {
		t.Fatalf("command split into %d shell words, want 5 (git -C <dir> show <rev>): %q", got, command)
	}
}

func TestBoundedOutputCapsRemoteCommandOutput(t *testing.T) {
	output := newBoundedOutput(5)

	if n, err := output.Write([]byte("abc")); err != nil || n != 3 {
		t.Fatalf("first write = (%d, %v), want (3, nil)", n, err)
	}
	if n, err := output.Write([]byte("def")); err != nil || n != 3 {
		t.Fatalf("overflow write = (%d, %v), want (3, nil)", n, err)
	}
	if got := string(output.Bytes()); got != "abcde" {
		t.Fatalf("bounded output = %q, want abcde", got)
	}
	if !errors.Is(output.Err(), errSSHCommandOutputTooLarge) {
		t.Fatalf("expected output-too-large error, got %v", output.Err())
	}
}
