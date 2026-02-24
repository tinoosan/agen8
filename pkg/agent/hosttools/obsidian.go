package hosttools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	llmtypes "github.com/tinoosan/agen8/pkg/llm/types"
	pkgobsidian "github.com/tinoosan/agen8/pkg/obsidian"
	"github.com/tinoosan/agen8/pkg/types"
)

var obsidianWikilinkRE = regexp.MustCompile(`\[\[([^\]]+)\]\]`)

type ObsidianTool struct {
	ProjectRoot string
}

func (t *ObsidianTool) Definition() llmtypes.Tool {
	return llmtypes.Tool{
		Type: "function",
		Function: llmtypes.ToolFunction{
			Name:        "obsidian",
			Description: "[KNOWLEDGE] Manage Obsidian-compatible vault notes using init/search/graph/upsert_note. command is optional: inferred as upsert_note for note-write fields, search for query/tag/link filters, otherwise graph. For upsert_note, noteType is optional and inferred from file/title cues, then defaults to F.",
			Strict:      false,
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"command": map[string]any{
						"type":        "string",
						"description": "Optional. One of: init, search, graph, upsert_note.",
					},
					"path": map[string]any{
						"type":        "string",
						"description": "Optional vault path (supports /project/... or /knowledge/...).",
					},
					"tag":   map[string]any{"type": "string"},
					"link":  map[string]any{"type": "string"},
					"type":  map[string]any{"type": "string"},
					"query": map[string]any{"type": "string"},
					"in":    map[string]any{"type": "string"},
					"top":   map[string]any{"type": "integer"},

					"file":      map[string]any{"type": "string"},
					"title":     map[string]any{"type": "string"},
					"noteType":  map[string]any{"type": "string", "description": "Optional for upsert_note. If omitted, inferred from path/title cues; defaults to F."},
					"content":   map[string]any{"type": "string"},
					"tags":      stringArrayOrNull,
					"aliases":   stringArrayOrNull,
					"source":    map[string]any{"type": "string"},
					"up":        map[string]any{"type": "string"},
					"id":        map[string]any{"type": "string"},
					"overwrite": map[string]any{"type": "boolean"},
				},
				"required":             []any{},
				"additionalProperties": false,
			},
		},
	}
}

type obsidianArgs struct {
	Command string `json:"command"`
	Path    string `json:"path,omitempty"`

	Tag   string `json:"tag,omitempty"`
	Link  string `json:"link,omitempty"`
	Type  string `json:"type,omitempty"`
	Query string `json:"query,omitempty"`
	In    string `json:"in,omitempty"`
	Top   int    `json:"top,omitempty"`

	File      string   `json:"file,omitempty"`
	Title     string   `json:"title,omitempty"`
	NoteType  string   `json:"noteType,omitempty"`
	Content   string   `json:"content,omitempty"`
	Tags      []string `json:"tags,omitempty"`
	Aliases   []string `json:"aliases,omitempty"`
	Source    string   `json:"source,omitempty"`
	Up        string   `json:"up,omitempty"`
	ID        string   `json:"id,omitempty"`
	Overwrite bool     `json:"overwrite,omitempty"`
}

func (t *ObsidianTool) Execute(_ context.Context, args json.RawMessage) (types.HostOpRequest, error) {
	status := pkgobsidian.DetectInstall()
	if !status.Installed {
		return types.HostOpRequest{}, fmt.Errorf("OBSIDIAN_NOT_INSTALLED: Obsidian desktop/CLI was not detected on host")
	}

	var in obsidianArgs
	if err := json.Unmarshal(args, &in); err != nil {
		return types.HostOpRequest{}, err
	}
	cmd := strings.ToLower(strings.TrimSpace(in.Command))
	if cmd == "" {
		cmd = inferObsidianCommand(in)
	}

	projectRoot := strings.TrimSpace(t.ProjectRoot)
	projectPath := pkgobsidian.ResolveProjectVaultPath(projectRoot)
	def, err := pkgobsidian.ResolveDefaultVaultPath(projectRoot, projectPath)
	if err != nil {
		return types.HostOpRequest{}, err
	}
	resolved, err := pkgobsidian.ResolveVaultPath(pkgobsidian.ResolveOptions{
		ExplicitPath:      strings.TrimSpace(in.Path),
		ProjectRoot:       projectRoot,
		ProjectVaultPath:  projectPath,
		KnowledgeRootHost: def.Host,
	})
	if err != nil {
		return types.HostOpRequest{}, err
	}

	var out any
	switch cmd {
	case "init":
		out, err = obsidianInit(resolved.Host)
	case "search":
		out, err = obsidianSearch(resolved.Host, in)
	case "graph":
		out, err = obsidianGraph(resolved.Host, in)
	case "upsert_note":
		out, err = obsidianUpsertNote(resolved.Host, in)
	default:
		return types.HostOpRequest{}, fmt.Errorf("obsidian.command must be one of init/search/graph/upsert_note")
	}
	if err != nil {
		return types.HostOpRequest{}, err
	}
	payload := map[string]any{
		"command": cmd,
		"data":    out,
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return types.HostOpRequest{}, err
	}
	return types.HostOpRequest{
		Op:    types.HostOpToolResult,
		Tag:   "obsidian",
		Text:  string(b),
		Input: b,
	}, nil
}

