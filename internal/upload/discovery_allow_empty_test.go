package upload

import "testing"

func TestDiscoverAllowsEmptyDependencyEvidence(t *testing.T) {
	discovery, err := Discover(DiscoverOptions{Root: t.TempDir(), AllowEmpty: true})
	if err != nil {
		t.Fatalf("discover empty evidence: %v", err)
	}

	if len(discovery.Files) != 0 {
		t.Fatalf("files = %#v, want no files", discovery.Files)
	}
}
