package runtime

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/tinoosan/workbench-core/pkg/agent"
	"github.com/tinoosan/workbench-core/pkg/config"
	"github.com/tinoosan/workbench-core/pkg/events"
	"github.com/tinoosan/workbench-core/pkg/fsutil"
	"github.com/tinoosan/workbench-core/pkg/profile"
	"github.com/tinoosan/workbench-core/pkg/resources"
	"github.com/tinoosan/workbench-core/pkg/skills"
	"github.com/tinoosan/workbench-core/pkg/store"
	"github.com/tinoosan/workbench-core/pkg/tools/builtins"
	"github.com/tinoosan/workbench-core/pkg/types"
	"github.com/tinoosan/workbench-core/pkg/vfs"
)

type Runtime struct {
	FS              *vfs.FS
	Executor        agent.HostExecutor
	TraceMiddleware *agent.TraceMiddleware
	Constructor     *agent.PromptBuilder
	Updater         *agent.PromptUpdater
	WorkdirBase     string
	MemStore        store.DailyMemoryStore

	Browser agent.BrowserManager

	browserCleanupCancel context.CancelFunc
}

type BuildConfig struct {
	Cfg                   config.Config
	Run                   types.Run
	Profile               string
	ProfileConfig         *profile.Profile
	WorkdirAbs            string
	SharedWorkspaceDir    string
	Model                 string
	ReasoningEffort       string
	ReasoningSummary      string
	ApprovalsMode         string
	HistoryStore          store.HistoryStore
	MemoryStore           store.DailyMemoryStore
	TraceStore            store.TraceStore
	ConstructorStore      store.ConstructorStateStore
	Emit                  func(ctx context.Context, ev events.Event)
	IncludeHistoryOps     bool
	RecentHistoryPairs    int
	MaxMemoryBytes        int
	MaxTraceBytes         int
	PriceInPerMTokensUSD  float64
	PriceOutPerMTokensUSD float64
	AuditReads            bool
	Guard                 func(fs *vfs.FS, req types.HostOpRequest) *types.HostOpResponse
	ArtifactObserve       func(path string)
	PersistRun            func(run types.Run) error
	LoadSession           func(sessionID string) (types.Session, error)
	SaveSession           func(session types.Session) error
}

func (cfg BuildConfig) Validate() error {
	if err := cfg.Cfg.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(cfg.WorkdirAbs) == "" {
		return fmt.Errorf("workdir is required")
	}
	if cfg.MemoryStore == nil {
		return fmt.Errorf("memory store is required")
	}
	if cfg.HistoryStore == nil {
		return fmt.Errorf("history store is required")
	}
	if cfg.TraceStore == nil {
		return fmt.Errorf("trace store is required")
	}
	if cfg.ConstructorStore == nil {
		return fmt.Errorf("constructor store is required")
	}
	return nil
}