func inferObsidianCommand(in obsidianArgs) string {
	if strings.TrimSpace(in.NoteType) != "" ||
		strings.TrimSpace(in.Title) != "" ||
		strings.TrimSpace(in.Content) != "" ||
		strings.TrimSpace(in.File) != "" ||
		strings.TrimSpace(in.Source) != "" ||
		strings.TrimSpace(in.Up) != "" ||
		strings.TrimSpace(in.ID) != "" ||
		len(in.Tags) > 0 ||
		len(in.Aliases) > 0 ||
		in.Overwrite {
		return "upsert_note"
	}
	if strings.TrimSpace(in.Query) != "" ||
		strings.TrimSpace(in.Tag) != "" ||
		strings.TrimSpace(in.Link) != "" ||
		strings.TrimSpace(in.Type) != "" ||
		strings.TrimSpace(in.In) != "" {
		return "search"
	}
	return "graph"
}

func obsidianInit(vault string) (map[string]any, error) {
	if err := os.MkdirAll(vault, 0o755); err != nil {
		return nil, err
	}
	dirs := []string{"inbox", "notes", "mocs", "journals", "templates", ".obsidian"}
	createdDirs := make([]string, 0)
	existingDirs := make([]string, 0)
	for _, rel := range dirs {
		p := filepath.Join(vault, rel)
		if st, err := os.Stat(p); err == nil && st.IsDir() {
			existingDirs = append(existingDirs, rel)
			continue
		}
		if err := os.MkdirAll(p, 0o755); err != nil {
			return nil, err
		}
		createdDirs = append(createdDirs, rel)
	}

	templates := map[string]string{
		"templates/fleeting.md":   templateText("F"),
		"templates/literature.md": templateText("L"),
		"templates/permanent.md":  templateText("P"),
		"templates/moc.md":        templateText("MOC"),
		"templates/journal.md":    templateText("JOURNAL"),
		"mocs/index.md":           "---\ntype: \"MOC\"\ntitle: \"Knowledge Index\"\ncreated: \"\"\ntags: [moc]\n---\n\n## Topics\n",
		".obsidian/app.json":      `{"legacyEditor": false, "showInlineTitle": true}`,
	}
	createdFiles := make([]string, 0)
	existingFiles := make([]string, 0)
	for rel, content := range templates {
		p := filepath.Join(vault, rel)
		if _, err := os.Stat(p); err == nil {
			existingFiles = append(existingFiles, rel)
			continue
		}
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return nil, err
		}
		if err := writeAtomic(p, []byte(content)); err != nil {
			return nil, err
		}
		createdFiles = append(createdFiles, rel)
	}
	sort.Strings(createdDirs)
	sort.Strings(existingDirs)
	sort.Strings(createdFiles)
	sort.Strings(existingFiles)
	return map[string]any{
		"status":        "ok",
		"vaultPath":     vault,
		"createdDirs":   createdDirs,
		"existingDirs":  existingDirs,
		"createdFiles":  createdFiles,
		"existingFiles": existingFiles,
	}, nil
}

