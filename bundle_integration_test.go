//go:build integration

package linters

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const bundleCommandTimeout = 3 * time.Minute

func TestBundle(t *testing.T) {
	binary := os.Getenv("GOLANGCI_LINT_BINARY")
	require.NotEmpty(t, binary, "GOLANGCI_LINT_BINARY must name the compiled golangci-lint binary")
	require.True(t, filepath.IsAbs(binary), "GOLANGCI_LINT_BINARY must be an absolute path: %q", binary)
	binaryInfo, err := os.Stat(binary)
	require.NoError(t, err, "stat GOLANGCI_LINT_BINARY %q", binary)
	require.True(t, binaryInfo.Mode().IsRegular(), "GOLANGCI_LINT_BINARY is not a regular file: %q", binary)
	require.NotZero(t, binaryInfo.Mode().Perm()&0o111, "GOLANGCI_LINT_BINARY is not executable: %q", binary)

	_, testFile, _, ok := runtime.Caller(0)
	require.True(t, ok, "unable to locate bundle integration test source")
	repoRoot := filepath.Dir(testFile)
	fixtureRoot := filepath.Join(repoRoot, "testdata", "src", "testlintdata")
	goldens := collectBundleGoldens(t, fixtureRoot)
	require.NotEmpty(t, goldens, "expected existing analyzer golden fixtures")
	fixablePackages := goldenFixturePackages(goldens)
	fixablePackages = append(fixablePackages, "./testlintdata/inlineguard")

	workspace := t.TempDir()
	gopath := filepath.Join(workspace, "gopath")
	sourceRoot := filepath.Join(gopath, "src")
	fixtureCopy := filepath.Join(sourceRoot, "testlintdata")
	require.NoError(t, os.MkdirAll(filepath.Join(workspace, "home"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(workspace, "tmp"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(workspace, "gocache"), 0o755))
	require.NoError(t, copyGoFixtures(fixtureRoot, fixtureCopy))
	env := bundleTestEnv(workspace, gopath)

	allFixtures := runBundleCommand(t, binary, lintArgs(repoRoot, false, []string{"./testlintdata/..."}), sourceRoot, env)
	require.Equal(t, 1, allFixtures.exitCode, "expected analyzer findings from the existing fixtures; stdout:\n%s\nstderr:\n%s", allFixtures.stdout, allFixtures.stderr)
	issues := decodeBundleIssues(t, allFixtures)

	for _, issue := range issues {
		require.Equal(t, "justtrack", issue.FromLinter, "unexpected linter for diagnostic at %s:%d", issue.Pos.Filename, issue.Pos.Line)
	}

	for _, fixturePackage := range fixablePackages {
		require.True(t, hasBundleIssueInPackage(issues, fixturePackage), "expected a diagnostic in fixture package %s", fixturePackage)
	}

	require.True(t, hasBundleIssueAt(issues, "testlintdata/noanonstruct/structs.go", 12), "expected the existing non-fixable noanonstruct diagnostic")
	require.True(t, hasBundleIssueAt(issues, "testlintdata/intest/helpers.go", 3), "expected the test-only production function diagnostic")
	require.True(t, hasBundleIssueAt(issues, "testlintdata/declfilecheck/types.go", 3), "expected the private declaration ownership diagnostic")
	require.True(t, hasBundleIssueAt(issues, "testlintdata/constlocal/constants.go", 3), "expected the function-owned constant diagnostic")
	require.True(t, hasBundleIssueAt(issues, "testlintdata/constlocal/local.go", 4), "expected the single-use local constant diagnostic")

	fix := runBundleCommand(t, binary, lintArgs(repoRoot, true, fixablePackages), sourceRoot, env)
	require.Equal(t, 1, fix.exitCode, "expected only withheld manual findings after --fix; stdout:\n%s\nstderr:\n%s", fix.stdout, fix.stderr)
	assertInlineGuardManualIssues(t, decodeBundleIssues(t, fix))

	buildArgs := append([]string{"build"}, fixablePackages...)
	build := runBundleCommand(t, "go", buildArgs, sourceRoot, env)
	require.Equal(t, 0, build.exitCode, "expected autofixed fixture packages to compile; stdout:\n%s\nstderr:\n%s", build.stdout, build.stderr)

	clean := runBundleCommand(t, binary, lintArgs(repoRoot, false, fixablePackages), sourceRoot, env)
	require.Equal(t, 1, clean.exitCode, "expected only withheld manual findings; stdout:\n%s\nstderr:\n%s", clean.stdout, clean.stderr)
	assertInlineGuardManualIssues(t, decodeBundleIssues(t, clean))
}

func TestBundleSelectiveInlineRules(t *testing.T) {
	binary := os.Getenv("GOLANGCI_LINT_BINARY")
	require.NotEmpty(t, binary)
	require.True(t, filepath.IsAbs(binary))
	fixtures := append(selectiveInlineFixtures(), struct{ name, source string }{"import-alias-collision", selectiveInlineImportSource})

	for _, fixture := range fixtures {
		for _, selection := range [][]string{{"iferrinline"}, {"okinline"}, {"iferrinline", "okinline"}, {"okinline", "iferrinline"}, {"justtrack"}} {
			t.Run(fixture.name+"/"+strings.Join(selection, "+"), func(t *testing.T) {
				workspace, sourceRoot, filename, env := inlineBundleWorkspace(t, fixture.source)
				if fixture.name == "import-alias-collision" {
					writeInlineSharedImportDependencies(t, sourceRoot)
				}

				config := writeInlineBundleConfig(t, workspace, selection)
				before := runBundleCommand(t, binary, inlineBundleArgs(config, false), sourceRoot, env)
				require.Equal(t, 1, before.exitCode, "stdout:\n%s\nstderr:\n%s", before.stdout, before.stderr)
				issues := decodeBundleIssues(t, before)
				expectedIssues := len(selection)

				if selection[0] == "justtrack" {
					expectedIssues = 2
				}

				require.Len(t, issues, expectedIssues)
				for _, issue := range issues {
					require.Contains(t, selection, issue.FromLinter)
				}

				passes := 1
				if len(selection) == 2 {
					passes = 2
				}

				runInlineBundleFixes(t, binary, config, sourceRoot, env, passes)
				contents, err := os.ReadFile(filename)
				require.NoError(t, err)

				aggregate := selection[0] == "justtrack"
				if aggregate {
					require.Equal(t, fixture.source, string(contents))
				}

				assertInlineBundleInitializers(t, contents, slices.Contains(selection, "iferrinline"), slices.Contains(selection, "okinline"))
				after := runBundleCommand(t, binary, inlineBundleArgs(config, false), sourceRoot, env)

				if aggregate {
					require.Equal(t, 1, after.exitCode, "stdout:\n%s\nstderr:\n%s", after.stdout, after.stderr)
					require.Len(t, decodeBundleIssues(t, after), 2)
				} else {
					require.Equal(t, 0, after.exitCode, "stdout:\n%s\nstderr:\n%s", after.stdout, after.stderr)
					require.Empty(t, decodeBundleIssues(t, after))
				}
			})
		}
	}
}

func TestBundleInlineSelectionCacheIsolation(t *testing.T) {
	binary := os.Getenv("GOLANGCI_LINT_BINARY")
	require.NotEmpty(t, binary)
	require.True(t, filepath.IsAbs(binary))
	standalone := []string{"noanonstruct", "iferrinline", "funcmethod", "okinline", "methodfilecheck", "multilineblock", "recvaftertype"}

	for _, direction := range []struct {
		name         string
		warm, target []string
	}{
		{"aggregate-to-standalone", []string{"justtrack"}, standalone},
		{"standalone-to-aggregate", standalone, []string{"justtrack"}},
	} {
		t.Run(direction.name, func(t *testing.T) {
			workspace, sourceRoot, filename, env := inlineBundleWorkspace(t, selectiveInlineSiblingSource)
			config := writeInlineBundleConfig(t, workspace, direction.warm)
			warm := runBundleCommand(t, binary, inlineBundleArgs(config, false), sourceRoot, env)
			require.Equal(t, 1, warm.exitCode, "stdout:\n%s\nstderr:\n%s", warm.stdout, warm.stderr)
			require.Len(t, decodeBundleIssues(t, warm), 2)
			// Keep paths, source, settings and cache identical; change only enable.
			config = writeInlineBundleConfig(t, workspace, direction.target)
			selected := runBundleCommand(t, binary, inlineBundleArgs(config, false), sourceRoot, env)
			require.Equal(t, 1, selected.exitCode, "stdout:\n%s\nstderr:\n%s", selected.stdout, selected.stderr)
			issues := decodeBundleIssues(t, selected)
			require.Len(t, issues, 2)

			for _, issue := range issues {
				require.Contains(t, direction.target, issue.FromLinter)
			}

			aggregate := direction.target[0] == "justtrack"
			passes := 2

			if aggregate {
				passes = 1
			}

			runInlineBundleFixes(t, binary, config, sourceRoot, env, passes)
			contents, err := os.ReadFile(filename)
			require.NoError(t, err)

			if aggregate {
				require.Equal(t, selectiveInlineSiblingSource, string(contents))
			}

			assertInlineBundleInitializers(t, contents, !aggregate, !aggregate)
		})
	}
}

func inlineBundleWorkspace(t *testing.T, source string) (workspace, sourceRoot, filename string, env []string) {
	t.Helper()
	workspace = t.TempDir()
	gopath := filepath.Join(workspace, "gopath")
	sourceRoot = filepath.Join(gopath, "src")
	filename = filepath.Join(sourceRoot, "fixture", "fixture.go")

	for _, directory := range []string{filepath.Dir(filename), filepath.Join(workspace, "home"), filepath.Join(workspace, "tmp"), filepath.Join(workspace, "gocache"), filepath.Join(workspace, "lintcache")} {
		require.NoError(t, os.MkdirAll(directory, 0o755))
	}

	require.NoError(t, os.WriteFile(filename, []byte(source), 0o644))
	env = append(bundleTestEnv(workspace, gopath), "GOLANGCI_LINT_CACHE="+filepath.Join(workspace, "lintcache"))
	return workspace, sourceRoot, filename, env
}

func writeInlineBundleConfig(t *testing.T, workspace string, selection []string) string {
	t.Helper()
	config := "version: \"2\"\nlinters:\n  default: none\n  enable:\n"

	for _, plugin := range selection {
		config += "    - " + plugin + "\n"
	}

	config += "  settings:\n    custom:\n"
	for _, plugin := range []string{"justtrack", "noanonstruct", "iferrinline", "funcmethod", "okinline", "methodfilecheck", "multilineblock", "recvaftertype"} {
		config += "      " + plugin + ":\n        type: module\n        description: inline regression\n        original-url: github.com/justtrackio/go-linters\n"
	}

	path := filepath.Join(workspace, "golangci.yml")
	require.NoError(t, os.WriteFile(path, []byte(config), 0o644))
	return path
}

func inlineBundleArgs(config string, fix bool) []string {
	args := lintArgs("", fix, []string{"./fixture"})
	args[1] = "--config=" + config
	return args
}

func runInlineBundleFixes(t *testing.T, binary, config, sourceRoot string, env []string, passes int) {
	t.Helper()
	// Each whole-file linter edit set is checked before the next pass can expose
	// more work. Competing shared owners must never produce an invalid union.
	for range passes {
		fixed := runBundleCommand(t, binary, inlineBundleArgs(config, true), sourceRoot, env)
		require.Contains(t, []int{0, 1}, fixed.exitCode, "stdout:\n%s\nstderr:\n%s", fixed.stdout, fixed.stderr)
		build := runBundleCommand(t, "go", []string{"build", "./fixture"}, sourceRoot, env)
		require.Equal(t, 0, build.exitCode, "stdout:\n%s\nstderr:\n%s", build.stdout, build.stderr)
	}
}

func assertInlineBundleInitializers(t *testing.T, source []byte, errorInlined, booleanInlined bool) {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), "fixture.go", source, 0)
	require.NoError(t, err)
	errorChecks, booleanChecks := 0, 0
	ast.Inspect(file, func(node ast.Node) bool {
		var ok bool
		var check *ast.IfStmt

		if check, ok = node.(*ast.IfStmt); !ok {
			return true
		}

		switch condition := check.Cond.(type) {
		case *ast.BinaryExpr:
			if name, ok := condition.X.(*ast.Ident); ok && name.Name == "err" {
				errorChecks++
				require.Equal(t, errorInlined, check.Init != nil)
			}
		case *ast.UnaryExpr:
			if name, ok := condition.X.(*ast.Ident); ok && name.Name == "ok" {
				booleanChecks++
				require.Equal(t, booleanInlined, check.Init != nil)
			}
		}

		return true
	})
	require.Equal(t, 1, errorChecks)
	require.Equal(t, 1, booleanChecks)
}

