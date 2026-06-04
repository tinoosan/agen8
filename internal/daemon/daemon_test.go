package daemon

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tinoosan/agen8-mcp-server/internal/caller"
	"github.com/tinoosan/agen8-mcp-server/internal/config"
	"github.com/tinoosan/agen8-mcp-server/internal/eventbus"
	authapp "github.com/tinoosan/agen8-mcp-server/internal/services/auth/app"
	decisiondomain "github.com/tinoosan/agen8-mcp-server/internal/services/decision/domain"
	harnessdomain "github.com/tinoosan/agen8-mcp-server/internal/services/harness/domain"
	humaninputdomain "github.com/tinoosan/agen8-mcp-server/internal/services/humaninput/domain"
	"github.com/tinoosan/agen8-mcp-server/internal/services/message/domain/conversation"
	missionrpc "github.com/tinoosan/agen8-mcp-server/internal/services/mission/rpc"
	projectapp "github.com/tinoosan/agen8-mcp-server/internal/services/project/app"
	projectdomain "github.com/tinoosan/agen8-mcp-server/internal/services/project/domain/project"
	spacedomain "github.com/tinoosan/agen8-mcp-server/internal/services/space/domain"
	"github.com/tinoosan/agen8-mcp-server/internal/services/space/domain/member"
	spacerpc "github.com/tinoosan/agen8-mcp-server/internal/services/space/rpc"
	taskrpc "github.com/tinoosan/agen8-mcp-server/internal/services/task/rpc"
	userapp "github.com/tinoosan/agen8-mcp-server/internal/services/user/app"
	userdomain "github.com/tinoosan/agen8-mcp-server/internal/services/user/domain"
	userinfra "github.com/tinoosan/agen8-mcp-server/internal/services/user/infra"
	implstore "github.com/tinoosan/agen8-mcp-server/internal/store"
	"github.com/tinoosan/agen8-mcp-server/pkg/protocol"
	"github.com/tinoosan/agen8-mcp-server/pkg/types"
)

func TestHTTPStrategyHealthz(t *testing.T) {
	d := newTestDaemon(t)
	handler, err := d.httpHandler()
	require.NoError(t, err)
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	resp, err := http.Get(srv.URL + "/healthz")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestHTTPStrategyServesWebUI(t *testing.T) {
	d := newTestDaemon(t)
	_ = createSetupAdminAPIKey(t, d)
	handler, err := d.httpHandler()
	require.NoError(t, err)
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	resp, err := http.Get(srv.URL + "/")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Contains(t, string(body), `id="root"`)
}

func TestHTTPStrategyProxiesWebUIToViteWhenConfigured(t *testing.T) {
	vite := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/", r.URL.Path)
		require.Empty(t, r.Header.Get("Authorization"))
		require.Empty(t, r.Header.Get("Cookie"))
		_, _ = w.Write([]byte("vite dev ui"))
	}))
	t.Cleanup(vite.Close)
	t.Setenv(EnvDevWebURL, vite.URL)

	d := newTestDaemon(t)
	_ = createSetupAdminAPIKey(t, d)
	handler, err := d.httpHandler()
	require.NoError(t, err)
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	req, err := http.NewRequest(http.MethodGet, srv.URL+"/", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer ses_test")
	req.Header.Set("Cookie", strings.Repeat("large=value;", 256))
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, "vite dev ui", string(body))

	health, err := http.Get(srv.URL + "/healthz")
	require.NoError(t, err)
	defer health.Body.Close()
	require.Equal(t, http.StatusOK, health.StatusCode)
}

func TestHTTPStrategyRedirectsRootToSetupWhenRegistrationOpen(t *testing.T) {
	d := newTestDaemon(t)
	handler, err := d.httpHandler()
	require.NoError(t, err)
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	client := &http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := client.Get(srv.URL + "/")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusFound, resp.StatusCode)
	require.Equal(t, "/setup?token="+d.cfg.SetupToken, resp.Header.Get("Location"))
}

func TestHTTPStrategyStartupOutputOmitsSetupToken(t *testing.T) {
	d := newTestDaemon(t)
	var out bytes.Buffer
	d.cfg.Out = &out
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := ln.Addr().String()
	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- d.serveHTTP(ctx, ln)
	}()
	require.Eventually(t, func() bool {
		return strings.Contains(out.String(), "agen8 daemon listening")
	}, time.Second, 10*time.Millisecond)
	cancel()
	require.NoError(t, <-errCh)
	output := out.String()
	require.Contains(t, output, "agen8 setup: http://"+addr+"/ (setup token omitted from logs)")
	require.Contains(t, output, "agen8 daemon listening on http://"+addr)
	require.NotContains(t, output, d.cfg.SetupToken)
	require.NotContains(t, output, "token=")
}

func TestHTTPStrategyEventsUsesEventStream(t *testing.T) {
	d := newTestDaemon(t)
	handler, err := d.httpHandler()
	require.NoError(t, err)
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	resp, err := http.Get(srv.URL + "/events")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Contains(t, resp.Header.Get("Content-Type"), "text/event-stream")
	buf := make([]byte, len(": connected\n\n"))
	_, err = io.ReadFull(resp.Body, buf)
	require.NoError(t, err)
	require.Equal(t, ": connected\n\n", string(buf))
}

func TestHTTPStrategyEventsStreamsConversationChangeNotifications(t *testing.T) {
	d := newTestDaemon(t)
	handler, err := d.httpHandler()
	require.NoError(t, err)
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	resp, err := http.Get(srv.URL + "/events")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	reader := bufio.NewReader(resp.Body)
	line, err := reader.ReadString('\n')
	require.NoError(t, err)
	require.Equal(t, ": connected\n", line)
	line, err = reader.ReadString('\n')
	require.NoError(t, err)
	require.Equal(t, "\n", line)

	err = d.NotifyConversationChanged(context.Background(), conversation.Message{
		ID:         "conversation-1",
		ChannelID:  "channel:space-1:member:member-1",
		SpaceID:    "space-1",
		MemberID:   "member-1",
		Direction:  conversation.DirectionOutbound,
		SenderType: "harness",
		Text:       "hello",
		Render:     conversation.RenderVisible,
		CreatedAt:  time.Date(2026, 5, 16, 12, 0, 0, 0, time.UTC),
		UpdatedAt:  time.Date(2026, 5, 16, 12, 0, 1, 0, time.UTC),
	})
	require.NoError(t, err)
	line, err = reader.ReadString('\n')
	require.NoError(t, err)
	require.Contains(t, line, `"method":"event.append"`)
	require.Contains(t, line, `"messageId":"conversation-1"`)
}

func TestHTTPStrategyEventsStreamsHumanInputChangeNotifications(t *testing.T) {
	d := newTestDaemon(t)
	handler, err := d.httpHandler()
	require.NoError(t, err)
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	resp, err := http.Get(srv.URL + "/events")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	reader := bufio.NewReader(resp.Body)
	line, err := reader.ReadString('\n')
	require.NoError(t, err)
	require.Equal(t, ": connected\n", line)
	line, err = reader.ReadString('\n')
	require.NoError(t, err)
	require.Equal(t, "\n", line)

	err = d.NotifyHumanInputChanged(context.Background(), humaninputdomain.Request{
		ID:            "hi-1",
		ProjectID:     "project-1",
		SpaceID:       "space-1",
		AskerMemberID: "member-1",
		ChannelID:     "channel-1",
		ToolCallID:    "call-1",
		ToolName:      "decision",
		Status:        humaninputdomain.StatusPending,
	})
	require.NoError(t, err)
	line, err = reader.ReadString('\n')
	require.NoError(t, err)
	require.Contains(t, line, `"method":"channel.human_input.changed"`)
	require.Contains(t, line, `"channelId":"channel-1"`)
	require.Contains(t, line, `"toolCallId":"call-1"`)
	require.Contains(t, line, `"status":"pending"`)
}

func TestHTTPStrategySetupCreatesFirstAdminAndCannotRunTwice(t *testing.T) {
	d := newTestDaemon(t)
	handler, err := d.httpHandler()
	require.NoError(t, err)
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	payload := map[string]string{
		"token":    d.cfg.SetupToken,
		"email":    "admin@example.com",
		"name":     "Admin",
		"password": "password123",
	}
	body, err := json.Marshal(payload)
	require.NoError(t, err)
	resp, err := http.Post(srv.URL+"/setup", "application/json", bytes.NewReader(body))
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var created struct {
		User struct {
			Role string `json:"role"`
		} `json:"user"`
		APIKey struct {
			Secret string `json:"secret"`
		} `json:"apiKey"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&created))
	require.Equal(t, "admin", created.User.Role)
	require.NotEmpty(t, created.APIKey.Secret)

	resp, err = http.Get(srv.URL + "/setup?token=" + d.cfg.SetupToken)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusNotFound, resp.StatusCode)

	resp, err = http.Post(srv.URL+"/setup", "application/json", bytes.NewReader(body))
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusConflict, resp.StatusCode)
}

func TestHTTPStrategySetupPageExplainsFirstRun(t *testing.T) {
	d := newTestDaemon(t)
	handler, err := d.httpHandler()
	require.NoError(t, err)
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	resp, err := http.Get(srv.URL + "/setup?token=" + d.cfg.SetupToken)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	html := string(body)
	require.Contains(t, html, "Welcome to agen8")
	require.Contains(t, html, "Bring your agents into one place")
	require.Contains(t, html, "Create your account")
	require.Contains(t, html, "--bg-app: #1a1a1c")
	require.Contains(t, html, `name="token" value="`+d.cfg.SetupToken+`"`)
}

func TestHTTPStrategySetupFormStoresAPIKey(t *testing.T) {
	d := newTestDaemon(t)
	handler, err := d.httpHandler()
	require.NoError(t, err)
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	form := "token=" + d.cfg.SetupToken + "&email=admin%40example.com&name=Admin&password=password123"
	resp, err := http.Post(srv.URL+"/setup", "application/x-www-form-urlencoded", strings.NewReader(form))
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Contains(t, string(body), `localStorage.setItem("agen8.sessionToken"`)
	require.Contains(t, string(body), `location.href = "/"`)
}

func TestHTTPStrategyRPCRequiresBearerAfterSetup(t *testing.T) {
	d := newTestDaemon(t)
	apiKey := createSetupAdminAPIKey(t, d)
	handler, err := d.httpHandler()
	require.NoError(t, err)
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	reqBody := []byte(`{"jsonrpc":"2.0","id":"1","method":"task.list","params":{}}`)
	resp, err := http.Post(srv.URL+"/rpc", "application/json", bytes.NewReader(reqBody))
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)

	req, err := http.NewRequest(http.MethodPost, srv.URL+"/rpc", bytes.NewReader(reqBody))
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var rpcResp struct {
		Result taskrpc.TaskListResult `json:"result"`
		Error  any                    `json:"error,omitempty"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&rpcResp))
	require.Nil(t, rpcResp.Error)
	require.Empty(t, rpcResp.Result.Tasks)
}

