package agent

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/tinoosan/workbench-core/internal/config"
	"github.com/tinoosan/workbench-core/pkg/fsutil"
	"github.com/tinoosan/workbench-core/internal/resources"
	"github.com/tinoosan/workbench-core/pkg/skills"
	"github.com/tinoosan/workbench-core/internal/store"
	"github.com/tinoosan/workbench-core/pkg/vfs"
)

type countingResource struct {
	r       vfs.Resource
	readCnt map[string]int
}

func newCountingResource(r vfs.Resource) *countingResource {
	return &countingResource{r: r, readCnt: map[string]int{}}
}

func (c *countingResource) List(path string) ([]vfs.Entry, error) { return c.r.List(path) }
func (c *countingResource) Write(path string, data []byte) error  { return c.r.Write(path, data) }
func (c *countingResource) Append(path string, data []byte) error { return c.r.Append(path, data) }

func (c *countingResource) Read(path string) ([]byte, error) {
	if c.readCnt == nil {
		c.readCnt = map[string]int{}
	}
	c.readCnt[path]++
	return c.r.Read(path)
}

func (c *countingResource) readsFor(path string) int {
	return c.readCnt[path]
}

func TestContextConstructor_CachesProfileAndMemoryPerTurn(t *testing.T) {
	cfg := config.Config{DataDir: t.TempDir()}

	_, run, err := store.CreateSession(cfg, "constructor cache test", 10)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	profStore, err := store.NewDiskProfileStore(cfg)
	if err != nil {
		t.Fatalf("NewDiskProfileStore: %v", err)
	}
	memStore, err := store.NewDiskMemoryStore(cfg, run.RunId)
	if err != nil {
		t.Fatalf("NewDiskMemoryStore: %v", err)
	}

	// Seed committed profile and run memory (directories are created by store constructors).
	if err := os.WriteFile(fsutil.GetProfilePath(cfg.DataDir), []byte("profile: remember me"), 0644); err != nil {
		t.Fatalf("WriteFile profile.md: %v", err)
	}
	if err := os.WriteFile(fsutil.GetRunMemoryPath(cfg.DataDir, run.RunId), []byte("memory: keep this"), 0644); err != nil {
		t.Fatalf("WriteFile memory.md: %v", err)
	}
	profRes, err := resources.NewProfileResource(profStore)
	if err != nil {
		t.Fatalf("NewProfileResource: %v", err)
	}
	memRes, err := resources.NewMemoryResource(memStore)
	if err != nil {
		t.Fatalf("NewMemoryResource: %v", err)
	}
	wsRes, err := resources.NewWorkspace(cfg, run.RunId)
	if err != nil {
		t.Fatalf("NewWorkspace: %v", err)
	}

	countProf := newCountingResource(profRes)
	countMem := newCountingResource(memRes)

	fs := vfs.NewFS()
	fs.Mount(vfs.MountProfile, countProf)
	fs.Mount(vfs.MountMemory, countMem)
	fs.Mount(vfs.MountScratch, wsRes)

	cc := &ContextConstructor{
		FS:              fs,
		Cfg:             cfg,
		RunID:           run.RunId,
		MaxProfileBytes: 2048,
		MaxMemoryBytes:  2048,
	}

	out1, err := cc.SystemPrompt(context.Background(), "base", 1)
	if err != nil {
		t.Fatalf("SystemPrompt step1: %v", err)
	}
	out2, err := cc.SystemPrompt(context.Background(), "base", 2)
	if err != nil {
		t.Fatalf("SystemPrompt step2: %v", err)
	}

	if countProf.readsFor("profile.md") != 1 {
		t.Fatalf("expected profile.md to be read once, got %d", countProf.readsFor("profile.md"))
	}
	if countMem.readsFor("memory.md") != 1 {
		t.Fatalf("expected memory.md to be read once, got %d", countMem.readsFor("memory.md"))
	}

	if !strings.Contains(out1, `<user_profile path="/profile/profile.md"`) || !strings.Contains(out1, "profile: remember me") {
		t.Fatalf("expected profile section in step1 prompt, got:\n%s", out1)
	}
	if !strings.Contains(out1, `<run_memory path="/memory/memory.md"`) || !strings.Contains(out1, "memory: keep this") {
		t.Fatalf("expected memory section in step1 prompt, got:\n%s", out1)
	}
	// Cached sections should still be present on subsequent steps.
	if !strings.Contains(out2, "<user_profile") || !strings.Contains(out2, "<run_memory") {
		t.Fatalf("expected cached sections in step2 prompt, got:\n%s", out2)
	}
}

