package hosttools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/tinoosan/agen8/pkg/llm/types"
	pkgsoul "github.com/tinoosan/agen8/pkg/services/soul"
	pkgt "github.com/tinoosan/agen8/pkg/types"
)

type SoulUpdater interface {
	Update(ctx context.Context, req pkgsoul.UpdateRequest) (pkgsoul.Doc, error)
}

type SoulUpdateTool struct {
	Updater SoulUpdater
	Actor   pkgsoul.ActorLayer
}

func (t *SoulUpdateTool) Definition() types.Tool {
	return types.Tool{
		Type: "function",
		Function: types.ToolFunction{
			Name:        "soul_update",
			Description: "[SOUL] Propose/update canonical SOUL content via daemon policy gate. Agent may only mutate adaptive sections.",
			Strict:      false,
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"content":         map[string]any{"type": "string", "description": "Full SOUL.md content including required sections."},
					"reason":          map[string]any{"type": "string", "description": "Required rationale for the update."},
					"expectedVersion": map[string]any{"type": "integer", "description": "Optional optimistic concurrency version."},
				},
				"required":             []any{"content", "reason"},
				"additionalProperties": false,
			},
		},
	}
}

func (t *SoulUpdateTool) Execute(ctx context.Context, args json.RawMessage) (pkgt.HostOpRequest, error) {
	if t == nil || t.Updater == nil {
		return pkgt.HostOpRequest{}, fmt.Errorf("soul_update: updater is not configured")
	}
	var payload struct {
		Content         string `json:"content"`
		Reason          string `json:"reason"`
		ExpectedVersion int    `json:"expectedVersion"`
	}
	if err := json.Unmarshal(args, &payload); err != nil {
		return pkgt.HostOpRequest{}, err
	}
	payload.Content = strings.TrimSpace(payload.Content)
	payload.Reason = strings.TrimSpace(payload.Reason)
	if payload.Content == "" {
		return pkgt.HostOpRequest{}, fmt.Errorf("soul_update.content is required")
	}
	if payload.Reason == "" {
		return pkgt.HostOpRequest{}, fmt.Errorf("soul_update.reason is required")
	}
	actor := t.Actor
	if actor == "" {
		actor = pkgsoul.ActorAgent
	}
	doc, err := t.Updater.Update(ctx, pkgsoul.UpdateRequest{
		Content:         payload.Content,
		Reason:          payload.Reason,
		Actor:           actor,
		ExpectedVersion: payload.ExpectedVersion,
	})
	if err != nil {
		return pkgt.HostOpRequest{}, fmt.Errorf("soul_update: %w", err)
	}
	msg := fmt.Sprintf("SOUL updated to version %d", doc.Version)
	return pkgt.HostOpRequest{Op: pkgt.HostOpToolResult, Tag: "soul_update", Text: msg}, nil
}