func TestHTTPStrategyMCPRegisterRequiresBearer(t *testing.T) {
	d := newTestDaemon(t)
	_ = createSetupAdminAPIKey(t, d)
	handler, err := d.httpHandler()
	require.NoError(t, err)
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	resp, err := http.Post(srv.URL+"/mcp/register", "application/json", bytes.NewReader([]byte(`{}`)))
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestHTTPStrategyMCPRegisterProvisionsExternalHarness(t *testing.T) {
	d := newTestDaemon(t)
	apiKey := createSetupAdminAPIKey(t, d)
	handler, err := d.httpHandler()
	require.NoError(t, err)
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	projectRoot := filepath.Join(t.TempDir(), "agen8-mcp-server")
	require.NoError(t, os.MkdirAll(projectRoot, 0o755))
	payload, err := json.Marshal(map[string]string{
		"projectRoot": projectRoot,
		"harnessKind": "codex",
		"model":       "gpt-5.5",
		"effort":      "high",
	})
	require.NoError(t, err)
	req, err := http.NewRequest(http.MethodPost, srv.URL+"/mcp/register", bytes.NewReader(payload))
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var registered struct {
		ProjectID   string   `json:"projectId"`
		ProjectRoot string   `json:"projectRoot"`
		SpaceID     string   `json:"spaceId"`
		MemberID    string   `json:"memberId"`
		ChannelID   string   `json:"channelId"`
		Token       string   `json:"token"`
		URL         string   `json:"url"`
		MCPServers  []string `json:"mcpServers"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&registered))
	require.NotEmpty(t, registered.ProjectID)
	require.Equal(t, projectRoot, registered.ProjectRoot)
	require.NotEmpty(t, registered.SpaceID)
	require.NotEmpty(t, registered.MemberID)
	require.Contains(t, registered.ChannelID, registered.MemberID)
	require.Equal(t, "agen8-local", registered.Token)
	require.Contains(t, registered.URL, "/mcp?token="+registered.Token)
	require.NotEmpty(t, registered.MCPServers)
	require.Contains(t, registered.MCPServers[0], "/mcp?token="+registered.Token)

	active, err := d.app.HarnessSvc.GetActiveSession(context.Background(), registered.MemberID)
	require.NoError(t, err)
	require.NotNil(t, active)
	require.Equal(t, registered.ProjectID, active.ProjectID)
	require.Equal(t, registered.SpaceID, active.SpaceID)

	mcpReq, err := newStatefulMCPRequest(http.MethodPost, srv.URL+"/mcp?token="+registered.Token, bytes.NewReader([]byte(`{
		"jsonrpc": "2.0",
		"id": "1",
		"method": "tools/list",
		"params": {}
	}`)))
	require.NoError(t, err)
	mcpReq.Header.Set("Content-Type", "application/json")
	mcpReq.Header.Set("Accept", "application/json, text/event-stream")
	mcpResp, err := http.DefaultClient.Do(mcpReq)
	require.NoError(t, err)
	defer mcpResp.Body.Close()
	require.Equal(t, http.StatusOK, mcpResp.StatusCode)

	var rpcResp struct {
		Result struct {
			Tools []struct {
				Name string `json:"name"`
			} `json:"tools"`
		} `json:"result"`
		Error any `json:"error,omitempty"`
	}
	require.NoError(t, json.NewDecoder(mcpResp.Body).Decode(&rpcResp))
	require.Nil(t, rpcResp.Error)
	var names []string
	for _, tool := range rpcResp.Result.Tools {
		names = append(names, tool.Name)
	}
	require.Contains(t, names, "space")
	require.Contains(t, names, "task")
}

func TestHTTPStrategyBootstrapMCPTokenAllowsSetupOnlyBeforeRegister(t *testing.T) {
	d := newTestDaemon(t)
	handler, err := d.httpHandler()
	require.NoError(t, err)
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	listReq, err := newStatefulMCPRequest(http.MethodPost, srv.URL+"/mcp?token=agen8-local", bytes.NewReader([]byte(`{
		"jsonrpc": "2.0",
		"id": "tools",
		"method": "tools/list",
		"params": {}
	}`)))
	require.NoError(t, err)
	listReq.Header.Set("Content-Type", "application/json")
	listReq.Header.Set("Accept", "application/json, text/event-stream")
	listResp, err := http.DefaultClient.Do(listReq)
	require.NoError(t, err)
	defer listResp.Body.Close()
	require.Equal(t, http.StatusOK, listResp.StatusCode)
	var listRPC struct {
		Result struct {
			Tools []struct {
				Name string `json:"name"`
			} `json:"tools"`
		} `json:"result"`
		Error any `json:"error,omitempty"`
	}
	require.NoError(t, json.NewDecoder(listResp.Body).Decode(&listRPC))
	require.Nil(t, listRPC.Error)
	var names []string
	for _, tool := range listRPC.Result.Tools {
		names = append(names, tool.Name)
	}
	require.Contains(t, names, "space")
	require.Contains(t, names, "task")

	spaceListReq, err := newStatefulMCPRequest(http.MethodPost, srv.URL+"/mcp?token=agen8-local", bytes.NewReader([]byte(`{
		"jsonrpc": "2.0",
		"id": "space-list",
		"method": "tools/call",
		"params": {"name":"space","arguments":{"action":"list"}}
	}`)))
	require.NoError(t, err)
	spaceListReq.Header.Set("Content-Type", "application/json")
	spaceListReq.Header.Set("Accept", "application/json, text/event-stream")
	spaceListResp, err := http.DefaultClient.Do(spaceListReq)
	require.NoError(t, err)
	defer spaceListResp.Body.Close()
	require.Equal(t, http.StatusOK, spaceListResp.StatusCode)
	var spaceListRPC struct {
		Result struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
			IsError bool `json:"isError"`
		} `json:"result"`
		Error any `json:"error,omitempty"`
	}
	require.NoError(t, json.NewDecoder(spaceListResp.Body).Decode(&spaceListRPC))
	require.Nil(t, spaceListRPC.Error)
	require.False(t, spaceListRPC.Result.IsError)

	taskReq, err := newStatefulMCPRequest(http.MethodPost, srv.URL+"/mcp?token=agen8-local", bytes.NewReader([]byte(`{
		"jsonrpc": "2.0",
		"id": "task-list",
		"method": "tools/call",
		"params": {"name":"task","arguments":{"action":"list"}}
	}`)))
	require.NoError(t, err)
	taskReq.Header.Set("Content-Type", "application/json")
	taskReq.Header.Set("Accept", "application/json, text/event-stream")
	taskResp, err := http.DefaultClient.Do(taskReq)
	require.NoError(t, err)
	defer taskResp.Body.Close()
	require.Equal(t, http.StatusOK, taskResp.StatusCode)
	var taskRPC struct {
		Result struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
			IsError bool `json:"isError"`
		} `json:"result"`
		Error any `json:"error,omitempty"`
	}
	require.NoError(t, json.NewDecoder(taskResp.Body).Decode(&taskRPC))
	require.Nil(t, taskRPC.Error)
	require.True(t, taskRPC.Result.IsError)
	require.Contains(t, taskRPC.Result.Content[0].Text, "mcp session is not registered; call space.register first")
}

func TestHTTPStrategyBootstrapMCPTokenRegistersContextFromTool(t *testing.T) {
	d := newTestDaemon(t)
	apiKey := createSetupAdminAPIKey(t, d)
	handler, err := d.httpHandler()
	require.NoError(t, err)
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	projectRoot := filepath.Join(t.TempDir(), "agen8-mcp-server")
	require.NoError(t, os.MkdirAll(projectRoot, 0o755))

	bootstrapListReq, err := newStatefulMCPRequest(http.MethodPost, srv.URL+"/mcp?token=agen8-local", bytes.NewReader([]byte(`{
		"jsonrpc": "2.0",
		"id": "bootstrap-list",
		"method": "tools/list",
		"params": {}
	}`)))
	require.NoError(t, err)
	bootstrapListReq.Header.Set("Content-Type", "application/json")
	bootstrapListReq.Header.Set("Accept", "application/json, text/event-stream")
	bootstrapListResp, err := http.DefaultClient.Do(bootstrapListReq)
	require.NoError(t, err)
	defer bootstrapListResp.Body.Close()
	require.Equal(t, http.StatusOK, bootstrapListResp.StatusCode)
	var bootstrapListRPC struct {
		Result struct {
			Tools []struct {
				Name string `json:"name"`
			} `json:"tools"`
		} `json:"result"`
		Error any `json:"error,omitempty"`
	}
	require.NoError(t, json.NewDecoder(bootstrapListResp.Body).Decode(&bootstrapListRPC))
	require.Nil(t, bootstrapListRPC.Error)
	var bootstrapToolNames []string
	for _, tool := range bootstrapListRPC.Result.Tools {
		bootstrapToolNames = append(bootstrapToolNames, tool.Name)
	}
	require.Contains(t, bootstrapToolNames, "space")
	require.Contains(t, bootstrapToolNames, "task")

	bootstrapSpaceListReq, err := newStatefulMCPRequest(http.MethodPost, srv.URL+"/mcp?token=agen8-local", bytes.NewReader([]byte(`{
		"jsonrpc": "2.0",
		"id": "bootstrap-space-list",
		"method": "tools/call",
		"params": {"name":"space","arguments":{"action":"list"}}
	}`)))
	require.NoError(t, err)
	bootstrapSpaceListReq.Header.Set("Content-Type", "application/json")
	bootstrapSpaceListReq.Header.Set("Accept", "application/json, text/event-stream")
	bootstrapSpaceListResp, err := http.DefaultClient.Do(bootstrapSpaceListReq)
	require.NoError(t, err)
	defer bootstrapSpaceListResp.Body.Close()
	require.Equal(t, http.StatusOK, bootstrapSpaceListResp.StatusCode)
	var bootstrapSpaceListRPC struct {
		Result struct {
			IsError bool `json:"isError"`
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"result"`
		Error any `json:"error,omitempty"`
	}
	require.NoError(t, json.NewDecoder(bootstrapSpaceListResp.Body).Decode(&bootstrapSpaceListRPC))
	require.Nil(t, bootstrapSpaceListRPC.Error)
	require.False(t, bootstrapSpaceListRPC.Result.IsError)

	registerReq, err := newStatefulMCPRequest(http.MethodPost, srv.URL+"/mcp?token=agen8-local", bytes.NewReader([]byte(fmt.Sprintf(`{
		"jsonrpc": "2.0",
		"id": "1",
		"method": "tools/call",
		"params": {
			"name": "space",
			"arguments": {"action":"register","project_root":%s}
		}
	}`, quoteJSON(projectRoot)))))
	require.NoError(t, err)
	registerReq.Header.Set("Content-Type", "application/json")
	registerReq.Header.Set("Accept", "application/json, text/event-stream")
	registerReq.Header.Set("Agen8-Native-Session-Id", "mcp-protocol-session-1")
	registerResp, err := http.DefaultClient.Do(registerReq)
	require.NoError(t, err)
	defer registerResp.Body.Close()
	require.Equal(t, http.StatusOK, registerResp.StatusCode)

	var rpcResp struct {
		Result struct {
			StructuredContent struct {
				ProjectID  string `json:"projectId"`
				SpaceID    string `json:"spaceId"`
				MemberID   string `json:"memberId"`
				MemberType string `json:"memberType"`
				ChannelID  string `json:"channelId"`
				Token      string `json:"token"`
			} `json:"structuredContent"`
		} `json:"result"`
		Error any `json:"error,omitempty"`
	}
	require.NoError(t, json.NewDecoder(registerResp.Body).Decode(&rpcResp))
	require.Nil(t, rpcResp.Error)
	require.NotEmpty(t, rpcResp.Result.StructuredContent.ProjectID)
	require.NotEmpty(t, rpcResp.Result.StructuredContent.SpaceID)
	require.NotEmpty(t, rpcResp.Result.StructuredContent.MemberID)
	require.Equal(t, "coordinator", rpcResp.Result.StructuredContent.MemberType)
	require.NotEmpty(t, rpcResp.Result.StructuredContent.ChannelID)
	require.Equal(t, "agen8-local", rpcResp.Result.StructuredContent.Token)

	active, err := d.app.HarnessSvc.GetActiveSession(context.Background(), rpcResp.Result.StructuredContent.MemberID)
	require.NoError(t, err)
	require.NotNil(t, active)
	require.Equal(t, rpcResp.Result.StructuredContent.Token, active.MCPToken)

	projectSpacesReq, err := http.NewRequest(http.MethodPost, srv.URL+"/rpc", bytes.NewReader([]byte(fmt.Sprintf(`{
		"jsonrpc": "2.0",
		"id": "project-spaces",
		"method": "project.space.list",
		"params": {"projectId": %s}
	}`, quoteJSON(rpcResp.Result.StructuredContent.ProjectID)))))
	require.NoError(t, err)
	projectSpacesReq.Header.Set("Authorization", "Bearer "+apiKey)
	projectSpacesReq.Header.Set("Content-Type", "application/json")
	projectSpacesResp, err := http.DefaultClient.Do(projectSpacesReq)
	require.NoError(t, err)
	defer projectSpacesResp.Body.Close()
	require.Equal(t, http.StatusOK, projectSpacesResp.StatusCode)
	var projectSpacesRPC struct {
		Result struct {
			Spaces []struct {
				SpaceID string `json:"spaceId"`
			} `json:"spaces"`
		} `json:"result"`
		Error any `json:"error,omitempty"`
	}
	require.NoError(t, json.NewDecoder(projectSpacesResp.Body).Decode(&projectSpacesRPC))
	require.Nil(t, projectSpacesRPC.Error)
	require.Len(t, projectSpacesRPC.Result.Spaces, 1)
	require.Equal(t, rpcResp.Result.StructuredContent.SpaceID, projectSpacesRPC.Result.Spaces[0].SpaceID)

	listReq, err := newStatefulMCPRequest(http.MethodPost, srv.URL+"/mcp?token="+rpcResp.Result.StructuredContent.Token, bytes.NewReader([]byte(fmt.Sprintf(`{
		"jsonrpc": "2.0",
		"id": "2",
		"method": "tools/call",
		"params": {"name":"space","arguments":{"action":"list","project_id":%s}}
	}`, quoteJSON(rpcResp.Result.StructuredContent.ProjectID)))))
	require.NoError(t, err)
	listReq.Header.Set("Content-Type", "application/json")
	listReq.Header.Set("Accept", "application/json, text/event-stream")
	listResp, err := http.DefaultClient.Do(listReq)
	require.NoError(t, err)
	defer listResp.Body.Close()
	require.Equal(t, http.StatusOK, listResp.StatusCode)

	var listRPC struct {
		Result struct {
			StructuredContent struct {
				ProjectID string `json:"projectId"`
				Count     int    `json:"count"`
			} `json:"structuredContent"`
		} `json:"result"`
		Error any `json:"error,omitempty"`
	}
	require.NoError(t, json.NewDecoder(listResp.Body).Decode(&listRPC))
	require.Nil(t, listRPC.Error)
	require.Equal(t, rpcResp.Result.StructuredContent.ProjectID, listRPC.Result.StructuredContent.ProjectID)
	require.Equal(t, 1, listRPC.Result.StructuredContent.Count)

	taskReq, err := newStatefulMCPRequest(http.MethodPost, srv.URL+"/mcp?token="+rpcResp.Result.StructuredContent.Token, bytes.NewReader([]byte(`{
		"jsonrpc": "2.0",
		"id": "task-list-after-register",
		"method": "tools/call",
		"params": {"name":"task","arguments":{"action":"list","limit":5}}
	}`)))
	require.NoError(t, err)
	taskReq.Header.Set("Content-Type", "application/json")
	taskReq.Header.Set("Accept", "application/json, text/event-stream")
	taskResp, err := http.DefaultClient.Do(taskReq)
	require.NoError(t, err)
	defer taskResp.Body.Close()
	require.Equal(t, http.StatusOK, taskResp.StatusCode)
	var taskRPC struct {
		Result struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
			IsError bool `json:"isError"`
		} `json:"result"`
		Error any `json:"error,omitempty"`
	}
	require.NoError(t, json.NewDecoder(taskResp.Body).Decode(&taskRPC))
	require.Nil(t, taskRPC.Error)
	require.False(t, taskRPC.Result.IsError, "task list should resolve registered token binding, got %+v", taskRPC.Result.Content)

	registerAgainReq, err := newStatefulMCPRequest(http.MethodPost, srv.URL+"/mcp?token=agen8-local", bytes.NewReader([]byte(fmt.Sprintf(`{
		"jsonrpc": "2.0",
		"id": "3",
		"method": "tools/call",
		"params": {
			"name": "space",
			"arguments": {"action":"register","project_root":%s}
		}
	}`, quoteJSON(projectRoot)))))
	require.NoError(t, err)
	registerAgainReq.Header.Set("Content-Type", "application/json")
	registerAgainReq.Header.Set("Accept", "application/json, text/event-stream")
	registerAgainResp, err := http.DefaultClient.Do(registerAgainReq)
	require.NoError(t, err)
	defer registerAgainResp.Body.Close()
	require.Equal(t, http.StatusOK, registerAgainResp.StatusCode)
	var registerAgainRPC struct {
		Result struct {
			StructuredContent struct {
				MemberID   string `json:"memberId"`
				MemberType string `json:"memberType"`
			} `json:"structuredContent"`
		} `json:"result"`
		Error any `json:"error,omitempty"`
	}
	require.NoError(t, json.NewDecoder(registerAgainResp.Body).Decode(&registerAgainRPC))
	require.Nil(t, registerAgainRPC.Error)
	require.Equal(t, rpcResp.Result.StructuredContent.MemberID, registerAgainRPC.Result.StructuredContent.MemberID)
	require.Equal(t, "coordinator", registerAgainRPC.Result.StructuredContent.MemberType)

	threadReq, err := newStatefulMCPRequest(http.MethodPost, srv.URL+"/mcp?token=agen8-local", bytes.NewReader([]byte(fmt.Sprintf(`{
			"jsonrpc": "2.0",
			"id": "4",
			"method": "tools/call",
			"params": {
				"name": "space",
				"arguments": {"action":"register","project_root":%s,"thread_id":"thread-2"}
			}
		}`, quoteJSON(projectRoot)))))
	require.NoError(t, err)
	threadReq.Header.Set("Content-Type", "application/json")
	threadReq.Header.Set("Accept", "application/json, text/event-stream")
	threadResp, err := http.DefaultClient.Do(threadReq)
	require.NoError(t, err)
	defer threadResp.Body.Close()
	require.Equal(t, http.StatusOK, threadResp.StatusCode)
	var threadRPC struct {
		Result struct {
			StructuredContent struct {
				MemberID   string `json:"memberId"`
				MemberType string `json:"memberType"`
			} `json:"structuredContent"`
		} `json:"result"`
		Error any `json:"error,omitempty"`
	}
	require.NoError(t, json.NewDecoder(threadResp.Body).Decode(&threadRPC))
	require.Nil(t, threadRPC.Error)
	require.NotEqual(t, rpcResp.Result.StructuredContent.MemberID, threadRPC.Result.StructuredContent.MemberID)
	require.Equal(t, "worker", threadRPC.Result.StructuredContent.MemberType)
}

