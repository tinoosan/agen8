package bridge

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
)

const Version = "bridge-v1"
const claudeBridgeStdinEOF = `{"agen8Bridge":"stdin_eof"}`
const claudeBridgeStartMessageType = "start"
const bridgeWebSocketReadLimit = 8 << 20

type Server struct {
	mu           sync.Mutex
	codexBackend *codexBackend
	codexLogs    bytes.Buffer
	logger       *slog.Logger
}

type codexBackend struct {
	addr    string
	proc    *exec.Cmd
	stdin   io.WriteCloser
	stdout  io.ReadCloser
	configs []string
	done    chan error
}

func NewServer() *Server {
	return &Server{logger: slog.Default().With("service", "bridge")}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Agen8-Bridge-Version", Version)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/diagnostics", s.handleDiagnostics)
	mux.HandleFunc("/logs", s.handleLogs)
	mux.HandleFunc("/codex", s.handleCodex)
	mux.HandleFunc("/claude", s.handleClaude)
	return recoverHTTP(s.logger, mux)
}

func (s *Server) Serve(ctx context.Context, addr string) error {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return fmt.Errorf("http address is required")
	}
	server := &http.Server{Addr: addr, Handler: s.Handler()}
	errCh := make(chan error, 1)
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
			return
		}
		errCh <- nil
	}()
	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
		s.Close()
		return ctx.Err()
	case err := <-errCh:
		s.Close()
		return err
	}
}

func (s *Server) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stopCodexBackendLocked()
	s.codexBackend = nil
}

func (s *Server) handleDiagnostics(w http.ResponseWriter, _ *http.Request) {
	codexPath, codexErr := resolveCodexExecutable()
	codexVersion := ""
	if codexErr == nil {
		output, err := exec.Command(codexPath, "--version").CombinedOutput()
		if err != nil {
			codexErr = fmt.Errorf("codex version: %w: %s", err, strings.TrimSpace(string(output)))
		} else {
			codexVersion = strings.TrimSpace(string(output))
		}
	}
	claudePath, claudeErr := resolveClaudeExecutable()
	claudeVersion := ""
	if claudeErr == nil {
		output, err := exec.Command(claudePath, "--version").CombinedOutput()
		if err != nil {
			claudeErr = fmt.Errorf("claude version: %w: %s", err, strings.TrimSpace(string(output)))
		} else {
			claudeVersion = strings.TrimSpace(string(output))
		}
	}
	w.Header().Set("Content-Type", "application/json")
	resp := map[string]any{
		"bridgeVersion": Version,
		"codexPath":     codexPath,
		"codexVersion":  codexVersion,
		"claudePath":    claudePath,
		"claudeVersion": claudeVersion,
	}
	if codexErr != nil {
		resp["codexError"] = codexErr.Error()
	}
	if claudeErr != nil {
		resp["claudeError"] = claudeErr.Error()
	}
	_ = json.NewEncoder(w).Encode(resp)
}

func (s *Server) handleLogs(w http.ResponseWriter, _ *http.Request) {
	text := strings.TrimSpace(s.codexLogs.String())
	if text == "" {
		text = "no codex app-server logs captured"
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte(text))
}

