package app

import (
	"path"
	"strings"
	"time"
)

// ArtifactIndex is a small, in-memory index of files the agent created or touched.
//
// This is a UX helper only. The source of truth remains:
// - VFS mounts (/workspace, /project)
// - persisted events/history logs
//
// The index enables:
// - @file references to pick up recently written workspace artifacts
// - a simple "publish to workdir" workflow without hunting through run folders
type ArtifactIndex struct {
	items []Artifact
}

// Artifact describes one indexed file.
type Artifact struct {
	Name      string    // basename, e.g. "report.md"
	VPath     string    // e.g. "/workspace/report.md"
	Origin    string    // "workspace", "workdir"
	UpdatedAt time.Time // best-effort host time

	// PublishedVPath is set when the artifact is copied to /project (or a subdir).
	PublishedVPath string
}

func newArtifactIndex() *ArtifactIndex { return &ArtifactIndex{} }

func (x *ArtifactIndex) ObserveWrite(vpath string) {
	if x == nil {
		return
	}
	vpath = strings.TrimSpace(vpath)
	if vpath == "" {
		return
	}
	origin := ""
	switch {
	case strings.HasPrefix(vpath, "/workspace/"):
		origin = "workspace"
	case strings.HasPrefix(vpath, "/project/"):
		origin = "workdir"
	default:
		return
	}
	name := path.Base(vpath)
	if name == "." || name == "/" || name == "" {
		return
	}
	now := time.Now()

	// Update existing entry if present.
	for i := len(x.items) - 1; i >= 0; i-- {
		if x.items[i].VPath == vpath {
			x.items[i].UpdatedAt = now
			x.items[i].Origin = origin
			return
		}
	}
	x.items = append(x.items, Artifact{
		Name:      name,
		VPath:     vpath,
		Origin:    origin,
		UpdatedAt: now,
	})
}

func (x *ArtifactIndex) RecordPublish(srcVPath, dstVPath string) {
	if x == nil {
		return
	}
	srcVPath = strings.TrimSpace(srcVPath)
	dstVPath = strings.TrimSpace(dstVPath)
	if srcVPath == "" || dstVPath == "" {
		return
	}
	for i := len(x.items) - 1; i >= 0; i-- {
		if x.items[i].VPath == srcVPath {
			x.items[i].PublishedVPath = dstVPath
			return
		}
	}
}

// Resolve returns the VFS path for a token, using the artifact index only.
//
// token may be:
// - "last" (last observed artifact)
// - an artifact name (basename)
func (x *ArtifactIndex) Resolve(token string) (vpath string, ok bool) {
	if x == nil {
		return "", false
	}
	token = strings.TrimSpace(strings.TrimPrefix(token, "@"))
	if token == "" {
		return "", false
	}
	if strings.EqualFold(token, "last") {
		if len(x.items) == 0 {
			return "", false
		}
		return x.items[len(x.items)-1].VPath, true
	}
	// Prefer most recently updated match.
	for i := len(x.items) - 1; i >= 0; i-- {
		if x.items[i].Name == token || strings.TrimPrefix(x.items[i].VPath, "/workspace/") == token {
			return x.items[i].VPath, true
		}
	}
	return "", false
}
