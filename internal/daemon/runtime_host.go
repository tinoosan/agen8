package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	harnessapp "github.com/tinoosan/agen8-mcp-server/internal/services/harness/app"
	locationdomain "github.com/tinoosan/agen8-mcp-server/internal/services/location/domain"
)

var codexAppServerListenPattern = regexp.MustCompile(`--listen(?:=|\s+)(ws://[^\s]+)`)

func (d *Daemon) ResolveRuntimeHost(ctx context.Context, input harnessapp.RuntimeHostRequest) (harnessapp.RuntimeHost, error) {
	locationID := strings.TrimSpace(input.LocationID)
	harnessKind := strings.ToLower(strings.TrimSpace(input.HarnessKind))
	if locationID == "" {
		return harnessapp.RuntimeHost{}, nil
	}
	if locationID == "local" {
		if harnessKind != "codex" {
			return harnessapp.RuntimeHost{}, nil
		}
		if appServerURL, err := findLocalCodexRemoteControlSocket(ctx); err != nil {
			return harnessapp.RuntimeHost{}, err
		} else if appServerURL != "" {
			d.mcpBinding.bindAppServerURL(input.SessionID, appServerURL)
			return harnessapp.RuntimeHost{
				AppServerURL: appServerURL,
				Diagnostics:  "local codex remote-control socket",
			}, nil
		}
		appServerURL := d.mcpBinding.appServerURL(input.SessionID)
		if appServerURL == "" {
			var err error
			appServerURL, err = findLocalCodexAppServerURL(ctx, input.MCPToken)
			if err != nil {
				return harnessapp.RuntimeHost{}, err
			}
			d.mcpBinding.bindAppServerURL(input.SessionID, appServerURL)
		}
		if appServerURL == "" {
			return harnessapp.RuntimeHost{}, nil
		}
		return harnessapp.RuntimeHost{
			AppServerURL: appServerURL,
			Diagnostics:  "local codex app-server discovered from active MCP connection",
		}, nil
	}
	if d == nil || d.app == nil || d.app.LocationSvc == nil {
		return harnessapp.RuntimeHost{}, fmt.Errorf("location service is required")
	}
	host, err := d.app.LocationSvc.EnsureBridge(ctx, locationdomain.ID(locationID))
	if err != nil {
		return harnessapp.RuntimeHost{}, err
	}
	ws := strings.TrimRight(strings.TrimSpace(host.WebSocketURL), "/")
	switch harnessKind {
	case "codex":
		ws += "/codex"
	case "claude-cli":
		ws += "/claude"
	}
	return harnessapp.RuntimeHost{
		AppServerURL: ws,
		MCPBaseURL:   strings.TrimSpace(host.MCPBaseURL),
		Diagnostics:  strings.TrimSpace(host.Diagnostics),
	}, nil
}

func findLocalCodexAppServerURL(ctx context.Context, token string) (string, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return "", nil
	}
	out, err := exec.CommandContext(ctx, "ps", "-axo", "pid=,command=").Output()
	if err != nil {
		return "", fmt.Errorf("list local codex app-server processes: %w", err)
	}
	type candidate struct {
		pid int
		url string
	}
	var candidates []candidate
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || !strings.Contains(line, "codex app-server") || !strings.Contains(line, "/mcp?token="+token) {
			continue
		}
		match := codexAppServerListenPattern.FindStringSubmatch(line)
		if len(match) != 2 {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		pid, err := strconv.Atoi(fields[0])
		if err != nil {
			continue
		}
		candidates = append(candidates, candidate{pid: pid, url: strings.TrimSpace(match[1])})
	}
	if len(candidates) == 0 {
		return "", nil
	}
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].pid > candidates[j].pid
	})
	return candidates[0].url, nil
}

func findLocalCodexRemoteControlSocket(ctx context.Context) (string, error) {
	socketPath := localCodexRemoteControlSocketPath()
	if socketPath == "" {
		return "", nil
	}
	if localCodexSocketExists(socketPath) {
		return "unix://" + socketPath, nil
	}
	codexPath := localCodexCLIPath()
	if codexPath == "" {
		return "", nil
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, codexPath, "remote-control", "start", "--json").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("start codex remote-control: %w: %s", err, strings.TrimSpace(string(out)))
	}
	var parsed struct {
		Daemon struct {
			SocketPath string `json:"socketPath"`
		} `json:"daemon"`
	}
	if err := json.Unmarshal(out, &parsed); err != nil {
		return "", fmt.Errorf("parse codex remote-control start output: %w: %s", err, strings.TrimSpace(string(out)))
	}
	if parsed.Daemon.SocketPath != "" {
		socketPath = strings.TrimSpace(parsed.Daemon.SocketPath)
	}
	if !localCodexSocketExists(socketPath) {
		return "", nil
	}
	return "unix://" + socketPath, nil
}

func localCodexRemoteControlSocketPath() string {
	codexHome := strings.TrimSpace(os.Getenv("CODEX_HOME"))
	if codexHome == "" {
		home, err := os.UserHomeDir()
		if err != nil || strings.TrimSpace(home) == "" {
			return ""
		}
		codexHome = filepath.Join(home, ".codex")
	}
	return filepath.Join(codexHome, "app-server-control", "app-server-control.sock")
}

func localCodexSocketExists(path string) bool {
	info, err := os.Stat(strings.TrimSpace(path))
	return err == nil && !info.IsDir()
}

func localCodexCLIPath() string {
	if path, err := exec.LookPath("codex"); err == nil && strings.TrimSpace(path) != "" {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return ""
	}
	path := filepath.Join(home, ".local", "bin", "codex")
	if info, err := os.Stat(path); err == nil && !info.IsDir() {
		return path
	}
	return ""
}