func recoverHTTP(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recovered := recover(); recovered != nil {
				if logger != nil {
					logger.ErrorContext(r.Context(), "bridge handler panic", "path", r.URL.Path, "panic", recovered, "stack", string(debug.Stack()))
				}
				http.Error(w, "bridge handler panic", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func (s *Server) handleCodex(w http.ResponseWriter, r *http.Request) {
	configs := append(
		compactHeaderValues(r.Header.Values("X-Agen8-Codex-Config")),
		compactHeaderValues(r.Header.Values("X-Agen8-Codex-MCP-Config"))...,
	)
	s.logger.InfoContext(r.Context(), "bridge codex request received", "config_count", len(configs))
	backend, err := s.ensureCodexAppServer(r.Context(), configs)
	if err != nil {
		s.logger.ErrorContext(r.Context(), "bridge codex backend ensure failed", "config_count", len(configs), "error", err)
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	clientConn, err := websocket.Accept(w, r, nil)
	if err != nil {
		s.logger.ErrorContext(r.Context(), "bridge codex websocket accept failed", "error", err)
		return
	}
	defer clientConn.CloseNow()
	clientConn.SetReadLimit(bridgeWebSocketReadLimit)
	if backend.addr != "" {
		s.logger.InfoContext(r.Context(), "bridge codex websocket proxy starting", "backend_addr", backend.addr, "config_count", len(backend.configs))
		backendConn, _, err := websocket.Dial(r.Context(), "ws://"+backend.addr, nil)
		if err != nil {
			s.logger.ErrorContext(r.Context(), "bridge codex backend websocket dial failed", "backend_addr", backend.addr, "error", err, "codex_logs", strings.TrimSpace(s.codexLogs.String()))
			_ = clientConn.Close(websocket.StatusBadGateway, err.Error())
			return
		}
		defer backendConn.CloseNow()
		backendConn.SetReadLimit(bridgeWebSocketReadLimit)
		result := proxyWebSocket(r.Context(), clientConn, backendConn, s.logger.With("component", "codex_proxy", "backend_addr", backend.addr))
		s.discardCodexBackend(backend)
		s.logger.InfoContext(r.Context(), "bridge codex websocket proxy stopped", "backend_addr", backend.addr, "direction", result.direction, "error", result.err, "codex_logs", strings.TrimSpace(s.codexLogs.String()))
		return
	}
	s.logger.InfoContext(r.Context(), "bridge codex stdio proxy starting", "config_count", len(backend.configs))
	result := proxyWebSocketStdio(r.Context(), clientConn, backend.stdin, backend.stdout, s.logger.With("component", "codex_stdio_proxy"))
	s.discardCodexBackend(backend)
	s.logger.InfoContext(r.Context(), "bridge codex stdio proxy stopped", "direction", result.direction, "error", result.err, "codex_logs", strings.TrimSpace(s.codexLogs.String()))
}

func (s *Server) handleClaude(w http.ResponseWriter, r *http.Request) {
	clientConn, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	defer clientConn.CloseNow()
	clientConn.SetReadLimit(bridgeWebSocketReadLimit)

	spec, err := readClaudeCommandSpec(r.Context(), clientConn)
	if err != nil {
		_ = clientConn.Close(websocket.StatusPolicyViolation, err.Error())
		return
	}
	command := strings.TrimSpace(spec.Command)
	if isClaudeCommand(command) {
		resolved, err := ensureClaudeExecutable(r.Context())
		if err != nil {
			_ = clientConn.Close(websocket.StatusInternalError, err.Error())
			return
		}
		command = resolved
	}
	cmd := exec.CommandContext(context.WithoutCancel(r.Context()), command, spec.Args...)
	if spec.Workdir != "" {
		cmd.Dir = spec.Workdir
	}
	cmd.Env = append(cmd.Environ(), spec.Env...)
	stderr := &bytes.Buffer{}
	cmd.Stderr = stderr
	stdin, err := cmd.StdinPipe()
	if err != nil {
		_ = clientConn.Close(websocket.StatusInternalError, err.Error())
		return
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = clientConn.Close(websocket.StatusInternalError, err.Error())
		return
	}
	if err := cmd.Start(); err != nil {
		_ = clientConn.Close(websocket.StatusInternalError, err.Error())
		return
	}

	done := make(chan proxyCopyResult, 2)
	go copyClaudeWebSocketToWriter(r.Context(), stdin, clientConn, done)
	go copyReaderToWebSocket(r.Context(), clientConn, stdout, "claude_stdout_to_client", done)
	result := <-done
	s.logger.InfoContext(r.Context(), "bridge claude proxy side stopped", "direction", result.direction, "error", result.err)
	_ = stdin.Close()
	if err := cmd.Wait(); err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = err.Error()
		}
		_ = clientConn.Close(websocket.StatusInternalError, message)
		return
	}
}

func copyClaudeWebSocketToWriter(ctx context.Context, dst io.Writer, src *websocket.Conn, done chan<- proxyCopyResult) {
	for {
		msgType, reader, err := src.Reader(ctx)
		if err != nil {
			done <- proxyCopyResult{direction: "claude_client_to_stdin", err: err}
			return
		}
		if msgType != websocket.MessageText {
			done <- proxyCopyResult{direction: "claude_client_to_stdin", err: fmt.Errorf("non-text websocket message")}
			return
		}
		data, err := io.ReadAll(io.LimitReader(reader, 8<<20))
		if err != nil {
			done <- proxyCopyResult{direction: "claude_client_to_stdin", err: err}
			return
		}
		if strings.TrimSpace(string(data)) == claudeBridgeStdinEOF {
			done <- proxyCopyResult{direction: "claude_client_to_stdin", err: nil}
			return
		}
		if _, err := dst.Write(data); err != nil {
			done <- proxyCopyResult{direction: "claude_client_to_stdin", err: err}
			return
		}
		if _, err := dst.Write([]byte("\n")); err != nil {
			done <- proxyCopyResult{direction: "claude_client_to_stdin", err: err}
			return
		}
	}
}

type claudeCommandSpec struct {
	Type    string   `json:"type,omitempty"`
	Command string   `json:"command"`
	Args    []string `json:"args"`
	Workdir string   `json:"workdir"`
	Env     []string `json:"env"`
}

func readClaudeCommandSpec(ctx context.Context, conn *websocket.Conn) (claudeCommandSpec, error) {
	msgType, reader, err := conn.Reader(ctx)
	if err != nil {
		return claudeCommandSpec{}, fmt.Errorf("read claude command spec: %w", err)
	}
	if msgType != websocket.MessageText {
		return claudeCommandSpec{}, fmt.Errorf("claude command spec must be a text message")
	}
	data, err := io.ReadAll(io.LimitReader(reader, 8<<20))
	if err != nil {
		return claudeCommandSpec{}, fmt.Errorf("read claude command spec body: %w", err)
	}
	return parseClaudeCommandSpec(data)
}

func parseClaudeCommandSpec(data []byte) (claudeCommandSpec, error) {
	var spec claudeCommandSpec
	if err := json.Unmarshal(data, &spec); err != nil {
		return claudeCommandSpec{}, fmt.Errorf("parse claude command spec: %w", err)
	}
	if strings.TrimSpace(spec.Type) != claudeBridgeStartMessageType {
		return claudeCommandSpec{}, fmt.Errorf("claude command spec start message is required")
	}
	spec.Command = strings.TrimSpace(spec.Command)
	spec.Workdir = strings.TrimSpace(spec.Workdir)
	if spec.Command == "" {
		return claudeCommandSpec{}, fmt.Errorf("claude command is required")
	}
	if spec.Workdir == "" {
		return claudeCommandSpec{}, fmt.Errorf("claude workdir is required")
	}
	return spec, nil
}

func (s *Server) ensureCodexAppServer(ctx context.Context, configs []string) (*codexBackend, error) {
	s.mu.Lock()
	if s.codexBackend != nil && s.codexBackend.proc != nil && s.codexBackend.proc.Process != nil {
		if exitErr, exited := s.codexBackend.exitStatus(); exited {
			s.logger.WarnContext(ctx, "bridge codex cached backend exited",
				"error", exitErr,
				"config_count", len(s.codexBackend.configs),
				"codex_logs", strings.TrimSpace(s.codexLogs.String()),
			)
			s.codexBackend = nil
			_ = exitErr
		}
		if s.codexBackend != nil && equalStringSlices(s.codexBackend.configs, configs) {
			backend := s.codexBackend
			s.logger.InfoContext(ctx, "bridge codex cached backend reused", "backend_addr", backend.addr, "config_count", len(backend.configs))
			s.mu.Unlock()
			return backend, nil
		}
		if s.codexBackend != nil {
			s.logger.InfoContext(ctx, "bridge codex cached backend replacing", "old_config_count", len(s.codexBackend.configs), "new_config_count", len(configs))
			s.stopCodexBackendLocked()
			s.codexBackend = nil
		}
	}
	port, err := reservePort()
	if err != nil {
		s.mu.Unlock()
		return nil, err
	}
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	codexPath, err := resolveCodexExecutable()
	if err != nil {
		s.mu.Unlock()
		return nil, err
	}
	s.logger.InfoContext(ctx, "bridge codex executable resolved", "codex_path", codexPath, "config_count", len(configs))
	if !codexSupportsListen(ctx, codexPath) {
		cmd := exec.CommandContext(context.WithoutCancel(ctx), codexPath, codexAppServerArgs(false, "", configs)...)
		s.codexLogs.Reset()
		cmd.Stderr = &s.codexLogs
		stdin, err := cmd.StdinPipe()
		if err != nil {
			s.mu.Unlock()
			return nil, fmt.Errorf("open codex app-server stdin: %w", err)
		}
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			s.mu.Unlock()
			return nil, fmt.Errorf("open codex app-server stdout: %w", err)
		}
		if err := cmd.Start(); err != nil {
			s.mu.Unlock()
			return nil, fmt.Errorf("start codex app-server: %w", err)
		}
		backend := newCodexBackend(cmd, "", stdin, stdout, configs)
		s.codexBackend = backend
		s.logger.InfoContext(ctx, "bridge codex app-server started", "transport", "stdio", "pid", cmd.Process.Pid, "config_count", len(configs))
		s.mu.Unlock()
		return backend, nil
	}
	cmd := exec.CommandContext(context.WithoutCancel(ctx), codexPath, codexAppServerArgs(true, "ws://"+addr, configs)...)
	s.codexLogs.Reset()
	cmd.Stdout = &s.codexLogs
	cmd.Stderr = &s.codexLogs
	if err := cmd.Start(); err != nil {
		s.mu.Unlock()
		return nil, fmt.Errorf("start codex app-server: %w", err)
	}
	backend := newCodexBackend(cmd, addr, nil, nil, configs)
	s.codexBackend = backend
	s.logger.InfoContext(ctx, "bridge codex app-server started", "transport", "websocket", "backend_addr", addr, "pid", cmd.Process.Pid, "config_count", len(configs))
	s.mu.Unlock()
	if err := waitReady(ctx, "http://"+addr+"/readyz"); err != nil {
		s.Close()
		return nil, fmt.Errorf("codex app-server ready: %w: %s", err, strings.TrimSpace(s.codexLogs.String()))
	}
	s.logger.InfoContext(ctx, "bridge codex app-server ready", "backend_addr", addr, "config_count", len(configs))
	return backend, nil
}

func newCodexBackend(cmd *exec.Cmd, addr string, stdin io.WriteCloser, stdout io.ReadCloser, configs []string) *codexBackend {
	backend := &codexBackend{
		addr:    addr,
		proc:    cmd,
		stdin:   stdin,
		stdout:  stdout,
		configs: append([]string(nil), configs...),
		done:    make(chan error, 1),
	}
	go func() {
		err := cmd.Wait()
		backend.done <- err
		close(backend.done)
	}()
	return backend
}

func (b *codexBackend) exitStatus() (error, bool) {
	if b == nil || b.done == nil {
		return nil, false
	}
	select {
	case err, ok := <-b.done:
		if !ok {
			return nil, true
		}
		return err, true
	default:
		return nil, false
	}
}

func (s *Server) stopCodexBackendLocked() {
	if s.codexBackend == nil || s.codexBackend.proc == nil || s.codexBackend.proc.Process == nil {
		return
	}
	s.logger.Info("bridge codex backend stopping", "pid", s.codexBackend.proc.Process.Pid, "backend_addr", s.codexBackend.addr, "config_count", len(s.codexBackend.configs))
	_ = s.codexBackend.proc.Process.Kill()
	if s.codexBackend.done != nil {
		select {
		case <-s.codexBackend.done:
		case <-time.After(2 * time.Second):
		}
	}
}

func (s *Server) discardCodexBackend(backend *codexBackend) {
	if backend == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.codexBackend != backend {
		return
	}
	s.stopCodexBackendLocked()
	s.codexBackend = nil
}

func codexSupportsListen(ctx context.Context, codexPath string) bool {
	helpCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	output, err := exec.CommandContext(helpCtx, codexPath, "app-server", "--help").CombinedOutput()
	return err == nil && strings.Contains(string(output), "--listen")
}

func codexAppServerArgs(withListen bool, listenURL string, configs []string) []string {
	args := []string{"app-server"}
	if withListen {
		args = append(args, "--listen", strings.TrimSpace(listenURL))
	}
	for _, config := range compactHeaderValues(configs) {
		args = append(args, "--config", config)
	}
	return args
}

func compactHeaderValues(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}

func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func resolveCodexExecutable() (string, error) {
	if path := strings.TrimSpace(os.Getenv("AGEN8_CODEX_BIN")); path != "" {
		if info, err := os.Stat(path); err == nil && !info.IsDir() && info.Mode().Perm()&0o111 != 0 {
			return path, nil
		}
		return "", fmt.Errorf("AGEN8_CODEX_BIN %q is not executable", path)
	}
	home := strings.TrimSpace(os.Getenv("HOME"))
	for _, path := range codexCandidatePaths(home) {
		if info, err := os.Stat(path); err == nil && !info.IsDir() && info.Mode().Perm()&0o111 != 0 {
			return path, nil
		}
	}
	if shell := strings.TrimSpace(os.Getenv("SHELL")); shell != "" {
		output, err := exec.Command(shell, "-lc", "command -v codex").Output()
		if err == nil {
			path := strings.TrimSpace(string(output))
			if path != "" && !isBundledEditorCodex(path) {
				return path, nil
			}
		}
	}
	if path, err := exec.LookPath("codex"); err == nil && !isBundledEditorCodex(path) {
		return path, nil
	}
	return "", fmt.Errorf("codex binary was not found for bridge process; install codex on the SSH location or ensure it is visible from PATH/login shell")
}

func ensureClaudeExecutable(ctx context.Context) (string, error) {
	path, err := resolveClaudeExecutable()
	if err == nil {
		return path, nil
	}
	installOutput, installErr := installClaudeExecutable(ctx)
	if installErr != nil {
		return "", fmt.Errorf("%w; auto-install failed: %v: %s", err, installErr, strings.TrimSpace(installOutput))
	}
	path, err = resolveClaudeExecutable()
	if err != nil {
		return "", fmt.Errorf("claude install completed but executable was not found: %w: %s", err, strings.TrimSpace(installOutput))
	}
	return path, nil
}

func resolveClaudeExecutable() (string, error) {
	if path := strings.TrimSpace(os.Getenv("AGEN8_CLAUDE_BIN")); path != "" {
		if info, err := os.Stat(path); err == nil && !info.IsDir() && info.Mode().Perm()&0o111 != 0 {
			return path, nil
		}
		return "", fmt.Errorf("AGEN8_CLAUDE_BIN %q is not executable", path)
	}
	home := strings.TrimSpace(os.Getenv("HOME"))
	for _, path := range cliCandidatePaths(home, "claude") {
		if info, err := os.Stat(path); err == nil && !info.IsDir() && info.Mode().Perm()&0o111 != 0 {
			return path, nil
		}
	}
	if shell := strings.TrimSpace(os.Getenv("SHELL")); shell != "" {
		output, err := exec.Command(shell, "-lc", "command -v claude").Output()
		if err == nil {
			path := strings.TrimSpace(string(output))
			if path != "" {
				return path, nil
			}
		}
	}
	if path, err := exec.LookPath("claude"); err == nil {
		return path, nil
	}
	return "", fmt.Errorf("claude binary was not found for bridge process")
}

func installClaudeExecutable(ctx context.Context) (string, error) {
	npmPath, err := resolveNPMExecutable()
	if err != nil {
		return "", err
	}
	home := strings.TrimSpace(os.Getenv("HOME"))
	if home == "" {
		return "", fmt.Errorf("HOME is required to install Claude Code")
	}
	prefix := filepath.Join(home, ".local")
	if err := os.MkdirAll(prefix, 0o755); err != nil {
		return "", fmt.Errorf("create claude install prefix: %w", err)
	}
	cmd := exec.CommandContext(ctx, npmPath, "install", "-g", "@anthropic-ai/claude-code", "--prefix", prefix)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("install Claude Code with npm: %w", err)
	}
	return string(output), nil
}

