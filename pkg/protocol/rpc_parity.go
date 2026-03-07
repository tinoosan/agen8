package protocol

import "github.com/tinoosan/agen8/pkg/types"

type SessionGetTotalsParams struct {
	ThreadID ThreadID `json:"threadId"`
	TeamID   string   `json:"teamId,omitempty"`
	RunID    string   `json:"runId,omitempty"`
}

type SessionStartParams struct {
	ThreadID    ThreadID `json:"threadId"`
	Mode        string   `json:"mode,omitempty"` // canonical: team; legacy aliases accepted on input
	Profile     string   `json:"profile,omitempty"`
	Goal        string   `json:"goal,omitempty"`
	Model       string   `json:"model,omitempty"`
	TeamID      string   `json:"teamId,omitempty"`
	ProjectID   string   `json:"projectId,omitempty"`
	ProjectRoot string   `json:"projectRoot,omitempty"` // project dir when created via agen8 new
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
	ProjectRoot   string   `json:"projectRoot,omitempty"` // filter sessions by project
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
	ProjectRoot   string `json:"projectRoot,omitempty"`
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
	// SpawnIndex is the ordinal among sibling sub-agent runs (1-based). Only set when ParentRunID is set.
	SpawnIndex int `json:"spawnIndex,omitempty"`
}

type AgentListResult struct {
	Agents []AgentListItem `json:"agents"`
}

// RunListChildrenParams are the params for run.listChildren.
type RunListChildrenParams struct {
	ParentRunID string `json:"parentRunId"`
}

