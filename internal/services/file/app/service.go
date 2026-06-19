package app

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"os"
	"os/exec"
	pathpkg "path"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/tinoosan/agen8/internal/core/types"
	filedomain "github.com/tinoosan/agen8/internal/services/file/domain/file"
)

const (
	defaultPreviewMaxBytes = 1024 * 1024
	previewMaxBytesCap     = 16 * 1024 * 1024
)

// errGitBaselineNotPermitted aliases the domain sentinel so Baseline can match
// the capability-denied case from the repository.
var errGitBaselineNotPermitted = filedomain.ErrGitBaselineNotPermitted

// remoteGitBaseliner is the optional capability a file repository may implement
// to produce git baselines for files on non-local locations. The repository
// owns the per-location capability gate; the service only asks.
type remoteGitBaseliner interface {
	GitBaseline(ctx context.Context, ref filedomain.Reference, dir, name string) (filedomain.GitBaseline, error)
}

type Service struct {
	files    filedomain.Repository
	projects ProjectLoader
	logger   *slog.Logger
}

type Config struct {
	Files    filedomain.Repository
	Projects ProjectLoader
	Logger   *slog.Logger
}

func NewService(cfg Config) (*Service, error) {
	if cfg.Files == nil {
		return nil, fmt.Errorf("file repository is required")
	}
	if cfg.Projects == nil {
		return nil, fmt.Errorf("file project loader is required")
	}
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default().With("service", "file")
	}
	return &Service{files: cfg.Files, projects: cfg.Projects, logger: logger}, nil
}

type ListDirInput struct {
	ProjectID   types.ProjectID
	ProjectRoot string
	Path        string
}

type ListDirResult struct {
	Path         string     `json:"path"`
	Entries      []DirEntry `json:"entries"`
	BrowseOnly   bool       `json:"browseOnly,omitempty"`
	DisplayName  string     `json:"displayName,omitempty"`
	RootKind     string     `json:"rootKind,omitempty"`
	RootLabel    string     `json:"rootLabel,omitempty"`
	RelativePath string     `json:"relativePath,omitempty"`
}

type DirEntry struct {
	Name         string     `json:"name"`
	DisplayName  string     `json:"displayName,omitempty"`
	Path         string     `json:"path"`
	IsDir        bool       `json:"isDir"`
	Writable     bool       `json:"writable"`
	RootKind     string     `json:"rootKind,omitempty"`
	RootLabel    string     `json:"rootLabel,omitempty"`
	RelativePath string     `json:"relativePath,omitempty"`
	Size         int64      `json:"size,omitempty"`
	HasSize      bool       `json:"hasSize,omitempty"`
	ModifiedAt   *time.Time `json:"modifiedAt,omitempty"`
}

type GetInput struct {
	ProjectID   types.ProjectID
	ProjectRoot string
	Path        string
	MaxBytes    int64
}

type GetResult struct {
	Artifact        types.ArtifactNode `json:"artifact"`
	Content         string             `json:"content"`
	ContentKind     string             `json:"contentKind,omitempty"`
	ContentType     string             `json:"contentType,omitempty"`
	ContentEncoding string             `json:"contentEncoding,omitempty"`
	BytesB64        string             `json:"bytesB64,omitempty"`
	Truncated       bool               `json:"truncated"`
	BytesRead       int                `json:"bytesRead"`
	FileSize        int64              `json:"fileSize,omitempty"`
}

type PathInput struct {
	ProjectID   types.ProjectID
	ProjectRoot string
	Path        string
}

type MoveInput struct {
	ProjectID   types.ProjectID
	ProjectRoot string
	Path        string
	Destination string
}

type UploadInput struct {
	ProjectID   types.ProjectID
	ProjectRoot string
	Path        string
	Bytes       []byte
	Content     string
	BytesB64    string
}

type UploadReaderInput struct {
	ProjectID   types.ProjectID
	ProjectRoot string
	Path        string
	Reader      io.Reader
}