func TestHTTPStrategyBootstrapMCPTokenRegistersContextFromCodexMetadata(t *testing.T) {
	d := newTestDaemon(t)
	handler, err := d.httpHandler()
	require.NoError(t, err)
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	projectRoot := filepath.Join(t.TempDir(), "agen8-mcp-server")
	require.NoError(t, os.MkdirAll(projectRoot, 0o755))
	registerReq, err := newStatefulMCPRequest(http.MethodPost, srv.URL+"/mcp?token=agen8-local", bytes.NewReader([]byte(fmt.Sprintf(`{
		"jsonrpc": "2.0",
		"id": "register-codex-meta",
		"method": "tools/call",
		"params": {
			"name": "space",
			"_meta": {
				"progressToken": 7,
				"x-codex-turn-metadata": {
					"session_id": "codex-session-1",
					"thread_id": "codex-thread-1",
					"turn_id": "codex-turn-1"
				}
			},
			"arguments": {"action":"register","project_root":%s,"thread_id":"stale-argument-thread"}
		}
	}`, quoteJSON(projectRoot)))))
	require.NoError(t, err)
	registerReq.Header.Set("Content-Type", "application/json")
	registerReq.Header.Set("Accept", "application/json, text/event-stream")
	registerReq.Header.Set("Agen8-Native-Session-Id", "mcp-protocol-session-1")
	registerResp, err := http.DefaultClient.Do(registerReq)
	require.NoError(t, err)
	defer registerResp.Body.Close()
	require.Equal(t, http.StatusOK, registerResp.StatusCode)

	var rpcResp struct {
		Result struct {
			StructuredContent struct {
				MemberID   string `json:"memberId"`
				MemberType string `json:"memberType"`
			} `json:"structuredContent"`
		} `json:"result"`
		Error any `json:"error,omitempty"`
	}
	require.NoError(t, json.NewDecoder(registerResp.Body).Decode(&rpcResp))
	require.Nil(t, rpcResp.Error)
	require.NotEmpty(t, rpcResp.Result.StructuredContent.MemberID)
	require.Equal(t, "coordinator", rpcResp.Result.StructuredContent.MemberType)

	active, err := d.app.HarnessSvc.GetActiveSession(context.Background(), rpcResp.Result.StructuredContent.MemberID)
	require.NoError(t, err)
	require.NotNil(t, active)
	require.Equal(t, "codex-thread-1", active.Ref)
	require.Equal(t, "agen8-local", active.MCPToken)
	turn := d.mcpBinding.activeCodexTurn(active.ID)
	require.Equal(t, "codex-thread-1", turn.threadID)
	require.Equal(t, "codex-turn-1", turn.turnID)
	turn = d.mcpBinding.activeCodexTurnForRef("codex-thread-1")
	require.Equal(t, "codex-thread-1", turn.threadID)
	require.Equal(t, "codex-turn-1", turn.turnID)
	require.False(t, d.app.MessageSvc.AgentDeliveryRunning(member.ID(rpcResp.Result.StructuredContent.MemberID)))

	registerAgainReq, err := newStatefulMCPRequest(http.MethodPost, srv.URL+"/mcp?token=agen8-local", bytes.NewReader([]byte(fmt.Sprintf(`{
		"jsonrpc": "2.0",
		"id": "register-codex-meta-again",
		"method": "tools/call",
		"params": {
			"name": "space",
			"_meta": {
				"threadId": "codex-thread-1",
				"x-codex-turn-metadata": {
					"session_id": "codex-session-1",
					"thread_id": "ignored-nested-thread"
				}
			},
			"arguments": {"action":"register","project_root":%s}
		}
	}`, quoteJSON(projectRoot)))))
	require.NoError(t, err)
	registerAgainReq.Header.Set("Content-Type", "application/json")
	registerAgainReq.Header.Set("Accept", "application/json, text/event-stream")
	registerAgainResp, err := http.DefaultClient.Do(registerAgainReq)
	require.NoError(t, err)
	defer registerAgainResp.Body.Close()
	require.Equal(t, http.StatusOK, registerAgainResp.StatusCode)

	var registerAgainRPC struct {
		Result struct {
			StructuredContent struct {
				MemberID string `json:"memberId"`
			} `json:"structuredContent"`
		} `json:"result"`
		Error any `json:"error,omitempty"`
	}
	require.NoError(t, json.NewDecoder(registerAgainResp.Body).Decode(&registerAgainRPC))
	require.Nil(t, registerAgainRPC.Error)
	require.Equal(t, rpcResp.Result.StructuredContent.MemberID, registerAgainRPC.Result.StructuredContent.MemberID)
}

