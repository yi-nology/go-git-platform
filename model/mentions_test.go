package model

import (
	"slices"
	"testing"
)

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
