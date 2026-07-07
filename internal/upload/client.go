package upload

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

const bundleContentType = "application/zip"

type UploadOptions struct {
	APIURL        string
	Token         string
	BundlePath    string
	ClientName    string
	ClientVersion string
	HTTPClient    *http.Client
}

type BundleMetadata struct {
	Format      string
	ContentType string
	SizeBytes   int64
	SHA256      string
}

type UploadResult struct {
	UploadID   string
	ArtifactID string
	Status     string
}

type initializeUploadPayload struct {
	Bundle   initializeBundlePayload   `json:"bundle"`
	Client   initializeClientPayload   `json:"client"`
	Manifest initializeManifestPayload `json:"manifest"`
}

type initializeBundlePayload struct {
	Format      string `json:"format"`
	ContentType string `json:"content_type"`
	SizeBytes   int64  `json:"size_bytes"`
	SHA256      string `json:"sha256"`
}

type initializeClientPayload struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type initializeManifestPayload struct {
	SchemaVersion int               `json:"schema_version"`
	Git           BundleManifestGit `json:"git"`
	FilesCount    int               `json:"files_count"`
	SHA256        string            `json:"sha256"`
}

type initializeUploadResponse struct {
	UploadID   string       `json:"upload_id"`
	ArtifactID string       `json:"artifact_id"`
	Status     string       `json:"status"`
	Upload     uploadTarget `json:"upload"`
}

type uploadTarget struct {
	Method  string        `json:"method"`
	URL     string        `json:"url"`
	Headers uploadHeaders `json:"headers"`
}

type storeUploadResponse struct {
	UploadID   string `json:"upload_id"`
	ArtifactID string `json:"artifact_id"`
	Status     string `json:"status"`
}

type uploadHeaders map[string]string

func UploadBundle(options UploadOptions) (UploadResult, error) {
	bundle, err := os.ReadFile(options.BundlePath)
	if err != nil {
		return UploadResult{}, err
	}

	return uploadBundleBytes(options, bundle)
}

func InspectBundle(bundlePath string) (BundleMetadata, error) {
	bundle, err := os.ReadFile(bundlePath)
	if err != nil {
		return BundleMetadata{}, err
	}

	if _, err := readManifestSummaryFromBundle(bundle); err != nil {
		return BundleMetadata{}, err
	}

	return bundleMetadata(bundle), nil
}

func uploadBundleBytes(options UploadOptions, bundle []byte) (UploadResult, error) {
	client := options.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}

	initialized, err := initializeUpload(client, options, bundle)
	if err != nil {
		return UploadResult{}, err
	}

	stored, err := storeBundle(client, initialized.Upload, bundle)
	if err != nil {
		return UploadResult{}, err
	}

	return UploadResult(stored), nil
}

func initializeUpload(client *http.Client, options UploadOptions, bundle []byte) (initializeUploadResponse, error) {
	metadata := bundleMetadata(bundle)
	manifest, err := readManifestSummaryFromBundle(bundle)
	if err != nil {
		return initializeUploadResponse{}, err
	}

	payload := initializeUploadPayload{
		Bundle: initializeBundlePayload(metadata),
		Client: initializeClientPayload{
			Name:    options.ClientName,
			Version: options.ClientVersion,
		},
		Manifest: initializeManifestPayload(manifest),
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return initializeUploadResponse{}, err
	}

	request, err := http.NewRequest(http.MethodPost, initializeUploadURL(options.APIURL), bytes.NewReader(body))
	if err != nil {
		return initializeUploadResponse{}, err
	}
	request.Header.Set("Authorization", "Bearer "+options.Token)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")

	response, err := client.Do(request)
	if err != nil {
		return initializeUploadResponse{}, err
	}
	defer func() {
		_ = response.Body.Close()
	}()

	if response.StatusCode != http.StatusCreated {
		return initializeUploadResponse{}, responseError("initialize upload", response)
	}

	var decoded initializeUploadResponse
	if err := json.NewDecoder(response.Body).Decode(&decoded); err != nil {
		return initializeUploadResponse{}, err
	}

	if strings.ToUpper(decoded.Upload.Method) != http.MethodPut {
		return initializeUploadResponse{}, fmt.Errorf("initialize upload returned unsupported upload method %q", decoded.Upload.Method)
	}
	if decoded.Upload.URL == "" {
		return initializeUploadResponse{}, fmt.Errorf("initialize upload did not return an upload URL")
	}

	return decoded, nil
}

func storeBundle(client *http.Client, target uploadTarget, bundle []byte) (storeUploadResponse, error) {
	request, err := http.NewRequest(http.MethodPut, target.URL, bytes.NewReader(bundle))
	if err != nil {
		return storeUploadResponse{}, err
	}
	request.Header.Set("Content-Type", bundleContentType)
	request.Header.Set("Accept", "application/json")
	request.ContentLength = int64(len(bundle))

	for name, value := range target.Headers {
		request.Header.Set(name, value)
	}

	response, err := client.Do(request)
	if err != nil {
		return storeUploadResponse{}, err
	}
	defer func() {
		_ = response.Body.Close()
	}()

	if response.StatusCode != http.StatusAccepted {
		return storeUploadResponse{}, responseError("upload bundle", response)
	}

	var decoded storeUploadResponse
	if err := json.NewDecoder(response.Body).Decode(&decoded); err != nil {
		return storeUploadResponse{}, err
	}

	return decoded, nil
}

func initializeUploadURL(apiURL string) string {
	base := strings.TrimRight(apiURL, "/")
	if base == "" {
		base = "https://stackradar.com"
	}

	return base + "/api/ingestions/evidence-uploads"
}

func responseError(operation string, response *http.Response) error {
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return fmt.Errorf("%s failed with HTTP %d", operation, response.StatusCode)
	}

	var payload struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &payload); err == nil && payload.Message != "" {
		return fmt.Errorf("%s failed with HTTP %d: %s", operation, response.StatusCode, payload.Message)
	}

	return fmt.Errorf("%s failed with HTTP %d", operation, response.StatusCode)
}

func (headers *uploadHeaders) UnmarshalJSON(data []byte) error {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" || trimmed == "[]" || trimmed == "null" {
		*headers = uploadHeaders{}
		return nil
	}

	values := map[string]string{}
	if err := json.Unmarshal(data, &values); err != nil {
		return err
	}

	*headers = uploadHeaders(values)
	return nil
}

func bundleMetadata(bundle []byte) BundleMetadata {
	sum := sha256.Sum256(bundle)

	return BundleMetadata{
		Format:      "zip",
		ContentType: bundleContentType,
		SizeBytes:   int64(len(bundle)),
		SHA256:      hex.EncodeToString(sum[:]),
	}
}
