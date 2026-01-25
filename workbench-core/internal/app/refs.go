package app

import (
	"fmt"
	"io/fs"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/tinoosan/workbench-core/internal/atref"
	"github.com/tinoosan/workbench-core/internal/resources"
	"github.com/tinoosan/workbench-core/internal/vfs"
	"github.com/tinoosan/workbench-core/internal/vfsutil"
	"github.com/tinoosan/workbench-core/pkg/agent"
)

type RefResolution struct {
	Attachments []agent.FileAttachment

	// AttachedSummaries are short "Attached: ..." strings suitable for UI events.
	AttachedSummaries []string

	// Unresolved contains tokens that could not be resolved.
	Unresolved []string

	// Ambiguous maps token -> candidate VFS paths (e.g. "/project/a.txt", "/scratch/b.txt").
	Ambiguous map[string][]string
}

// ResolveAtRefs resolves @tokens to bounded file attachments.
//
// Resolution order:
//  1. exact path under /project
//  2. artifact index match (e.g. recently written /scratch file)
//  3. fuzzy file search under workdir (auto-select only if unambiguous)
func ResolveAtRefs(fsys *vfs.FS, workdirBase string, artifacts *ArtifactIndex, userText string, maxFiles, maxBytesTotal, maxBytesPerFile int) (RefResolution, error) {
	if fsys == nil {
		return RefResolution{}, fmt.Errorf("fs is required")
	}
	tokens := atref.ExtractAtRefs(userText)
	if len(tokens) == 0 {
		return RefResolution{}, nil
	}
	if maxFiles <= 0 {
		maxFiles = 6
	}
	if maxBytesTotal <= 0 {
		maxBytesTotal = 48 * 1024
	}
	if maxBytesPerFile <= 0 {
		maxBytesPerFile = 12 * 1024
	}

	var res RefResolution
	res.Ambiguous = make(map[string][]string)
	workspaceBase := workspaceBaseDirFromVFS(fsys)

	usedBytes := 0
	for _, tok := range tokens {
		if len(res.Attachments) >= maxFiles || usedBytes >= maxBytesTotal {
			break
		}

		att, ok, amb, err := resolveOneRef(fsys, workdirBase, workspaceBase, artifacts, tok, maxBytesPerFile)
		if err != nil {
			return RefResolution{}, err
		}
		if ok {
			if usedBytes+att.BytesIncluded > maxBytesTotal {
				// Skip if it would exceed total budget.
				continue
			}
			usedBytes += att.BytesIncluded
			res.Attachments = append(res.Attachments, att)
			if att.DisplayName != "" {
				res.AttachedSummaries = append(res.AttachedSummaries, fmt.Sprintf("%s (%dB)", att.DisplayName, att.BytesIncluded))
			} else {
				res.AttachedSummaries = append(res.AttachedSummaries, fmt.Sprintf("%s (%dB)", att.VPath, att.BytesIncluded))
			}
			continue
		}
		if len(amb) != 0 {
			res.Ambiguous[tok] = amb
			continue
		}
		res.Unresolved = append(res.Unresolved, tok)
	}
	return res, nil
}

func resolveOneRef(fsys *vfs.FS, workdirBase, workspaceBase string, artifacts *ArtifactIndex, token string, maxBytes int) (agent.FileAttachment, bool, []string, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return agent.FileAttachment{}, false, nil, nil
	}

	// Allow explicit VFS paths for disambiguation (restricted to safe mounts).
	if vp, ok := normalizeExplicitRefVPath(token); ok {
		return readAttachment(fsys, token, vp, displayNameForVPath(vp), maxBytes)
	}
	// Do not allow arbitrary absolute paths or mounts.
	if strings.HasPrefix(token, "/") {
		return agent.FileAttachment{}, false, nil, nil
	}

	if strings.EqualFold(token, "last") {
		if vp, ok := artifacts.Resolve("last"); ok {
			return readAttachment(fsys, token, vp, displayNameForVPath(vp), maxBytes)
		}
		return agent.FileAttachment{}, false, nil, nil
	}

	if clean, _, err := vfsutil.NormalizeResourceSubpath(token); err == nil && clean != "" && clean != "." {
		// 1) exact under /project
		vp := "/project/" + clean
		if att, ok, err := tryReadVPath(fsys, token, vp, clean, maxBytes); err != nil {
			return agent.FileAttachment{}, false, nil, err
		} else if ok {
			return att, true, nil, nil
		}

		// 1b) exact under /scratch (run-scoped scratch).
		vp = "/scratch/" + clean
		if att, ok, err := tryReadVPath(fsys, token, vp, clean, maxBytes); err != nil {
			return agent.FileAttachment{}, false, nil, err
		} else if ok {
			return att, true, nil, nil
		}
	}

	// 2) artifact index (scratch outputs).
	if vp, ok := artifacts.Resolve(token); ok {
		return readAttachment(fsys, token, vp, displayNameForVPath(vp), maxBytes)
	}

	// 3) fuzzy search under workdir + workspace.
	candsWorkdir, err := fuzzyFindWorkdir(workdirBase, token, 40, 5000)
	if err != nil {
		return agent.FileAttachment{}, false, nil, nil
	}
	candsWorkspace, err := fuzzyFindWorkdir(workspaceBase, token, 40, 5000)
	if err != nil {
		return agent.FileAttachment{}, false, nil, nil
	}
	all := mergeCandidateVPaths(candsWorkdir, candsWorkspace)
	if len(all) == 1 {
		vp := all[0]
		return readAttachment(fsys, token, vp, displayNameForVPath(vp), maxBytes)
	}
	if len(all) > 1 {
		return agent.FileAttachment{}, false, all, nil
	}

	return agent.FileAttachment{}, false, nil, nil
}

