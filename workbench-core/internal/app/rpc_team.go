package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"

	implstore "github.com/tinoosan/workbench-core/internal/store"
	"github.com/tinoosan/workbench-core/pkg/agent/state"
	"github.com/tinoosan/workbench-core/pkg/config"
	"github.com/tinoosan/workbench-core/pkg/cost"
	"github.com/tinoosan/workbench-core/pkg/fsutil"
	"github.com/tinoosan/workbench-core/pkg/protocol"
	"github.com/tinoosan/workbench-core/pkg/types"
)

func registerTeamHandlers(s *RPCServer, reg methodRegistry) error {
	return registerHandlers(
		func() error {
			return addBoundHandler[protocol.TeamGetStatusParams, protocol.TeamGetStatusResult](reg, protocol.MethodTeamGetStatus, false, s.teamGetStatus)
		},
		func() error {
			return addBoundHandler[protocol.TeamGetManifestParams, protocol.TeamGetManifestResult](reg, protocol.MethodTeamGetManifest, false, s.teamGetManifest)
		},
		func() error {
			return addBoundHandler[protocol.PlanGetParams, protocol.PlanGetResult](reg, protocol.MethodPlanGet, false, s.planGet)
		},
		func() error {
			return addBoundHandler[protocol.ModelListParams, protocol.ModelListResult](reg, protocol.MethodModelList, false, s.modelList)
		},
	)
}

func pricingKnownForRun(cfg config.Config, runID string) bool {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return false
	}
	run, err := implstore.LoadRun(cfg, runID)
	if err != nil || run.Runtime == nil {
		return false
	}
	if run.Runtime.PriceInPerMTokensUSD != 0 || run.Runtime.PriceOutPerMTokensUSD != 0 {
		return true
	}
	modelID := strings.TrimSpace(run.Runtime.Model)
	if modelID == "" {
		return false
	}
	_, _, ok := cost.DefaultPricing().Lookup(modelID)
	return ok
}

