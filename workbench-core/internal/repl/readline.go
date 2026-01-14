package repl

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/chzyer/readline"
)

// Reader is a thin wrapper around github.com/chzyer/readline.
//
// Why this exists:
// - The workbench demo needs a reliable interactive terminal UX:
//   - arrow-key navigation
//   - editing/backspace
//   - history
//   - correct redraw behavior while the program is also printing logs
//
// - Implementing a line editor correctly is deceptively hard; readline is a proven solution.
//
// Reader implements the small interface used by the REPL code in cmd/workbench:
//
//	ReadLine(prompt) (string, error)
type Reader struct {
	rl *readline.Instance
}

// NewReader creates a new readline-backed Reader.
//
// historyPath is optional. If set, user inputs are persisted across runs.
func NewReader(historyPath string) (*Reader, error) {
	cfg := &readline.Config{
		Prompt:          "you> ",
		HistoryFile:     strings.TrimSpace(historyPath),
		InterruptPrompt: "^C",
		EOFPrompt:       "exit",
	}
	rl, err := readline.NewEx(cfg)
	if err != nil {
		return nil, err
	}
	return &Reader{rl: rl}, nil
}

// Close restores terminal state and closes any underlying resources.
func (r *Reader) Close() error {
	if r == nil || r.rl == nil {
		return nil
	}
	return r.rl.Close()
}

// ReadLine reads a single line of input (without a trailing newline).
//
// The prompt is set for this read.
func (r *Reader) ReadLine(prompt string) (string, error) {
	if r == nil || r.rl == nil {
		return "", fmt.Errorf("repl reader not initialized")
	}
	r.rl.SetPrompt(prompt)
	return r.rl.Readline()
}

// Printf prints output in a readline-friendly way.
//
// Use this when you need to write to stdout while readline is active; it keeps the
// prompt and line buffer consistent.
func (r *Reader) Printf(format string, args ...any) {
	if r == nil || r.rl == nil {
		_, _ = fmt.Fprintf(os.Stdout, format, args...)
		return
	}
	_, _ = r.rl.Write([]byte(fmt.Sprintf(format, args...)))
	r.rl.Refresh()
}

// Write implements io.Writer by forwarding to readline.Printf-style output.
//
// This is handy for code that expects an io.Writer (e.g. help text).
func (r *Reader) Write(p []byte) (int, error) {
	if r == nil || r.rl == nil {
		return os.Stdout.Write(p)
	}
	n, err := r.rl.Write(p)
	r.rl.Refresh()
	return n, err
}

var _ io.Writer = (*Reader)(nil)
