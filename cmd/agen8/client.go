package main

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/tinoosan/agen8/internal/clientsetup"
)

func newClientCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "client",
		Short: "Configure Agen8 on an AI harness machine",
		Args:  cobra.NoArgs,
	}
	cmd.AddCommand(newClientSetupCmd())
	return cmd
}

func newClientSetupCmd() *cobra.Command {
	var opts clientsetup.Options
	cmd := &cobra.Command{
		Use:   "setup",
		Short: "Install Agen8 skills, hooks, and MCP configuration",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			opts.Context = cmd.Context()
			result, err := clientsetup.Install(opts)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "configured agen8 for %s\nproject: %s\nskills: %s\nhooks: %s\nmcp: %s\nserver: %s\n",
				result.Harness, result.ProjectDir, result.SkillRoot, result.HooksPath, result.MCPPath, result.MCPURL)
			return nil
		},
	}
	cmd.Flags().StringVar(&opts.Harness, "harness", "claude", "target harness: claude")
	cmd.Flags().StringVar(&opts.BaseURL, "url", "http://127.0.0.1:7777", "Agen8 daemon base URL")
	cmd.Flags().StringVar(&opts.Token, "token", "", "Agen8 API key (ak_...)")
	cmd.Flags().StringVar(&opts.ProjectDir, "project-dir", "", "local project directory (default: cwd)")
	cmd.Flags().StringVar(&opts.HomeDir, "home", "", "home directory override")
	return cmd
}
