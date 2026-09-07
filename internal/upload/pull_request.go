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
	Number               int      `json:"number"`
	HeadSHA              string   `json:"head_sha"`
	BaseSHA              string   `json:"base_sha"`
	HeadRepositoryID     string   `json:"head_repository_id"`
	BaselineEligibleSHAs []string `json:"baseline_eligible_shas"`
}

var commitSHA = regexp.MustCompile(`^[a-f0-9]{40}$`)
var repositoryID = regexp.MustCompile(`^[1-9][0-9]*$`)

func ReadPullRequestContext(path string) (*PullRequestContext, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	decoder := json.NewDecoder(io.LimitReader(file, 65537))
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
	if len(context.BaselineEligibleSHAs) < 1 || len(context.BaselineEligibleSHAs) > 1000 {
		return fmt.Errorf("PR context must include between 1 and 1000 eligible baseline commits")
	}
	seen := make(map[string]bool)
	for _, sha := range context.BaselineEligibleSHAs {
		if !commitSHA.MatchString(sha) || seen[sha] {
			return fmt.Errorf("PR context contains invalid or duplicate baseline commits")
		}
		seen[sha] = true
	}
	return nil
}
