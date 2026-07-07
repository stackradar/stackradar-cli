package upload

import (
	"bufio"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

var ErrNoDependencyFiles = errors.New("no supported dependency files found")

type DiscoverOptions struct {
	Root     string
	Excludes []string
}

type Discovery struct {
	Files              []File
	SkippedDirectories []SkippedDirectory
}

type File struct {
	Path      string
	Ecosystem string
	SizeBytes int64
}

type SkippedDirectory struct {
	Path   string
	Reason string
}

var ignoredDirectoryNames = map[string]struct{}{
	".git":             {},
	".hg":              {},
	".pnpm":            {},
	".svn":             {},
	".yarn":            {},
	"__fixtures__":     {},
	"bower_components": {},
	"fixtures":         {},
	"jspm_packages":    {},
	"node_modules":     {},
	"test":             {},
	"tests":            {},
	"vendor":           {},
}

var supportedFileEcosystems = map[string]string{
	"Pipfile":              "python",
	"Pipfile.lock":         "python",
	"composer.json":        "composer",
	"composer.lock":        "composer",
	"deno.lock":            "npm",
	"npm-shrinkwrap.json":  "npm",
	"package-lock.json":    "npm",
	"package.json":         "npm",
	"pnpm-lock.yaml":       "npm",
	"poetry.lock":          "python",
	"pyproject.toml":       "python",
	"requirements-dev.txt": "python",
	"requirements.txt":     "python",
	"uv.lock":              "python",
	"yarn.lock":            "npm",
}

func Discover(options DiscoverOptions) (Discovery, error) {
	root := options.Root
	if root == "" {
		root = "."
	}

	root = filepath.Clean(root)

	if err := validateExcludePatterns(options.Excludes); err != nil {
		return Discovery{}, err
	}

	gitignore, err := loadRootGitignore(root)
	if err != nil {
		return Discovery{}, err
	}

	var discovery Discovery

	err = filepath.WalkDir(root, func(currentPath string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		relativePath, err := filepath.Rel(root, currentPath)
		if err != nil {
			return err
		}

		if relativePath == "." {
			return nil
		}

		relativePath = filepath.ToSlash(relativePath)

		if entry.Type()&os.ModeSymlink != 0 {
			if entry.IsDir() {
				discovery.SkippedDirectories = append(discovery.SkippedDirectories, SkippedDirectory{
					Path:   relativePath,
					Reason: "symlink",
				})
			}

			return nil
		}

		if entry.IsDir() {
			if _, ignored := ignoredDirectoryNames[entry.Name()]; ignored {
				discovery.SkippedDirectories = append(discovery.SkippedDirectories, SkippedDirectory{
					Path:   relativePath,
					Reason: "ignored directory",
				})

				return filepath.SkipDir
			}

			if matchesExclude(relativePath, options.Excludes) {
				discovery.SkippedDirectories = append(discovery.SkippedDirectories, SkippedDirectory{
					Path:   relativePath,
					Reason: "excluded",
				})

				return filepath.SkipDir
			}

			if gitignore.matches(relativePath, true) {
				discovery.SkippedDirectories = append(discovery.SkippedDirectories, SkippedDirectory{
					Path:   relativePath,
					Reason: "gitignored",
				})

				return filepath.SkipDir
			}

			return nil
		}

		if matchesExclude(relativePath, options.Excludes) {
			return nil
		}

		if gitignore.matches(relativePath, false) {
			return nil
		}

		ecosystem, supported := supportedFileEcosystems[entry.Name()]
		if !supported {
			return nil
		}

		info, err := entry.Info()
		if err != nil {
			return err
		}

		discovery.Files = append(discovery.Files, File{
			Path:      relativePath,
			Ecosystem: ecosystem,
			SizeBytes: info.Size(),
		})

		return nil
	})

	if err != nil {
		return Discovery{}, err
	}

	sort.Slice(discovery.Files, func(left int, right int) bool {
		return discovery.Files[left].Path < discovery.Files[right].Path
	})
	sort.Slice(discovery.SkippedDirectories, func(left int, right int) bool {
		return discovery.SkippedDirectories[left].Path < discovery.SkippedDirectories[right].Path
	})

	if len(discovery.Files) == 0 {
		return discovery, ErrNoDependencyFiles
	}

	return discovery, nil
}

func validateExcludePatterns(patterns []string) error {
	for _, pattern := range patterns {
		normalized := normalizeExcludePattern(pattern)
		if normalized == "" || strings.HasSuffix(normalized, "/**") {
			continue
		}

		if _, err := path.Match(normalized, "placeholder"); err != nil {
			return fmt.Errorf("invalid exclude pattern %q: %w", pattern, err)
		}
	}

	return nil
}

func matchesExclude(relativePath string, patterns []string) bool {
	for _, pattern := range patterns {
		normalized := normalizeExcludePattern(pattern)
		if normalized == "" {
			continue
		}

		if strings.HasSuffix(normalized, "/**") {
			prefix := strings.TrimSuffix(normalized, "/**")
			if relativePath == prefix || strings.HasPrefix(relativePath, prefix+"/") {
				return true
			}
		}

		if matched, _ := path.Match(normalized, relativePath); matched {
			return true
		}

		if !strings.Contains(normalized, "/") {
			if matched, _ := path.Match(normalized, path.Base(relativePath)); matched {
				return true
			}
		}
	}

	return false
}

func normalizeExcludePattern(pattern string) string {
	normalized := filepath.ToSlash(strings.TrimSpace(pattern))
	normalized = strings.TrimPrefix(normalized, "./")

	return strings.TrimPrefix(normalized, "/")
}

type gitignoreMatcher struct {
	rules []gitignoreRule
}

type gitignoreRule struct {
	pattern       string
	negative      bool
	directoryOnly bool
	anchored      bool
	hasSlash      bool
}

func loadRootGitignore(root string) (gitignoreMatcher, error) {
	file, err := os.Open(filepath.Join(root, ".gitignore"))
	if errors.Is(err, os.ErrNotExist) {
		return gitignoreMatcher{}, nil
	}

	if err != nil {
		return gitignoreMatcher{}, err
	}

	var matcher gitignoreMatcher
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		rule, ok := parseGitignoreRule(scanner.Text())
		if ok {
			matcher.rules = append(matcher.rules, rule)
		}
	}

	if err := scanner.Err(); err != nil {
		if closeErr := file.Close(); closeErr != nil {
			return gitignoreMatcher{}, fmt.Errorf("%w; close .gitignore: %v", err, closeErr)
		}

		return gitignoreMatcher{}, err
	}

	if err := file.Close(); err != nil {
		return gitignoreMatcher{}, err
	}

	return matcher, nil
}