func resolveNPMExecutable() (string, error) {
	home := strings.TrimSpace(os.Getenv("HOME"))
	for _, path := range cliCandidatePaths(home, "npm") {
		if info, err := os.Stat(path); err == nil && !info.IsDir() && info.Mode().Perm()&0o111 != 0 {
			return path, nil
		}
	}
	if shell := strings.TrimSpace(os.Getenv("SHELL")); shell != "" {
		output, err := exec.Command(shell, "-lc", "command -v npm").Output()
		if err == nil {
			path := strings.TrimSpace(string(output))
			if path != "" {
				return path, nil
			}
		}
	}
	if path, err := exec.LookPath("npm"); err == nil {
		return path, nil
	}
	return "", fmt.Errorf("npm binary was not found for Claude Code auto-install")
}

func isClaudeCommand(command string) bool {
	base := strings.ToLower(strings.TrimSpace(filepath.Base(command)))
	return base == "claude" || base == "claude.cmd" || base == "claude.exe"
}

func isBundledEditorCodex(path string) bool {
	cleaned := filepath.ToSlash(strings.TrimSpace(path))
	return strings.Contains(cleaned, "/.cursor-server/extensions/") ||
		strings.Contains(cleaned, "/.vscode-server/extensions/")
}

func codexCandidatePaths(home string) []string {
	return cliCandidatePaths(home, "codex")
}