func (s *RPCServer) teamGetStatus(ctx context.Context, p protocol.TeamGetStatusParams) (protocol.TeamGetStatusResult, error) {
	scope, err := s.resolveTeamOrRunScope(ctx, p.ThreadID, p.TeamID, "")
	if err != nil {
		return protocol.TeamGetStatusResult{}, err
	}
	if strings.TrimSpace(scope.teamID) == "" {
		return protocol.TeamGetStatusResult{}, &protocol.ProtocolError{Code: protocol.CodeInvalidParams, Message: "team scope is required"}
	}
	teamID := strings.TrimSpace(scope.teamID)
	pending, _ := s.taskStore.CountTasks(ctx, state.TaskFilter{TeamID: teamID, Status: []types.TaskStatus{types.TaskStatusPending}})
	active, _ := s.taskStore.CountTasks(ctx, state.TaskFilter{TeamID: teamID, Status: []types.TaskStatus{types.TaskStatusActive}})
	done, _ := s.taskStore.CountTasks(ctx, state.TaskFilter{TeamID: teamID, Status: []types.TaskStatus{types.TaskStatusSucceeded, types.TaskStatusFailed, types.TaskStatusCanceled}})

	roleInfo := map[string]string{}
	roleByRunID := map[string]string{}
	runIDSet := map[string]struct{}{}
	pendingTasks, _ := s.taskStore.ListTasks(ctx, state.TaskFilter{TeamID: teamID, Status: []types.TaskStatus{types.TaskStatusPending}, SortBy: "created_at", SortDesc: false, Limit: 200})
	activeTasks, _ := s.taskStore.ListTasks(ctx, state.TaskFilter{TeamID: teamID, Status: []types.TaskStatus{types.TaskStatusActive}, SortBy: "updated_at", SortDesc: true, Limit: 200})
	completedTasks, _ := s.taskStore.ListTasks(ctx, state.TaskFilter{TeamID: teamID, Status: []types.TaskStatus{types.TaskStatusSucceeded, types.TaskStatusFailed, types.TaskStatusCanceled}, SortBy: "finished_at", SortDesc: true, Limit: 500})

	for _, task := range pendingTasks {
		role := strings.TrimSpace(task.AssignedRole)
		if role == "" {
			role = "(coordinator)"
		}
		if runID := strings.TrimSpace(task.RunID); runID != "" {
			if _, ok := roleByRunID[runID]; !ok {
				roleByRunID[runID] = role
			}
			runIDSet[runID] = struct{}{}
		}
		if _, exists := roleInfo[role]; !exists {
			roleInfo[role] = "pending: " + truncateText(strings.TrimSpace(task.Goal), 52)
		}
	}
	for _, task := range activeTasks {
		role := strings.TrimSpace(task.AssignedRole)
		if role == "" {
			role = "(coordinator)"
		}
		if runID := strings.TrimSpace(task.RunID); runID != "" {
			if _, ok := roleByRunID[runID]; !ok {
				roleByRunID[runID] = role
			}
			runIDSet[runID] = struct{}{}
		}
		roleInfo[role] = "active: " + truncateText(strings.TrimSpace(task.Goal), 52)
	}
	for _, task := range completedTasks {
		role := strings.TrimSpace(task.AssignedRole)
		if role == "" {
			role = "(coordinator)"
		}
		if runID := strings.TrimSpace(task.RunID); runID != "" {
			if _, ok := roleByRunID[runID]; !ok {
				roleByRunID[runID] = role
			}
			runIDSet[runID] = struct{}{}
		}
	}
	roleKeys := make([]string, 0, len(roleInfo))
	for role := range roleInfo {
		roleKeys = append(roleKeys, role)
	}
	sort.Strings(roleKeys)
	roles := make([]protocol.TeamRoleStatus, 0, len(roleKeys))
	for _, role := range roleKeys {
		roles = append(roles, protocol.TeamRoleStatus{Role: role, Info: roleInfo[role]})
	}
	runIDs := make([]string, 0, len(runIDSet))
	for runID := range runIDSet {
		runIDs = append(runIDs, runID)
	}
	sort.Strings(runIDs)
	totalTokens := 0
	totalCostUSD := 0.0
	pricingKnown := true
	for _, runID := range runIDs {
		stats, err := s.taskStore.GetRunStats(ctx, runID)
		if err != nil {
			continue
		}
		totalTokens += stats.TotalTokens
		totalCostUSD += stats.TotalCost
		if stats.TotalTokens > 0 && stats.TotalCost <= 0 && !pricingKnownForRun(s.cfg, runID) {
			pricingKnown = false
		}
	}
	if totalTokens == 0 {
		pricingKnown = true
	}
	return protocol.TeamGetStatusResult{
		Pending:      pending,
		Active:       active,
		Done:         done,
		Roles:        roles,
		RunIDs:       runIDs,
		RoleByRunID:  roleByRunID,
		TotalTokens:  totalTokens,
		TotalCostUSD: totalCostUSD,
		PricingKnown: pricingKnown,
	}, nil
}

func (s *RPCServer) teamGetManifest(ctx context.Context, p protocol.TeamGetManifestParams) (protocol.TeamGetManifestResult, error) {
	scope, err := s.resolveTeamOrRunScope(ctx, p.ThreadID, p.TeamID, "")
	if err != nil {
		return protocol.TeamGetManifestResult{}, err
	}
	teamID := strings.TrimSpace(scope.teamID)
	if teamID == "" {
		return protocol.TeamGetManifestResult{}, &protocol.ProtocolError{Code: protocol.CodeInvalidParams, Message: "team scope is required"}
	}
	path := filepath.Join(fsutil.GetTeamDir(s.cfg.DataDir, teamID), "team.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		return protocol.TeamGetManifestResult{}, err
	}
	var mf struct {
		TeamID          string                            `json:"teamId"`
		ProfileID       string                            `json:"profileId"`
		TeamModel       string                            `json:"teamModel,omitempty"`
		ModelChange     *protocol.TeamManifestModelChange `json:"modelChange,omitempty"`
		CoordinatorRole string                            `json:"coordinatorRole"`
		CoordinatorRun  string                            `json:"coordinatorRunId"`
		Roles           []protocol.TeamManifestRole       `json:"roles"`
		CreatedAt       string                            `json:"createdAt"`
	}
	if err := json.Unmarshal(raw, &mf); err != nil {
		return protocol.TeamGetManifestResult{}, err
	}
	return protocol.TeamGetManifestResult{
		TeamID:          strings.TrimSpace(mf.TeamID),
		ProfileID:       strings.TrimSpace(mf.ProfileID),
		TeamModel:       strings.TrimSpace(mf.TeamModel),
		ModelChange:     mf.ModelChange,
		CoordinatorRole: strings.TrimSpace(mf.CoordinatorRole),
		CoordinatorRun:  strings.TrimSpace(mf.CoordinatorRun),
		Roles:           mf.Roles,
		CreatedAt:       strings.TrimSpace(mf.CreatedAt),
	}, nil
}

