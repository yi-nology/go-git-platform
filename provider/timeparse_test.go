package provider

import (
	"errors"
	"testing"
	"time"
)

// issue #52：时间戳选项的共享解析器——空串=不过滤、RFC3339 通过、其余一律
// ErrInvalidInput（命名选项与期望版式）。全后端经它统一接受/拒绝面。
func TestParseTimestamp(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		value   string
		wantErr bool
		want    time.Time
	}{
		{name: "empty means no filter", value: "", wantErr: false},
		{name: "RFC3339 UTC", value: "2026-01-01T00:00:00Z", wantErr: false, want: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)},
		{name: "RFC3339 with offset", value: "2026-01-01T08:30:00+08:00", wantErr: false, want: time.Date(2026, 1, 1, 8, 30, 0, 0, time.FixedZone("", 8*3600))},
		{name: "date-only is rejected", value: "2026-01-01", wantErr: true},
		{name: "slash date is rejected", value: "2026/01/01", wantErr: true},
		{name: "garbage is rejected", value: "not-a-date", wantErr: true},
		{name: "unix seconds are rejected", value: "1767225600", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseTimestamp("Since", tc.value)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("ParseTimestamp(%q) = nil error, want ErrInvalidInput", tc.value)
				}
				if !errors.Is(err, ErrInvalidInput) {
					t.Fatalf("ParseTimestamp(%q) = %v, want an error wrapping ErrInvalidInput", tc.value, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseTimestamp(%q) = %v", tc.value, err)
			}
			if !got.Equal(tc.want) {
				t.Fatalf("ParseTimestamp(%q) = %v, want %v", tc.value, got, tc.want)
			}
		})
	}
}
