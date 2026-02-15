package protocol

import "github.com/tinoosan/workbench-core/pkg/types"

type SessionGetTotalsParams struct {
	ThreadID ThreadID `json:"threadId"`
	TeamID   string   `json:"teamId,omitempty"`
	RunID    string   `json:"runId,omitempty"`
}

type SessionStartParams struct {
	ThreadID ThreadID `json:"threadId"`
	Mode     string   `json:"mode,omitempty"` // standalone|team
	Profile  string   `json:"profile,omitempty"`
	Goal     string   `json:"goal,omitempty"`
	Model    string   `json:"model,omitempty"`
}

type SessionStartResult struct {
	SessionID    string   `json:"sessionId"`
	PrimaryRunID string   `json:"primaryRunId"`
	Mode         string   `json:"mode"`
	Profile      string   `json:"profile,omitempty"`
	Model        string   `json:"model,omitempty"`
	TeamID       string   `json:"teamId,omitempty"`
	RunIDs       []string `json:"runIds,omitempty"`
}

type SessionListParams struct {
	ThreadID      ThreadID `json:"threadId"`
	TitleContains string   `json:"titleContains,omitempty"`
	Limit         int      `json:"limit,omitempty"`
	Offset        int      `json:"offset,omitempty"`
}

type SessionListItem struct {
	SessionID     string `json:"sessionId"`
	Title         string `json:"title,omitempty"`
	CurrentRunID  string `json:"currentRunId,omitempty"`
	ActiveModel   string `json:"activeModel,omitempty"`
	Mode          string `json:"mode,omitempty"`
	TeamID        string `json:"teamId,omitempty"`
	Profile       string `json:"profile,omitempty"`
	RunningAgents int    `json:"runningAgents,omitempty"`
	PausedAgents  int    `json:"pausedAgents,omitempty"`
	TotalAgents   int    `json:"totalAgents,omitempty"`
	CreatedAt     string `json:"createdAt,omitempty"`
	UpdatedAt     string `json:"updatedAt,omitempty"`
}

type SessionListResult struct {
	Sessions   []SessionListItem `json:"sessions"`
	TotalCount int               `json:"totalCount,omitempty"`
}

type SessionRenameParams struct {
	ThreadID  ThreadID `json:"threadId"`
	SessionID string   `json:"sessionId,omitempty"`
	Title     string   `json:"title"`
}

type SessionRenameResult struct {
	SessionID string `json:"sessionId"`
	Title     string `json:"title"`
}

type AgentListParams struct {
	ThreadID  ThreadID `json:"threadId"`
	SessionID string   `json:"sessionId,omitempty"`
}

type AgentListItem struct {
	RunID       string `json:"runId"`
	SessionID   string `json:"sessionId"`
	Profile     string `json:"profile,omitempty"`
	Status      string `json:"status,omitempty"`
	Goal        string `json:"goal,omitempty"`
	TeamID      string `json:"teamId,omitempty"`
	Role        string `json:"role,omitempty"`
	StartedAt   string `json:"startedAt,omitempty"`
	FinishedAt  string `json:"finishedAt,omitempty"`
	ParentRunID string `json:"parentRunId,omitempty"`
}

type AgentListResult struct {
	Agents []AgentListItem `json:"agents"`
}

type AgentStartParams struct {
	ThreadID  ThreadID `json:"threadId"`
	SessionID string   `json:"sessionId,omitempty"`
	Profile   string   `json:"profile,omitempty"`
	Goal      string   `json:"goal,omitempty"`
	Model     string   `json:"model,omitempty"`
}

type AgentStartResult struct {
	RunID     string `json:"runId"`
	SessionID string `json:"sessionId"`
	Profile   string `json:"profile,omitempty"`
	Model     string `json:"model,omitempty"`
}

type AgentPauseParams struct {
	ThreadID ThreadID `json:"threadId"`
	RunID    string   `json:"runId"`
}

type AgentPauseResult struct {
	RunID  string `json:"runId"`
	Status string `json:"status"`
}

type AgentResumeParams struct {
	ThreadID ThreadID `json:"threadId"`
	RunID    string   `json:"runId"`
}

type AgentResumeResult struct {
	RunID  string `json:"runId"`
	Status string `json:"status"`
}

type SessionPauseParams struct {
	ThreadID  ThreadID `json:"threadId"`
	SessionID string   `json:"sessionId,omitempty"`
}

type SessionPauseResult struct {
	SessionID      string   `json:"sessionId"`
	AffectedRunIDs []string `json:"affectedRunIds,omitempty"`
}

type SessionResumeParams struct {
	ThreadID  ThreadID `json:"threadId"`
	SessionID string   `json:"sessionId,omitempty"`
}

type SessionResumeResult struct {
	SessionID      string   `json:"sessionId"`
	AffectedRunIDs []string `json:"affectedRunIds,omitempty"`
}

