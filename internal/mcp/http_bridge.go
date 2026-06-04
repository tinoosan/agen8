package mcp

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

const (
	EnvBridgeMCPURL          = "AGEN8_MCP_URL"
	EnvBridgeNativeSessionID = "AGEN8_NATIVE_SESSION_ID"
	EnvBridgeProjectRoot     = "AGEN8_BRIDGE_PROJECT_ROOT"
	EnvBridgeSessionDir      = "AGEN8_BRIDGE_SESSION_DIR"
	EnvBridgeEphemeral       = "AGEN8_BRIDGE_EPHEMERAL"
)

type HTTPBridgeOptions struct {
	MCPURL          string
	NativeSessionID string
	ProjectRoot     string
	Ephemeral       bool
	In              io.Reader
	Out             io.Writer
	ErrOut          io.Writer
	HTTPClient      *http.Client
}

func RunHTTPBridge(ctx context.Context, opts HTTPBridgeOptions) error {
	rawURL := strings.TrimSpace(opts.MCPURL)
	if rawURL == "" {
		rawURL = strings.TrimSpace(os.Getenv(EnvBridgeMCPURL))
	}
	if rawURL == "" {
		rawURL = "http://127.0.0.1:7777/mcp?token=agen8-local"
	}
	nativeSessionID := strings.TrimSpace(opts.NativeSessionID)
	if nativeSessionID == "" {
		nativeSessionID = strings.TrimSpace(os.Getenv(EnvBridgeNativeSessionID))
	}
	if nativeSessionID == "" {
		nativeSessionID = strings.TrimSpace(os.Getenv("CLAUDE_SESSION_ID"))
	}
	if nativeSessionID == "" {
		var err error
		nativeSessionID, err = bridgeSessionID(opts.ProjectRoot, opts.Ephemeral)
		if err != nil {
			return err
		}
	}
	in := opts.In
	if in == nil {
		in = os.Stdin
	}
	out := opts.Out
	if out == nil {
		out = os.Stdout
	}
	errOut := opts.ErrOut
	if errOut == nil {
		errOut = os.Stderr
	}
	client := opts.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 0, 64*1024), 16<<20)
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		if err := forwardBridgeMessage(ctx, client, rawURL, nativeSessionID, line, out, errOut); err != nil {
			writeBridgeRPCError(line, out, err)
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read mcp stdio: %w", err)
	}
	return nil
}

func forwardBridgeMessage(ctx context.Context, client *http.Client, rawURL string, nativeSessionID string, body []byte, out io.Writer, errOut io.Writer) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, rawURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Agen8-Native-Session-Id", nativeSessionID)
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("post daemon mcp: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusAccepted {
		return nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("daemon mcp status %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	contentType := strings.ToLower(resp.Header.Get("Content-Type"))
	if strings.Contains(contentType, "text/event-stream") {
		return copyBridgeSSE(resp.Body, out)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	data = bytes.TrimSpace(data)
	if len(data) == 0 {
		return nil
	}
	_, err = fmt.Fprintln(out, string(data))
	return err
}

func copyBridgeSSE(r io.Reader, out io.Writer) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 16<<20)
	var dataLines []string
	flush := func() error {
		if len(dataLines) == 0 {
			return nil
		}
		data := strings.TrimSpace(strings.Join(dataLines, "\n"))
		dataLines = dataLines[:0]
		if data == "" || data == "[DONE]" {
			return nil
		}
		_, err := fmt.Fprintln(out, data)
		return err
	}
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			if err := flush(); err != nil {
				return err
			}
			continue
		}
		if strings.HasPrefix(line, "data:") {
			dataLines = append(dataLines, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	return flush()
}

func writeBridgeRPCError(request []byte, out io.Writer, cause error) {
	id := json.RawMessage("null")
	var raw struct {
		ID json.RawMessage `json:"id"`
	}
	if err := json.Unmarshal(request, &raw); err == nil && len(bytes.TrimSpace(raw.ID)) > 0 {
		id = raw.ID
	}
	resp := map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"error": map[string]any{
			"code":    -32001,
			"message": cause.Error(),
		},
	}
	encoded, err := json.Marshal(resp)
	if err != nil {
		return
	}
	_, _ = fmt.Fprintln(out, string(encoded))
}

func newBridgeSessionID() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("generate bridge session id: %w", err)
	}
	return "bridge-" + hex.EncodeToString(raw[:]), nil
}

func bridgeSessionID(projectRoot string, ephemeral bool) (string, error) {
	if ephemeral || envTruthy(os.Getenv(EnvBridgeEphemeral)) {
		return newBridgeSessionID()
	}
	root, err := resolveBridgeProjectRoot(projectRoot)
	if err != nil {
		return "", err
	}
	cachePath, err := bridgeSessionCachePath(root)
	if err != nil {
		return "", err
	}
	if raw, err := os.ReadFile(cachePath); err == nil {
		if cached := strings.TrimSpace(string(raw)); cached != "" {
			return cached, nil
		}
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("read bridge session cache: %w", err)
	}
	sessionID, err := newBridgeSessionID()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(cachePath), 0o700); err != nil {
		return "", fmt.Errorf("create bridge session cache dir: %w", err)
	}
	if err := os.WriteFile(cachePath, []byte(sessionID+"\n"), 0o600); err != nil {
		return "", fmt.Errorf("write bridge session cache: %w", err)
	}
	return sessionID, nil
}

func resolveBridgeProjectRoot(projectRoot string) (string, error) {
	for _, candidate := range []string{
		projectRoot,
		os.Getenv(EnvBridgeProjectRoot),
		os.Getenv("CLAUDE_PROJECT_DIR"),
	} {
		if root := strings.TrimSpace(candidate); root != "" {
			abs, err := filepath.Abs(root)
			if err != nil {
				return "", fmt.Errorf("resolve bridge project root: %w", err)
			}
			return filepath.Clean(abs), nil
		}
	}
	root, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("resolve bridge project root: %w", err)
	}
	return filepath.Clean(root), nil
}

func bridgeSessionCachePath(projectRoot string) (string, error) {
	dir := strings.TrimSpace(os.Getenv(EnvBridgeSessionDir))
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home dir: %w", err)
		}
		dir = filepath.Join(home, ".agen8", "bridge-sessions")
	}
	hash := sha256.Sum256([]byte(projectRoot))
	return filepath.Join(dir, hex.EncodeToString(hash[:])+".uuid"), nil
}

func envTruthy(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "y", "on":
		return true
	default:
		return false
	}
}