func readPlanFilesForRun(dataDir string, run types.Run) (checklist string, checklistErr string, details string, detailsErr string) {
	runDir := fsutil.GetRunDir(dataDir, run)
	planDir := filepath.Join(runDir, "plan")
	load := func(name string) (string, string) {
		b, err := os.ReadFile(filepath.Join(planDir, name))
		if err != nil {
			if os.IsNotExist(err) {
				return "", ""
			}
			return "", err.Error()
		}
		return string(b), ""
	}
	details, detailsErr = load("HEAD.md")
	checklist, checklistErr = load("CHECKLIST.md")
	return checklist, checklistErr, details, detailsErr
}

func loadTeamManifestRunRoles(dataDir, teamID string) ([]string, map[string]string) {
	teamID = strings.TrimSpace(teamID)
	if teamID == "" {
		return nil, map[string]string{}
	}
	path := filepath.Join(fsutil.GetTeamDir(dataDir, teamID), "team.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, map[string]string{}
	}
	var mf struct {
		CoordinatorRun string                      `json:"coordinatorRunId"`
		Roles          []protocol.TeamManifestRole `json:"roles"`
	}
	if err := json.Unmarshal(raw, &mf); err != nil {
		return nil, map[string]string{}
	}
	runIDs := make([]string, 0, len(mf.Roles)+1)
	roleByRun := make(map[string]string, len(mf.Roles)+1)
	seen := map[string]struct{}{}
	if runID := strings.TrimSpace(mf.CoordinatorRun); runID != "" {
		runIDs = append(runIDs, runID)
		seen[runID] = struct{}{}
	}
	for _, role := range mf.Roles {
		runID := strings.TrimSpace(role.RunID)
		if runID == "" {
			continue
		}
		if _, ok := seen[runID]; !ok {
			runIDs = append(runIDs, runID)
			seen[runID] = struct{}{}
		}
		if _, ok := roleByRun[runID]; !ok {
			roleByRun[runID] = strings.TrimSpace(role.RoleName)
		}
	}
	return runIDs, roleByRun
}

