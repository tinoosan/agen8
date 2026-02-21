package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/tinoosan/agen8/pkg/agent"
	"github.com/tinoosan/agen8/pkg/types"
)

// WebhookNotifier posts task results to an external HTTP endpoint.
type WebhookNotifier struct {
	URL    string
	Client *http.Client
}

func (n WebhookNotifier) Notify(ctx context.Context, task types.Task, result types.TaskResult) error {
	url := strings.TrimSpace(n.URL)
	if url == "" {
		return nil
	}
	client := n.Client
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}
	payload := struct {
		Task   types.Task       `json:"task"`
		Result types.TaskResult `json:"result"`
	}{
		Task:   task,
		Result: result,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("webhook status %d", resp.StatusCode)
	}
	return nil
}

var _ agent.Notifier = WebhookNotifier{}
