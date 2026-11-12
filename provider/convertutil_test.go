package provider

import (
	"slices"
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

func TestExtractMentions(t *testing.T) {
	tests := []struct {
		name string
		body string
		want []string
	}{
		{"single", "cc @alice", []string{"alice"}},
		{"multiple", "@alice please review @bob", []string{"alice", "bob"}},
		{"dedup", "@alice and @alice again", []string{"alice"}},
		{"email excluded", "send to user@example.com not a mention", nil},
		{"mixed", "email user@host.com and @charlie", []string{"charlie"}},
		{"hyphen", "@user-name works", []string{"user-name"}},
		{"underscore", "@user_name works", []string{"user_name"}},
		{"dot", "@first.last works", []string{"first.last"}},
		{"at start of body", "@alice at the beginning", []string{"alice"}},
		{"parenthesized", "(@alice)", []string{"alice"}},
		{"markdown link URL excluded", "[click](https://example.com/@user) and @real", []string{"real"}},
		{"bare URL excluded", "see https://example.com/@user and @real", []string{"real"}},
		{"mailto excluded", "mailto:user@host.com and @real", []string{"real"}},
		{"empty", "", nil},
		{"no mentions", "nothing here", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractMentions(tt.body)
			if len(got) == 0 && len(tt.want) == 0 {
				return
			}
			if len(got) != len(tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("got[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestIssueCommentMentions(t *testing.T) {
	c := &IssueComment{Body: "cc @alice @bob"}
	m := c.Mentions()
	if len(m) != 2 || m[0] != "alice" || m[1] != "bob" {
		t.Fatalf("unexpected mentions: %v", m)
	}
}

func TestCRCommentMentions(t *testing.T) {
	c := &CRComment{Body: "@dev please look"}
	m := c.Mentions()
	if len(m) != 1 || m[0] != "dev" {
		t.Fatalf("unexpected mentions: %v", m)
	}
}

func TestReviewCommentMentions(t *testing.T) {
	c := &ReviewComment{Body: "@reviewer nit: style"}
	m := c.Mentions()
	if len(m) != 1 || m[0] != "reviewer" {
		t.Fatalf("unexpected mentions: %v", m)
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

// TestExtractMentions_TrailingPunctuation 回归（issue #145）：句末标点被吞进
// 用户名——"cc @alice." 解析出 "alice."，真实账号被漏掉。合法账号名不以
// '.'/'-' 结尾，只裁尾部、保留内部点号（@first.last 不受影响）；去重按
// 裁剪后的名字（"@alice. @alice" 是同一个人）。
func TestExtractMentions_TrailingPunctuation(t *testing.T) {
	tests := []struct {
		name string
		body string
		want []string
	}{
		{"sentence period", "cc @alice.", []string{"alice"}},
		{"sentence period mid", "ping @bob. please review", []string{"bob"}},
		{"ellipsis", "cc @carol...", []string{"carol"}},
		{"trailing hyphen", "cc @dave-", []string{"dave"}},
		{"trailing dot-hyphen mix", "cc @erin.-", []string{"erin"}},
		{"internal dot kept", "cc @first.last.", []string{"first.last"}},
		{"internal hyphen kept", "cc @user-name.", []string{"user-name"}},
		{"dedup after trim", "@alice. and @alice", []string{"alice"}},
		{"mention next to cjk period", "请看 @alice。", []string{"alice"}},
		{"bare punctuation is not a user", "hello @...", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractMentions(tt.body)
			if !slices.Equal(got, tt.want) {
				t.Fatalf("ExtractMentions(%q) = %v, want %v", tt.body, got, tt.want)
			}
		})
	}
}
