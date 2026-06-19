package architecture_test

import (
	"fmt"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
)

func TestServiceAppPackagesDoNotImportOtherServiceAppPackages(t *testing.T) {
	assertServiceImportBoundary(t, serviceImportBoundary{
		name:          "service app packages must not import other service app packages",
		importerLayer: "app",
		importedLayer: "app",
	})
}

func TestServiceDomainPackagesDoNotImportUnapprovedForeignServiceDomainPackages(t *testing.T) {
	assertServiceImportBoundary(t, serviceImportBoundary{
		name:          "service domain packages must not import unapproved foreign service domain packages",
		importerLayer: "domain",
		importedLayer: "domain",
		allowedForeignImports: map[importAllowance]string{
			{file: "internal/services/auth/apikey/key.go", importPath: "github.com/tinoosan/agen8/internal/services/user/domain"}:                           "debt: auth API key model still stores user domain ids",
			{file: "internal/services/auth/apikey/repository.go", importPath: "github.com/tinoosan/agen8/internal/services/user/domain"}:                    "debt: auth API key repository contract still uses user domain ids",
			{file: "internal/services/auth/linktoken/key.go", importPath: "github.com/tinoosan/agen8/internal/services/user/domain"}:                        "debt: auth link token model still stores user domain ids",
			{file: "internal/services/auth/password/credential.go", importPath: "github.com/tinoosan/agen8/internal/services/user/domain"}:                  "debt: auth password credential model still stores user domain ids",
			{file: "internal/services/auth/password/repository.go", importPath: "github.com/tinoosan/agen8/internal/services/user/domain"}:                  "debt: auth password repository contract still uses user domain ids",
			{file: "internal/services/auth/session/session.go", importPath: "github.com/tinoosan/agen8/internal/services/user/domain"}:                      "debt: auth session model still stores user domain ids",
			{file: "internal/services/task/domain/new_task.go", importPath: "github.com/tinoosan/agen8/internal/services/project/domain/member"}:            "debt: task domain still stores project member ids as member.MemberID",
			{file: "internal/services/task/domain/task.go", importPath: "github.com/tinoosan/agen8/internal/services/project/domain/member"}:                "debt: task aggregate still stores project member ids as member.MemberID",
			{file: "internal/services/task/domain/task_aggregate.go", importPath: "github.com/tinoosan/agen8/internal/services/project/domain/member"}:      "debt: task aggregate helpers still store project member ids as member.MemberID",
			{file: "internal/services/task/domain/task_aggregate_test.go", importPath: "github.com/tinoosan/agen8/internal/services/project/domain/member"}: "debt: tests cover the current member id coupling until the domain owns a neutral id",
			{file: "internal/services/task/domain/types.go", importPath: "github.com/tinoosan/agen8/internal/services/project/domain/member"}:               "debt: task domain request types still use project member ids",
		},
	})
}

