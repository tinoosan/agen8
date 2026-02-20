package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tinoosan/agen8/pkg/profile"
)

var profilesCmd = &cobra.Command{
	Use:   "profiles",
	Short: "Profile utilities",
}

var profilesCheckCmd = &cobra.Command{
	Use:   "check <path>",
	Short: "Validate a profile directory",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		pth := strings.TrimSpace(args[0])
		if pth == "" {
			return fmt.Errorf("path is required")
		}
		p, err := profile.Load(pth)
		if err != nil {
			return err
		}

		// Best-effort skills existence check when profile lives in .../profiles/<id>/.
		profileDir := pth
		if st, err := os.Stat(pth); err == nil && !st.IsDir() {
			profileDir = filepath.Dir(pth)
		}
		skillsDir := filepath.Clean(filepath.Join(profileDir, "..", "..", "skills"))
		if st, err := os.Stat(skillsDir); err == nil && st.IsDir() {
			for _, sk := range p.Skills {
				sk = strings.TrimSpace(sk)
				if sk == "" {
					continue
				}
				if _, err := os.Stat(filepath.Join(skillsDir, sk+".md")); err != nil {
					return fmt.Errorf("profile %s: missing skill %s in %s", p.ID, sk, skillsDir)
				}
			}
		}

		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "ok: %s\n", p.ID)
		return nil
	},
}

func init() {
	profilesCmd.AddCommand(profilesCheckCmd)
}

