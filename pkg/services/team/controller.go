package team

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/tinoosan/agen8/pkg/agent/state"
	"github.com/tinoosan/agen8/pkg/services/session"
	"github.com/tinoosan/agen8/pkg/services/task"
	"github.com/tinoosan/agen8/pkg/types"
)

// Sentinel errors for app to map to protocol.ProtocolError.
var (
	ErrThreadNotFound = errors.New("thread not found")
	ErrRunNotFound    = errors.New("run not found")
	ErrCancelActive   = errors.New("cancel active tasks failed")
	ErrStopRunFailed  = errors.New("stop run failed")
)

// RoleRunController is the per-run handle used by Controller (SetPaused, SetModel, SetReasoning).
// App wraps each teamRoleRuntime in an adapter that implements this interface.
type RoleRunController interface {
	RunID() string
	SessionID() string
	SetPaused(paused bool)
	SetModel(ctx context.Context, model string) error
	SetReasoning(ctx context.Context, effort, summary string) error
}

// Controller implements team RPC behavior: SetModel, SetReasoning, PauseRuns, ResumeRuns, StopRuns.
// App builds it with session/task/state and runtimes, then wires RPC handlers to delegate here.
type Controller struct {
	sessionService session.Service
	taskStore      state.TaskStore
	taskCanceler   task.ActiveTaskCanceler
	stateMgr       *StateManager
	runtimes       []RoleRunController
	applier        ModelApplier
	runStopper     RunStopper

	defaultReasoningEffort  string
	defaultReasoningSummary string
}

// ControllerConfig configures a Controller.
type ControllerConfig struct {
	SessionService          session.Service
	TaskStore               state.TaskStore
	TaskCanceler            task.ActiveTaskCanceler
	StateMgr                *StateManager
	Runtimes                []RoleRunController
	Applier                 ModelApplier
	RunStopper              RunStopper
	DefaultReasoningEffort  string
	DefaultReasoningSummary string
}

// NewController creates a Controller from the given config.
func NewController(cfg ControllerConfig) *Controller {
	return &Controller{
		sessionService:          cfg.SessionService,
		taskStore:               cfg.TaskStore,
		taskCanceler:            cfg.TaskCanceler,
		stateMgr:                cfg.StateMgr,
		runtimes:                cfg.Runtimes,
		applier:                 cfg.Applier,
		runStopper:              cfg.RunStopper,
		defaultReasoningEffort:  strings.TrimSpace(cfg.DefaultReasoningEffort),
		defaultReasoningSummary: strings.TrimSpace(cfg.DefaultReasoningSummary),
	}
}

func (c *Controller) validThreadIDs() map[string]struct{} {
	out := map[string]struct{}{}
	for _, r := range c.runtimes {
		if s := strings.TrimSpace(r.SessionID()); s != "" {
			out[s] = struct{}{}
		}
	}
	return out
}

func (c *Controller) isValidThread(threadID string) bool {
	threadID = strings.TrimSpace(threadID)
	if threadID == "" {
		return false
	}
	_, ok := c.validThreadIDs()[threadID]
	return ok
}