func obsidianSearch(vault string, in obsidianArgs) ([]map[string]any, error) {
	files, err := markdownFiles(vault)
	if err != nil {
		return nil, err
	}
	tagFilter := strings.TrimSpace(in.Tag)
	linkFilter := normalizeLink(strings.TrimSpace(in.Link))
	typeFilter := strings.ToUpper(strings.TrimSpace(in.Type))
	queryFilter := strings.ToLower(strings.TrimSpace(in.Query))
	inScope := strings.Trim(strings.TrimSpace(in.In), "/")

	results := make([]map[string]any, 0)
	for _, path := range files {
		rel, _ := filepath.Rel(vault, path)
		rel = filepath.ToSlash(rel)
		if inScope != "" {
			if rel != inScope && !strings.HasPrefix(rel, inScope+"/") {
				continue
			}
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		text := string(raw)
		front, body := splitFrontmatter(text)
		noteType := parseFM(front, "type")
		title := parseFM(front, "title")
		if title == "" {
			title = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		}

		allTags := extractTags(front + "\n" + body)
		links := obsidianWikilinkRE.FindAllStringSubmatch(text, -1)
		normalizedLinks := map[string]struct{}{}
		for _, m := range links {
			if len(m) < 2 {
				continue
			}
			normalizedLinks[normalizeLink(m[1])] = struct{}{}
		}

		matchTypes := make([]string, 0)
		if typeFilter != "" {
			if strings.ToUpper(noteType) != typeFilter {
				continue
			}
			matchTypes = append(matchTypes, "type")
		}
		if tagFilter != "" {
			if _, ok := allTags[tagFilter]; !ok {
				continue
			}
			matchTypes = append(matchTypes, "tag")
		}
		if linkFilter != "" {
			if _, ok := normalizedLinks[linkFilter]; !ok {
				continue
			}
			matchTypes = append(matchTypes, "link")
		}
		snippets := make([]string, 0)
		if queryFilter != "" {
			lines := strings.Split(text, "\n")
			for _, line := range lines {
				if strings.Contains(strings.ToLower(line), queryFilter) {
					snippets = append(snippets, strings.TrimSpace(line))
					if len(snippets) >= 3 {
						break
					}
				}
			}
			if len(snippets) == 0 {
				continue
			}
			matchTypes = append(matchTypes, "query")
		}
		if len(matchTypes) == 0 {
			matchTypes = append(matchTypes, "all")
		}
		results = append(results, map[string]any{
			"file":     path,
			"relative": rel,
			"title":    title,
			"types":    []string{strings.ToUpper(strings.TrimSpace(noteType))},
			"matched":  matchTypes,
			"snippets": snippets,
		})
	}
	sort.Slice(results, func(i, j int) bool {
		return fmt.Sprint(results[i]["relative"]) < fmt.Sprint(results[j]["relative"])
	})
	return results, nil
}

func obsidianGraph(vault string, in obsidianArgs) (map[string]any, error) {
	files, err := markdownFiles(vault)
	if err != nil {
		return nil, err
	}
	type noteInfo struct {
		Rel         string
		Type        string
		HasFM       bool
		Links       []string
		BasenameKey string
	}
	notes := make([]noteInfo, 0, len(files))
	basenameIndex := map[string][]string{}
	for _, path := range files {
		rel, _ := filepath.Rel(vault, path)
		rel = filepath.ToSlash(rel)
		bn := strings.ToLower(strings.TrimSuffix(filepath.Base(rel), filepath.Ext(rel)))
		basenameIndex[bn] = append(basenameIndex[bn], rel)

		raw, _ := os.ReadFile(path)
		text := string(raw)
		front, body := splitFrontmatter(text)
		links := make([]string, 0)
		for _, m := range obsidianWikilinkRE.FindAllStringSubmatch(body, -1) {
			if len(m) > 1 {
				links = append(links, m[1])
			}
		}
		notes = append(notes, noteInfo{
			Rel:         rel,
			Type:        strings.ToUpper(strings.TrimSpace(parseFM(front, "type"))),
			HasFM:       strings.TrimSpace(front) != "",
			Links:       links,
			BasenameKey: bn,
		})
	}

	inbound := map[string]int{}
	outbound := map[string]int{}
	broken := make([]map[string]string, 0)
	for _, n := range notes {
		for _, raw := range n.Links {
			outbound[n.Rel]++
			norm := normalizeLink(raw)
			targets := basenameIndex[norm]
			if len(targets) == 0 {
				broken = append(broken, map[string]string{
					"from":              n.Rel,
					"raw_target":        raw,
					"normalized_target": norm,
				})
				continue
			}
			sort.Strings(targets)
			inbound[targets[0]]++
		}
	}

	orphans := make([]string, 0)
	typeCounts := map[string]int{}
	withFM := 0
	withoutFM := 0
	hubs := make([]map[string]any, 0, len(notes))
	for _, n := range notes {
		if n.HasFM {
			withFM++
		} else {
			withoutFM++
		}
		nType := n.Type
		if nType == "" {
			nType = "UNKNOWN"
		}
		typeCounts[nType]++
		total := inbound[n.Rel] + outbound[n.Rel]
		hubs = append(hubs, map[string]any{
			"note":     n.Rel,
			"inbound":  inbound[n.Rel],
			"outbound": outbound[n.Rel],
			"total":    total,
		})
		if inbound[n.Rel] == 0 && outbound[n.Rel] == 0 {
			orphans = append(orphans, n.Rel)
		}
	}
	sort.Strings(orphans)
	sort.Slice(hubs, func(i, j int) bool {
		li := hubs[i]["total"].(int)
		lj := hubs[j]["total"].(int)
		if li != lj {
			return li > lj
		}
		return fmt.Sprint(hubs[i]["note"]) < fmt.Sprint(hubs[j]["note"])
	})
	top := in.Top
	if top <= 0 {
		top = 10
	}
	if top < len(hubs) {
		hubs = hubs[:top]
	}
	return map[string]any{
		"status":    "ok",
		"vaultPath": vault,
		"stats": map[string]any{
			"total_notes":               len(notes),
			"total_links":               sumCounts(outbound),
			"notes_with_frontmatter":    withFM,
			"notes_without_frontmatter": withoutFM,
		},
		"orphans":        orphans,
		"broken_links":   broken,
		"top_hubs":       hubs,
		"type_breakdown": typeCounts,
	}, nil
}

func obsidianUpsertNote(vault string, in obsidianArgs) (map[string]any, error) {
	noteType := resolveUpsertNoteType(in)
	title := strings.TrimSpace(in.Title)
	if title == "" {
		return nil, fmt.Errorf("obsidian.title is required for upsert_note")
	}
	now := time.Now().UTC()
	id := strings.TrimSpace(in.ID)
	if id == "" {
		id = fmt.Sprintf("%s-%s-%s", noteType, now.Format("20060102150405"), slugify(title))
	}
	created := now.Format(time.RFC3339)

	target := strings.TrimSpace(in.File)
	if target == "" {
		sub := "notes"
		switch noteType {
		case "F":
			sub = "inbox"
		case "MOC":
			sub = "mocs"
		case "JOURNAL":
			sub = "journals"
		}
		target = filepath.Join(vault, sub, fmt.Sprintf("%s-%s-%s.md", noteType, now.Format("20060102150405"), slugify(title)))
	} else {
		if strings.HasPrefix(target, "/") {
			target = filepath.Clean(target)
		} else {
			target = filepath.Join(vault, target)
		}
		if !strings.HasPrefix(filepath.Clean(target), filepath.Clean(vault)) {
			return nil, fmt.Errorf("obsidian.file must be inside vault root")
		}
	}

	if _, err := os.Stat(target); err == nil && !in.Overwrite {
		return nil, fmt.Errorf("target note exists; set overwrite=true to update")
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return nil, err
	}
	content := buildNoteContent(id, noteType, title, created, in.Tags, in.Aliases, in.Source, in.Up, in.Content)
	if err := writeAtomic(target, []byte(content)); err != nil {
		return nil, err
	}
	return map[string]any{
		"status":    "ok",
		"file":      target,
		"id":        id,
		"type":      noteType,
		"title":     title,
		"createdAt": created,
		"overwrote": in.Overwrite,
	}, nil
}

func resolveUpsertNoteType(in obsidianArgs) string {
	if explicit := strings.ToUpper(strings.TrimSpace(in.NoteType)); explicit != "" {
		return explicit
	}

	fileLower := strings.ToLower(filepath.ToSlash(strings.TrimSpace(in.File)))
	switch {
	case strings.Contains(fileLower, "/journals/") || strings.HasPrefix(fileLower, "journals/"):
		return "JOURNAL"
	case strings.Contains(fileLower, "/mocs/") || strings.HasPrefix(fileLower, "mocs/"):
		return "MOC"
	case strings.Contains(fileLower, "/inbox/") || strings.HasPrefix(fileLower, "inbox/"):
		return "F"
	}

	nameCue := strings.ToLower(strings.TrimSpace(in.Title))
	if nameCue == "" {
		nameCue = strings.ToLower(strings.TrimSpace(strings.TrimSuffix(filepath.Base(in.File), filepath.Ext(in.File))))
	}
	switch {
	case strings.Contains(nameCue, "journal"):
		return "JOURNAL"
	case strings.Contains(nameCue, "moc"):
		return "MOC"
	}

	return "F"
}

func buildNoteContent(id string, noteType string, title string, created string, tags []string, aliases []string, source string, up string, body string) string {
	tagBits := make([]string, 0, len(tags))
	for _, t := range tags {
		t = strings.TrimSpace(t)
		if t != "" {
			tagBits = append(tagBits, t)
		}
	}
	aliasBits := make([]string, 0, len(aliases))
	for _, a := range aliases {
		a = strings.TrimSpace(a)
		if a != "" {
			aliasBits = append(aliasBits, strconvQuote(a))
		}
	}
	lines := []string{
		"---",
		fmt.Sprintf("id: %q", id),
		fmt.Sprintf("type: %q", noteType),
		fmt.Sprintf("title: %q", title),
		fmt.Sprintf("created: %q", created),
		fmt.Sprintf("tags: [%s]", strings.Join(tagBits, ", ")),
		fmt.Sprintf("aliases: [%s]", strings.Join(aliasBits, ", ")),
		fmt.Sprintf("source: %q", strings.TrimSpace(source)),
		fmt.Sprintf("up: %q", strings.TrimSpace(up)),
		"---",
		"",
		strings.TrimSpace(body),
		"",
	}
	return strings.Join(lines, "\n")
}

func templateText(noteType string) string {
	return fmt.Sprintf("---\ntype: %q\ntitle: \"\"\ncreated: \"\"\ntags: []\nsource: \"\"\nup: \"\"\n---\n\n", noteType)
}

func splitFrontmatter(text string) (front string, body string) {
	if !strings.HasPrefix(text, "---\n") {
		return "", text
	}
	end := strings.Index(text[4:], "\n---\n")
	if end < 0 {
		return "", text
	}
	end += 4
	return text[4:end], text[end+5:]
}

func parseFM(front string, key string) string {
	lines := strings.Split(front, "\n")
	prefix := key + ":"
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, prefix) {
			continue
		}
		val := strings.TrimSpace(strings.TrimPrefix(line, prefix))
		val = strings.Trim(val, `"'`)
		return val
	}
	return ""
}