type PathResult struct {
	Path string `json:"path"`
}

func (s *Service) ListDir(ctx context.Context, input ListDirInput) (ListDirResult, error) {
	project, err := s.validProject(ctx, input.ProjectID, input.ProjectRoot)
	if err != nil {
		return ListDirResult{}, err
	}
	vpath := normalizeVPath(input.Path)
	if vpath == "/" {
		return ListDirResult{
			Path:        "/",
			DisplayName: "Files",
			Entries: []DirEntry{
				rootEntry("project", "Project"),
				rootEntry("workspace", "Workspace"),
			},
		}, nil
	}
	resolved, err := resolveVPath(project, vpath)
	if err != nil {
		return ListDirResult{}, err
	}
	info, err := s.files.Stat(ctx, resolved.ref)
	if err != nil {
		return ListDirResult{}, fmt.Errorf("stat %s: %w", vpath, err)
	}
	if !info.IsDir {
		return ListDirResult{}, fmt.Errorf("%s is not a directory", vpath)
	}
	items, err := s.files.ListDir(ctx, resolved.ref)
	if err != nil {
		return ListDirResult{}, fmt.Errorf("list %s: %w", vpath, err)
	}
	entries := make([]DirEntry, 0, len(items))
	for _, item := range items {
		childPath := vpathJoin(vpath, item.Name)
		entries = append(entries, entryForInfo(childPath, item.Info, resolved.rootKind))
	}
	return ListDirResult{
		Path:         vpath,
		Entries:      entries,
		DisplayName:  pathpkg.Base(vpath),
		RootKind:     resolved.rootKind,
		RootLabel:    rootLabel(resolved.rootKind),
		RelativePath: relativeVPath(vpath),
	}, nil
}

func (s *Service) Get(ctx context.Context, input GetInput) (GetResult, error) {
	project, err := s.validProject(ctx, input.ProjectID, input.ProjectRoot)
	if err != nil {
		return GetResult{}, err
	}
	if strings.TrimSpace(input.Path) == "" {
		return GetResult{}, fmt.Errorf("path is required")
	}
	resolved, err := resolveVPath(project, normalizeVPath(input.Path))
	if err != nil {
		return GetResult{}, err
	}
	info, err := s.files.Stat(ctx, resolved.ref)
	if err != nil {
		return GetResult{}, fmt.Errorf("stat %s: %w", resolved.vpath, err)
	}
	if info.IsDir {
		return GetResult{}, fmt.Errorf("%s is a directory", resolved.vpath)
	}
	maxBytes := input.MaxBytes
	if maxBytes <= 0 {
		maxBytes = defaultPreviewMaxBytes
	}
	if maxBytes > previewMaxBytesCap {
		maxBytes = previewMaxBytesCap
	}
	content, err := s.files.Read(ctx, resolved.ref, maxBytes)
	if err != nil {
		return GetResult{}, fmt.Errorf("read %s: %w", resolved.vpath, err)
	}
	raw := content.Bytes
	contentType := detectContentType(resolved.ref.Path, raw)
	contentKind := contentKindFor(contentType, resolved.ref.Path, raw)
	result := GetResult{
		Artifact:        artifactForPath(resolved.vpath),
		ContentKind:     contentKind,
		ContentType:     contentType,
		ContentEncoding: "utf8",
		BytesB64:        base64.StdEncoding.EncodeToString(raw),
		Truncated:       content.Truncated,
		BytesRead:       len(raw),
		FileSize:        content.FileSize,
	}
	if contentKind == "text" {
		result.Content = string(raw)
	}
	return result, nil
}