// SetModel updates session model and reasoning, requests model change, and applies reasoning to affected runtimes.
func (c *Controller) SetModel(ctx context.Context, threadID, target, model string) ([]string, error) {
	threadID = strings.TrimSpace(threadID)
	if !c.isValidThread(threadID) {
		return nil, ErrThreadNotFound
	}
	loaded, err := c.sessionService.LoadSession(ctx, threadID)
	if err != nil || strings.TrimSpace(loaded.SessionID) != threadID {
		return nil, ErrThreadNotFound
	}
	model = strings.TrimSpace(model)
	loaded.ActiveModel = model
	if c.defaultReasoningEffort != "" {
		loaded.ReasoningEffort = c.defaultReasoningEffort
	}
	if c.defaultReasoningSummary != "" {
		loaded.ReasoningSummary = c.defaultReasoningSummary
	}
	if err := c.sessionService.SaveSession(ctx, loaded); err != nil {
		return nil, err
	}
	applied, err := RequestModelChange(ctx, c.taskStore, c.stateMgr, c.applier, model, strings.TrimSpace(target), "rpc.control.setModel")
	if err != nil {
		return nil, err
	}
	for _, r := range c.runtimes {
		runID := strings.TrimSpace(r.RunID())
		if runID == "" {
			continue
		}
		if len(applied) != 0 {
			found := false
			for _, id := range applied {
				if strings.TrimSpace(id) == runID {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}
		_ = r.SetReasoning(ctx, loaded.ReasoningEffort, loaded.ReasoningSummary)
	}
	return applied, nil
}

// SetReasoning updates session reasoning and applies to runtimes (optionally filtered by target).
func (c *Controller) SetReasoning(ctx context.Context, threadID, target, effort, summary string) ([]string, error) {
	threadID = strings.TrimSpace(threadID)
	if !c.isValidThread(threadID) {
		return nil, ErrThreadNotFound
	}
	loaded, err := c.sessionService.LoadSession(ctx, threadID)
	if err != nil || strings.TrimSpace(loaded.SessionID) != threadID {
		return nil, ErrThreadNotFound
	}
	effort = strings.ToLower(strings.TrimSpace(effort))
	summary = strings.ToLower(strings.TrimSpace(summary))
	if summary == "none" {
		summary = "off"
	}
	loaded.ReasoningEffort = effort
	loaded.ReasoningSummary = summary
	if err := c.sessionService.SaveSession(ctx, loaded); err != nil {
		return nil, err
	}
	target = strings.TrimSpace(target)
	applied := make([]string, 0, len(c.runtimes))
	for _, r := range c.runtimes {
		runID := strings.TrimSpace(r.RunID())
		if runID == "" {
			continue
		}
		if target != "" && target != threadID && target != runID {
			continue
		}
		if err := r.SetReasoning(ctx, effort, summary); err != nil {
			return applied, err
		}
		applied = append(applied, runID)
	}
	if target != "" && target != threadID && len(applied) == 0 {
		return nil, ErrRunNotFound
	}
	return applied, nil
}

// PauseRuns sets each run's status to paused and SetPaused(true).
func (c *Controller) PauseRuns(ctx context.Context, threadID, sessionID string) ([]string, error) {
	if !c.isValidThread(threadID) {
		return nil, ErrThreadNotFound
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		sessionID = threadID
	}
	if !c.isValidThread(sessionID) {
		return nil, ErrThreadNotFound
	}
	return c.pauseResumeOrStop(ctx, "pause", func(r RoleRunController, runID string) error {
		loaded, err := c.sessionService.LoadRun(ctx, runID)
		if err != nil {
			return err
		}
		loaded.Status = types.RunStatusPaused
		loaded.FinishedAt = nil
		loaded.Error = nil
		if err := c.sessionService.SaveRun(ctx, loaded); err != nil {
			return err
		}
		r.SetPaused(true)
		return nil
	})
}

// ResumeRuns sets each run's status to running and SetPaused(false).
func (c *Controller) ResumeRuns(ctx context.Context, threadID, sessionID string) ([]string, error) {
	if !c.isValidThread(threadID) {
		return nil, ErrThreadNotFound
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		sessionID = threadID
	}
	if !c.isValidThread(sessionID) {
		return nil, ErrThreadNotFound
	}
	return c.pauseResumeOrStop(ctx, "resume", func(r RoleRunController, runID string) error {
		loaded, err := c.sessionService.LoadRun(ctx, runID)
		if err != nil {
			return err
		}
		switch strings.ToLower(strings.TrimSpace(loaded.Status)) {
		case types.RunStatusCanceled, types.RunStatusSucceeded, types.RunStatusFailed:
			return fmt.Errorf("run %s is terminal (%s)", runID, loaded.Status)
		}
		loaded.Status = types.RunStatusRunning
		loaded.FinishedAt = nil
		loaded.Error = nil
		if err := c.sessionService.SaveRun(ctx, loaded); err != nil {
			return err
		}
		r.SetPaused(false)
		return nil
	})
}

// StopRuns sets each run to paused, SetPaused(true), cancels the run loop via RunStopper, and cancels active tasks.
func (c *Controller) StopRuns(ctx context.Context, threadID, sessionID string) ([]string, error) {
	if !c.isValidThread(threadID) {
		return nil, ErrThreadNotFound
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		sessionID = threadID
	}
	if !c.isValidThread(sessionID) {
		return nil, ErrThreadNotFound
	}
	return c.pauseResumeOrStop(ctx, "stop", func(r RoleRunController, runID string) error {
		loaded, err := c.sessionService.LoadRun(ctx, runID)
		if err != nil {
			return err
		}
		loaded.Status = types.RunStatusCanceled
		now := time.Now().UTC()
		loaded.FinishedAt = &now
		loaded.Error = nil
		if err := c.sessionService.SaveRun(ctx, loaded); err != nil {
			return err
		}
		r.SetPaused(true)
		var opErr error
		if c.runStopper != nil {
			if err := c.runStopper.StopRun(ctx, runID); err != nil {
				opErr = errors.Join(opErr, fmt.Errorf("%w %s: %w", ErrStopRunFailed, runID, err))
			}
		}
		if c.taskCanceler != nil {
			if _, err := c.taskCanceler.CancelActiveTasksByRun(ctx, runID, "run stopped"); err != nil {
				opErr = errors.Join(opErr, fmt.Errorf("%w for run %s: %w", ErrCancelActive, runID, err))
			}
		}
		return opErr
	})
}

func (c *Controller) pauseResumeOrStop(ctx context.Context, op string, fn func(r RoleRunController, runID string) error) ([]string, error) {
	var mu sync.Mutex
	var wg sync.WaitGroup
	affected := make([]string, 0, len(c.runtimes))
	errs := make([]error, 0, len(c.runtimes))
	for _, r := range c.runtimes {
		runID := strings.TrimSpace(r.RunID())
		if runID == "" {
			continue
		}
		r, runID := r, runID
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := fn(r, runID); err != nil {
				mu.Lock()
				errs = append(errs, fmt.Errorf("run %s: %w", runID, err))
				mu.Unlock()
				return
			}
			mu.Lock()
			affected = append(affected, runID)
			mu.Unlock()
		}()
	}
	wg.Wait()
	if len(errs) != 0 {
		return affected, fmt.Errorf("%s session partial failure: %w", op, errors.Join(errs...))
	}
	return affected, nil
}