func assertInlineGuardManualIssues(t *testing.T, issues []bundleLintIssue) {
	t.Helper()
	expected := map[string]int{"iferrinline": 2, "okinline": 2}
	require.Len(t, issues, 5)
	privateWithoutInterface := false

	for _, issue := range issues {
		require.Equal(t, "justtrack", issue.FromLinter)

		filename := filepath.ToSlash(issue.Pos.Filename)
		if strings.HasSuffix(filename, "testlintdata/iferrinline/iferrinline.go") {
			// Upstream already omitted this fix: the private concrete result
			// has no accessible interface through which to declare it.
			require.False(t, privateWithoutInterface)
			require.True(t, strings.HasPrefix(issue.Text, "iferrinline:"))
			privateWithoutInterface = true
		} else {
			require.True(t, strings.HasSuffix(filename, "testlintdata/inlineguard/inlineguard.go"))
			rule, _, found := strings.Cut(issue.Text, ":")
			require.True(t, found)
			require.Positive(t, expected[rule], "unexpected manual finding from %s", rule)
			expected[rule]--
		}

		require.Contains(t, issue.Text, "manual fix required")
	}

	require.True(t, privateWithoutInterface)
	for _, remaining := range expected {
		require.Zero(t, remaining)
	}
}

type bundleGolden struct {
	relativePath string
	packagePath  string
}