// BaselineResult carries the git HEAD version of a file so the web can diff
// the working tree against it. Tracked=false (new/untracked file, or no git
// repo) is a normal answer, not an error — the viewer degrades to normal view.
type BaselineResult struct {
	Path      string `json:"path"`
	Tracked   bool   `json:"tracked"`
	Binary    bool   `json:"binary,omitempty"`
	Content   string `json:"content,omitempty"`
	Truncated bool   `json:"truncated,omitempty"`
	// Unsupported carries a human-readable reason when this file's location
	// cannot produce a baseline at all (e.g. remote SSH). A structured answer
	// rather than an error so the viewer can degrade with the real reason
	// instead of a generic failure.
	Unsupported string `json:"unsupported,omitempty"`
}

// Baseline returns the committed (git HEAD) content of a file for diffing.
// v1 supports local locations only: the daemon shells out to git on its own
// host, which says nothing about a file on a remote SSH location.
func (s *Service) Baseline(ctx context.Context, input GetInput) (BaselineResult, error) {
	project, err := s.validProject(ctx, input.ProjectID, input.ProjectRoot)
	if err != nil {
		return BaselineResult{}, err
	}
	if strings.TrimSpace(input.Path) == "" {
		return BaselineResult{}, fmt.Errorf("path is required")
	}
	resolved, err := resolveVPath(project, normalizeVPath(input.Path))
	if err != nil {
		return BaselineResult{}, err
	}
	// `git -C <dir> show HEAD:./<name>` resolves the path relative to the
	// file's own directory, so it works whether the git root is the project
	// root or an ancestor of it.
	dir := filepath.Dir(resolved.ref.Path)
	name := filepath.Base(resolved.ref.Path)

	if locationID := strings.TrimSpace(string(resolved.ref.LocationID)); locationID != "" && locationID != "local" {
		return s.remoteBaseline(ctx, resolved, dir, name)
	}
	if err := validateLocalBaselineDir(project.root, dir); err != nil {
		s.logger.Warn("local git baseline path rejected", "vpath", resolved.vpath, "err", err)
		return BaselineResult{Path: resolved.vpath, Tracked: false}, nil
	}

	out, err := exec.CommandContext(ctx, "git", "-C", dir, "show", "HEAD:./"+name).Output()
	if err != nil {
		// Untracked file, file new in the working tree, or no git repo at
		// all: there is no baseline to diff against. Degrade, don't fail.
		return BaselineResult{Path: resolved.vpath, Tracked: false}, nil
	}
	return s.baselineFromBytes(resolved.vpath, out), nil
}

func validateLocalBaselineDir(projectRoot, dir string) error {
	root, err := filepath.Abs(strings.TrimSpace(projectRoot))
	if err != nil {
		return fmt.Errorf("resolve project root: %w", err)
	}
	dir, err = filepath.Abs(strings.TrimSpace(dir))
	if err != nil {
		return fmt.Errorf("resolve baseline directory: %w", err)
	}
	rootReal, err := filepath.EvalSymlinks(root)
	if err != nil {
		return fmt.Errorf("resolve project root symlinks: %w", err)
	}
	dirReal, err := filepath.EvalSymlinks(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("baseline directory does not exist: %w", err)
		}
		return fmt.Errorf("resolve baseline directory symlinks: %w", err)
	}
	rel, err := filepath.Rel(rootReal, dirReal)
	if err != nil {
		return fmt.Errorf("compare baseline directory to project root: %w", err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return fmt.Errorf("baseline directory escapes project root")
	}
	return nil
}