func cliCandidatePaths(home string, binary string) []string {
	binary = strings.TrimSpace(binary)
	paths := []string{}
	if home == "" {
		return []string{"/usr/local/bin/" + binary, "/opt/homebrew/bin/" + binary, "/snap/bin/" + binary}
	}
	patterns := []string{
		filepath.Join(home, ".local/bin", binary),
		filepath.Join(home, ".npm-global/bin", binary),
		filepath.Join(home, ".yarn/bin", binary),
		filepath.Join(home, ".bun/bin", binary),
		filepath.Join(home, ".volta/bin", binary),
		filepath.Join(home, ".asdf/shims", binary),
		filepath.Join(home, ".local/share/mise/shims", binary),
		filepath.Join(home, ".linuxbrew/bin", binary),
		filepath.Join(home, ".homebrew/bin", binary),
		filepath.Join(home, ".nvm/versions/node/*/bin", binary),
		filepath.Join(home, ".asdf/installs/nodejs/*/bin", binary),
		filepath.Join(home, ".local/share/mise/installs/node/*/bin", binary),
		filepath.Join(home, ".local/share/fnm/node-versions/*/installation/bin", binary),
	}
	for _, pattern := range patterns {
		matches, err := filepath.Glob(pattern)
		if err != nil || len(matches) == 0 {
			paths = append(paths, pattern)
			continue
		}
		paths = append(paths, matches...)
	}
	return append(paths, "/home/linuxbrew/.linuxbrew/bin/"+binary, "/usr/local/bin/"+binary, "/opt/homebrew/bin/"+binary, "/snap/bin/"+binary)
}

