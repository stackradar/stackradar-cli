package upload

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
)

// PullRequestContext is supplied by the trusted reusable workflow, not by OIDC.
// The server validates the workflow identity before trusting the PR head and history.
type PullRequestContext struct {
	Number           int                 `json:"number"`
	HeadSHA          string              `json:"head_sha"`
	BaseSHA          string              `json:"base_sha"`
	HeadRepositoryID string              `json:"head_repository_id"`
	ChangedFiles     []ChangedFile       `json:"changed_files"`
	Collection       *CollectionCoverage `json:"collection,omitempty"`
}

var commitSHA = regexp.MustCompile(`^[a-f0-9]{40}$`)
var repositoryID = regexp.MustCompile(`^[1-9][0-9]*$`)

func ReadPullRequestContext(path string) (*PullRequestContext, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = file.Close()
	}()
	decoder := json.NewDecoder(io.LimitReader(file, 4194305))
	decoder.DisallowUnknownFields()
	var context PullRequestContext
	if err := decoder.Decode(&context); err != nil {
		return nil, fmt.Errorf("read PR context: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return nil, fmt.Errorf("PR context must contain exactly one JSON object")
	}
	if err := context.Validate(); err != nil {
		return nil, err
	}
	return &context, nil
}

func (context *PullRequestContext) Validate() error {
	if context.Number < 1 || !commitSHA.MatchString(context.HeadSHA) || !commitSHA.MatchString(context.BaseSHA) || !repositoryID.MatchString(context.HeadRepositoryID) {
		return fmt.Errorf("PR context has invalid repository, PR number, or commit identifiers")
	}
	if context.ChangedFiles == nil || len(context.ChangedFiles) > 10000 {
		return fmt.Errorf("PR context must include changed files")
	}
	for _, file := range context.ChangedFiles {
		if file.Path == "" {
			return fmt.Errorf("changed file path is required")
		}
		switch file.Status {
		case "added", "modified", "removed", "renamed":
		default:
			return fmt.Errorf("invalid changed file status")
		}
		if file.Status == "renamed" && file.PreviousPath == "" {
			return fmt.Errorf("renamed file requires previous path")
		}
	}
	return nil
}

type ChangedFile struct {
	Path         string `json:"path"`
	Status       string `json:"status"`
	PreviousPath string `json:"previous_path,omitempty"`
}

type CollectionCoverage struct {
	Complete bool     `json:"complete"`
	Paths    []string `json:"paths"`
	Errors   []string `json:"errors"`
}
