package tools_test

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tinoosan/workbench-core/internal/config"
	"github.com/tinoosan/workbench-core/internal/resources"
	"github.com/tinoosan/workbench-core/internal/store"
	"github.com/tinoosan/workbench-core/internal/tools"
	"github.com/tinoosan/workbench-core/internal/types"
	"github.com/tinoosan/workbench-core/internal/vfs"
)

func TestBuiltinGit_Status_Add_Commit_OK(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed; skipping")
	}

	tmpDir := t.TempDir()
	cfg := config.Config{DataDir: tmpDir}
	run, err := store.CreateRun(cfg, "builtin git status/add/commit test", 100)
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}

	resultsStore := store.NewInMemoryResultsStore()
	resultsRes, err := resources.NewResultsResource(resultsStore)
	if err != nil {
		t.Fatalf("NewResultsResource: %v", err)
	}

	rootDir := t.TempDir()
	gitInitRepo(t, rootDir)

	// Base commit.
	writeFile(t, filepath.Join(rootDir, "a.txt"), "hello\n")
	gitRun(t, rootDir, "add", "a.txt")
	gitRun(t, rootDir, "commit", "-m", "initial")

	// Working tree changes.
	writeFile(t, filepath.Join(rootDir, "a.txt"), "hello\nmore\n")
	writeFile(t, filepath.Join(rootDir, "b.txt"), "untracked\n")

	fs := vfs.NewFS()
	fs.Mount(vfs.MountResults, resultsRes)

	inv := tools.NewBuiltinGitInvoker(rootDir)
	runner := tools.Runner{
		Results: resultsStore,
		ToolRegistry: tools.MapRegistry{
			types.ToolID("builtin.git"): inv,
		},
	}

	// status
	resp, err := runner.Run(context.Background(), types.ToolID("builtin.git"), "status", json.RawMessage(`{"cwd":"."}`), 0)
	if err != nil {
		t.Fatalf("Run status: %v", err)
	}
	if !resp.Ok {
		t.Fatalf("expected ok=true, got %+v", resp)
	}
	var st struct {
		ExitCode   int      `json:"exitCode"`
		Untracked  []string `json:"untracked"`
		Unstaged   []string `json:"unstaged"`
		Staged     []string `json:"staged"`
		Conflicts  []string `json:"conflicts"`
		Head       string   `json:"head"`
		StdoutPath string   `json:"stdoutPath"`
	}
	if err := json.Unmarshal(resp.Output, &st); err != nil {
		t.Fatalf("Unmarshal status output: %v", err)
	}
	if st.ExitCode != 0 {
		// For a valid repo, exitCode should be 0; if it isn't, dump stdoutPath (if any) to help debug.
		t.Fatalf("expected exitCode=0, got %d (stdoutPath=%q)", st.ExitCode, st.StdoutPath)
	}
	if !sliceContains(st.Untracked, "b.txt") {
		t.Fatalf("expected b.txt untracked, got %+v", st.Untracked)
	}
	if !sliceContains(st.Unstaged, "a.txt") {
		t.Fatalf("expected a.txt unstaged, got %+v", st.Unstaged)
	}
	if len(st.Staged) != 0 {
		t.Fatalf("expected no staged files, got %+v", st.Staged)
	}
	if len(st.Conflicts) != 0 {
		t.Fatalf("expected no conflicts, got %+v", st.Conflicts)
	}
	if strings.TrimSpace(st.Head) == "" {
		t.Fatalf("expected head to be non-empty")
	}

	// add a.txt
	resp, err = runner.Run(context.Background(), types.ToolID("builtin.git"), "add", json.RawMessage(`{"cwd":".","files":["a.txt"]}`), 0)
	if err != nil {
		t.Fatalf("Run add: %v", err)
	}
	if !resp.Ok {
		t.Fatalf("expected ok=true, got %+v", resp)
	}
	var addOut struct {
		ExitCode int `json:"exitCode"`
	}
	if err := json.Unmarshal(resp.Output, &addOut); err != nil {
		t.Fatalf("Unmarshal add output: %v", err)
	}
	if addOut.ExitCode != 0 {
		t.Fatalf("expected add exitCode=0, got %d", addOut.ExitCode)
	}

	// status should show staged a.txt now.
	resp, err = runner.Run(context.Background(), types.ToolID("builtin.git"), "status", json.RawMessage(`{"cwd":"."}`), 0)
	if err != nil {
		t.Fatalf("Run status2: %v", err)
	}
	if !resp.Ok {
		t.Fatalf("expected ok=true, got %+v", resp)
	}
	if err := json.Unmarshal(resp.Output, &st); err != nil {
		t.Fatalf("Unmarshal status2 output: %v", err)
	}
	if !sliceContains(st.Staged, "a.txt") {
		t.Fatalf("expected a.txt staged, got %+v", st.Staged)
	}

	// commit
	resp, err = runner.Run(context.Background(), types.ToolID("builtin.git"), "commit", json.RawMessage(`{"cwd":".","message":"update a"}`), 0)
	if err != nil {
		t.Fatalf("Run commit: %v", err)
	}
	if !resp.Ok {
		t.Fatalf("expected ok=true, got %+v", resp)
	}
	var commitOut struct {
		ExitCode   int    `json:"exitCode"`
		Committed  bool   `json:"committed"`
		Head       string `json:"head"`
		Stderr     string `json:"stderr"`
		StderrPath string `json:"stderrPath"`
	}
	if err := json.Unmarshal(resp.Output, &commitOut); err != nil {
		t.Fatalf("Unmarshal commit output: %v", err)
	}
	if commitOut.ExitCode != 0 || !commitOut.Committed {
		t.Fatalf("expected committed exitCode=0, got exitCode=%d committed=%v stderr=%q stderrPath=%q", commitOut.ExitCode, commitOut.Committed, commitOut.Stderr, commitOut.StderrPath)
	}
	if strings.TrimSpace(commitOut.Head) == "" {
		t.Fatalf("expected head after commit")
	}

	// Ensure response.json persisted.
	if _, err := fs.Read("/results/" + resp.CallID + "/response.json"); err != nil {
		t.Fatalf("expected persisted response.json: %v", err)
	}

	// Ensure no on-disk results directory was created for the run.
	if _, err := os.Stat(filepath.Join(tmpDir, "runs", run.RunId, "results")); err == nil {
		t.Fatalf("expected no on-disk results directory")
	}
}