type bundleLintIssue struct {
	FromLinter string `json:"FromLinter"`
	Text       string `json:"Text"`
	Pos        struct {
		Filename string `json:"Filename"`
		Line     int    `json:"Line"`
	} `json:"Pos"`
}

type bundleLintReport struct {
	Issues []bundleLintIssue `json:"Issues"`
}

type bundleCommandResult struct {
	exitCode int
	stdout   string
	stderr   string
}

func collectBundleGoldens(t *testing.T, fixtureRoot string) []bundleGolden {
	t.Helper()
	var goldens []bundleGolden
	err := filepath.WalkDir(fixtureRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		var err error
		var relativePath string

		if walkErr != nil {
			return walkErr
		}

		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go.golden") {
			return nil
		}

		if relativePath, err = filepath.Rel(fixtureRoot, strings.TrimSuffix(path, ".golden")); err != nil {
			return err
		}

		goldens = append(goldens, bundleGolden{
			relativePath: relativePath,
			packagePath:  "./" + filepath.ToSlash(filepath.Join("testlintdata", filepath.Dir(relativePath))),
		})

		return nil
	})
	require.NoError(t, err, "discover existing analyzer goldens")
	sort.Slice(goldens, func(i, j int) bool { return goldens[i].relativePath < goldens[j].relativePath })

	return goldens
}

