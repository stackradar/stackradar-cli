package upload

import (
	"fmt"
	"os"
	"os/exec"
	"path"
	"strconv"
	"strings"
)

// DiscoverCommit lists committed dependency inputs without consulting the working tree.
func DiscoverCommit(root, commit string) (Discovery, error) {
	if !commitSHA.MatchString(commit) {
		return Discovery{}, fmt.Errorf("commit must be an exact Git SHA")
	}
	output, err := commitGit(root, "ls-tree", "-r", "-l", "-z", "--full-tree", commit)
	if err != nil {
		return Discovery{}, fmt.Errorf("list committed dependency files: %w", err)
	}
	discovery := Discovery{Files: []File{}}
	for _, entry := range strings.Split(string(output), "\x00") {
		if entry == "" {
			continue
		}
		metadata, filename, ok := strings.Cut(entry, "\t")
		fields := strings.Fields(metadata)
		if !ok || len(fields) != 4 {
			return Discovery{}, fmt.Errorf("invalid Git tree entry")
		}
		ecosystem := committedFileEcosystem(filename)
		if ecosystem == "" {
			continue
		}
		size, _ := strconv.ParseInt(fields[3], 10, 64)
		discovery.Files = append(discovery.Files, File{Path: filename, Ecosystem: ecosystem, SizeBytes: size, BlobSHA: fields[2], Mode: fields[0]})
	}
	if len(discovery.Files) == 0 {
		return discovery, ErrNoDependencyFiles
	}
	return discovery, nil
}

func committedFileEcosystem(filename string) string {
	for _, segment := range strings.Split(path.Dir(filename), "/") {
		if _, ignored := ignoredDirectoryNames[segment]; ignored {
			return ""
		}
	}
	if ecosystem := supportedFileEcosystems[path.Base(filename)]; ecosystem != "" {
		return ecosystem
	}
	if path.Base(filename) == "pnpm-workspace.yaml" {
		return "npm"
	}
	extension := strings.ToLower(path.Ext(filename))
	if extension == ".txt" || extension == ".in" {
		if strings.HasPrefix(path.Base(filename), "requirements") || strings.HasPrefix(path.Base(filename), "constraints") {
			return "python"
		}
		for _, segment := range strings.Split(path.Dir(filename), "/") {
			if segment == "requirements" || segment == "constraints" {
				return "python"
			}
		}
	}
	return ""
}

func readCommitFile(root string, file File) ([]byte, error) {
	if (file.Mode != "100644" && file.Mode != "100755") || !commitSHA.MatchString(file.BlobSHA) {
		return nil, fmt.Errorf("dependency input is not a regular Git blob")
	}
	return commitGit(root, "cat-file", "blob", file.BlobSHA)
}

func commitGit(root string, args ...string) ([]byte, error) {
	if root == "" {
		root = "."
	}
	command := exec.Command("git", append([]string{"-C", root}, args...)...)
	command.Env = append(os.Environ(), "GIT_NO_REPLACE_OBJECTS=1", "GIT_TERMINAL_PROMPT=0")
	return command.Output()
}