type SessionStopParams struct {
	ThreadID  ThreadID `json:"threadId"`
	SessionID string   `json:"sessionId,omitempty"`
}

type SessionStopResult struct {
	SessionID      string   `json:"sessionId"`
	AffectedRunIDs []string `json:"affectedRunIds,omitempty"`
}

type SessionGetTotalsResult struct {
	LastTurnTokensIn  int     `json:"lastTurnTokensIn"`
	LastTurnTokensOut int     `json:"lastTurnTokensOut"`
	LastTurnTokens    int     `json:"lastTurnTokens"`
	TotalTokensIn     int     `json:"totalTokensIn"`
	TotalTokensOut    int     `json:"totalTokensOut"`
	TotalTokens       int     `json:"totalTokens"`
	LastTurnCostUSD   string  `json:"lastTurnCostUSD,omitempty"`
	TotalCostUSD      float64 `json:"totalCostUSD"`
	PricingKnown      bool    `json:"pricingKnown"`
	TasksDone         int     `json:"tasksDone"`
}

type ActivityListParams struct {
	ThreadID ThreadID `json:"threadId"`
	TeamID   string   `json:"teamId,omitempty"`
	RunID    string   `json:"runId,omitempty"`
	Role     string   `json:"role,omitempty"`
	Limit    int      `json:"limit,omitempty"`
	Offset   int      `json:"offset,omitempty"`
	SortDesc bool     `json:"sortDesc,omitempty"`
}

type ActivityListResult struct {
	Activities []types.Activity `json:"activities"`
	TotalCount int              `json:"totalCount"`
	NextOffset int              `json:"nextOffset,omitempty"`
}

type TeamGetStatusParams struct {
	ThreadID ThreadID `json:"threadId"`
	TeamID   string   `json:"teamId,omitempty"`
}

type TeamRoleStatus struct {
	Role string `json:"role"`
	Info string `json:"info"`
}

type TeamGetStatusResult struct {
	Pending      int               `json:"pending"`
	Active       int               `json:"active"`
	Done         int               `json:"done"`
	Roles        []TeamRoleStatus  `json:"roles"`
	RunIDs       []string          `json:"runIds"`
	RoleByRunID  map[string]string `json:"roleByRunId"`
	TotalTokens  int               `json:"totalTokens"`
	TotalCostUSD float64           `json:"totalCostUSD"`
	PricingKnown bool              `json:"pricingKnown"`
}

type TeamGetManifestParams struct {
	ThreadID ThreadID `json:"threadId"`
	TeamID   string   `json:"teamId,omitempty"`
}

type TeamManifestModelChange struct {
	RequestedModel string `json:"requestedModel,omitempty"`
	Status         string `json:"status,omitempty"`
	RequestedAt    string `json:"requestedAt,omitempty"`
	AppliedAt      string `json:"appliedAt,omitempty"`
	Reason         string `json:"reason,omitempty"`
	Error          string `json:"error,omitempty"`
}

type TeamManifestRole struct {
	RoleName  string `json:"roleName"`
	RunID     string `json:"runId"`
	SessionID string `json:"sessionId"`
}

type TeamGetManifestResult struct {
	TeamID          string                   `json:"teamId"`
	ProfileID       string                   `json:"profileId"`
	TeamModel       string                   `json:"teamModel,omitempty"`
	ModelChange     *TeamManifestModelChange `json:"modelChange,omitempty"`
	CoordinatorRole string                   `json:"coordinatorRole"`
	CoordinatorRun  string                   `json:"coordinatorRunId"`
	Roles           []TeamManifestRole       `json:"roles"`
	CreatedAt       string                   `json:"createdAt"`
}

type PlanGetParams struct {
	ThreadID      ThreadID `json:"threadId"`
	TeamID        string   `json:"teamId,omitempty"`
	RunID         string   `json:"runId,omitempty"`
	AggregateTeam bool     `json:"aggregateTeam,omitempty"`
}

type PlanGetResult struct {
	Checklist    string   `json:"checklist"`
	ChecklistErr string   `json:"checklistErr,omitempty"`
	Details      string   `json:"details"`
	DetailsErr   string   `json:"detailsErr,omitempty"`
	SourceRuns   []string `json:"sourceRuns,omitempty"`
}

type ModelListParams struct {
	ThreadID ThreadID `json:"threadId"`
	Provider string   `json:"provider,omitempty"`
	Query    string   `json:"query,omitempty"`
}

type ModelProvider struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

type ModelEntry struct {
	ID          string  `json:"id"`
	Provider    string  `json:"provider"`
	InputPerM   float64 `json:"inputPerM"`
	OutputPerM  float64 `json:"outputPerM"`
	IsReasoning bool    `json:"isReasoning"`
}

type ModelListResult struct {
	Providers []ModelProvider `json:"providers"`
	Models    []ModelEntry    `json:"models"`
}
