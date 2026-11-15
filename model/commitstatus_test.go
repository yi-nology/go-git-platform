package model

import (
	"testing"
)

func TestNormalizeCommitStatusState(t *testing.T) {
	tests := []struct {
		in   string
		want CommitStatusState
	}{
		{"success", CommitStatusSuccess},
		{"SUCCESS", CommitStatusSuccess},
		{"warning", CommitStatusSuccess},
		{"neutral", CommitStatusSuccess},
		{"failure", CommitStatusFailure},
		{"failed", CommitStatusFailure},
		{"error", CommitStatusError},
		{"canceled", CommitStatusCanceled},
		{"cancelled", CommitStatusCanceled},
		{"running", CommitStatusRunning},
		{"created", CommitStatusRunning},
		{"waiting_for_resource", CommitStatusRunning},
		{"pending", CommitStatusPending},
		{"", CommitStatusPending},
		{"some-new-verb", CommitStatusPending},
	}
	for _, tt := range tests {
		if got := NormalizeCommitStatusState(tt.in); got != tt.want {
			t.Errorf("NormalizeCommitStatusState(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestCombineCommitStatusStates(t *testing.T) {
	p, r, s, e, c := CommitStatusPending, CommitStatusRunning, CommitStatusSuccess, CommitStatusError, CommitStatusCanceled
	tests := []struct {
		name   string
		states []CommitStatusState
		want   CommitStatusState
	}{
		{"empty is pending", nil, CommitStatusPending},
		{"all success", []CommitStatusState{s, s}, CommitStatusSuccess},
		{"success then pending stays pending", []CommitStatusState{s, p}, CommitStatusPending},
		{"pending then running stays pending", []CommitStatusState{p, r}, CommitStatusPending},
		{"failure dominates others", []CommitStatusState{s, c, CommitStatusFailure, p}, CommitStatusFailure},
		{"error dominates all", []CommitStatusState{s, c, p, e}, CommitStatusError},
		{"canceled dominates running", []CommitStatusState{c, p, r}, CommitStatusCanceled},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CombineCommitStatusStates(tt.states); got != tt.want {
				t.Errorf("CombineCommitStatusStates(%v) = %q, want %q", tt.states, got, tt.want)
			}
		})
	}
}