// remoteBaseline produces a git baseline for a file on a non-local location by
// delegating to a capability-gated remote git runner (the LocationRepository).
// If the repository doesn't support remote baselines, or the location hasn't
// been granted the capability, it degrades to a structured Unsupported answer
// rather than erroring — the viewer shows the reason and falls back to normal
// view.
func (s *Service) remoteBaseline(ctx context.Context, resolved resolvedPath, dir, name string) (BaselineResult, error) {
	baseliner, ok := s.files.(remoteGitBaseliner)
	if !ok {
		return BaselineResult{
			Path:        resolved.vpath,
			Unsupported: "Diff is not available for files on remote locations.",
		}, nil
	}
	baseline, err := baseliner.GitBaseline(ctx, resolved.ref, dir, name)
	if err != nil {
		if errors.Is(err, errGitBaselineNotPermitted) {
			return BaselineResult{
				Path:        resolved.vpath,
				Unsupported: "Diff is off for this location. Enable it on the Locations page to review remote changes.",
			}, nil
		}
		s.logger.Warn("remote git baseline failed", "vpath", resolved.vpath, "err", err)
		return BaselineResult{
			Path:        resolved.vpath,
			Unsupported: "Could not load the remote git baseline.",
		}, nil
	}
	s.logger.Info("remote git baseline", "vpath", resolved.vpath, "tracked", baseline.Tracked, "bytes", len(baseline.Bytes))
	if !baseline.Tracked {
		return BaselineResult{Path: resolved.vpath, Tracked: false}, nil
	}
	return s.baselineFromBytes(resolved.vpath, baseline.Bytes), nil
}

func (s *Service) baselineFromBytes(vpath string, out []byte) BaselineResult {
	result := BaselineResult{Path: vpath, Tracked: true}
	if len(out) > previewMaxBytesCap {
		out = out[:previewMaxBytesCap]
		result.Truncated = true
	}
	if !utf8.Valid(out) {
		result.Binary = true
		return result
	}
	result.Content = string(out)
	return result
}

func (s *Service) CreateDir(ctx context.Context, input PathInput) (PathResult, error) {
	resolved, err := s.resolveWritable(ctx, input.ProjectID, input.ProjectRoot, input.Path)
	if err != nil {
		return PathResult{}, err
	}
	if err := s.files.CreateDir(ctx, resolved.ref); err != nil {
		return PathResult{}, fmt.Errorf("create directory %s: %w", resolved.vpath, err)
	}
	return PathResult{Path: resolved.vpath}, nil
}

func (s *Service) CreateFile(ctx context.Context, input PathInput) (PathResult, error) {
	resolved, err := s.resolveWritable(ctx, input.ProjectID, input.ProjectRoot, input.Path)
	if err != nil {
		return PathResult{}, err
	}
	if err := s.files.CreateFile(ctx, resolved.ref); err != nil {
		return PathResult{}, fmt.Errorf("create file %s: %w", resolved.vpath, err)
	}
	return PathResult{Path: resolved.vpath}, nil
}

func (s *Service) Move(ctx context.Context, input MoveInput) (PathResult, error) {
	src, dst, err := s.resolveMove(ctx, input)
	if err != nil {
		return PathResult{}, err
	}
	if err := s.files.Move(ctx, src.ref, dst.ref); err != nil {
		return PathResult{}, fmt.Errorf("move %s to %s: %w", src.vpath, dst.vpath, err)
	}
	return PathResult{Path: dst.vpath}, nil
}

func (s *Service) Copy(ctx context.Context, input MoveInput) (PathResult, error) {
	src, dst, err := s.resolveMove(ctx, input)
	if err != nil {
		return PathResult{}, err
	}
	if err := s.files.Copy(ctx, src.ref, dst.ref); err != nil {
		return PathResult{}, fmt.Errorf("copy %s to %s: %w", src.vpath, dst.vpath, err)
	}
	return PathResult{Path: dst.vpath}, nil
}

func (s *Service) Delete(ctx context.Context, input PathInput) (struct{}, error) {
	resolved, err := s.resolveWritable(ctx, input.ProjectID, input.ProjectRoot, input.Path)
	if err != nil {
		return struct{}{}, err
	}
	if resolved.vpath == "/project" || resolved.vpath == "/workspace" {
		return struct{}{}, fmt.Errorf("cannot delete file root %s", resolved.vpath)
	}
	if err := s.files.Delete(ctx, resolved.ref); err != nil {
		return struct{}{}, fmt.Errorf("delete %s: %w", resolved.vpath, err)
	}
	return struct{}{}, nil
}