func goldenFixturePackages(goldens []bundleGolden) []string {
	seen := make(map[string]struct{}, len(goldens))
	packages := make([]string, 0, len(goldens))

	for _, golden := range goldens {
		if _, exists := seen[golden.packagePath]; exists {
			continue
		}

		seen[golden.packagePath] = struct{}{}
		packages = append(packages, golden.packagePath)
	}

	sort.Strings(packages)

	return packages
}

func copyGoFixtures(sourceRoot, destinationRoot string) error {
	return filepath.WalkDir(sourceRoot, func(sourcePath string, entry fs.DirEntry, walkErr error) error {
		var err error
		var relativePath string
		var contents []byte

		if walkErr != nil {
			return walkErr
		}

		if relativePath, err = filepath.Rel(sourceRoot, sourcePath); err != nil {
			return err
		}

		destinationPath := filepath.Join(destinationRoot, relativePath)
		if entry.IsDir() {
			return os.MkdirAll(destinationPath, 0o755)
		}

		if !strings.HasSuffix(entry.Name(), ".go") {
			return nil
		}

		if contents, err = os.ReadFile(sourcePath); err != nil {
			return err
		}

		return os.WriteFile(destinationPath, contents, 0o644)
	})
}

func bundleTestEnv(workspace, gopath string) []string {
	return []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + filepath.Join(workspace, "home"),
		"TMPDIR=" + filepath.Join(workspace, "tmp"),
		"GOPATH=" + gopath,
		"GOCACHE=" + filepath.Join(workspace, "gocache"),
		"GO111MODULE=off",
		"GOENV=off",
	}
}