func extractTags(text string) map[string]struct{} {
	out := map[string]struct{}{}
	frontTags := regexp.MustCompile(`(?m)^tags:\s*\[([^\]]*)\]`).FindStringSubmatch(text)
	if len(frontTags) > 1 {
		for _, bit := range strings.Split(frontTags[1], ",") {
			tag := strings.TrimSpace(strings.Trim(bit, `"'`))
			if tag != "" {
				out[tag] = struct{}{}
			}
		}
	}
	inline := regexp.MustCompile(`(^|\s)#([\w\-/]+)`).FindAllStringSubmatch(text, -1)
	for _, m := range inline {
		if len(m) > 2 {
			out[m[2]] = struct{}{}
		}
	}
	return out
}

func markdownFiles(root string) ([]string, error) {
	files := make([]string, 0)
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if strings.EqualFold(filepath.Ext(path), ".md") {
			files = append(files, path)
		}
		return nil
	})
	sort.Strings(files)
	return files, err
}

func normalizeLink(raw string) string {
	v := strings.TrimSpace(raw)
	if i := strings.Index(v, "|"); i >= 0 {
		v = v[:i]
	}
	if i := strings.Index(v, "#"); i >= 0 {
		v = v[:i]
	}
	v = strings.TrimSuffix(v, ".md")
	v = strings.TrimSuffix(v, ".MD")
	v = filepath.Base(v)
	return strings.ToLower(strings.TrimSpace(v))
}

func sumCounts(m map[string]int) int {
	total := 0
	for _, n := range m {
		total += n
	}
	return total
}

func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	re := regexp.MustCompile(`[^a-z0-9]+`)
	s = re.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if s == "" {
		return "note"
	}
	return s
}

func writeAtomic(path string, data []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func strconvQuote(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return `"` + s + `"`
}