func parseGitignoreRule(line string) (gitignoreRule, bool) {
	pattern := strings.TrimSpace(line)
	if pattern == "" || strings.HasPrefix(pattern, "#") {
		return gitignoreRule{}, false
	}

	negative := false
	if strings.HasPrefix(pattern, "!") {
		negative = true
		pattern = strings.TrimPrefix(pattern, "!")
	}

	pattern = strings.TrimSpace(pattern)
	pattern = filepath.ToSlash(pattern)
	pattern = strings.TrimPrefix(pattern, "./")

	if pattern == "" {
		return gitignoreRule{}, false
	}

	anchored := strings.HasPrefix(pattern, "/")
	pattern = strings.TrimPrefix(pattern, "/")
	directoryOnly := strings.HasSuffix(pattern, "/")
	pattern = strings.TrimSuffix(pattern, "/")

	if pattern == "" {
		return gitignoreRule{}, false
	}

	return gitignoreRule{
		pattern:       pattern,
		negative:      negative,
		directoryOnly: directoryOnly,
		anchored:      anchored,
		hasSlash:      strings.Contains(pattern, "/"),
	}, true
}

func (matcher gitignoreMatcher) matches(relativePath string, isDirectory bool) bool {
	ignored := false

	for _, rule := range matcher.rules {
		if rule.matches(relativePath, isDirectory) {
			ignored = !rule.negative
		}
	}

	return ignored
}

func (rule gitignoreRule) matches(relativePath string, isDirectory bool) bool {
	if rule.directoryOnly && !isDirectory && !strings.HasPrefix(relativePath, rule.pattern+"/") {
		return false
	}

	if rule.anchored {
		return matchesGitignorePattern(rule.pattern, relativePath, rule.directoryOnly)
	}

	if rule.hasSlash {
		if matchesGitignorePattern(rule.pattern, relativePath, rule.directoryOnly) {
			return true
		}

		return matchesGitignorePattern("*/"+rule.pattern, relativePath, rule.directoryOnly)
	}

	segments := strings.Split(relativePath, "/")
	for index, segment := range segments {
		if matched, _ := path.Match(rule.pattern, segment); matched {
			return isDirectory || index < len(segments)-1 || !rule.directoryOnly
		}
	}

	return false
}

func matchesGitignorePattern(pattern string, relativePath string, directoryOnly bool) bool {
	if matched, _ := path.Match(pattern, relativePath); matched {
		return true
	}

	return directoryOnly && strings.HasPrefix(relativePath, pattern+"/")
}