func Build(cfg BuildConfig) (*Runtime, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	run := cfg.Run
	run.Runtime = &types.RunRuntimeConfig{
		DataDir:          cfg.Cfg.DataDir,
		Profile:          strings.TrimSpace(cfg.Profile),
		Model:            cfg.Model,
		ReasoningEffort:  strings.TrimSpace(cfg.ReasoningEffort),
		ReasoningSummary: strings.TrimSpace(cfg.ReasoningSummary),
		ApprovalsMode:    strings.TrimSpace(cfg.ApprovalsMode),

		MaxTraceBytes:         cfg.MaxTraceBytes,
		MaxMemoryBytes:        cfg.MaxMemoryBytes,
		RecentHistoryPairs:    cfg.RecentHistoryPairs,
		IncludeHistoryOps:     cfg.IncludeHistoryOps,
		PriceInPerMTokensUSD:  cfg.PriceInPerMTokensUSD,
		PriceOutPerMTokensUSD: cfg.PriceOutPerMTokensUSD,
	}
	if cfg.PersistRun != nil {
		if err := cfg.PersistRun(run); err != nil && cfg.Emit != nil {
			cfg.Emit(context.Background(), events.Event{
				Type:    "runtime.warning",
				Message: "Failed to persist run state",
				Data:    map[string]string{"error": err.Error()},
			})
		}
	}

	if cfg.LoadSession != nil {
		if sess, err := cfg.LoadSession(run.SessionID); err == nil {
			changed := false
			if strings.TrimSpace(sess.ActiveModel) != strings.TrimSpace(cfg.Model) {
				sess.ActiveModel = strings.TrimSpace(cfg.Model)
				changed = true
			}
			if strings.TrimSpace(sess.ReasoningEffort) != strings.TrimSpace(cfg.ReasoningEffort) {
				sess.ReasoningEffort = strings.TrimSpace(cfg.ReasoningEffort)
				changed = true
			}
			if strings.TrimSpace(sess.ReasoningSummary) != strings.TrimSpace(cfg.ReasoningSummary) {
				sess.ReasoningSummary = strings.TrimSpace(cfg.ReasoningSummary)
				changed = true
			}
			approvalMode := strings.TrimSpace(cfg.ApprovalsMode)
			if approvalMode == "" {
				// Autonomous-first: approvals are disabled by default.
				approvalMode = "disabled"
			}
			if strings.TrimSpace(sess.ApprovalsMode) != approvalMode {
				sess.ApprovalsMode = approvalMode
				changed = true
			}
			if changed && cfg.SaveSession != nil {
				if err := cfg.SaveSession(sess); err != nil && cfg.Emit != nil {
					cfg.Emit(context.Background(), events.Event{
						Type:    "runtime.warning",
						Message: "Failed to persist session state",
						Data:    map[string]string{"error": err.Error()},
					})
				}
			}
		}
	}

	fs := vfs.NewFS()

	workdirRes, err := resources.NewWorkdirResource(cfg.WorkdirAbs)
	if err != nil {
		return nil, fmt.Errorf("create workdir: %w", err)
	}
	if err := fs.Mount(vfs.MountProject, workdirRes); err != nil {
		return nil, fmt.Errorf("mount %s: %w", vfs.MountProject, err)
	}

	runDir := fsutil.GetAgentDir(cfg.Cfg.DataDir, cfg.Run.RunID)
	var wsRes *resources.DirResource
	sharedWorkspaceDir := strings.TrimSpace(cfg.SharedWorkspaceDir)
	if sharedWorkspaceDir != "" {
		if err := os.MkdirAll(sharedWorkspaceDir, 0o755); err != nil {
			return nil, fmt.Errorf("prepare shared workspace dir: %w", err)
		}
		wsRes, err = resources.NewDirResource(sharedWorkspaceDir, vfs.MountWorkspace)
		if err != nil {
			return nil, fmt.Errorf("create shared workspace resource: %w", err)
		}
	} else {
		wsRes, err = resources.NewWorkspace(cfg.Cfg, cfg.Run.RunID)
		if err != nil {
			return nil, fmt.Errorf("create workspace resource: %w", err)
		}
	}
	if err := fs.Mount(vfs.MountWorkspace, wsRes); err != nil {
		return nil, fmt.Errorf("mount %s: %w", vfs.MountWorkspace, err)
	}

	planDir := filepath.Join(runDir, "plan")
	if err := os.MkdirAll(planDir, 0755); err != nil {
		return nil, fmt.Errorf("prepare plan dir: %w", err)
	}
	ensureFile := func(path string, contents string) error {
		if st, err := os.Stat(path); err == nil {
			if st.IsDir() {
				return fmt.Errorf("path %s is a directory", path)
			}
			return nil
		} else if !os.IsNotExist(err) {
			return err
		}
		return os.WriteFile(path, []byte(contents), 0644)
	}
	if err := ensureFile(filepath.Join(planDir, "HEAD.md"), "# Current Plan\n\n_No active plan._\n"); err != nil {
		return nil, fmt.Errorf("prepare plan details: %w", err)
	}
	if err := ensureFile(filepath.Join(planDir, "CHECKLIST.md"), "# Task Checklist\n\n_No tasks yet._\n"); err != nil {
		return nil, fmt.Errorf("prepare plan checklist: %w", err)
	}
	planRes, err := resources.NewDirResource(planDir, vfs.MountPlan)
	if err != nil {
		return nil, fmt.Errorf("create plan resource: %w", err)
	}
	if err := fs.Mount(vfs.MountPlan, planRes); err != nil {
		return nil, fmt.Errorf("mount %s: %w", vfs.MountPlan, err)
	}

	skillDir, err := fsutil.GetAgentsSkillsDir()
	if err != nil {
		return nil, fmt.Errorf("resolve skills dir: %w", err)
	}
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		return nil, fmt.Errorf("prepare skills dir: %w", err)
	}
	skillMgr := skills.NewManager([]string{skillDir})
	skillMgr.WritableRoot = skillDir
	if profileSkills := resolveProfileSkills(cfg); len(profileSkills) > 0 {
		skillMgr.AllowedSkills = profileSkills
		if cfg.Emit != nil {
			cfg.Emit(context.Background(), events.Event{
				Type:    "runtime.info",
				Message: "Applied profile-scoped skills visibility",
				Data: map[string]string{
					"profile":      strings.TrimSpace(cfg.Profile),
					"allowedCount": strconv.Itoa(len(profileSkills)),
				},
			})
		}
	}
	if err := skillMgr.Scan(); err != nil {
		return nil, fmt.Errorf("scan skills: %w", err)
	}
	if err := fs.Mount(vfs.MountSkills, skills.NewResource(skillMgr)); err != nil {
		return nil, fmt.Errorf("mount %s: %w", vfs.MountSkills, err)
	}

	memStore := cfg.MemoryStore

	if memStore == nil {
		return nil, fmt.Errorf("memory store is required")
	}
	memRes, err := resources.NewDailyMemoryResource(fsutil.GetMemoryDir(cfg.Cfg.DataDir))
	if err != nil {
		return nil, fmt.Errorf("create memory resource: %w", err)
	}
	if err := fs.Mount(vfs.MountMemory, memRes); err != nil {
		return nil, fmt.Errorf("mount %s: %w", vfs.MountMemory, err)
	}

	if cfg.Emit != nil {
		cfg.Emit(context.Background(), events.Event{
			Type:    "host.mounted",
			Message: "Mounted VFS resources",
			Data: map[string]string{
				"/project":   workdirRes.BaseDir,
				"/workspace": wsRes.BaseDir,
				"/plan":      planDir,
				"/skills":    "(virtual)",
				"/memory":    "(virtual)",
			},
			Console: boolPtr(false),
		})
	}

	absWorkdirRoot, err := filepath.Abs(workdirRes.BaseDir)
	if err != nil {
		return nil, fmt.Errorf("resolve workdir root: %w", err)
	}

	traceStore := cfg.TraceStore
	traceMiddleware := &agent.TraceMiddleware{
		Store: traceStore,
		FS:    fs,
	}
	shellInvoker := builtins.NewBuiltinShellInvoker(absWorkdirRoot, nil, vfs.MountProject)
	shellInvoker.MountRoots[vfs.MountWorkspace] = wsRes.BaseDir
	shellInvoker.MountRoots[vfs.MountSkills] = skillDir
	shellInvoker.MountRoots[vfs.MountPlan] = planDir
	shellInvoker.MountRoots[vfs.MountMemory] = memRes.BaseDir
	if raw := strings.TrimSpace(os.Getenv("WORKBENCH_SHELL_VFS_TRANSLATION")); raw != "" {
		switch strings.ToLower(raw) {
		case "0", "false", "off", "no":
			shellInvoker.EnableVFSPathTranslation = false
		}
	}

	httpInvoker := builtins.NewBuiltinHTTPInvoker()
	traceInvoker := builtins.BuiltinTraceInvoker{Store: traceStore}

	browserMgr, err := builtins.NewBrowserSessionManager(30 * time.Minute)
	if err != nil {
		return nil, fmt.Errorf("create browser manager: %w", err)
	}

	// Initialize email sender (optional; send-only).
	//
	// Gmail OAuth2 (XOAUTH2) configuration (no password-based SMTP for now).
	dotEnv := loadDotEnv(cfg.WorkdirAbs)
	var emailClient builtins.EmailSender
	{
		user := envWithFallback("GMAIL_USER", dotEnv, "GMAIL_SMTP_USER", "GMAIL_USERNAME", "GMAIL_EMAIL")
		from := envWithFallback("GMAIL_FROM", dotEnv)
		clientID := envWithFallback("GOOGLE_OAUTH_CLIENT_ID", dotEnv)
		clientSecret := envWithFallback("GOOGLE_OAUTH_CLIENT_SECRET", dotEnv)
		refreshToken := envWithFallback("GOOGLE_OAUTH_REFRESH_TOKEN", dotEnv)
		accessToken := envWithFallback("GOOGLE_OAUTH_ACCESS_TOKEN", dotEnv)

		port := 587
		if portStr := envWithFallback("GMAIL_SMTP_PORT", dotEnv); portStr != "" {
			if p, err := strconv.Atoi(portStr); err == nil && p > 0 {
				port = p
			}
		}

		if strings.TrimSpace(user) != "" && (accessToken != "" || (clientID != "" && clientSecret != "" && refreshToken != "")) {
			c, err := builtins.NewGmailOAuthClient(builtins.GmailOAuthConfig{
				User:         user,
				From:         from,
				ClientID:     clientID,
				ClientSecret: clientSecret,
				RefreshToken: refreshToken,
				AccessToken:  accessToken,
				Host:         "smtp.gmail.com",
				Port:         port,
			})
			if err != nil {
				if cfg.Emit != nil {
					cfg.Emit(context.Background(), events.Event{
						Type:    "runtime.warning",
						Message: "Email configured but OAuth setup invalid; email disabled",
						Data:    map[string]string{"error": err.Error()},
					})
				}
			} else {
				emailClient = c
			}
		} else if strings.TrimSpace(user) != "" || clientID != "" || clientSecret != "" || refreshToken != "" || accessToken != "" {
			if cfg.Emit != nil {
				cfg.Emit(context.Background(), events.Event{
					Type:    "runtime.warning",
					Message: "Email configured but missing required OAuth values; email disabled",
					Data: map[string]string{
						"requires": "GMAIL_USER and GOOGLE_OAUTH_CLIENT_ID/GOOGLE_OAUTH_CLIENT_SECRET/GOOGLE_OAUTH_REFRESH_TOKEN (or GOOGLE_OAUTH_ACCESS_TOKEN)",
					},
				})
			}
		}
	}

	executor := &agent.HostOpExecutor{
		FS:              fs,
		ShellInvoker:    shellInvoker,
		HTTPInvoker:     httpInvoker,
		TraceInvoker:    traceInvoker,
		EmailClient:     emailClient,
		Browser:         browserMgr,
		WorkspaceDir:    wsRes.BaseDir,
		ProjectDir:      absWorkdirRoot,
		DefaultMaxBytes: 4096,
		MaxReadBytes:    256 * 1024,
	}

	// Periodically cleanup idle browser sessions.
	cleanupCtx, cleanupCancel := context.WithCancel(context.Background())
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-cleanupCtx.Done():
				return
			case <-ticker.C:
				browserMgr.CleanupStale()
			}
		}
	}()

	constructor := &agent.PromptBuilder{
		FS:             fs,
		Skills:         skillMgr,
		MaxMemoryBytes: cfg.MaxMemoryBytes,
		Emit:           cfg.Emit,
	}

	updater := &agent.PromptUpdater{
		FS:             fs,
		Trace:          traceMiddleware,
		MaxMemoryBytes: cfg.MaxMemoryBytes,
		MaxTraceBytes:  cfg.MaxTraceBytes,
		Emit:           cfg.Emit,
		ManifestPath:   "/workspace/context_constructor_manifest.json",
	}

	auditObs := newAuditObserver(cfg.Run.RunID, cfg.Emit, cfg.AuditReads)

	exec := NewExecutor(executor, ExecutorOptions{
		Emit:      cfg.Emit,
		Model:     cfg.Model,
		RunID:     cfg.Run.RunID,
		SessionID: cfg.Run.SessionID,
		FS:        fs,
		Guard: func(req types.HostOpRequest) *types.HostOpResponse {
			if cfg.Guard == nil {
				return nil
			}
			return cfg.Guard(fs, req)
		},
		Observers:       []HostOpObserver{constructor, updater, auditObs},
		ArtifactObserve: cfg.ArtifactObserve,
	})

	return &Runtime{
		FS:                   fs,
		Executor:             exec,
		TraceMiddleware:      traceMiddleware,
		Constructor:          constructor,
		Updater:              updater,
		WorkdirBase:          workdirRes.BaseDir,
		MemStore:             memStore,
		Browser:              browserMgr,
		browserCleanupCancel: cleanupCancel,
	}, nil
}

