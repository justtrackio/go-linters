//go:build integration

package linters

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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
