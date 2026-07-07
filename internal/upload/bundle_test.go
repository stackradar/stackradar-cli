package upload

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"os/exec"
	"reflect"
	"strings"
	"testing"
)

func TestBuildBundleCreatesDeterministicZip(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "b/composer.lock", "{\"packages\":[]}\n")
	writeTestFile(t, root, "a/package-lock.json", "{\"lockfileVersion\":3}\n")

	files := []File{
		{Path: "b/composer.lock", Ecosystem: "composer"},
		{Path: "a/package-lock.json", Ecosystem: "npm"},
	}

	first, err := BuildBundle(root, files)

	if err != nil {
		t.Fatalf("expected first bundle build to succeed, got %v", err)
	}

	second, err := BuildBundle(root, files)

	if err != nil {
		t.Fatalf("expected second bundle build to succeed, got %v", err)
	}

	if !bytes.Equal(first.Bytes, second.Bytes) {
		t.Fatal("expected repeated bundle builds to produce identical bytes")
	}

	if first.SHA256 == "" {
		t.Fatal("expected bundle SHA-256 to be populated")
	}

	if first.SHA256 != second.SHA256 {
		t.Fatalf("bundle SHA-256 = %q, want %q", second.SHA256, first.SHA256)
	}

	if first.SizeBytes != int64(len(first.Bytes)) {
		t.Fatalf("bundle size = %d, want %d", first.SizeBytes, len(first.Bytes))
	}

	entries := readZipEntries(t, first.Bytes)
	if entries["a/package-lock.json"] != "{\"lockfileVersion\":3}\n" {
		t.Fatalf("package-lock contents = %q", entries["a/package-lock.json"])
	}
	if entries["b/composer.lock"] != "{\"packages\":[]}\n" {
		t.Fatalf("composer.lock contents = %q", entries["b/composer.lock"])
	}
	if _, ok := entries[BundleManifestPath]; !ok {
		t.Fatalf("expected bundle manifest entry, got %#v", entries)
	}
}

func TestBuildBundleSortsEntriesByRelativePath(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "z/package.json", "{}")
	writeTestFile(t, root, "a/composer.json", "{}")

	bundle, err := BuildBundle(root, []File{
		{Path: "z/package.json", Ecosystem: "npm"},
		{Path: "a/composer.json", Ecosystem: "composer"},
	})

	if err != nil {
		t.Fatalf("expected bundle build to succeed, got %v", err)
	}

	reader, err := zip.NewReader(bytes.NewReader(bundle.Bytes), int64(len(bundle.Bytes)))

	if err != nil {
		t.Fatalf("open zip: %v", err)
	}

	got := make([]string, 0, len(reader.File))

	for _, file := range reader.File {
		got = append(got, file.Name)
	}

	want := []string{"a/composer.json", "z/package.json", BundleManifestPath}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("zip entry order = %#v, want %#v", got, want)
	}
}

func TestBuildBundleManifestDescribesFiles(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "package.json", "{\"name\":\"radar\"}\n")
	writeTestFile(t, root, "composer.lock", "{\"packages\":[]}\n")

	bundle, err := BuildBundle(root, []File{
		{Path: "package.json", Ecosystem: "npm"},
		{Path: "composer.lock", Ecosystem: "composer"},
	})

	if err != nil {
		t.Fatalf("expected bundle build to succeed, got %v", err)
	}

	var manifest BundleManifest
	if err := json.Unmarshal([]byte(readZipEntries(t, bundle.Bytes)[BundleManifestPath]), &manifest); err != nil {
		t.Fatalf("decode manifest: %v", err)
	}

	if manifest.SchemaVersion != 1 {
		t.Fatalf("schema version = %d, want 1", manifest.SchemaVersion)
	}
	if manifest.CLI.Name != "stackradar-cli" {
		t.Fatalf("cli name = %q, want stackradar-cli", manifest.CLI.Name)
	}
	if len(manifest.Files) != 2 {
		t.Fatalf("manifest files = %#v, want 2 entries", manifest.Files)
	}

	files := map[string]BundleManifestFile{}
	for _, file := range manifest.Files {
		files[file.Path] = file
	}

	if files["composer.lock"].Ecosystem != "composer" {
		t.Fatalf("composer ecosystem = %q", files["composer.lock"].Ecosystem)
	}
	if files["composer.lock"].SizeBytes != int64(len("{\"packages\":[]}\n")) {
		t.Fatalf("composer size = %d", files["composer.lock"].SizeBytes)
	}
	if files["composer.lock"].SHA256 == "" || strings.Contains(files["composer.lock"].SHA256, " ") {
		t.Fatalf("composer sha256 = %q", files["composer.lock"].SHA256)
	}
	if files["package.json"].Ecosystem != "npm" {
		t.Fatalf("package ecosystem = %q", files["package.json"].Ecosystem)
	}
}

func TestBuildBundleManifestMarksCleanGitCheckoutNotDirty(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "package.json", "{\"name\":\"radar\"}\n")
	runGit(t, root, "init")
	runGit(t, root, "config", "user.email", "tests@stackradar.com")
	runGit(t, root, "config", "user.name", "StackRadar Tests")
	runGit(t, root, "add", "package.json")
	runGit(t, root, "commit", "-m", "Initial commit")

	bundle, err := BuildBundle(root, []File{{Path: "package.json", Ecosystem: "npm"}})

	if err != nil {
		t.Fatalf("expected bundle build to succeed, got %v", err)
	}

	var manifest BundleManifest
	if err := json.Unmarshal([]byte(readZipEntries(t, bundle.Bytes)[BundleManifestPath]), &manifest); err != nil {
		t.Fatalf("decode manifest: %v", err)
	}

	if manifest.Git.Dirty == nil {
		t.Fatal("expected clean git checkout to set dirty to false, got nil")
	}
	if *manifest.Git.Dirty {
		t.Fatal("expected clean git checkout to set dirty to false, got true")
	}
}

func readZipEntries(t *testing.T, data []byte) map[string]string {
	t.Helper()

	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))

	if err != nil {
		t.Fatalf("open zip: %v", err)
	}

	result := make(map[string]string, len(reader.File))

	for _, file := range reader.File {
		handle, err := file.Open()

		if err != nil {
			t.Fatalf("open zip entry %s: %v", file.Name, err)
		}

		contents, err := io.ReadAll(handle)

		if closeErr := handle.Close(); closeErr != nil {
			t.Fatalf("close zip entry %s: %v", file.Name, closeErr)
		}

		if err != nil {
			t.Fatalf("read zip entry %s: %v", file.Name, err)
		}

		result[file.Name] = string(contents)
	}

	return result
}

func runGit(t *testing.T, root string, args ...string) {
	t.Helper()

	command := exec.Command("git", append([]string{"-C", root}, args...)...)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, string(output))
	}
}
