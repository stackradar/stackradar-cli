package upload

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestDiscoverFindsSupportedDependencyFiles(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "package-lock.json", "{}")
	writeTestFile(t, root, "package.json", "{}")
	writeTestFile(t, root, "apps/api/composer.lock", "{}")
	writeTestFile(t, root, "apps/api/composer.json", "{}")
	writeTestFile(t, root, "services/frontend/pnpm-lock.yaml", "lockfileVersion: '9'\n")
	writeTestFile(t, root, "services/worker/requirements.txt", "requests==2.31.0\n")
	writeTestFile(t, root, "services/worker/pyproject.toml", "[project]\n")
	writeTestFile(t, root, "README.md", "# ignored\n")

	discovery, err := Discover(DiscoverOptions{Root: root})

	if err != nil {
		t.Fatalf("expected discovery to succeed, got %v", err)
	}

	got := pathsByEcosystem(discovery.Files)
	want := map[string]string{
		"apps/api/composer.json":           "composer",
		"apps/api/composer.lock":           "composer",
		"package-lock.json":                "npm",
		"package.json":                     "npm",
		"services/frontend/pnpm-lock.yaml": "npm",
		"services/worker/pyproject.toml":   "python",
		"services/worker/requirements.txt": "python",
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("discovered files = %#v, want %#v", got, want)
	}
}

func TestDiscoverSkipsIgnoredDirectoriesAndSymlinks(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "app/package-lock.json", "{}")
	writeTestFile(t, root, "node_modules/package-lock.json", "{}")
	writeTestFile(t, root, "vendor/composer.lock", "{}")
	writeTestFile(t, root, "tests/package.json", "{}")
	writeTestFile(t, root, ".git/package.json", "{}")
	writeTestFile(t, root, "real/package.json", "{}")

	if err := os.Symlink(filepath.Join(root, "real"), filepath.Join(root, "linked-real")); err != nil {
		t.Skipf("skipping symlink assertion: %v", err)
	}

	discovery, err := Discover(DiscoverOptions{Root: root})

	if err != nil {
		t.Fatalf("expected discovery to succeed, got %v", err)
	}

	got := paths(discovery.Files)
	want := []string{"app/package-lock.json", "real/package.json"}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("discovered paths = %#v, want %#v", got, want)
	}

	if !skippedDirectory(discovery.SkippedDirectories, "node_modules", "ignored directory") {
		t.Fatalf("expected node_modules to be reported as skipped, got %#v", discovery.SkippedDirectories)
	}

	if skippedDirectory(discovery.SkippedDirectories, "linked-real", "ignored directory") {
		t.Fatalf("expected linked-real to be skipped as symlink, got %#v", discovery.SkippedDirectories)
	}
}

func TestDiscoverHonorsExcludePatterns(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "app/package-lock.json", "{}")
	writeTestFile(t, root, "docs/package.json", "{}")
	writeTestFile(t, root, "services/api/composer.lock", "{}")
	writeTestFile(t, root, "services/web/package.json", "{}")

	discovery, err := Discover(DiscoverOptions{
		Root: root,
		Excludes: []string{
			"package-lock.json",
			"docs/**",
			"services/api/*",
		},
	})

	if err != nil {
		t.Fatalf("expected discovery to succeed, got %v", err)
	}

	got := paths(discovery.Files)
	want := []string{"services/web/package.json"}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("discovered paths = %#v, want %#v", got, want)
	}
}

func TestDiscoverHonorsRootGitignorePatterns(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, ".gitignore", "/tmp/\nignored-anywhere/\n")
	writeTestFile(t, root, "package-lock.json", "{}")
	writeTestFile(t, root, "tmp/pptx/package-lock.json", "{}")
	writeTestFile(t, root, "nested/ignored-anywhere/package.json", "{}")

	discovery, err := Discover(DiscoverOptions{Root: root})

	if err != nil {
		t.Fatalf("expected discovery to succeed, got %v", err)
	}

	got := paths(discovery.Files)
	want := []string{"package-lock.json"}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("discovered paths = %#v, want %#v", got, want)
	}

	if !skippedDirectory(discovery.SkippedDirectories, "tmp", "gitignored") {
		t.Fatalf("expected tmp to be reported as gitignored, got %#v", discovery.SkippedDirectories)
	}

	if !skippedDirectory(discovery.SkippedDirectories, "nested/ignored-anywhere", "gitignored") {
		t.Fatalf("expected nested ignored directory to be reported as gitignored, got %#v", discovery.SkippedDirectories)
	}
}

func TestDiscoverReturnsNoDependencyFilesError(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "README.md", "# ignored\n")

	_, err := Discover(DiscoverOptions{Root: root})

	if !errors.Is(err, ErrNoDependencyFiles) {
		t.Fatalf("expected ErrNoDependencyFiles, got %v", err)
	}
}

func pathsByEcosystem(files []File) map[string]string {
	result := make(map[string]string, len(files))

	for _, file := range files {
		result[file.Path] = file.Ecosystem
	}

	return result
}

func paths(files []File) []string {
	result := make([]string, 0, len(files))

	for _, file := range files {
		result = append(result, file.Path)
	}

	return result
}

func skippedDirectory(directories []SkippedDirectory, path string, reason string) bool {
	for _, directory := range directories {
		if directory.Path == path && directory.Reason == reason {
			return true
		}
	}

	return false
}

func writeTestFile(t *testing.T, root string, relativePath string, contents string) {
	t.Helper()

	fullPath := filepath.Join(root, filepath.FromSlash(relativePath))

	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		t.Fatalf("create parent directories for %s: %v", relativePath, err)
	}

	if err := os.WriteFile(fullPath, []byte(contents), 0o644); err != nil {
		t.Fatalf("write %s: %v", relativePath, err)
	}
}
