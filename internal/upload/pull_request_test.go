package upload

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func validPullRequestContext() PullRequestContext {
	return PullRequestContext{Number: 447, HeadSHA: strings.Repeat("a", 40), BaseSHA: strings.Repeat("b", 40), HeadRepositoryID: "123", BaselineEligibleSHAs: []string{strings.Repeat("b", 40)}}
}

func TestReadPullRequestContext(t *testing.T) {
	context := validPullRequestContext()
	valid, _ := json.Marshal(context)
	for name, contents := range map[string]string{
		"valid":           string(valid),
		"trailing object": string(valid) + "{}",
		"unknown field":   strings.Replace(string(valid), "{", `{"unexpected":true,`, 1),
		"invalid head":    strings.Replace(string(valid), context.HeadSHA, "invalid", 1),
		"no baseline":     strings.Replace(string(valid), `[`+`"`+context.BaseSHA+`"`+`]`, "[]", 1),
		"null":            "null",
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "context.json")
			if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
				t.Fatal(err)
			}
			got, err := ReadPullRequestContext(path)
			if name == "valid" {
				if err != nil || got.HeadSHA != context.HeadSHA {
					t.Fatalf("context = %v, error = %v", got, err)
				}
			} else if err == nil {
				t.Fatal("expected invalid context to fail")
			}
		})
	}
	context.BaselineEligibleSHAs = append(context.BaselineEligibleSHAs, context.BaseSHA)
	if context.Validate() == nil {
		t.Fatal("duplicate baseline was accepted")
	}
}

func TestInitializePullRequestEvidenceRequiresCleanHeadAndSendsContext(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "package.json", "{}\n")
	runGit(t, root, "init")
	runGit(t, root, "config", "user.email", "tests@stackradar.com")
	runGit(t, root, "config", "user.name", "StackRadar Tests")
	runGit(t, root, "add", ".")
	runGit(t, root, "commit", "-m", "Fixture")
	bundle, err := BuildBundle(root, []File{{Path: "package.json", Ecosystem: "npm"}})
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := readManifestSummaryFromBundle(bundle.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	context := validPullRequestContext()
	context.HeadSHA = *manifest.Git.CommitSHA
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var payload initializeUploadPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if payload.Purpose != "pull_request" || payload.PullRequest == nil || payload.PullRequest.HeadSHA != context.HeadSHA || payload.PullRequest.Number != 447 {
			t.Fatalf("unexpected payload: %+v", payload)
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"upload_id":"run-123","upload":{"method":"PUT","url":"https://example.test/upload"}}`))
	}))
	defer server.Close()
	options := UploadOptions{APIURL: server.URL, PullRequest: &context}
	if _, err := initializeUpload(server.Client(), options, bundle.Bytes); err != nil {
		t.Fatal(err)
	}
	context.HeadSHA = strings.Repeat("c", 40)
	if _, err := initializeUpload(server.Client(), options, bundle.Bytes); err == nil {
		t.Fatal("mismatched head accepted")
	}
	context.HeadSHA = *manifest.Git.CommitSHA
	writeTestFile(t, root, "package.json", "{\"changed\":true}\n")
	dirty, err := BuildBundle(root, []File{{Path: "package.json", Ecosystem: "npm"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := initializeUpload(server.Client(), options, dirty.Bytes); err == nil {
		t.Fatal("dirty head accepted")
	}
	if calls != 1 {
		t.Fatalf("unexpected network calls: %d", calls)
	}
}
