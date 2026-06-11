package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/tinoosan/agen8/internal/claudehook"
	"github.com/tinoosan/agen8/internal/config"
	"github.com/tinoosan/agen8/internal/daemon"
	"github.com/tinoosan/agen8/internal/hookinstaller"
	"github.com/tinoosan/agen8/internal/skillinstaller"
	"github.com/tinoosan/agen8/pkg/buildinfo"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "agen8: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return runDaemonStart(nil)
	}
	switch args[0] {
	case "version", "--version", "-v":
		info := buildinfo.Current()
		fmt.Printf("agen8 %s\ncommit: %s\n", info.Version, info.Commit)
		if info.BuildDate != "" {
			fmt.Printf("built: %s\n", info.BuildDate)
		}
		return nil
	case "claude":
		return runClaude(args[1:])
	case "skill":
		return runSkill(args[1:])
	case "hooks":
		return runHooks(args[1:])
	}
	if args[0] != "daemon" {
		return fmt.Errorf("unknown command %q", args[0])
	}
	if len(args) < 2 || args[1] != "start" {
		return fmt.Errorf("usage: agen8 daemon start [--data-dir DIR] [--listener http] [--http-addr ADDR]")
	}
	return runDaemonStart(args[2:])
}

// runClaude dispatches the Claude Code integration subcommands. Today the only
// one is `claude hook`, the PreToolUse entrypoint that stamps a conversation's
// session_id into agen8 tool calls so each Claude conversation resolves to its
// own member. It reads a hook payload on stdin and writes the hook response on
// stdout (see internal/claudehook).
func runClaude(args []string) error {
	if len(args) == 0 || args[0] != "hook" {
		return fmt.Errorf("usage: agen8 claude hook")
	}
	return claudehook.Run(os.Stdin, os.Stdout, os.Stderr)
}

func runSkill(args []string) error {
	if len(args) == 0 || args[0] != "install" {
		return fmt.Errorf("usage: agen8 skill install --harness codex|claude-cli [--home DIR]")
	}
	fs := flag.NewFlagSet("skill install", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	var (
		harness string
		homeDir string
	)
	fs.StringVar(&harness, "harness", "", "target harness: codex or claude-cli")
	fs.StringVar(&homeDir, "home", "", "home directory override")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args(), " "))
	}
	if strings.TrimSpace(harness) == "" {
		return fmt.Errorf("usage: agen8 skill install --harness codex|claude-cli [--home DIR]")
	}
	result, err := skillinstaller.Install(skillinstaller.Options{
		Harness: skillinstaller.Harness(harness),
		HomeDir: homeDir,
	})
	if err != nil {
		return err
	}
	fmt.Printf("installed agen8 skills for %s\nroot: %s\nskills: %s\nrerun this command to refresh\n", result.Harness, result.Root, strings.Join(result.Skills, ", "))
	return nil
}

// runHooks dispatches the attention-hook provisioning subcommands. `hooks
// install` writes the curl-based harness hooks that report "this agent is
// waiting on you" to the daemon (see internal/hookinstaller). Explicit by
// design: agen8 never installs hooks silently.
func runHooks(args []string) error {
	const usage = "usage: agen8 hooks install --harness claude|codex --token TOKEN [--url URL] [--project-dir DIR] [--home DIR]"
	if len(args) == 0 || args[0] != "install" {
		return errors.New(usage)
	}
	fs := flag.NewFlagSet("hooks install", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	var (
		harness    string
		baseURL    string
		token      string
		projectDir string
		homeDir    string
	)
	fs.StringVar(&harness, "harness", "", "target harness: claude or codex")
	fs.StringVar(&baseURL, "url", "http://127.0.0.1:7777", "agen8 daemon base URL")
	fs.StringVar(&token, "token", "", "agen8 API key the hooks authenticate with (ak_...)")
	fs.StringVar(&projectDir, "project-dir", "", "project directory for Claude Code settings (default: cwd)")
	fs.StringVar(&homeDir, "home", "", "home directory override for Codex config")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args(), " "))
	}
	if strings.TrimSpace(harness) == "" {
		return errors.New(usage)
	}
	result, err := hookinstaller.Install(hookinstaller.Options{
		Harness:    hookinstaller.Harness(harness),
		BaseURL:    baseURL,
		Token:      token,
		ProjectDir: projectDir,
		HomeDir:    homeDir,
	})
	if err != nil {
		return err
	}
	fmt.Printf("installed agen8 hooks for %s\nwrote: %s\nrerun this command to refresh (e.g. after rotating the token)\n", result.Harness, result.Path)
	return nil
}

func runDaemonStart(args []string) error {
	fs := flag.NewFlagSet("daemon start", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	var (
		dataDir  string
		listener string
		httpAddr string
	)
	fs.StringVar(&dataDir, "data-dir", "", "agen8 data directory")
	fs.StringVar(&listener, "listener", daemon.ListenerHTTP, "daemon listener")
	fs.StringVar(&httpAddr, "http-addr", daemon.DefaultHTTPAddr, "HTTP listen address")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args(), " "))
	}

	hostConfig := config.Default()
	resolvedDataDir, err := config.ResolveDataDir(dataDir, strings.TrimSpace(dataDir) != "")
	if err != nil {
		return err
	}
	hostConfig.DataDir = resolvedDataDir
	d, err := daemon.New(daemon.Config{
		AppConfig: hostConfig,
		Listener:  listener,
		HTTPAddr:  httpAddr,
		Out:       os.Stdout,
	})
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	err = daemon.HTTPStrategy{}.Run(ctx, d)
	if errors.Is(err, context.Canceled) {
		return nil
	}
	return err
}
