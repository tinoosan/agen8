package opcatalog

import (
	"sort"
	"strings"
)

type metadata struct {
	category              string
	useSharedRequestTitle bool
}

var catalog = map[string]metadata{
	"fs_list": {
		category:              "Explored",
		useSharedRequestTitle: true,
	},
	"fs_read": {
		category:              "Explored",
		useSharedRequestTitle: true,
	},
	"fs_search": {
		category:              "Explored",
		useSharedRequestTitle: true,
	},
	"fs_write": {
		category:              "Updated",
		useSharedRequestTitle: true,
	},
	"fs_append": {
		category:              "Updated",
		useSharedRequestTitle: true,
	},
	"fs_edit": {
		category:              "Updated",
		useSharedRequestTitle: true,
	},
	"fs_patch": {
		category:              "Updated",
		useSharedRequestTitle: true,
	},
	"shell_exec": {
		category:              "Ran",
		useSharedRequestTitle: true,
	},
	"code_exec": {
		category:              "Ran",
		useSharedRequestTitle: true,
	},
	"http_fetch": {
		category:              "Fetched",
		useSharedRequestTitle: true,
	},
	"browser": {
		category: "Browsed",
	},
	"trace_run": {
		category:              "Traced",
		useSharedRequestTitle: true,
	},
	"email": {
		category: "Sent",
	},
	"agent_spawn": {
		category:              "Delegated",
		useSharedRequestTitle: true,
	},
	"task_create": {
		category:              "Created",
		useSharedRequestTitle: true,
	},
	"obsidian": {
		category:              "Knowledge",
		useSharedRequestTitle: true,
	},
	"task_review": {
		category:              "Reviewed",
		useSharedRequestTitle: true,
	},
	"soul_update": {
		category:              "Updated",
		useSharedRequestTitle: true,
	},
}

func Category(op string) (string, bool) {
	meta, ok := catalog[strings.TrimSpace(op)]
	if !ok {
		return "", false
	}
	if strings.TrimSpace(meta.category) == "" {
		return "", false
	}
	return meta.category, true
}

func UsesSharedRequestTitle(op string) bool {
	meta, ok := catalog[strings.TrimSpace(op)]
	return ok && meta.useSharedRequestTitle
}

func KnownOps() []string {
	ops := make([]string, 0, len(catalog))
	for op := range catalog {
		ops = append(ops, op)
	}
	sort.Strings(ops)
	return ops
}
