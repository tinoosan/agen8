package app

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	implstore "github.com/tinoosan/agen8/internal/store"
	"github.com/tinoosan/agen8/pkg/config"
	"github.com/tinoosan/agen8/pkg/protocol"
	"github.com/tinoosan/agen8/pkg/services/team"
	"github.com/tinoosan/agen8/pkg/store"
	"github.com/tinoosan/agen8/pkg/types"
)

type benchmarkManifestStore struct {
	manifest team.Manifest
}

func (s *benchmarkManifestStore) Load(_ context.Context, teamID string) (*team.Manifest, error) {
	teamID = strings.TrimSpace(teamID)
	if teamID == "" || teamID != strings.TrimSpace(s.manifest.TeamID) {
		return nil, fmt.Errorf("manifest not found")
	}
	copy := s.manifest
	return &copy, nil
}

func (s *benchmarkManifestStore) Save(_ context.Context, manifest team.Manifest) error {
	s.manifest = manifest
	return nil
}

func BenchmarkSessionList_1kSessions5Runs(b *testing.B) {
	const (
		sessionCount   = 1000
		runsPerSession = 5
	)

	ctx := context.Background()
	cfg := config.Config{DataDir: b.TempDir()}
	sessStore := store.NewMemorySessionStore()

	rootSession := types.NewSession("benchmark-root")
	rootSession.System = true
	rootRun := types.NewRun("benchmark-root", 8*1024, rootSession.SessionID)
	rootSession.CurrentRunID = rootRun.RunID
	rootSession.Runs = []string{rootRun.RunID}
	if err := sessStore.SaveSession(ctx, rootSession); err != nil {
		b.Fatalf("save root session: %v", err)
	}
	if err := sessStore.SaveRun(ctx, rootRun); err != nil {
		b.Fatalf("save root run: %v", err)
	}

	statuses := []string{
		types.RunStatusRunning,
		types.RunStatusPaused,
		types.RunStatusSucceeded,
	}
	for i := range sessionCount {
		sess := types.NewSession(fmt.Sprintf("goal-%04d", i))
		sess.Mode = "multi-agent"
		runIDs := make([]string, 0, runsPerSession)
		for j := range runsPerSession {
			run := types.NewRun(fmt.Sprintf("goal-%04d-%d", i, j), 8*1024, sess.SessionID)
			run.Status = statuses[j%len(statuses)]
			run.Runtime = &types.RunRuntimeConfig{
				TeamID:  fmt.Sprintf("team-%02d", i%50),
				Profile: "market_researcher",
				Role:    fmt.Sprintf("role-%d", j),
			}
			if err := sessStore.SaveRun(ctx, run); err != nil {
				b.Fatalf("save run %d/%d: %v", i, j, err)
			}
			runIDs = append(runIDs, run.RunID)
		}
		sess.Runs = runIDs
		sess.CurrentRunID = runIDs[0]
		sess.TeamID = fmt.Sprintf("team-%02d", i%50)
		sess.Profile = "market_researcher"
		if err := sessStore.SaveSession(ctx, sess); err != nil {
			b.Fatalf("save session %d: %v", i, err)
		}
	}

	srv := NewRPCServer(RPCServerConfig{
		Cfg:     cfg,
		Run:     rootRun,
		Session: newTestSessionService(cfg, sessStore),
		Index:   protocol.NewIndex(0, 0),
	})
	params := protocol.SessionListParams{
		ThreadID: protocol.ThreadID(rootSession.SessionID),
		Limit:    500,
		Offset:   0,
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		out, err := srv.sessionList(ctx, params)
		if err != nil {
			b.Fatalf("sessionList: %v", err)
		}
		if out.TotalCount < sessionCount {
			b.Fatalf("totalCount=%d want >=%d", out.TotalCount, sessionCount)
		}
		if len(out.Sessions) == 0 {
			b.Fatalf("sessions page unexpectedly empty")
		}
	}
}