func tryReadVPath(fsys *vfs.FS, token, vpath, display string, maxBytes int) (agent.FileAttachment, bool, error) {
	b, err := fsys.Read(vpath)
	if err != nil {
		return agent.FileAttachment{}, false, nil
	}
	return buildAttachment(token, vpath, display, b, maxBytes), true, nil
}

func readAttachment(fsys *vfs.FS, token, vpath, display string, maxBytes int) (agent.FileAttachment, bool, []string, error) {
	b, err := fsys.Read(vpath)
	if err != nil {
		return agent.FileAttachment{}, false, nil, nil
	}
	att := buildAttachment(token, vpath, display, b, maxBytes)
	return att, true, nil, nil
}

func buildAttachment(token, vpath, display string, full []byte, maxBytes int) agent.FileAttachment {
	if maxBytes <= 0 {
		maxBytes = 12 * 1024
	}
	incl := full
	tr := false
	if len(incl) > maxBytes {
		incl = incl[:maxBytes]
		tr = true
	}
	return agent.FileAttachment{
		Token:         token,
		VPath:         vpath,
		DisplayName:   display,
		Content:       string(incl),
		BytesTotal:    len(full),
		BytesIncluded: len(incl),
		Truncated:     tr,
	}
}

func displayNameForVPath(vpath string) string {
	vpath = strings.TrimSpace(vpath)
	switch {
	case strings.HasPrefix(vpath, "/project/"):
		return strings.TrimPrefix(vpath, "/project/")
	case strings.HasPrefix(vpath, "/scratch/"):
		return strings.TrimPrefix(vpath, "/scratch/")
	case strings.HasPrefix(vpath, "/results/"):
		return strings.TrimPrefix(vpath, "/results/")
	default:
		return vpath
	}
}

func fuzzyFindWorkdir(baseDir string, token string, maxCandidates int, maxVisited int) ([]string, error) {
	baseDir = strings.TrimSpace(baseDir)
	token = strings.TrimSpace(token)
	if baseDir == "" || token == "" {
		return nil, nil
	}
	if strings.HasPrefix(token, "/") {
		return nil, nil
	}
	if _, _, err := vfsutil.NormalizeResourceSubpath(token); err != nil {
		// Treat invalid paths as non-resolvable; do not walk.
		return nil, nil
	}

	wantBase := strings.Contains(token, "/") == false
	want := token
	if wantBase {
		want = path.Base(token)
	}

	cands := make([]string, 0, 8)
	visited := 0

	err := filepath.WalkDir(baseDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			// Skip hidden directories to keep search bounded and less noisy.
			if p != baseDir && strings.HasPrefix(d.Name(), ".") {
				return fs.SkipDir
			}
			return nil
		}
		visited++
		if maxVisited > 0 && visited > maxVisited {
			return fs.SkipAll
		}

		rel, err := filepath.Rel(baseDir, p)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)

		if wantBase {
			if path.Base(rel) == want {
				cands = append(cands, rel)
			}
		} else {
			if strings.HasSuffix(rel, want) {
				cands = append(cands, rel)
			}
		}
		if maxCandidates > 0 && len(cands) >= maxCandidates {
			return fs.SkipAll
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(cands)
	return cands, nil
}

func workspaceBaseDirFromVFS(fsys *vfs.FS) string {
	if fsys == nil {
		return ""
	}
	_, r, _, err := fsys.Resolve("/" + vfs.MountScratch)
	if err != nil || r == nil {
		return ""
	}
	dr, ok := r.(*resources.DirResource)
	if !ok {
		return ""
	}
	return strings.TrimSpace(dr.BaseDir)
}

func normalizeExplicitRefVPath(token string) (vpath string, ok bool) {
	token = strings.TrimSpace(token)
	if token == "" || !strings.HasPrefix(token, "/") {
		return "", false
	}
	switch {
	case strings.HasPrefix(token, "/project/"):
		sub := strings.TrimPrefix(token, "/project/")
		clean, _, err := vfsutil.NormalizeResourceSubpath(sub)
		if err != nil || clean == "" || clean == "." {
			return "", false
		}
		return "/project/" + clean, true
	case strings.HasPrefix(token, "/scratch/"):
		sub := strings.TrimPrefix(token, "/scratch/")
		clean, _, err := vfsutil.NormalizeResourceSubpath(sub)
		if err != nil || clean == "" || clean == "." {
			return "", false
		}
		return "/scratch/" + clean, true
	case strings.HasPrefix(token, "/results/"):
		sub := strings.TrimPrefix(token, "/results/")
		clean, _, err := vfsutil.NormalizeResourceSubpath(sub)
		if err != nil || clean == "" || clean == "." {
			return "", false
		}
		return "/results/" + clean, true
	default:
		return "", false
	}
}

func mergeCandidateVPaths(workdirRels, workspaceRels []string) []string {
	seen := make(map[string]bool, len(workdirRels)+len(workspaceRels))
	out := make([]string, 0, len(workdirRels)+len(workspaceRels))
	add := func(vp string) {
		vp = strings.TrimSpace(vp)
		if vp == "" || seen[vp] {
			return
		}
		seen[vp] = true
		out = append(out, vp)
	}
	for _, rel := range workdirRels {
		rel = strings.TrimSpace(rel)
		if rel == "" {
			continue
		}
		add("/project/" + rel)
	}
	for _, rel := range workspaceRels {
		rel = strings.TrimSpace(rel)
		if rel == "" {
			continue
		}
		add("/scratch/" + rel)
	}
	sort.Strings(out)
	return out
}