func TestHTTPStrategyBootstrapMCPTokenResolvesClaudeSessionHeader(t *testing.T) {
	d := newTestDaemon(t)
	handler, err := d.httpHandler()
	require.NoError(t, err)
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	projectRoot := filepath.Join(t.TempDir(), "agen8-mcp-server")
	require.NoError(t, os.MkdirAll(projectRoot, 0o755))
	registerReq, err := newStatefulMCPRequest(http.MethodPost, srv.URL+"/mcp?token=agen8-local", bytes.NewReader([]byte(fmt.Sprintf(`{
		"jsonrpc": "2.0",
		"id": "register-claude-header",
		"method": "tools/call",
		"params": {
			"name": "space",
			"arguments": {"action":"register","project_root":%s,"harness_kind":"claude-cli"}
		}
	}`, quoteJSON(projectRoot)))))
	require.NoError(t, err)
	registerReq.Header.Set("Content-Type", "application/json")
	registerReq.Header.Set("Accept", "application/json, text/event-stream")
	registerReq.Header.Set("Agen8-Native-Session-Id", "claude-session-1")
	registerResp, err := http.DefaultClient.Do(registerReq)
	require.NoError(t, err)
	defer registerResp.Body.Close()
	require.Equal(t, http.StatusOK, registerResp.StatusCode)

	var registerRPC struct {
		Result struct {
			StructuredContent struct {
				MemberID         string `json:"memberId"`
				NativeSessionRef string `json:"nativeSessionRef"`
			} `json:"structuredContent"`
		} `json:"result"`
		Error any `json:"error,omitempty"`
	}
	require.NoError(t, json.NewDecoder(registerResp.Body).Decode(&registerRPC))
	require.Nil(t, registerRPC.Error)
	require.NotEmpty(t, registerRPC.Result.StructuredContent.MemberID)
	require.Equal(t, "claude-session-1", registerRPC.Result.StructuredContent.NativeSessionRef)

	memberListReq, err := newStatefulMCPRequest(http.MethodPost, srv.URL+"/mcp?token=agen8-local", bytes.NewReader([]byte(`{
		"jsonrpc": "2.0",
		"id": "claude-member-list",
		"method": "tools/call",
		"params": {"name":"space","arguments":{"action":"member_list"}}
	}`)))
	require.NoError(t, err)
	memberListReq.Header.Set("Content-Type", "application/json")
	memberListReq.Header.Set("Accept", "application/json, text/event-stream")
	memberListReq.Header.Set("Agen8-Native-Session-Id", "claude-session-1")
	memberListResp, err := http.DefaultClient.Do(memberListReq)
	require.NoError(t, err)
	defer memberListResp.Body.Close()
	require.Equal(t, http.StatusOK, memberListResp.StatusCode)

	var memberListRPC struct {
		Result struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
			IsError           bool `json:"isError"`
			StructuredContent struct {
				Count int `json:"count"`
			} `json:"structuredContent"`
		} `json:"result"`
		Error any `json:"error,omitempty"`
	}
	require.NoError(t, json.NewDecoder(memberListResp.Body).Decode(&memberListRPC))
	require.Nil(t, memberListRPC.Error)
	require.False(t, memberListRPC.Result.IsError, "member_list returned error content: %+v", memberListRPC.Result.Content)
	require.GreaterOrEqual(t, memberListRPC.Result.StructuredContent.Count, 1)
}

func TestHTTPStrategyBootstrapMCPTokenRejectsAmbiguousSharedTokenWithoutNativeRef(t *testing.T) {
	d := newTestDaemon(t)
	handler, err := d.httpHandler()
	require.NoError(t, err)
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	projectRoot := filepath.Join(t.TempDir(), "agen8-mcp-server")
	require.NoError(t, os.MkdirAll(projectRoot, 0o755))

	registerCodexReq, err := newStatefulMCPRequest(http.MethodPost, srv.URL+"/mcp?token=agen8-local", bytes.NewReader([]byte(fmt.Sprintf(`{
		"jsonrpc": "2.0",
		"id": "register-codex-shared-token",
		"method": "tools/call",
		"params": {
			"name": "space",
			"_meta": {"x-codex-turn-metadata": {"thread_id": "codex-thread-shared"}},
			"arguments": {"action":"register","project_root":%s,"harness_kind":"codex"}
		}
	}`, quoteJSON(projectRoot)))))
	require.NoError(t, err)
	registerCodexReq.Header.Set("Content-Type", "application/json")
	registerCodexReq.Header.Set("Accept", "application/json, text/event-stream")
	registerCodexResp, err := http.DefaultClient.Do(registerCodexReq)
	require.NoError(t, err)
	defer registerCodexResp.Body.Close()
	require.Equal(t, http.StatusOK, registerCodexResp.StatusCode)

	registerClaudeReq, err := newStatefulMCPRequest(http.MethodPost, srv.URL+"/mcp?token=agen8-local", bytes.NewReader([]byte(fmt.Sprintf(`{
		"jsonrpc": "2.0",
		"id": "register-claude-shared-token",
		"method": "tools/call",
		"params": {
			"name": "space",
			"arguments": {"action":"register","project_root":%s,"harness_kind":"claude-cli"}
		}
	}`, quoteJSON(projectRoot)))))
	require.NoError(t, err)
	registerClaudeReq.Header.Set("Content-Type", "application/json")
	registerClaudeReq.Header.Set("Accept", "application/json, text/event-stream")
	registerClaudeReq.Header.Set("Agen8-Native-Session-Id", "claude-session-shared")
	registerClaudeResp, err := http.DefaultClient.Do(registerClaudeReq)
	require.NoError(t, err)
	defer registerClaudeResp.Body.Close()
	require.Equal(t, http.StatusOK, registerClaudeResp.StatusCode)

	ambiguousReq, err := newStatefulMCPRequest(http.MethodPost, srv.URL+"/mcp?token=agen8-local", bytes.NewReader([]byte(`{
		"jsonrpc": "2.0",
		"id": "message-without-native-ref",
		"method": "tools/call",
		"params": {"name":"message","arguments":{"action":"send","destination_member_id":"member-does-not-matter","kind":"inform","body":"should not inherit codex"}}
	}`)))
	require.NoError(t, err)
	ambiguousReq.Header.Set("Content-Type", "application/json")
	ambiguousReq.Header.Set("Accept", "application/json, text/event-stream")
	ambiguousResp, err := http.DefaultClient.Do(ambiguousReq)
	require.NoError(t, err)
	defer ambiguousResp.Body.Close()
	require.Equal(t, http.StatusOK, ambiguousResp.StatusCode)

	var rpcResp struct {
		Error *struct {
			Message string `json:"message"`
		} `json:"error,omitempty"`
	}
	require.NoError(t, json.NewDecoder(ambiguousResp.Body).Decode(&rpcResp))
	require.NotNil(t, rpcResp.Error)
	require.Contains(t, rpcResp.Error.Message, "multiple active harness sessions")
	require.Contains(t, rpcResp.Error.Message, "native session metadata is required")

	registerAgainReq, err := newStatefulMCPRequest(http.MethodPost, srv.URL+"/mcp?token=agen8-local", bytes.NewReader([]byte(fmt.Sprintf(`{
		"jsonrpc": "2.0",
		"id": "register-with-protocol-session-after-shared-token",
		"method": "tools/call",
		"params": {
			"name": "space",
			"arguments": {"action":"register","project_root":%s,"harness_kind":"claude-cli"}
		}
	}`, quoteJSON(projectRoot)))))
	require.NoError(t, err)
	registerAgainReq.Header.Set("Content-Type", "application/json")
	registerAgainReq.Header.Set("Accept", "application/json, text/event-stream")
	registerAgainResp, err := http.DefaultClient.Do(registerAgainReq)
	require.NoError(t, err)
	defer registerAgainResp.Body.Close()
	require.Equal(t, http.StatusOK, registerAgainResp.StatusCode)

	var registerAgainRPC struct {
		Result struct {
			StructuredContent struct {
				MemberID string `json:"memberId"`
			} `json:"structuredContent"`
			IsError bool `json:"isError"`
		} `json:"result"`
		Error any `json:"error,omitempty"`
	}
	require.NoError(t, json.NewDecoder(registerAgainResp.Body).Decode(&registerAgainRPC))
	require.Nil(t, registerAgainRPC.Error)
	require.False(t, registerAgainRPC.Result.IsError)
	require.NotEmpty(t, registerAgainRPC.Result.StructuredContent.MemberID)
}

