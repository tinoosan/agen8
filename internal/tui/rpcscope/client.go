package rpcscope

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/tinoosan/agen8/pkg/protocol"
)

const detachedThreadID = protocol.ThreadID("detached-control")

// Client provides scope-aware, auto-recovering RPC calls.
type Client struct {
	endpoint  string
	timeout   time.Duration
	sessionID string
	policy    RecoveryPolicy

	mu    sync.RWMutex
	state ScopeState
	ready bool
}

func NewClient(endpoint, sessionID string) *Client {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		endpoint = protocol.DefaultRPCEndpoint
	}
	return &Client{
		endpoint:  endpoint,
		timeout:   5 * time.Second,
		sessionID: strings.TrimSpace(sessionID),
		policy:    DefaultRecoveryPolicy(),
	}
}

func (c *Client) WithPolicy(policy RecoveryPolicy) *Client {
	if policy.MaxRetries < 0 {
		policy.MaxRetries = 0
	}
	if policy.BaseBackoff <= 0 {
		policy.BaseBackoff = 500 * time.Millisecond
	}
	if policy.MaxBackoff <= 0 {
		policy.MaxBackoff = 8 * time.Second
	}
	if len(policy.RetryableErrors) == 0 {
		policy.RetryableErrors = DefaultRecoveryPolicy().RetryableErrors
	}
	c.policy = policy
	return c
}

func (c *Client) WithTimeout(timeout time.Duration) *Client {
	if timeout > 0 {
		c.timeout = timeout
	}
	return c
}

func (c *Client) State() ScopeState {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.state
}

func (c *Client) SetState(state ScopeState) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.state = normalizeState(state)
	c.ready = strings.TrimSpace(c.state.SessionID) != ""
}

func (c *Client) call(ctx context.Context, method string, params, out any) error {
	cli := protocol.TCPClient{Endpoint: c.endpoint, Timeout: c.timeout}
	if err := cli.Call(ctx, method, params, out); err != nil {
		return fmt.Errorf("rpc %s: %w", method, err)
	}
	return nil
}

// ResolveControlSessionID resolves a usable control session ID without project-wide session enumeration.
func ResolveControlSessionID(ctx context.Context, endpoint, preferredSessionID, teamID string) (string, error) {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		endpoint = protocol.DefaultRPCEndpoint
	}
	preferredSessionID = strings.TrimSpace(preferredSessionID)
	teamID = strings.TrimSpace(teamID)
	if preferredSessionID != "" {
		return preferredSessionID, nil
	}
	if teamID != "" {
		return "", fmt.Errorf("%w: team control session unavailable", ErrScopeUnavailable)
	}
	return "", fmt.Errorf("%w: control session unavailable", ErrScopeUnavailable)
}

func (c *Client) RefreshScope(ctx context.Context) (ScopeState, error) {
	sid := strings.TrimSpace(c.sessionID)
	if sid == "" {
		return ScopeState{}, fmt.Errorf("%w: session id is required", ErrScopeUnavailable)
	}

	state := ScopeState{
		SessionID: sid,
		ThreadID:  sid,
		Mode:      "team",
	}

	var resolved protocol.SessionResolveThreadResult
	if err := c.call(ctx, protocol.MethodSessionResolveThread, protocol.SessionResolveThreadParams{
		SessionID: sid,
		RunID:     "",
	}, &resolved); err == nil {
		if threadID := strings.TrimSpace(resolved.ThreadID); threadID != "" {
			state.ThreadID = threadID
		}
		if runID := strings.TrimSpace(resolved.RunID); runID != "" {
			state.RunID = runID
		}
		if teamID := strings.TrimSpace(resolved.TeamID); teamID != "" {
			state.TeamID = teamID
		}
		if runID := strings.TrimSpace(resolved.RunID); runID != "" {
			state.RunID = runID
		}
	}

	if state.TeamID != "" {
		state.Mode = "team"
		var manifest protocol.TeamGetManifestResult
		if err := c.call(ctx, protocol.MethodTeamGetManifest, protocol.TeamGetManifestParams{
			ThreadID: protocol.ThreadID(sid),
			TeamID:   state.TeamID,
		}, &manifest); err == nil {
			if v := strings.TrimSpace(manifest.CoordinatorRun); v != "" {
				state.RunID = v
			}
			state.CoordinatorRole = strings.TrimSpace(manifest.CoordinatorRole)
		}
	}

	if state.RunID == "" {
		var agents protocol.AgentListResult
		if err := c.call(ctx, protocol.MethodAgentList, protocol.AgentListParams{
			ThreadID:  protocol.ThreadID(sid),
			SessionID: sid,
		}, &agents); err == nil {
			state.Mode = "team"
			for _, agent := range agents.Agents {
				runID := strings.TrimSpace(agent.RunID)
				if runID != "" {
					state.RunID = runID
					break
				}
			}
		}
	}

	state = normalizeState(state)
	c.mu.Lock()
	c.state = state
	c.ready = true
	c.mu.Unlock()
	return state, nil
}

func (c *Client) scope(ctx context.Context) (ScopeState, error) {
	c.mu.RLock()
	ready := c.ready
	state := c.state
	c.mu.RUnlock()
	if ready && strings.TrimSpace(state.SessionID) != "" {
		return state, nil
	}
	return c.RefreshScope(ctx)
}

func (c *Client) Call(ctx context.Context, method string, params, out any) error {
	if err := validateInvariants(method, params); err != nil {
		return err
	}
	return c.call(ctx, method, params, out)
}

