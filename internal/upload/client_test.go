package upload

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUploadBundleInitializesAndStoresBundle(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "package.json", "{\"name\":\"radar\"}\n")
	bundle, err := BuildBundle(root, []File{{Path: "package.json", Ecosystem: "npm"}})
	if err != nil {
		t.Fatalf("build bundle: %v", err)
	}
	bundleBytes := bundle.Bytes
	bundlePath := filepath.Join(t.TempDir(), "stackradar.zip")
	if err := os.WriteFile(bundlePath, bundleBytes, 0o644); err != nil {
		t.Fatalf("write bundle: %v", err)
	}

	var serverURL string
	initCalled := false
	storeCalled := false

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/api/ingestions/evidence-uploads":
			initCalled = true
			assertRequestMethod(t, request, http.MethodPost)
			assertRequestHeader(t, request, "Authorization", "Bearer secret-token")
			assertRequestHeader(t, request, "Content-Type", "application/json")

			var payload initializeUploadPayload
			if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
				t.Fatalf("decode initialize payload: %v", err)
			}

			wantSHA := sha256.Sum256(bundleBytes)
			if payload.Bundle.Format != "zip" {
				t.Fatalf("bundle format = %q, want zip", payload.Bundle.Format)
			}
			if payload.Bundle.ContentType != "application/zip" {
				t.Fatalf("bundle content type = %q, want application/zip", payload.Bundle.ContentType)
			}
			if payload.Bundle.SizeBytes != int64(len(bundleBytes)) {
				t.Fatalf("bundle size = %d, want %d", payload.Bundle.SizeBytes, len(bundleBytes))
			}
			if payload.Bundle.SHA256 != hex.EncodeToString(wantSHA[:]) {
				t.Fatalf("bundle sha256 = %q, want %q", payload.Bundle.SHA256, hex.EncodeToString(wantSHA[:]))
			}
			if payload.Client.Name != "stackradar-cli" {
				t.Fatalf("client name = %q, want stackradar-cli", payload.Client.Name)
			}
			if payload.Client.Version != "dev" {
				t.Fatalf("client version = %q, want dev", payload.Client.Version)
			}
			if payload.Manifest.SchemaVersion != 1 {
				t.Fatalf("manifest schema version = %d, want 1", payload.Manifest.SchemaVersion)
			}
			if payload.Manifest.FilesCount != 1 {
				t.Fatalf("manifest file count = %d, want 1", payload.Manifest.FilesCount)
			}
			if payload.Manifest.SHA256 == "" {
				t.Fatal("expected manifest sha256 to be populated")
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
					"headers": {"X-Upload-Token": "target-token"},
					"expires_at": "2026-07-06T12:00:00Z"
				}
			}`)); err != nil {
				t.Fatalf("write initialize response: %v", err)
			}

		case "/signed-upload":
			storeCalled = true
			assertRequestMethod(t, request, http.MethodPut)
			assertRequestHeader(t, request, "Content-Type", "application/zip")
			assertRequestHeader(t, request, "X-Upload-Token", "target-token")
			if request.ContentLength != int64(len(bundleBytes)) {
				t.Fatalf("content length = %d, want %d", request.ContentLength, len(bundleBytes))
			}

			body, err := io.ReadAll(request.Body)
			if err != nil {
				t.Fatalf("read upload body: %v", err)
			}
			if string(body) != string(bundleBytes) {
				t.Fatalf("upload body = %q, want %q", string(body), string(bundleBytes))
			}

			writer.Header().Set("Content-Type", "application/json")
			writer.WriteHeader(http.StatusAccepted)
			if _, err := writer.Write([]byte(`{
				"upload_id": "run-123",
				"artifact_id": "artifact-456",
				"status": "processing"
			}`)); err != nil {
				t.Fatalf("write store response: %v", err)
			}

		default:
			t.Fatalf("unexpected request path %s", request.URL.Path)
		}
	}))
	defer server.Close()
	serverURL = server.URL

	result, err := UploadBundle(UploadOptions{
		APIURL:        server.URL,
		Token:         "secret-token",
		BundlePath:    bundlePath,
		ClientName:    "stackradar-cli",
		ClientVersion: "dev",
		HTTPClient:    server.Client(),
	})

	if err != nil {
		t.Fatalf("expected upload to succeed, got %v", err)
	}

	if !initCalled {
		t.Fatal("expected initialize request")
	}
	if !storeCalled {
		t.Fatal("expected bundle store request")
	}
	if result.UploadID != "run-123" {
		t.Fatalf("upload ID = %q, want run-123", result.UploadID)
	}
	if result.ArtifactID != "artifact-456" {
		t.Fatalf("artifact ID = %q, want artifact-456", result.ArtifactID)
	}
	if result.Status != "processing" {
		t.Fatalf("status = %q, want processing", result.Status)
	}
}

func TestUploadBundleReturnsInitializeErrorWithoutTokenLeak(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "package.json", "{}\n")
	bundle, err := BuildBundle(root, []File{{Path: "package.json", Ecosystem: "npm"}})
	if err != nil {
		t.Fatalf("build bundle: %v", err)
	}
	bundlePath := filepath.Join(t.TempDir(), "stackradar.zip")
	if err := os.WriteFile(bundlePath, bundle.Bytes, 0o644); err != nil {
		t.Fatalf("write bundle: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		assertRequestMethod(t, request, http.MethodPost)
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusForbidden)
		if _, err := writer.Write([]byte(`{"message":"GitHub organization is not connected to StackRadar."}`)); err != nil {
			t.Fatalf("write initialize response: %v", err)
		}
	}))
	defer server.Close()

	_, err = UploadBundle(UploadOptions{
		APIURL:        server.URL,
		Token:         "secret-token",
		BundlePath:    bundlePath,
		ClientName:    "stackradar-cli",
		ClientVersion: "dev",
		HTTPClient:    server.Client(),
	})

	if err == nil {
		t.Fatal("expected upload error")
	}

	message := err.Error()
	if !strings.Contains(message, "403") {
		t.Fatalf("expected error to include status code, got %q", message)
	}
	if !strings.Contains(message, "GitHub organization is not connected to StackRadar.") {
		t.Fatalf("expected error to include server message, got %q", message)
	}
	if strings.Contains(message, "secret-token") {
		t.Fatalf("expected error not to leak token, got %q", message)
	}
}

func TestInspectBundleReturnsUploadMetadata(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "package.json", "{}\n")
	bundle, err := BuildBundle(root, []File{{Path: "package.json", Ecosystem: "npm"}})
	if err != nil {
		t.Fatalf("build bundle: %v", err)
	}
	bundleBytes := bundle.Bytes
	bundlePath := filepath.Join(t.TempDir(), "stackradar.zip")
	if err := os.WriteFile(bundlePath, bundleBytes, 0o644); err != nil {
		t.Fatalf("write bundle: %v", err)
	}

	metadata, err := InspectBundle(bundlePath)

	if err != nil {
		t.Fatalf("expected inspect bundle to succeed, got %v", err)
	}

	wantSHA := sha256.Sum256(bundleBytes)
	if metadata.Format != "zip" {
		t.Fatalf("format = %q, want zip", metadata.Format)
	}
	if metadata.ContentType != "application/zip" {
		t.Fatalf("content type = %q, want application/zip", metadata.ContentType)
	}
	if metadata.SizeBytes != int64(len(bundleBytes)) {
		t.Fatalf("size = %d, want %d", metadata.SizeBytes, len(bundleBytes))
	}
	if metadata.SHA256 != hex.EncodeToString(wantSHA[:]) {
		t.Fatalf("sha256 = %q, want %q", metadata.SHA256, hex.EncodeToString(wantSHA[:]))
	}
}

func TestInspectBundleRejectsMissingManifest(t *testing.T) {
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	if err := addBundleEntry(writer, "package.json", []byte("{}\n")); err != nil {
		t.Fatalf("add bundle entry: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close bundle: %v", err)
	}

	bundlePath := filepath.Join(t.TempDir(), "stackradar.zip")
	if err := os.WriteFile(bundlePath, buffer.Bytes(), 0o644); err != nil {
		t.Fatalf("write bundle: %v", err)
	}

	_, err := InspectBundle(bundlePath)

	if err == nil {
		t.Fatal("expected missing manifest error")
	}
	if !strings.Contains(err.Error(), BundleManifestPath) {
		t.Fatalf("expected error to mention %s, got %q", BundleManifestPath, err.Error())
	}
}

func assertRequestMethod(t *testing.T, request *http.Request, method string) {
	t.Helper()

	if request.Method != method {
		t.Fatalf("method = %s, want %s", request.Method, method)
	}
}

func assertRequestHeader(t *testing.T, request *http.Request, name string, value string) {
	t.Helper()

	if got := request.Header.Get(name); got != value {
		t.Fatalf("%s header = %q, want %q", name, got, value)
	}
}