func TestHTTPStrategyBootstrapMCPTokenRoutesMessageByMCPSessionHeader(t *testing.T) {
	d := newTestDaemon(t)
	handler, err := d.httpHandler()
	require.NoError(t, err)
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	projectRoot := filepath.Join(t.TempDir(), "agen8-mcp-server")
	require.NoError(t, os.MkdirAll(projectRoot, 0o755))

	registerCodexReq, err := newStatefulMCPRequest(http.MethodPost, srv.URL+"/mcp?token=agen8-local", bytes.NewReader([]byte(fmt.Sprintf(`{
		"jsonrpc": "2.0",
		"id": "register-codex-message-route",
		"method": "tools/call",
		"params": {
			"name": "space",
			"_meta": {"x-codex-turn-metadata": {"thread_id": "codex-thread-message-route"}},
			"arguments": {"action":"register","project_root":%s,"harness_kind":"codex"}
		}
	}`, quoteJSON(projectRoot)))))
	require.NoError(t, err)
	registerCodexReq.Header.Set("Content-Type", "application/json")
	registerCodexReq.Header.Set("Accept", "application/json, text/event-stream")
	registerCodexResp, err := http.DefaultClient.Do(registerCodexReq)
	require.NoError(t, err)
	defer registerCodexResp.Body.Close()
	require.Equal(t, http.StatusOK, registerCodexResp.StatusCode)

	var registerCodexRPC struct {
		Result struct {
			StructuredContent struct {
				MemberID string `json:"memberId"`
			} `json:"structuredContent"`
		} `json:"result"`
		Error any `json:"error,omitempty"`
	}
	require.NoError(t, json.NewDecoder(registerCodexResp.Body).Decode(&registerCodexRPC))
	require.Nil(t, registerCodexRPC.Error)
	codexMemberID := strings.TrimSpace(registerCodexRPC.Result.StructuredContent.MemberID)
	require.NotEmpty(t, codexMemberID)

	registerClaudeReq, err := newStatefulMCPRequest(http.MethodPost, srv.URL+"/mcp?token=agen8-local", bytes.NewReader([]byte(fmt.Sprintf(`{
		"jsonrpc": "2.0",
		"id": "register-claude-message-route",
		"method": "tools/call",
		"params": {
			"name": "space",
			"arguments": {"action":"register","project_root":%s,"harness_kind":"claude-cli"}
		}
	}`, quoteJSON(projectRoot)))))
	require.NoError(t, err)
	registerClaudeReq.Header.Set("Content-Type", "application/json")
	registerClaudeReq.Header.Set("Accept", "application/json, text/event-stream")
	registerClaudeReq.Header.Set("Agen8-Native-Session-Id", "claude-session-message-route")
	registerClaudeResp, err := http.DefaultClient.Do(registerClaudeReq)
	require.NoError(t, err)
	defer registerClaudeResp.Body.Close()
	require.Equal(t, http.StatusOK, registerClaudeResp.StatusCode)

	var registerClaudeRPC struct {
		Result struct {
			StructuredContent struct {
				MemberID         string `json:"memberId"`
				NativeSessionRef string `json:"nativeSessionRef"`
			} `json:"structuredContent"`
		} `json:"result"`
		Error any `json:"error,omitempty"`
	}
	require.NoError(t, json.NewDecoder(registerClaudeResp.Body).Decode(&registerClaudeRPC))
	require.Nil(t, registerClaudeRPC.Error)
	claudeMemberID := strings.TrimSpace(registerClaudeRPC.Result.StructuredContent.MemberID)
	require.NotEmpty(t, claudeMemberID)
	require.NotEqual(t, codexMemberID, claudeMemberID)
	require.Equal(t, "claude-session-message-route", registerClaudeRPC.Result.StructuredContent.NativeSessionRef)

	messageReq, err := newStatefulMCPRequest(http.MethodPost, srv.URL+"/mcp?token=agen8-local", bytes.NewReader([]byte(fmt.Sprintf(`{
		"jsonrpc": "2.0",
		"id": "claude-message-to-codex",
		"method": "tools/call",
		"params": {
			"name": "message",
			"arguments": {"action":"send","destination_member_id":%s,"kind":"inform","subject":"Claude route test","body":"must be sourced by Claude"}
		}
	}`, quoteJSON(codexMemberID)))))
	require.NoError(t, err)
	messageReq.Header.Set("Content-Type", "application/json")
	messageReq.Header.Set("Accept", "application/json, text/event-stream")
	messageReq.Header.Set("Agen8-Native-Session-Id", "claude-session-message-route")
	messageResp, err := http.DefaultClient.Do(messageReq)
	require.NoError(t, err)
	defer messageResp.Body.Close()
	require.Equal(t, http.StatusOK, messageResp.StatusCode)

	var messageRPC struct {
		Result struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
			IsError           bool `json:"isError"`
			StructuredContent struct {
				SourceMemberID      string `json:"sourceMemberId"`
				DestinationMemberID string `json:"destinationMemberId"`
			} `json:"structuredContent"`
		} `json:"result"`
		Error any `json:"error,omitempty"`
	}
	require.NoError(t, json.NewDecoder(messageResp.Body).Decode(&messageRPC))
	require.Nil(t, messageRPC.Error)
	require.False(t, messageRPC.Result.IsError, "message send returned error content: %+v", messageRPC.Result.Content)
	require.Equal(t, claudeMemberID, messageRPC.Result.StructuredContent.SourceMemberID)
	require.Equal(t, codexMemberID, messageRPC.Result.StructuredContent.DestinationMemberID)
}

func TestHTTPStrategyBootstrapMCPTokenResolvesRegisteredCodexThreadMetadata(t *testing.T) {
	d := newTestDaemon(t)
	handler, err := d.httpHandler()
	require.NoError(t, err)
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	projectRoot := filepath.Join(t.TempDir(), "agen8-mcp-server")
	require.NoError(t, os.MkdirAll(projectRoot, 0o755))
	registerReq, err := newStatefulMCPRequest(http.MethodPost, srv.URL+"/mcp?token=agen8-local", bytes.NewReader([]byte(fmt.Sprintf(`{
		"jsonrpc": "2.0",
		"id": "register-codex-meta",
		"method": "tools/call",
		"params": {
			"name": "space",
			"_meta": {"x-codex-turn-metadata": {"thread_id": "codex-thread-1"}},
			"arguments": {"action":"register","project_root":%s}
		}
	}`, quoteJSON(projectRoot)))))
	require.NoError(t, err)
	registerReq.Header.Set("Content-Type", "application/json")
	registerReq.Header.Set("Accept", "application/json, text/event-stream")
	registerResp, err := http.DefaultClient.Do(registerReq)
	require.NoError(t, err)
	defer registerResp.Body.Close()
	require.Equal(t, http.StatusOK, registerResp.StatusCode)

	var registerRPC struct {
		Result struct {
			StructuredContent struct {
				ProjectID string `json:"projectId"`
				MemberID  string `json:"memberId"`
			} `json:"structuredContent"`
		} `json:"result"`
		Error any `json:"error,omitempty"`
	}
	require.NoError(t, json.NewDecoder(registerResp.Body).Decode(&registerRPC))
	require.Nil(t, registerRPC.Error)
	require.NotEmpty(t, registerRPC.Result.StructuredContent.ProjectID)
	require.NotEmpty(t, registerRPC.Result.StructuredContent.MemberID)

	taskReq, err := newStatefulMCPRequest(http.MethodPost, srv.URL+"/mcp?token=agen8-local", bytes.NewReader([]byte(`{
		"jsonrpc": "2.0",
		"id": "task-list-protocol-header",
		"method": "tools/call",
		"params": {
			"name": "task",
			"arguments": {"action":"list","limit":5}
		}
	}`)))
	require.NoError(t, err)
	taskReq.Header.Set("Content-Type", "application/json")
	taskReq.Header.Set("Accept", "application/json, text/event-stream")
	taskReq.Header.Set("Agen8-Native-Thread-Id", "codex-thread-1")
	taskResp, err := http.DefaultClient.Do(taskReq)
	require.NoError(t, err)
	defer taskResp.Body.Close()
	require.Equal(t, http.StatusOK, taskResp.StatusCode)
	var taskRPC struct {
		Result struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
			IsError bool `json:"isError"`
		} `json:"result"`
		Error any `json:"error,omitempty"`
	}
	require.NoError(t, json.NewDecoder(taskResp.Body).Decode(&taskRPC))
	require.Nil(t, taskRPC.Error)
	require.False(t, taskRPC.Result.IsError, "task list returned error content: %+v", taskRPC.Result.Content)

	decisionReq, err := newStatefulMCPRequest(http.MethodPost, srv.URL+"/mcp?token=agen8-local", bytes.NewReader([]byte(`{
		"jsonrpc": "2.0",
		"id": "decision-log",
		"method": "tools/call",
		"params": {
			"name": "decision",
			"_meta": {"x-codex-turn-metadata": {"thread_id": "codex-thread-1"}},
			"arguments": {"action":"log","title":"Resolved bootstrap actor","rationale":"verify bootstrap resolves by registered thread metadata"}
		}
	}`)))
	require.NoError(t, err)
	decisionReq.Header.Set("Content-Type", "application/json")
	decisionReq.Header.Set("Accept", "application/json, text/event-stream")
	decisionResp, err := http.DefaultClient.Do(decisionReq)
	require.NoError(t, err)
	defer decisionResp.Body.Close()
	require.Equal(t, http.StatusOK, decisionResp.StatusCode)
	var decisionRPC struct {
		Result struct {
			IsError bool `json:"isError,omitempty"`
		} `json:"result"`
		Error any `json:"error,omitempty"`
	}
	require.NoError(t, json.NewDecoder(decisionResp.Body).Decode(&decisionRPC))
	require.Nil(t, decisionRPC.Error)
	require.False(t, decisionRPC.Result.IsError)

	decisions, err := d.app.DecisionSvc.List(context.Background(), decisiondomain.DecisionFilter{
		ProjectID: registerRPC.Result.StructuredContent.ProjectID,
		Query:     "Resolved bootstrap actor",
		Limit:     1,
	})
	require.NoError(t, err)
	require.Len(t, decisions, 1)
	require.Equal(t, registerRPC.Result.StructuredContent.MemberID, decisions[0].SourceIdentity)
}

func TestHTTPStrategyRegisterRejectsMalformedUUIDThreadRef(t *testing.T) {
	d := newTestDaemon(t)
	handler, err := d.httpHandler()
	require.NoError(t, err)
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	projectRoot := filepath.Join(t.TempDir(), "agen8-mcp-server")
	require.NoError(t, os.MkdirAll(projectRoot, 0o755))
	registerReq, err := newStatefulMCPRequest(http.MethodPost, srv.URL+"/mcp?token=agen8-local", bytes.NewReader([]byte(fmt.Sprintf(`{
		"jsonrpc": "2.0",
		"id": "register-bad-thread",
		"method": "tools/call",
		"params": {
			"name": "space",
			"arguments": {"action":"register","project_root":%s,"thread_id":"019e8d57-2197-7230-9d6d-b85e79418ee"}
		}
	}`, quoteJSON(projectRoot)))))
	require.NoError(t, err)
	registerReq.Header.Set("Content-Type", "application/json")
	registerReq.Header.Set("Accept", "application/json, text/event-stream")
	registerResp, err := http.DefaultClient.Do(registerReq)
	require.NoError(t, err)
	defer registerResp.Body.Close()
	require.Equal(t, http.StatusOK, registerResp.StatusCode)

	var rpcResp struct {
		Result struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
			IsError bool `json:"isError"`
		} `json:"result"`
		Error any `json:"error,omitempty"`
	}
	require.NoError(t, json.NewDecoder(registerResp.Body).Decode(&rpcResp))
	require.Nil(t, rpcResp.Error)
	require.True(t, rpcResp.Result.IsError)
	require.Contains(t, rpcResp.Result.Content[0].Text, "threadId")
	require.Contains(t, rpcResp.Result.Content[0].Text, "malformed")
}

