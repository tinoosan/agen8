package app

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/tinoosan/workbench-core/pkg/fsutil"
	"golang.org/x/term"
)

const (
	envDisableDefaultsSeed    = "WORKBENCH_NO_DEFAULTS_SEED"
	skillsMigrationMarkerFile = ".workbench-skills-migrated-v1"
)

type seedConflictStrategy int

const (
	seedConflictUnset seedConflictStrategy = iota
	seedConflictOverwrite
	seedConflictKeep
	seedConflictAbort
)

var (
	detectInteractiveTTY = func() bool {
		return term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stdout.Fd()))
	}
)

func maybeSeedRepoDefaults(dataDir string) error {
	if envBoolDefault(envDisableDefaultsSeed, false) {
		return nil
	}

	defaultsRoot, ok := findRepoDefaultsRoot()
	if !ok {
		return nil
	}

	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return fmt.Errorf("prepare data dir: %w", err)
	}

	if err := seedDir(filepath.Join(defaultsRoot, "profiles"), filepath.Join(dataDir, "profiles")); err != nil {
		return fmt.Errorf("seed profiles: %w", err)
	}

	agentsHome, err := fsutil.GetAgentsHomeDir()
	if err != nil {
		return fmt.Errorf("resolve agents home: %w", err)
	}
	skillsDst, err := fsutil.GetAgentsSkillsDir()
	if err != nil {
		return fmt.Errorf("resolve agents skills dir: %w", err)
	}
	if err := os.MkdirAll(agentsHome, 0o755); err != nil {
		return fmt.Errorf("prepare agents home: %w", err)
	}
	if err := os.MkdirAll(skillsDst, 0o755); err != nil {
		return fmt.Errorf("prepare skills dir: %w", err)
	}

	resolver := newSeedConflictResolver(detectInteractiveTTY(), os.Stdin, os.Stdout)

	markerPath := filepath.Join(agentsHome, skillsMigrationMarkerFile)
	if _, err := os.Stat(markerPath); errors.Is(err, os.ErrNotExist) {
		legacySkills := fsutil.GetSkillsDir(dataDir)
		if err := copyDirWithConflictStrategy(legacySkills, skillsDst, resolver); err != nil {
			return fmt.Errorf("migrate legacy skills: %w", err)
		}
		if err := os.WriteFile(markerPath, []byte("ok\n"), 0o644); err != nil {
			return fmt.Errorf("write skills migration marker: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("stat skills migration marker: %w", err)
	}

	if err := copyDirWithConflictStrategy(filepath.Join(defaultsRoot, "skills"), skillsDst, resolver); err != nil {
		return fmt.Errorf("seed skills: %w", err)
	}
	resolver.logSummary()
	return nil
}

func findRepoDefaultsRoot() (string, bool) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", false
	}
	dir := cwd
	for {
		candidates := []string{
			filepath.Join(dir, "defaults"),
			filepath.Join(dir, "workbench-core", "defaults"),
		}
		for _, candidate := range candidates {
			if isDir(filepath.Join(candidate, "profiles")) || isDir(filepath.Join(candidate, "skills")) {
				return candidate, true
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}

func seedDir(srcDir, dstDir string) error {
	if !isDir(srcDir) {
		return nil
	}
	if err := os.RemoveAll(dstDir); err != nil {
		return err
	}
	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		return err
	}
	return filepath.WalkDir(srcDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		// Refuse traversal even though Rel+WalkDir should keep us inside.
		if strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." {
			return fmt.Errorf("invalid defaults path %q", rel)
		}
		target := filepath.Join(dstDir, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlinks are not supported in defaults: %s", path)
		}
		return copyFile(path, target, info.Mode().Perm())
	})
}

func copyDirWithConflictStrategy(srcDir, dstDir string, resolver *seedConflictResolver) error {
	if !isDir(srcDir) {
		return nil
	}
	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		return err
	}
	return filepath.WalkDir(srcDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		if strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." {
			return fmt.Errorf("invalid defaults path %q", rel)
		}

		target := filepath.Join(dstDir, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}

		info, err := d.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlinks are not supported in defaults: %s", path)
		}

		if _, err := os.Stat(target); err == nil {
			if resolver == nil {
				return nil
			}
			overwrite, err := resolver.resolveConflict(target)
			if err != nil {
				return err
			}
			if !overwrite {
				return nil
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}

		return copyFile(path, target, info.Mode().Perm())
	})
}

func copyFile(src, dst string, perm fs.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

func isDir(path string) bool {
	st, err := os.Stat(path)
	return err == nil && st.IsDir()
}

func envBoolDefault(key string, def bool) bool {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return def
	}
	return b
}

type seedConflictResolver struct {
	interactive bool
	reader      *bufio.Reader
	writer      io.Writer
	strategy    seedConflictStrategy
	skipped     int
	overwritten int
}

func newSeedConflictResolver(interactive bool, in io.Reader, out io.Writer) *seedConflictResolver {
	r := &seedConflictResolver{
		interactive: interactive,
		writer:      out,
	}
	if in == nil {
		in = os.Stdin
	}
	if out == nil {
		r.writer = os.Stdout
	}
	r.reader = bufio.NewReader(in)
	return r
}

func (r *seedConflictResolver) resolveConflict(path string) (bool, error) {
	if r.strategy == seedConflictUnset {
		if !r.interactive {
			r.strategy = seedConflictKeep
		} else {
			strategy, err := r.promptConflictStrategy(path)
			if err != nil {
				return false, err
			}
			r.strategy = strategy
		}
	}

	switch r.strategy {
	case seedConflictOverwrite:
		r.overwritten++
		return true, nil
	case seedConflictKeep:
		r.skipped++
		return false, nil
	case seedConflictAbort:
		return false, fmt.Errorf("skills seeding aborted by user")
	default:
		r.skipped++
		return false, nil
	}
}

func (r *seedConflictResolver) promptConflictStrategy(path string) (seedConflictStrategy, error) {
	for {
		if _, err := fmt.Fprintf(r.writer,
			"Workbench found an existing skills file:\n  %s\nChoose conflict strategy for all skills files:\n  [o] overwrite all\n  [k] keep existing\n  [a] abort\n> ",
			path,
		); err != nil {
			return seedConflictKeep, err
		}
		choice, err := r.reader.ReadString('\n')
		if err != nil {
			return seedConflictKeep, err
		}
		switch strings.ToLower(strings.TrimSpace(choice)) {
		case "o", "overwrite":
			return seedConflictOverwrite, nil
		case "k", "keep", "skip":
			return seedConflictKeep, nil
		case "a", "abort":
			return seedConflictAbort, nil
		default:
			if _, err := fmt.Fprintln(r.writer, "Invalid choice. Enter o, k, or a."); err != nil {
				return seedConflictKeep, err
			}
		}
	}
}

func (r *seedConflictResolver) logSummary() {
	if r == nil {
		return
	}
	if r.skipped > 0 && !r.interactive {
		log.Printf("defaults seed: skipped %d existing skill files (non-interactive mode)", r.skipped)
	}
	if r.overwritten > 0 {
		log.Printf("defaults seed: overwritten %d skill files", r.overwritten)
	}
}
