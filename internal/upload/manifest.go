package upload

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

const BundleManifestPath = "stackradar-manifest.json"

type BundleManifest struct {
	SchemaVersion int                  `json:"schema_version"`
	CLI           BundleManifestCLI    `json:"cli"`
	Git           BundleManifestGit    `json:"git"`
	Files         []BundleManifestFile `json:"files"`
}

type BundleManifestCLI struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type BundleManifestGit struct {
	CommitSHA *string `json:"commit_sha"`
	Ref       *string `json:"ref"`
	Branch    *string `json:"branch"`
	Dirty     *bool   `json:"dirty"`
}

type BundleManifestFile struct {
	Path      string `json:"path"`
	Ecosystem string `json:"ecosystem"`
	SizeBytes int64  `json:"size_bytes"`
	SHA256    string `json:"sha256"`
}

type BundleManifestSummary struct {
	SchemaVersion int               `json:"schema_version"`
	Git           BundleManifestGit `json:"git"`
	FilesCount    int               `json:"files_count"`
	SHA256        string            `json:"sha256"`
}

func discoverGitContext(root string) BundleManifestGit {
	commitSHA := gitOutput(root, "rev-parse", "--verify", "HEAD")
	ref := gitOutput(root, "symbolic-ref", "-q", "HEAD")
	branch := gitOutput(root, "branch", "--show-current")

	return BundleManifestGit{
		CommitSHA: commitSHA,
		Ref:       ref,
		Branch:    branch,
		Dirty:     gitDirty(root),
	}
}

func gitDirty(root string) *bool {
	command := exec.Command("git", "-C", root, "status", "--porcelain")
	output, err := command.Output()
	if err != nil {
		return nil
	}

	value := strings.TrimSpace(string(output)) != ""

	return &value
}

func gitOutput(root string, args ...string) *string {
	command := exec.Command("git", append([]string{"-C", root}, args...)...)
	output, err := command.Output()
	if err != nil {
		return nil
	}

	value := strings.TrimSpace(string(output))
	if value == "" {
		return nil
	}

	return &value
}

func readManifestSummaryFromBundle(bundle []byte) (BundleManifestSummary, error) {
	reader, err := zip.NewReader(bytes.NewReader(bundle), int64(len(bundle)))
	if err != nil {
		return BundleManifestSummary{}, fmt.Errorf("open bundle zip: %w", err)
	}

	for _, file := range reader.File {
		if file.Name != BundleManifestPath {
			continue
		}

		contents, err := readZipFile(file)
		if err != nil {
			return BundleManifestSummary{}, err
		}

		var manifest BundleManifest
		if err := json.Unmarshal(contents, &manifest); err != nil {
			return BundleManifestSummary{}, fmt.Errorf("decode bundle manifest: %w", err)
		}
		if manifest.SchemaVersion != 1 {
			return BundleManifestSummary{}, fmt.Errorf("bundle manifest schema version %d is not supported", manifest.SchemaVersion)
		}

		sum := sha256.Sum256(contents)

		return BundleManifestSummary{
			SchemaVersion: manifest.SchemaVersion,
			Git:           manifest.Git,
			FilesCount:    len(manifest.Files),
			SHA256:        hex.EncodeToString(sum[:]),
		}, nil
	}

	return BundleManifestSummary{}, fmt.Errorf("bundle is missing %s", BundleManifestPath)
}

func readZipFile(file *zip.File) ([]byte, error) {
	handle, err := file.Open()
	if err != nil {
		return nil, err
	}

	contents, readErr := io.ReadAll(handle)
	closeErr := handle.Close()
	if readErr != nil {
		return nil, readErr
	}
	if closeErr != nil {
		return nil, closeErr
	}

	return contents, nil
}