func TestHTTPStrategyRegisterToolKeepsStableUserToken(t *testing.T) {
	d := newTestDaemon(t)
	handler, err := d.httpHandler()
	require.NoError(t, err)
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	projectRoot := filepath.Join(t.TempDir(), "agen8-mcp-server")
	require.NoError(t, os.MkdirAll(projectRoot, 0o755))
	registerCodexReq, err := newStatefulMCPRequest(http.MethodPost, srv.URL+"/mcp?token=agen8-local", bytes.NewReader([]byte(fmt.Sprintf(`{
		"jsonrpc": "2.0",
		"id": "register-codex",
		"method": "tools/call",
		"params": {
			"name": "space",
			"_meta": {"x-codex-turn-metadata": {"thread_id": "codex-thread-1"}},
			"arguments": {"action":"register","project_root":%s,"display_name":"Codex"}
		}
	}`, quoteJSON(projectRoot)))))
	require.NoError(t, err)
	registerCodexReq.Header.Set("Content-Type", "application/json")
	registerCodexReq.Header.Set("Accept", "application/json, text/event-stream")
	registerCodexResp, err := http.DefaultClient.Do(registerCodexReq)
	require.NoError(t, err)
	defer registerCodexResp.Body.Close()
	require.Equal(t, http.StatusOK, registerCodexResp.StatusCode)
	var codexRPC struct {
		Result struct {
			StructuredContent struct {
				ProjectID string `json:"projectId"`
				MemberID  string `json:"memberId"`
				Token     string `json:"token"`
			} `json:"structuredContent"`
		} `json:"result"`
		Error any `json:"error,omitempty"`
	}
	require.NoError(t, json.NewDecoder(registerCodexResp.Body).Decode(&codexRPC))
	require.Nil(t, codexRPC.Error)
	require.NotEmpty(t, codexRPC.Result.StructuredContent.ProjectID)
	require.NotEmpty(t, codexRPC.Result.StructuredContent.MemberID)
	require.Equal(t, "agen8-local", codexRPC.Result.StructuredContent.Token)

	registerClaudeReq, err := newStatefulMCPRequest(http.MethodPost, srv.URL+"/mcp?token=agen8-local", bytes.NewReader([]byte(fmt.Sprintf(`{
		"jsonrpc": "2.0",
		"id": "register-claude",
		"method": "tools/call",
		"params": {
			"name": "space",
			"arguments": {"action":"register","project_root":%s,"harness_kind":"claude-cli","session_id":"claude-session-1"}
		}
	}`, quoteJSON(projectRoot)))))
	require.NoError(t, err)
	registerClaudeReq.Header.Set("Content-Type", "application/json")
	registerClaudeReq.Header.Set("Accept", "application/json, text/event-stream")
	registerClaudeResp, err := http.DefaultClient.Do(registerClaudeReq)
	require.NoError(t, err)
	defer registerClaudeResp.Body.Close()
	require.Equal(t, http.StatusOK, registerClaudeResp.StatusCode)

	decisionReq, err := newStatefulMCPRequest(http.MethodPost, srv.URL+"/mcp?token="+codexRPC.Result.StructuredContent.Token, bytes.NewReader([]byte(`{
		"jsonrpc": "2.0",
		"id": "decision-log",
		"method": "tools/call",
		"params": {
			"name":"decision",
			"_meta": {"x-codex-turn-metadata": {"thread_id": "codex-thread-1"}},
			"arguments":{"action":"log","title":"Stable token identity smoke","rationale":"verify returned token remains bound to original member"}
		}
	}`)))
	require.NoError(t, err)
	decisionReq.Header.Set("Content-Type", "application/json")
	decisionReq.Header.Set("Accept", "application/json, text/event-stream")
	decisionResp, err := http.DefaultClient.Do(decisionReq)
	require.NoError(t, err)
	defer decisionResp.Body.Close()
	require.Equal(t, http.StatusOK, decisionResp.StatusCode)
	var decisionRPC struct {
		Result any `json:"result"`
		Error  any `json:"error,omitempty"`
	}
	require.NoError(t, json.NewDecoder(decisionResp.Body).Decode(&decisionRPC))
	require.Nil(t, decisionRPC.Error)
	decisions, err := d.app.DecisionSvc.List(context.Background(), decisiondomain.DecisionFilter{
		ProjectID: codexRPC.Result.StructuredContent.ProjectID,
		Query:     "Stable token identity smoke",
		Limit:     1,
	})
	require.NoError(t, err)
	require.Len(t, decisions, 1)
	require.Equal(t, codexRPC.Result.StructuredContent.MemberID, decisions[0].SourceIdentity)
}

func TestHTTPStrategyBootstrapMCPTokenUsesLatestAuthenticatedLocalUser(t *testing.T) {
	d := newTestDaemon(t)
	_ = createSetupAdminAPIKey(t, d)
	secondUserID, secondAPIKey := createAdditionalUserAPIKey(t, d, "ui@example.com", "UI User")
	handler, err := d.httpHandler()
	require.NoError(t, err)
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	statusReq, err := http.NewRequest(http.MethodPost, srv.URL+"/rpc", bytes.NewReader([]byte(`{
		"jsonrpc": "2.0",
		"id": "status",
		"method": "auth.status",
		"params": {}
	}`)))
	require.NoError(t, err)
	statusReq.Header.Set("Authorization", "Bearer "+secondAPIKey)
	statusReq.Header.Set("Content-Type", "application/json")
	statusResp, err := http.DefaultClient.Do(statusReq)
	require.NoError(t, err)
	defer statusResp.Body.Close()
	require.Equal(t, http.StatusOK, statusResp.StatusCode)

	projectRoot := filepath.Join(t.TempDir(), "current-user-project")
	require.NoError(t, os.MkdirAll(projectRoot, 0o755))
	registerReq, err := newStatefulMCPRequest(http.MethodPost, srv.URL+"/mcp?token=agen8-local", bytes.NewReader([]byte(fmt.Sprintf(`{
		"jsonrpc": "2.0",
		"id": "register",
		"method": "tools/call",
		"params": {
			"name": "space",
			"arguments": {"action":"register","project_root":%s}
		}
	}`, quoteJSON(projectRoot)))))
	require.NoError(t, err)
	registerReq.Header.Set("Content-Type", "application/json")
	registerReq.Header.Set("Accept", "application/json, text/event-stream")
	registerResp, err := http.DefaultClient.Do(registerReq)
	require.NoError(t, err)
	defer registerResp.Body.Close()
	require.Equal(t, http.StatusOK, registerResp.StatusCode)

	var registered struct {
		Result struct {
			StructuredContent struct {
				ProjectID string `json:"projectId"`
				SpaceID   string `json:"spaceId"`
			} `json:"structuredContent"`
		} `json:"result"`
		Error any `json:"error,omitempty"`
	}
	require.NoError(t, json.NewDecoder(registerResp.Body).Decode(&registered))
	require.Nil(t, registered.Error)
	require.NotEmpty(t, registered.Result.StructuredContent.ProjectID)
	require.NotEmpty(t, registered.Result.StructuredContent.SpaceID)

	ctx := caller.ContextWithCaller(context.Background(), caller.Caller{UserID: secondUserID})
	space, err := d.app.SpaceSvc.Get(ctx, spacedomain.SpaceID(registered.Result.StructuredContent.SpaceID))
	require.NoError(t, err)
	require.Equal(t, secondUserID, space.UserID)
}

func TestHTTPStrategyRPCDispatchesAuthStatus(t *testing.T) {
	d := newTestDaemon(t)
	apiKey := createSetupAdminAPIKey(t, d)
	handler, err := d.httpHandler()
	require.NoError(t, err)
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	req, err := http.NewRequest(http.MethodPost, srv.URL+"/rpc", bytes.NewReader([]byte(`{
		"jsonrpc": "2.0",
		"id": "1",
		"method": "auth.status",
		"params": {}
	}`)))
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var rpcResp struct {
		Result struct {
			Authenticated bool `json:"authenticated"`
			User          struct {
				ID   string `json:"id"`
				Role string `json:"role"`
			} `json:"user"`
		} `json:"result"`
		Error any `json:"error,omitempty"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&rpcResp))
	require.Nil(t, rpcResp.Error)
	require.True(t, rpcResp.Result.Authenticated)
	require.NotEmpty(t, rpcResp.Result.User.ID)
	require.Equal(t, "admin", rpcResp.Result.User.Role)
}

func TestHTTPStrategyRPCAllowsPublicAuthMethodsWithoutBearer(t *testing.T) {
	d := newTestDaemon(t)
	createSetupAdminAPIKey(t, d)
	handler, err := d.httpHandler()
	require.NoError(t, err)
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	resp, err := http.Post(srv.URL+"/rpc", "application/json", bytes.NewReader([]byte(`{
		"jsonrpc": "2.0",
		"id": "1",
		"method": "auth.status",
		"params": {}
	}`)))
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var statusResp struct {
		Result struct {
			Authenticated bool `json:"authenticated"`
		} `json:"result"`
		Error any `json:"error,omitempty"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&statusResp))
	require.Nil(t, statusResp.Error)
	require.False(t, statusResp.Result.Authenticated)
}

func TestHTTPStrategyRPCDispatchesSpaceCreate(t *testing.T) {
	d := newTestDaemon(t)
	apiKey := createSetupAdminAPIKey(t, d)
	handler, err := d.httpHandler()
	require.NoError(t, err)
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	req, err := http.NewRequest(http.MethodPost, srv.URL+"/rpc", bytes.NewReader([]byte(`{
		"jsonrpc": "2.0",
		"id": "1",
		"method": "space.create",
		"params": {
			"spaceId": "space-test",
			"projectId": "project-test",
			"title": "Test space"
		}
	}`)))
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var rpcResp struct {
		Result spacerpc.SpaceCreateResult `json:"result"`
		Error  any                        `json:"error,omitempty"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&rpcResp))
	require.Nil(t, rpcResp.Error)
	require.Equal(t, "space-test", rpcResp.Result.Space.ID)
	require.Equal(t, "project-test", rpcResp.Result.Space.ProjectID)
}

func TestHTTPStrategyRPCDispatchesMissionCreate(t *testing.T) {
	d := newTestDaemon(t)
	apiKey := createSetupAdminAPIKey(t, d)
	handler, err := d.httpHandler()
	require.NoError(t, err)
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	req, err := http.NewRequest(http.MethodPost, srv.URL+"/rpc", bytes.NewReader([]byte(`{
		"jsonrpc": "2.0",
		"id": "1",
		"method": "mission.create",
		"params": {
			"projectId": "project-test",
			"title": "Test mission"
		}
	}`)))
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var rpcResp struct {
		Result missionrpc.CreateMissionResult `json:"result"`
		Error  any                            `json:"error,omitempty"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&rpcResp))
	require.Nil(t, rpcResp.Error)
	require.NotEmpty(t, rpcResp.Result.Mission.ID)
	require.Equal(t, "project-test", rpcResp.Result.Mission.ProjectID)
	require.Equal(t, "Test mission", rpcResp.Result.Mission.Title)
}