func proxyWebSocket(ctx context.Context, a, b *websocket.Conn, logger *slog.Logger) proxyCopyResult {
	done := make(chan proxyCopyResult, 2)
	go copyWebSocket(ctx, a, b, "client_to_backend", done)
	go copyWebSocket(ctx, b, a, "backend_to_client", done)
	result := <-done
	if logger != nil {
		logger.InfoContext(ctx, "bridge websocket proxy side stopped", "direction", result.direction, "error", result.err)
	}
	return result
}

func proxyWebSocketStdio(ctx context.Context, conn *websocket.Conn, stdin io.Writer, stdout io.Reader, logger *slog.Logger) proxyCopyResult {
	done := make(chan proxyCopyResult, 2)
	go copyWebSocketToWriter(ctx, stdin, conn, "client_to_stdin", done)
	go copyReaderToWebSocket(ctx, conn, stdout, "stdout_to_client", done)
	result := <-done
	if logger != nil {
		logger.InfoContext(ctx, "bridge stdio proxy side stopped", "direction", result.direction, "error", result.err)
	}
	return result
}

type proxyCopyResult struct {
	direction string
	err       error
}

func copyWebSocket(ctx context.Context, dst, src *websocket.Conn, direction string, done chan<- proxyCopyResult) {
	for {
		msgType, reader, err := src.Reader(ctx)
		if err != nil {
			done <- proxyCopyResult{direction: direction, err: err}
			return
		}
		writer, err := dst.Writer(ctx, msgType)
		if err != nil {
			done <- proxyCopyResult{direction: direction, err: err}
			return
		}
		_, copyErr := io.Copy(writer, reader)
		closeErr := writer.Close()
		if copyErr != nil || closeErr != nil {
			if copyErr != nil {
				done <- proxyCopyResult{direction: direction, err: copyErr}
			} else {
				done <- proxyCopyResult{direction: direction, err: closeErr}
			}
			return
		}
	}
}

