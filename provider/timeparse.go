package provider

import (
	"time"

	oekiterrors "github.com/oekit/pkg/errors"
)

// ParseTimestamp resolves one RFC3339 timestamp option (issue #52). The
// option fields carrying timestamps (ListCommitsOptions.Since/Until,
// ListNotificationsOptions.Since, MarkNotificationsOptions.LastReadAt)
// document the RFC3339 layout; an unparsable value must NOT silently widen
// the result set: a date-only string in an incremental sync used to drop the
// filter and return the full history with err == nil — the failure looked
// like "the sync was slow", never like "the filter was ignored".
//
// An empty value resolves to the zero time ("no filter requested"). Every
// backend resolves its timestamp options through this one parser, so the
// same input is accepted or rejected identically on every platform —
// including the backends that forward the raw string to the platform
// (Gitee/GitCode/TencentCode): they validate here first, then forward.
func ParseTimestamp(field, value string) (time.Time, error) {
	if value == "" {
		return time.Time{}, nil
	}
	t, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, oekiterrors.Errorf("%w: %s %q is not RFC3339: %v", ErrInvalidInput, field, value, err)
	}
	return t, nil
}
