package upload

import (
	"archive/zip"
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestCommitBundleUsesGitBlobsIncludingTrackedIgnoredFiles(t *testing.T) {
	root := t.TempDir()
	runGit(t, root, "init")
	runGit(t, root, "config", "user.name", "Tests")
	runGit(t, root, "config", "user.email", "tests@example.test")
	writeTestFile(t, root, "package.json", "{}")
	writeTestFile(t, root, "package-lock.json", "committed lock")
	writeTestFile(t, root, "packages/a/package.json", "workspace")
	writeTestFile(t, root, "pnpm-workspace.yaml", "packages: [packages/*]")
	writeTestFile(t, root, "requirements/base.in", "example==1.0")
	runGit(t, root, "add", ".")
	runGit(t, root, "commit", "-m", "Inputs")
	writeTestFile(t, root, ".gitignore", "package-lock.json\nyarn.lock\n")
	runGit(t, root, "add", ".gitignore")
	runGit(t, root, "commit", "-m", "Ignore tracked lock")
	sha := *gitOutput(root, "rev-parse", "HEAD")
	writeTestFile(t, root, "package-lock.json", "modified locally")
	writeTestFile(t, root, "yarn.lock", "ignored untracked")
	writeTestFile(t, root, "composer.lock", "untracked")
	discovery, err := DiscoverCommit(root, sha)
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := BuildBundleWithOptions(BuildBundleOptions{Root: root, Commit: sha, Files: discovery.Files})
	if err != nil {
		t.Fatal(err)
	}
	if len(bundle.Files) != 5 || !bundle.Manifest.Collection.Complete || *bundle.Manifest.Git.Dirty {
		t.Fatalf("incomplete committed evidence: %+v", bundle.Manifest)
	}
	reader, err := zip.NewReader(bytes.NewReader(bundle.Bytes), int64(len(bundle.Bytes)))
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range reader.File {
		if file.Name == "package-lock.json" {
			contents, err := readZipFile(file)
			if err != nil || string(contents) != "committed lock" {
				t.Fatalf("did not read committed blob: %q, %v", contents, err)
			}
		}
	}
}

func TestCommittedSymlinkReportsIncompleteCoverage(t *testing.T) {
	root := t.TempDir()
	runGit(t, root, "init")
	runGit(t, root, "config", "user.name", "Tests")
	runGit(t, root, "config", "user.email", "tests@example.test")
	if err := os.Symlink("outside", root+"/package-lock.json"); err != nil {
		t.Fatal(err)
	}
	runGit(t, root, "add", ".")
	runGit(t, root, "commit", "-m", "Symlink")
	sha := *gitOutput(root, "rev-parse", "HEAD")
	discovery, err := DiscoverCommit(root, sha)
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := BuildBundleWithOptions(BuildBundleOptions{Root: root, Commit: sha, Files: discovery.Files})
	if err != nil {
		t.Fatal(err)
	}
	coverage := bundle.Manifest.Collection
	if coverage.Complete || len(coverage.Errors) != 1 || len(coverage.Paths) != 1 || len(bundle.Files) != 0 {
		t.Fatalf("missing coverage: %+v", coverage)
	}
	if _, err := DiscoverCommit(root, "--help"); err == nil {
		t.Fatal("accepted non-SHA commit")
	}
	if _, err := DiscoverCommit(root, strings.Repeat("a", 40)); err == nil {
		t.Fatal("accepted missing commit")
	}
}