// RunListChildrenResult is the result of run.listChildren (child runs of the given parent).
type RunListChildrenResult struct {
	Runs []types.Run `json:"runs"`
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

type SessionClearHistoryParams struct {
	ThreadID  ThreadID `json:"threadId"`
	SessionID string   `json:"sessionId,omitempty"`
	TeamID    string   `json:"teamId,omitempty"`
}

type SessionClearHistoryResult struct {
	SessionID           string   `json:"sessionId,omitempty"`
	TeamID              string   `json:"teamId,omitempty"`
	SourceRuns          []string `json:"sourceRuns,omitempty"`
	EventsDeleted       int64    `json:"eventsDeleted"`
	HistoryDeleted      int64    `json:"historyDeleted"`
	ActivitiesDeleted   int64    `json:"activitiesDeleted"`
	ConstructorState    int64    `json:"constructorStateDeleted"`
	ConstructorManifest int64    `json:"constructorManifestDeleted"`
}

type SessionDeleteResult struct {
	SessionID string `json:"sessionId"`
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

type SessionResolveThreadParams struct {
	SessionID string `json:"sessionId"`
	RunID     string `json:"runId,omitempty"`
	ThreadID  string `json:"threadId,omitempty"`
}

type SessionResolveThreadResult struct {
	SessionID string `json:"sessionId"`
	ThreadID  string `json:"threadId"`
	RunID     string `json:"runId,omitempty"`
	TeamID    string `json:"teamId,omitempty"`
	Exists    bool   `json:"exists"`
}

type ActivityListParams struct {
	ThreadID         ThreadID `json:"threadId"`
	TeamID           string   `json:"teamId,omitempty"`
	RunID            string   `json:"runId,omitempty"`
	Role             string   `json:"role,omitempty"`
	Limit            int      `json:"limit,omitempty"`
	Offset           int      `json:"offset,omitempty"`
	SortDesc         bool     `json:"sortDesc,omitempty"`
	IncludeChildRuns bool     `json:"includeChildRuns,omitempty"` // When true (standalone), merge activities from child runs and prefix with "[Sub-agent N]"
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
	Pending        int               `json:"pending"`
	Active         int               `json:"active"`
	Done           int               `json:"done"`
	Roles          []TeamRoleStatus  `json:"roles"`
	RunIDs         []string          `json:"runIds"`
	RoleByRunID    map[string]string `json:"roleByRunId"`
	TotalTokensIn  int               `json:"totalTokensIn"`
	TotalTokensOut int               `json:"totalTokensOut"`
	TotalTokens    int               `json:"totalTokens"`
	TotalCostUSD   float64           `json:"totalCostUSD"`
	PricingKnown   bool              `json:"pricingKnown"`
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
	TeamID                string                   `json:"teamId"`
	ProfileID             string                   `json:"profileId"`
	TeamModel             string                   `json:"teamModel,omitempty"`
	ModelChange           *TeamManifestModelChange `json:"modelChange,omitempty"`
	CoordinatorRole       string                   `json:"coordinatorRole"`
	ReviewerRole          string                   `json:"reviewerRole,omitempty"`
	CoordinatorRun        string                   `json:"coordinatorRunId"`
	CoordinatorThreadID   string                   `json:"coordinatorThreadId,omitempty"`
	Roles                 []TeamManifestRole       `json:"roles"`
	DesiredReplicasByRole map[string]int           `json:"desiredReplicasByRole,omitempty"`
	CreatedAt             string                   `json:"createdAt"`
}

type TeamDeleteResult struct {
	TeamID             string   `json:"teamId"`
	ProjectRoot        string   `json:"projectRoot,omitempty"`
	DeletedSessionIDs  []string `json:"deletedSessionIds,omitempty"`
	DeletedArtifactSet bool     `json:"deletedArtifactSet,omitempty"`
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

// EventsListPaginatedParams are the params for events.listPaginated.
type EventsListPaginatedParams struct {
	RunID     string   `json:"runId"`
	Limit     int      `json:"limit,omitempty"`
	Offset    int      `json:"offset,omitempty"`
	AfterSeq  int64    `json:"afterSeq,omitempty"`
	BeforeSeq int64    `json:"beforeSeq,omitempty"`
	Types     []string `json:"types,omitempty"`
	SortDesc  bool     `json:"sortDesc,omitempty"`
}

// EventsListPaginatedResult is the result of events.listPaginated.
type EventsListPaginatedResult struct {
	Events []types.EventRecord `json:"events"`
	Next   int64               `json:"next,omitempty"`
}

// EventsLatestSeqParams are the params for events.latestSeq.
type EventsLatestSeqParams struct {
	RunID string `json:"runId"`
}

// EventsLatestSeqResult is the result of events.latestSeq.
type EventsLatestSeqResult struct {
	Seq int64 `json:"seq"`
}

// EventsCountParams are the params for events.count.
type EventsCountParams struct {
	RunID string   `json:"runId"`
	Types []string `json:"types,omitempty"`
}

// EventsCountResult is the result of events.count.
type EventsCountResult struct {
	Count int `json:"count"`
}

type ProjectConfig struct {
	ProjectID          string `json:"projectId,omitempty"`
	DefaultProfile     string `json:"defaultProfile,omitempty"`
	DefaultMode        string `json:"defaultMode,omitempty"`
	DefaultTeamProfile string `json:"defaultTeamProfile,omitempty"`
	RPCEndpoint        string `json:"rpcEndpoint,omitempty"`
	DataDirOverride    string `json:"dataDirOverride,omitempty"`
	ObsidianVaultPath  string `json:"obsidianVaultPath,omitempty"`
	ObsidianEnabled    bool   `json:"obsidianEnabled,omitempty"`
	CreatedAt          string `json:"createdAt,omitempty"`
	Version            int    `json:"version,omitempty"`
}

type ProjectState struct {
	ActiveSessionID string `json:"activeSessionId,omitempty"`
	ActiveTeamID    string `json:"activeTeamId,omitempty"`
	ActiveRunID     string `json:"activeRunId,omitempty"`
	ActiveThreadID  string `json:"activeThreadId,omitempty"`
	LastAttachedAt  string `json:"lastAttachedAt,omitempty"`
	LastCommand     string `json:"lastCommand,omitempty"`
}

type ProjectContext struct {
	Cwd        string        `json:"cwd,omitempty"`
	RootDir    string        `json:"rootDir,omitempty"`
	ProjectDir string        `json:"projectDir,omitempty"`
	ConfigPath string        `json:"configPath,omitempty"`
	StatePath  string        `json:"statePath,omitempty"`
	Exists     bool          `json:"exists"`
	Config     ProjectConfig `json:"config"`
	State      ProjectState  `json:"state"`
}

type ProjectGetContextParams struct {
	Cwd string `json:"cwd,omitempty"`
}

type ProjectGetContextResult struct {
	Context ProjectContext `json:"context"`
}

type ProjectListTeamsParams struct {
	Cwd         string `json:"cwd,omitempty"`
	ProjectRoot string `json:"projectRoot,omitempty"`
}

type ProjectTeamSummary struct {
	ProjectID        string `json:"projectId,omitempty"`
	ProjectRoot      string `json:"projectRoot,omitempty"`
	TeamID           string `json:"teamId,omitempty"`
	ProfileID        string `json:"profileId,omitempty"`
	PrimarySessionID string `json:"primarySessionId,omitempty"`
	CoordinatorRunID string `json:"coordinatorRunId,omitempty"`
	Status           string `json:"status,omitempty"`
	CreatedAt        string `json:"createdAt,omitempty"`
	UpdatedAt        string `json:"updatedAt,omitempty"`
	ManifestPresent  bool   `json:"manifestPresent,omitempty"`
	DesiredEnabled   bool   `json:"desiredEnabled,omitempty"`
	ReconcileStatus  string `json:"reconcileStatus,omitempty"`
	ManagedBy        string `json:"managedBy,omitempty"`
}

type ProjectListTeamsResult struct {
	Teams []ProjectTeamSummary `json:"teams"`
}

type ProjectGetTeamParams struct {
	Cwd         string `json:"cwd,omitempty"`
	ProjectRoot string `json:"projectRoot,omitempty"`
	TeamID      string `json:"teamId"`
}

type ProjectGetTeamResult struct {
	Team ProjectTeamSummary `json:"team"`
}

type ProjectDeleteTeamsResult struct {
	ProjectRoot       string   `json:"projectRoot,omitempty"`
	DeletedTeamIDs    []string `json:"deletedTeamIds,omitempty"`
	DeletedSessionIDs []string `json:"deletedSessionIds,omitempty"`
}

type ProjectDiffParams struct {
	Cwd         string `json:"cwd,omitempty"`
	ProjectRoot string `json:"projectRoot,omitempty"`
}

type ProjectApplyParams struct {
	Cwd         string `json:"cwd,omitempty"`
	ProjectRoot string `json:"projectRoot,omitempty"`
}

type ProjectDesiredTeam struct {
	Profile          string `json:"profile,omitempty"`
	Enabled          bool   `json:"enabled,omitempty"`
	OverrideInterval string `json:"overrideInterval,omitempty"`
}

type ProjectReconcileAction struct {
	Action  string `json:"action,omitempty"`
	Profile string `json:"profile,omitempty"`
	TeamID  string `json:"teamId,omitempty"`
	Reason  string `json:"reason,omitempty"`
	Managed bool   `json:"managed,omitempty"`
}

type ProjectDiffResult struct {
	ProjectRoot  string                   `json:"projectRoot,omitempty"`
	ProjectID    string                   `json:"projectId,omitempty"`
	DesiredTeams []ProjectDesiredTeam     `json:"desiredTeams,omitempty"`
	ActualTeams  []ProjectTeamSummary     `json:"actualTeams,omitempty"`
	Actions      []ProjectReconcileAction `json:"actions,omitempty"`
	Converged    bool                     `json:"converged"`
	Status       string                   `json:"status,omitempty"`
	Error        string                   `json:"error,omitempty"`
}

type ProjectSetActiveSessionParams struct {
	Cwd             string `json:"cwd,omitempty"`
	ActiveSessionID string `json:"activeSessionId"`
	ActiveTeamID    string `json:"activeTeamId,omitempty"`
	ActiveRunID     string `json:"activeRunId,omitempty"`
	ActiveThreadID  string `json:"activeThreadId,omitempty"`
	LastCommand     string `json:"lastCommand,omitempty"`
}

type ProjectSetActiveSessionResult struct {
	Context ProjectContext `json:"context"`
}

type LogsQueryParams struct {
	RunID     string   `json:"runId,omitempty"`
	SessionID string   `json:"sessionId,omitempty"`
	AgentID   string   `json:"agentId,omitempty"`
	Types     []string `json:"types,omitempty"`
	Limit     int      `json:"limit,omitempty"`
	Offset    int      `json:"offset,omitempty"`
	AfterSeq  int64    `json:"afterSeq,omitempty"`
	SortDesc  bool     `json:"sortDesc,omitempty"`
}

type LogsQueryResult struct {
	Events []types.EventRecord `json:"events"`
	Next   int64               `json:"next,omitempty"`
}

type ActivityStreamParams struct {
	RunID    string   `json:"runId,omitempty"`
	AfterSeq int64    `json:"afterSeq,omitempty"`
	Limit    int      `json:"limit,omitempty"`
	Types    []string `json:"types,omitempty"`
}

type ActivityStreamResult struct {
	Events    []types.EventRecord `json:"events"`
	Next      int64               `json:"next,omitempty"`
	LatestSeq int64               `json:"latestSeq,omitempty"`
}

type RuntimeGetRunStateParams struct {
	SessionID string `json:"sessionId"`
	RunID     string `json:"runId"`
}

type RuntimeGetSessionStateParams struct {
	SessionID string `json:"sessionId"`
}

type RuntimeRunState struct {
	SessionID       string  `json:"sessionId"`
	RunID           string  `json:"runId"`
	Model           string  `json:"model,omitempty"`
	RunTotalTokens  int     `json:"runTotalTokens,omitempty"`
	RunTotalCostUSD float64 `json:"runTotalCostUSD,omitempty"`
	PersistedStatus string  `json:"persistedStatus,omitempty"`
	WorkerPresent   bool    `json:"workerPresent"`
	PausedFlag      bool    `json:"pausedFlag"`
	LastHeartbeatAt string  `json:"lastHeartbeatAt,omitempty"`
	EffectiveStatus string  `json:"effectiveStatus,omitempty"`
}

type RuntimeGetRunStateResult struct {
	State RuntimeRunState `json:"state"`
}

type RuntimeGetSessionStateResult struct {
	SessionID string            `json:"sessionId"`
	Runs      []RuntimeRunState `json:"runs"`
}

type ProjectListProjectsParams struct{}

type ProjectRegistrySummary struct {
	ProjectRoot  string         `json:"projectRoot"`
	ProjectID    string         `json:"projectId"`
	ManifestPath string         `json:"manifestPath,omitempty"`
	Enabled      bool           `json:"enabled"`
	CreatedAt    string         `json:"createdAt,omitempty"`
	UpdatedAt    string         `json:"updatedAt,omitempty"`
	Metadata     map[string]any `json:"metadata,omitempty"`
}

type ProjectListProjectsResult struct {
	Projects []ProjectRegistrySummary `json:"projects"`
}