func TestServiceAppPackagesDoNotImportUnapprovedForeignServiceDomainPackages(t *testing.T) {
	assertServiceImportBoundary(t, serviceImportBoundary{
		name:          "service app packages must not import unapproved foreign service domain packages",
		importerLayer: "app",
		importedLayer: "domain",
		allowedForeignImports: map[importAllowance]string{
			{file: "internal/services/auth/app/service.go", importPath: "github.com/tinoosan/agen8/internal/services/user/domain"}:                 "debt: auth app still accepts user domain ids while identity ports are split out",
			{file: "internal/services/auth/app/service_test.go", importPath: "github.com/tinoosan/agen8/internal/services/user/domain"}:            "debt: auth app tests cover the current user domain id coupling",
			{file: "internal/services/decision/app/service.go", importPath: "github.com/tinoosan/agen8/internal/services/graph/domain"}:            "debt: decision app still writes graph refs using graph domain types",
			{file: "internal/services/decision/app/service.go", importPath: "github.com/tinoosan/agen8/internal/services/project/domain/member"}:   "debt: decision app still records author ids as project member ids",
			{file: "internal/services/file/app/ports.go", importPath: "github.com/tinoosan/agen8/internal/services/project/domain/project"}:        "debt: file app port still exposes project domain ids",
			{file: "internal/services/file/app/service.go", importPath: "github.com/tinoosan/agen8/internal/services/project/domain/project"}:      "debt: file app still receives project domain ids",
			{file: "internal/services/file/app/service_test.go", importPath: "github.com/tinoosan/agen8/internal/services/project/domain/project"}: "debt: tests cover the current project id coupling",
			{file: "internal/services/mission/app/ports.go", importPath: "github.com/tinoosan/agen8/internal/services/task/domain"}:                "debt: mission app still exposes task refs through task domain ids",
			{file: "internal/services/mission/app/service.go", importPath: "github.com/tinoosan/agen8/internal/services/task/domain"}:              "debt: mission app still exposes task refs through task domain ids",
			{file: "internal/services/mission/app/service_test.go", importPath: "github.com/tinoosan/agen8/internal/services/task/domain"}:         "debt: mission app tests cover the current task domain coupling",
			{file: "internal/services/task/app/service.go", importPath: "github.com/tinoosan/agen8/internal/services/project/domain/member"}:       "debt: task app still uses project member ids for assignment",
			{file: "internal/services/task/app/service.go", importPath: "github.com/tinoosan/agen8/internal/services/project/domain/project"}:      "debt: task app still scopes tasks with project domain ids",
			{file: "internal/services/task/app/service_test.go", importPath: "github.com/tinoosan/agen8/internal/services/project/domain/member"}:  "debt: task app tests cover the current member id coupling",
		},
	})
}

func TestServiceInfraPackagesDoNotImportForeignServiceAppPackages(t *testing.T) {
	assertServiceImportBoundary(t, serviceImportBoundary{
		name:          "service infra packages must not import foreign service app packages",
		importerLayer: "infra",
		importedLayer: "app",
	})
}

type serviceImportBoundary struct {
	name                  string
	importerLayer         string
	importedLayer         string
	allowedForeignImports map[importAllowance]string
}

type importAllowance struct {
	file       string
	importPath string
}

func assertServiceImportBoundary(t *testing.T, boundary serviceImportBoundary) {
	t.Helper()

	repoRoot := findRepoRoot(t)
	modulePath := readModulePath(t, filepath.Join(repoRoot, "go.mod"))
	importPattern := regexp.MustCompile("^" + regexp.QuoteMeta(modulePath) + `/internal/services/([^/]+)/` + regexp.QuoteMeta(boundary.importedLayer) + `(?:/.*)?$`)

	importerDirs, err := filepath.Glob(filepath.Join(repoRoot, "internal", "services", "*", boundary.importerLayer))
	if err != nil {
		t.Fatalf("find service %s packages: %v", boundary.importerLayer, err)
	}
	importerRoots := serviceImporterRoots(t, repoRoot, boundary.importerLayer, importerDirs)

	usedAllowances := make(map[importAllowance]struct{})
	var violations []string
	for _, importerRoot := range importerRoots {
		goFiles := goFilesUnder(t, importerRoot.path)
		for _, goFile := range goFiles {
			fileViolations := serviceImportViolations(t, repoRoot, goFile, importerRoot.service, boundary, importPattern, usedAllowances)
			violations = append(violations, fileViolations...)
		}
	}
	violations = append(violations, unusedAllowances(boundary, usedAllowances)...)

	if len(violations) > 0 {
		t.Fatalf("%s:\n%s", boundary.name, strings.Join(violations, "\n"))
	}
}

type serviceImporterRoot struct {
	path    string
	service string
}