func TestHTTPStrategyRPCDispatchesFilesGet(t *testing.T) {
	d := newTestDaemon(t)
	project, err := d.app.ProjectSvc.GetProject(context.Background(), "project-1")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(project.Root(), "notes.md"), []byte("# Notes\n"), 0o644))
	apiKey := createSetupAdminAPIKey(t, d)
	handler, err := d.httpHandler()
	require.NoError(t, err)
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	body := []byte(`{
		"jsonrpc": "2.0",
		"id": "1",
		"method": "files.get",
		"params": {
			"projectRoot": ` + quoteJSON(project.Root()) + `,
			"path": "/project/notes.md"
		}
	}`)
	req, err := http.NewRequest(http.MethodPost, srv.URL+"/rpc", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var rpcResp struct {
		Result struct {
			Content     string `json:"content"`
			ContentKind string `json:"contentKind"`
		} `json:"result"`
		Error any `json:"error,omitempty"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&rpcResp))
	require.Nil(t, rpcResp.Error)
	require.Equal(t, "# Notes\n", rpcResp.Result.Content)
	require.Equal(t, "text", rpcResp.Result.ContentKind)
}

func TestHTTPStrategyMCPListsNativeMissionToolForProvisionedMember(t *testing.T) {
	d := newTestDaemon(t)
	handler, err := d.httpHandler()
	require.NoError(t, err)
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	err = d.handleSpaceMemberLifecycle(context.Background(), eventbus.SpaceMemberLifecycleEvent{
		ProjectID:      "project-1",
		SpaceID:        "space-1",
		MemberID:       "member-1",
		ChannelID:      "channel:space-1:member:member-1",
		DisplayName:    "Worker One",
		MemberType:     "worker",
		EventType:      eventbus.SpaceMemberEventRegistered,
		LifecycleState: "active",
		HarnessKind:    "codex",
		Model:          "gpt-5.5",
		Effort:         "high",
	})
	require.NoError(t, err)

	req, err := newStatefulMCPRequest(http.MethodPost, srv.URL+"/mcp?token=agen8-local", bytes.NewReader([]byte(`{
		"jsonrpc": "2.0",
		"id": "1",
		"method": "tools/list",
		"params": {}
	}`)))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var rpcResp struct {
		Result struct {
			Tools []struct {
				Name string `json:"name"`
			} `json:"tools"`
		} `json:"result"`
		Error any `json:"error,omitempty"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&rpcResp))
	require.Nil(t, rpcResp.Error)
	var names []string
	for _, tool := range rpcResp.Result.Tools {
		names = append(names, tool.Name)
	}
	require.Contains(t, names, "mission")
}

func TestHTTPStrategyRestoresMCPTokenStoreForActiveSessionsAfterRestart(t *testing.T) {
	dataDir := t.TempDir()
	cfg := Config{Listener: ListenerHTTP, HTTPAddr: "127.0.0.1:0"}
	first := newTestDaemonWithDataDir(t, cfg, dataDir)
	err := first.handleSpaceMemberLifecycle(context.Background(), eventbus.SpaceMemberLifecycleEvent{
		ProjectID:      "project-1",
		SpaceID:        "space-1",
		MemberID:       "member-1",
		ChannelID:      "channel:space-1:member:member-1",
		DisplayName:    "Worker One",
		MemberType:     "worker",
		EventType:      eventbus.SpaceMemberEventRegistered,
		LifecycleState: "active",
		HarnessKind:    "codex",
		Model:          "gpt-5.5",
		Effort:         "high",
	})
	require.NoError(t, err)

	restarted := newTestDaemonWithDataDir(t, cfg, dataDir)
	handler, err := restarted.httpHandler()
	require.NoError(t, err)
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	req, err := newStatefulMCPRequest(http.MethodPost, srv.URL+"/mcp?token=agen8-local", bytes.NewReader([]byte(`{
		"jsonrpc": "2.0",
		"id": "1",
		"method": "tools/list",
		"params": {}
	}`)))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var rpcResp struct {
		Result struct {
			Tools []struct {
				Name string `json:"name"`
			} `json:"tools"`
		} `json:"result"`
		Error any `json:"error,omitempty"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&rpcResp))
	require.Nil(t, rpcResp.Error)
	var names []string
	for _, tool := range rpcResp.Result.Tools {
		names = append(names, tool.Name)
	}
	require.Contains(t, names, "space")
	require.Contains(t, names, "mission")
}

func TestHTTPStrategyRestoresStableUserMCPBindingAfterRestart(t *testing.T) {
	dataDir := t.TempDir()
	cfg := Config{Listener: ListenerHTTP, HTTPAddr: "127.0.0.1:0"}
	first := newTestDaemonWithDataDir(t, cfg, dataDir)
	err := first.handleSpaceMemberLifecycle(context.Background(), eventbus.SpaceMemberLifecycleEvent{
		ProjectID:      "project-1",
		SpaceID:        "space-1",
		MemberID:       "member-1",
		ChannelID:      "channel:space-1:member:member-1",
		DisplayName:    "Worker One",
		MemberType:     "worker",
		EventType:      eventbus.SpaceMemberEventRegistered,
		LifecycleState: "active",
		HarnessKind:    "codex",
		Model:          "gpt-5.5",
		Effort:         "high",
	})
	require.NoError(t, err)
	active, err := first.app.HarnessSvc.GetActiveSession(context.Background(), "member-1")
	require.NoError(t, err)
	require.NotNil(t, active)
	_, err = first.app.HarnessSvc.RefreshSessionMCPBinding(context.Background(), active.ID, bootstrapMCPToken, first.mcpURL(bootstrapMCPToken))
	require.NoError(t, err)

	restarted := newTestDaemonWithDataDir(t, cfg, dataDir)
	active, err = restarted.app.HarnessSvc.GetActiveSession(context.Background(), "member-1")
	require.NoError(t, err)
	require.NotNil(t, active)
	require.Equal(t, "agen8-local", active.MCPToken)
	require.Contains(t, active.MCPServers[0], "/mcp?token=agen8-local")

	handler, err := restarted.httpHandler()
	require.NoError(t, err)
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	req, err := newStatefulMCPRequest(http.MethodPost, srv.URL+"/mcp?token=agen8-local", bytes.NewReader([]byte(`{
		"jsonrpc": "2.0",
		"id": "stable-token-list",
		"method": "tools/list",
		"params": {}
	}`)))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var rpcResp struct {
		Result struct {
			Tools []struct {
				Name string `json:"name"`
			} `json:"tools"`
		} `json:"result"`
		Error any `json:"error,omitempty"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&rpcResp))
	require.Nil(t, rpcResp.Error)
	var names []string
	for _, tool := range rpcResp.Result.Tools {
		names = append(names, tool.Name)
	}
	require.Contains(t, names, "task")
}

func TestLocalStrategyAcceptsOneJSONRPCRequest(t *testing.T) {
	socketPath := filepath.Join(os.TempDir(), fmt.Sprintf("agen8-%d.sock", time.Now().UnixNano()))
	t.Cleanup(func() { _ = os.Remove(socketPath) })
	endpoint := "unix://" + socketPath
	d := newTestDaemonWithConfig(t, Config{Listener: ListenerLocal, Endpoint: endpoint, HTTPAddr: "127.0.0.1:0"})
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	errCh := make(chan error, 1)
	go func() {
		errCh <- d.Run(ctx)
	}()
	require.Eventually(t, func() bool {
		_, err := os.Stat(socketPath)
		return err == nil
	}, time.Second, 20*time.Millisecond)

	client := protocol.RPCClient{Endpoint: endpoint, Timeout: time.Second}
	var out taskrpc.TaskListResult
	require.NoError(t, client.Call(context.Background(), "task.list", map[string]any{}, &out))
	require.Empty(t, out.Tasks)

	cancel()
	require.Eventually(t, func() bool {
		select {
		case err := <-errCh:
			require.NoError(t, err)
			return true
		default:
			return false
		}
	}, time.Second, 20*time.Millisecond)
}

func TestHarnessSpaceMemberLifecycleHandlerPreservesSessionForModelAndEffortChanges(t *testing.T) {
	d := newTestDaemon(t)
	ctx := context.Background()

	err := d.handleSpaceMemberLifecycle(ctx, eventbus.SpaceMemberLifecycleEvent{
		ProjectID:      "project-1",
		SpaceID:        "space-1",
		MemberID:       "member-1",
		ChannelID:      "channel:space-1:member:member-1",
		DisplayName:    "Worker One",
		MemberType:     "worker",
		EventType:      eventbus.SpaceMemberEventRegistered,
		LifecycleState: "active",
		HarnessKind:    "codex",
		Model:          "gpt-5.5",
		Effort:         "high",
	})
	require.NoError(t, err)

	active, err := d.app.HarnessSvc.GetActiveSession(ctx, "member-1")
	require.NoError(t, err)
	require.NotNil(t, active)
	require.Equal(t, "gpt-5.5", active.Model)
	require.NotEmpty(t, active.MCPToken)
	require.Contains(t, active.MCPServers[0], "mcp_servers.agen8.url=")
	firstSessionID := active.ID

	err = d.handleSpaceMemberLifecycle(ctx, eventbus.SpaceMemberLifecycleEvent{
		ProjectID:      "project-1",
		SpaceID:        "space-1",
		MemberID:       "member-1",
		ChannelID:      "channel:space-1:member:member-1",
		DisplayName:    "Worker One",
		MemberType:     "worker",
		EventType:      eventbus.SpaceMemberEventConfigChanged,
		LifecycleState: "active",
		HarnessKind:    "codex",
		Model:          "gpt-5.4",
		Effort:         "medium",
	})
	require.NoError(t, err)

	sameSession, err := d.app.HarnessSvc.GetSession(ctx, firstSessionID)
	require.NoError(t, err)
	require.NotNil(t, sameSession)
	require.Equal(t, harnessdomain.SessionActive, sameSession.Status)
	require.Empty(t, sameSession.InactiveReason)

	active, err = d.app.HarnessSvc.GetActiveSession(ctx, "member-1")
	require.NoError(t, err)
	require.NotNil(t, active)
	require.Equal(t, firstSessionID, active.ID)
	require.Equal(t, "gpt-5.4", active.Model)
	require.Equal(t, "medium", active.Effort)
	require.Contains(t, active.SystemPrompt, `runtime_model value="gpt-5.4"`)
}

func TestHarnessSpaceMemberLifecycleHandlerProvisionsClaudeMCPConfigFile(t *testing.T) {
	d := newTestDaemon(t)
	ctx := context.Background()

	err := d.handleSpaceMemberLifecycle(ctx, eventbus.SpaceMemberLifecycleEvent{
		ProjectID:      "project-1",
		SpaceID:        "space-1",
		MemberID:       "member-1",
		ChannelID:      "channel:space-1:member:member-1",
		DisplayName:    "Worker One",
		MemberType:     "worker",
		EventType:      eventbus.SpaceMemberEventRegistered,
		LifecycleState: "active",
		HarnessKind:    "claude-cli",
		Model:          "claude-opus-4-7",
		Effort:         "high",
	})
	require.NoError(t, err)

	active, err := d.app.HarnessSvc.GetActiveSession(ctx, "member-1")
	require.NoError(t, err)
	require.NotNil(t, active)
	require.NotEmpty(t, active.MCPToken)
	require.Len(t, active.MCPServers, 1)
	require.NotContains(t, active.MCPServers[0], "mcp_servers.agen8.url=")
	require.FileExists(t, active.MCPServers[0])

	raw, err := os.ReadFile(active.MCPServers[0])
	require.NoError(t, err)
	var cfg struct {
		MCPServers map[string]struct {
			Type string `json:"type"`
			URL  string `json:"url"`
		} `json:"mcpServers"`
	}
	require.NoError(t, json.Unmarshal(raw, &cfg))
	require.Equal(t, "http", cfg.MCPServers["agen8"].Type)
	require.Contains(t, cfg.MCPServers["agen8"].URL, "/mcp?token=agen8-local")
}

