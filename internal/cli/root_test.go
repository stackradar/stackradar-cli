package cli

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stackradar/stackradar-cli/internal/upload"
)

var errWriteFailed = errors.New("write failed")

type failingWriter struct{}

func (failingWriter) Write(bytes []byte) (int, error) {
	return 0, errWriteFailed
}

func executeForTest(t *testing.T, args ...string) (string, string, error) {
	t.Helper()

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	command := NewRootCommand(Streams{
		Out: &stdout,
		Err: &stderr,
	})
	command.SetArgs(args)

	err := command.Execute()

	return stdout.String(), stderr.String(), err
}

func TestRootCommandShowsHelp(t *testing.T) {
	stdout, stderr, err := executeForTest(t, "--help")

	if err != nil {
		t.Fatalf("expected help command to succeed, got %v", err)
	}

	if stderr != "" {
		t.Fatalf("expected no stderr, got %q", stderr)
	}

	for _, expected := range []string{"StackRadar dependency evidence uploader", "bundle", "upload", "version"} {
		if !strings.Contains(stdout, expected) {
			t.Fatalf("expected help output to contain %q, got %q", expected, stdout)
		}
	}
}

func TestVersionCommandPrintsBuildMetadata(t *testing.T) {
	stdout, stderr, err := executeForTest(t, "version")

	if err != nil {
		t.Fatalf("expected version command to succeed, got %v", err)
	}

	if stderr != "" {
		t.Fatalf("expected no stderr, got %q", stderr)
	}

	if !strings.Contains(stdout, "stackradar dev") {
		t.Fatalf("expected version output to include dev version, got %q", stdout)
	}
}

func TestVersionCommandReturnsWriteErrors(t *testing.T) {
	command := NewRootCommand(Streams{
		Out: failingWriter{},
		Err: &bytes.Buffer{},
	})
	command.SetArgs([]string{"version"})

	err := command.Execute()

	if !errors.Is(err, errWriteFailed) {
		t.Fatalf("expected write error, got %v", err)
	}
}

func TestUploadCommandExposesPlannedFlags(t *testing.T) {
	stdout, stderr, err := executeForTest(t, "upload", "--help")

	if err != nil {
		t.Fatalf("expected upload help command to succeed, got %v", err)
	}

	if stderr != "" {
		t.Fatalf("expected no stderr, got %q", stderr)
	}

	for _, expected := range []string{"<bundle.zip>", "--api-url", "--dry-run", "--token", "--verbose"} {
		if !strings.Contains(stdout, expected) {
			t.Fatalf("expected upload help output to contain %q, got %q", expected, stdout)
		}
	}

	for _, unexpected := range []string{"--path", "--exclude"} {
		if strings.Contains(stdout, unexpected) {
			t.Fatalf("expected upload help output not to contain %q, got %q", unexpected, stdout)
		}
	}
}

func TestBundleCommandExposesPlannedFlags(t *testing.T) {
	stdout, stderr, err := executeForTest(t, "bundle", "--help")

	if err != nil {
		t.Fatalf("expected bundle help command to succeed, got %v", err)
	}

	if stderr != "" {
		t.Fatalf("expected no stderr, got %q", stderr)
	}

	for _, expected := range []string{"--path", "--exclude", "--output"} {
		if !strings.Contains(stdout, expected) {
			t.Fatalf("expected bundle help output to contain %q, got %q", expected, stdout)
		}
	}

	if !strings.Contains(stdout, defaultBundleOutputPath) {
		t.Fatalf("expected bundle help output to contain default output path %q, got %q", defaultBundleOutputPath, stdout)
	}
}

func TestUploadCommandRequiresToken(t *testing.T) {
	t.Setenv(uploadTokenEnv, "")

	bundlePath := filepath.Join(t.TempDir(), "stackradar-upload.zip")
	writeCLIFile(t, filepath.Dir(bundlePath), filepath.Base(bundlePath), "not a real zip")

	stdout, stderr, err := executeForTest(t, "upload", bundlePath)

	if err == nil {
		t.Fatal("expected upload command to require a token")
	}

	if stdout != "" {
		t.Fatalf("expected no stdout, got %q", stdout)
	}

	if !strings.Contains(stderr, "upload token is required") {
		t.Fatalf("expected stderr to contain missing token message, got %q", stderr)
	}
}

