package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/tinoosan/agen8/internal/skillinstaller"
)

func newSkillCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "skill",
		Short: "Manage the agen8 skill tree in a harness",
		Args:  cobra.NoArgs,
	}
	cmd.AddCommand(newSkillInstallCmd())
	return cmd
}

func newSkillInstallCmd() *cobra.Command {
	var (
		harness string
		homeDir string
	)
	cmd := &cobra.Command{
		Use:   "install",
		Short: "Install the agen8 skill tree into a harness (codex|claude-cli)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if strings.TrimSpace(harness) == "" {
				return fmt.Errorf("--harness is required (codex or claude-cli)")
			}
			result, err := skillinstaller.Install(skillinstaller.Options{
				Harness: skillinstaller.Harness(harness),
				HomeDir: homeDir,
			})
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "installed agen8 skills for %s\nroot: %s\nskills: %s\nrerun this command to refresh\n",
				result.Harness, result.Root, strings.Join(result.Skills, ", "))
			return nil
		},
	}
	cmd.Flags().StringVar(&harness, "harness", "", "target harness: codex or claude-cli")
	cmd.Flags().StringVar(&homeDir, "home", "", "home directory override")
	return cmd
}