func lintArgs(repoRoot string, fix bool, packages []string) []string {
	args := []string{
		"run",
		"--config=" + filepath.Join(repoRoot, ".golangci.yml"),
		"--output.json.path=stdout",
		"--output.text.path=" + os.DevNull,
		"--show-stats=false",
		"--max-same-issues=0",
		"--max-issues-per-linter=0",
		"--uniq-by-line=false",
	}

	if fix {
		args = append(args, "--fix")
	}

	return append(args, packages...)
}

func runBundleCommand(t *testing.T, executable string, args []string, workdir string, env []string) bundleCommandResult {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), bundleCommandTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, executable, args...)
	cmd.Dir = workdir
	cmd.Env = env
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()

	if ctx.Err() != nil {
		require.NoError(t, ctx.Err(), "command timed out: %s %s", executable, strings.Join(args, " "))
	}

	if err == nil {
		return bundleCommandResult{stdout: stdout.String(), stderr: stderr.String()}
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return bundleCommandResult{exitCode: exitErr.ExitCode(), stdout: stdout.String(), stderr: stderr.String()}
	}

	require.NoError(t, err, "run command %s %s", executable, strings.Join(args, " "))

	return bundleCommandResult{exitCode: -1, stdout: stdout.String(), stderr: stderr.String()}
}

func decodeBundleIssues(t *testing.T, result bundleCommandResult) []bundleLintIssue {
	t.Helper()
	var report bundleLintReport
	require.NoError(t, json.Unmarshal([]byte(result.stdout), &report), "decode golangci-lint JSON output; stderr:\n%s\nstdout:\n%s", result.stderr, result.stdout)

	return report.Issues
}

func decodeBundleIssuesIfPresent(t *testing.T, result bundleCommandResult) []bundleLintIssue {
	t.Helper()
	if strings.TrimSpace(result.stdout) == "" {
		return nil
	}

	return decodeBundleIssues(t, result)
}

func hasBundleIssueInPackage(issues []bundleLintIssue, packagePath string) bool {
	packageSuffix := strings.TrimPrefix(filepath.ToSlash(packagePath), "./") + "/"
	for _, issue := range issues {
		filename := filepath.ToSlash(issue.Pos.Filename)
		if strings.Contains(filename, packageSuffix) {
			return true
		}
	}

	return false
}

func hasBundleIssueAt(issues []bundleLintIssue, filenameSuffix string, line int) bool {
	for _, issue := range issues {
		if filepath.ToSlash(issue.Pos.Filename) == filenameSuffix || strings.HasSuffix(filepath.ToSlash(issue.Pos.Filename), "/"+filenameSuffix) {
			if issue.Pos.Line == line && strings.HasPrefix(issue.FromLinter, "justtrack") {
				return true
			}
		}
	}

	return false
}