func TestUploadCommandUploadsBundleWithTokenFlag(t *testing.T) {
	bundlePath := filepath.Join(t.TempDir(), "stackradar-upload.zip")
	writeUploadBundleForCLITest(t, bundlePath)
	server := newUploadCommandTestServer(t, "flag-token")

	stdout, stderr, err := executeForTest(t, "upload", bundlePath, "--token", "flag-token", "--api-url", server.URL)

	if err != nil {
		t.Fatalf("expected upload command to succeed, got %v", err)
	}

	if stderr != "" {
		t.Fatalf("expected no stderr, got %q", stderr)
	}

	for _, expected := range []string{
		"Bundle uploaded:",
		"upload_id: run-123",
		"artifact_id: artifact-456",
		"status: processing",
	} {
		if !strings.Contains(stdout, expected) {
			t.Fatalf("expected stdout to contain %q, got %q", expected, stdout)
		}
	}

	if strings.Contains(stdout, "flag-token") || strings.Contains(stderr, "flag-token") {
		t.Fatalf("expected output not to contain the token, got stdout %q stderr %q", stdout, stderr)
	}
}

func TestUploadCommandUploadsBundleWithTokenEnvironmentVariable(t *testing.T) {
	t.Setenv(uploadTokenEnv, "env-token")

	bundlePath := filepath.Join(t.TempDir(), "stackradar-upload.zip")
	writeUploadBundleForCLITest(t, bundlePath)
	server := newUploadCommandTestServer(t, "env-token")

	stdout, stderr, err := executeForTest(t, "upload", bundlePath, "--api-url", server.URL)

	if err != nil {
		t.Fatalf("expected upload command to succeed, got %v", err)
	}

	if stderr != "" {
		t.Fatalf("expected no stderr, got %q", stderr)
	}

	if !strings.Contains(stdout, "status: processing") {
		t.Fatalf("expected stdout to contain upload status, got %q", stdout)
	}

	if strings.Contains(stdout, "env-token") || strings.Contains(stderr, "env-token") {
		t.Fatalf("expected output not to contain the token, got stdout %q stderr %q", stdout, stderr)
	}
}

func TestUploadCommandRequiresBundleArgument(t *testing.T) {
	stdout, stderr, err := executeForTest(t, "upload", "--token", "flag-token")

	if err == nil {
		t.Fatal("expected upload command to require a bundle path")
	}

	if stdout != "" {
		t.Fatalf("expected no stdout, got %q", stdout)
	}

	if !strings.Contains(stderr, "accepts 1 arg") {
		t.Fatalf("expected stderr to contain argument count error, got %q", stderr)
	}
}

func TestUploadCommandDryRunDoesNotRequireToken(t *testing.T) {
	t.Setenv(uploadTokenEnv, "")

	bundlePath := filepath.Join(t.TempDir(), "stackradar-upload.zip")
	writeUploadBundleForCLITest(t, bundlePath)
	metadata, err := upload.InspectBundle(bundlePath)
	if err != nil {
		t.Fatalf("inspect bundle: %v", err)
	}

	stdout, stderr, err := executeForTest(t, "upload", bundlePath, "--dry-run", "--api-url", "https://stackradar.test")

	if err != nil {
		t.Fatalf("expected upload dry run command to succeed, got %v", err)
	}

	if stderr != "" {
		t.Fatalf("expected no stderr, got %q", stderr)
	}

	for _, expected := range []string{
		"Upload dry run:",
		"bundle: " + bundlePath,
		"format: zip",
		"content_type: application/zip",
		"size: " + formatBytes(metadata.SizeBytes),
		"sha256:",
		"api_url: https://stackradar.test",
		"status: dry-run",
	} {
		if !strings.Contains(stdout, expected) {
			t.Fatalf("expected stdout to contain %q, got %q", expected, stdout)
		}
	}

	if strings.Contains(stdout, "token") || strings.Contains(stderr, "token") {
		t.Fatalf("expected dry-run output not to mention a token, got stdout %q stderr %q", stdout, stderr)
	}
}