func serviceImporterRoots(t *testing.T, repoRoot, importerLayer string, layerDirs []string) []serviceImporterRoot {
	t.Helper()

	var roots []serviceImporterRoot
	for _, layerDir := range layerDirs {
		roots = append(roots, serviceImporterRoot{
			path:    layerDir,
			service: filepath.Base(filepath.Dir(layerDir)),
		})
	}
	if importerLayer == "domain" {
		roots = append(roots, serviceDomainModelRoots(t, repoRoot)...)
	}

	slices.SortFunc(roots, func(a, b serviceImporterRoot) int {
		return strings.Compare(a.path, b.path)
	})
	return roots
}

func serviceDomainModelRoots(t *testing.T, repoRoot string) []serviceImporterRoot {
	t.Helper()

	serviceDirs, err := filepath.Glob(filepath.Join(repoRoot, "internal", "services", "*"))
	if err != nil {
		t.Fatalf("find service directories: %v", err)
	}
	slices.Sort(serviceDirs)

	var roots []serviceImporterRoot
	for _, serviceDir := range serviceDirs {
		service := filepath.Base(serviceDir)
		entries, err := os.ReadDir(serviceDir)
		if err != nil {
			t.Fatalf("read service directory %s: %v", serviceDir, err)
		}
		for _, entry := range entries {
			if !entry.IsDir() || !isDomainModelPackage(entry.Name()) {
				continue
			}
			roots = append(roots, serviceImporterRoot{
				path:    filepath.Join(serviceDir, entry.Name()),
				service: service,
			})
		}
	}
	return roots
}

func isDomainModelPackage(name string) bool {
	switch name {
	case "app", "domain", "infra", "rpc":
		return false
	default:
		return true
	}
}

func goFilesUnder(t *testing.T, root string) []string {
	t.Helper()

	var goFiles []string
	if err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		if strings.HasSuffix(entry.Name(), ".go") {
			goFiles = append(goFiles, path)
		}
		return nil
	}); err != nil {
		t.Fatalf("find Go files in %s: %v", root, err)
	}
	slices.Sort(goFiles)
	return goFiles
}

func serviceImportViolations(t *testing.T, repoRoot, goFile, importingService string, boundary serviceImportBoundary, importPattern *regexp.Regexp, usedAllowances map[importAllowance]struct{}) []string {
	t.Helper()

	fileSet := token.NewFileSet()
	parsed, err := parser.ParseFile(fileSet, goFile, nil, parser.ImportsOnly)
	if err != nil {
		t.Fatalf("parse imports from %s: %v", goFile, err)
	}

	var violations []string
	relFile := slashRelPath(t, repoRoot, goFile)
	for _, imported := range parsed.Imports {
		importPath := strings.Trim(imported.Path.Value, `"`)
		matches := importPattern.FindStringSubmatch(importPath)
		if len(matches) != 2 {
			continue
		}
		importedService := matches[1]
		if importedService == importingService {
			continue
		}

		position := fileSet.Position(imported.Pos())
		allowance := importAllowance{file: relFile, importPath: importPath}
		if _, ok := boundary.allowedForeignImports[allowance]; ok {
			usedAllowances[allowance] = struct{}{}
			continue
		}
		violations = append(violations, fmt.Sprintf(
			"- %s:%s imports %s (%s %s -> %s %s)",
			relFile,
			positionString(position.Line),
			importPath,
			importingService,
			boundary.importerLayer,
			importedService,
			boundary.importedLayer,
		))
	}
	return violations
}

func unusedAllowances(boundary serviceImportBoundary, usedAllowances map[importAllowance]struct{}) []string {
	var unused []string
	for allowance, reason := range boundary.allowedForeignImports {
		if _, ok := usedAllowances[allowance]; ok {
			continue
		}
		unused = append(unused, fmt.Sprintf(
			"- stale allowlist entry for %s importing %s (%s -> %s): %s",
			allowance.file,
			allowance.importPath,
			boundary.importerLayer,
			boundary.importedLayer,
			reason,
		))
	}
	slices.Sort(unused)
	return unused
}

func slashRelPath(t *testing.T, root, path string) string {
	t.Helper()

	relPath, err := filepath.Rel(root, path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(relPath)
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