func BenchmarkActivityList_50Runs10kActivities(b *testing.B) {
	const (
		runCount         = 50
		activitiesPerRun = 200
		pageLimit        = 200
		expectedTotal    = runCount * activitiesPerRun
	)

	ctx := context.Background()
	cfg := config.Config{DataDir: b.TempDir()}
	sessStore, err := implstore.NewSQLiteSessionStore(cfg)
	if err != nil {
		b.Fatalf("new sqlite session store: %v", err)
	}

	rootSession := types.NewSession("benchmark-root")
	rootRun := types.NewRun("benchmark-root", 8*1024, rootSession.SessionID)
	rootSession.CurrentRunID = rootRun.RunID
	rootSession.Runs = []string{rootRun.RunID}
	if err := sessStore.SaveSession(ctx, rootSession); err != nil {
		b.Fatalf("save root session: %v", err)
	}
	if err := sessStore.SaveRun(ctx, rootRun); err != nil {
		b.Fatalf("save root run: %v", err)
	}

	teamID := "team-bench"
	runIDs := make([]string, 0, runCount)
	roles := make([]team.RoleRecord, 0, runCount)
	for i := range runCount {
		run := types.NewRun(fmt.Sprintf("bench-run-%02d", i), 8*1024, rootSession.SessionID)
		role := fmt.Sprintf("role-%02d", i)
		run.Runtime = &types.RunRuntimeConfig{
			TeamID:  teamID,
			Profile: "market_researcher",
			Role:    role,
		}
		if err := sessStore.SaveRun(ctx, run); err != nil {
			b.Fatalf("save team run %d: %v", i, err)
		}
		runIDs = append(runIDs, run.RunID)
		roles = append(roles, team.RoleRecord{RoleName: role, RunID: run.RunID, SessionID: rootSession.SessionID})
	}
	rootSession.Runs = append(rootSession.Runs, runIDs...)
	if err := sessStore.SaveSession(ctx, rootSession); err != nil {
		b.Fatalf("save root session with runs: %v", err)
	}

	manifestStore := &benchmarkManifestStore{
		manifest: team.BuildManifest(
			teamID,
			"benchmark-profile",
			"role-00",
			runIDs[0],
			"openai/gpt-5-mini",
			roles,
			nil,
			time.Now().UTC().Format(time.RFC3339Nano),
		),
	}

	baseTime := time.Now().UTC().Add(-time.Hour)
	seq := 0
	for runIdx, runID := range runIDs {
		for eventIdx := range activitiesPerRun {
			seq++
			if err := implstore.AppendEvent(ctx, cfg, types.EventRecord{
				RunID:     runID,
				Timestamp: baseTime.Add(time.Duration(seq) * time.Millisecond),
				Type:      "agent.op.request",
				Message:   fmt.Sprintf("run-%02d activity-%03d", runIdx, eventIdx),
				Data: map[string]string{
					"opId": fmt.Sprintf("%02d-%03d", runIdx, eventIdx),
					"op":   "fs_read",
					"path": fmt.Sprintf("/workspace/%02d/%03d.txt", runIdx, eventIdx),
				},
			}); err != nil {
				b.Fatalf("append event run=%s idx=%d: %v", runID, eventIdx, err)
			}
		}
	}

	srv := NewRPCServer(RPCServerConfig{
		Cfg:           cfg,
		Run:           rootRun,
		Session:       newTestSessionService(cfg, sessStore),
		ManifestStore: manifestStore,
		Index:         protocol.NewIndex(0, 0),
	})
	params := protocol.ActivityListParams{
		ThreadID: protocol.ThreadID(rootSession.SessionID),
		TeamID:   teamID,
		Limit:    pageLimit,
		SortDesc: true,
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		out, err := srv.activityList(ctx, params)
		if err != nil {
			b.Fatalf("activityList: %v", err)
		}
		if out.TotalCount != expectedTotal {
			b.Fatalf("totalCount=%d want %d", out.TotalCount, expectedTotal)
		}
		if len(out.Activities) != pageLimit {
			b.Fatalf("page size=%d want %d", len(out.Activities), pageLimit)
		}
	}
}
