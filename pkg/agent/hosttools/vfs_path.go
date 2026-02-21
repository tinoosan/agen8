package hosttools

import (
	"path"
	"strings"
)

func resolveVFSPath(p string) string {
	pathStr := strings.TrimSpace(p)
	if pathStr == "" {
		return "/project"
	}
	if strings.HasPrefix(pathStr, "/") {
		return pathStr
	}
	cleaned := path.Clean(pathStr)
	joined := path.Join("/project", cleaned)
	if !strings.HasPrefix(joined, "/project") {
		return "/project"
	}
	return joined
}
