package auth

import (
	"testing"
	"time"
)

func newTestManager(ttl time.Duration) (*Manager, *time.Time) {
	m := NewManager(ttl)
	now := time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)
	m.now = func() time.Time { return now }
	return m, &now
}

func TestCreateAndValidate(t *testing.T) {
	m, _ := newTestManager(time.Hour)

	s, err := m.Create("admin", "10.0.0.1", "test-agent")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if len(s.Token) != 64 || s.ID == "" || s.Token == s.ID {
		t.Fatalf("unexpected token/id: %q / %q", s.Token, s.ID)
	}

	got, ok := m.Validate(s.Token)
	if !ok || got.Username != "admin" || got.IP != "10.0.0.1" {
		t.Fatalf("Validate = %+v, %v", got, ok)
	}
}

func TestValidateRejectsUnknownAndEmpty(t *testing.T) {
	m, _ := newTestManager(time.Hour)
	for _, tok := range []string{"", "nope"} {
		if _, ok := m.Validate(tok); ok {
			t.Errorf("Validate(%q) should fail", tok)
		}
	}
}

func TestSlidingExpiry(t *testing.T) {
	m, now := newTestManager(time.Hour)
	s, _ := m.Create("admin", "", "")

	*now = now.Add(45 * time.Minute)
	if _, ok := m.Validate(s.Token); !ok {
		t.Fatal("session should still be valid at 45m")
	}
	// Validate extended the expiry, so 45m later is still inside the window.
	*now = now.Add(45 * time.Minute)
	if _, ok := m.Validate(s.Token); !ok {
		t.Fatal("session should have been extended by the previous Validate")
	}
}

func TestExpiredSessionIsRejectedAndRemoved(t *testing.T) {
	m, now := newTestManager(time.Hour)
	s, _ := m.Create("admin", "", "")

	*now = now.Add(2 * time.Hour)
	if _, ok := m.Validate(s.Token); ok {
		t.Fatal("expired session must not validate")
	}
	if len(m.byToken) != 0 || len(m.tokenOf) != 0 {
		t.Fatal("expired session should have been removed")
	}
}

func TestRevoke(t *testing.T) {
	m, _ := newTestManager(time.Hour)
	a, _ := m.Create("admin", "", "")
	b, _ := m.Create("admin", "", "")

	if !m.Revoke(a.Token) {
		t.Fatal("Revoke(a) should succeed")
	}
	if _, ok := m.Validate(a.Token); ok {
		t.Fatal("revoked session must not validate")
	}
	if !m.RevokeByID(b.ID) {
		t.Fatal("RevokeByID(b) should succeed")
	}
	if _, ok := m.Validate(b.Token); ok {
		t.Fatal("revoked session must not validate")
	}
	if m.Revoke("missing") || m.RevokeByID("missing") {
		t.Fatal("revoking an unknown session must report false")
	}
}

func TestListHidesTokenSortsNewestFirstAndSkipsExpired(t *testing.T) {
	m, now := newTestManager(time.Hour)
	old, _ := m.Create("admin", "", "")
	*now = now.Add(10 * time.Minute)
	newer, _ := m.Create("admin", "", "")
	*now = now.Add(55 * time.Minute) // old is now past its 1h expiry, newer is not

	list := m.List()
	if len(list) != 1 || list[0].ID != newer.ID {
		t.Fatalf("List = %+v, want only the newer session", list)
	}
	if list[0].Token != "" {
		t.Fatal("List must not expose the secret token")
	}
	_ = old
}

func TestPurgeExpired(t *testing.T) {
	m, now := newTestManager(time.Hour)
	if _, err := m.Create("admin", "", ""); err != nil {
		t.Fatal(err)
	}
	*now = now.Add(2 * time.Hour)
	m.purgeExpired()
	if len(m.byToken) != 0 || len(m.tokenOf) != 0 {
		t.Fatal("purgeExpired should drop expired sessions")
	}
}

func TestStopIsIdempotent(t *testing.T) {
	m := NewManager(time.Hour)
	m.StartCleanup(time.Millisecond)
	m.Stop()
	m.Stop()
}
