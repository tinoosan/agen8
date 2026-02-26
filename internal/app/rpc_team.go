package app

import (
	"context"
	"os"
	"sort"
	"strings"

	"github.com/tinoosan/agen8/pkg/agent/state"
	"github.com/tinoosan/agen8/pkg/cost"
	"github.com/tinoosan/agen8/pkg/protocol"
	pkgsession "github.com/tinoosan/agen8/pkg/services/session"
	"github.com/tinoosan/agen8/pkg/services/team"
	"github.com/tinoosan/agen8/pkg/types"
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

func pricingKnownForRun(ctx context.Context, session pkgsession.Service, runID string) bool {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return false
	}
	run, err := session.LoadRun(ctx, runID)
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
	_, _, ok := cost.LookupPricing(ctx, modelID)
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
	pending, _ := s.taskService.CountTasks(ctx, state.TaskFilter{TeamID: teamID, Status: []types.TaskStatus{types.TaskStatusPending}})
	active, _ := s.taskService.CountTasks(ctx, state.TaskFilter{TeamID: teamID, Status: []types.TaskStatus{types.TaskStatusActive}})
	done, _ := s.taskService.CountTasks(ctx, state.TaskFilter{TeamID: teamID, Status: []types.TaskStatus{types.TaskStatusSucceeded, types.TaskStatusFailed, types.TaskStatusCanceled}})

	roleInfo := map[string]string{}
	manifestRunIDs, roleByRunID := s.loadTeamManifestRunRoles(ctx, teamID)
	runIDSet := map[string]struct{}{}
	for _, runID := range manifestRunIDs {
		runID = strings.TrimSpace(runID)
		if runID == "" {
			continue
		}
		runIDSet[runID] = struct{}{}
	}
	manifestRoster := len(runIDSet) > 0
	pendingTasks, _ := s.taskService.ListTasks(ctx, state.TaskFilter{TeamID: teamID, Status: []types.TaskStatus{types.TaskStatusPending}, SortBy: "created_at", SortDesc: false, Limit: 200})
	activeTasks, _ := s.taskService.ListTasks(ctx, state.TaskFilter{TeamID: teamID, Status: []types.TaskStatus{types.TaskStatusActive}, SortBy: "updated_at", SortDesc: true, Limit: 200})
	completedTasks, _ := s.taskService.ListTasks(ctx, state.TaskFilter{TeamID: teamID, Status: []types.TaskStatus{types.TaskStatusSucceeded, types.TaskStatusFailed, types.TaskStatusCanceled}, SortBy: "finished_at", SortDesc: true, Limit: 500})

	for _, task := range pendingTasks {
		role := strings.TrimSpace(task.AssignedRole)
		if role == "" {
			role = "(coordinator)"
		}
		if runID := strings.TrimSpace(task.RunID); runID != "" {
			if !manifestRoster {
				if _, ok := roleByRunID[runID]; !ok {
					roleByRunID[runID] = role
				}
				runIDSet[runID] = struct{}{}
			} else if _, ok := runIDSet[runID]; ok {
				if _, seenRole := roleByRunID[runID]; !seenRole {
					roleByRunID[runID] = role
				}
			}
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
			if !manifestRoster {
				if _, ok := roleByRunID[runID]; !ok {
					roleByRunID[runID] = role
				}
				runIDSet[runID] = struct{}{}
			} else if _, ok := runIDSet[runID]; ok {
				if _, seenRole := roleByRunID[runID]; !seenRole {
					roleByRunID[runID] = role
				}
			}
		}
		roleInfo[role] = "active: " + truncateText(strings.TrimSpace(task.Goal), 52)
	}
	for _, task := range completedTasks {
		role := strings.TrimSpace(task.AssignedRole)
		if role == "" {
			role = "(coordinator)"
		}
		if runID := strings.TrimSpace(task.RunID); runID != "" {
			if !manifestRoster {
				if _, ok := roleByRunID[runID]; !ok {
					roleByRunID[runID] = role
				}
				runIDSet[runID] = struct{}{}
			} else if _, ok := runIDSet[runID]; ok {
				if _, seenRole := roleByRunID[runID]; !seenRole {
					roleByRunID[runID] = role
				}
			}
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
	totalTokensIn := 0
	totalTokensOut := 0
	statsTotalTokens := 0
	totalCostUSD := 0.0
	pricingKnown := true
	for _, runID := range runIDs {
		if s.session != nil {
			if run, err := s.session.LoadRun(ctx, runID); err == nil {
				if sessionID := strings.TrimSpace(run.SessionID); sessionID != "" {
					if sess, serr := s.session.LoadSession(ctx, sessionID); serr == nil {
						totalTokensIn += sess.InputTokens
						totalTokensOut += sess.OutputTokens
					}
				}
			}
		}
		stats, err := s.taskService.GetRunStats(ctx, runID)
		if err != nil {
			continue
		}
		statsTotalTokens += stats.TotalTokens
		totalCostUSD += stats.TotalCost
		if stats.TotalTokens > 0 && stats.TotalCost <= 0 && !pricingKnownForRun(ctx, s.session, runID) {
			pricingKnown = false
		}
	}
	totalTokens := totalTokensIn + totalTokensOut
	if totalTokens == 0 {
		totalTokens = statsTotalTokens
	}
	if totalTokens == 0 {
		pricingKnown = true
	}
	return protocol.TeamGetStatusResult{
		Pending:        pending,
		Active:         active,
		Done:           done,
		Roles:          roles,
		RunIDs:         runIDs,
		RoleByRunID:    roleByRunID,
		TotalTokensIn:  totalTokensIn,
		TotalTokensOut: totalTokensOut,
		TotalTokens:    totalTokens,
		TotalCostUSD:   totalCostUSD,
		PricingKnown:   pricingKnown,
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
	manifest, err := s.manifestStore.Load(ctx, teamID)
	if err != nil {
		return protocol.TeamGetManifestResult{}, err
	}
	if manifest == nil {
		return protocol.TeamGetManifestResult{}, &protocol.ProtocolError{Code: protocol.CodeItemNotFound, Message: "team manifest not found"}
	}
	roles := make([]protocol.TeamManifestRole, len(manifest.Roles))
	for i, r := range manifest.Roles {
		roles[i] = protocol.TeamManifestRole{
			RoleName:  strings.TrimSpace(r.RoleName),
			RunID:     strings.TrimSpace(r.RunID),
			SessionID: strings.TrimSpace(r.SessionID),
		}
	}
	var modelChange *protocol.TeamManifestModelChange
	if manifest.ModelChange != nil {
		mc := manifest.ModelChange
		modelChange = &protocol.TeamManifestModelChange{
			RequestedModel: strings.TrimSpace(mc.RequestedModel),
			Status:         strings.TrimSpace(mc.Status),
			RequestedAt:    strings.TrimSpace(mc.RequestedAt),
			AppliedAt:      strings.TrimSpace(mc.AppliedAt),
			Reason:         strings.TrimSpace(mc.Reason),
			Error:          strings.TrimSpace(mc.Error),
		}
	}
	return protocol.TeamGetManifestResult{
		TeamID:          strings.TrimSpace(manifest.TeamID),
		ProfileID:       strings.TrimSpace(manifest.ProfileID),
		TeamModel:       strings.TrimSpace(manifest.TeamModel),
		ModelChange:     modelChange,
		CoordinatorRole: strings.TrimSpace(manifest.CoordinatorRole),
		ReviewerRole:    s.resolveManifestReviewerRole(manifest.ProfileID, manifest.CoordinatorRole),
		CoordinatorRun:  strings.TrimSpace(manifest.CoordinatorRun),
		Roles:           roles,
		CreatedAt:       strings.TrimSpace(manifest.CreatedAt),
	}, nil
}

func (s *RPCServer) resolveManifestReviewerRole(profileID, coordinatorRole string) string {
	coordinatorRole = strings.TrimSpace(coordinatorRole)
	prof, _, err := resolveProfileRef(s.cfg, strings.TrimSpace(profileID))
	if err != nil || prof == nil {
		return coordinatorRole
	}
	roles, _ := prof.RolesForSession()
	if _, profileCoordinatorRole, verr := team.ValidateTeamRoles(roles); verr == nil {
		coordinatorRole = strings.TrimSpace(profileCoordinatorRole)
	}
	if reviewerCfg, ok := prof.ReviewerForSession(); ok && reviewerCfg != nil {
		if reviewerName := strings.TrimSpace(reviewerCfg.EffectiveName()); reviewerName != "" {
			return reviewerName
		}
	}
	return coordinatorRole
}

func (s *RPCServer) readPlanFilesForRun(ctx context.Context, run types.Run) (checklist string, checklistErr string, details string, detailsErr string) {
	checklist, details, checklistErr, detailsErr = s.planReader.ReadPlan(ctx, run)
	return checklist, checklistErr, details, detailsErr
}

func (s *RPCServer) loadTeamManifestRunRoles(ctx context.Context, teamID string) ([]string, map[string]string) {
	return loadTeamManifestRunRolesFromStore(ctx, s.manifestStore, teamID)
}

// loadTeamManifestRunRolesFromStore returns run IDs and role-by-run from a manifest store.
// Used by RPC and daemon when a ManifestStore is available.
func loadTeamManifestRunRolesFromStore(ctx context.Context, store team.ManifestStore, teamID string) ([]string, map[string]string) {
	teamID = strings.TrimSpace(teamID)
	if teamID == "" {
		return nil, map[string]string{}
	}
	manifest, err := store.Load(ctx, teamID)
	if err != nil || manifest == nil {
		return nil, map[string]string{}
	}
	runIDs := make([]string, 0, len(manifest.Roles)+1)
	roleByRun := make(map[string]string, len(manifest.Roles)+1)
	seen := map[string]struct{}{}
	if runID := strings.TrimSpace(manifest.CoordinatorRun); runID != "" {
		runIDs = append(runIDs, runID)
		seen[runID] = struct{}{}
	}
	for _, role := range manifest.Roles {
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
		run, _ := s.session.LoadRun(ctx, runID)
		if run.RunID == "" {
			run = types.Run{RunID: runID}
		}
		checklist, checklistErr, details, detailsErr := s.readPlanFilesForRun(ctx, run)
		return protocol.PlanGetResult{
			Checklist: checklist, ChecklistErr: checklistErr, Details: details, DetailsErr: detailsErr, SourceRuns: []string{runID},
		}, nil
	}
	if strings.TrimSpace(scope.runID) != "" {
		runID := strings.TrimSpace(scope.runID)
		run, _ := s.session.LoadRun(ctx, runID)
		if run.RunID == "" {
			run = types.Run{RunID: runID}
		}
		checklist, checklistErr, details, detailsErr := s.readPlanFilesForRun(ctx, run)
		return protocol.PlanGetResult{
			Checklist: checklist, ChecklistErr: checklistErr, Details: details, DetailsErr: detailsErr, SourceRuns: []string{runID},
		}, nil
	}
	aggregate := p.AggregateTeam
	if !aggregate {
		return protocol.PlanGetResult{}, &protocol.ProtocolError{Code: protocol.CodeInvalidParams, Message: "aggregateTeam must be true when runId is omitted in team mode"}
	}
	manifestRunIDs, roleByRun := s.loadTeamManifestRunRoles(ctx, strings.TrimSpace(scope.teamID))
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
	if len(runIDs) == 0 {
		tasks, _ := s.taskService.ListTasks(ctx, state.TaskFilter{TeamID: strings.TrimSpace(scope.teamID), Limit: 1000, SortBy: "created_at", SortDesc: true})
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
		run, _ := s.session.LoadRun(ctx, runID)
		if run.RunID == "" {
			run = types.Run{RunID: runID}
		}
		check, checkErr, det, detErr := s.readPlanFilesForRun(ctx, run)
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
	providerFilter := strings.ToLower(strings.TrimSpace(p.Provider))
	query := strings.ToLower(strings.TrimSpace(p.Query))
	infos := cost.SupportedModelInfos()
	if shouldUseOpenRouterModelCatalog() {
		if dynamic, ok := cost.OpenRouterModelInfos(ctx); ok && len(dynamic) > 0 {
			infos = dynamic
		}
	}
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

func shouldUseOpenRouterModelCatalog() bool {
	if strings.TrimSpace(os.Getenv("OPENROUTER_API_KEY")) == "" {
		return false
	}
	baseURL := strings.ToLower(strings.TrimSpace(os.Getenv("OPENROUTER_BASE_URL")))
	if baseURL == "" {
		// Default OpenRouter endpoint used throughout runtime when unset.
		return true
	}
	return strings.Contains(baseURL, "openrouter.ai")
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
