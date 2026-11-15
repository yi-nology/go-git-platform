package runtime

import "testing"

func TestNormalizeCRAction(t *testing.T) {
	tests := []struct {
		action string
		merged bool
		want   string
	}{
		// GitHub-style actions.
		{"opened", false, CRActionOpened},
		{"closed", false, CRActionClosed},
		{"closed", true, CRActionMerged},
		{"reopened", false, CRActionReopened},
		{"synchronize", false, CRActionUpdated},
		{"edited", false, CRActionUpdated},

		// GitLab-style actions.
		{"open", false, CRActionOpened},
		{"close", false, CRActionClosed},
		{"close", true, CRActionMerged},
		{"merge", false, CRActionMerged},
		{"reopen", false, CRActionReopened},
		{"update", false, CRActionUpdated},

		// Direct merged variants (always merged regardless of flag).
		{"merged", false, CRActionMerged},
		{"merged", true, CRActionMerged},

		// Sync alias.
		{"sync", false, CRActionUpdated},

		// Edit alias.
		{"edit", false, CRActionUpdated},

		// Case insensitivity.
		{"Opened", false, CRActionOpened},
		{"CLOSED", false, CRActionClosed},
		{"CLOSED", true, CRActionMerged},
		{"REOPENED", false, CRActionReopened},
		{"SYNCHRONIZE", false, CRActionUpdated},

		// Unknown action passes through unchanged.
		{"unknown", false, "unknown"},
		{"labeled", false, "labeled"},
	}
	for _, tt := range tests {
		got := NormalizeCRAction(tt.action, tt.merged)
		if got != tt.want {
			t.Errorf("NormalizeCRAction(%q, %v) = %q, want %q", tt.action, tt.merged, got, tt.want)
		}
	}
}

func TestNormalizeTagAction(t *testing.T) {
	tests := []struct {
		action string
		want   string
	}{
		// Standard mappings.
		{"push", "created"},
		{"pushed", "created"},
		{"created", "created"},
		{"create", "created"},

		// Case insensitivity.
		{"PUSH", "created"},
		{"Pushed", "created"},
		{"CREATED", "created"},
		{"Create", "created"},

		// Unknown action passes through unchanged.
		{"deleted", "deleted"},
		{"unknown", "unknown"},
	}
	for _, tt := range tests {
		got := NormalizeTagAction(tt.action)
		if got != tt.want {
			t.Errorf("NormalizeTagAction(%q) = %q, want %q", tt.action, got, tt.want)
		}
	}
}

func TestNormalizeBranchAction(t *testing.T) {
	tests := []struct {
		action string
		want   string
	}{
		// Standard mappings.
		{"created", "created"},
		{"create", "created"},
		{"deleted", "deleted"},
		{"delete", "deleted"},

		// Case insensitivity.
		{"CREATED", "created"},
		{"Create", "created"},
		{"DELETED", "deleted"},
		{"Delete", "deleted"},

		// Unknown action passes through unchanged.
		{"push", "push"},
		{"unknown", "unknown"},
	}
	for _, tt := range tests {
		got := NormalizeBranchAction(tt.action)
		if got != tt.want {
			t.Errorf("NormalizeBranchAction(%q) = %q, want %q", tt.action, got, tt.want)
		}
	}
}

func TestNormalizeIssueAction(t *testing.T) {
	tests := []struct {
		action string
		want   string
	}{
		// Standard mappings.
		{"opened", "opened"},
		{"open", "opened"},
		{"closed", "closed"},
		{"close", "closed"},
		{"reopened", "reopened"},
		{"reopen", "reopened"},
		{"edited", "updated"},
		{"edit", "updated"},
		{"updated", "updated"},
		{"update", "updated"},

		// Case insensitivity.
		{"OPENED", "opened"},
		{"Open", "opened"},
		{"CLOSED", "closed"},
		{"Close", "closed"},
		{"REOPENED", "reopened"},
		{"Reopen", "reopened"},
		{"EDITED", "updated"},
		{"Edit", "updated"},
		{"UPDATED", "updated"},
		{"Update", "updated"},

		// Unknown action passes through unchanged.
		{"labeled", "labeled"},
		{"unknown", "unknown"},
	}
	for _, tt := range tests {
		got := NormalizeIssueAction(tt.action)
		if got != tt.want {
			t.Errorf("NormalizeIssueAction(%q) = %q, want %q", tt.action, got, tt.want)
		}
	}
}