func TestContextConstructor_AttachmentsIncludedAcrossSteps(t *testing.T) {
	cfg := config.Config{DataDir: t.TempDir()}

	_, run, err := store.CreateSession(cfg, "constructor attachments test", 10)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	wsRes, err := resources.NewWorkspace(cfg, run.RunId)
	if err != nil {
		t.Fatalf("NewWorkspace: %v", err)
	}

	// Minimal mounts needed for constructor state persistence.
	fs := vfs.NewFS()
	fs.Mount(vfs.MountScratch, wsRes)
	// Profile/memory mounts can be absent; reads will error and be treated as empty.

	cc := &ContextConstructor{
		FS:    fs,
		Cfg:   cfg,
		RunID: run.RunId,
	}
	cc.SetFileAttachments([]FileAttachment{
		{
			Token:         "go.mod",
			VPath:         "/project/go.mod",
			DisplayName:   "go.mod",
			Content:       "module example.com/foo\n",
			BytesTotal:    20,
			BytesIncluded: 20,
			Truncated:     false,
		},
	})
	defer cc.ClearFileAttachments()

	out1, err := cc.SystemPrompt(context.Background(), "base", 1)
	if err != nil {
		t.Fatalf("SystemPrompt step1: %v", err)
	}
	out2, err := cc.SystemPrompt(context.Background(), "base", 2)
	if err != nil {
		t.Fatalf("SystemPrompt step2: %v", err)
	}

	if !strings.Contains(out1, "<referenced_files>") || !strings.Contains(out1, "module example.com/foo") {
		t.Fatalf("expected referenced files section in step1 prompt, got:\n%s", out1)
	}
	if !strings.Contains(out2, "<referenced_files>") || !strings.Contains(out2, "module example.com/foo") {
		t.Fatalf("expected referenced files section in step2 prompt, got:\n%s", out2)
	}
}

func TestContextConstructor_SkillsInjectedIntoPrompt(t *testing.T) {
	cfg := config.Config{DataDir: t.TempDir()}

	_, run, err := store.CreateSession(cfg, "constructor skills test", 10)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	wsRes, err := resources.NewWorkspace(cfg, run.RunId)
	if err != nil {
		t.Fatalf("NewWorkspace: %v", err)
	}

	// Create a temporary skills directory with a test skill.
	skillsDir := t.TempDir()
	skillDir := skillsDir + "/hello-world"
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatalf("MkdirAll skill dir: %v", err)
	}
	skillContent := `---
name: hello-world
description: Says hello to the user
---
# Hello World Skill

Run the command: echo "Hello"
`
	if err := os.WriteFile(skillDir+"/SKILL.md", []byte(skillContent), 0644); err != nil {
		t.Fatalf("WriteFile SKILL.md: %v", err)
	}

	// Create and scan skills manager.
	skillMgr := skills.NewManager([]string{skillsDir})
	if err := skillMgr.Scan(); err != nil {
		t.Fatalf("skillMgr.Scan: %v", err)
	}

	fs := vfs.NewFS()
	fs.Mount(vfs.MountScratch, wsRes)

	cc := &ContextConstructor{
		FS:            fs,
		Cfg:           cfg,
		RunID:         run.RunId,
		SkillsManager: skillMgr,
	}

	out, err := cc.SystemPrompt(context.Background(), "base", 1)
	if err != nil {
		t.Fatalf("SystemPrompt: %v", err)
	}

	// Verify skills are injected.
	if !strings.Contains(out, "<available_skills>") {
		t.Fatalf("expected <available_skills> tag in prompt, got:\n%s", out)
	}
	if !strings.Contains(out, "hello-world") {
		t.Fatalf("expected 'hello-world' skill name in prompt, got:\n%s", out)
	}
	if !strings.Contains(out, "Says hello to the user") {
		t.Fatalf("expected skill description in prompt, got:\n%s", out)
	}
	if !strings.Contains(out, "/skills/hello-world/SKILL.md") {
		t.Fatalf("expected skill location in prompt, got:\n%s", out)
	}
}