func (s *Service) Upload(ctx context.Context, input UploadInput) (PathResult, error) {
	resolved, err := s.resolveWritable(ctx, input.ProjectID, input.ProjectRoot, input.Path)
	if err != nil {
		return PathResult{}, err
	}
	var raw []byte
	if input.Bytes != nil {
		raw = input.Bytes
	} else if strings.TrimSpace(input.BytesB64) != "" {
		raw, err = base64.StdEncoding.DecodeString(strings.TrimSpace(input.BytesB64))
		if err != nil {
			return PathResult{}, fmt.Errorf("decode upload bytes: %w", err)
		}
	} else {
		raw = []byte(input.Content)
	}
	if err := s.files.WriteFile(ctx, resolved.ref, raw); err != nil {
		return PathResult{}, fmt.Errorf("write upload %s: %w", resolved.vpath, err)
	}
	return PathResult{Path: resolved.vpath}, nil
}

func (s *Service) UploadReader(ctx context.Context, input UploadReaderInput) (PathResult, error) {
	resolved, err := s.resolveWritable(ctx, input.ProjectID, input.ProjectRoot, input.Path)
	if err != nil {
		return PathResult{}, err
	}
	if input.Reader == nil {
		return PathResult{}, fmt.Errorf("reader is required")
	}
	if writer, ok := s.files.(interface {
		WriteFileReader(context.Context, filedomain.Reference, io.Reader) error
	}); ok {
		if err := writer.WriteFileReader(ctx, resolved.ref, input.Reader); err != nil {
			return PathResult{}, fmt.Errorf("write upload %s: %w", resolved.vpath, err)
		}
		return PathResult{Path: resolved.vpath}, nil
	}
	raw, err := io.ReadAll(input.Reader)
	if err != nil {
		return PathResult{}, fmt.Errorf("read upload %s: %w", resolved.vpath, err)
	}
	if err := s.files.WriteFile(ctx, resolved.ref, raw); err != nil {
		return PathResult{}, fmt.Errorf("write upload %s: %w", resolved.vpath, err)
	}
	return PathResult{Path: resolved.vpath}, nil
}

func (s *Service) validProject(ctx context.Context, projectID types.ProjectID, root string) (projectContext, error) {
	if s == nil {
		return projectContext{}, fmt.Errorf("file service is nil")
	}
	projectID = types.ProjectID(strings.TrimSpace(string(projectID)))
	if projectID != "" {
		project, err := s.projects.GetProject(ctx, projectID)
		if err != nil {
			return projectContext{}, fmt.Errorf("load project %s: %w", projectID, err)
		}
		locationID := project.LocationID
		if locationID == "" {
			locationID = "local"
		}
		return projectContext{id: project.ID, root: project.EffectiveRoot(), locationID: locationID}, nil
	}
	root = strings.TrimSpace(root)
	if root == "" {
		return projectContext{}, fmt.Errorf("projectId is required")
	}
	projects, err := s.projects.ListProjects(ctx, ProjectFilter{})
	if err != nil {
		return projectContext{}, fmt.Errorf("check projectRoot: %w", err)
	}
	for _, project := range projects {
		locationID := project.LocationID
		if locationID == "" {
			locationID = "local"
		}
		matches, err := projectRootMatches(locationID, project.Root, root)
		if err != nil {
			return projectContext{}, err
		}
		if matches {
			return projectContext{id: project.ID, root: strings.TrimSpace(project.Root), locationID: locationID}, nil
		}
	}
	return projectContext{}, fmt.Errorf("projectRoot is not registered")
}

func (s *Service) resolveWritable(ctx context.Context, projectID types.ProjectID, projectRoot string, vpath string) (resolvedPath, error) {
	project, err := s.validProject(ctx, projectID, projectRoot)
	if err != nil {
		return resolvedPath{}, err
	}
	if strings.TrimSpace(vpath) == "" {
		return resolvedPath{}, fmt.Errorf("path is required")
	}
	return resolveVPath(project, normalizeVPath(vpath))
}