func boolPtr(v bool) *bool { return &v }

func resolveProfileSkills(cfg BuildConfig) []string {
	if cfg.ProfileConfig != nil {
		return uniqueProfileSkills(cfg.ProfileConfig)
	}

	ref := strings.TrimSpace(cfg.Profile)
	if ref == "" {
		return nil
	}

	p, err := loadProfileByRef(cfg.Cfg, ref)
	if err != nil || p == nil {
		return nil
	}
	return uniqueProfileSkills(p)
}

func loadProfileByRef(cfg config.Config, ref string) (*profile.Profile, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil, nil
	}
	if st, err := os.Stat(ref); err == nil {
		if st.IsDir() {
			return profile.Load(ref)
		}
		return profile.Load(ref)
	}
	return profile.Load(filepath.Join(fsutil.GetProfilesDir(cfg.DataDir), ref))
}

func uniqueProfileSkills(p *profile.Profile) []string {
	if p == nil {
		return nil
	}
	out := make([]string, 0, len(p.Skills))
	seen := map[string]struct{}{}
	add := func(skills []string) {
		for _, s := range skills {
			s = strings.TrimSpace(s)
			if s == "" {
				continue
			}
			if _, ok := seen[s]; ok {
				continue
			}
			seen[s] = struct{}{}
			out = append(out, s)
		}
	}

	add(p.Skills)
	if p.Team != nil {
		for _, role := range p.Team.Roles {
			add(role.Skills)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func loadDotEnv(root string) map[string]string {
	if strings.TrimSpace(root) == "" {
		return nil
	}
	path := filepath.Join(root, ".env")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	out := make(map[string]string)
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		if key == "" {
			continue
		}
		out[key] = value
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func envWithFallback(key string, dotEnv map[string]string, aliases ...string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	for _, alias := range aliases {
		if v := strings.TrimSpace(os.Getenv(alias)); v != "" {
			return v
		}
	}
	if dotEnv == nil {
		return ""
	}
	if v, ok := dotEnv[key]; ok && strings.TrimSpace(v) != "" {
		return strings.TrimSpace(v)
	}
	for _, alias := range aliases {
		if v, ok := dotEnv[alias]; ok && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func (r *Runtime) Shutdown(_ context.Context) error {
	if r == nil {
		return nil
	}
	if r.browserCleanupCancel != nil {
		r.browserCleanupCancel()
	}
	if r.Browser != nil {
		return r.Browser.Shutdown()
	}
	return nil
}
