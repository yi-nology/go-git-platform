package provider

import (
	"testing"

	"github.com/yi-nology/go-git-platform/model"
)

func TestMapMRStateToCR(t *testing.T) {
	tests := []struct {
		input    string
		expected model.CRState
	}{
		{"opened", model.CRStateOpened},
		{"open", model.CRStateOpened},
		{"merged", model.CRStateMerged},
		{"closed", model.CRStateClosed},
		{"unknown", model.CRStateOpened},
	}
	for _, tt := range tests {
		result := MapMRStateToCR(tt.input)
		if result != tt.expected {
			t.Errorf("MapMRStateToCR(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestMapBoolStateToCR(t *testing.T) {
	tests := []struct {
		state    string
		merged   bool
		expected model.CRState
	}{
		{"closed", true, model.CRStateMerged},
		{"closed", false, model.CRStateClosed},
		{"opened", false, model.CRStateOpened},
	}
	for _, tt := range tests {
		result := MapBoolStateToCR(tt.state, tt.merged)
		if result != tt.expected {
			t.Errorf("MapBoolStateToCR(%q, %v) = %q, want %q", tt.state, tt.merged, result, tt.expected)
		}
	}
}

func TestMapStateToCR_NilFn(t *testing.T) {
	result := MapStateToCR("merged", nil)
	if result != model.CRStateMerged {
		t.Fatalf("expected CRStateMerged, got %q", result)
	}
}
