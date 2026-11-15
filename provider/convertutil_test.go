package provider

import (
	"testing"
)

func TestSplitFullName(t *testing.T) {
	tests := []struct {
		input     string
		wantOwner string
		wantName  string
	}{
		{"owner/repo", "owner", "repo"},
		{"org/sub/repo", "org", "sub/repo"},
		{"repo-only", "", "repo-only"},
		{"", "", ""},
	}
	for _, tt := range tests {
		owner, name := SplitFullName(tt.input)
		if owner != tt.wantOwner {
			t.Errorf("SplitFullName(%q) owner = %q, want %q", tt.input, owner, tt.wantOwner)
		}
		if name != tt.wantName {
			t.Errorf("SplitFullName(%q) name = %q, want %q", tt.input, name, tt.wantName)
		}
	}
}

func TestExtractOwnerFromFullName(t *testing.T) {
	tests := []struct {
		input  string
		expect string
	}{
		{"owner/repo", "owner"},
		{"repo-only", ""},
		{"", ""},
	}
	for _, tt := range tests {
		result := ExtractOwnerFromFullName(tt.input)
		if result != tt.expect {
			t.Errorf("ExtractOwnerFromFullName(%q) = %q, want %q", tt.input, result, tt.expect)
		}
	}
}

func TestBuildEventRepo(t *testing.T) {
	er := BuildEventRepo("owner/repo")
	if er.FullName != "owner/repo" {
		t.Fatalf("expected owner/repo, got %q", er.FullName)
	}
	if er.Owner != "owner" {
		t.Fatalf("expected owner, got %q", er.Owner)
	}
	if er.Name != "repo" {
		t.Fatalf("expected repo, got %q", er.Name)
	}
}

func TestBuildEventRepo_NoSlash(t *testing.T) {
	er := BuildEventRepo("repo-only")
	if er.FullName != "repo-only" {
		t.Fatalf("expected repo-only, got %q", er.FullName)
	}
	if er.Owner != "" {
		t.Fatalf("expected empty owner, got %q", er.Owner)
	}
	if er.Name != "repo-only" {
		t.Fatalf("expected repo-only, got %q", er.Name)
	}
}

func TestResolveMRSHAs(t *testing.T) {
	tests := []struct {
		name                          string
		diffHead, diffBase, diffStart string
		mergeCommit, lastCommit       string
		wantHead, wantBase, wantStart string
	}{
		{
			name:     "all present: merge_commit wins for base, diff_refs head wins",
			diffHead: "hSHA", diffBase: "bSHA", diffStart: "sSHA",
			mergeCommit: "mcSHA", lastCommit: "abc",
			wantHead: "hSHA", wantBase: "mcSHA", wantStart: "sSHA",
		},
		{
			name:     "no merge_commit: base falls back to diff_refs.base_sha",
			diffHead: "hSHA", diffBase: "bSHA", diffStart: "sSHA",
			mergeCommit: "", lastCommit: "abc",
			wantHead: "hSHA", wantBase: "bSHA", wantStart: "sSHA",
		},
		{
			name:     "no diff_refs: head falls back to last_commit, base/start empty",
			diffHead: "", diffBase: "", diffStart: "",
			mergeCommit: "", lastCommit: "abcOnly",
			wantHead: "abcOnly", wantBase: "", wantStart: "",
		},
		{
			name:     "merged without diff_refs: merge_commit used as base",
			diffHead: "", diffBase: "", diffStart: "",
			mergeCommit: "mcSHA", lastCommit: "abc",
			wantHead: "abc", wantBase: "mcSHA", wantStart: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			head, base, start := ResolveMRSHAs(tt.diffHead, tt.diffBase, tt.diffStart, tt.mergeCommit, tt.lastCommit)
			if head != tt.wantHead {
				t.Errorf("head = %q, want %q", head, tt.wantHead)
			}
			if base != tt.wantBase {
				t.Errorf("base = %q, want %q", base, tt.wantBase)
			}
			if start != tt.wantStart {
				t.Errorf("start = %q, want %q", start, tt.wantStart)
			}
		})
	}
}
