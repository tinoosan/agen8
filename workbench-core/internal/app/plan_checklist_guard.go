package app

import (
	"errors"
	iofs "io/fs"
	"os"
	"strings"

	"github.com/tinoosan/workbench-core/internal/types"
	"github.com/tinoosan/workbench-core/internal/vfs"
)

const (
	planChecklistPath = "/plan/HEAD.md"
)

type planChecklistStatus struct {
	exists   bool
	valid    bool
	hasItems bool
	hasOpen  bool
	text     string
	rawLines []string
}

func planChecklistStatusFromText(text string) planChecklistStatus {
	status := planChecklistStatus{
		exists:   true,
		text:     text,
		rawLines: strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n"),
	}
	invalid := false
	for _, line := range status.rawLines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		ok, checked := isChecklistLine(line)
		if !ok {
			invalid = true
			continue
		}
		status.hasItems = true
		if !checked {
			status.hasOpen = true
		}
	}
	if status.hasItems {
		status.valid = !invalid
	}
	return status
}

func readPlanChecklist(fs *vfs.FS) (planChecklistStatus, error) {
	if fs == nil {
		return planChecklistStatus{}, errors.New("vfs not available")
	}
	b, err := fs.Read(planChecklistPath)
	if err != nil {
		if errors.Is(err, iofs.ErrNotExist) || errors.Is(err, os.ErrNotExist) {
			return planChecklistStatus{exists: false}, nil
		}
		return planChecklistStatus{}, err
	}
	status := planChecklistStatusFromText(string(b))
	status.exists = true
	return status, nil
}

func isChecklistLine(line string) (bool, bool) {
	trimmed := strings.TrimLeft(line, " \t")
	if len(trimmed) < 5 {
		return false, false
	}
	bullet := trimmed[0]
	if bullet != '-' && bullet != '*' && bullet != '+' {
		return false, false
	}
	if len(trimmed) < 5 || trimmed[1] != ' ' || trimmed[2] != '[' || trimmed[4] != ']' {
		return false, false
	}
	switch trimmed[3] {
	case 'x', 'X':
		return true, true
	case ' ':
		return true, false
	default:
		return false, false
	}
}

func shouldBypassPlanChecklist(req types.HostOpRequest) bool {
	path := strings.TrimSpace(req.Path)
	if path == planChecklistPath {
		return true
	}
	return false
}

func isSideEffectOp(op string) bool {
	switch strings.ToLower(strings.TrimSpace(op)) {
	case types.HostOpFSWrite, types.HostOpFSAppend, types.HostOpFSEdit, types.HostOpFSPatch,
		types.HostOpShellExec, types.HostOpHTTPFetch, types.HostOpToolRun:
		return true
	default:
		return false
	}
}

func planChecklistWarning(status planChecklistStatus) string {
	if !status.exists {
		return "- [ ] Add a checklist to /plan/HEAD.md before running side-effect ops."
	}
	if !status.valid || !status.hasItems {
		return "- [ ] Fix /plan/HEAD.md to contain only checklist items (- [ ]/- [x])."
	}
	if !status.hasOpen {
		return "- [ ] Add new checklist items for the current work before side-effect ops."
	}
	return ""
}

func appendPlanChecklistWarning(fs *vfs.FS, status planChecklistStatus, warningLine string) {
	if fs == nil || strings.TrimSpace(warningLine) == "" {
		return
	}
	if status.exists && strings.Contains(status.text, warningLine) {
		return
	}
	var next string
	if status.exists {
		next = strings.TrimRight(status.text, "\n") + "\n" + warningLine + "\n"
	} else {
		next = warningLine + "\n"
	}
	_ = fs.Write(planChecklistPath, []byte(next))
}

func validatePlanChecklistWrite(req types.HostOpRequest) (string, bool) {
	if strings.TrimSpace(req.Path) != planChecklistPath {
		return "", false
	}
	if req.Op != types.HostOpFSWrite && req.Op != types.HostOpFSAppend {
		return "", false
	}
	status := planChecklistStatusFromText(req.Text)
	if !status.hasItems || !status.valid {
		return "Checklist writes to /plan/HEAD.md must contain only - [ ] / - [x] items.", true
	}
	return "", false
}

func enforcePlanChecklist(fs *vfs.FS, req types.HostOpRequest) *types.HostOpResponse {
	if !isSideEffectOp(req.Op) {
		return nil
	}
	if shouldBypassPlanChecklist(req) {
		if msg, invalid := validatePlanChecklistWrite(req); invalid {
			return &types.HostOpResponse{
				Op:        req.Op,
				Ok:        false,
				Error:     msg,
				ErrorCode: "plan_checklist_invalid",
			}
		}
		return nil
	}
	status, err := readPlanChecklist(fs)
	if err != nil {
		return nil
	}
	if !status.exists || !status.valid || !status.hasItems || !status.hasOpen {
		warning := planChecklistWarning(status)
		appendPlanChecklistWarning(fs, status, warning)
		return &types.HostOpResponse{
			Op:        req.Op,
			Ok:        false,
			Error:     "Checklist required: update /plan/HEAD.md before running side-effect ops.",
			ErrorCode: "plan_checklist_required",
			Warning:   warning,
		}
	}
	return nil
}
