package daemon

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/tinoosan/agen8/internal/core/types"
	"github.com/tinoosan/agen8/internal/mcp"
	projecttool "github.com/tinoosan/agen8/internal/mcp/tools/project"
	authapp "github.com/tinoosan/agen8/internal/services/auth/app"
	credentialapp "github.com/tinoosan/agen8/internal/services/credential/app"
	decisionapp "github.com/tinoosan/agen8/internal/services/decision/app"
	fileapp "github.com/tinoosan/agen8/internal/services/file/app"
	graphapp "github.com/tinoosan/agen8/internal/services/graph/app"
	missionapp "github.com/tinoosan/agen8/internal/services/mission/app"
	projectapp "github.com/tinoosan/agen8/internal/services/project/app"
	"github.com/tinoosan/agen8/internal/services/project/domain/member"
	taskapp "github.com/tinoosan/agen8/internal/services/task/app"
	userapp "github.com/tinoosan/agen8/internal/services/user/app"
)

type mcpSessionResolverConfig struct {
	tokenStore         *mcp.TokenStore
	auth               *authapp.Service
	users              *userapp.Service
	projects           *projectapp.Service
	decisions          *decisionapp.Service
	graph              *graphapp.Service
	credentials        *credentialapp.Service
	tasks              *taskapp.Service
	files              *fileapp.Service
	missions           *missionapp.Service
	projectProvisioner *projectHooksProvisioner
	externalBaseURL    string
}

type mcpSessionResolver struct {
	tokenStore         *mcp.TokenStore
	auth               *authapp.Service
	users              *userapp.Service
	projects           *projectapp.Service
	decisions          *decisionapp.Service
	graph              *graphapp.Service
	credentials        *credentialapp.Service
	tasks              *taskapp.Service
	files              *fileapp.Service
	missions           *missionapp.Service
	projectProvisioner *projectHooksProvisioner
	externalBaseURL    string
}

func newMCPSessionResolver(cfg mcpSessionResolverConfig) *mcpSessionResolver {
	return &mcpSessionResolver{
		tokenStore:         cfg.tokenStore,
		auth:               cfg.auth,
		users:              cfg.users,
		projects:           cfg.projects,
		decisions:          cfg.decisions,
		graph:              cfg.graph,
		credentials:        cfg.credentials,
		tasks:              cfg.tasks,
		files:              cfg.files,
		missions:           cfg.missions,
		projectProvisioner: cfg.projectProvisioner,
		externalBaseURL:    cfg.externalBaseURL,
	}
}

func (r *mcpSessionResolver) Resolve(ctx context.Context, token string, header http.Header, body []byte) (mcp.Session, error) {
	session, err := r.resolveToken(ctx, token)
	if err != nil {
		return mcp.Session{}, err
	}

	sessionID, threadID := sessionRefs(header, body)
	if strings.TrimSpace(session.HarnessKind) == "" {
		nativeRef := sessionID
		if nativeRef == "" {
			nativeRef = threadID
		}
		session.HarnessKind = mcp.HarnessFromJSONRPCBody(body, nativeRef)
	}
	if sessionID == "" && threadID == "" {
		return session, nil
	}

	rosterMember, err := r.projects.ResolveMCPContext(ctx, projectapp.ResolveMCPContextInput{
		Token:     token,
		UserID:    r.userID(ctx, token),
		ProjectID: session.ProjectID,
		// Resolve harness-agnostically. A shared user-scoped API key or link token can
		// serve more than one harness, so the token's own HarnessKind must not filter
		// the lookup. The resolved member's real HarnessKind is restored below.
		HarnessKind: "",
		SessionID:   sessionID,
		ThreadID:    threadID,
	})
	if err != nil {
		if errors.Is(err, member.ErrNotFound) {
			return session, nil
		}
		return mcp.Session{}, err
	}
	return sessionWithMember(session, rosterMember), nil
}

func (r *mcpSessionResolver) resolveToken(ctx context.Context, token string) (mcp.Session, error) {
	session, err := r.tokenStore.Resolve(token)
	if err == nil {
		return session, nil
	}

	// Token resolution priority: bootstrap/web-registered tokens above, then wlt_
	// link tokens, then ak_ API keys. A wlt_ token is recognised first and binds the
	// session to a project server-side; an invalid one fails loudly rather than
	// falling through to the API-key path.
	if strings.HasPrefix(strings.TrimSpace(token), "wlt_") {
		bind, bindErr := r.auth.ValidateLinkToken(ctx, token)
		if bindErr != nil {
			return mcp.Session{}, err
		}
		session = r.baseSession(token, bind.User.ID.String(), "")
		session.ProjectID = strings.TrimSpace(bind.ProjectID)
		return session, nil
	}

	account, authErr := r.auth.ValidateAPIKey(ctx, token)
	if authErr != nil {
		return mcp.Session{}, err
	}
	return r.baseSession(token, account.ID.String(), ""), nil
}

func (r *mcpSessionResolver) baseSession(token, userID, harnessKind string) mcp.Session {
	return mcp.Session{
		Token:       strings.TrimSpace(token),
		Bootstrap:   false,
		UserID:      strings.TrimSpace(userID),
		HarnessKind: strings.TrimSpace(harnessKind),
		ContextRegistrar: projectMCPContextRegistrar{
			projects: r.projects,
			users:    r.users,
			auth:     r.auth,
			baseURL:  r.externalBaseURL,
		},
		MemberDirectory: r.projects,
		MemberRegistrar: r.projects,
		ClaudeMCP:       projectClaudeMCPConfigurator{projects: r.projects, provisioner: r.projectProvisioner},
		TaskMembers:     r.projects,
		DecisionService: r.decisions,
		GraphService:    r.graph,
		CredentialResolver: httpCredentialResolver{
			credentials: r.credentials,
		},
		TaskService:     r.tasks,
		TaskFiles:       r.files,
		MissionService:  r.missions,
		MissionKRs:      r.missions,
		MissionProgress: r.missions,
	}
}

