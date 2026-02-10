package protocol

import "github.com/tinoosan/workbench-core/pkg/types"

type SessionGetTotalsParams struct {
	ThreadID ThreadID `json:"threadId"`
	TeamID   string   `json:"teamId,omitempty"`
	RunID    string   `json:"runId,omitempty"`
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