func TestBundleCommandWritesBundleAndPrintsSummary(t *testing.T) {
	root := t.TempDir()
	writeCLIFile(t, root, "package-lock.json", "{\"lockfileVersion\":3}\n")
	writeCLIFile(t, root, "backend/composer.lock", "{\"packages\":[]}\n")
	writeCLIFile(t, root, "node_modules/package-lock.json", "{}\n")
	outputPath := filepath.Join(t.TempDir(), "stackradar-upload.zip")

	stdout, stderr, err := executeForTest(t, "bundle", "--path", root, "--output", outputPath)

	if err != nil {
		t.Fatalf("expected bundle command to succeed, got %v", err)
	}

	if stderr != "" {
		t.Fatalf("expected no stderr, got %q", stderr)
	}

	for _, expected := range []string{
		"Discovered dependency files:",
		"backend/composer.lock (composer",
		"package-lock.json (npm",
		"Skipped directories:",
		"node_modules (ignored directory)",
		"Bundle:",
		"output: " + outputPath,
		"files: 2",
		"sha256:",
		"bundle written.",
	} {
		if !strings.Contains(stdout, expected) {
			t.Fatalf("expected bundle output to contain %q, got %q", expected, stdout)
		}
	}

	got := readZipFileEntries(t, outputPath)
	for path, contents := range map[string]string{
		"backend/composer.lock": "{\"packages\":[]}\n",
		"package-lock.json":     "{\"lockfileVersion\":3}\n",
	} {
		if got[path] != contents {
			t.Fatalf("bundle entry %s = %q, want %q", path, got[path], contents)
		}
	}
	if _, ok := got[upload.BundleManifestPath]; !ok {
		t.Fatalf("expected bundle manifest entry, got %#v", got)
	}
}

func TestBundleCommandHonorsGitignore(t *testing.T) {
	root := t.TempDir()
	writeCLIFile(t, root, ".gitignore", "/tmp/\n")
	writeCLIFile(t, root, "package-lock.json", "{\"lockfileVersion\":3}\n")
	writeCLIFile(t, root, "tmp/pptx/package-lock.json", "{\"lockfileVersion\":3}\n")
	outputPath := filepath.Join(t.TempDir(), "stackradar-upload.zip")

	stdout, stderr, err := executeForTest(t, "bundle", "--path", root, "--output", outputPath)

	if err != nil {
		t.Fatalf("expected bundle command to succeed, got %v", err)
	}

	if stderr != "" {
		t.Fatalf("expected no stderr, got %q", stderr)
	}

	if strings.Contains(stdout, "tmp/pptx/package-lock.json") {
		t.Fatalf("expected gitignored tmp file to be omitted, got %q", stdout)
	}

	if !strings.Contains(stdout, "tmp (gitignored)") {
		t.Fatalf("expected dry-run output to report gitignored tmp directory, got %q", stdout)
	}

	if !strings.Contains(stdout, "files: 1") {
		t.Fatalf("expected bundle summary to include one file, got %q", stdout)
	}
}

func TestBundleCommandDefaultsOutputPathToWorkingDirectory(t *testing.T) {
	root := t.TempDir()
	writeCLIFile(t, root, "package-lock.json", "{\"lockfileVersion\":3}\n")
	workingDirectory := t.TempDir()
	chdirForTest(t, workingDirectory)

	stdout, stderr, err := executeForTest(t, "bundle", "--path", root)

	if err != nil {
		t.Fatalf("expected bundle command to succeed, got %v", err)
	}

	if stderr != "" {
		t.Fatalf("expected no stderr, got %q", stderr)
	}

	outputPath := filepath.Join(workingDirectory, defaultBundleOutputPath)
	if _, err := os.Stat(outputPath); err != nil {
		t.Fatalf("expected default bundle output at %s, got %v", outputPath, err)
	}

	if !strings.Contains(stdout, "output: "+defaultBundleOutputPath) {
		t.Fatalf("expected stdout to mention default output path, got %q", stdout)
	}
}

func TestBundleCommandReturnsErrorWhenNoDependencyFilesAreFound(t *testing.T) {
	root := t.TempDir()
	writeCLIFile(t, root, "README.md", "# ignored\n")
	outputPath := filepath.Join(t.TempDir(), "stackradar-upload.zip")

	stdout, stderr, err := executeForTest(t, "bundle", "--path", root, "--output", outputPath)

	if err == nil {
		t.Fatal("expected bundle command with no dependency files to fail")
	}

	if stdout != "" {
		t.Fatalf("expected no stdout, got %q", stdout)
	}

	if !strings.Contains(stderr, "no supported dependency files found") {
		t.Fatalf("expected stderr to contain no dependency files message, got %q", stderr)
	}
}

