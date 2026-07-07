package upload

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

type Bundle struct {
	Bytes     []byte
	SizeBytes int64
	SHA256    string
	Files     []File
	Manifest  BundleManifest
}

var zipModifiedTime = time.Date(1980, time.January, 1, 0, 0, 0, 0, time.UTC)

type BuildBundleOptions struct {
	Root          string
	Files         []File
	ClientName    string
	ClientVersion string
}

func BuildBundle(root string, files []File) (Bundle, error) {
	return BuildBundleWithOptions(BuildBundleOptions{
		Root:          root,
		Files:         files,
		ClientName:    "stackradar-cli",
		ClientVersion: "dev",
	})
}

func BuildBundleWithOptions(options BuildBundleOptions) (Bundle, error) {
	root := options.Root
	if root == "" {
		root = "."
	}
	clientName := options.ClientName
	if clientName == "" {
		clientName = "stackradar-cli"
	}
	clientVersion := options.ClientVersion
	if clientVersion == "" {
		clientVersion = "dev"
	}

	sortedFiles := append([]File(nil), options.Files...)
	sort.Slice(sortedFiles, func(left int, right int) bool {
		return sortedFiles[left].Path < sortedFiles[right].Path
	})

	manifestFiles := make([]BundleManifestFile, 0, len(sortedFiles))
	fileContents := make(map[string][]byte, len(sortedFiles))
	for _, file := range sortedFiles {
		contents, err := readBundleFile(root, file)
		if err != nil {
			return Bundle{}, err
		}

		sum := sha256.Sum256(contents)
		fileContents[file.Path] = contents
		manifestFiles = append(manifestFiles, BundleManifestFile{
			Path:      file.Path,
			Ecosystem: file.Ecosystem,
			SizeBytes: int64(len(contents)),
			SHA256:    hex.EncodeToString(sum[:]),
		})
	}

	manifest := BundleManifest{
		SchemaVersion: 1,
		CLI: BundleManifestCLI{
			Name:    clientName,
			Version: clientVersion,
		},
		Git:   discoverGitContext(root),
		Files: manifestFiles,
	}
	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		return Bundle{}, err
	}
	manifestBytes = append(manifestBytes, '\n')

	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)

	for _, file := range sortedFiles {
		if err := addBundleEntry(writer, file.Path, fileContents[file.Path]); err != nil {
			closeErr := writer.Close()
			if closeErr != nil {
				return Bundle{}, fmt.Errorf("%w; close zip: %v", err, closeErr)
			}

			return Bundle{}, err
		}
	}

	if err := addBundleEntry(writer, BundleManifestPath, manifestBytes); err != nil {
		closeErr := writer.Close()
		if closeErr != nil {
			return Bundle{}, fmt.Errorf("%w; close zip: %v", err, closeErr)
		}

		return Bundle{}, err
	}

	if err := writer.Close(); err != nil {
		return Bundle{}, err
	}

	bytes := buffer.Bytes()
	sum := sha256.Sum256(bytes)

	return Bundle{
		Bytes:     bytes,
		SizeBytes: int64(len(bytes)),
		SHA256:    hex.EncodeToString(sum[:]),
		Files:     sortedFiles,
		Manifest:  manifest,
	}, nil
}

func readBundleFile(root string, file File) ([]byte, error) {
	if file.Path == "" || filepath.IsAbs(file.Path) {
		return nil, fmt.Errorf("invalid bundle file path %q", file.Path)
	}

	sourcePath := filepath.Join(root, filepath.FromSlash(file.Path))
	info, err := os.Lstat(sourcePath)
	if err != nil {
		return nil, err
	}

	if info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("refusing to bundle symlink %q", file.Path)
	}

	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("refusing to bundle non-regular file %q", file.Path)
	}

	return os.ReadFile(sourcePath)
}

func addBundleEntry(writer *zip.Writer, path string, contents []byte) error {
	header := &zip.FileHeader{
		Name:     path,
		Method:   zip.Deflate,
		Modified: zipModifiedTime,
	}
	header.SetMode(0o644)

	entry, err := writer.CreateHeader(header)
	if err != nil {
		return err
	}

	_, err = entry.Write(contents)

	return err
}