func (s *Service) resolveMove(ctx context.Context, input MoveInput) (resolvedPath, resolvedPath, error) {
	project, err := s.validProject(ctx, input.ProjectID, input.ProjectRoot)
	if err != nil {
		return resolvedPath{}, resolvedPath{}, err
	}
	if strings.TrimSpace(input.Path) == "" {
		return resolvedPath{}, resolvedPath{}, fmt.Errorf("path is required")
	}
	src, err := resolveVPath(project, normalizeVPath(input.Path))
	if err != nil {
		return resolvedPath{}, resolvedPath{}, err
	}
	if strings.TrimSpace(input.Destination) == "" {
		return resolvedPath{}, resolvedPath{}, fmt.Errorf("destination is required")
	}
	dst, err := resolveVPath(project, normalizeVPath(input.Destination))
	if err != nil {
		return resolvedPath{}, resolvedPath{}, err
	}
	if src.rootKind != dst.rootKind {
		return resolvedPath{}, resolvedPath{}, fmt.Errorf("cannot move files across roots")
	}
	return src, dst, nil
}

type resolvedPath struct {
	vpath    string
	ref      filedomain.Reference
	rootKind string
}

type projectContext struct {
	id         types.ProjectID
	root       string
	locationID types.LocationID
}

func resolveVPath(project projectContext, vpath string) (resolvedPath, error) {
	vpath = normalizeVPath(vpath)
	rootKind := ""
	prefix := ""
	switch {
	case vpath == "/project" || strings.HasPrefix(vpath, "/project/"):
		rootKind = "project"
		prefix = "/project"
	case vpath == "/workspace" || strings.HasPrefix(vpath, "/workspace/"):
		rootKind = "workspace"
		prefix = "/workspace"
	default:
		return resolvedPath{}, fmt.Errorf("path must be under /project or /workspace")
	}
	rootPath := project.root
	if rootKind == "workspace" {
		rootPath = joinPath(project.locationID, project.root, "workspace")
	}
	rel := strings.TrimPrefix(vpath, prefix)
	rel = strings.TrimPrefix(rel, "/")
	filePath := rootPath
	if rel != "" {
		filePath = joinPath(project.locationID, rootPath, rel)
	}
	cleanRoot := cleanLocationPath(project.locationID, rootPath)
	cleanFile := cleanLocationPath(project.locationID, filePath)
	relToRoot, err := relativeLocationPath(project.locationID, cleanRoot, cleanFile)
	if err != nil {
		return resolvedPath{}, fmt.Errorf("resolve relative path: %w", err)
	}
	if relToRoot == ".." || strings.HasPrefix(relToRoot, "../") || strings.HasPrefix(relToRoot, ".."+string(filepath.Separator)) || filepath.IsAbs(relToRoot) {
		return resolvedPath{}, fmt.Errorf("path escapes %s root", rootKind)
	}
	return resolvedPath{
		vpath:    vpath,
		ref:      filedomain.Reference{LocationID: project.locationID, Path: cleanFile},
		rootKind: rootKind,
	}, nil
}

func rootEntry(rootKind string, label string) DirEntry {
	return DirEntry{
		Name:         rootKind,
		DisplayName:  label,
		Path:         "/" + rootKind,
		IsDir:        true,
		Writable:     true,
		RootKind:     rootKind,
		RootLabel:    label,
		RelativePath: "/",
	}
}

func entryForInfo(vpath string, info filedomain.Info, rootKind string) DirEntry {
	modified := info.ModifiedAt.UTC()
	entry := DirEntry{
		Name:         pathpkg.Base(vpath),
		DisplayName:  pathpkg.Base(vpath),
		Path:         vpath,
		IsDir:        info.IsDir,
		Writable:     true,
		RootKind:     rootKind,
		RootLabel:    rootLabel(rootKind),
		RelativePath: relativeVPath(vpath),
		ModifiedAt:   &modified,
	}
	if !info.IsDir {
		entry.Size = info.Size
		entry.HasSize = true
	}
	return entry
}