func readZipFileEntries(t *testing.T, zipPath string) map[string]string {
	t.Helper()

	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		t.Fatalf("open zip file %s: %v", zipPath, err)
	}
	defer func() {
		if err := reader.Close(); err != nil {
			t.Fatalf("close zip file %s: %v", zipPath, err)
		}
	}()

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

func newUploadCommandTestServer(t *testing.T, expectedToken string) *httptest.Server {
	t.Helper()

	var serverURL string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/api/ingestions/evidence-uploads":
			if request.Method != http.MethodPost {
				t.Fatalf("initialize method = %s, want POST", request.Method)
			}
			if got := request.Header.Get("Authorization"); got != "Bearer "+expectedToken {
				t.Fatalf("authorization header = %q, want bearer token", got)
			}

			var payload map[string]any
			if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
				t.Fatalf("decode initialize payload: %v", err)
			}
			if _, ok := payload["bundle"].(map[string]any); !ok {
				t.Fatalf("expected initialize payload to include bundle metadata, got %#v", payload)
			}
			manifest, ok := payload["manifest"].(map[string]any)
			if !ok {
				t.Fatalf("expected initialize payload to include manifest summary, got %#v", payload)
			}
			if manifest["schema_version"] != float64(1) {
				t.Fatalf("manifest schema version = %#v, want 1", manifest["schema_version"])
			}
			if manifest["files_count"] != float64(1) {
				t.Fatalf("manifest files count = %#v, want 1", manifest["files_count"])
			}

			writer.Header().Set("Content-Type", "application/json")
			writer.WriteHeader(http.StatusCreated)
			if _, err := writer.Write([]byte(`{
				"upload_id": "run-123",
				"artifact_id": "artifact-456",
				"status": "upload_pending",
				"upload": {
					"method": "PUT",
					"url": "` + serverURL + `/signed-upload",
					"headers": [],
					"expires_at": "2026-07-06T12:00:00Z"
				}
			}`)); err != nil {
				t.Fatalf("write initialize response: %v", err)
			}

		case "/signed-upload":
			if request.Method != http.MethodPut {
				t.Fatalf("upload method = %s, want PUT", request.Method)
			}
			if got := request.Header.Get("Content-Type"); got != "application/zip" {
				t.Fatalf("content type = %q, want application/zip", got)
			}

			writer.Header().Set("Content-Type", "application/json")
			writer.WriteHeader(http.StatusAccepted)
			if _, err := writer.Write([]byte(`{
				"upload_id": "run-123",
				"artifact_id": "artifact-456",
				"status": "processing"
			}`)); err != nil {
				t.Fatalf("write upload response: %v", err)
			}

		default:
			t.Fatalf("unexpected request path %s", request.URL.Path)
		}
	}))
	serverURL = server.URL

	t.Cleanup(server.Close)

	return server
}

func chdirForTest(t *testing.T, directory string) {
	t.Helper()

	previousDirectory, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}

	if err := os.Chdir(directory); err != nil {
		t.Fatalf("change working directory to %s: %v", directory, err)
	}

	t.Cleanup(func() {
		if err := os.Chdir(previousDirectory); err != nil {
			t.Fatalf("restore working directory to %s: %v", previousDirectory, err)
		}
	})
}

func writeCLIFile(t *testing.T, root string, relativePath string, contents string) {
	t.Helper()

	fullPath := filepath.Join(root, filepath.FromSlash(relativePath))

	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		t.Fatalf("create parent directories for %s: %v", relativePath, err)
	}

	if err := os.WriteFile(fullPath, []byte(contents), 0o644); err != nil {
		t.Fatalf("write %s: %v", relativePath, err)
	}
}

func writeUploadBundleForCLITest(t *testing.T, path string) {
	t.Helper()

	root := t.TempDir()
	writeCLIFile(t, root, "package.json", "{}\n")

	bundle, err := upload.BuildBundle(root, []upload.File{{Path: "package.json", Ecosystem: "npm"}})
	if err != nil {
		t.Fatalf("build upload bundle: %v", err)
	}

	if err := os.WriteFile(path, bundle.Bytes, 0o644); err != nil {
		t.Fatalf("write upload bundle: %v", err)
	}
}
