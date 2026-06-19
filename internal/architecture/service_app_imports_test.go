package architecture_test

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
)

func TestServiceAppPackagesDoNotImportOtherServiceAppPackages(t *testing.T) {
	repoRoot := findRepoRoot(t)
	modulePath := readModulePath(t, filepath.Join(repoRoot, "go.mod"))
	serviceAppImport := regexp.MustCompile("^" + regexp.QuoteMeta(modulePath) + `/internal/services/([^/]+)/app(?:/.*)?$`)

	appDirs, err := filepath.Glob(filepath.Join(repoRoot, "internal", "services", "*", "app"))
	if err != nil {
		t.Fatalf("find service app packages: %v", err)
	}
	slices.Sort(appDirs)

	var violations []string
	for _, appDir := range appDirs {
		importingService := filepath.Base(filepath.Dir(appDir))
		goFiles, err := filepath.Glob(filepath.Join(appDir, "*.go"))
		if err != nil {
			t.Fatalf("find Go files in %s: %v", appDir, err)
		}
		slices.Sort(goFiles)
		for _, goFile := range goFiles {
			fileViolations := serviceAppImportViolations(t, repoRoot, goFile, importingService, serviceAppImport)
			violations = append(violations, fileViolations...)
		}
	}

	if len(violations) > 0 {
		t.Fatalf("service app packages must not import other service app packages:\n%s", strings.Join(violations, "\n"))
	}
}

func serviceAppImportViolations(t *testing.T, repoRoot, goFile, importingService string, serviceAppImport *regexp.Regexp) []string {
	t.Helper()

	fileSet := token.NewFileSet()
	parsed, err := parser.ParseFile(fileSet, goFile, nil, parser.ImportsOnly)
	if err != nil {
		t.Fatalf("parse imports from %s: %v", goFile, err)
	}

	var violations []string
	for _, imported := range parsed.Imports {
		importPath := strings.Trim(imported.Path.Value, `"`)
		matches := serviceAppImport.FindStringSubmatch(importPath)
		if len(matches) != 2 {
			continue
		}
		importedService := matches[1]
		if importedService == importingService {
			continue
		}

		position := fileSet.Position(imported.Pos())
		relFile, err := filepath.Rel(repoRoot, position.Filename)
		if err != nil {
			relFile = position.Filename
		}
		violations = append(violations, strings.Join([]string{
			"- " + filepath.ToSlash(relFile),
			":",
			positionString(position.Line),
			" imports ",
			importPath,
			" (",
			importingService,
			" app -> ",
			importedService,
			" app)",
		}, ""))
	}
	return violations
}

func positionString(line int) string {
	return strconv.Itoa(line)
}

func findRepoRoot(t *testing.T) string {
	t.Helper()

	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find repository root containing go.mod")
		}
		dir = parent
	}
}

func readModulePath(t *testing.T, goModPath string) string {
	t.Helper()

	content, err := os.ReadFile(goModPath)
	if err != nil {
		t.Fatalf("read %s: %v", goModPath, err)
	}
	for _, line := range strings.Split(string(content), "\n") {
		if modulePath, ok := strings.CutPrefix(strings.TrimSpace(line), "module "); ok {
			modulePath = strings.TrimSpace(modulePath)
			if modulePath == "" {
				t.Fatalf("%s has an empty module path", goModPath)
			}
			return modulePath
		}
	}
	t.Fatalf("%s does not declare a module path", goModPath)
	return ""
}
