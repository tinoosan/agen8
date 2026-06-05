package rpc

import (
	"context"
	"fmt"
	"strings"

	"github.com/tinoosan/agen8-mcp-server/internal/core/types"
	fileapp "github.com/tinoosan/agen8-mcp-server/internal/services/file/app"
)

type Handler struct {
	svc *fileapp.Service
}

func NewHandler(svc *fileapp.Service) (*Handler, error) {
	if svc == nil {
		return nil, fmt.Errorf("file service is required")
	}
	return &Handler{svc: svc}, nil
}

func MustNewHandler(svc *fileapp.Service) *Handler {
	handler, err := NewHandler(svc)
	if err != nil {
		panic(err)
	}
	return handler
}

func (h *Handler) ListDir(ctx context.Context, p ListDirParams) (fileapp.ListDirResult, error) {
	if strings.TrimSpace(p.ProjectID) == "" && strings.TrimSpace(p.ProjectRoot) == "" {
		return fileapp.ListDirResult{}, invalidParams("projectId is required")
	}
	return h.svc.ListDir(ctx, fileapp.ListDirInput{
		ProjectID:   cleanProjectID(p.ProjectID),
		ProjectRoot: p.ProjectRoot,
		Path:        p.Path,
	})
}

func (h *Handler) Get(ctx context.Context, p GetParams) (fileapp.GetResult, error) {
	if strings.TrimSpace(p.ProjectID) == "" && strings.TrimSpace(p.ProjectRoot) == "" {
		return fileapp.GetResult{}, invalidParams("projectId is required")
	}
	if strings.TrimSpace(p.Path) == "" {
		return fileapp.GetResult{}, invalidParams("path is required")
	}
	return h.svc.Get(ctx, fileapp.GetInput{
		ProjectID:   cleanProjectID(p.ProjectID),
		ProjectRoot: p.ProjectRoot,
		Path:        p.Path,
		MaxBytes:    p.MaxBytes,
	})
}

func (h *Handler) CreateDir(ctx context.Context, p PathParams) (fileapp.PathResult, error) {
	if err := requirePathParams(p); err != nil {
		return fileapp.PathResult{}, err
	}
	return h.svc.CreateDir(ctx, fileapp.PathInput{ProjectID: cleanProjectID(p.ProjectID), ProjectRoot: p.ProjectRoot, Path: p.Path})
}

func (h *Handler) CreateFile(ctx context.Context, p PathParams) (fileapp.PathResult, error) {
	if err := requirePathParams(p); err != nil {
		return fileapp.PathResult{}, err
	}
	return h.svc.CreateFile(ctx, fileapp.PathInput{ProjectID: cleanProjectID(p.ProjectID), ProjectRoot: p.ProjectRoot, Path: p.Path})
}

func (h *Handler) Move(ctx context.Context, p MoveParams) (fileapp.PathResult, error) {
	if err := requireMoveParams(p); err != nil {
		return fileapp.PathResult{}, err
	}
	return h.svc.Move(ctx, fileapp.MoveInput{ProjectID: cleanProjectID(p.ProjectID), ProjectRoot: p.ProjectRoot, Path: p.Path, Destination: p.Destination})
}

func (h *Handler) Copy(ctx context.Context, p MoveParams) (fileapp.PathResult, error) {
	if err := requireMoveParams(p); err != nil {
		return fileapp.PathResult{}, err
	}
	return h.svc.Copy(ctx, fileapp.MoveInput{ProjectID: cleanProjectID(p.ProjectID), ProjectRoot: p.ProjectRoot, Path: p.Path, Destination: p.Destination})
}

func (h *Handler) Delete(ctx context.Context, p PathParams) (struct{}, error) {
	if err := requirePathParams(p); err != nil {
		return struct{}{}, err
	}
	return h.svc.Delete(ctx, fileapp.PathInput{ProjectID: cleanProjectID(p.ProjectID), ProjectRoot: p.ProjectRoot, Path: p.Path})
}

func (h *Handler) Upload(ctx context.Context, p UploadParams) (fileapp.PathResult, error) {
	if strings.TrimSpace(p.ProjectID) == "" && strings.TrimSpace(p.ProjectRoot) == "" {
		return fileapp.PathResult{}, invalidParams("projectId is required")
	}
	if strings.TrimSpace(p.Path) == "" {
		return fileapp.PathResult{}, invalidParams("path is required")
	}
	if p.Content == "" && strings.TrimSpace(p.BytesB64) == "" {
		return fileapp.PathResult{}, invalidParams("content or bytesB64 is required")
	}
	return h.svc.Upload(ctx, fileapp.UploadInput{
		ProjectID:   cleanProjectID(p.ProjectID),
		ProjectRoot: p.ProjectRoot,
		Path:        p.Path,
		Content:     p.Content,
		BytesB64:    p.BytesB64,
	})
}

func requirePathParams(p PathParams) error {
	if strings.TrimSpace(p.ProjectID) == "" && strings.TrimSpace(p.ProjectRoot) == "" {
		return invalidParams("projectId is required")
	}
	if strings.TrimSpace(p.Path) == "" {
		return invalidParams("path is required")
	}
	return nil
}

func requireMoveParams(p MoveParams) error {
	if strings.TrimSpace(p.ProjectID) == "" && strings.TrimSpace(p.ProjectRoot) == "" {
		return invalidParams("projectId is required")
	}
	if strings.TrimSpace(p.Path) == "" {
		return invalidParams("path is required")
	}
	if strings.TrimSpace(p.Destination) == "" {
		return invalidParams("destination is required")
	}
	return nil
}

func cleanProjectID(value string) types.ProjectID {
	return types.ProjectID(strings.TrimSpace(value))
}

func invalidParams(message string) error {
	return rpcError{code: -32602, message: strings.TrimSpace(message)}
}

type rpcError struct {
	code    int
	message string
}

func (e rpcError) Error() string {
	return e.message
}

func (e rpcError) RPCCode() int {
	return e.code
}
