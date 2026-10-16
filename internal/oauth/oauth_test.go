package oauth

import (
	"encoding/base64"
	"testing"
)

func TestProviders(t *testing.T) {
	if len(Providers()) != 2 {
		t.Fatal("expected google + microsoft")
	}
	g, ok := GetProvider("google")
	if !ok || g.IMAPHost != "imap.gmail.com" {
		t.Fatalf("google wrong: %+v", g)
	}
	if _, ok := GetProvider("nope"); ok {
		t.Fatal("unknown provider must fail")
	}
}

func TestDefaultClientIDPriority(t *testing.T) {
	if got := DefaultClientID("google", " manual "); got != "manual" {
		t.Fatalf("manual must win, got %q", got)
	}
	// env fallback (no build flags in test binary)
	t.Setenv("EMAILCLIENT_GOOGLE_CLIENT_ID", "env-id")
	if got := DefaultClientID("google", ""); got != "env-id" {
		t.Fatalf("env fallback wrong: %q", got)
	}
	if got := DefaultClientID("unknown", ""); got != "" {
		t.Fatalf("unknown must be empty, got %q", got)
	}
}

func idToken(payload string) string {
	return "header." + base64.RawURLEncoding.EncodeToString([]byte(payload)) + ".sig"
}

func TestEmailFromIDToken(t *testing.T) {
	if got := emailFromIDToken(idToken(`{"email":"User@Example.COM"}`)); got != "user@example.com" {
		t.Fatalf("got %q", got)
	}
	if got := emailFromIDToken(idToken(`{"preferred_username":"upn@corp.test"}`)); got != "upn@corp.test" {
		t.Fatalf("got %q", got)
	}
	if got := emailFromIDToken("garbage"); got != "" {
		t.Fatalf("garbage must be empty, got %q", got)
	}
	if got := emailFromIDToken(idToken(`{"email":"not-an-email"}`)); got != "" {
		t.Fatalf("non-email must be empty, got %q", got)
	}
}

func TestStartFlowValidation(t *testing.T) {
	if _, err := StartFlow("nope", "x"); err == nil {
		t.Fatal("unknown provider must error")
	}
	if _, err := StartFlow("google", ""); err == nil && DefaultClientID("google", "") != "" {
		t.Fatal("unreachable")
	}
}