func TestHarnessSpaceMemberLifecycleHandlerReplacesSessionForHarnessKindChanges(t *testing.T) {
	d := newTestDaemon(t)
	ctx := context.Background()

	err := d.handleSpaceMemberLifecycle(ctx, eventbus.SpaceMemberLifecycleEvent{
		ProjectID:      "project-1",
		SpaceID:        "space-1",
		MemberID:       "member-1",
		ChannelID:      "channel:space-1:member:member-1",
		DisplayName:    "Worker One",
		MemberType:     "worker",
		EventType:      eventbus.SpaceMemberEventRegistered,
		LifecycleState: "active",
		HarnessKind:    "codex",
		Model:          "gpt-5.5",
		Effort:         "high",
	})
	require.NoError(t, err)

	active, err := d.app.HarnessSvc.GetActiveSession(ctx, "member-1")
	require.NoError(t, err)
	require.NotNil(t, active)
	firstSessionID := active.ID

	err = d.handleSpaceMemberLifecycle(ctx, eventbus.SpaceMemberLifecycleEvent{
		ProjectID:      "project-1",
		SpaceID:        "space-1",
		MemberID:       "member-1",
		ChannelID:      "channel:space-1:member:member-1",
		DisplayName:    "Worker One",
		MemberType:     "worker",
		EventType:      eventbus.SpaceMemberEventConfigChanged,
		LifecycleState: "active",
		HarnessKind:    "claude-cli",
		Model:          "claude-opus-4-7",
		Effort:         "medium",
	})
	require.NoError(t, err)

	oldSession, err := d.app.HarnessSvc.GetSession(ctx, firstSessionID)
	require.NoError(t, err)
	require.NotNil(t, oldSession)
	require.Equal(t, harnessdomain.SessionInactive, oldSession.Status)
	require.Equal(t, harnessdomain.ReasonConfigChanged, oldSession.InactiveReason)

	active, err = d.app.HarnessSvc.GetActiveSession(ctx, "member-1")
	require.NoError(t, err)
	require.NotNil(t, active)
	require.NotEqual(t, firstSessionID, active.ID)
	require.Equal(t, "claude-cli", active.Kind)
	require.Equal(t, "claude-opus-4-7", active.Model)
}

func TestHarnessSpaceMemberLifecycleHandlerReplacesSessionForIdentityChanges(t *testing.T) {
	d := newTestDaemon(t)
	ctx := context.Background()

	err := d.handleSpaceMemberLifecycle(ctx, eventbus.SpaceMemberLifecycleEvent{
		ProjectID:      "project-1",
		SpaceID:        "space-1",
		MemberID:       "member-1",
		ChannelID:      "channel:space-1:member:member-1",
		DisplayName:    "Worker One",
		MemberType:     "worker",
		EventType:      eventbus.SpaceMemberEventRegistered,
		LifecycleState: "active",
		HarnessKind:    "codex",
		Model:          "gpt-5.5",
		Effort:         "high",
	})
	require.NoError(t, err)

	active, err := d.app.HarnessSvc.GetActiveSession(ctx, "member-1")
	require.NoError(t, err)
	require.NotNil(t, active)
	firstSessionID := active.ID

	err = d.handleSpaceMemberLifecycle(ctx, eventbus.SpaceMemberLifecycleEvent{
		ProjectID:      "project-1",
		SpaceID:        "space-1",
		MemberID:       "member-1",
		ChannelID:      "channel:space-1:member:member-1",
		DisplayName:    "Coordinator One",
		MemberType:     "coordinator",
		EventType:      eventbus.SpaceMemberEventIdentityChanged,
		LifecycleState: "active",
		HarnessKind:    "codex",
		Model:          "gpt-5.5",
		Effort:         "high",
	})
	require.NoError(t, err)

	oldSession, err := d.app.HarnessSvc.GetSession(ctx, firstSessionID)
	require.NoError(t, err)
	require.NotNil(t, oldSession)
	require.Equal(t, harnessdomain.SessionInactive, oldSession.Status)
	require.Equal(t, harnessdomain.ReasonConfigChanged, oldSession.InactiveReason)

	active, err = d.app.HarnessSvc.GetActiveSession(ctx, "member-1")
	require.NoError(t, err)
	require.NotNil(t, active)
	require.NotEqual(t, firstSessionID, active.ID)
	require.Equal(t, "Coordinator One", active.DisplayName)
	require.Equal(t, "coordinator", active.MemberType)
	require.Contains(t, active.SystemPrompt, `display_name="Coordinator One"`)
}

func TestHarnessSpaceMemberLifecycleHandlerDeactivatesRemovedMember(t *testing.T) {
	d := newTestDaemon(t)
	ctx := context.Background()
	_, err := d.app.HarnessSvc.ActivateSession(ctx, harnessActivationFromEvent(eventbus.SpaceMemberLifecycleEvent{
		ProjectID:      "project-1",
		SpaceID:        "space-1",
		MemberID:       "member-1",
		ChannelID:      "channel:space-1:member:member-1",
		DisplayName:    "Worker One",
		MemberType:     "worker",
		LifecycleState: "active",
		HarnessKind:    "codex",
		Model:          "gpt-5.5",
		Effort:         "high",
	}))
	require.NoError(t, err)

	err = d.handleSpaceMemberLifecycle(ctx, eventbus.SpaceMemberLifecycleEvent{
		ProjectID:      "project-1",
		SpaceID:        "space-1",
		MemberID:       "member-1",
		ChannelID:      "channel:space-1:member:member-1",
		DisplayName:    "Worker One",
		MemberType:     "worker",
		EventType:      eventbus.SpaceMemberEventRemoved,
		LifecycleState: "removed",
		HarnessKind:    "codex",
		Model:          "gpt-5.5",
		Effort:         "high",
	})
	require.NoError(t, err)

	active, err := d.app.HarnessSvc.GetActiveSession(ctx, "member-1")
	require.NoError(t, err)
	require.Nil(t, active)
}

func newTestDaemon(t *testing.T) *Daemon {
	t.Helper()
	return newTestDaemonWithConfig(t, Config{Listener: ListenerHTTP, HTTPAddr: "127.0.0.1:0"})
}

func newTestDaemonWithConfig(t *testing.T, cfg Config) *Daemon {
	t.Helper()
	return newTestDaemonWithDataDir(t, cfg, t.TempDir())
}

func newTestDaemonWithDataDir(t *testing.T, cfg Config, dataDir string) *Daemon {
	t.Helper()
	cfg.AppConfig = config.Default()
	cfg.AppConfig.DataDir = dataDir
	cfg.SetupToken = "test-setup-token"
	d, err := New(cfg)
	require.NoError(t, err)
	projectRoot := filepath.Join(dataDir, "project-1")
	require.NoError(t, os.MkdirAll(projectRoot, 0o755))
	_, err = d.app.ProjectSvc.SaveProject(context.Background(), projectapp.SaveProjectInput{
		ID:         "project-1",
		LocationID: types.LocationID("local"),
		Root:       projectRoot,
		Title:      "Test Project",
		Status:     projectdomain.StatusOpen,
	})
	require.NoError(t, err)
	return d
}

func createSetupAdminAPIKey(t *testing.T, d *Daemon) string {
	t.Helper()
	created, err := d.app.UserSvc.SetupFirstUser(context.Background(), userapp.SetupFirstUserParams{
		Email: "admin@example.com",
		Name:  "Admin",
	})
	require.NoError(t, err)
	err = d.app.AuthSvc.CreatePassword(context.Background(), authapp.CreatePasswordParams{
		UserID:   created.User.ID,
		Password: "password123",
	})
	require.NoError(t, err)
	apiKey, err := d.app.AuthSvc.CreateAPIKey(context.Background(), authapp.CreateAPIKeyParams{
		UserID: created.User.ID,
		Name:   "test",
	})
	require.NoError(t, err)
	return apiKey.Token
}

func createAdditionalUserAPIKey(t *testing.T, d *Daemon, email string, name string) (string, string) {
	t.Helper()
	handle, err := implstore.GetDBHandle(context.Background(), d.cfg.AppConfig)
	require.NoError(t, err)
	userRepo, err := userinfra.NewRepository(handle)
	require.NoError(t, err)
	userID, err := userdomain.NewID("user_test_ui")
	require.NoError(t, err)
	record, err := userdomain.New(userdomain.NewInput{
		ID:    userID,
		Email: email,
		Name:  name,
		Role:  userdomain.RoleUser,
		Now:   time.Now().UTC(),
	})
	require.NoError(t, err)
	require.NoError(t, userRepo.Create(context.Background(), record))
	apiKey, err := d.app.AuthSvc.CreateAPIKey(context.Background(), authapp.CreateAPIKeyParams{
		UserID: record.ID,
		Name:   "ui test",
	})
	require.NoError(t, err)
	return record.ID.String(), apiKey.Token
}

func newStatefulMCPRequest(method string, target string, body io.Reader) (*http.Request, error) {
	bodyBytes, err := io.ReadAll(body)
	if err != nil {
		return nil, err
	}
	initBody := bytes.NewReader([]byte(`{
		"jsonrpc": "2.0",
		"id": "test-init",
		"method": "initialize",
		"params": {
			"protocolVersion": "2025-06-18",
			"capabilities": {},
			"clientInfo": {"name":"agen8-daemon-test","version":"0.0.0"}
		}
	}`))
	initReq, err := http.NewRequest(http.MethodPost, target, initBody)
	if err != nil {
		return nil, err
	}
	initReq.Header.Set("Content-Type", "application/json")
	initReq.Header.Set("Accept", "application/json, text/event-stream")
	initResp, err := http.DefaultClient.Do(initReq)
	if err != nil {
		return nil, err
	}
	defer initResp.Body.Close()
	if initResp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(initResp.Body)
		return nil, fmt.Errorf("mcp initialize status=%d body=%s", initResp.StatusCode, string(data))
	}
	sessionID := strings.TrimSpace(initResp.Header.Get("Mcp-Session-Id"))
	if sessionID == "" {
		return nil, fmt.Errorf("mcp initialize response missing Mcp-Session-Id")
	}
	initializedReq, err := http.NewRequest(http.MethodPost, target, bytes.NewReader([]byte(`{
		"jsonrpc": "2.0",
		"method": "notifications/initialized",
		"params": {}
	}`)))
	if err != nil {
		return nil, err
	}
	initializedReq.Header.Set("Content-Type", "application/json")
	initializedReq.Header.Set("Accept", "application/json, text/event-stream")
	initializedReq.Header.Set("Mcp-Session-Id", sessionID)
	initializedResp, err := http.DefaultClient.Do(initializedReq)
	if err != nil {
		return nil, err
	}
	defer initializedResp.Body.Close()
	if initializedResp.StatusCode != http.StatusAccepted && initializedResp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(initializedResp.Body)
		return nil, fmt.Errorf("mcp initialized status=%d body=%s", initializedResp.StatusCode, string(data))
	}
	req, err := http.NewRequest(method, target, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Mcp-Session-Id", sessionID)
	return req, nil
}

func quoteJSON(value string) string {
	raw, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return string(raw)
}
