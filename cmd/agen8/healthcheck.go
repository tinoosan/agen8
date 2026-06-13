package main

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

func newHealthcheckCmd() *cobra.Command {
	var (
		url     string
		timeout time.Duration
	)
	cmd := &cobra.Command{
		Use:   "healthcheck",
		Short: "Probe the daemon /healthz endpoint",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			if strings.TrimSpace(url) == "" {
				return fmt.Errorf("healthcheck url is required")
			}
			client := &http.Client{Timeout: timeout}
			resp, err := client.Get(url)
			if err != nil {
				return fmt.Errorf("healthcheck request failed: %w", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
				return fmt.Errorf("healthcheck failed: %s", resp.Status)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&url, "url", "http://127.0.0.1:7777/healthz", "health endpoint URL")
	cmd.Flags().DurationVar(&timeout, "timeout", 2*time.Second, "request timeout")
	return cmd
}
