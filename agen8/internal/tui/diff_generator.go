package tui

import (
	"errors"
	"fmt"
	iofs "io/fs"
	"strings"

	"github.com/tinoosan/agen8/pkg/agent"
	"github.com/tinoosan/agen8/pkg/types"
	"github.com/tinoosan/agen8/pkg/vfs"
)

// GeneratePendingOpDiff builds a diff preview for a file operation before execution.
func GeneratePendingOpDiff(vfsFS *vfs.FS, op types.HostOpRequest) (string, error) {
	if vfsFS == nil {
		return "", fmt.Errorf("vfs is nil")
	}
	path := strings.TrimSpace(op.Path)
	if path == "" {
		return "", fmt.Errorf("op path is empty")
	}

	before, hadBefore, err := readBeforeState(vfsFS, path)
	if err != nil {
		return "", fmt.Errorf("read before state: %w", err)
	}

	after, err := computeAfterState(before, op)
	if err != nil {
		return "", err
	}

	diff, _, _, _ := buildFileChangePreview(op.Op, path, before, after, hadBefore, false, "", false, false)
	return diff, nil
}

func readBeforeState(vfsFS *vfs.FS, path string) (string, bool, error) {
	data, err := vfsFS.Read(path)
	if err != nil {
		if errors.Is(err, iofs.ErrNotExist) {
			return "", false, nil
		}
		return "", false, err
	}
	return string(data), true, nil
}

func computeAfterState(before string, op types.HostOpRequest) (string, error) {
	switch strings.TrimSpace(op.Op) {
	case types.HostOpFSWrite:
		return op.Text, nil
	case types.HostOpFSAppend:
		return before + op.Text, nil
	case types.HostOpFSEdit:
		if len(op.Input) == 0 {
			return "", fmt.Errorf("fs_edit input missing")
		}
		return agent.ApplyStructuredEdits(before, op.Input)
	case types.HostOpFSPatch:
		return agent.ApplyUnifiedDiffStrict(before, op.Text)
	default:
		return "", fmt.Errorf("unsupported op %q", op.Op)
	}
}