func TestBuiltinGit_Status_RejectsEscapeCwd(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed; skipping")
	}

	tmpDir := t.TempDir()
	cfg := config.Config{DataDir: tmpDir}
	_, err := store.CreateRun(cfg, "builtin git cwd escape test", 100)
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}

	resultsStore := store.NewInMemoryResultsStore()
	_, err = resources.NewResultsResource(resultsStore)
	if err != nil {
		t.Fatalf("NewResultsResource: %v", err)
	}

	runner := tools.Runner{
		Results: resultsStore,
		ToolRegistry: tools.MapRegistry{
			types.ToolID("builtin.git"): tools.NewBuiltinGitInvoker(t.TempDir()),
		},
	}

	resp, err := runner.Run(context.Background(), types.ToolID("builtin.git"), "status", json.RawMessage(`{"cwd":"../"}`), 0)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if resp.Ok {
		t.Fatalf("expected ok=false, got %+v", resp)
	}
	if resp.Error == nil || resp.Error.Code != "invalid_input" {
		t.Fatalf("expected invalid_input error, got %+v", resp.Error)
	}
}

func TestBuiltinGit_Diff_TruncatesAndWritesPatchArtifact(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed; skipping")
	}

	tmpDir := t.TempDir()
	cfg := config.Config{DataDir: tmpDir}
	run, err := store.CreateRun(cfg, "builtin git diff truncation test", 100)
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}

	resultsStore := store.NewInMemoryResultsStore()
	resultsRes, err := resources.NewResultsResource(resultsStore)
	if err != nil {
		t.Fatalf("NewResultsResource: %v", err)
	}

	rootDir := t.TempDir()
	gitInitRepo(t, rootDir)
	writeFile(t, filepath.Join(rootDir, "a.txt"), "hello\n")
	gitRun(t, rootDir, "add", "a.txt")
	gitRun(t, rootDir, "commit", "-m", "initial")

	// Make a big diff.
	writeFile(t, filepath.Join(rootDir, "a.txt"), strings.Repeat("x", 1024*8)+"\n")

	fs := vfs.NewFS()
	fs.Mount(vfs.MountResults, resultsRes)

	inv := tools.NewBuiltinGitInvoker(rootDir)
	inv.MaxBytes = 16
	runner := tools.Runner{
		Results: resultsStore,
		ToolRegistry: tools.MapRegistry{
			types.ToolID("builtin.git"): inv,
		},
	}

	resp, err := runner.Run(context.Background(), types.ToolID("builtin.git"), "diff", json.RawMessage(`{"cwd":".","staged":false}`), 0)
	if err != nil {
		t.Fatalf("Run diff: %v", err)
	}
	if !resp.Ok {
		t.Fatalf("expected ok=true, got %+v", resp)
	}

	var out struct {
		Patch     string `json:"patch"`
		PatchPath string `json:"patchPath"`
		Truncated bool   `json:"truncated"`
	}
	if err := json.Unmarshal(resp.Output, &out); err != nil {
		t.Fatalf("Unmarshal diff output: %v", err)
	}
	if !out.Truncated {
		t.Fatalf("expected truncated=true")
	}
	if out.PatchPath != "diff.patch" {
		t.Fatalf("expected patchPath=diff.patch, got %q", out.PatchPath)
	}
	if len(out.Patch) != 16 {
		t.Fatalf("expected patch preview length 16, got %d", len(out.Patch))
	}

	b, err := fs.Read("/results/" + resp.CallID + "/diff.patch")
	if err != nil {
		t.Fatalf("Read diff.patch artifact: %v", err)
	}
	if len(b) == 0 {
		t.Fatalf("expected non-empty diff.patch artifact")
	}

	// Ensure no on-disk results directory was created for the run.
	if _, err := os.Stat(filepath.Join(tmpDir, "runs", run.RunId, "results")); err == nil {
		t.Fatalf("expected no on-disk results directory")
	}
}

func gitInitRepo(t *testing.T, dir string) {
	t.Helper()
	gitRun(t, dir, "init")
	// Ensure commits work in temp repos without relying on global config.
	gitRun(t, dir, "config", "user.email", "test@example.com")
	gitRun(t, dir, "config", "user.name", "Test User")
}

func gitRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, string(out))
	}
}

func writeFile(t *testing.T, path string, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

func sliceContains(xs []string, want string) bool {
	for _, x := range xs {
		if x == want {
			return true
		}
	}
	return false
}