func (s *RPCServer) planGet(ctx context.Context, p protocol.PlanGetParams) (protocol.PlanGetResult, error) {
	scope, err := s.resolveTeamOrRunScope(ctx, p.ThreadID, p.TeamID, p.RunID)
	if err != nil {
		return protocol.PlanGetResult{}, err
	}
	if strings.TrimSpace(scope.teamID) == "" {
		runID := strings.TrimSpace(scope.runID)
		run, _ := implstore.LoadRun(s.cfg, runID)
		if run.RunID == "" {
			run = types.Run{RunID: runID}
		}
		checklist, checklistErr, details, detailsErr := readPlanFilesForRun(s.cfg.DataDir, run)
		return protocol.PlanGetResult{
			Checklist: checklist, ChecklistErr: checklistErr, Details: details, DetailsErr: detailsErr, SourceRuns: []string{runID},
		}, nil
	}
	if strings.TrimSpace(scope.runID) != "" {
		runID := strings.TrimSpace(scope.runID)
		run, _ := implstore.LoadRun(s.cfg, runID)
		if run.RunID == "" {
			run = types.Run{RunID: runID}
		}
		checklist, checklistErr, details, detailsErr := readPlanFilesForRun(s.cfg.DataDir, run)
		return protocol.PlanGetResult{
			Checklist: checklist, ChecklistErr: checklistErr, Details: details, DetailsErr: detailsErr, SourceRuns: []string{runID},
		}, nil
	}
	aggregate := p.AggregateTeam
	if !aggregate {
		return protocol.PlanGetResult{}, &protocol.ProtocolError{Code: protocol.CodeInvalidParams, Message: "aggregateTeam must be true when runId is omitted in team mode"}
	}
	manifestRunIDs, roleByRun := loadTeamManifestRunRoles(s.cfg.DataDir, strings.TrimSpace(scope.teamID))
	if roleByRun == nil {
		roleByRun = map[string]string{}
	}
	runIDs := make([]string, 0, len(manifestRunIDs)+16)
	seenRuns := map[string]struct{}{}
	for _, runID := range manifestRunIDs {
		runID = strings.TrimSpace(runID)
		if runID == "" {
			continue
		}
		if _, ok := seenRuns[runID]; ok {
			continue
		}
		runIDs = append(runIDs, runID)
		seenRuns[runID] = struct{}{}
	}
	tasks, _ := s.taskStore.ListTasks(ctx, state.TaskFilter{TeamID: strings.TrimSpace(scope.teamID), Limit: 1000, SortBy: "created_at", SortDesc: true})
	for _, t := range tasks {
		runID := strings.TrimSpace(t.RunID)
		if runID == "" {
			continue
		}
		if _, ok := seenRuns[runID]; !ok {
			runIDs = append(runIDs, runID)
			seenRuns[runID] = struct{}{}
		}
		if _, ok := roleByRun[runID]; !ok {
			roleByRun[runID] = strings.TrimSpace(t.AssignedRole)
		}
	}
	if len(runIDs) == 0 {
		return protocol.PlanGetResult{
			Checklist: "No team plan files found yet.",
			Details:   "Waiting for team runs to publish plan files.",
		}, nil
	}
	checkParts := make([]string, 0, len(runIDs))
	detailParts := make([]string, 0, len(runIDs))
	errParts := []string{}
	for _, runID := range runIDs {
		role := strings.TrimSpace(roleByRun[runID])
		if role == "" {
			role = runID
		}
		run, _ := implstore.LoadRun(s.cfg, runID)
		if run.RunID == "" {
			run = types.Run{RunID: runID}
		}
		check, checkErr, det, detErr := readPlanFilesForRun(s.cfg.DataDir, run)
		if checkErr != "" {
			errParts = append(errParts, "["+role+"] checklist: "+checkErr)
		}
		if detErr != "" {
			errParts = append(errParts, "["+role+"] details: "+detErr)
		}
		if strings.TrimSpace(check) != "" {
			checkParts = append(checkParts, "## "+role+"\n\n"+strings.TrimSpace(check))
		}
		if strings.TrimSpace(det) != "" {
			detailParts = append(detailParts, "## "+role+"\n\n"+strings.TrimSpace(det))
		}
	}
	checklist := "No team checklist files found yet."
	if len(checkParts) > 0 {
		checklist = strings.Join(checkParts, "\n\n---\n\n")
	}
	details := "No team plan detail files found yet."
	if len(detailParts) > 0 {
		details = strings.Join(detailParts, "\n\n---\n\n")
	}
	joinedErr := strings.Join(errParts, " | ")
	return protocol.PlanGetResult{
		Checklist: checklist, ChecklistErr: joinedErr, Details: details, DetailsErr: joinedErr, SourceRuns: runIDs,
	}, nil
}

func (s *RPCServer) modelList(ctx context.Context, p protocol.ModelListParams) (protocol.ModelListResult, error) {
	if _, err := s.resolveThreadID(p.ThreadID); err != nil {
		return protocol.ModelListResult{}, err
	}
	_ = ctx
	providerFilter := strings.ToLower(strings.TrimSpace(p.Provider))
	query := strings.ToLower(strings.TrimSpace(p.Query))
	infos := cost.SupportedModelInfos()
	models := make([]protocol.ModelEntry, 0, len(infos))
	counts := map[string]int{}
	for _, info := range infos {
		provider := strings.TrimSpace(info.Provider)
		id := strings.TrimSpace(info.ID)
		if providerFilter != "" && strings.ToLower(provider) != providerFilter {
			continue
		}
		if query != "" && !strings.Contains(strings.ToLower(id), query) && !strings.Contains(strings.ToLower(provider), query) {
			continue
		}
		counts[provider]++
		models = append(models, protocol.ModelEntry{
			ID:          id,
			Provider:    provider,
			InputPerM:   info.InputPerM,
			OutputPerM:  info.OutputPerM,
			IsReasoning: info.IsReasoning,
		})
	}
	providers := make([]protocol.ModelProvider, 0, len(counts))
	for name, count := range counts {
		providers = append(providers, protocol.ModelProvider{Name: name, Count: count})
	}
	sort.Slice(providers, func(i, j int) bool { return providers[i].Name < providers[j].Name })
	sort.Slice(models, func(i, j int) bool {
		if models[i].Provider != models[j].Provider {
			return models[i].Provider < models[j].Provider
		}
		return models[i].ID < models[j].ID
	})
	return protocol.ModelListResult{Providers: providers, Models: models}, nil
}

func truncateText(s string, max int) string {
	s = strings.TrimSpace(s)
	if max <= 0 || len(s) <= max {
		return s
	}
	if max <= 3 {
		return s[:max]
	}
	return s[:max-3] + "..."
}