func artifactForPath(vpath string) types.ArtifactNode {
	name := pathpkg.Base(vpath)
	return types.ArtifactNode{
		NodeKey:     "file:" + vpath,
		Kind:        "file",
		Label:       name,
		DisplayName: name,
		VPath:       vpath,
	}
}

func normalizeVPath(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "/"
	}
	if !strings.HasPrefix(value, "/") {
		value = "/" + value
	}
	return pathpkg.Clean(value)
}

func vpathJoin(base string, name string) string {
	return pathpkg.Clean(strings.TrimRight(base, "/") + "/" + name)
}

func relativeVPath(vpath string) string {
	if vpath == "/project" || vpath == "/workspace" {
		return "/"
	}
	if strings.HasPrefix(vpath, "/project/") {
		return strings.TrimPrefix(vpath, "/project")
	}
	if strings.HasPrefix(vpath, "/workspace/") {
		return strings.TrimPrefix(vpath, "/workspace")
	}
	return vpath
}

func projectRootMatches(locationID types.LocationID, registered string, requested string) (bool, error) {
	if isLocalLocation(locationID) {
		cleanRegistered, err := filepath.Abs(strings.TrimSpace(registered))
		if err != nil {
			return false, fmt.Errorf("resolve registered project root: %w", err)
		}
		cleanRequested, err := filepath.Abs(strings.TrimSpace(requested))
		if err != nil {
			return false, fmt.Errorf("resolve projectRoot: %w", err)
		}
		return filepath.Clean(cleanRegistered) == filepath.Clean(cleanRequested), nil
	}
	return pathpkg.Clean(strings.TrimSpace(registered)) == pathpkg.Clean(strings.TrimSpace(requested)), nil
}

func isLocalLocation(locationID types.LocationID) bool {
	return strings.TrimSpace(string(locationID)) == "" || strings.TrimSpace(string(locationID)) == "local"
}

func joinPath(locationID types.LocationID, base string, name string) string {
	if isLocalLocation(locationID) {
		return filepath.Join(base, filepath.FromSlash(name))
	}
	return pathpkg.Join(base, name)
}

func cleanLocationPath(locationID types.LocationID, value string) string {
	if isLocalLocation(locationID) {
		return filepath.Clean(value)
	}
	return pathpkg.Clean(strings.ReplaceAll(value, "\\", "/"))
}

func relativeLocationPath(locationID types.LocationID, root string, target string) (string, error) {
	if isLocalLocation(locationID) {
		return filepath.Rel(root, target)
	}
	root = strings.TrimRight(pathpkg.Clean(root), "/")
	target = pathpkg.Clean(target)
	if target == root {
		return ".", nil
	}
	if strings.HasPrefix(target, root+"/") {
		return strings.TrimPrefix(target, root+"/"), nil
	}
	return "..", nil
}

func rootLabel(rootKind string) string {
	if rootKind == "project" {
		return "Project"
	}
	return "Workspace"
}

func detectContentType(diskPath string, raw []byte) string {
	if contentType := mime.TypeByExtension(strings.ToLower(filepath.Ext(diskPath))); strings.TrimSpace(contentType) != "" {
		return contentType
	}
	if len(raw) == 0 {
		return "application/octet-stream"
	}
	return http.DetectContentType(raw)
}

func contentKindFor(contentType string, diskPath string, raw []byte) string {
	lower := strings.ToLower(contentType)
	ext := strings.ToLower(filepath.Ext(diskPath))
	if strings.Contains(lower, "pdf") {
		return "pdf"
	}
	if strings.HasPrefix(lower, "image/") && ext != ".svg" {
		return "image"
	}
	if strings.HasPrefix(lower, "text/") || strings.Contains(lower, "json") || strings.Contains(lower, "xml") || ext == ".svg" {
		return "text"
	}
	if utf8.Valid(raw) {
		return "text"
	}
	return "binary"
}