// CallWithRecovery builds params from the latest scope, validates invariants,
// and retries once after scope refresh when a retryable scope error occurs.
func (c *Client) CallWithRecovery(ctx context.Context, method string, buildParamsFn func(ScopeState) (any, error), out any) (ScopeState, bool, error) {
	if buildParamsFn == nil {
		return ScopeState{}, false, fmt.Errorf("build params function is required")
	}

	var (
		attempt   int
		recovered bool
		lastScope ScopeState
	)
	for {
		scope, err := c.scope(ctx)
		if err != nil {
			return ScopeState{}, recovered, err
		}
		lastScope = scope

		params, err := buildParamsFn(scope)
		if err != nil {
			return scope, recovered, err
		}
		if err := validateInvariants(method, params); err != nil {
			if c.policy.RefreshOnError && attempt < c.policy.MaxRetries {
				attempt++
				recovered = true
				if _, rerr := c.RefreshScope(ctx); rerr != nil {
					return scope, recovered, err
				}
				continue
			}
			return scope, recovered, err
		}

		err = c.call(ctx, method, params, out)
		if err == nil {
			lastScope = c.State()
			return lastScope, recovered, nil
		}
		if !c.isRetryable(err) || attempt >= c.policy.MaxRetries {
			return scope, recovered, err
		}

		attempt++
		recovered = true
		if c.policy.RefreshOnError {
			if _, rerr := c.RefreshScope(ctx); rerr != nil {
				return scope, recovered, err
			}
		}
		backoff := c.policy.BaseBackoff
		for i := 1; i < attempt; i++ {
			backoff *= 2
			if backoff >= c.policy.MaxBackoff {
				backoff = c.policy.MaxBackoff
				break
			}
		}
		if backoff > 0 {
			timer := time.NewTimer(backoff)
			select {
			case <-ctx.Done():
				timer.Stop()
				return scope, recovered, ctx.Err()
			case <-timer.C:
			}
		}
	}
}

func validateInvariants(method string, params any) error {
	switch method {
	case protocol.MethodTaskList:
		p, ok := params.(protocol.TaskListParams)
		if !ok {
			return nil
		}
		if strings.TrimSpace(string(p.ThreadID)) == "" {
			return fmt.Errorf("%w: task.list requires thread id", ErrScopeUnavailable)
		}
		if strings.TrimSpace(p.TeamID) == "" && strings.TrimSpace(p.RunID) == "" {
			return fmt.Errorf("%w: task.list requires team or run scope", ErrScopeUnavailable)
		}
	case protocol.MethodTaskCreate:
		p, ok := params.(protocol.TaskCreateParams)
		if !ok {
			return nil
		}
		if strings.TrimSpace(string(p.ThreadID)) == "" {
			return fmt.Errorf("%w: task.create requires thread id", ErrScopeUnavailable)
		}
		if strings.TrimSpace(p.TeamID) == "" && strings.TrimSpace(p.RunID) == "" {
			return fmt.Errorf("%w: task.create requires team or run scope", ErrScopeUnavailable)
		}
	case protocol.MethodSessionPause:
		p, ok := params.(protocol.SessionPauseParams)
		if !ok {
			return nil
		}
		if strings.TrimSpace(string(p.ThreadID)) == "" || strings.TrimSpace(p.SessionID) == "" {
			return fmt.Errorf("%w: session.pause requires session thread", ErrScopeUnavailable)
		}
	case protocol.MethodSessionResume:
		p, ok := params.(protocol.SessionResumeParams)
		if !ok {
			return nil
		}
		if strings.TrimSpace(string(p.ThreadID)) == "" || strings.TrimSpace(p.SessionID) == "" {
			return fmt.Errorf("%w: session.resume requires session thread", ErrScopeUnavailable)
		}
	case protocol.MethodSessionStop:
		p, ok := params.(protocol.SessionStopParams)
		if !ok {
			return nil
		}
		if strings.TrimSpace(string(p.ThreadID)) == "" || strings.TrimSpace(p.SessionID) == "" {
			return fmt.Errorf("%w: session.stop requires session thread", ErrScopeUnavailable)
		}
	case protocol.MethodEventsListPaginated:
		p, ok := params.(protocol.EventsListPaginatedParams)
		if !ok {
			return nil
		}
		if strings.TrimSpace(p.RunID) == "" {
			return fmt.Errorf("%w: events.listPaginated requires run scope", ErrScopeUnavailable)
		}
	}
	return nil
}

func normalizeState(state ScopeState) ScopeState {
	state.SessionID = strings.TrimSpace(state.SessionID)
	state.ThreadID = strings.TrimSpace(state.ThreadID)
	state.RunID = strings.TrimSpace(state.RunID)
	state.TeamID = strings.TrimSpace(state.TeamID)
	state.Mode = fallback(strings.TrimSpace(state.Mode), "team")
	state.CoordinatorRole = strings.TrimSpace(state.CoordinatorRole)
	if state.ThreadID == "" {
		state.ThreadID = state.SessionID
	}
	return state
}

func fallback(v, d string) string {
	if strings.TrimSpace(v) == "" {
		return d
	}
	return strings.TrimSpace(v)
}

func pickControlSessionID(sessions []protocol.SessionListItem, preferredSessionID, teamID string) string {
	preferredSessionID = strings.TrimSpace(preferredSessionID)
	teamID = strings.TrimSpace(teamID)

	if preferredSessionID != "" {
		for _, item := range sessions {
			sid := strings.TrimSpace(item.SessionID)
			if sid == "" || sid != preferredSessionID {
				continue
			}
			if teamID == "" || strings.TrimSpace(item.TeamID) == teamID {
				return sid
			}
		}
	}

	for _, item := range sessions {
		sid := strings.TrimSpace(item.SessionID)
		if sid == "" {
			continue
		}
		if teamID != "" && strings.TrimSpace(item.TeamID) != teamID {
			continue
		}
		return sid
	}
	return ""
}