type projectClaudeMCPConfigurator struct {
	projects    *projectapp.Service
	provisioner *projectHooksProvisioner
}

func (c projectClaudeMCPConfigurator) ConfigureClaudeMCP(ctx context.Context, req projecttool.ConfigureClaudeMCPRequest) (projecttool.ConfigureClaudeMCPResult, error) {
	if c.projects == nil {
		return projecttool.ConfigureClaudeMCPResult{}, fmt.Errorf("project service is not configured")
	}
	if c.provisioner == nil {
		return projecttool.ConfigureClaudeMCPResult{}, fmt.Errorf("claude mcp provisioner is not configured")
	}
	projectID := strings.TrimSpace(req.ProjectID)
	if projectID == "" {
		return projecttool.ConfigureClaudeMCPResult{}, fmt.Errorf("project id is required")
	}
	project, err := c.projects.GetProject(ctx, types.ProjectID(projectID))
	if err != nil {
		return projecttool.ConfigureClaudeMCPResult{}, err
	}
	result, err := c.provisioner.ProvisionClaudeMCP(ctx, strings.TrimSpace(req.UserID), project.Title(), c.projects.ResolveRoot(ctx, project))
	if err != nil {
		return projecttool.ConfigureClaudeMCPResult{}, err
	}
	return projecttool.ConfigureClaudeMCPResult{
		ProjectID:  projectID,
		Installed:  result.Installed,
		Path:       result.Path,
		ServerName: result.ServerName,
		URL:        result.URL,
	}, nil
}

func sessionRefs(header http.Header, body []byte) (string, string) {
	// Prefer the in-band session refs carried in the JSON-RPC body over the
	// transport header. The body is the authoritative per-call identity; the
	// transport header is only a connection-level memo and can be stale when several
	// conversations share one token over one connection.
	bodyRefs := mcp.SessionRequestContextFromJSONRPCBody(body)
	sessionID, threadID := bodyRefs.SessionID, bodyRefs.ThreadID
	if sessionID == "" && threadID == "" {
		sessionID, threadID = mcp.SessionRefsFromHTTPHeader(header)
	}
	return sessionID, threadID
}

func sessionWithMember(session mcp.Session, rosterMember member.Record) mcp.Session {
	session.UserID = strings.TrimSpace(rosterMember.UserID)
	session.MemberID = strings.TrimSpace(string(rosterMember.ID))
	session.ProjectID = strings.TrimSpace(rosterMember.ProjectID)
	session.ChannelID = types.ChannelID(strings.TrimSpace(rosterMember.ChannelID))
	session.HarnessKind = strings.TrimSpace(rosterMember.HarnessKind)
	return session
}

type projectMCPContextRegistrar struct {
	projects *projectapp.Service
	users    *userapp.Service
	auth     *authapp.Service
	baseURL  string
}

func (r projectMCPContextRegistrar) RegisterMCPContext(ctx context.Context, req projecttool.RegisterContextRequest) (projecttool.RegisterContextResult, error) {
	if r.projects == nil {
		return projecttool.RegisterContextResult{}, fmt.Errorf("project service is required")
	}
	result, err := r.projects.RegisterMCPContext(ctx, projectapp.RegisterMCPContextInput{
		Token:            req.Token,
		BoundProjectID:   req.BoundProjectID,
		UserID:           mcpUserID(ctx, r.users, r.auth, req.Token),
		ProjectID:        req.ProjectID,
		ProjectRoot:      req.ProjectRoot,
		LocationID:       req.LocationID,
		DisplayName:      req.DisplayName,
		HarnessKind:      req.HarnessKind,
		SessionID:        req.SessionID,
		ThreadID:         req.ThreadID,
		NativeSessionRef: req.NativeSessionRef,
		Model:            req.Model,
		Effort:           req.Effort,
		PermissionMode:   req.PermissionMode,
		ConfigRef:        req.ConfigRef,
	})
	if err != nil {
		return projecttool.RegisterContextResult{}, err
	}
	mcpURL := strings.TrimRight(r.baseURL, "/") + "/mcp?token=" + result.Token
	return projecttool.RegisterContextResult{
		ProjectID:         result.ProjectID,
		ProjectRoot:       result.ProjectRoot,
		LocationID:        result.LocationID,
		MemberID:          result.MemberID,
		DisplayName:       result.DisplayName,
		MemberType:        result.MemberType,
		ChannelID:         result.ChannelID,
		SessionID:         result.SessionID,
		ThreadID:          result.ThreadID,
		NativeSessionRef:  result.NativeSessionRef,
		Token:             result.Token,
		URL:               mcpURL,
		MCPServers:        result.MCPServers,
		AlreadyRegistered: result.AlreadyRegistered,
	}, nil
}

func (r *mcpSessionResolver) userID(ctx context.Context, token string) string {
	return mcpUserID(ctx, r.users, r.auth, token)
}

func mcpUserID(ctx context.Context, users *userapp.Service, auth *authapp.Service, token string) string {
	if auth != nil {
		if strings.HasPrefix(strings.TrimSpace(token), "wlt_") {
			binding, err := auth.ValidateLinkToken(ctx, token)
			if err == nil && strings.TrimSpace(binding.User.ID.String()) != "" {
				return strings.TrimSpace(binding.User.ID.String())
			}
		}
		record, err := auth.ValidateAPIKey(ctx, token)
		if err == nil && strings.TrimSpace(record.ID.String()) != "" {
			return strings.TrimSpace(record.ID.String())
		}
	}
	return "local"
}
