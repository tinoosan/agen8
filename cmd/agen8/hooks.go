package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/tinoosan/agen8/internal/hookinstaller"
)

func newHooksCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "hooks",
		Short: "Manage agen8 attention hooks in a harness",
		Args:  cobra.NoArgs,
	}
	cmd.AddCommand(newHooksInstallCmd())
	return cmd
}

func newHooksInstallCmd() *cobra.Command {
	var (
		harness    string
		baseURL    string
		token      string
		projectDir string
		homeDir    string
	)
	cmd := &cobra.Command{
		Use:   "install",
		Short: "Install attention hooks into a harness (claude|codex)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if strings.TrimSpace(harness) == "" {
				return fmt.Errorf("--harness is required (claude or codex)")
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
			fmt.Fprintf(cmd.OutOrStdout(), "installed agen8 hooks for %s\nwrote: %s\nrerun this command to refresh (e.g. after rotating the token)\n",
				result.Harness, result.Path)
			return nil
		},
	}
	cmd.Flags().StringVar(&harness, "harness", "", "target harness: claude or codex")
	cmd.Flags().StringVar(&baseURL, "url", "http://127.0.0.1:7777", "agen8 daemon base URL")
	cmd.Flags().StringVar(&token, "token", "", "agen8 API key the hooks authenticate with (ak_...)")
	cmd.Flags().StringVar(&projectDir, "project-dir", "", "project directory for Claude Code settings (default: cwd)")
	cmd.Flags().StringVar(&homeDir, "home", "", "home directory override for Codex config")
	return cmd
}
