package config

import "testing"

func TestIsPlaceholder(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"CHANGEME_USE_openssl_rand_hex_32", true},
		{"CHANGEME_USE_A_STRONG_PASSWORD", true},
		{"changeme", true},
		{"  ChangeMe-please", true},
		{"", false},
		{"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", false},
		{"my-changeme-password", false}, // only a leading placeholder counts
	}
	for _, c := range cases {
		if got := IsPlaceholder(c.in); got != c.want {
			t.Errorf("IsPlaceholder(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestWebUISessionTTLDefaultAndOverride(t *testing.T) {
	t.Setenv("WEB_UI_SESSION_TTL", "")
	if got := NewConfig().WebUISessionTTL.Hours(); got != 24 {
		t.Errorf("default TTL = %vh, want 24h", got)
	}
	t.Setenv("WEB_UI_SESSION_TTL", "30m")
	if got := NewConfig().WebUISessionTTL.Minutes(); got != 30 {
		t.Errorf("TTL = %vm, want 30m", got)
	}
	t.Setenv("WEB_UI_SESSION_TTL", "garbage")
	if got := NewConfig().WebUISessionTTL.Hours(); got != 24 {
		t.Errorf("invalid TTL should fall back to 24h, got %vh", got)
	}
}