func copyWebSocketToWriter(ctx context.Context, dst io.Writer, src *websocket.Conn, direction string, done chan<- proxyCopyResult) {
	for {
		msgType, reader, err := src.Reader(ctx)
		if err != nil {
			done <- proxyCopyResult{direction: direction, err: err}
			return
		}
		if msgType != websocket.MessageText {
			done <- proxyCopyResult{direction: direction, err: fmt.Errorf("non-text websocket message")}
			return
		}
		if _, err := io.Copy(dst, reader); err != nil {
			done <- proxyCopyResult{direction: direction, err: err}
			return
		}
		if _, err := dst.Write([]byte("\n")); err != nil {
			done <- proxyCopyResult{direction: direction, err: err}
			return
		}
	}
}

func copyReaderToWebSocket(ctx context.Context, dst *websocket.Conn, src io.Reader, direction string, done chan<- proxyCopyResult) {
	buffer := make([]byte, 64*1024)
	line := make([]byte, 0, 64*1024)
	for {
		n, err := src.Read(buffer)
		if n > 0 {
			chunk := buffer[:n]
			for len(chunk) > 0 {
				idx := bytes.IndexByte(chunk, '\n')
				if idx < 0 {
					line = append(line, chunk...)
					break
				}
				line = append(line, chunk[:idx]...)
				if len(bytes.TrimSpace(line)) > 0 {
					if err := dst.Write(ctx, websocket.MessageText, line); err != nil {
						done <- proxyCopyResult{direction: direction, err: err}
						return
					}
				}
				line = line[:0]
				chunk = chunk[idx+1:]
			}
		}
		if err != nil {
			done <- proxyCopyResult{direction: direction, err: err}
			return
		}
	}
}

func reservePort() (int, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, fmt.Errorf("reserve bridge port: %w", err)
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port, nil
}

func waitReady(ctx context.Context, url string) error {
	client := http.Client{Timeout: 250 * time.Millisecond}
	deadline := time.Now().Add(30 * time.Second)
	for {
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out waiting for %s", url)
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return err
		}
		resp, err := client.Do(req)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
}
