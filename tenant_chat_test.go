package main

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

type fakeRealtime struct {
	publishes int
}

func (f *fakeRealtime) CreateChannel(context.Context, string, string) error { return nil }
func (f *fakeRealtime) IssueToken(context.Context, string, string, string) (json.RawMessage, error) {
	return json.RawMessage(`{"value":"client-token"}`), nil
}
func (f *fakeRealtime) Publish(context.Context, string, string, string, any, string) (json.RawMessage, error) {
	f.publishes++
	return json.RawMessage(`{"published":true}`), nil
}

func TestAnnouncementPolicy(t *testing.T) {
	tests := []struct {
		name          string
		state         AccountState
		wantErr       error
		wantPublishes int
	}{
		{name: "active account publishes", state: AccountActive, wantPublishes: 1},
		{name: "suspended account is blocked", state: AccountSuspended, wantErr: ErrAccountSuspended},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := &fakeRealtime{}
			chat := NewTenantChat(api)
			if err := chat.SetAccountState("acme", tt.state); err != nil {
				t.Fatal(err)
			}
			_, err := chat.Announce(context.Background(), "acme", "notice", map[string]string{"text": "Maintenance at 18:00 UTC"}, "req-1")
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Announce() error = %v, want %v", err, tt.wantErr)
			}
			if api.publishes != tt.wantPublishes {
				t.Fatalf("publish calls = %d, want %d", api.publishes, tt.wantPublishes)
			}
		})
	}
}

func TestAnnouncementMissingAccountIsBlocked(t *testing.T) {
	api := &fakeRealtime{}
	chat := NewTenantChat(api)
	_, err := chat.Announce(context.Background(), "missing", "notice", nil, "req-1")
	if !errors.Is(err, ErrAccountNotFound) {
		t.Fatalf("Announce() error = %v, want %v", err, ErrAccountNotFound)
	}
	if api.publishes != 0 {
		t.Fatalf("publish calls = %d, want 0", api.publishes)
	}
}
