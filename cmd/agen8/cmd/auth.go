package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tinoosan/agen8/internal/app"
	authpkg "github.com/tinoosan/agen8/pkg/auth"
)

var authProviderFlag string

var authCmd = &cobra.Command{
	Use:   "auth",
	Short: "Manage Agen8 authentication providers",
}

var authLoginCmd = &cobra.Command{
	Use:   "login",
	Short: "Log in to the selected auth provider",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := effectiveConfig(cmd)
		if err != nil {
			return err
		}
		provider, err := selectedAuthProvider()
		if err != nil {
			return err
		}
		mgr, err := authpkg.NewManager(cfg.DataDir, provider)
		if err != nil {
			return err
		}
		p := mgr.Provider()
		if p == nil {
			return fmt.Errorf("auth provider is not available")
		}
		if err := p.Login(cmd.Context(), true); err != nil {
			return err
		}
		if err := app.PersistRuntimeAuthProvider(cfg.DataDir, p.Name()); err != nil {
			return err
		}
		st, err := p.Status(cmd.Context())
		if err == nil {
			fmt.Fprintf(cmd.OutOrStdout(), "Logged in via %s\n", strings.TrimSpace(st.Provider))
		}
		return nil
	},
}

var authStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show login status for the selected auth provider",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := effectiveConfig(cmd)
		if err != nil {
			return err
		}
		provider, err := selectedAuthProvider()
		if err != nil {
			return err
		}
		mgr, err := authpkg.NewManager(cfg.DataDir, provider)
		if err != nil {
			return err
		}
		p := mgr.Provider()
		if p == nil {
			return fmt.Errorf("auth provider is not available")
		}
		st, err := p.Status(cmd.Context())
		if err != nil {
			return err
		}
		expires := "n/a"
		if !st.ExpiresAt.IsZero() {
			expires = st.ExpiresAt.UTC().Format("2006-01-02T15:04:05Z")
		}
		acct := strings.TrimSpace(st.AccountID)
		if len(acct) > 6 {
			acct = acct[:3] + "..." + acct[len(acct)-3:]
		}
		if acct == "" {
			acct = "n/a"
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Provider: %s\n", st.Provider)
		fmt.Fprintf(cmd.OutOrStdout(), "Logged in: %t\n", st.LoggedIn)
		fmt.Fprintf(cmd.OutOrStdout(), "Expires: %s\n", expires)
		fmt.Fprintf(cmd.OutOrStdout(), "Account: %s\n", acct)
		fmt.Fprintf(cmd.OutOrStdout(), "Source: %s\n", st.Source)
		return nil
	},
}

var authLogoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "Log out from the selected auth provider",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := effectiveConfig(cmd)
		if err != nil {
			return err
		}
		provider, err := selectedAuthProvider()
		if err != nil {
			return err
		}
		mgr, err := authpkg.NewManager(cfg.DataDir, provider)
		if err != nil {
			return err
		}
		p := mgr.Provider()
		if p == nil {
			return fmt.Errorf("auth provider is not available")
		}
		if err := p.Logout(cmd.Context()); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Logged out from %s\n", p.Name())
		return nil
	},
}

func init() {
	authLoginCmd.Flags().StringVar(&authProviderFlag, "provider", "", "auth provider (api_key or chatgpt_account)")
	authStatusCmd.Flags().StringVar(&authProviderFlag, "provider", "", "auth provider (api_key or chatgpt_account)")
	authLogoutCmd.Flags().StringVar(&authProviderFlag, "provider", "", "auth provider (api_key or chatgpt_account)")
	authCmd.AddCommand(authLoginCmd)
	authCmd.AddCommand(authStatusCmd)
	authCmd.AddCommand(authLogoutCmd)
}

func selectedAuthProvider() (string, error) {
	if v := strings.TrimSpace(authProviderFlag); v != "" {
		return authpkg.ParseProvider(v)
	}
	if v := strings.TrimSpace(os.Getenv(authpkg.EnvAuthProvider)); v != "" {
		return authpkg.ParseProvider(v)
	}
	return authpkg.ParseProvider(authProvider)
}
