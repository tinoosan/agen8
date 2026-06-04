package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/tinoosan/agen8-mcp-server/internal/app"
	"github.com/tinoosan/agen8-mcp-server/internal/bridge"
	"github.com/tinoosan/agen8-mcp-server/internal/config"
	"github.com/tinoosan/agen8-mcp-server/internal/daemon"
	"github.com/tinoosan/agen8-mcp-server/internal/logging"
	agen8mcp "github.com/tinoosan/agen8-mcp-server/internal/mcp"
	"github.com/tinoosan/agen8-mcp-server/internal/services/harness/infra/claudecli"
	spacedomain "github.com/tinoosan/agen8-mcp-server/internal/services/space/domain"
	"github.com/tinoosan/agen8-mcp-server/pkg/protocol"
	"github.com/tinoosan/agen8-mcp-server/pkg/types"
)

func main() {
	if err := newRootCommand().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

var runDaemon = func(ctx context.Context, cfg daemon.Config) error {
	d, err := daemon.New(cfg)
	if err != nil {
		return err
	}
	return d.Run(ctx)
}

var runBridge = func(ctx context.Context, httpAddr string) error {
	return bridge.NewServer().Serve(ctx, httpAddr)
}

var runMCPStdio = func(ctx context.Context) error {
	session, err := stdioSessionFromEnv()
	if err != nil {
		return err
	}
	return agen8mcp.RunStdio(ctx, session)
}

// claudeHookTimeout bounds the binding call so a slow or unavailable daemon
// can never hang the Claude Code session that invoked the hook.
const claudeHookTimeout = 15 * time.Second

var runClaudeHook = func(ctx context.Context) error {
	drainCtx, cancel := context.WithTimeout(ctx, claudeHookTimeout)
	defer cancel()
	claudecli.RunHook(drainCtx, claudecli.MCPHookBinder{}, os.Stdin, os.Stdout, os.Stderr)
	return nil
}

var runClaudeSetup = func(_ context.Context, opts claudecli.SetupOptions) (claudecli.SetupResult, error) {
	if strings.TrimSpace(opts.HookCommand) == "" {
		executable, err := os.Executable()
		if err != nil {
			return claudecli.SetupResult{}, err
		}
		opts.HookCommand = executable
	}
	return claudecli.SetupProject(opts)
}

func stdioSessionFromEnv() (agen8mcp.Session, error) {
	resolvedDataDir, err := config.ResolveDataDir("", false)
	if err != nil {
		return agen8mcp.Session{}, err
	}
	hostCfg := config.Default()
	hostCfg.DataDir = resolvedDataDir
	hostCfg.DBDriver = os.Getenv(config.EnvDBDriver)
	hostCfg.DatabaseURL = os.Getenv(config.EnvDatabaseURL)
	application, err := app.NewApplication(app.Config{
		Host:    hostCfg,
		Logging: logging.Config{Level: os.Getenv(logging.EnvLogLevel)},
	})
	if err != nil {
		return agen8mcp.Session{}, fmt.Errorf("build mcp stdio application: %w", err)
	}
	memberID := strings.TrimSpace(os.Getenv("AGEN8_MEMBER_ID"))
	spaceID := strings.TrimSpace(os.Getenv("AGEN8_SPACE_ID"))
	projectID := strings.TrimSpace(os.Getenv("AGEN8_PROJECT_ID"))
	channelID := strings.TrimSpace(os.Getenv("AGEN8_CHANNEL_ID"))
	if channelID == "" && memberID != "" {
		channelID = memberID
	}
	return agen8mcp.Session{
		ChannelID:         types.ChannelID(channelID),
		SpaceID:           spacedomain.SpaceID(spaceID),
		MemberID:          memberID,
		HarnessKind:       strings.TrimSpace(os.Getenv("AGEN8_HARNESS_KIND")),
		SpaceReader:       application.SpaceSvc,
		MemberDirectory:   application.SpaceSvc,
		MemberRegistrar:   application.SpaceSvc,
		TaskMembers:       application.SpaceSvc,
		MessagePublisher:  application.MessageSvc,
		DecisionService:   application.DecisionSvc,
		GraphService:      application.GraphSvc,
		HumanInputAwaiter: application.HumanInputMCPAwaiter,
		TaskService:       application.TaskSvc,
		ScheduleService:   application.ScheduleSvc,
		OperatorService:   application.OperatorSvc,
		MissionService:    application.MissionSvc,
		MissionKRs:        application.MissionSvc,
		MissionProgress:   application.MissionSvc,
		ProjectID:         projectID,
	}, nil
}

func newRootCommand() *cobra.Command {
	var dataDir string
	var listener string
	var endpoint string
	var httpAddr string

	root := &cobra.Command{
		Use:           "agen8-mcp-server",
		Short:         "Agen8 MCP-first work-context server",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	daemonCmd := &cobra.Command{
		Use:   "daemon",
		Short: "Run daemon commands",
	}
	startCmd := &cobra.Command{
		Use:   "start",
		Short: "Start the Agen8 MCP server daemon",
		RunE: func(cmd *cobra.Command, _ []string) error {
			dataDirSet := cmd.Flags().Changed("data-dir")
			resolvedDataDir, err := config.ResolveDataDir(dataDir, dataDirSet)
			if err != nil {
				return err
			}
			appCfg := config.Default()
			appCfg.DataDir = resolvedDataDir
			appCfg.DBDriver = os.Getenv(config.EnvDBDriver)
			appCfg.DatabaseURL = os.Getenv(config.EnvDatabaseURL)
			if err := appCfg.Validate(); err != nil {
				return err
			}
			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			return runDaemon(ctx, daemon.Config{
				AppConfig: appCfg,
				Listener:  listener,
				Endpoint:  endpoint,
				HTTPAddr:  httpAddr,
				Logging:   logging.Config{Level: os.Getenv(logging.EnvLogLevel)},
				Out:       cmd.OutOrStdout(),
			})
		},
	}
	startCmd.Flags().StringVar(&dataDir, "data-dir", "", "Agen8 data directory")
	startCmd.Flags().StringVar(&listener, "listener", envDefault(daemon.EnvListener, daemon.ListenerLocal), "daemon listener strategy: local or http")
	startCmd.Flags().StringVar(&endpoint, "endpoint", envDefault(daemon.EnvEndpoint, protocol.DefaultRPCEndpoint()), "local RPC endpoint")
	startCmd.Flags().StringVar(&httpAddr, "http-addr", envDefault(daemon.EnvHTTPAddr, daemon.DefaultHTTPAddr), "HTTP daemon address")

	daemonCmd.AddCommand(startCmd)
	root.AddCommand(daemonCmd)

	var bridgeHTTPAddr string
	bridgeCmd := &cobra.Command{
		Use:   "bridge",
		Short: "Run bridge commands",
	}
	bridgeServeCmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the bridge",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			return runBridge(ctx, bridgeHTTPAddr)
		},
	}
	bridgeServeCmd.Flags().StringVar(&bridgeHTTPAddr, "http-addr", "127.0.0.1:0", "bridge HTTP address")
	bridgeCmd.AddCommand(bridgeServeCmd)
	root.AddCommand(bridgeCmd)

	mcpCmd := &cobra.Command{
		Use:   "mcp",
		Short: "Run MCP transports and helpers",
	}
	mcpStdioCmd := &cobra.Command{
		Use:   "stdio",
		Short: "Serve Agen8 MCP over stdio for local harnesses",
		RunE: func(_ *cobra.Command, _ []string) error {
			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			return runMCPStdio(ctx)
		},
	}
	mcpCmd.AddCommand(mcpStdioCmd)
	root.AddCommand(mcpCmd)

	claudeCmd := &cobra.Command{
		Use:   "claude",
		Short: "Claude Code integration helpers",
	}
	claudeHookCmd := &cobra.Command{
		Use:   "hook",
		Short: "Bind the calling Claude Code session to Agen8 (reads hook payload on stdin)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			return runClaudeHook(ctx)
		},
	}
	var claudeSetupProjectRoot string
	var claudeSetupMCPURL string
	var claudeSetupHookCommand string
	var claudeSetupHookArgs []string
	claudeSetupCmd := &cobra.Command{
		Use:   "setup",
		Short: "Install Agen8 Claude Code MCP and session-binding hook config for a project",
		RunE: func(cmd *cobra.Command, _ []string) error {
			result, err := runClaudeSetup(cmd.Context(), claudecli.SetupOptions{
				ProjectRoot: claudeSetupProjectRoot,
				MCPURL:      claudeSetupMCPURL,
				HookCommand: claudeSetupHookCommand,
				HookArgs:    claudeSetupHookArgs,
			})
			if err != nil {
				return err
			}
			encoder := json.NewEncoder(cmd.OutOrStdout())
			encoder.SetIndent("", "  ")
			return encoder.Encode(result)
		},
	}
	claudeSetupCmd.Flags().StringVar(&claudeSetupProjectRoot, "project-root", "", "Project root to configure; defaults to the current working directory")
	claudeSetupCmd.Flags().StringVar(&claudeSetupMCPURL, "mcp-url", "", "Agen8 MCP URL; defaults to AGEN8_MCP_URL, AGEN8_MCP_TOKEN, existing .mcp.json, or local dev URL")
	claudeSetupCmd.Flags().StringVar(&claudeSetupHookCommand, "hook-command", "", "Hook executable; defaults to the current agen8-mcp-server binary")
	claudeSetupCmd.Flags().StringSliceVar(&claudeSetupHookArgs, "hook-arg", nil, "Hook argument; repeat to override the default 'claude hook' args")
	claudeCmd.AddCommand(claudeHookCmd)
	claudeCmd.AddCommand(claudeSetupCmd)
	root.AddCommand(claudeCmd)
	return root
}

func envDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
