package app

import (
	"context"
	"io"
	"time"

	locationdomain "github.com/tinoosan/agen8-mcp-server/internal/services/location/domain"
)

type Clock interface {
	Now() time.Time
}

type HostnameResolver func() (string, error)

type ProjectReferenceChecker interface {
	HasProjectsForLocation(ctx context.Context, locationID locationdomain.ID) (bool, error)
}

type Transport interface {
	Probe(ctx context.Context, location locationdomain.Location) (ProbeResult, error)
	ListDir(ctx context.Context, location locationdomain.Location, path string) ([]DirEntry, error)
	InstallCodex(ctx context.Context, location locationdomain.Location) (InstallResult, error)
	CodexAuthStatus(ctx context.Context, location locationdomain.Location) (CodexAuthStatusResult, error)
	BeginCodexLogin(ctx context.Context, location locationdomain.Location) (CodexLoginResult, error)
	InstallClaude(ctx context.Context, location locationdomain.Location) (InstallResult, error)
	ClaudeAuthStatus(ctx context.Context, location locationdomain.Location) (ClaudeAuthStatusResult, error)
	BeginClaudeLogin(ctx context.Context, location locationdomain.Location) (ClaudeLoginResult, error)
	CompleteClaudeLogin(ctx context.Context, location locationdomain.Location, code string) (ClaudeLoginResult, error)
	StartCommand(ctx context.Context, location locationdomain.Location, spec CommandSpec) (CommandProcess, error)
	EnsureBridge(ctx context.Context, location locationdomain.Location) (Bridge, error)
}

type ProbeResult struct {
	Reachable    bool
	FileBrowsing bool
	Exec         bool
	Codex        bool
	Claude       bool
	Status       locationdomain.ProbeStatus
	FailureCode  locationdomain.FailureCode
	Message      string
	ProbedAt     time.Time
}

type DirEntry struct {
	Name string
	Path string
	Type locationdomain.DirEntryType
	Size int64
}

type InstallResult struct {
	Output string
}

type CodexAuthStatusResult struct {
	LoggedIn bool
	Method   string
	Output   string
}

type CodexLoginResult struct {
	Output   string
	LoginURL string
	LogPath  string
	PID      string
}

type ClaudeAuthStatusResult struct {
	LoggedIn   bool
	AuthMethod string
	Provider   string
	RawJSON    string
}

type ClaudeLoginResult struct {
	Output   string
	LoginURL string
	LogPath  string
	PID      string
}

type CommandSpec struct {
	Command string
	Args    []string
	Workdir string
	Env     []string
}

type CommandProcess interface {
	StdinPipe() (io.WriteCloser, error)
	StdoutPipe() (io.ReadCloser, error)
	StderrText() string
	Start() error
	Wait() error
	Kill() error
}

type TCPForward interface {
	LocalAddr() string
	Close() error
}

type Bridge struct {
	BaseURL      string
	WebSocketURL string
	MCPBaseURL   string
	Diagnostics  string
}
