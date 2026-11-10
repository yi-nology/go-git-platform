package giteafamily

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/yi-nology/go-git-platform/provider"
)

type fakeNotifyPort struct {
	markAll     MarkNotificationOptions
	markAllCnt  int
	markRepo    MarkNotificationOptions
	markRepoCnt int
}

func (f *fakeNotifyPort) ListNotifications(context.Context, ListNotificationOptions) ([]*NotificationThread, error) {
	return nil, nil
}
func (f *fakeNotifyPort) ListRepoNotifications(context.Context, string, string, ListNotificationOptions) ([]*NotificationThread, error) {
	return nil, nil
}
func (f *fakeNotifyPort) MarkNotificationReadByID(context.Context, int64) error { return nil }
func (f *fakeNotifyPort) MarkNotificationsRead(_ context.Context, opt MarkNotificationOptions) error {
	f.markAllCnt++
	f.markAll = opt
	return nil
}
func (f *fakeNotifyPort) MarkRepoNotificationsRead(_ context.Context, _ string, _ string, opt MarkNotificationOptions) error {
	f.markRepoCnt++
	f.markRepo = opt
	return nil
}

// TestMarkNotificationsRead_LastReadAtValidated pins the timestamp gate
// (issue #97, the #52/#75 family's third site): a malformed LastReadAt used
// to be swallowed by `_ =` into the zero time and sent on the wire — the
// mark range silently widened to whatever the zero value means. Now every
// invalid value fails the call with ErrInvalidInput before any request, and
// a valid RFC3339 value lands on the wire unchanged.
func TestMarkNotificationsRead_LastReadAtValidated(t *testing.T) {
	cases := []struct {
		name       string
		lastReadAt string
		wantErr    bool
		wantWire   string // RFC3339 the port must see; "" = zero time expected
	}{
		{name: "empty passes through as mark-all", lastReadAt: "", wantWire: ""},
		{name: "valid RFC3339 lands on the wire", lastReadAt: "2026-01-02T03:04:05Z", wantWire: "2026-01-02T03:04:05Z"},
		{name: "date-only is rejected", lastReadAt: "2026/11/11", wantErr: true},
		{name: "loose layout is rejected", lastReadAt: "yesterday", wantErr: true},
	}
	for _, tc := range cases {
		for _, scope := range []string{"all", "repo"} {
			t.Run(tc.name+"/"+scope, func(t *testing.T) {
				fam := Family{Platform: provider.PlatformGitea}
				port := &fakeNotifyPort{}
				opts := provider.MarkNotificationsOptions{LastReadAt: tc.lastReadAt}

				var err error
				if scope == "all" {
					err = MarkNotificationsRead(context.Background(), fam, port, opts)
				} else {
					err = MarkRepoNotificationsRead(context.Background(), fam, port, "o", "r", opts)
				}
				if tc.wantErr {
					if err == nil {
						t.Fatal("malformed LastReadAt must fail the call, got nil")
					}
					if !errors.Is(err, provider.ErrInvalidInput) {
						t.Fatalf("want ErrInvalidInput chain, got %v", err)
					}
					if scope == "all" && port.markAllCnt != 0 || scope == "repo" && port.markRepoCnt != 0 {
						t.Fatal("rejection must happen before any wire call")
					}
					return
				}
				if err != nil {
					t.Fatalf("valid LastReadAt %q must pass, got %v", tc.lastReadAt, err)
				}
				got := port.markAll
				if scope == "repo" {
					got = port.markRepo
				}
				if tc.wantWire == "" {
					if !got.LastReadAt.IsZero() {
						t.Fatalf("empty LastReadAt must keep the zero time, got %v", got.LastReadAt)
					}
					return
				}
				want, _ := time.Parse(time.RFC3339, tc.wantWire)
				if !got.LastReadAt.Equal(want) {
					t.Fatalf("wire LastReadAt = %v, want %v", got.LastReadAt, want)
				}
			})
		}
	}
}
